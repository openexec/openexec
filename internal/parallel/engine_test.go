package parallel

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/openexec/openexec/internal/agent"
	"github.com/openexec/openexec/internal/blueprint"
	"github.com/openexec/openexec/internal/types"
)

// MockStageExecutor is a mock executor for testing.
type MockStageExecutor struct {
	results map[string]*blueprint.StageResult
}

func NewMockStageExecutor() *MockStageExecutor {
	return &MockStageExecutor{
		results: make(map[string]*blueprint.StageResult),
	}
}

func (m *MockStageExecutor) Execute(ctx context.Context, stage *blueprint.Stage, input *blueprint.StageInput) (*blueprint.StageResult, error) {
	if result, ok := m.results[stage.Name]; ok {
		return result, nil
	}
	result := blueprint.NewStageResult(stage.Name, 1)
	result.Complete("mock result")
	return result, nil
}

func (m *MockStageExecutor) SetResult(stageName string, result *blueprint.StageResult) {
	m.results[stageName] = result
}

func TestParallelEngine(t *testing.T) {
	tmpDir := t.TempDir()
	os.MkdirAll(filepath.Join(tmpDir, ".openexec"), 0755)

	registry, err := agent.NewAgentRegistry(tmpDir)
	if err != nil {
		t.Fatalf("failed to create registry: %v", err)
	}
	defer registry.Close()

	executor := NewMockStageExecutor()

	t.Run("Create Engine", func(t *testing.T) {
		bp := blueprint.DefaultBlueprint
		config := DefaultParallelConfig()

		engine, err := NewParallelEngine(bp, executor, registry, config)
		if err != nil {
			t.Fatalf("NewParallelEngine failed: %v", err)
		}

		if engine == nil {
			t.Fatal("expected engine, got nil")
		}
		if engine.config.MaxAgents != 4 {
			t.Errorf("expected MaxAgents 4, got %d", engine.config.MaxAgents)
		}
	})

	t.Run("Should Parallelize", func(t *testing.T) {
		bp := blueprint.DefaultBlueprint
		config := DefaultParallelConfig()
		engine, _ := NewParallelEngine(bp, executor, registry, config)

		// Test with parallel enabled and enough files
		stage := &ParallelStage{
			Stage: &blueprint.Stage{
				Name: "implement",
				Type: types.StageTypeAgentic,
			},
			EnableParallel: true,
		}

		files := make([]string, 10)
		for i := range files {
			files[i] = filepath.Join("src", string(rune('a'+i))+".go")
		}

		if !engine.shouldParallelize(stage, files) {
			t.Error("expected to parallelize with 10 files")
		}

		// Test with too few files
		fewFiles := []string{"file1.go", "file2.go"}
		if engine.shouldParallelize(stage, fewFiles) {
			t.Error("should not parallelize with only 2 files")
		}

		// Test with parallel disabled
		stage.EnableParallel = false
		if engine.shouldParallelize(stage, files) {
			t.Error("should not parallelize when disabled")
		}

		// Test with deterministic stage
		stage.EnableParallel = true
		stage.Type = types.StageTypeDeterministic
		if engine.shouldParallelize(stage, files) {
			t.Error("should not parallelize deterministic stages")
		}
	})

	t.Run("Create Batches", func(t *testing.T) {
		bp := blueprint.DefaultBlueprint
		config := DefaultParallelConfig()
		engine, _ := NewParallelEngine(bp, executor, registry, config)

		stage := &ParallelStage{
			Stage: &blueprint.Stage{
				Name: "implement",
				Type: types.StageTypeAgentic,
			},
			EnableParallel: true,
			MaxAgents:      3,
			BatchStrategy:  agent.BatchByFiles,
		}

		files := []string{
			"file1.go", "file2.go", "file3.go",
			"file4.go", "file5.go", "file6.go",
			"file7.go", "file8.go", "file9.go",
		}

		batches := engine.createBatches(stage, files)

		if len(batches) != 3 {
			t.Errorf("expected 3 batches, got %d", len(batches))
		}

		// Check total files
		totalFiles := 0
		for _, batch := range batches {
			totalFiles += len(batch.Files)
		}
		if totalFiles != len(files) {
			t.Errorf("expected %d total files, got %d", len(files), totalFiles)
		}
	})

	t.Run("Get Max Agents", func(t *testing.T) {
		bp := blueprint.DefaultBlueprint
		config := &ParallelConfig{
			MaxAgents: 4,
		}
		engine, _ := NewParallelEngine(bp, executor, registry, config)

		// Test with stage override
		stage := &ParallelStage{
			Stage:     &blueprint.Stage{Name: "test"},
			MaxAgents: 2,
		}

		maxAgents := engine.getMaxAgents(stage)
		if maxAgents != 2 {
			t.Errorf("expected 2 agents (stage override), got %d", maxAgents)
		}

		// Test without stage override
		stage.MaxAgents = 0
		maxAgents = engine.getMaxAgents(stage)
		if maxAgents != 4 {
			t.Errorf("expected 4 agents (global config), got %d", maxAgents)
		}

		// Test stage override exceeding global limit
		stage.MaxAgents = 10
		maxAgents = engine.getMaxAgents(stage)
		if maxAgents != 4 {
			t.Errorf("expected 4 agents (capped by global), got %d", maxAgents)
		}
	})

	t.Run("Parallel Config", func(t *testing.T) {
		config := DefaultParallelConfig()

		if config.MaxAgents != 4 {
			t.Errorf("expected default MaxAgents 4, got %d", config.MaxAgents)
		}
		if config.MinFilesForParallel != 5 {
			t.Errorf("expected default MinFilesForParallel 5, got %d", config.MinFilesForParallel)
		}
		if config.DefaultMergeStrategy != agent.MergeReconcile {
			t.Errorf("expected default MergeStrategy reconcile, got %s", config.DefaultMergeStrategy)
		}
		if !config.EnableParallelism {
			t.Error("expected parallelism to be enabled by default")
		}
	})
}

func TestParallelBlueprintBuilder(t *testing.T) {
	t.Run("Build Blueprint", func(t *testing.T) {
		builder := NewParallelBlueprintBuilder("test", "Test Blueprint")
		bp := builder.
			AddSequentialStage("stage1", types.StageTypeDeterministic, "toolset1", "stage2").
			AddParallelStage("stage2", types.StageTypeAgentic, "toolset2", 4, "complete").
			Build()

		if bp.ID != "test" {
			t.Errorf("expected ID 'test', got %s", bp.ID)
		}
		if bp.Name != "Test Blueprint" {
			t.Errorf("expected name 'Test Blueprint', got %s", bp.Name)
		}
		if len(bp.Stages) != 2 {
			t.Errorf("expected 2 stages, got %d", len(bp.Stages))
		}
		if bp.InitialStage != "stage1" {
			t.Errorf("expected initial stage 'stage1', got %s", bp.InitialStage)
		}
	})

	t.Run("Default Parallel Blueprint", func(t *testing.T) {
		bp := CreateDefaultParallelBlueprint()

		if bp.ID != "parallel_task" {
			t.Errorf("expected ID 'parallel_task', got %s", bp.ID)
		}
		if len(bp.Stages) != 5 {
			t.Errorf("expected 5 stages, got %d", len(bp.Stages))
		}

		// Check that implement stage exists
		if _, ok := bp.Stages["implement"]; !ok {
			t.Error("expected 'implement' stage")
		}
	})
}

func TestParallelStage(t *testing.T) {
	t.Run("Create Parallel Stage", func(t *testing.T) {
		baseStage := &blueprint.Stage{
			Name:      "implement",
			Type:      types.StageTypeAgentic,
			Toolset:   "coding_backend",
			MaxRetries: 3,
			Timeout:   10 * time.Minute,
			OnSuccess: "lint",
		}

		parallelStage := &ParallelStage{
			Stage:          baseStage,
			EnableParallel: true,
			MaxAgents:      4,
			BatchStrategy:  agent.BatchByFiles,
			MergeStrategy:  agent.MergeReconcile,
		}

		if !parallelStage.EnableParallel {
			t.Error("expected EnableParallel to be true")
		}
		if parallelStage.MaxAgents != 4 {
			t.Errorf("expected MaxAgents 4, got %d", parallelStage.MaxAgents)
		}
		if parallelStage.BatchStrategy != agent.BatchByFiles {
			t.Errorf("expected BatchByFiles, got %s", parallelStage.BatchStrategy)
		}
		if parallelStage.MergeStrategy != agent.MergeReconcile {
			t.Errorf("expected MergeReconcile, got %s", parallelStage.MergeStrategy)
		}
	})
}

func TestMin(t *testing.T) {
	tests := []struct {
		a        int
		b        int
		expected int
	}{
		{1, 2, 1},
		{2, 1, 1},
		{5, 5, 5},
		{0, 10, 0},
		{-5, 5, -5},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("min(%d,%d)", tt.a, tt.b), func(t *testing.T) {
			result := min(tt.a, tt.b)
			if result != tt.expected {
				t.Errorf("min(%d, %d) = %d, expected %d", tt.a, tt.b, result, tt.expected)
			}
		})
	}
}


