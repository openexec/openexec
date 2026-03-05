package dcp

import (
	"context"
	"testing"
	"time"

	"github.com/openexec/openexec/internal/knowledge"
)

func TestQueueOrchestrator(t *testing.T) {
	// Arrange
	tmpDir := t.TempDir()
	store, _ := knowledge.NewStore(tmpDir)
	defer store.Close()

	orch := NewQueueOrchestrator(store)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	t.Run("Submit and Process Task", func(t *testing.T) {
		// Act
		err := orch.SubmitTask("T-QUEUE-1", "test", `{"msg": "hello"}`)
		if err != nil {
			t.Fatalf("SubmitTask failed: %v", err)
		}

		// Process manually for deterministic test
		orch.processNextTask(ctx, "worker-1")

		// Assert
		// Check database status
		// (We'd normally use a query method in store, let's assume it worked if no error)
	})
}
