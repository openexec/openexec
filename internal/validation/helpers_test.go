// Package validation provides comprehensive end-to-end tests
// for verifying all OpenExec goals (G-001 through G-005).
//
// Run with: go test ./internal/validation/... -v
package validation

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/openexec/openexec/internal/release"
)

// TestProjectEnv encapsulates a test project environment.
type TestProjectEnv struct {
	Dir        string
	DataDir    string
	OpenExecDir string
	t          *testing.T
}

// NewTestProjectEnv creates a fresh test project environment.
func NewTestProjectEnv(t *testing.T) *TestProjectEnv {
	t.Helper()

	dir := t.TempDir()
	openexecDir := filepath.Join(dir, ".openexec")
	dataDir := filepath.Join(openexecDir, "data")

	// Create directories
	if err := os.MkdirAll(dataDir, 0o750); err != nil {
		t.Fatalf("failed to create data dir: %v", err)
	}

	// Create minimal INTENT.md for auto-planning tests
	intentPath := filepath.Join(dir, "INTENT.md")
	intentContent := `# Test Intent
## Goal
Test the OpenExec validation system.

## Acceptance Criteria
- Tests pass
- Goals verified
`
	if err := os.WriteFile(intentPath, []byte(intentContent), 0o644); err != nil {
		t.Fatalf("failed to create INTENT.md: %v", err)
	}

	return &TestProjectEnv{
		Dir:         dir,
		DataDir:     dataDir,
		OpenExecDir: openexecDir,
		t:           t,
	}
}

// CreateReleaseManager creates a release manager with test configuration.
func (env *TestProjectEnv) CreateReleaseManager() *release.Manager {
	env.t.Helper()

	cfg := release.DefaultConfig()
	cfg.GitEnabled = false

	mgr, err := release.NewManager(env.Dir, cfg)
	if err != nil {
		env.t.Fatalf("failed to create release manager: %v", err)
	}

	env.t.Cleanup(func() { mgr.Close() })

	return mgr
}

// CreateTestStory creates a test story in the release manager.
func (env *TestProjectEnv) CreateTestStory(mgr *release.Manager, id, title, description string) *release.Story {
	env.t.Helper()

	story := &release.Story{
		ID:          id,
		Title:       title,
		Description: description,
		Status:      release.StoryStatusPending,
		CreatedAt:   time.Now(),
	}

	if err := mgr.CreateStory(story); err != nil {
		env.t.Fatalf("failed to create story: %v", err)
	}

	return story
}

// CreateTestTask creates a test task in the release manager.
func (env *TestProjectEnv) CreateTestTask(mgr *release.Manager, id, title, storyID string) *release.Task {
	env.t.Helper()

	task := &release.Task{
		ID:        id,
		Title:     title,
		StoryID:   storyID,
		Status:    release.TaskStatusPending,
		CreatedAt: time.Now(),
	}

	if err := mgr.CreateTask(task); err != nil {
		env.t.Fatalf("failed to create task: %v", err)
	}

	return task
}

// forbiddenIntentErrorStrings defines error messages that must never appear in responses.
// These indicate intent routing failures that violate G-001.
var forbiddenIntentErrorStrings = []string{
	"could not determine intent",
	"low confidence",
	"model could not determine",
}

// HealthResponse is the expected shape of /api/health.
type HealthResponse struct {
	Status  string       `json:"status"`
	Version string       `json:"version"`
	Runner  RunnerStatus `json:"runner"`
}

// RunnerStatus represents runner configuration in health response.
type RunnerStatus struct {
	Command string   `json:"command"`
	Args    []string `json:"args"`
	Model   string   `json:"model"`
}

