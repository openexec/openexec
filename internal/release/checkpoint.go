// Package release provides release management with git integration and optional approval workflows.
package release

import (
	"crypto/sha256"
	"encoding/hex"
	"time"
)

// Checkpoint represents a snapshot of run state at a particular stage.
// Checkpoints enable resumable execution by capturing message history,
// tool call logs for idempotency, and accumulated artifacts.
type Checkpoint struct {
	// ID is the unique identifier for this checkpoint.
	ID string `json:"id"`

	// RunID is the ID of the blueprint run this checkpoint belongs to.
	RunID string `json:"run_id"`

	// Stage is the stage name where this checkpoint was created.
	Stage string `json:"stage"`

	// MessageHistory contains the JSON-encoded conversation history.
	// This allows resuming agentic stages with prior context.
	MessageHistory []byte `json:"message_history,omitempty"`

	// ToolCallLog contains idempotency keys for tool calls that have been executed.
	// When resuming, these tool calls can be skipped to avoid duplicate work.
	ToolCallLog []string `json:"tool_call_log,omitempty"`

	// Artifacts contains files or data produced up to this checkpoint.
	Artifacts map[string]string `json:"artifacts,omitempty"`

	// ContextHash is a hash of the context used for this checkpoint.
	// Can be used to detect if context has changed since checkpoint.
	ContextHash string `json:"context_hash,omitempty"`

	// CreatedAt is when the checkpoint was created.
	CreatedAt time.Time `json:"created_at"`
}

// NewCheckpoint creates a new checkpoint with the given parameters.
func NewCheckpoint(id, runID, stage string) *Checkpoint {
	return &Checkpoint{
		ID:          id,
		RunID:       runID,
		Stage:       stage,
		ToolCallLog: make([]string, 0),
		Artifacts:   make(map[string]string),
		CreatedAt:   time.Now().UTC(),
	}
}

// SetMessageHistory sets the message history for the checkpoint.
func (c *Checkpoint) SetMessageHistory(history []byte) {
	c.MessageHistory = history
}

// AddToolCall records a tool call idempotency key.
func (c *Checkpoint) AddToolCall(key string) {
	c.ToolCallLog = append(c.ToolCallLog, key)
}

// HasToolCall checks if a tool call has already been executed.
func (c *Checkpoint) HasToolCall(key string) bool {
	for _, k := range c.ToolCallLog {
		if k == key {
			return true
		}
	}
	return false
}

// SetArtifact sets an artifact value.
func (c *Checkpoint) SetArtifact(name, value string) {
	if c.Artifacts == nil {
		c.Artifacts = make(map[string]string)
	}
	c.Artifacts[name] = value
}

// ComputeContextHash computes and sets a hash for the given context data.
func (c *Checkpoint) ComputeContextHash(contextData []byte) {
	hash := sha256.Sum256(contextData)
	c.ContextHash = hex.EncodeToString(hash[:])
}
