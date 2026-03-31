package validation

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/openexec/openexec/internal/project"
	"github.com/openexec/openexec/internal/tui"
)

var (
	compatBinaryOnce sync.Once
	compatBinaryPath string
	compatBinaryErr  error
)

func buildCompatibilityBinary(t *testing.T) string {
	t.Helper()

	compatBinaryOnce.Do(func() {
		tmpDir := os.TempDir()
		compatBinaryPath = filepath.Join(tmpDir, "openexec-compat-test")

		cmd := exec.Command("go", "build", "-o", compatBinaryPath, "./cmd/openexec")
		cmd.Dir = projectRoot(t)
		out, err := cmd.CombinedOutput()
		if err != nil {
			compatBinaryErr = err
			compatBinaryPath = string(out)
		}
	})

	if compatBinaryErr != nil {
		t.Fatalf("failed to build compatibility binary: %v\nOutput: %s", compatBinaryErr, compatBinaryPath)
	}

	return compatBinaryPath
}

func projectRoot(t *testing.T) string {
	t.Helper()

	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to determine working directory: %v", err)
	}

	dir := wd
	for i := 0; i < 10; i++ {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}

	t.Fatalf("failed to locate repository root from %s", wd)
	return ""
}

func writeFile(t *testing.T, path, content string, mode os.FileMode) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("failed to create directory for %s: %v", path, err)
	}
	if err := os.WriteFile(path, []byte(content), mode); err != nil {
		t.Fatalf("failed to write %s: %v", path, err)
	}
}

func createCurrentProjectFixture(t *testing.T, baseDir string) string {
	t.Helper()

	projectDir := filepath.Join(baseDir, "current-project")
	writeFile(t, filepath.Join(projectDir, ".git", "HEAD"), "ref: refs/heads/main\n", 0o644)
	writeFile(t, filepath.Join(projectDir, "openexec.yaml"), "project:\n  name: current-project\n", 0o644)
	writeFile(t, filepath.Join(projectDir, ".openexec", "config.json"), `{
  "name": "current-project",
  "project_dir": "",
  "execution": {
    "exec_mode": "workspace-write"
  }
}
`, 0o600)
	return projectDir
}

func createLegacyProjectFixture(t *testing.T, baseDir string) string {
	t.Helper()

	projectDir := filepath.Join(baseDir, "legacy-project")
	writeFile(t, filepath.Join(projectDir, ".git", "HEAD"), "ref: refs/heads/main\n", 0o644)
	writeFile(t, filepath.Join(projectDir, ".uaos", "project.json"), `{
  "name": "legacy-project",
  "git_enabled": true,
  "execution": {
    "exec_mode": "workspace-write"
  }
}
`, 0o600)
	return projectDir
}

func createLegacyTasksFallbackFixture(t *testing.T, baseDir string) string {
	t.Helper()

	projectDir := filepath.Join(baseDir, "legacy-tasks-project")
	writeFile(t, filepath.Join(projectDir, ".openexec", "state.json"), `{
  "status": "running",
  "phase": "implement",
  "worker_count": 2,
  "progress": 0
}
`, 0o644)
	writeFile(t, filepath.Join(projectDir, ".openexec", "tasks.json"), `{
  "tasks": [
    {"status": "completed"},
    {"status": "done"},
    {"status": "running"},
    {"status": "pending"}
  ]
}
`, 0o644)
	return projectDir
}

func TestCompatibility_ExistingProjects_StatusCLI(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping compatibility binary test in short mode")
	}

	binaryPath := buildCompatibilityBinary(t)
	fixtureRoot := t.TempDir()

	currentDir := createCurrentProjectFixture(t, fixtureRoot)
	legacyDir := createLegacyProjectFixture(t, fixtureRoot)

	testCases := []struct {
		name        string
		projectDir  string
		projectName string
	}{
		{name: "current_openexec_project", projectDir: currentDir, projectName: "current-project"},
		{name: "legacy_uaos_project", projectDir: legacyDir, projectName: "legacy-project"},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			cmd := exec.CommandContext(ctx, binaryPath, "status", "--json")
			cmd.Dir = tc.projectDir
			out, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("status failed for %s: %v\nOutput: %s", tc.projectName, err, string(out))
			}

			if strings.Contains(strings.ToLower(string(out)), "project not initialized") {
				t.Fatalf("legacy compatibility regression for %s:\n%s", tc.projectName, string(out))
			}

			var payload struct {
				Daemon struct {
					Running bool `json:"running"`
				} `json:"daemon"`
				Project struct {
					Name string `json:"name"`
					Path string `json:"path"`
				} `json:"project"`
			}
			if err := json.Unmarshal(out, &payload); err != nil {
				t.Fatalf("failed to parse status JSON for %s: %v\nOutput: %s", tc.projectName, err, string(out))
			}

			if payload.Project.Name != tc.projectName {
				t.Fatalf("status project name = %q, want %q", payload.Project.Name, tc.projectName)
			}
			resolvedPath := payload.Project.Path
			if !filepath.IsAbs(resolvedPath) {
				resolvedPath = filepath.Clean(filepath.Join(tc.projectDir, payload.Project.Path))
			}
			if resolvedPath != tc.projectDir {
				t.Fatalf("status project path = %q (resolved %q), want %q", payload.Project.Path, resolvedPath, tc.projectDir)
			}
			if payload.Daemon.Running {
				t.Fatalf("expected daemon to be stopped for fixture project %s", tc.projectName)
			}
		})
	}
}

func TestCompatibility_LegacyProjectConfigFallback(t *testing.T) {
	fixtureRoot := t.TempDir()
	legacyDir := createLegacyProjectFixture(t, fixtureRoot)

	cfg, err := project.LoadProjectConfig(legacyDir)
	if err != nil {
		t.Fatalf("expected legacy .uaos config to load, got error: %v", err)
	}

	if cfg.Name != "legacy-project" {
		t.Fatalf("legacy config name = %q, want legacy-project", cfg.Name)
	}
	if cfg.ProjectDir != legacyDir {
		t.Fatalf("legacy config project dir = %q, want %q", cfg.ProjectDir, legacyDir)
	}
}

func TestCompatibility_LegacyTasksJSONFallback(t *testing.T) {
	baseDir := t.TempDir()
	createLegacyTasksFallbackFixture(t, baseDir)

	source := tui.NewFileSource(baseDir)
	t.Cleanup(source.Close)

	deadline := time.Now().Add(5 * time.Second)
	for {
		projects, err := source.List()
		if err != nil {
			t.Fatalf("list failed: %v", err)
		}
		if len(projects) == 1 {
			proj := projects[0]
			if proj.Name != "legacy-tasks-project" {
				t.Fatalf("project name = %q, want legacy-tasks-project", proj.Name)
			}
			if proj.Status != "running" {
				t.Fatalf("project status = %q, want running", proj.Status)
			}
			if proj.Phase != "implement" {
				t.Fatalf("project phase = %q, want implement", proj.Phase)
			}
			if proj.WorkerCount != 2 {
				t.Fatalf("worker count = %d, want 2", proj.WorkerCount)
			}
			if proj.Progress != 50 {
				t.Fatalf("legacy tasks fallback progress = %d, want 50", proj.Progress)
			}
			return
		}

		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for FileSource to detect fixture project")
		}
		time.Sleep(25 * time.Millisecond)
	}
}
