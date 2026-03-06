package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadPendingTasks(t *testing.T) {
	tmpDir := t.TempDir()

	t.Run("From root tasks.json", func(t *testing.T) {
		tasks := TasksFile{
			Tasks: []Task{
				{ID: "T-001", Title: "Task 1", Status: "pending"},
				{ID: "T-002", Title: "Task 2", Status: "completed"},
			},
		}
		data, _ := json.Marshal(tasks)
		os.WriteFile(filepath.Join(tmpDir, "tasks.json"), data, 0644)

		got, err := loadPendingTasks(tmpDir)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(got) != 2 {
			t.Errorf("got %d tasks, want 2", len(got))
		}
		if got[0].ID != "T-001" {
			t.Errorf("got task ID %q, want %q", got[0].ID, "T-001")
		}
	})

	t.Run("From stories.json", func(t *testing.T) {
		os.Remove(filepath.Join(tmpDir, "tasks.json"))
		os.MkdirAll(filepath.Join(tmpDir, ".openexec"), 0755)
		
		storiesContent := `{
			"stories": [
				{
					"id": "S-001",
					"title": "Story 1",
					"status": "pending",
					"tasks": [
						{"id": "T-003", "title": "Task 3", "description": "Desc 3"}
					]
				}
			]
		}`
		os.WriteFile(filepath.Join(tmpDir, ".openexec", "stories.json"), []byte(storiesContent), 0644)

		got, err := loadPendingTasks(tmpDir)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(got) != 1 {
			t.Errorf("got %d tasks, want 1", len(got))
		}
		if got[0].ID != "T-003" {
			t.Errorf("got task ID %q, want %q", got[0].ID, "T-003")
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

func TestSaveTaskStatus(t *testing.T) {
	tmpDir := t.TempDir()
	tasksPath := filepath.Join(tmpDir, "tasks.json")
	
	tasks := TasksFile{
		Tasks: []Task{
			{ID: "T-001", Status: "pending"},
		},
	}
	data, _ := json.Marshal(tasks)
	os.WriteFile(tasksPath, data, 0644)

	err := saveTaskStatus(tmpDir, "T-001", "completed")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify
	data, _ = os.ReadFile(tasksPath)
	var tf TasksFile
	json.Unmarshal(data, &tf)
	if tf.Tasks[0].Status != "completed" {
		t.Errorf("got status %q, want %q", tf.Tasks[0].Status, "completed")
	}
}

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

