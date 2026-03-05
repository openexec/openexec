package policy

import (
	"context"
	"fmt"
	"strings"

	"github.com/openexec/openexec/internal/knowledge"
)

// Engine enforces deterministic rules stored in the knowledge base.
type Engine struct {
	store *knowledge.Store
}

func NewEngine(store *knowledge.Store) *Engine {
	return &Engine{store: store}
}

// ValidateAction checks if a tool execution is allowed by policy.
func (e *Engine) ValidateAction(ctx context.Context, toolName string, action string) (bool, string) {
	policyKey := fmt.Sprintf("tool_%s", toolName)
	record, err := e.store.GetRecord(knowledge.TypePolicy, policyKey)
	if err != nil {
		return false, fmt.Sprintf("failed to fetch policy: %v", err)
	}

	if record == nil {
		// Default: allow if no specific policy exists
		return true, ""
	}

	// Simple keyword matching for this scaffold.
	// In production, this would use a 1-bit LLM or a rego-like engine.
	if strings.Contains(record.Value, "deny") && strings.Contains(action, "force") {
		return false, "Policy violation: 'force' operations are denied for this tool."
	}

	return true, ""
}

// ValidateCodeChange checks proposed code edits against safety policies.
func (e *Engine) ValidateCodeChange(ctx context.Context, filePath string, content string) (bool, string) {
	// Fetch global safety policy
	record, _ := e.store.GetRecord(knowledge.TypePolicy, "safety_code")
	if record != nil {
		if strings.Contains(record.Value, "no_secrets") {
			// Basic secret detection
			if strings.Contains(content, "API_KEY") || strings.Contains(content, "PASSWORD") {
				return false, "Policy violation: No hardcoded secrets allowed."
			}
		}
	}

	return true, ""
}
