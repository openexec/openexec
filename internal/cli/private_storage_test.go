package cli

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/openexec/openexec/internal/blueprint"
	"github.com/openexec/openexec/internal/execution/gates"
	"github.com/openexec/openexec/pkg/runtime"
)

// Real initialization ignore rules, real failing subprocesses, disk reads and
// Git's actual index form the storage-to-source-commit boundary.
func TestPrivateStorageRepositoryJourney(t *testing.T) {
	for _, layout := range []string{"fresh", "initialized", "whole-tree"} {
		for _, kind := range []string{"gate", "deterministic"} {
			t.Run(layout+"/"+kind, func(t *testing.T) {
				dir := t.TempDir()
				git := func(args ...string) string {
					t.Helper()
					cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
					out, err := cmd.CombinedOutput()
					if err != nil {
						t.Fatalf("git %s failed", args[0])
					}
					return string(out)
				}
				write := func(name, value string) {
					t.Helper()
					if err := os.WriteFile(filepath.Join(dir, name), []byte(value), 0600); err != nil {
						t.Fatal(err)
					}
				}
				git("init", "-q")
				switch layout {
				case "fresh":
					ensureGitignore(dir)
				case "initialized":
					write(".gitignore", "# OpenExec Managed Block\n.openexec/logs/\n.openexec/data/\n.openexec/engram/cache/\n")
					// No reinitialization: existing installations must already be protected.
					if err := os.MkdirAll(filepath.Join(dir, ".openexec", "data"), 0755); err != nil {
						t.Fatal(err)
					}
				case "whole-tree":
					write(".gitignore", ".openexec/\n")
				}
				config := "quality:\n  gates: [test]\n  custom:\n    - name: test\n      command: 'exit 2'\n"
				write("openexec.yaml", config)
				git("add", ".")
				git("-c", "user.name=Fixture", "-c", "user.email=fixture@example.invalid", "commit", "-qm", "baseline")
				if git("status", "--porcelain", "--untracked-files=all") != "" {
					t.Fatal("baseline is dirty")
				}
				secret := rand.Text()
				command := "TOKEN=" + secret + "; printf 'token=%s\\n' \"$TOKEN\" >&2; exit 2"
				var refs map[string]string
				var public []byte
				if kind == "gate" {
					configured, _ := json.Marshal(map[string]any{"quality": map[string]any{"gates": []string{"test"}, "custom": []map[string]string{{"name": "test", "command": command}}}})
					write("openexec.yaml", string(configured))
					runner, err := gates.NewRunner(dir, 10*time.Second)
					if err != nil {
						t.Fatal(err)
					}
					write("openexec.yaml", config) // Restore baseline before executing; never stage private argv.
					report := runner.RunAll(context.Background())
					if report.Passed || len(report.Results) != 1 || report.Results[0].ExitCode != 2 {
						t.Fatal("expected real gate failure")
					}
					refs = gates.VerificationFailureArtifacts(gates.NewFailure(report))
					public, _ = json.Marshal(report)
				} else {
					executor := blueprint.NewDefaultExecutor(dir)
					stage := &blueprint.Stage{Name: "verify", Type: runtime.StageTypeDeterministic, Commands: []string{command}, Timeout: 10 * time.Second}
					executor.VerificationStages = map[*blueprint.Stage]bool{stage: true}
					executor.OnVerificationFailure = func(_ *blueprint.Stage, err error) { refs = gates.VerificationFailureArtifacts(err) }
					result, err := executor.Execute(context.Background(), stage, blueprint.NewStageInput("private-storage", "", dir))
					if err != nil || result == nil || result.Status != runtime.StageStatusFailed {
						t.Fatal("expected deterministic failure")
					}
					public, _ = json.Marshal(result)
				}
				if !gates.ValidateVerificationFailureArtifacts(refs) {
					t.Fatal("receipt missing")
				}
				encoded, _ := json.Marshal(refs)
				if strings.Contains(string(public)+string(encoded), secret) {
					t.Fatal("public credential leak")
				}
				count := 0
				for hash, path := range refs {
					if len(hash) != 64 {
						continue
					}
					count++
					if path != filepath.Join(dir, ".openexec", "data", "verification", hash+".json") {
						t.Error("unprotected evidence path")
					}
					private, err := runtime.ReadCommandEvidence(dir, hash)
					if err != nil || private.ExitCode != 2 || private.Cwd != dir || !strings.Contains(private.Stderr, secret) {
						t.Fatal("private persisted evidence lost")
					}
					if len(private.Argv) != 3 || private.Argv[2] != command {
						t.Fatal("private argv lost")
					}
					for p, mode := range map[string]os.FileMode{filepath.Dir(path): 0700, path: 0600} {
						info, err := os.Stat(p)
						if err != nil || info.Mode().Perm() != mode {
							t.Fatal("private permissions lost")
						}
					}
					if err := exec.Command("git", "-C", dir, "check-ignore", "-q", path).Run(); err != nil {
						t.Error("private artifact not ignored")
					}
				}
				if count != 1 {
					t.Fatal("private reference missing")
				}
				if git("status", "--porcelain", "--untracked-files=all") != "" {
					t.Error("capture dirtied repository")
				}
				git("add", ".")
				if git("diff", "--cached", "--name-only") != "" {
					t.Error("capture entered index")
				}
				if strings.Contains(git("ls-files"), "verification") {
					t.Fatal("private artifact tracked")
				}
			})
		}
	}
}
