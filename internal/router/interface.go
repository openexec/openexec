package router

import (
	"context"
)

// Intent represents the parsed goal from the user
type Intent struct {
	ToolName string                 `json:"tool_name"`
	Args     map[string]interface{} `json:"args"`
	Confidence float64              `json:"confidence"`
}

// Router defines the interface for local intent parsing
type Router interface {
	// ParseIntent takes natural language and returns a deterministic tool call
	ParseIntent(ctx context.Context, query string) (*Intent, error)
	
	// RegisterTool makes the router aware of a new tool and its purpose
	RegisterTool(name string, description string, schema string) error
}
