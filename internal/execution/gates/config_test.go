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

func TestGetGate(t *testing.T) {
	cfg := &Config{
		Quality: QualityConfig{
			Custom: []CustomGate{
				{Name: "gate1", Command: "cmd1"},
				{Name: "gate2", Command: "cmd2"},
			},
		},
	}

	tests := []struct {
		name     string
		gateName string
		found    bool
	}{
		{"found gate1", "gate1", true},
		{"found gate2", "gate2", true},
		{"not found", "gate3", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gate := cfg.GetGate(tt.gateName)
			if (gate != nil) != tt.found {
				t.Errorf("GetGate(%q) found = %v, want %v", tt.gateName, gate != nil, tt.found)
			}
		})
	}
}
