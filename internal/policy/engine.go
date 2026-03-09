package policy

import (
	"context"
	"fmt"
	"os"
	"os/exec"
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
	record, err := e.store.GetPolicy(policyKey)
	if err != nil {
		return false, fmt.Sprintf("failed to fetch policy: %v", err)
	}

	if record == nil {
		// Default: allow if no specific policy exists
		return true, ""
	}

	// Simple keyword matching for this scaffold.
	if strings.Contains(record.Value, "deny") && strings.Contains(action, "force") {
		return false, "Policy violation: 'force' operations are denied for this tool."
	}

	return true, ""
}

// ValidateCompliance runs mandatory quality gates for the project type.
func (e *Engine) ValidateCompliance(ctx context.Context, projectDir string) (bool, string) {
	// 1. Detect project type (Simplified for demo)
	isGo := false
	isPython := false
	if _, err := os.Stat("go.mod"); err == nil {
		isGo = true
	}
	if _, err := os.Stat("pyproject.toml"); err == nil {
		isPython = true
	}

	// 2. Execute mandatory gates
	if isGo {
		cmd := exec.CommandContext(ctx, "go", "vet", "./...")
		if out, err := cmd.CombinedOutput(); err != nil {
			return false, fmt.Sprintf("Compliance Failure (go vet):\n%s", string(out))
		}
	}

	if isPython {
		// Run ruff and mypy
		cmd := exec.CommandContext(ctx, "ruff", "check", ".")
		if out, err := cmd.CombinedOutput(); err != nil {
			return false, fmt.Sprintf("Compliance Failure (ruff):\n%s", string(out))
		}

		// Note: we use python -m mypy to ensure it uses the local environment
		cmd = exec.CommandContext(ctx, "python3", "-m", "mypy", ".")
		if out, err := cmd.CombinedOutput(); err != nil {
			// We only block if it's a hard policy
			return false, fmt.Sprintf("Compliance Failure (mypy):\n%s", string(out))
		}
	}

	return true, ""
}
