package tools

import (
	"context"
)

// Tool defines the standard interface for DCP tools
type Tool interface {
	Name() string
	Description() string
	InputSchema() string // JSON schema for tool arguments

	// Execute performs the tool action. It receives the knowledge store
	// and raw arguments from the LLM.
	Execute(ctx context.Context, args map[string]interface{}) (any, error)
}

// PolicyStore defines the interface for checking if an action is allowed
type PolicyStore interface {
	IsAllowed(ctx context.Context, toolName string, action string) (bool, string)
}
