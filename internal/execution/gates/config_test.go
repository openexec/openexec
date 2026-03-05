package gates

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfig(t *testing.T) {
	// Create a temp directory for testing
	tmpDir, err := os.MkdirTemp("", "gates-config-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Case 1: missing config
	_, err = LoadConfig(tmpDir)
	if err == nil {
		t.Error("expected error for missing config, got nil")
	}

	// Case 2: valid config in root
	yamlContent := `
project:
  name: "test-proj"
  type: "go"
quality:
  gates: ["lint", "test"]
  custom:
    - name: "custom-gate"
      command: "echo hello"
      mode: "blocking"
`
	err = os.WriteFile(filepath.Join(tmpDir, "openexec.yaml"), []byte(yamlContent), 0644)
	if err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	cfg, err := LoadConfig(tmpDir)
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	if cfg.Project.Name != "test-proj" {
		t.Errorf("expected project name 'test-proj', got %q", cfg.Project.Name)
	}

	if len(cfg.Quality.Gates) != 2 {
		t.Errorf("expected 2 enabled gates, got %d", len(cfg.Quality.Gates))
	}

	// Case 3: custom gate lookup
	gate := cfg.GetGate("custom-gate")
	if gate == nil {
		t.Fatal("expected to find custom-gate")
	}
	if gate.Command != "echo hello" {
		t.Errorf("expected command 'echo hello', got %q", gate.Command)
	}

	// Case 4: config in .openexec/
	tmpDir2, _ := os.MkdirTemp("", "gates-config-test-2-*")
	defer os.RemoveAll(tmpDir2)
	os.MkdirAll(filepath.Join(tmpDir2, ".openexec"), 0755)
	os.WriteFile(filepath.Join(tmpDir2, ".openexec", "openexec.yaml"), []byte(yamlContent), 0644)

	cfg2, err := LoadConfig(tmpDir2)
	if err != nil {
		t.Fatalf("failed to load config from .openexec/: %v", err)
	}
	if cfg2.Project.Name != "test-proj" {
		t.Errorf("expected project name 'test-proj', got %q", cfg2.Project.Name)
	}
}
