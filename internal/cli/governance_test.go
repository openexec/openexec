package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// runGov executes a governance command against a temp project dir and returns
// combined output. It uses the --project-dir flag so it does not depend on cwd.
func runGov(t *testing.T, baseDir string, args ...string) (string, error) {
	t.Helper()
	full := append([]string{"governance", "--project-dir", baseDir}, args...)
	b := &bytes.Buffer{}
	rootCmd.SetOut(b)
	rootCmd.SetErr(b)
	rootCmd.SetArgs(full)
	err := rootCmd.Execute()
	return b.String(), err
}

// TestGovernanceCLI exercises the governance command tree end to end for the
// operations that need neither an AI provider nor the gh CLI: create a release,
// inspect its status, and query an empty approved-work backlog. The commands are
// thin adapters over internal/governance/service, so this verifies the wiring
// (store open, service construction, rendering) without re-testing service logic.
func TestGovernanceCLI(t *testing.T) {
	baseDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(baseDir, ".openexec"), 0o750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	t.Run("release create", func(t *testing.T) {
		out, err := runGov(t, baseDir, "release", "create", "R-TEST", "--name", "Test Release", "--owner", "perttu")
		if err != nil {
			t.Fatalf("create failed: %v\noutput: %s", err, out)
		}
		if !strings.Contains(out, "Created release R-TEST [draft]") {
			t.Errorf("unexpected output: %s", out)
		}
	})

	t.Run("release status", func(t *testing.T) {
		out, err := runGov(t, baseDir, "release", "status", "R-TEST")
		if err != nil {
			t.Fatalf("status failed: %v\noutput: %s", err, out)
		}
		if !strings.Contains(out, "Release: R-TEST [draft]") {
			t.Errorf("expected release header, got: %s", out)
		}
		if !strings.Contains(out, "Owner: perttu") {
			t.Errorf("expected owner, got: %s", out)
		}
		if !strings.Contains(out, "Change records (0)") {
			t.Errorf("expected zero change records, got: %s", out)
		}
	})

	t.Run("release status json", func(t *testing.T) {
		out, err := runGov(t, baseDir, "release", "status", "R-TEST", "--json")
		if err != nil {
			t.Fatalf("status --json failed: %v\noutput: %s", err, out)
		}
		if !strings.Contains(out, `"ID": "R-TEST"`) {
			t.Errorf("expected JSON release id, got: %s", out)
		}
	})

	t.Run("release status missing", func(t *testing.T) {
		_, err := runGov(t, baseDir, "release", "status", "R-NOPE")
		if err == nil {
			t.Fatalf("expected error for missing release")
		}
		if !strings.Contains(err.Error(), "not found") {
			t.Errorf("expected not-found error, got: %v", err)
		}
	})

	t.Run("work next empty backlog", func(t *testing.T) {
		out, err := runGov(t, baseDir, "work", "next", "--project", "demo")
		if err != nil {
			t.Fatalf("work next failed: %v\noutput: %s", err, out)
		}
		if !strings.Contains(out, "No approved work available.") {
			t.Errorf("expected empty backlog message, got: %s", out)
		}
	})
}
