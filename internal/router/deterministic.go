package router

import (
	"context"
	"fmt"
	"strings"

	"github.com/openexec/openexec/internal/toolset"
)

// DeterministicRouter implements the Router interface using pure keyword matching.
// No model inference is performed — intent is determined entirely from keyword
// patterns extracted from the coordinator's classifyMode and BitNet's simulateInference.
type DeterministicRouter struct {
	tools           map[string]string // tool name -> description/schema
	toolsetRegistry *toolset.Registry
	toolsetSelector *toolset.Selector
}

// NewDeterministicRouter creates a new DeterministicRouter with default toolset registry.
func NewDeterministicRouter() *DeterministicRouter {
	registry := toolset.NewRegistry()
	return &DeterministicRouter{
		tools:           make(map[string]string),
		toolsetRegistry: registry,
		toolsetSelector: toolset.NewSelector(registry),
	}
}

// NewDeterministicRouterWithRegistry creates a DeterministicRouter with a custom toolset registry.
func NewDeterministicRouterWithRegistry(registry *toolset.Registry) *DeterministicRouter {
	return &DeterministicRouter{
		tools:           make(map[string]string),
		toolsetRegistry: registry,
		toolsetSelector: toolset.NewSelector(registry),
	}
}

// RegisterTool makes the router aware of a new tool and its purpose.
func (r *DeterministicRouter) RegisterTool(name, description, schema string) error {
	r.tools[name] = fmt.Sprintf("Description: %s, Schema: %s", description, schema)
	return nil
}

// deterministicToolMatch defines a keyword-to-tool mapping for deterministic routing.
type deterministicToolMatch struct {
	keywords   []string
	toolName   string
	confidence float64
}

// deterministicToolMatches defines the keyword matching rules.
// Order matters: first match wins. Patterns are sourced from
// coordinator.classifyMode and bitnet.simulatedToolMatches.
var deterministicToolMatches = []deterministicToolMatch{
	// From BitNet simulatedToolMatches
	{[]string{"symbol", "function"}, "read_symbol", 0.7},
	{[]string{"deploy", "prod"}, "deploy", 0.7},
	{[]string{"commit", "save", "push"}, "safe_commit", 0.7},

	// Run-mode actions (from coordinator classifyMode runKeywords)
	{[]string{"implement", "refactor", "migrate", "convert"}, "run_shell_command", 0.7},
	{[]string{"fix all", "update all", "rewrite"}, "run_shell_command", 0.7},
	{[]string{"execute pipeline"}, "run_shell_command", 0.7},

	// File operations
	{[]string{"read file", "show file", "cat "}, "read_file", 0.7},
	{[]string{"write file", "create file", "write to"}, "write_file", 0.7},
	{[]string{"apply patch", "patch"}, "git_apply_patch", 0.7},

	// Task-mode actions (from coordinator classifyMode taskKeywords)
	{[]string{"add", "remove", "delete", "create", "update"}, "write_file", 0.7},
	{[]string{"fix", "change", "rename", "move", "copy"}, "write_file", 0.7},
	{[]string{"edit", "modify", "set", "configure"}, "write_file", 0.7},

	// Search / read operations
	{[]string{"search", "find", "grep", "look for"}, "grep", 0.7},
	{[]string{"list", "ls", "directory"}, "list_directory", 0.7},
	{[]string{"git status"}, "git_status", 0.7},
	{[]string{"git diff", "diff"}, "git_diff", 0.7},
	{[]string{"git log", "log", "history"}, "git_log", 0.7},

	// Chat-mode indicators (from coordinator classifyMode chatKeywords)
	{[]string{"what is", "how does", "explain", "why"}, GeneralChatTool, 0.7},
	{[]string{"describe", "tell me about", "what are"}, GeneralChatTool, 0.7},
	{[]string{"can you explain", "understand", "help"}, GeneralChatTool, 0.7},
}

// ParseIntent takes natural language and returns a deterministic tool call
// based on keyword matching. No model inference is performed.
func (r *DeterministicRouter) ParseIntent(ctx context.Context, query string) (*Intent, error) {
	lower := strings.ToLower(query)

	// Check each tool match rule in order
	for _, match := range deterministicToolMatches {
		for _, kw := range match.keywords {
			if strings.Contains(lower, kw) {
				return &Intent{
					ToolName:   match.toolName,
					Args:       map[string]interface{}{"query": query},
					Confidence: match.confidence,
				}, nil
			}
		}
	}

	// Fallback: general_chat with lower confidence
	return &Intent{
		ToolName:   GeneralChatTool,
		Args:       map[string]interface{}{"query": query},
		Confidence: FallbackConfidence,
	}, nil
}
