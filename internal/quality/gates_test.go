package quality

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestQualityGates(t *testing.T) {
	tmpDir := t.TempDir()

	t.Run("Default Gates", func(t *testing.T) {
		goGates := DefaultGates("go")
		if len(goGates) == 0 {
			t.Error("expected Go gates")
		}

		pythonGates := DefaultGates("python")
		if len(pythonGates) == 0 {
			t.Error("expected Python gates")
		}

		jsGates := DefaultGates("javascript")
		if len(jsGates) == 0 {
			t.Error("expected JavaScript gates")
		}

		tsGates := DefaultGates("typescript")
		if len(tsGates) == 0 {
			t.Error("expected TypeScript gates")
		}

		rustGates := DefaultGates("rust")
		if len(rustGates) == 0 {
			t.Error("expected Rust gates")
		}
	})

	t.Run("Manager Creation", func(t *testing.T) {
		gates := []Gate{
			{
				Name:    "test-gate",
				Type:    GateTypeLint,
				Command: "echo",
				Args:    []string{"test"},
				Mode:    GateModeBlock,
				Timeout: 10 * time.Second,
			},
		}

		manager := NewManager(tmpDir, gates)
		if manager == nil {
			t.Fatal("expected manager")
		}

		if len(manager.GetGates()) != 1 {
			t.Errorf("expected 1 gate, got %d", len(manager.GetGates()))
		}
	})

	t.Run("Run Gate", func(t *testing.T) {
		gates := []Gate{
			{
				Name:    "echo-gate",
				Type:    GateTypeCustom,
				Command: "echo",
				Args:    []string{"hello"},
				Mode:    GateModeBlock,
				Timeout: 10 * time.Second,
			},
		}

		manager := NewManager(tmpDir, gates)
		ctx := context.Background()

		result, err := manager.RunGate(ctx, "echo-gate")
		if err != nil {
			t.Fatalf("RunGate failed: %v", err)
		}

		if !result.Passed {
			t.Error("expected gate to pass")
		}
		if result.GateName != "echo-gate" {
			t.Errorf("expected gate name 'echo-gate', got %s", result.GateName)
		}
	})

	t.Run("Run All Gates", func(t *testing.T) {
		gates := []Gate{
			{
				Name:    "pass-gate",
				Type:    GateTypeCustom,
				Command: "echo",
				Args:    []string{"pass"},
				Mode:    GateModeBlock,
				Timeout: 10 * time.Second,
			},
			{
				Name:    "warn-gate",
				Type:    GateTypeCustom,
				Command: "echo",
				Args:    []string{"warn"},
				Mode:    GateModeWarn,
				Timeout: 10 * time.Second,
			},
		}

		manager := NewManager(tmpDir, gates)
		ctx := context.Background()

		summary, err := manager.RunAll(ctx)
		if err != nil {
			t.Fatalf("RunAll failed: %v", err)
		}

		if summary.TotalGates != 2 {
			t.Errorf("expected 2 gates, got %d", summary.TotalGates)
		}
		if summary.PassedGates != 2 {
			t.Errorf("expected 2 passed, got %d", summary.PassedGates)
		}
		if summary.Blocked {
			t.Error("expected not blocked")
		}
	})

	t.Run("Blocked Gate", func(t *testing.T) {
		gates := []Gate{
			{
				Name:    "fail-gate",
				Type:    GateTypeCustom,
				Command: "false", // Always fails
				Args:    []string{},
				Mode:    GateModeBlock,
				Timeout: 10 * time.Second,
			},
		}

		manager := NewManager(tmpDir, gates)
		ctx := context.Background()

		summary, err := manager.RunAll(ctx)
		if err != nil {
			t.Fatalf("RunAll failed: %v", err)
		}

		if !summary.Blocked {
			t.Error("expected blocked")
		}
		if summary.FailedGates != 1 {
			t.Errorf("expected 1 failed gate, got %d", summary.FailedGates)
		}
	})

	t.Run("Missing Command", func(t *testing.T) {
		gates := []Gate{
			{
				Name:    "missing-gate",
				Type:    GateTypeCustom,
				Command: "this-command-does-not-exist-12345",
				Args:    []string{},
				Mode:    GateModeBlock,
				Timeout: 10 * time.Second,
			},
		}

		manager := NewManager(tmpDir, gates)
		ctx := context.Background()

		result, _ := manager.RunGate(ctx, "missing-gate")
		if result.Passed {
			t.Error("expected gate to fail when command missing")
		}
		if result.Error == "" {
			t.Error("expected error message")
		}
	})

	t.Run("Add and Remove Gate", func(t *testing.T) {
		manager := NewManager(tmpDir, []Gate{})

		gate := Gate{
			Name:    "new-gate",
			Type:    GateTypeLint,
			Command: "echo",
			Args:    []string{"test"},
			Mode:    GateModeBlock,
			Timeout: 10 * time.Second,
		}

		manager.AddGate(gate)
		if len(manager.GetGates()) != 1 {
			t.Errorf("expected 1 gate after add, got %d", len(manager.GetGates()))
		}

		removed := manager.RemoveGate("new-gate")
		if !removed {
			t.Error("expected gate to be removed")
		}
		if len(manager.GetGates()) != 0 {
			t.Errorf("expected 0 gates after remove, got %d", len(manager.GetGates()))
		}

		// Try removing non-existent
		removed = manager.RemoveGate("nonexistent")
		if removed {
			t.Error("expected false for non-existent gate")
		}
	})

	t.Run("Detect Project Type", func(t *testing.T) {
		tests := []struct {
			files    map[string]string
			expected string
		}{
			{
				map[string]string{"go.mod": "module test"},
				"go",
			},
			{
				map[string]string{"requirements.txt": "requests"},
				"python",
			},
			{
				map[string]string{"pyproject.toml": "[tool.poetry]"},
				"python",
			},
			{
				map[string]string{"package.json": "{}", "tsconfig.json": "{}"},
				"typescript",
			},
			{
				map[string]string{"package.json": "{}"},
				"javascript",
			},
			{
				map[string]string{"Cargo.toml": "[package]"},
				"rust",
			},
		}

		for _, tt := range tests {
			t.Run(tt.expected, func(t *testing.T) {
				projectDir := t.TempDir()
				for file, content := range tt.files {
					os.WriteFile(filepath.Join(projectDir, file), []byte(content), 0644)
				}

				projectType := DetectProjectType(projectDir)
				if projectType != tt.expected {
					t.Errorf("expected %s, got %s", tt.expected, projectType)
				}
			})
		}
	})

	t.Run("Gate Types", func(t *testing.T) {
		types := []GateType{
			GateTypeLint,
			GateTypeTest,
			GateTypeFormat,
			GateTypeSecurity,
			GateTypeCustom,
		}

		for _, gt := range types {
			if string(gt) == "" {
				t.Errorf("gate type %v has empty string", gt)
			}
		}
	})

	t.Run("Gate Modes", func(t *testing.T) {
		modes := []GateMode{
			GateModeBlock,
			GateModeWarn,
			GateModeIgnore,
		}

		for _, mode := range modes {
			if string(mode) == "" {
				t.Errorf("gate mode %v has empty string", mode)
			}
		}
	})

	t.Run("Format Summary", func(t *testing.T) {
		summary := &GateSummary{
			Results: []GateResult{
				{
					GateName: "test-gate",
					Passed:   true,
					Duration: 100 * time.Millisecond,
				},
			},
			TotalGates:    1,
			PassedGates:   1,
			FailedGates:   0,
			Blocked:       false,
			TotalDuration: 100 * time.Millisecond,
		}

		formatted := FormatSummary(summary)
		if formatted == "" {
			t.Error("expected formatted summary")
		}
		if !contains(formatted, "PASS") {
			t.Error("expected PASS in summary")
		}
	})

	t.Run("Format Summary Blocked", func(t *testing.T) {
		summary := &GateSummary{
			Results: []GateResult{
				{
					GateName: "fail-gate",
					Passed:   false,
					Error:    "test error",
					Duration: 100 * time.Millisecond,
				},
			},
			TotalGates:    1,
			PassedGates:   0,
			FailedGates:   1,
			Blocked:       true,
			TotalDuration: 100 * time.Millisecond,
		}

		formatted := FormatSummary(summary)
		if formatted == "" {
			t.Error("expected formatted summary")
		}
		if !contains(formatted, "BLOCKED") {
			t.Error("expected BLOCKED in summary")
		}
	})
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
