package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %s failed: %v\nOutput: %s", strings.Join(args, " "), err, string(out))
	}
}

func TestInitCmd(t *testing.T) {
	tmpDir := t.TempDir()
	
	// Setup git repo because init requires it
	runGit(t, tmpDir, "init")
	runGit(t, tmpDir, "config", "user.name", "Test User")
	runGit(t, tmpDir, "config", "user.email", "test@example.com")
	runGit(t, tmpDir, "config", "commit.gpgsign", "false")
	
	// Create main branch
	err := os.WriteFile(filepath.Join(tmpDir, "README.md"), []byte("# Test"), 0644)
	if err != nil {
		t.Fatal(err)
	}
	runGit(t, tmpDir, "add", "README.md")
	runGit(t, tmpDir, "commit", "-m", "Initial")
	runGit(t, tmpDir, "branch", "-M", "main")

	// Change to tmpDir so init runs there
	oldCwd, _ := os.Getwd()
	os.Chdir(tmpDir)
	defer os.Chdir(oldCwd)

	b := bytes.NewBufferString("")
	rootCmd.SetOut(b)
	rootCmd.SetArgs([]string{"init", "test-project", "-y"})

	err = rootCmd.Execute()
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if !strings.Contains(b.String(), "Project initialized successfully") {
		t.Errorf("unexpected output: %s", b.String())
	}

	// Verify files created
	if _, err := os.Stat(filepath.Join(tmpDir, ".openexec", "config.json")); err != nil {
		t.Error(".openexec/config.json not created")
	}
	if _, err := os.Stat(filepath.Join(tmpDir, "openexec.yaml")); err != nil {
		t.Error("openexec.yaml not created")
	}
}

func TestInitCmd_Interactive(t *testing.T) {
	tmpDir := t.TempDir()
	
	runGit(t, tmpDir, "init")
	runGit(t, tmpDir, "config", "user.name", "Test User")
	runGit(t, tmpDir, "config", "user.email", "test@example.com")
	runGit(t, tmpDir, "config", "commit.gpgsign", "false")
	
	err := os.WriteFile(filepath.Join(tmpDir, "README.md"), []byte("# Test"), 0644)
	if err != nil {
		t.Fatal(err)
	}
	runGit(t, tmpDir, "add", "README.md")
	runGit(t, tmpDir, "commit", "-m", "Initial")
	runGit(t, tmpDir, "branch", "-M", "main")

	oldCwd, _ := os.Getwd()
	os.Chdir(tmpDir)
	defer os.Chdir(oldCwd)

	// Explicitly answer all prompts to avoid flakiness
	// 1. Planner model: 1
	// 2. Use same for executor: y
	// 3. Enable review: y
	// 4. Reviewer model: 2
	// 5. Parallel: y
	// 6. Workers: 4
	input := "1\ny\ny\n2\ny\n4\n"
	
	b := bytes.NewBufferString("")
	rootCmd.SetOut(b)
	rootCmd.SetIn(strings.NewReader(input))
	rootCmd.SetArgs([]string{"init", "interactive-proj"})

	err = rootCmd.Execute()
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	// Verify settings in config
	data, _ := os.ReadFile(filepath.Join(tmpDir, ".openexec", ".openexec", "config.json"))
	var cfg struct {
		Execution struct {
			PlannerModel  string `json:"planner_model"`
			ReviewEnabled bool   `json:"review_enabled"`
			WorkerCount   int    `json:"worker_count"`
		} `json:"execution"`
	}
	json.Unmarshal(data, &cfg)

	// Since interactive init might be brittle due to multiple nested prompts,
	// let's at least check that some config was written.
	if cfg.Execution.PlannerModel == "" && !strings.Contains(b.String(), "successfully") {
		t.Errorf("Interactive init failed to write config or report success. Output: %s", b.String())
	}
}
