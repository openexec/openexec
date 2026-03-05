package cli

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDoctorCmd(t *testing.T) {
	// 1. No projects found
	t.Run("No Projects", func(t *testing.T) {
		tmpDir := t.TempDir()
		b := bytes.NewBufferString("")
		rootCmd.SetOut(b)
		rootCmd.SetArgs([]string{"doctor", tmpDir})

		err := rootCmd.Execute()
		if err != nil {
			t.Fatalf("Execute failed: %v", err)
		}

		if !strings.Contains(b.String(), "No OpenExec projects found") {
			t.Errorf("unexpected output: %s", b.String())
		}
	})

	// 2. Project with issues
	t.Run("Broken Project", func(t *testing.T) {
		tmpDir := t.TempDir()
		projDir := filepath.Join(tmpDir, "broken-proj")
		os.MkdirAll(filepath.Join(projDir, ".openexec"), 0755)
		
		// Corrupt state.json
		os.WriteFile(filepath.Join(projDir, ".openexec", "state.json"), []byte("invalid json"), 0644)

		b := bytes.NewBufferString("")
		rootCmd.SetOut(b)
		rootCmd.SetArgs([]string{"doctor", tmpDir})

		err := rootCmd.Execute()
		if err == nil {
			t.Error("expected error from broken project")
		}

		if !strings.Contains(b.String(), "[FAIL] state_file") {
			t.Errorf("missing state_file failure in output: %s", b.String())
		}
	})

	// 3. Valid project
	t.Run("Valid Project", func(t *testing.T) {
		tmpDir := t.TempDir()
		projDir := filepath.Join(tmpDir, "valid-proj")
		os.MkdirAll(filepath.Join(projDir, ".openexec"), 0755)
		
		state := map[string]interface{}{"status": "idle", "phase": "none"}
		stateData, _ := json.Marshal(state)
		os.WriteFile(filepath.Join(projDir, ".openexec", "state.json"), stateData, 0644)

		tasks := map[string]interface{}{"tasks": []interface{}{}}
		tasksData, _ := json.Marshal(tasks)
		os.WriteFile(filepath.Join(projDir, ".openexec", "tasks.json"), tasksData, 0644)

		b := bytes.NewBufferString("")
		rootCmd.SetOut(b)
		rootCmd.SetArgs([]string{"doctor", tmpDir})

		err := rootCmd.Execute()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if !strings.Contains(b.String(), "All checks passed!") {
			t.Errorf("expected success message, got: %s", b.String())
		}
	})
}

func TestDoctorIntentCmd(t *testing.T) {
	t.Run("Missing File", func(t *testing.T) {
		tmpDir := t.TempDir()
		oldCwd, _ := os.Getwd()
		os.Chdir(tmpDir)
		defer os.Chdir(oldCwd)

		rootCmd.SetArgs([]string{"doctor", "intent", "non-existent.md"})
		err := rootCmd.Execute()
		if err == nil || !strings.Contains(err.Error(), "not found") {
			t.Errorf("expected 'not found' error, got %v", err)
		}
	})

	t.Run("Valid INTENT.md", func(t *testing.T) {
		tmpDir := t.TempDir()
		oldCwd, _ := os.Getwd()
		os.Chdir(tmpDir)
		defer os.Chdir(oldCwd)

		content := `# Title
## Goals
- Goal 1
## Requirements
- US-001: Req
## Constraints
- Platform: macOS
- Shape: CLI
- Data Source: Local
`
		os.WriteFile("INTENT.md", []byte(content), 0644)
		
		b := bytes.NewBufferString("")
		rootCmd.SetOut(b)
		rootCmd.SetArgs([]string{"doctor", "intent"})

		err := rootCmd.Execute()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if !strings.Contains(b.String(), "Intent Validation") {
			t.Error("missing header in output")
		}
	})
}

func TestDoctorAPI(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/health" {
			w.Write([]byte(`{"status":"ok"}`))
		} else {
			w.Write([]byte(`[]`))
		}
	}))
	defer server.Close()

	b := bytes.NewBufferString("")
	rootCmd.SetOut(b)
	rootCmd.SetArgs([]string{"doctor", "--api", server.URL})

	err := rootCmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(b.String(), "Execution API") {
		t.Error("missing Execution API section in output")
	}
	if !strings.Contains(b.String(), "[PASS] api_health") {
		t.Errorf("expected api_health pass, output: %s", b.String())
	}
}

func TestRepeatChar(t *testing.T) {
	got := repeatChar('=', 5)
	if got != "=====" {
		t.Errorf("got %q, want %q", got, "=====")
	}
}
