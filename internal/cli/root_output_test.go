package cli

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/openexec/openexec/pkg/version"
)

// Run Execute in a fresh process so other tests' SetOut calls cannot mask
// missing production wiring, and Execute's os.Exit can be tested safely.
func TestExecuteOutput(t *testing.T) {
	if os.Getenv("OPENEXEC_TEST_EXECUTE_OUTPUT") == "1" {
		for i, arg := range os.Args {
			if arg == "--" {
				os.Args = append([]string{"openexec"}, os.Args[i+1:]...)
				Execute()
				os.Exit(0)
			}
		}
		t.Fatal("missing CLI arguments")
	}

	for _, command := range []string{"version", "unknown-d2-command"} {
		t.Run(command, func(t *testing.T) {
			cmd := exec.Command(os.Args[0], "-test.run=^TestExecuteOutput$", "--", command)
			cmd.Env = append(os.Environ(), "OPENEXEC_TEST_EXECUTE_OUTPUT=1", "HOME="+t.TempDir())
			cmd.Dir = t.TempDir()
			var stdout, stderr bytes.Buffer
			cmd.Stdout, cmd.Stderr = &stdout, &stderr
			err := cmd.Run()
			if command == "version" {
				if err != nil {
					t.Fatalf("Execute version: %v; stderr=%q", err, stderr.String())
				}
				want := fmt.Sprintf("OpenExec CLI v%s\n  Commit:     %s\n  Build Date: %s\n", version.Version, version.Commit, BuildDate)
				if stdout.String() != want || stderr.Len() != 0 {
					t.Fatalf("want stdout=%q and empty stderr; got stdout=%q stderr=%q", want, stdout.String(), stderr.String())
				}
			} else {
				if exit, ok := err.(*exec.ExitError); !ok || exit.ExitCode() != 1 {
					t.Fatalf("want exit 1, got %v", err)
				}
				if stdout.Len() != 0 || !strings.Contains(stderr.String(), `unknown command "unknown-d2-command"`) {
					t.Fatalf("want error on stderr only; got stdout=%q stderr=%q", stdout.String(), stderr.String())
				}
			}
		})
	}
}
