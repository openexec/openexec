package checkpoint

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/openexec/openexec/internal/blueprint"
)

func TestCheckpointManager(t *testing.T) {
	tmpDir := t.TempDir()
	os.MkdirAll(filepath.Join(tmpDir, ".openexec"), 0755)

	manager, err := NewManager(tmpDir)
	if err != nil {
		t.Fatalf("failed to create checkpoint manager: %v", err)
	}
	defer manager.Close()

	t.Run("Create Checkpoint", func(t *testing.T) {
		// Create test files
		testFile := filepath.Join(tmpDir, "test.go")
		os.WriteFile(testFile, []byte("package main"), 0644)

		run := &blueprint.Run{
			ID:           "test-run-1",
			BlueprintID:  "test-blueprint",
			CurrentStage: "implement",
			Results: []*blueprint.StageResult{
				{
					StageName: "gather_context",
					Status:    "completed",
				},
			},
		}

		checkpoint, err := manager.Create(run, tmpDir)
		if err != nil {
			t.Fatalf("Create failed: %v", err)
		}

		if checkpoint.ID == "" {
			t.Error("expected checkpoint ID")
		}
		if checkpoint.RunID != run.ID {
			t.Errorf("expected run ID %s, got %s", run.ID, checkpoint.RunID)
		}
		if checkpoint.StageName != run.CurrentStage {
			t.Errorf("expected stage %s, got %s", run.CurrentStage, checkpoint.StageName)
		}
		if checkpoint.Checksum == "" {
			t.Error("expected checksum")
		}
		if checkpoint.Status != CheckpointStatusValid {
			t.Errorf("expected status valid, got %s", checkpoint.Status)
		}
	})

	t.Run("Restore Checkpoint", func(t *testing.T) {
		// Create test file
		testFile := filepath.Join(tmpDir, "restore.go")
		os.WriteFile(testFile, []byte("package main"), 0644)

		run := &blueprint.Run{
			ID:           "test-run-2",
			BlueprintID:  "test-blueprint",
			CurrentStage: "test",
			Results:      []*blueprint.StageResult{},
		}

		// Create checkpoint
		checkpoint, _ := manager.Create(run, tmpDir)

		// Restore
		restored, err := manager.Restore(checkpoint.ID)
		if err != nil {
			t.Fatalf("Restore failed: %v", err)
		}

		if restored.ID != checkpoint.ID {
			t.Errorf("expected ID %s, got %s", checkpoint.ID, restored.ID)
		}
		if restored.RunID != checkpoint.RunID {
			t.Errorf("expected run ID %s, got %s", checkpoint.RunID, restored.RunID)
		}
	})

	t.Run("Restore Stale Checkpoint", func(t *testing.T) {
		// Create test file
		testFile := filepath.Join(tmpDir, "stale.go")
		os.WriteFile(testFile, []byte("original content"), 0644)

		run := &blueprint.Run{
			ID:           "test-run-3",
			BlueprintID:  "test-blueprint",
			CurrentStage: "implement",
			Results:      []*blueprint.StageResult{},
		}

		// Create checkpoint
		checkpoint, _ := manager.Create(run, tmpDir)

		// Modify file
		time.Sleep(10 * time.Millisecond)
		os.WriteFile(testFile, []byte("modified content"), 0644)

		// Try to restore - should detect staleness
		restored, err := manager.Restore(checkpoint.ID)
		if err == nil {
			t.Error("expected error for stale checkpoint")
		}
		if restored.Status != CheckpointStatusStale {
			t.Errorf("expected status stale, got %s", restored.Status)
		}
	})

	t.Run("Get Latest Checkpoint", func(t *testing.T) {
		run := &blueprint.Run{
			ID:           "test-run-4",
			BlueprintID:  "test-blueprint",
			CurrentStage: "stage1",
			Results:      []*blueprint.StageResult{},
		}

		// Create first checkpoint
		_, _ = manager.Create(run, tmpDir)
		time.Sleep(10 * time.Millisecond)

		// Update run and create second checkpoint
		run.CurrentStage = "stage2"
		cp2, _ := manager.Create(run, tmpDir)

		// Get latest
		latest, err := manager.GetLatest(run.ID)
		if err != nil {
			t.Fatalf("GetLatest failed: %v", err)
		}

		if latest == nil {
			t.Fatal("expected latest checkpoint")
		}
		if latest.ID != cp2.ID {
			t.Errorf("expected latest checkpoint %s, got %s", cp2.ID, latest.ID)
		}
		if latest.StageName != "stage2" {
			t.Errorf("expected stage stage2, got %s", latest.StageName)
		}
	})

	t.Run("List Checkpoints", func(t *testing.T) {
		run := &blueprint.Run{
			ID:           "test-run-5",
			BlueprintID:  "test-blueprint",
			CurrentStage: "stage1",
			Results:      []*blueprint.StageResult{},
		}

		// Create multiple checkpoints
		manager.Create(run, tmpDir)
		time.Sleep(10 * time.Millisecond)
		manager.Create(run, tmpDir)

		// List
		checkpoints, err := manager.List(run.ID)
		if err != nil {
			t.Fatalf("List failed: %v", err)
		}

		if len(checkpoints) != 2 {
			t.Errorf("expected 2 checkpoints, got %d", len(checkpoints))
		}
	})

	t.Run("Delete Checkpoint", func(t *testing.T) {
		run := &blueprint.Run{
			ID:           "test-run-6",
			BlueprintID:  "test-blueprint",
			CurrentStage: "stage1",
			Results:      []*blueprint.StageResult{},
		}

		checkpoint, _ := manager.Create(run, tmpDir)

		// Delete
		err := manager.Delete(checkpoint.ID)
		if err != nil {
			t.Fatalf("Delete failed: %v", err)
		}

		// Try to restore - should fail
		_, err = manager.Restore(checkpoint.ID)
		if err == nil {
			t.Error("expected error after deletion")
		}
	})

	t.Run("Cleanup Old Checkpoints", func(t *testing.T) {
		run := &blueprint.Run{
			ID:           "test-run-7",
			BlueprintID:  "test-blueprint",
			CurrentStage: "stage1",
			Results:      []*blueprint.StageResult{},
		}

		checkpoint, _ := manager.Create(run, tmpDir)

		// Cleanup old checkpoints (older than 1 hour ago)
		err := manager.Cleanup(time.Now().Add(-1 * time.Hour))
		if err != nil {
			t.Fatalf("Cleanup failed: %v", err)
		}

		// Checkpoint should still exist (just created)
		_, err = manager.Restore(checkpoint.ID)
		if err != nil {
			t.Error("checkpoint should still exist")
		}
	})

	t.Run("File State Capture", func(t *testing.T) {
		// Create test file with known content
		testFile := filepath.Join(tmpDir, "capture.go")
		content := []byte("package main\n\nfunc main() {}")
		os.WriteFile(testFile, content, 0644)

		run := &blueprint.Run{
			ID:           "test-run-8",
			BlueprintID:  "test-blueprint",
			CurrentStage: "implement",
			Results:      []*blueprint.StageResult{},
		}

		checkpoint, _ := manager.Create(run, tmpDir)

		// Check file state was captured
		state, ok := checkpoint.WorkingState[testFile]
		if !ok {
			t.Fatal("expected file state to be captured")
		}
		if state.Path != testFile {
			t.Errorf("expected path %s, got %s", testFile, state.Path)
		}
		if state.Size != int64(len(content)) {
			t.Errorf("expected size %d, got %d", len(content), state.Size)
		}
		if state.Hash == "" {
			t.Error("expected file hash")
		}
	})
}

func TestCheckpointStatus(t *testing.T) {
	tests := []struct {
		status   CheckpointStatus
		expected string
	}{
		{CheckpointStatusValid, "valid"},
		{CheckpointStatusStale, "stale"},
		{CheckpointStatusCorrupted, "corrupted"},
	}

	for _, tt := range tests {
		t.Run(string(tt.status), func(t *testing.T) {
			if string(tt.status) != tt.expected {
				t.Errorf("expected %s, got %s", tt.expected, tt.status)
			}
		})
	}
}

func TestFileState(t *testing.T) {
	state := FileState{
		Path:       "/path/to/file.go",
		Hash:       "abc123",
		Size:       1024,
		ModifiedAt: time.Now(),
	}

	if state.Path != "/path/to/file.go" {
		t.Errorf("expected path /path/to/file.go, got %s", state.Path)
	}
	if state.Hash != "abc123" {
		t.Errorf("expected hash abc123, got %s", state.Hash)
	}
	if state.Size != 1024 {
		t.Errorf("expected size 1024, got %d", state.Size)
	}
}

func TestShouldIgnore(t *testing.T) {
	tests := []struct {
		path     string
		expected bool
	}{
		{"/project/.git/config", true},
		{"/project/node_modules/package.json", true},
		{"/project/__pycache__/module.pyc", true},
		{"/project/dist/bundle.js", true},
		{"/project/.openexec/config.json", true},
		{"/project/src/main.go", false},
		{"/project/README.md", false},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			result := shouldIgnore(tt.path)
			if result != tt.expected {
				t.Errorf("shouldIgnore(%q) = %v, expected %v", tt.path, result, tt.expected)
			}
		})
	}
}

func TestChecksum(t *testing.T) {
	tmpDir := t.TempDir()
	os.MkdirAll(filepath.Join(tmpDir, ".openexec"), 0755)

	manager, _ := NewManager(tmpDir)
	defer manager.Close()

	run := &blueprint.Run{
		ID:           "checksum-test",
		BlueprintID:  "test",
		CurrentStage: "stage1",
		Results:      []*blueprint.StageResult{},
	}

	checkpoint, _ := manager.Create(run, tmpDir)

	// Verify checksum is calculated
	if checkpoint.Checksum == "" {
		t.Error("expected checksum to be calculated")
	}

	// Verify checksum verification works
	if !manager.verifyChecksum(checkpoint) {
		t.Error("checksum verification failed for valid checkpoint")
	}

	// Corrupt data and verify checksum fails
	checkpoint.StageName = "corrupted"
	if manager.verifyChecksum(checkpoint) {
		t.Error("checksum verification should fail for corrupted data")
	}
}
