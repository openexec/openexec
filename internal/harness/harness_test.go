package harness

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/openexec/openexec/internal/blueprint"
	"github.com/openexec/openexec/internal/types"
)

func TestHarness(t *testing.T) {
	tmpDir := t.TempDir()
	os.MkdirAll(filepath.Join(tmpDir, ".openexec"), 0755)

	t.Run("Create Harness", func(t *testing.T) {
		config := DefaultHarnessConfig(tmpDir)
		
		harness, err := NewHarness(config)
		if err != nil {
			t.Fatalf("NewHarness failed: %v", err)
		}
		defer harness.Close()

		if harness == nil {
			t.Fatal("expected harness, got nil")
		}
		if harness.projectDir != tmpDir {
			t.Errorf("expected projectDir %s, got %s", tmpDir, harness.projectDir)
		}
	})

	t.Run("Harness With Caches", func(t *testing.T) {
		config := DefaultHarnessConfig(tmpDir)
		config.CacheEnabled = true
		
		harness, err := NewHarness(config)
		if err != nil {
			t.Fatalf("NewHarness failed: %v", err)
		}
		defer harness.Close()

		if harness.knowledgeCache == nil {
			t.Error("expected knowledge cache to be initialized")
		}
		if harness.toolResultCache == nil {
			t.Error("expected tool result cache to be initialized")
		}
	})

	t.Run("Harness With Memory", func(t *testing.T) {
		config := DefaultHarnessConfig(tmpDir)
		config.MemoryEnabled = true
		
		harness, err := NewHarness(config)
		if err != nil {
			t.Fatalf("NewHarness failed: %v", err)
		}
		defer harness.Close()

		if harness.memorySystem == nil {
			t.Error("expected memory system to be initialized")
		}
		if harness.memoryManager == nil {
			t.Error("expected memory manager to be initialized")
		}
	})

	t.Run("Harness With Multi-Agent", func(t *testing.T) {
		config := DefaultHarnessConfig(tmpDir)
		config.MultiAgentEnabled = true
		config.MaxAgents = 4
		
		harness, err := NewHarness(config)
		if err != nil {
			t.Fatalf("NewHarness failed: %v", err)
		}
		defer harness.Close()

		if harness.agentRegistry == nil {
			t.Error("expected agent registry to be initialized")
		}
	})

	t.Run("Harness With Blueprint", func(t *testing.T) {
		config := DefaultHarnessConfig(tmpDir)
		config.BlueprintEnabled = true
		config.Blueprint = blueprint.DefaultBlueprint
		
		harness, err := NewHarness(config)
		if err != nil {
			t.Fatalf("NewHarness failed: %v", err)
		}
		defer harness.Close()

		if harness.blueprintEngine == nil {
			t.Error("expected blueprint engine to be initialized")
		}
	})

	t.Run("Getters", func(t *testing.T) {
		config := DefaultHarnessConfig(tmpDir)
		config.CacheEnabled = true
		config.MemoryEnabled = true
		config.MultiAgentEnabled = true
		config.BlueprintEnabled = true
		
		harness, err := NewHarness(config)
		if err != nil {
			t.Fatalf("NewHarness failed: %v", err)
		}
		defer harness.Close()

		if harness.GetKnowledgeCache() == nil {
			t.Error("GetKnowledgeCache returned nil")
		}
		if harness.GetToolResultCache() == nil {
			t.Error("GetToolResultCache returned nil")
		}
		if harness.GetMemoryManager() == nil {
			t.Error("GetMemoryManager returned nil")
		}
		if harness.GetAgentRegistry() == nil {
			t.Error("GetAgentRegistry returned nil")
		}
		if harness.GetBlueprintEngine() == nil {
			t.Error("GetBlueprintEngine returned nil")
		}
	})
}

func TestHarnessConfig(t *testing.T) {
	tmpDir := t.TempDir()

	t.Run("Default Config", func(t *testing.T) {
		config := DefaultHarnessConfig(tmpDir)

		if config.ProjectDir != tmpDir {
			t.Errorf("expected ProjectDir %s, got %s", tmpDir, config.ProjectDir)
		}
		if !config.CacheEnabled {
			t.Error("expected CacheEnabled to be true")
		}
		if !config.MemoryEnabled {
			t.Error("expected MemoryEnabled to be true")
		}
		if !config.MultiAgentEnabled {
			t.Error("expected MultiAgentEnabled to be true")
		}
		if config.MaxAgents != 4 {
			t.Errorf("expected MaxAgents 4, got %d", config.MaxAgents)
		}
		if config.CacheTTL != 1*time.Hour {
			t.Errorf("expected CacheTTL 1h, got %v", config.CacheTTL)
		}
	})
}

func TestHarnessStageExecutor(t *testing.T) {
	tmpDir := t.TempDir()
	os.MkdirAll(filepath.Join(tmpDir, ".openexec"), 0755)

	config := DefaultHarnessConfig(tmpDir)
	config.CacheEnabled = true
	
	harness, err := NewHarness(config)
	if err != nil {
		t.Fatalf("NewHarness failed: %v", err)
	}
	defer harness.Close()

	executor := &HarnessStageExecutor{harness: harness}

	t.Run("Execute Stage", func(t *testing.T) {
		stage := &blueprint.Stage{
			Name: "test-stage",
			Type: types.StageTypeDeterministic,
		}
		input := blueprint.NewStageInput("test", "task", tmpDir)

		result, err := executor.Execute(context.Background(), stage, input)
		if err != nil {
			t.Fatalf("Execute failed: %v", err)
		}

		if result.StageName != "test-stage" {
			t.Errorf("expected stage name 'test-stage', got %s", result.StageName)
		}
		if result.Status != types.StageStatusCompleted {
			t.Errorf("expected status completed, got %s", result.Status)
		}
	})
}

func TestHelperFunctions(t *testing.T) {
	t.Run("Contains", func(t *testing.T) {
		tests := []struct {
			text     string
			keywords []string
			expected bool
		}{
			{"This is a pattern", []string{"pattern"}, true},
			{"This is a pattern", []string{"convention"}, false},
			{"This follows convention", []string{"pattern", "convention"}, true},
			{"", []string{"pattern"}, false},
		}

		for _, tt := range tests {
			result := contains(tt.text, tt.keywords...)
			if result != tt.expected {
				t.Errorf("contains(%q, %v) = %v, expected %v", tt.text, tt.keywords, result, tt.expected)
			}
		}
	})

	t.Run("Min", func(t *testing.T) {
		tests := []struct {
			a        int
			b        int
			expected int
		}{
			{1, 2, 1},
			{2, 1, 1},
			{5, 5, 5},
			{0, 10, 0},
		}

		for _, tt := range tests {
			result := min(tt.a, tt.b)
			if result != tt.expected {
				t.Errorf("min(%d, %d) = %d, expected %d", tt.a, tt.b, result, tt.expected)
			}
		}
	})
}

func TestHarnessIntegration(t *testing.T) {
	tmpDir := t.TempDir()
	os.MkdirAll(filepath.Join(tmpDir, ".openexec"), 0755)

	t.Run("Full Integration", func(t *testing.T) {
		config := &HarnessConfig{
			ProjectDir:        tmpDir,
			CacheEnabled:      true,
			CacheTTL:          1 * time.Hour,
			MemoryEnabled:     true,
			MultiAgentEnabled: true,
			MaxAgents:         2,
			Blueprint:         blueprint.DefaultBlueprint,
			BlueprintEnabled:  true,
		}

		harness, err := NewHarness(config)
		if err != nil {
			t.Fatalf("NewHarness failed: %v", err)
		}
		defer harness.Close()

		// Verify all components initialized
		if harness.knowledgeCache == nil {
			t.Error("knowledge cache not initialized")
		}
		if harness.toolResultCache == nil {
			t.Error("tool result cache not initialized")
		}
		if harness.memorySystem == nil {
			t.Error("memory system not initialized")
		}
		if harness.memoryManager == nil {
			t.Error("memory manager not initialized")
		}
		if harness.agentRegistry == nil {
			t.Error("agent registry not initialized")
		}
		if harness.blueprintEngine == nil {
			t.Error("blueprint engine not initialized")
		}
	})
}
