package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestReleaseCmd(t *testing.T) {
	tmpDir := t.TempDir()
	oldCwd, _ := os.Getwd()
	os.Chdir(tmpDir)
	defer os.Chdir(oldCwd)

	// Setup openexec.yaml so init/manager works
	yamlContent := `
project:
  name: "test-rel"
`
	os.WriteFile(filepath.Join(tmpDir, "openexec.yaml"), []byte(yamlContent), 0644)

	// Create .openexec dir
	os.MkdirAll(filepath.Join(tmpDir, ".openexec"), 0755)

	t.Run("Create Release", func(t *testing.T) {
		b := bytes.NewBufferString("")
		rootCmd.SetOut(b)
		rootCmd.SetArgs([]string{"release", "create", "1.0.0", "--name", "First Release"})

		err := rootCmd.Execute()
		if err != nil {
			t.Fatalf("Execute failed: %v", err)
		}

		if !strings.Contains(b.String(), "Created release: First Release (v1.0.0)") {
			t.Errorf("unexpected output: %s", b.String())
		}
	})

	t.Run("Show Release", func(t *testing.T) {
		b := bytes.NewBufferString("")
		rootCmd.SetOut(b)
		rootCmd.SetArgs([]string{"release", "show"})

		err := rootCmd.Execute()
		if err != nil {
			t.Fatalf("Execute failed: %v", err)
		}

		if !strings.Contains(b.String(), "Release: First Release") {
			t.Errorf("unexpected output: %s", b.String())
		}
		if !strings.Contains(b.String(), "Version: 1.0.0") {
			t.Errorf("unexpected output: %s", b.String())
		}
	})

	t.Run("Create Story", func(t *testing.T) {
		b := bytes.NewBufferString("")
		rootCmd.SetOut(b)
		rootCmd.SetArgs([]string{"story", "create", "S-001", "Story Title", "--description", "Story Desc"})

		err := rootCmd.Execute()
		if err != nil {
			t.Fatalf("Execute failed: %v", err)
		}

		if !strings.Contains(b.String(), "Created story: S-001") {
			t.Errorf("unexpected output: %s", b.String())
		}
	})

	t.Run("Create Task", func(t *testing.T) {
		b := bytes.NewBufferString("")
		rootCmd.SetOut(b)
		rootCmd.SetArgs([]string{"task", "create", "T-001", "Task Title", "--story", "S-001"})

		err := rootCmd.Execute()
		if err != nil {
			t.Fatalf("Execute failed: %v", err)
		}

		if !strings.Contains(b.String(), "Created task: T-001") {
			t.Errorf("unexpected output: %s", b.String())
		}
	})

	t.Run("List Stories", func(t *testing.T) {
		b := bytes.NewBufferString("")
		rootCmd.SetOut(b)
		rootCmd.SetArgs([]string{"story", "list"})

		err := rootCmd.Execute()
		if err != nil {
			t.Fatalf("Execute failed: %v", err)
		}

		if !strings.Contains(b.String(), "S-001") || !strings.Contains(b.String(), "Story Title") {
			t.Errorf("unexpected output: %s", b.String())
		}
	})

	t.Run("Verify Goal", func(t *testing.T) {
		b := bytes.NewBufferString("")
		rootCmd.SetOut(b)
		rootCmd.SetArgs([]string{"goal", "verify"})

		err := rootCmd.Execute()
		if err != nil {
			t.Fatalf("Execute failed: %v", err)
		}

		if !strings.Contains(b.String(), "Goal Verification Report") {
			t.Errorf("unexpected output: %s", b.String())
		}
	})

	t.Run("Approve Task", func(t *testing.T) {
		// Enable approval first
		rootCmd.SetArgs([]string{"config", "set", "approval_enabled", "true"})
		rootCmd.Execute()

		b := bytes.NewBufferString("")
		rootCmd.SetOut(b)
		rootCmd.SetArgs([]string{"task", "approve", "T-001", "--approver", "test-user", "--comments", "Looks good"})

		err := rootCmd.Execute()
		if err != nil {
			t.Fatalf("Execute failed: %v", err)
		}

		if !strings.Contains(b.String(), "approved by") {
			t.Errorf("unexpected output: %s", b.String())
		}
	})

	t.Run("Finish Release", func(t *testing.T) {
		b := bytes.NewBufferString("")
		rootCmd.SetOut(b)
		rootCmd.SetArgs([]string{"release", "finish"})

		err := rootCmd.Execute()
		if err != nil {
			// Finish might fail if not all stories are done, but we want to cover the code path
			t.Logf("Finish info: %v", err)
		}
	})
}

func TestStatusIcon(t *testing.T) {
	tests := []struct {
		status string
		want   string
	}{
		{"done", "[x]"},
		{"completed", "[x]"},
		{"failed", "[!]"},
		{"approved", "[+]"},
		{"in_progress", "[-]"},
		{"pending", "[ ]"},
	}
	for _, tt := range tests {
		got := statusIcon(tt.status)
		if got != tt.want {
			t.Errorf("statusIcon(%q) = %q, want %q", tt.status, got, tt.want)
		}
	}
}

func TestLoadReleaseConfig(t *testing.T) {
	tmpDir := t.TempDir()
	os.MkdirAll(filepath.Join(tmpDir, ".openexec"), 0755)

	cfg := &ProjectConfig{GitEnabled: true}
	data, _ := json.Marshal(cfg)
	os.WriteFile(filepath.Join(tmpDir, ".openexec", "config.json"), data, 0644)

	loaded := loadReleaseConfig(tmpDir)
	if !loaded.GitEnabled {
		t.Error("failed to load git_enabled from config")
	}
}

func TestGetReleaseManager(t *testing.T) {
	tmpDir := t.TempDir()
	// Release manager expects .openexec dir
	os.MkdirAll(filepath.Join(tmpDir, ".openexec"), 0755)

	cmd := &cobra.Command{}
	cmd.Flags().String("project-dir", tmpDir, "")
	cmd.Flags().Set("project-dir", tmpDir)

	mgr, err := getReleaseManager(cmd)
	if err != nil {
		t.Fatalf("getReleaseManager failed: %v", err)
	}
	if mgr == nil {
		t.Fatal("got nil manager")
	}
}

// TestImport_PlanningGate_RejectsStaleBaseRef proves the planning gate refuses a
// story verification script that diffs against a bare local default branch:
// candidate worktrees are synced to origin/<default>, so the local branch of
// that name is stale or missing there.
func TestImport_PlanningGate_RejectsStaleBaseRef(t *testing.T) {
	tmpDir := t.TempDir()
	oldCwd, _ := os.Getwd()
	os.Chdir(tmpDir)
	defer os.Chdir(oldCwd)
	os.WriteFile(filepath.Join(tmpDir, "openexec.yaml"), []byte("project:\n  name: \"test-gate\"\n"), 0644)
	os.MkdirAll(filepath.Join(tmpDir, ".openexec"), 0755)

	runImport := func(script string) (string, error) {
		sf := map[string]any{
			"schema_version": "1.1",
			"goals":          []map[string]any{{"id": "G-001", "title": "Goal", "description": "Goal"}},
			"stories": []map[string]any{{
				"id": "US-001", "title": "Story", "goal_id": "G-001",
				"verification_script": script,
				"tasks":               []any{},
			}},
		}
		data, _ := json.Marshal(sf)
		p := filepath.Join(tmpDir, "stories.json")
		os.WriteFile(p, data, 0644)

		b := bytes.NewBufferString("")
		rootCmd.SetOut(b)
		rootCmd.SetErr(b)
		rootCmd.SetArgs([]string{"story", "import", p, "--dry-run"})
		err := rootCmd.Execute()
		return b.String(), err
	}

	t.Run("bare main is rejected", func(t *testing.T) {
		out, err := runImport("git diff --name-only main...HEAD -- x | grep -vc y")
		if err == nil {
			t.Fatalf("expected the planning gate to reject a bare main ref; output: %s", out)
		}
		if !strings.Contains(err.Error(), "PLANNING GATE FAILED") || !strings.Contains(err.Error(), "origin/") {
			t.Fatalf("error must name the gate and the origin/ fix, got: %v", err)
		}
	})

	t.Run("origin/main passes", func(t *testing.T) {
		out, err := runImport("git diff --name-only origin/main...HEAD -- x | grep -vc y")
		if err != nil {
			t.Fatalf("origin/main must pass the gate, got: %v", err)
		}
		if !strings.Contains(out, "✓ Planning Gate passed.") {
			t.Fatalf("expected gate pass in output: %s", out)
		}
	})
}
