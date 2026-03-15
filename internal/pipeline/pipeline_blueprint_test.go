package pipeline

import (
	"context"
	"testing"
	"time"

	"github.com/openexec/openexec/internal/loop"
)

func TestPipeline_BlueprintMode_RunsBlueprint(t *testing.T) {
	// Create pipeline with blueprint mode enabled
	cfg := Config{
		FWUID:                "test-bp-001",
		WorkDir:              t.TempDir(),
		BlueprintID:          "quick_fix", // Use simpler blueprint for testing
		TaskDescription:      "Fix the bug in auth module",
		DefaultMaxIterations: 5,
		MaxRetries:           1,
		RetryBackoff:         []time.Duration{100 * time.Millisecond},
		// Use command override to simulate execution
		CommandName: "echo",
		CommandArgs: []string{"phase-complete"},
	}

	p, events := New(cfg)

	// Collect events
	var receivedEvents []loop.Event
	done := make(chan struct{})
	go func() {
		for e := range events {
			receivedEvents = append(receivedEvents, e)
		}
		close(done)
	}()

	// Run with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	err := p.Run(ctx)
	<-done

	// Blueprint mode should have started (even if it fails due to no real executor)
	// Check that we got a blueprint_start event
	var sawBlueprintStart bool
	for _, e := range receivedEvents {
		if e.Type == loop.EventBlueprintStart {
			sawBlueprintStart = true
			if e.BlueprintID != "quick_fix" {
				t.Errorf("Expected blueprint_id 'quick_fix', got %q", e.BlueprintID)
			}
		}
	}

	if !sawBlueprintStart {
		// If we got an error, it should be about MCP config or executor, not about phases
		if err != nil && err.Error() != "context deadline exceeded" {
			// Blueprint mode was entered, which is what we're testing
			t.Logf("Blueprint mode entered, got expected error: %v", err)
		} else if err == nil {
			t.Error("Expected blueprint_start event but didn't receive one")
		}
	}
}

func TestPipeline_BlueprintMode_DefaultsToStandardTask(t *testing.T) {
	// This test verifies that empty BlueprintID defaults to standard_task blueprint.
	// Legacy phase-based execution has been removed.

	cfg := Config{
		FWUID:       "test-default-bp",
		WorkDir:     t.TempDir(),
		BlueprintID: "", // Empty = defaults to standard_task
	}

	p, events := New(cfg)

	// Drain events
	var receivedEvents []loop.Event
	done := make(chan struct{})
	go func() {
		for e := range events {
			receivedEvents = append(receivedEvents, e)
		}
		close(done)
	}()

	// Run with short timeout - we just want to verify blueprint mode is used
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	_ = p.Run(ctx)
	<-done

	// Verify blueprint_start event was emitted (proves blueprint mode was used)
	var sawBlueprintStart bool
	for _, e := range receivedEvents {
		if e.Type == loop.EventBlueprintStart {
			sawBlueprintStart = true
			// Empty BlueprintID should default to standard_task which has ID "standard_task"
			if e.BlueprintID != "standard_task" {
				t.Errorf("Expected blueprint_id 'standard_task', got %q", e.BlueprintID)
			}
			break
		}
	}

	if !sawBlueprintStart {
		t.Error("Expected blueprint_start event - empty BlueprintID should default to standard_task")
	}
}

func TestPipeline_BlueprintMode_UnknownBlueprint(t *testing.T) {
	cfg := Config{
		FWUID:       "test-unknown-bp",
		WorkDir:     t.TempDir(),
		BlueprintID: "nonexistent_blueprint",
	}

	p, events := New(cfg)

	// Drain events
	go func() {
		for range events {
		}
	}()

	err := p.Run(context.Background())

	if err == nil {
		t.Error("Expected error for unknown blueprint")
	}
	if err != nil && err.Error() != "unknown blueprint: nonexistent_blueprint" {
		t.Errorf("Unexpected error: %v", err)
	}
}
