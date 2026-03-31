package agent

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/openexec/openexec/internal/blueprint"
)

func TestParallelStage(t *testing.T) {
	t.Run("CreateParallelStage", func(t *testing.T) {
		baseStage := &blueprint.Stage{
			Name:        "implement",
			Description: "Implement changes",
			Type:        "agentic",
			Toolset:     "coding_backend",
		}

		parallelStage := &ParallelStage{
			Stage:         baseStage,
			Parallel:      true,
			MaxAgents:     4,
			BatchStrategy: BatchByFiles,
			MergeStrategy: MergeReconcile,
		}

		if !parallelStage.Parallel {
			t.Error("expected Parallel to be true")
		}
		if parallelStage.MaxAgents != 4 {
			t.Errorf("expected MaxAgents 4, got %d", parallelStage.MaxAgents)
		}
		if parallelStage.BatchStrategy != BatchByFiles {
			t.Errorf("expected BatchStrategy %s, got %s", BatchByFiles, parallelStage.BatchStrategy)
		}
		if parallelStage.MergeStrategy != MergeReconcile {
			t.Errorf("expected MergeStrategy %s, got %s", MergeReconcile, parallelStage.MergeStrategy)
		}
	})
}

func TestWorkBatch(t *testing.T) {
	t.Run("CreateWorkBatch", func(t *testing.T) {
		batch := WorkBatch{
			Index:     0,
			Files:     []string{"file1.go", "file2.go"},
			Directory: "/project/src",
			Symbols:   []string{"Func1", "Func2"},
		}

		if batch.Index != 0 {
			t.Errorf("expected Index 0, got %d", batch.Index)
		}
		if len(batch.Files) != 2 {
			t.Errorf("expected 2 files, got %d", len(batch.Files))
		}
		if batch.Directory != "/project/src" {
			t.Errorf("expected Directory /project/src, got %s", batch.Directory)
		}
	})
}

func TestAgentResult(t *testing.T) {
	t.Run("CreateAgentResult", func(t *testing.T) {
		result := &AgentResult{
			FilesProcessed: []string{"file1.go", "file2.go"},
			Summary:        "Completed successfully",
			Changes: []FileChange{
				{
					Path:      "file1.go",
					Operation: "modified",
					Diff:      "+added line",
				},
			},
			Artifacts: map[string]string{
				"report": "test passed",
			},
		}

		if len(result.FilesProcessed) != 2 {
			t.Errorf("expected 2 files processed, got %d", len(result.FilesProcessed))
		}
		if result.Summary != "Completed successfully" {
			t.Errorf("expected Summary 'Completed successfully', got %s", result.Summary)
		}
		if len(result.Changes) != 1 {
			t.Errorf("expected 1 change, got %d", len(result.Changes))
		}
		if result.Changes[0].Operation != "modified" {
			t.Errorf("expected Operation 'modified', got %s", result.Changes[0].Operation)
		}
	})
}

func TestFileChange(t *testing.T) {
	tests := []struct {
		name      string
		operation string
	}{
		{"modified file", "modified"},
		{"created file", "created"},
		{"deleted file", "deleted"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			change := FileChange{
				Path:      "test.go",
				Operation: tt.operation,
				Diff:      "diff content",
			}

			if change.Operation != tt.operation {
				t.Errorf("expected Operation %s, got %s", tt.operation, change.Operation)
			}
		})
	}
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

func TestParallelExecutorCreation(t *testing.T) {
	t.Run("DefaultMaxAgents", func(t *testing.T) {
		executor := NewParallelExecutor(nil, nil, 0)
		if executor.maxAgents != 4 {
			t.Errorf("expected default maxAgents 4, got %d", executor.maxAgents)
		}
	})

	t.Run("CustomMaxAgents", func(t *testing.T) {
		executor := NewParallelExecutor(nil, nil, 8)
		if executor.maxAgents != 8 {
			t.Errorf("expected maxAgents 8, got %d", executor.maxAgents)
		}
	})
}

func TestCreateBatchInput(t *testing.T) {
	tmpDir := t.TempDir()
	os.MkdirAll(filepath.Join(tmpDir, ".openexec"), 0755)

	registry, _ := NewAgentRegistry(tmpDir)
	defer registry.Close()

	executor := NewParallelExecutor(registry, nil, 4)

	baseInput := blueprint.NewStageInput("run-1", "test task", "/project")
	baseInput.Variables["key"] = "value"

	batch := WorkBatch{
		Index:     2,
		Files:     []string{"file1.go", "file2.go"},
		Directory: "/project/src",
	}

	batchInput := executor.createBatchInput(baseInput, batch)

	// Check batch-specific variables
	if batchInput.Variables["batch_index"] != "2" {
		t.Errorf("expected batch_index '2', got %s", batchInput.Variables["batch_index"])
	}
	if batchInput.Variables["batch_directory"] != "/project/src" {
		t.Errorf("expected batch_directory '/project/src', got %s", batchInput.Variables["batch_directory"])
	}
	if batchInput.Variables["key"] != "value" {
		t.Errorf("expected original variable 'value', got %s", batchInput.Variables["key"])
	}
}

func TestFormatMergedOutput(t *testing.T) {
	tmpDir := t.TempDir()
	os.MkdirAll(filepath.Join(tmpDir, ".openexec"), 0755)

	registry, _ := NewAgentRegistry(tmpDir)
	defer registry.Close()

	executor := NewParallelExecutor(registry, nil, 4)

	stage := &ParallelStage{
		Stage: &blueprint.Stage{
			Name: "implement",
		},
	}

	outputs := []string{
		"Agent 1 completed successfully",
		"Agent 2 completed successfully",
	}

	errors := []string{
		"Agent 3 failed",
	}

	merged := executor.formatMergedOutput(stage, outputs, errors)

	// Check that output contains expected content
	if !strings.Contains(merged, "Parallel Execution Results") {
		t.Error("expected output to contain 'Parallel Execution Results'")
	}
	if !strings.Contains(merged, "Errors") {
		t.Error("expected output to contain 'Errors'")
	}
	if !strings.Contains(merged, "Agent 1 completed successfully") {
		t.Error("expected output to contain Agent 1 output")
	}
}

func TestParallelBlueprint(t *testing.T) {
	baseBlueprint := &blueprint.Blueprint{
		ID:           "standard_task",
		Name:         "Standard Task",
		Description:  "Default blueprint",
		InitialStage: "gather_context",
		Version:      "1.0",
		Stages: map[string]*blueprint.Stage{
			"gather_context": {
				Name:      "gather_context",
				Type:      "deterministic",
				Toolset:   "repo_readonly",
				OnSuccess: "implement",
			},
			"implement": {
				Name:      "implement",
				Type:      "agentic",
				Toolset:   "coding_backend",
				OnSuccess: "complete",
			},
		},
	}

	parallelBp := ParallelBlueprint(baseBlueprint)

	if parallelBp.ID != "standard_task-parallel" {
		t.Errorf("expected ID 'standard_task-parallel', got %s", parallelBp.ID)
	}
	if parallelBp.Name != "Standard Task (Parallel)" {
		t.Errorf("expected Name 'Standard Task (Parallel)', got %s", parallelBp.Name)
	}
	if len(parallelBp.Stages) != 2 {
		t.Errorf("expected 2 stages, got %d", len(parallelBp.Stages))
	}
}


