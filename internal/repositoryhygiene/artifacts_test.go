package repositoryhygiene

import (
	"os/exec"
	"strings"
	"testing"
)

// Exercise Git's actual index and ignore rules, including forced additions.
// This runs in the ordinary Go suite, without the defectrepro build tag.
func TestBuildArtifactsUntrackedAndIgnored(t *testing.T) {
	root, err := exec.Command("git", "rev-parse", "--show-toplevel").Output()
	if err != nil {
		t.Fatal(err)
	}
	git := func(args ...string) []byte {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = strings.TrimSpace(string(root))
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
		return out
	}
	tracked := git("ls-files", "--", ".gocache", "bin/openexec", "openexec", "data/fwus.jso")
	if len(tracked) != 0 {
		t.Errorf("build artifacts must not be tracked: %s", tracked)
	}
	for _, path := range []string{".gocache/00/new-cache-entry", "bin/openexec", "openexec", "data/fwus.jso"} {
		git("check-ignore", "--no-index", "--", path)
	}
	// Exact output names must not hide source directories or legitimate data.
	for _, path := range []string{"cmd/openexec/main.go", "internal/cli/openexec.go", "data/fwus.json", "bin/helper.sh"} {
		cmd := exec.Command("git", "check-ignore", "--no-index", "--", path)
		cmd.Dir = strings.TrimSpace(string(root))
		out, err := cmd.CombinedOutput()
		if exit, ok := err.(*exec.ExitError); !ok || exit.ExitCode() != 1 {
			t.Errorf("source path %s must not be ignored: %v %s", path, err, out)
		}
	}
}
