package validation

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// The graph deploy script runs against stubbed systemctl, curl, make and npm.
// The stubs model the daemon: a restart makes it report whatever commit the
// installed binary reports, so each case checks which build is left running.
const (
	stubSystemctl = `#!/usr/bin/env bash
n=$(( $(cat "$STUB_STATE/restarts" 2>/dev/null || echo 0) + 1 ))
echo "$n" > "$STUB_STATE/restarts"
if [ "$n" = "${STUB_FAIL_RESTART:-0}" ]; then exit 1; fi
if [ -x "$STUB_BINARY" ]; then "$STUB_BINARY" version | sed -n 's/^Commit: *//p' > "$STUB_STATE/running"; else : > "$STUB_STATE/running"; fi
`
	stubCurl = `#!/usr/bin/env bash
for arg in "$@"; do
  case "$arg" in
    */api/health) printf '{"status":"ok","commit":"%s"}' "$(cat "$STUB_STATE/running" 2>/dev/null)"; exit 0 ;;
    */api/v1/repository-graph/*) printf '%s' "${STUB_UNAUTH_STATUS:-401}"; exit 0 ;;
  esac
done
exit 7
`
	stubMake = `#!/usr/bin/env bash
dir="$2"; commit="${4#COMMIT=}"
echo built > "$STUB_STATE/make"
mkdir -p "$dir/bin"
cat > "$dir/bin/openexec" <<EOF
#!/usr/bin/env bash
if [ "\$1" = version ]; then echo "Commit: $commit"; exit 0; fi
mode=walk; out=""
while [ \$# -gt 0 ]; do case "\$1" in --requery) mode=requery ;; --evidence) out="\$2" ;; esac; shift; done
echo "\$mode" >> "$STUB_STATE/journey.log"
[ -n "\$out" ] && echo "{\"mode\":\"\$mode\"}" > "\$out"
[ "\$mode" = "\${STUB_FAIL_JOURNEY:-none}" ] && exit 1
exit 0
EOF
chmod +x "$dir/bin/openexec"
`
	stubOldBinary = "#!/usr/bin/env bash\necho 'Commit: old'\n"
)

type deployFixture struct {
	repo, home, state, binary string
	env                       []string
}

func newDeployFixture(t *testing.T, withPrevious bool) deployFixture {
	t.Helper()
	root := t.TempDir()
	f := deployFixture{repo: filepath.Join(root, "repo"), home: filepath.Join(root, "home"), state: filepath.Join(root, "state")}
	f.binary = filepath.Join(f.home, ".local", "bin", "openexec")
	stubs := filepath.Join(root, "stubs")
	for _, dir := range []string{filepath.Join(f.repo, "scripts"), filepath.Join(f.home, ".agent-console"), f.state, stubs, filepath.Dir(f.binary)} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	script, err := os.ReadFile(filepath.Join("..", "..", "scripts", "reviewed-deploy-graph.sh"))
	if err != nil {
		t.Fatal(err)
	}
	writeExecutable(t, filepath.Join(f.repo, "scripts", "reviewed-deploy-graph.sh"), string(script))
	for name, body := range map[string]string{"systemctl": stubSystemctl, "curl": stubCurl, "make": stubMake, "npm": "#!/usr/bin/env bash\nexit 0\n"} {
		writeExecutable(t, filepath.Join(stubs, name), body)
	}
	if withPrevious {
		writeExecutable(t, f.binary, stubOldBinary)
		if err := os.WriteFile(filepath.Join(f.state, "running"), []byte("old"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(f.home, ".agent-console", "openexec-evidence.env"), []byte("OPENEXEC_REPOSITORY_GRAPH_TOKEN=test\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"init", "-q"}, {"add", "."}, {"-c", "user.email=t@example.com", "-c", "user.name=t", "commit", "-qm", "merge"}, {"checkout", "-q", "--detach"}} {
		if out, err := exec.Command("git", append([]string{"-C", f.repo}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	f.env = append(os.Environ(), "HOME="+f.home, "PATH="+stubs+":"+os.Getenv("PATH"), "STUB_STATE="+f.state, "STUB_BINARY="+f.binary,
		"AGENT_CONSOLE_DEPLOY_REPOSITORY=", "AGENT_CONSOLE_DEPLOY_WORKTREE=")
	return f
}

func writeExecutable(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
}

func (f deployFixture) run(t *testing.T, extraEnv ...string) (string, error) {
	t.Helper()
	cmd := exec.Command("bash", filepath.Join(f.repo, "scripts", "reviewed-deploy-graph.sh"), "--wait-seconds", "4")
	cmd.Env = append(append([]string{}, f.env...), extraEnv...)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func (f deployFixture) read(t *testing.T, name string) string {
	t.Helper()
	raw, _ := os.ReadFile(filepath.Join(f.state, name))
	return strings.TrimSpace(string(raw))
}

func (f deployFixture) installedCommit(t *testing.T) string {
	t.Helper()
	out, err := exec.Command(f.binary, "version").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(string(out)), "Commit:"))
}

func TestReviewedGraphDeployWalksJourneyAndReload(t *testing.T) {
	f := newDeployFixture(t, true)
	out, err := f.run(t)
	if err != nil {
		t.Fatalf("deploy failed: %v\n%s", err, out)
	}
	commit := f.read(t, "running")
	if commit == "old" || commit == "" || f.installedCommit(t) != commit {
		t.Fatalf("running=%q installed=%q\n%s", commit, f.installedCommit(t), out)
	}
	if got := f.read(t, "journey.log"); got != "walk\nrequery" {
		t.Fatalf("journey calls = %q, want walk then requery", got)
	}
	if got := f.read(t, "restarts"); got != "2" {
		t.Fatalf("restarts = %s, want deploy plus reload", got)
	}
	evidence, _ := filepath.Glob(filepath.Join(f.home, ".agent-console", "deploy-evidence", "openexec-graph", commit+"-*", "*.json"))
	if len(evidence) != 2 {
		t.Fatalf("evidence files = %v", evidence)
	}
}

func TestReviewedGraphDeployRollsBack(t *testing.T) {
	cases := map[string][]string{
		"restart fails":           {"STUB_FAIL_RESTART=1"},
		"unauthenticated allowed": {"STUB_UNAUTH_STATUS=200"},
		"journey fails":           {"STUB_FAIL_JOURNEY=walk"},
		"reload restart fails":    {"STUB_FAIL_RESTART=2"},
		"requery fails":           {"STUB_FAIL_JOURNEY=requery"},
	}
	for name, env := range cases {
		t.Run(name, func(t *testing.T) {
			f := newDeployFixture(t, true)
			out, err := f.run(t, env...)
			if err == nil {
				t.Fatalf("deploy succeeded:\n%s", out)
			}
			if !strings.Contains(out, "rolling back") {
				t.Fatalf("no rollback:\n%s", out)
			}
			if got := f.installedCommit(t); got != "old" {
				t.Fatalf("installed binary = %q after rollback, want old\n%s", got, out)
			}
			if got := f.read(t, "running"); got != "old" {
				t.Fatalf("running = %q after rollback, want old\n%s", got, out)
			}
		})
	}
}

func TestReviewedGraphDeployWithoutPreviousRemovesUnverifiedBuild(t *testing.T) {
	f := newDeployFixture(t, false)
	out, err := f.run(t, "STUB_FAIL_JOURNEY=walk")
	if err == nil {
		t.Fatalf("deploy succeeded:\n%s", out)
	}
	if _, statErr := os.Stat(f.binary); !os.IsNotExist(statErr) {
		t.Fatalf("unverified binary left installed (stat err %v)\n%s", statErr, out)
	}
}

func TestReviewedGraphDeployRefusesWithoutGraphBearer(t *testing.T) {
	f := newDeployFixture(t, true)
	if err := os.Remove(filepath.Join(f.home, ".agent-console", "openexec-evidence.env")); err != nil {
		t.Fatal(err)
	}
	out, err := f.run(t)
	if err == nil || !strings.Contains(out, "graph bearer file") {
		t.Fatalf("err=%v\n%s", err, out)
	}
	if f.read(t, "make") != "" || f.installedCommit(t) != "old" {
		t.Fatalf("deploy built or installed without a bearer\n%s", out)
	}
}
