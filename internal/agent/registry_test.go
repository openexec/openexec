package agent

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestAgentRegistry(t *testing.T) {
	// Arrange
	tmpDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(tmpDir, ".openexec"), 0755); err != nil {
		t.Fatalf("failed to create .openexec dir: %v", err)
	}

	registry, err := NewAgentRegistry(tmpDir)
	if err != nil {
		t.Fatalf("failed to create agent registry: %v", err)
	}
	defer registry.Close()

	t.Run("Register and Get", func(t *testing.T) {
		agent := &Agent{
			ID:          "test-agent-1",
			Type:        AgentTypeWorker,
			Status:      AgentStatusIdle,
			BlueprintID: "test-blueprint",
			RunID:       "test-run-1",
			StageName:   "implement",
			BatchIndex:  0,
			BatchSize:   5,
			StartedAt:   time.Now().UTC(),
		}

		// Register
		err := registry.Register(agent)
		if err != nil {
			t.Fatalf("Register failed: %v", err)
		}

		// Get
		retrieved, err := registry.Get(agent.ID)
		if err != nil {
			t.Fatalf("Get failed: %v", err)
		}

		// Assert
		if retrieved.ID != agent.ID {
			t.Errorf("expected ID %s, got %s", agent.ID, retrieved.ID)
		}
		if retrieved.Type != agent.Type {
			t.Errorf("expected Type %s, got %s", agent.Type, retrieved.Type)
		}
		if retrieved.Status != agent.Status {
			t.Errorf("expected Status %s, got %s", agent.Status, retrieved.Status)
		}
	})

	t.Run("UpdateStatus", func(t *testing.T) {
		agent := &Agent{
			ID:          "test-agent-2",
			Type:        AgentTypeWorker,
			Status:      AgentStatusIdle,
			BlueprintID: "test-blueprint",
			RunID:       "test-run-2",
			StartedAt:   time.Now().UTC(),
		}

		err := registry.Register(agent)
		if err != nil {
			t.Fatalf("Register failed: %v", err)
		}

		// Update status
		err = registry.UpdateStatus(agent.ID, AgentStatusRunning)
		if err != nil {
			t.Fatalf("UpdateStatus failed: %v", err)
		}

		// Verify
		retrieved, err := registry.Get(agent.ID)
		if err != nil {
			t.Fatalf("Get failed: %v", err)
		}
		if retrieved.Status != AgentStatusRunning {
			t.Errorf("expected Status %s, got %s", AgentStatusRunning, retrieved.Status)
		}
	})

	t.Run("Complete", func(t *testing.T) {
		agent := &Agent{
			ID:          "test-agent-3",
			Type:        AgentTypeWorker,
			Status:      AgentStatusRunning,
			BlueprintID: "test-blueprint",
			RunID:       "test-run-3",
			StartedAt:   time.Now().UTC(),
		}

		err := registry.Register(agent)
		if err != nil {
			t.Fatalf("Register failed: %v", err)
		}

		// Complete with result
		result := &AgentResult{
			FilesProcessed: []string{"file1.go", "file2.go"},
			Summary:        "Completed successfully",
			Changes: []FileChange{
				{Path: "file1.go", Operation: "modified", Diff: "+added line"},
			},
		}

		err = registry.Complete(agent.ID, result)
		if err != nil {
			t.Fatalf("Complete failed: %v", err)
		}

		// Verify
		retrieved, err := registry.Get(agent.ID)
		if err != nil {
			t.Fatalf("Get failed: %v", err)
		}
		if retrieved.Status != AgentStatusCompleted {
			t.Errorf("expected Status %s, got %s", AgentStatusCompleted, retrieved.Status)
		}
		if retrieved.Result == nil {
			t.Fatal("expected Result to be set")
		}
		if len(retrieved.Result.FilesProcessed) != 2 {
			t.Errorf("expected 2 files processed, got %d", len(retrieved.Result.FilesProcessed))
		}
	})

	t.Run("Fail", func(t *testing.T) {
		agent := &Agent{
			ID:          "test-agent-4",
			Type:        AgentTypeWorker,
			Status:      AgentStatusRunning,
			BlueprintID: "test-blueprint",
			RunID:       "test-run-4",
			StartedAt:   time.Now().UTC(),
		}

		err := registry.Register(agent)
		if err != nil {
			t.Fatalf("Register failed: %v", err)
		}

		// Mark as failed
		err = registry.Fail(agent.ID, "something went wrong")
		if err != nil {
			t.Fatalf("Fail failed: %v", err)
		}

		// Verify
		retrieved, err := registry.Get(agent.ID)
		if err != nil {
			t.Fatalf("Get failed: %v", err)
		}
		if retrieved.Status != AgentStatusFailed {
			t.Errorf("expected Status %s, got %s", AgentStatusFailed, retrieved.Status)
		}
		if retrieved.Error != "something went wrong" {
			t.Errorf("expected Error 'something went wrong', got %s", retrieved.Error)
		}
	})

	t.Run("ListByRun", func(t *testing.T) {
		blueprintID := "list-test-blueprint"
		runID := "list-test-run"

		// Create multiple agents
		for i := 0; i < 3; i++ {
			agent := &Agent{
				ID:          fmt.Sprintf("list-agent-%d", i),
				Type:        AgentTypeWorker,
				Status:      AgentStatusIdle,
				BlueprintID: blueprintID,
				RunID:       runID,
				StartedAt:   time.Now().UTC(),
			}
			err := registry.Register(agent)
			if err != nil {
				t.Fatalf("Register failed: %v", err)
			}
		}

		// List agents
		agents, err := registry.ListByRun(blueprintID, runID)
		if err != nil {
			t.Fatalf("ListByRun failed: %v", err)
		}

		if len(agents) != 3 {
			t.Errorf("expected 3 agents, got %d", len(agents))
		}
	})

	t.Run("ListByStatus", func(t *testing.T) {
		// Create agents with different statuses
		statuses := []AgentStatus{AgentStatusIdle, AgentStatusRunning, AgentStatusCompleted}
		for i, status := range statuses {
			agent := &Agent{
				ID:          fmt.Sprintf("status-agent-%d", i),
				Type:        AgentTypeWorker,
				Status:      status,
				BlueprintID: "status-test",
				RunID:       "status-run",
				StartedAt:   time.Now().UTC(),
			}
			err := registry.Register(agent)
			if err != nil {
				t.Fatalf("Register failed: %v", err)
			}
		}

		// List by status
		agents, err := registry.ListByStatus(AgentStatusCompleted)
		if err != nil {
			t.Fatalf("ListByStatus failed: %v", err)
		}

		found := false
		for _, agent := range agents {
			if agent.ID == "status-agent-2" {
				found = true
				break
			}
		}
		if !found {
			t.Error("expected to find completed agent")
		}
	})

	t.Run("CountByStatus", func(t *testing.T) {
		blueprintID := "count-test-blueprint"
		runID := "count-test-run"

		// Create agents
		for i := 0; i < 5; i++ {
			status := AgentStatusIdle
			if i >= 3 {
				status = AgentStatusCompleted
			}
			agent := &Agent{
				ID:          fmt.Sprintf("count-agent-%d", i),
				Type:        AgentTypeWorker,
				Status:      status,
				BlueprintID: blueprintID,
				RunID:       runID,
				StartedAt:   time.Now().UTC(),
			}
			err := registry.Register(agent)
			if err != nil {
				t.Fatalf("Register failed: %v", err)
			}
		}

		// Count by status
		counts, err := registry.CountByStatus(blueprintID, runID)
		if err != nil {
			t.Fatalf("CountByStatus failed: %v", err)
		}

		if counts[AgentStatusIdle] != 3 {
			t.Errorf("expected 3 idle agents, got %d", counts[AgentStatusIdle])
		}
		if counts[AgentStatusCompleted] != 2 {
			t.Errorf("expected 2 completed agents, got %d", counts[AgentStatusCompleted])
		}
	})

	t.Run("Cleanup", func(t *testing.T) {
		// Create old agent
		oldAgent := &Agent{
			ID:          "old-agent",
			Type:        AgentTypeWorker,
			Status:      AgentStatusCompleted,
			BlueprintID: "cleanup-test",
			RunID:       "cleanup-run",
			StartedAt:   time.Now().UTC().Add(-48 * time.Hour),
		}
		err := registry.Register(oldAgent)
		if err != nil {
			t.Fatalf("Register failed: %v", err)
		}

		// Manually set completed_at to old time
		_, err = registry.db.Exec(
			"UPDATE agents SET completed_at = ? WHERE id = ?",
			time.Now().UTC().Add(-48*time.Hour),
			oldAgent.ID,
		)
		if err != nil {
			t.Fatalf("Failed to update completed_at: %v", err)
		}

		// Cleanup old records
		err = registry.Cleanup(time.Now().UTC().Add(-24 * time.Hour))
		if err != nil {
			t.Fatalf("Cleanup failed: %v", err)
		}

		// Verify agent is gone
		_, err = registry.Get(oldAgent.ID)
		if err == nil {
			t.Error("expected agent to be cleaned up")
		}
	})
}

func TestWorkBatchCreation(t *testing.T) {
	tmpDir := t.TempDir()
	os.MkdirAll(filepath.Join(tmpDir, ".openexec"), 0755)

	registry, _ := NewAgentRegistry(tmpDir)
	defer registry.Close()

	executor := NewParallelExecutor(registry, nil, 4)

	t.Run("BatchByFiles", func(t *testing.T) {
		stage := &ParallelStage{
			Parallel:      true,
			MaxAgents:     3,
			BatchStrategy: BatchByFiles,
		}

		files := []string{
			"file1.go", "file2.go", "file3.go",
			"file4.go", "file5.go", "file6.go",
			"file7.go", "file8.go", "file9.go",
		}

		batches := executor.createBatches(stage, files)

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

	t.Run("BatchWithFewFiles", func(t *testing.T) {
		stage := &ParallelStage{
			Parallel:      true,
			MaxAgents:     5,
			BatchStrategy: BatchByFiles,
		}

		files := []string{"file1.go", "file2.go"}

		batches := executor.createBatches(stage, files)

		if len(batches) != 2 {
			t.Errorf("expected 2 batches for 2 files, got %d", len(batches))
		}
	})

	t.Run("BatchWithSingleFile", func(t *testing.T) {
		stage := &ParallelStage{
			Parallel:      true,
			MaxAgents:     4,
			BatchStrategy: BatchByFiles,
		}

		files := []string{"file1.go"}

		batches := executor.createBatches(stage, files)

		if len(batches) != 1 {
			t.Errorf("expected 1 batch for 1 file, got %d", len(batches))
		}
	})
}

func TestAgentTypes(t *testing.T) {
	tests := []struct {
		agentType AgentType
		expected  string
	}{
		{AgentTypeWorker, "worker"},
		{AgentTypeCoordinator, "coordinator"},
		{AgentTypeReviewer, "reviewer"},
	}

	for _, tt := range tests {
		t.Run(string(tt.agentType), func(t *testing.T) {
			if string(tt.agentType) != tt.expected {
				t.Errorf("expected %s, got %s", tt.expected, tt.agentType)
			}
		})
	}
}

func TestAgentStatuses(t *testing.T) {
	tests := []struct {
		status   AgentStatus
		expected string
	}{
		{AgentStatusIdle, "idle"},
		{AgentStatusRunning, "running"},
		{AgentStatusCompleted, "completed"},
		{AgentStatusFailed, "failed"},
		{AgentStatusCancelled, "cancelled"},
	}

	for _, tt := range tests {
		t.Run(string(tt.status), func(t *testing.T) {
			if string(tt.status) != tt.expected {
				t.Errorf("expected %s, got %s", tt.expected, tt.status)
			}
		})
	}
}

func TestBatchStrategies(t *testing.T) {
	tests := []struct {
		strategy BatchStrategy
		expected string
	}{
		{BatchByFiles, "files"},
		{BatchByDirectory, "directory"},
		{BatchBySymbol, "symbol"},
	}

	for _, tt := range tests {
		t.Run(string(tt.strategy), func(t *testing.T) {
			if string(tt.strategy) != tt.expected {
				t.Errorf("expected %s, got %s", tt.expected, tt.strategy)
			}
		})
	}
}

func TestMergeStrategies(t *testing.T) {
	tests := []struct {
		strategy MergeStrategy
		expected string
	}{
		{MergeReconcile, "reconcile"},
		{MergeSequential, "sequential"},
		{MergeManual, "manual"},
	}

	for _, tt := range tests {
		t.Run(string(tt.strategy), func(t *testing.T) {
			if string(tt.strategy) != tt.expected {
				t.Errorf("expected %s, got %s", tt.expected, tt.strategy)
			}
		})
	}
}


