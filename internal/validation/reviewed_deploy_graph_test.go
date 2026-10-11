package validation

import (
	"fmt"
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
if [ -x "$STUB_BINARY" ]; then "$STUB_BINARY" version 2>&1 | sed -n 's/^ *Commit: *//p' > "$STUB_STATE/running"; else : > "$STUB_STATE/running"; fi
`
	// STUB_HEALTH_DOWN_AT makes health unreachable while the daemon is on
	// that restart, modelling a build that starts but never answers.
	stubCurl = `#!/usr/bin/env bash
for arg in "$@"; do
  case "$arg" in
    */api/health)
      if [ "$(cat "$STUB_STATE/restarts" 2>/dev/null)" = "${STUB_HEALTH_DOWN_AT:-none}" ]; then exit 7; fi
      printf '{"status":"ok","commit":"%s"}' "$(cat "$STUB_STATE/running" 2>/dev/null)"; exit 0 ;;
    */api/v1/repository-graph/*) printf '%s' "${STUB_UNAUTH_STATUS:-401}"; exit 0 ;;
  esac
done
exit 7
`
	// The built stub logs each journey call with the commit it was told to
	// require, so a case can tell the deploy pinned the walk to its build.
	stubMake = `#!/usr/bin/env bash
dir="$2"; commit="${STUB_BUILD_COMMIT:-${4#COMMIT=}}"
echo built > "$STUB_STATE/make"
mkdir -p "$dir/bin"
cat > "$dir/bin/openexec" <<EOF
#!/usr/bin/env bash
if [ "\$1" = version ]; then printf 'OpenExec CLI v0.0.0\\n  Commit:     %s\\n' "$commit" >&2; exit 0; fi
mode=walk; out=""; want=""
while [ \$# -gt 0 ]; do case "\$1" in --requery) mode=requery ;; --evidence) out="\$2" ;; --want-commit) want="\$2" ;; esac; shift; done
echo "\$mode \$want" >> "$STUB_STATE/journey.log"
[ -n "\$out" ] && echo "{\"mode\":\"\$mode\",\"commit\":\"\$want\"}" > "\$out"
[ "\$mode" = "\${STUB_FAIL_JOURNEY:-none}" ] && exit 1
exit 0
EOF
chmod +x "$dir/bin/openexec"
`
	stubOldBinary = "#!/usr/bin/env bash\nprintf 'OpenExec CLI v0.0.0\\n  Commit:     old\\n' >&2\n"
	deployUnit    = "openexec-graph.service"
)

type deployFixture struct {
	repo, home, state, binary string
	commit                    string
	env                       []string
}

func deployScript(t *testing.T) string {
	t.Helper()
	script, err := os.ReadFile(filepath.Join("..", "..", "scripts", "reviewed-deploy-graph.sh"))
	if err != nil {
		t.Fatal(err)
	}
	return string(script)
}

func newDeployFixture(t *testing.T, withPrevious bool, script string) deployFixture {
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
	writeExecutable(t, filepath.Join(f.repo, "scripts", "reviewed-deploy-graph.sh"), script)
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
	f.git(t, "init", "-q")
	f.git(t, "add", ".")
	f.git(t, "-c", "user.email=t@example.com", "-c", "user.name=t", "commit", "-qm", "merge")
	f.git(t, "checkout", "-q", "--detach")
	f.commit = f.git(t, "rev-parse", "--short", "HEAD")
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

func (f deployFixture) git(t *testing.T, args ...string) string {
	t.Helper()
	out, err := exec.Command("git", append([]string{"-C", f.repo}, args...)...).CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func (f deployFixture) run(t *testing.T, extraEnv ...string) (string, error) {
	t.Helper()
	cmd := exec.Command("bash", filepath.Join(f.repo, "scripts", "reviewed-deploy-graph.sh"), "--wait-seconds", "2")
	cmd.Env = append(append([]string{}, f.env...), extraEnv...)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func (f deployFixture) read(name string) string {
	raw, _ := os.ReadFile(filepath.Join(f.state, name))
	return strings.TrimSpace(string(raw))
}

func (f deployFixture) installedCommit() string {
	// Like the real CLI, the stubs print their version to stderr.
	out, err := exec.Command(f.binary, "version").CombinedOutput()
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(out), "\n") {
		if rest, ok := strings.CutPrefix(strings.TrimSpace(line), "Commit:"); ok {
			return strings.TrimSpace(rest)
		}
	}
	return ""
}

func hasLine(out, prefix string) bool {
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(line, prefix) {
			return true
		}
	}
	return false
}

// mutation removes one guard from the script. Count pins how often the text
// occurs, so an edited script cannot leave the mutation silently unapplied.
type mutation struct {
	guard, from, to string
	count           int
}

func (m mutation) apply(t *testing.T, script string) string {
	t.Helper()
	if got := strings.Count(script, m.from); got != m.count {
		t.Fatalf("guard %q: %q occurs %d times in the script, want %d", m.guard, m.from, got, m.count)
	}
	return strings.ReplaceAll(script, m.from, m.to)
}

// deployCase is one branch of the script. check returns why the branch did
// not hold, or "" when it did; each mutation must make check fail.
type deployCase struct {
	name         string
	withPrevious bool
	setup        func(t *testing.T, f deployFixture) []string
	check        func(f deployFixture, out string, err error) string
	mutations    []mutation
}

func env(vars ...string) func(*testing.T, deployFixture) []string {
	return func(*testing.T, deployFixture) []string { return vars }
}

// refused holds when the script stopped before building, installing or
// touching the service.
func refused(reason string) func(deployFixture, string, error) string {
	return func(f deployFixture, out string, err error) string {
		switch {
		case err == nil:
			return "deploy succeeded"
		case !strings.Contains(out, reason):
			return fmt.Sprintf("refusal does not name %q", reason)
		case f.read("make") != "":
			return "built before refusing"
		case f.read("restarts") != "":
			return "restarted the service before refusing"
		case f.installedCommit() != "old":
			return fmt.Sprintf("installed %q, want old", f.installedCommit())
		}
		return ""
	}
}

// rolledBack holds when the named failure rolled back and the previous build
// is both installed and running again.
func rolledBack(reason string) func(deployFixture, string, error) string {
	return func(f deployFixture, out string, err error) string {
		switch {
		case err == nil:
			return "deploy succeeded"
		case !hasLine(out, reason) || !strings.Contains(out, "; rolling back"):
			return fmt.Sprintf("did not roll back for %q", reason)
		case f.installedCommit() != "old":
			return fmt.Sprintf("installed %q after rollback, want old", f.installedCommit())
		case f.read("running") != "old":
			return fmt.Sprintf("running %q after rollback, want old", f.read("running"))
		}
		return ""
	}
}

var (
	dropRollbackRestore = mutation{"rollback restores the previous binary", `mv -f "$previous" "$binary_path"`, `:`, 1}
	dropRollbackRestart = mutation{"rollback restarts the previous binary", "if ! systemctl --user restart \"$unit\"; then\n    echo \"rollback restart", "if ! true; then\n    echo \"rollback restart", 1}
)

func neutralize(guard, call string) mutation {
	return mutation{guard, `rollback "` + call, `: "` + call, 1}
}

func deployCases() []deployCase {
	return []deployCase{
		{
			name: "deploys, walks the journey pinned to its commit, and re-queries after reload", withPrevious: true,
			setup: env(),
			check: func(f deployFixture, out string, err error) string {
				commit := f.commit
				switch {
				case err != nil:
					return fmt.Sprintf("deploy failed: %v", err)
				case f.read("running") != commit || f.installedCommit() != commit:
					return fmt.Sprintf("running=%q installed=%q, want %s", f.read("running"), f.installedCommit(), commit)
				case f.read("journey.log") != "walk "+commit+"\nrequery "+commit:
					return fmt.Sprintf("journey calls = %q, want walk then requery pinned to %s", f.read("journey.log"), commit)
				case f.read("restarts") != "2":
					return fmt.Sprintf("restarts = %s, want deploy plus reload", f.read("restarts"))
				}
				evidence, _ := filepath.Glob(filepath.Join(f.home, ".agent-console", "deploy-evidence", "openexec-graph", commit+"-*", "*.json"))
				if len(evidence) != 2 {
					return fmt.Sprintf("evidence files = %v", evidence)
				}
				return ""
			},
			mutations: []mutation{{"journey is pinned to the installed commit", `--want-commit "$commit" `, ``, 2}},
		},
		{
			name: "refuses outside its server-minted worktree", withPrevious: true,
			setup: func(t *testing.T, f deployFixture) []string {
				return []string{"AGENT_CONSOLE_DEPLOY_WORKTREE=" + t.TempDir()}
			},
			check:     refused("outside its server-minted worktree"),
			mutations: []mutation{{"worktree guard", `[ "$repo_dir" != "$deploy_worktree" ]`, `false`, 1}},
		},
		{
			name: "refuses a branch checkout", withPrevious: true,
			setup: func(t *testing.T, f deployFixture) []string {
				f.git(t, "checkout", "-q", "-b", "topic")
				return nil
			},
			check:     refused("from a branch checkout"),
			mutations: []mutation{{"branch guard", `if git -C "$repo_dir" symbolic-ref`, `if false && git -C "$repo_dir" symbolic-ref`, 1}},
		},
		{
			name: "refuses a dirty worktree", withPrevious: true,
			setup: func(t *testing.T, f deployFixture) []string {
				if err := os.WriteFile(filepath.Join(f.repo, "stray.txt"), []byte("x"), 0o600); err != nil {
					t.Fatal(err)
				}
				return nil
			},
			check:     refused("dirty exact-merge worktree"),
			mutations: []mutation{{"dirty guard", `if [ -n "$(git -C "$repo_dir" status --porcelain --untracked-files=normal)" ]`, `if false`, 1}},
		},
		{
			name: "refuses without the graph bearer", withPrevious: true,
			setup: func(t *testing.T, f deployFixture) []string {
				if err := os.Remove(filepath.Join(f.home, ".agent-console", "openexec-evidence.env")); err != nil {
					t.Fatal(err)
				}
				return nil
			},
			check:     refused("graph bearer file"),
			mutations: []mutation{{"bearer guard", `if [ ! -r "$token_file" ]`, `if false`, 1}},
		},
		{
			name: "refuses a build that reports another commit", withPrevious: true,
			setup: env("STUB_BUILD_COMMIT=elsewhere"),
			check: func(f deployFixture, out string, err error) string {
				switch {
				case err == nil:
					return "deploy succeeded"
				case !strings.Contains(out, "built binary does not report commit"):
					return "refusal does not name the build commit"
				case f.read("restarts") != "" || f.installedCommit() != "old":
					return "installed or restarted a build of another commit"
				}
				return ""
			},
			mutations: []mutation{{"built commit guard", `if ! "$built" version 2>&1 | grep -q "Commit: *$commit\$"; then`, `if false; then`, 1}},
		},
		{
			name: "rolls back when the restart fails", withPrevious: true,
			setup:     env("STUB_FAIL_RESTART=1"),
			check:     rolledBack("restart of " + deployUnit + " failed"),
			mutations: []mutation{neutralize("restart guard", "restart of $unit failed")},
		},
		{
			name: "rolls back when the health check never reports the commit", withPrevious: true,
			setup:     env("STUB_HEALTH_DOWN_AT=1"),
			check:     rolledBack(deployUnit + " did not report commit"),
			mutations: []mutation{neutralize("health guard", "$unit did not report commit")},
		},
		{
			name: "rolls back when unauthenticated reads are allowed", withPrevious: true,
			setup:     env("STUB_UNAUTH_STATUS=200"),
			check:     rolledBack("unauthenticated graph read returned 200"),
			mutations: []mutation{neutralize("bearer-enforced guard", "unauthenticated graph read")},
		},
		{
			name: "rolls back when the live journey fails", withPrevious: true,
			setup:     env("STUB_FAIL_JOURNEY=walk"),
			check:     rolledBack("live Explore journey failed"),
			mutations: []mutation{neutralize("journey guard", "live Explore journey failed"), dropRollbackRestore, dropRollbackRestart},
		},
		{
			name: "rolls back when the reload restart fails", withPrevious: true,
			setup:     env("STUB_FAIL_RESTART=2"),
			check:     rolledBack("reload restart of " + deployUnit + " failed"),
			mutations: []mutation{neutralize("reload restart guard", "reload restart of")},
		},
		{
			name: "rolls back when the health check fails after reload", withPrevious: true,
			setup:     env("STUB_HEALTH_DOWN_AT=2"),
			check:     rolledBack(deployUnit + " did not come back on commit"),
			mutations: []mutation{neutralize("reload health guard", "$unit did not come back on commit")},
		},
		{
			name: "rolls back when the re-query after reload fails", withPrevious: true,
			setup:     env("STUB_FAIL_JOURNEY=requery"),
			check:     rolledBack("re-query after reload failed"),
			mutations: []mutation{neutralize("re-query guard", "re-query after reload failed")},
		},
		{
			name:  "removes an unverified build when there is no previous one",
			setup: env("STUB_FAIL_JOURNEY=walk"),
			check: func(f deployFixture, out string, err error) string {
				if err == nil {
					return "deploy succeeded"
				}
				if _, statErr := os.Stat(f.binary); !os.IsNotExist(statErr) {
					return fmt.Sprintf("unverified binary left installed (stat err %v)", statErr)
				}
				return ""
			},
			mutations: []mutation{{"unverified build removal", "  else\n    rm -f \"$binary_path\"\n", "  else\n    :\n", 1}},
		},
	}
}

func TestReviewedGraphDeployBranches(t *testing.T) {
	script := deployScript(t)
	for _, c := range deployCases() {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			f := newDeployFixture(t, c.withPrevious, script)
			out, err := f.run(t, c.setup(t, f)...)
			if failure := c.check(f, out, err); failure != "" {
				t.Fatalf("%s\nerr=%v\n%s", failure, err, out)
			}
		})
	}
}

// Each guard is proven necessary: with it removed from the script, the case
// that relies on it must no longer hold.
func TestReviewedGraphDeployGuardsAreLoadBearing(t *testing.T) {
	script := deployScript(t)
	for _, c := range deployCases() {
		for _, m := range c.mutations {
			t.Run(m.guard, func(t *testing.T) {
				t.Parallel()
				mutated := m.apply(t, script)
				if out, err := exec.Command("bash", "-n", "-c", mutated).CombinedOutput(); err != nil {
					t.Fatalf("mutated script does not parse, so it proves nothing: %v\n%s", err, out)
				}
				f := newDeployFixture(t, c.withPrevious, mutated)
				out, err := f.run(t, c.setup(t, f)...)
				failure := c.check(f, out, err)
				if failure == "" {
					t.Fatalf("case %q still holds with %q removed:\nerr=%v\n%s", c.name, m.guard, err, out)
				}
				t.Logf("without %s, %q fails: %s", m.guard, c.name, failure)
			})
		}
	}
}
