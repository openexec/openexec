package cli

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/openexec/openexec/internal/project"
)

func TestDoctorCmd(t *testing.T) {
	// 1. No projects found
	t.Run("No_Projects", func(t *testing.T) {
		tmpDir := t.TempDir()
		oldCwd, _ := os.Getwd()
		os.Chdir(tmpDir)
		defer os.Chdir(oldCwd)

		b := bytes.NewBufferString("")
		rootCmd.SetOut(b)
		rootCmd.SetArgs([]string{"doctor"})

		err := rootCmd.Execute()
		if err == nil {
			t.Fatal("expected error when no project initialized")
		}

		if !strings.Contains(b.String(), "Project not initialized") {
			t.Errorf("unexpected output: %s", b.String())
		}
	})

	// 2. Project initialized
	t.Run("Valid_Project", func(t *testing.T) {
		tmpDir := t.TempDir()
		oldCwd, _ := os.Getwd()
		os.Chdir(tmpDir)
		defer os.Chdir(oldCwd)

		// Correctly initialize project
		_, err := project.Initialize("test-proj", ".")
		if err != nil {
			t.Fatalf("failed to init project: %v", err)
		}
		os.WriteFile("INTENT.md", []byte("# Intent"), 0644)

		b := bytes.NewBufferString("")
		rootCmd.SetOut(b)
		rootCmd.SetArgs([]string{"doctor"})

		err = rootCmd.Execute()
		if err != nil {
			// May still fail if 'claude' CLI is not on path, but we check output
			t.Logf("doctor returned error (expected if runner missing): %v", err)
		}

		if !strings.Contains(b.String(), "Project config valid") {
			t.Errorf("expected config valid message, got: %s", b.String())
		}
	})
}

func TestDoctorAPI(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/health" {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprintln(w, `{"status":"ok","runner":{"command":"test-cli"}}`)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	// Need a valid project for doctor to even try the API
	tmpDir := t.TempDir()
	oldCwd, _ := os.Getwd()
	os.Chdir(tmpDir)
	defer os.Chdir(oldCwd)
	os.MkdirAll(".openexec", 0755)
	os.WriteFile("openexec.yaml", []byte(`name: test`), 0644)

	b := bytes.NewBufferString("")
	rootCmd.SetOut(b)
	rootCmd.SetArgs([]string{"doctor", "--api", server.URL})

	_ = rootCmd.Execute() // error likely due to runner resolution in local part

	if !strings.Contains(b.String(), "Checking Execution API Health") {
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

func repeatChar(c rune, n int) string {
	b := make([]rune, n)
	for i := range b {
		b[i] = c
	}
	return string(b)
}
