// Package mcp provides MCP (Model Context Protocol) server functionality.
// This file implements idempotency checking backed by the unified state store.
package mcp

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/openexec/openexec/pkg/db/state"
)

// StoreIdempotencyChecker implements IdempotencyChecker using the state.Store.
type StoreIdempotencyChecker struct {
	store     *state.Store
	sessionID string
	messageID string
}

// NewStoreIdempotencyChecker creates a new checker backed by the state store.
func NewStoreIdempotencyChecker(store *state.Store, sessionID, messageID string) *StoreIdempotencyChecker {
	return &StoreIdempotencyChecker{
		store:     store,
		sessionID: sessionID,
		messageID: messageID,
	}
}

// WasApplied checks if a tool call with this idempotency key was already completed.
func (c *StoreIdempotencyChecker) WasApplied(key string) (bool, error) {
	if c.store == nil || key == "" {
		return false, nil
	}
	return c.store.CheckIdempotencyKey(context.Background(), key)
}

// MarkApplied records that a tool call with this key completed successfully.
func (c *StoreIdempotencyChecker) MarkApplied(key string, toolName string, output string) error {
	if c.store == nil || key == "" {
		return nil
	}

	// Generate a unique ID for this tool call
	id := uuid.New().String()

	// Use the context-appropriate session/message IDs
	sessionID := c.sessionID
	if sessionID == "" {
		sessionID = "system"
	}
	messageID := c.messageID
	if messageID == "" {
		messageID = "msg-" + time.Now().UTC().Format("20060102-150405")
	}

	// Record the tool call
	err := c.store.RecordToolCall(context.Background(), id, messageID, sessionID, toolName, "", key)
	if err != nil {
		return err
	}

	// Mark it as completed
	return c.store.UpdateToolCallStatus(context.Background(), id, "completed", output, "")
}

// InMemoryIdempotencyChecker is a simple in-memory implementation for testing.
type InMemoryIdempotencyChecker struct {
	applied map[string]bool
}

// NewInMemoryIdempotencyChecker creates a new in-memory checker.
func NewInMemoryIdempotencyChecker() *InMemoryIdempotencyChecker {
	return &InMemoryIdempotencyChecker{
		applied: make(map[string]bool),
	}
}

// WasApplied checks if a key was previously marked as applied.
func (c *InMemoryIdempotencyChecker) WasApplied(key string) (bool, error) {
	return c.applied[key], nil
}

// MarkApplied records that a key was successfully applied.
func (c *InMemoryIdempotencyChecker) MarkApplied(key string, toolName string, output string) error {
	c.applied[key] = true
	return nil
}
