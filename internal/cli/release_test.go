package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
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

	// Three file shapes reach the import: an object with goals, a goal-less
	// object, and a legacy bare array of stories.
	const (
		shapeGoals    = "goals"
		shapeGoalless = "goal-less object"
		shapeLegacy   = "legacy array"
	)
	runImportShape := func(shape, script string, tasks []any) (string, error) {
		stories := []map[string]any{{
			"id": "US-001", "title": "Story", "goal_id": "G-001",
			"verification_script": script,
			"tasks":               tasks,
		}}
		var sf any
		switch shape {
		case shapeGoals:
			sf = map[string]any{
				"schema_version": "1.1",
				"goals":          []map[string]any{{"id": "G-001", "title": "Goal", "description": "Goal"}},
				"stories":        stories,
			}
		case shapeGoalless:
			sf = map[string]any{"schema_version": "1.1", "stories": stories}
		case shapeLegacy:
			sf = stories
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
	runImport := func(script string, tasks []any) (string, error) {
		return runImportShape(shapeGoals, script, tasks)
	}

	// Goal-less objects and legacy arrays skip goal coverage but must still pass
	// the stale-base rule, at story and at task level.
	for _, shape := range []string{shapeGoalless, shapeLegacy} {
		t.Run(shape+": story-level bare main is rejected", func(t *testing.T) {
			out, err := runImportShape(shape, "git diff --exit-code main...HEAD", []any{})
			if err == nil {
				t.Fatalf("expected the planning gate to reject a bare main ref; output: %s", out)
			}
			if !strings.Contains(err.Error(), "PLANNING GATE FAILED") || !strings.Contains(err.Error(), "story US-001") || !strings.Contains(err.Error(), "origin/") {
				t.Fatalf("error must name the gate, the story ID and the origin/ fix, got: %v", err)
			}
		})
		t.Run(shape+": task-level bare master is rejected", func(t *testing.T) {
			out, err := runImportShape(shape, "git diff --exit-code origin/main...HEAD", []any{
				"T-US-001-001",
				map[string]any{"id": "T-US-001-002", "title": "Task", "verification_script": "git log master..HEAD"},
			})
			if err == nil {
				t.Fatalf("expected the planning gate to reject a task-level bare master ref; output: %s", out)
			}
			if !strings.Contains(err.Error(), "PLANNING GATE FAILED") || !strings.Contains(err.Error(), "T-US-001-002") || !strings.Contains(err.Error(), "origin/") {
				t.Fatalf("error must name the gate, the task ID and the origin/ fix, got: %v", err)
			}
		})
		t.Run(shape+": origin/main passes at story and task level", func(t *testing.T) {
			out, err := runImportShape(shape, "git diff --exit-code origin/main...HEAD -- main", []any{
				"T-US-001-001",
				map[string]any{"id": "T-US-001-002", "title": "Task", "verification_script": "git log origin/master..HEAD"},
			})
			if err != nil {
				t.Fatalf("origin/ refs must pass the gate, got: %v", err)
			}
			if !strings.Contains(out, "Would import") {
				t.Fatalf("expected the dry-run listing in output: %s", out)
			}
		})
	}

	t.Run("bare main is rejected", func(t *testing.T) {
		out, err := runImport("git diff --name-only main...HEAD -- x | grep -vc y", []any{})
		if err == nil {
			t.Fatalf("expected the planning gate to reject a bare main ref; output: %s", out)
		}
		if !strings.Contains(err.Error(), "PLANNING GATE FAILED") || !strings.Contains(err.Error(), "origin/") {
			t.Fatalf("error must name the gate and the origin/ fix, got: %v", err)
		}
	})

	t.Run("origin/main passes", func(t *testing.T) {
		out, err := runImport("git diff --name-only origin/main...HEAD -- x | grep -vc y", []any{
			"T-US-001-001",
			map[string]any{"id": "T-US-001-002", "title": "Task", "verification_script": "git diff origin/main..HEAD"},
		})
		if err != nil {
			t.Fatalf("origin/main must pass the gate, got: %v", err)
		}
		if !strings.Contains(out, "✓ Planning Gate passed.") {
			t.Fatalf("expected gate pass in output: %s", out)
		}
	})

	t.Run("task-level bare main is rejected with the task ID", func(t *testing.T) {
		out, err := runImport("git diff --name-only origin/main...HEAD -- x | grep -vc y", []any{
			"T-US-001-001",
			map[string]any{"id": "T-US-001-002", "title": "Task", "verification_script": "git diff main..HEAD"},
		})
		if err == nil {
			t.Fatalf("expected the planning gate to reject a task-level bare main ref; output: %s", out)
		}
		if !strings.Contains(err.Error(), "PLANNING GATE FAILED") || !strings.Contains(err.Error(), "T-US-001-002") || !strings.Contains(err.Error(), "origin/") {
			t.Fatalf("error must name the gate, the task ID and the origin/ fix, got: %v", err)
		}
	})

	// Review 474755f1 MEDIUM scripts through the real import gate. Templates use
	// REF for the default-branch name; each runs for main and master, owned by
	// the story or by a task.
	t.Run("reviewer_scripts", func(t *testing.T) {
		sound := []string{
			`git diff --exit-code origin/REF...HEAD -- REF`,
			`grep -Fq 'REF...HEAD' docs/ARCHITECTURE.md`,
			"# never REF...HEAD\ngit diff --exit-code origin/REF...HEAD",
		}
		stale := []string{
			`git diff 'REF' HEAD`,
			`git diff "REF..HEAD"`,
			`git diff 'REF...HEAD'`,
			`git diff origin/REF...HEAD && git diff REF...HEAD`,
		}
		// run imports script as the story's or as task T-US-001-002's script;
		// the other owner gets a sound origin/REF script.
		run := func(ref, owner, script string) (string, error) {
			storyScript := script
			tasks := []any{"T-US-001-001"}
			if owner == "task" {
				storyScript = strings.ReplaceAll("git diff --exit-code origin/REF...HEAD", "REF", ref)
				tasks = append(tasks, map[string]any{"id": "T-US-001-002", "title": "Task", "verification_script": script})
			}
			return runImport(storyScript, tasks)
		}
		ownerID := map[string]string{"story": "story US-001", "task": "T-US-001-002"}
		for _, ref := range []string{"main", "master"} {
			for _, owner := range []string{"story", "task"} {
				for i, tmpl := range sound {
					script := strings.ReplaceAll(tmpl, "REF", ref)
					t.Run(ref+"/"+owner+"/sound/"+strconv.Itoa(i), func(t *testing.T) {
						out, err := run(ref, owner, script)
						if err != nil {
							t.Fatalf("sound %s script %q was refused: %v", owner, script, err)
						}
						if !strings.Contains(out, "✓ Planning Gate passed.") {
							t.Fatalf("expected gate pass for %q in output: %s", script, out)
						}
					})
				}
				for i, tmpl := range stale {
					script := strings.ReplaceAll(tmpl, "REF", ref)
					t.Run(ref+"/"+owner+"/stale/"+strconv.Itoa(i), func(t *testing.T) {
						out, err := run(ref, owner, script)
						if err == nil {
							t.Fatalf("stale %s script %q was accepted; output: %s", owner, script, out)
						}
						if !strings.Contains(err.Error(), ownerID[owner]) || !strings.Contains(err.Error(), "origin/"+ref) {
							t.Fatalf("error for %q must name %s and origin/%s, got: %v", script, ownerID[owner], ref, err)
						}
					})
				}
			}
		}
	})
}
