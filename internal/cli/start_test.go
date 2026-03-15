package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/openexec/openexec/internal/release"
)

func TestLoadPendingTasks(t *testing.T) {
	tmpDir := t.TempDir()
	// Create .openexec directory for SQLite
	openexecDir := filepath.Join(tmpDir, ".openexec")
	os.MkdirAll(openexecDir, 0755)

	// Create release manager backed by SQLite
	mgr, err := release.NewManager(tmpDir, nil)
	if err != nil {
		t.Fatalf("failed to create release manager: %v", err)
	}

	t.Run("From SQLite database", func(t *testing.T) {
		// Create a story first
		story := &release.Story{
			ID:     "US-001",
			Title:  "Test Story",
			Status: "pending",
		}
		if err := mgr.CreateStory(story); err != nil {
			t.Fatalf("failed to create story: %v", err)
		}

		// Create tasks in SQLite
		task1 := &release.Task{
			ID:      "T-001",
			Title:   "Task 1",
			StoryID: "US-001",
			Status:  "pending",
		}
		task2 := &release.Task{
			ID:      "T-002",
			Title:   "Task 2",
			StoryID: "US-001",
			Status:  "completed",
		}
		if err := mgr.CreateTask(task1); err != nil {
			t.Fatalf("failed to create task1: %v", err)
		}
		if err := mgr.CreateTask(task2); err != nil {
			t.Fatalf("failed to create task2: %v", err)
		}

		got, err := loadPendingTasks(tmpDir, mgr, true)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		// Only pending tasks should be returned
		if len(got) != 1 {
			t.Errorf("got %d tasks, want 1 (pending only)", len(got))
		}
		if len(got) > 0 && got[0].ID != "T-001" {
			t.Errorf("got task ID %q, want %q", got[0].ID, "T-001")
		}
	})

	t.Run("Requires release manager", func(t *testing.T) {
		// Calling with nil manager should return error
		_, err := loadPendingTasks(tmpDir, nil, true)
		if err == nil {
			t.Error("expected error when manager is nil")
		}
	})
}

func TestBuildTaskPromptWithRetry(t *testing.T) {
	task := Task{
		ID:          "T-001",
		Title:       "Test Task",
		Description: "Doing something",
	}

	t.Run("New Task", func(t *testing.T) {
		prompt := buildTaskPromptWithRetry(task, nil, "")
		if !strings.Contains(prompt, "TASK ID: T-001") {
			t.Error("missing task ID in prompt")
		}
		if strings.Contains(prompt, "SELF-HEALING") {
			t.Error("should not contain self-healing context")
		}
	})

	t.Run("Retry Task", func(t *testing.T) {
		prompt := buildTaskPromptWithRetry(task, nil, "compilation error")
		if !strings.Contains(prompt, "SELF-HEALING CONTEXT") {
			t.Error("missing self-healing context")
		}
		if !strings.Contains(prompt, "compilation error") {
			t.Error("missing error information in prompt")
		}
	})
}

// TestSaveTaskStatus removed: Task status updates now go through release.Manager
// which persists to SQLite. See internal/release/manager_test.go for tests.

func TestEnsureMCPConfig(t *testing.T) {
	tmpDir := t.TempDir()

	path, err := ensureMCPConfig(tmpDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.HasSuffix(path, "mcp.json") {
		t.Errorf("unexpected path: %s", path)
	}

	if _, err := os.Stat(path); err != nil {
		t.Error("mcp.json not created")
	}

	data, _ := os.ReadFile(path)
	var config map[string]interface{}
	json.Unmarshal(data, &config)

	if _, ok := config["mcpServers"]; !ok {
		t.Error("missing mcpServers in config")
	}
}

func TestPIDFileManagement(t *testing.T) {
	tmpDir := t.TempDir()
	os.MkdirAll(filepath.Join(tmpDir, ".openexec"), 0755)
	port := 8888

	t.Run("Write and Read PID", func(t *testing.T) {
		err := writePIDFile(tmpDir, port)
		if err != nil {
			t.Fatalf("failed to write PID file: %v", err)
		}

		pid, gotPort, err := readPID(tmpDir)
		if err != nil {
			t.Fatalf("failed to read PID file: %v", err)
		}

		if pid != os.Getpid() {
			t.Errorf("got PID %d, want %d", pid, os.Getpid())
		}
		if gotPort != port {
			t.Errorf("got port %d, want %d", gotPort, port)
		}
	})

	t.Run("Remove PID", func(t *testing.T) {
		err := removePIDFile(tmpDir)
		if err != nil {
			t.Fatalf("failed to remove PID file: %v", err)
		}

		_, _, err = readPID(tmpDir)
		if err == nil {
			t.Error("expected error reading removed PID file, got nil")
		}
	})
}

func TestPortDiscovery(t *testing.T) {
	t.Run("Find Available Port", func(t *testing.T) {
		port, err := findAvailablePort(9000)
		if err != nil {
			t.Fatalf("failed to find available port: %v", err)
		}
		if port < 9000 {
			t.Errorf("got port %d, want >= 9000", port)
		}
	})
}
