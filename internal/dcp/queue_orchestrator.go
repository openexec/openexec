package dcp

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/openexec/openexec/internal/knowledge"
)

// QueueOrchestrator manages durable task execution within Go
type QueueOrchestrator struct {
	store *knowledge.Store
}

func NewQueueOrchestrator(s *knowledge.Store) *QueueOrchestrator {
	return &QueueOrchestrator{store: s}
}

// SubmitTask adds a task to the durable queue
func (o *QueueOrchestrator) SubmitTask(id, tType, payload string) error {
	log.Printf("[Queue] Submitting task %s (%s)", id, tType)
	return o.store.EnqueueTask(id, tType, payload)
}

// StartWorker begins a long-running worker that polls the SQLite queue
func (o *QueueOrchestrator) StartWorker(ctx context.Context, workerID string) {
	log.Printf("[Queue] Starting durable worker %s", workerID)

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			o.processNextTask(ctx, workerID)
		}
	}
}

func (o *QueueOrchestrator) processNextTask(ctx context.Context, workerID string) {
	// 1. Claim a task atomically
	id, tType, payload, err := o.store.ClaimTask(workerID)
	if err != nil {
		return // No tasks or error
	}
	if id == "" {
		return
	}

	log.Printf("[Queue] Worker %s claimed task %s", workerID, id)

	// 2. Execute with Panic Recovery (BEAM-style self-healing)
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[Queue] CRITICAL: Task %s panicked: %v", id, r)
			o.store.UpdateTaskStatus(id, "failed", fmt.Sprintf("panic: %v", r))
		}
	}()

	// 3. Execution logic
	// In production, this would call the Coordinator.ProcessQuery or Agent loop
	err = o.dummyExecute(id, tType, payload)

	if err != nil {
		log.Printf("[Queue] Task %s failed: %v", id, err)
		o.store.UpdateTaskStatus(id, "failed", err.Error())
	} else {
		log.Printf("[Queue] Task %s completed successfully", id)
		o.store.UpdateTaskStatus(id, "completed", "")
	}
}

func (o *QueueOrchestrator) dummyExecute(id, tType, payload string) error {
	// Simulate work
	time.Sleep(500 * time.Millisecond)
	return nil
}
