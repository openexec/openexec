package gates

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfig(t *testing.T) {
	tmpDir := t.TempDir()

	// 1. No config file
	_, err := LoadConfig(tmpDir)
	if err == nil {
		t.Error("expected error when no config file exists")
	}

	// 2. config in root
	configPath := filepath.Join(tmpDir, "openexec.yaml")
	configContent := `
project:
  name: test-project
  type: web
quality:
  gates:
    - lint
    - typecheck
  custom:
    - name: custom-gate
      command: echo "hello"
      mode: blocking
`
	err = os.WriteFile(configPath, []byte(configContent), 0644)
	if err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	cfg, err := LoadConfig(tmpDir)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	if cfg.Project.Name != "test-project" {
		t.Errorf("got project name %q, want %q", cfg.Project.Name, "test-project")
	}

	if len(cfg.Quality.Gates) != 2 {
		t.Errorf("got %d gates, want 2", len(cfg.Quality.Gates))
	}

	gate := cfg.GetGate("custom-gate")
	if gate == nil {
		t.Fatal("custom-gate not found")
	}
	if gate.Command != "echo \"hello\"" {
		t.Errorf("got command %q, want %q", gate.Command, "echo \"hello\"")
	}

	// 3. config in .openexec/openexec.yaml
	err = os.Remove(configPath)
	if err != nil {
		t.Fatalf("failed to remove config: %v", err)
	}

	oeDir := filepath.Join(tmpDir, ".openexec")
	err = os.Mkdir(oeDir, 0755)
	if err != nil {
		t.Fatalf("failed to create .openexec dir: %v", err)
	}

	oeConfigPath := filepath.Join(oeDir, "openexec.yaml")
	err = os.WriteFile(oeConfigPath, []byte(configContent), 0644)
	if err != nil {
		t.Fatalf("failed to write config in .openexec: %v", err)
	}

	cfg, err = LoadConfig(tmpDir)
	if err != nil {
		t.Fatalf("LoadConfig from .openexec failed: %v", err)
	}
	if cfg.Project.Name != "test-project" {
		t.Errorf("got project name %q from .openexec, want %q", cfg.Project.Name, "test-project")
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
		{"found", "gate1", true},
		{"found", "gate2", true},
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
