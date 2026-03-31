package release

import (
	"context"
	"database/sql"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

func TestCheckpoint_NewCheckpoint(t *testing.T) {
	cp := NewCheckpoint("cp-1", "run-1", "implement")

	if cp.ID != "cp-1" {
		t.Errorf("expected ID cp-1, got %s", cp.ID)
	}
	if cp.RunID != "run-1" {
		t.Errorf("expected RunID run-1, got %s", cp.RunID)
	}
	if cp.Stage != "implement" {
		t.Errorf("expected Stage implement, got %s", cp.Stage)
	}
	if cp.ToolCallLog == nil {
		t.Error("expected ToolCallLog to be initialized")
	}
	if cp.Artifacts == nil {
		t.Error("expected Artifacts to be initialized")
	}
	if cp.CreatedAt.IsZero() {
		t.Error("expected CreatedAt to be set")
	}
}

func TestCheckpoint_ToolCallTracking(t *testing.T) {
	cp := NewCheckpoint("cp-1", "run-1", "implement")

	if cp.HasToolCall("tool-call-1") {
		t.Error("expected HasToolCall to return false for unknown key")
	}

	cp.AddToolCall("tool-call-1")

	if !cp.HasToolCall("tool-call-1") {
		t.Error("expected HasToolCall to return true after adding")
	}
	if cp.HasToolCall("tool-call-2") {
		t.Error("expected HasToolCall to return false for different key")
	}
}

func TestCheckpoint_Artifacts(t *testing.T) {
	cp := NewCheckpoint("cp-1", "run-1", "implement")

	cp.SetArtifact("file.go", "content")

	if cp.Artifacts["file.go"] != "content" {
		t.Errorf("expected artifact content, got %s", cp.Artifacts["file.go"])
	}
}

func TestCheckpoint_ContextHash(t *testing.T) {
	cp := NewCheckpoint("cp-1", "run-1", "implement")

	cp.ComputeContextHash([]byte("test context data"))

	if cp.ContextHash == "" {
		t.Error("expected ContextHash to be computed")
	}
	if len(cp.ContextHash) != 64 {
		t.Errorf("expected ContextHash to be 64 chars (sha256 hex), got %d", len(cp.ContextHash))
	}
}

func TestSQLiteStore_CheckpointOperations(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()

	store, err := NewSQLiteStore(db)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}

	ctx := context.Background()

	// Create a checkpoint
	cp := NewCheckpoint("cp-1", "run-1", "implement")
	cp.SetMessageHistory([]byte(`{"messages": []}`))
	cp.AddToolCall("tool-1")
	cp.SetArtifact("main.go", "package main")
	cp.ComputeContextHash([]byte("context"))

	if err := store.CreateCheckpoint(ctx, cp); err != nil {
		t.Fatalf("failed to create checkpoint: %v", err)
	}

	// Retrieve the checkpoint
	retrieved, err := store.GetCheckpoint(ctx, "cp-1")
	if err != nil {
		t.Fatalf("failed to get checkpoint: %v", err)
	}

	if retrieved.ID != cp.ID {
		t.Errorf("expected ID %s, got %s", cp.ID, retrieved.ID)
	}
	if retrieved.RunID != cp.RunID {
		t.Errorf("expected RunID %s, got %s", cp.RunID, retrieved.RunID)
	}
	if retrieved.Stage != cp.Stage {
		t.Errorf("expected Stage %s, got %s", cp.Stage, retrieved.Stage)
	}
	if string(retrieved.MessageHistory) != string(cp.MessageHistory) {
		t.Errorf("expected MessageHistory %s, got %s", string(cp.MessageHistory), string(retrieved.MessageHistory))
	}
	if len(retrieved.ToolCallLog) != 1 || retrieved.ToolCallLog[0] != "tool-1" {
		t.Errorf("expected ToolCallLog [tool-1], got %v", retrieved.ToolCallLog)
	}
	if retrieved.Artifacts["main.go"] != "package main" {
		t.Errorf("expected artifact 'package main', got %s", retrieved.Artifacts["main.go"])
	}
	if retrieved.ContextHash != cp.ContextHash {
		t.Errorf("expected ContextHash %s, got %s", cp.ContextHash, retrieved.ContextHash)
	}

	// Test duplicate creation
	if err := store.CreateCheckpoint(ctx, cp); err != ErrCheckpointAlreadyExist {
		t.Errorf("expected ErrCheckpointAlreadyExist, got %v", err)
	}

	// Test not found
	_, err = store.GetCheckpoint(ctx, "non-existent")
	if err != ErrCheckpointNotFound {
		t.Errorf("expected ErrCheckpointNotFound, got %v", err)
	}
}

func TestSQLiteStore_ListCheckpointsForRun(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()

	store, err := NewSQLiteStore(db)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}

	ctx := context.Background()

	// Create multiple checkpoints for the same run
	cp1 := NewCheckpoint("cp-1", "run-1", "gather_context")
	cp1.CreatedAt = time.Now().Add(-2 * time.Hour)

	cp2 := NewCheckpoint("cp-2", "run-1", "implement")
	cp2.CreatedAt = time.Now().Add(-1 * time.Hour)

	cp3 := NewCheckpoint("cp-3", "run-2", "implement") // Different run

	if err := store.CreateCheckpoint(ctx, cp1); err != nil {
		t.Fatalf("failed to create checkpoint 1: %v", err)
	}
	if err := store.CreateCheckpoint(ctx, cp2); err != nil {
		t.Fatalf("failed to create checkpoint 2: %v", err)
	}
	if err := store.CreateCheckpoint(ctx, cp3); err != nil {
		t.Fatalf("failed to create checkpoint 3: %v", err)
	}

	// List checkpoints for run-1
	checkpoints, err := store.ListCheckpointsForRun(ctx, "run-1")
	if err != nil {
		t.Fatalf("failed to list checkpoints: %v", err)
	}

	if len(checkpoints) != 2 {
		t.Fatalf("expected 2 checkpoints, got %d", len(checkpoints))
	}

	// Verify ordering by created_at ASC
	if checkpoints[0].ID != "cp-1" {
		t.Errorf("expected first checkpoint cp-1, got %s", checkpoints[0].ID)
	}
	if checkpoints[1].ID != "cp-2" {
		t.Errorf("expected second checkpoint cp-2, got %s", checkpoints[1].ID)
	}
}

func TestSQLiteStore_GetLatestCheckpoint(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()

	store, err := NewSQLiteStore(db)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}

	ctx := context.Background()

	// Create multiple checkpoints
	cp1 := NewCheckpoint("cp-1", "run-1", "gather_context")
	cp1.CreatedAt = time.Now().Add(-2 * time.Hour)

	cp2 := NewCheckpoint("cp-2", "run-1", "implement")
	cp2.CreatedAt = time.Now().Add(-1 * time.Hour)

	if err := store.CreateCheckpoint(ctx, cp1); err != nil {
		t.Fatalf("failed to create checkpoint 1: %v", err)
	}
	if err := store.CreateCheckpoint(ctx, cp2); err != nil {
		t.Fatalf("failed to create checkpoint 2: %v", err)
	}

	// Get latest checkpoint
	latest, err := store.GetLatestCheckpoint(ctx, "run-1")
	if err != nil {
		t.Fatalf("failed to get latest checkpoint: %v", err)
	}

	if latest.ID != "cp-2" {
		t.Errorf("expected latest checkpoint cp-2, got %s", latest.ID)
	}

	// Test not found for non-existent run
	_, err = store.GetLatestCheckpoint(ctx, "non-existent-run")
	if err != ErrCheckpointNotFound {
		t.Errorf("expected ErrCheckpointNotFound, got %v", err)
	}
}

func TestSQLiteStore_DeleteCheckpoint(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()

	store, err := NewSQLiteStore(db)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}

	ctx := context.Background()

	// Create a checkpoint
	cp := NewCheckpoint("cp-1", "run-1", "implement")
	if err := store.CreateCheckpoint(ctx, cp); err != nil {
		t.Fatalf("failed to create checkpoint: %v", err)
	}

	// Delete the checkpoint
	if err := store.DeleteCheckpoint(ctx, "cp-1"); err != nil {
		t.Fatalf("failed to delete checkpoint: %v", err)
	}

	// Verify it's deleted
	_, err = store.GetCheckpoint(ctx, "cp-1")
	if err != ErrCheckpointNotFound {
		t.Errorf("expected ErrCheckpointNotFound after delete, got %v", err)
	}

	// Test delete non-existent
	if err := store.DeleteCheckpoint(ctx, "non-existent"); err != ErrCheckpointNotFound {
		t.Errorf("expected ErrCheckpointNotFound, got %v", err)
	}
}

func TestSQLiteStore_DeleteCheckpointsForRun(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()

	store, err := NewSQLiteStore(db)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}

	ctx := context.Background()

	// Create checkpoints for two runs
	cp1 := NewCheckpoint("cp-1", "run-1", "gather_context")
	cp2 := NewCheckpoint("cp-2", "run-1", "implement")
	cp3 := NewCheckpoint("cp-3", "run-2", "implement")

	if err := store.CreateCheckpoint(ctx, cp1); err != nil {
		t.Fatalf("failed to create checkpoint 1: %v", err)
	}
	if err := store.CreateCheckpoint(ctx, cp2); err != nil {
		t.Fatalf("failed to create checkpoint 2: %v", err)
	}
	if err := store.CreateCheckpoint(ctx, cp3); err != nil {
		t.Fatalf("failed to create checkpoint 3: %v", err)
	}

	// Delete all checkpoints for run-1
	if err := store.DeleteCheckpointsForRun(ctx, "run-1"); err != nil {
		t.Fatalf("failed to delete checkpoints for run: %v", err)
	}

	// Verify run-1 checkpoints are deleted
	checkpoints, err := store.ListCheckpointsForRun(ctx, "run-1")
	if err != nil {
		t.Fatalf("failed to list checkpoints: %v", err)
	}
	if len(checkpoints) != 0 {
		t.Errorf("expected 0 checkpoints for run-1, got %d", len(checkpoints))
	}

	// Verify run-2 checkpoint still exists
	cp, err := store.GetCheckpoint(ctx, "cp-3")
	if err != nil {
		t.Fatalf("expected run-2 checkpoint to exist: %v", err)
	}
	if cp.ID != "cp-3" {
		t.Errorf("expected checkpoint cp-3, got %s", cp.ID)
	}
}

func TestManager_CheckpointOperations(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()

	mgr, err := NewManagerWithDB("/tmp", nil, db)
	if err != nil {
		t.Fatalf("failed to create manager: %v", err)
	}
	defer mgr.Close()

	// Create a checkpoint
	cp := NewCheckpoint("cp-1", "run-1", "implement")
	cp.SetArtifact("test.go", "package test")

	if err := mgr.CreateCheckpoint(cp); err != nil {
		t.Fatalf("failed to create checkpoint: %v", err)
	}

	// Get checkpoint
	retrieved := mgr.GetCheckpoint("cp-1")
	if retrieved == nil {
		t.Fatal("expected checkpoint to exist")
	}
	if retrieved.Stage != "implement" {
		t.Errorf("expected stage implement, got %s", retrieved.Stage)
	}

	// Get non-existent
	if got := mgr.GetCheckpoint("non-existent"); got != nil {
		t.Error("expected nil for non-existent checkpoint")
	}

	// Create more checkpoints (ensure different timestamp)
	time.Sleep(10 * time.Millisecond)
	cp2 := NewCheckpoint("cp-2", "run-1", "review")
	if err := mgr.CreateCheckpoint(cp2); err != nil {
		t.Fatalf("failed to create checkpoint 2: %v", err)
	}

	// List checkpoints
	checkpoints := mgr.ListCheckpointsForRun("run-1")
	if len(checkpoints) != 2 {
		t.Errorf("expected 2 checkpoints, got %d", len(checkpoints))
	}

	// Get latest
	latest := mgr.GetLatestCheckpoint("run-1")
	if latest == nil {
		t.Error("expected latest checkpoint to exist, got nil")
	} else if latest.ID != "cp-2" {
		t.Errorf("expected latest checkpoint to be cp-2, got %s (stage: %s, created: %v)", latest.ID, latest.Stage, latest.CreatedAt)
	}

	// Delete single checkpoint
	if err := mgr.DeleteCheckpoint("cp-1"); err != nil {
		t.Fatalf("failed to delete checkpoint: %v", err)
	}

	// Delete all for run
	if err := mgr.DeleteCheckpointsForRun("run-1"); err != nil {
		t.Fatalf("failed to delete checkpoints for run: %v", err)
	}

	// Verify all deleted
	checkpoints = mgr.ListCheckpointsForRun("run-1")
	if len(checkpoints) != 0 {
		t.Errorf("expected 0 checkpoints after delete, got %d", len(checkpoints))
	}
}
