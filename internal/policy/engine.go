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
	store       *knowledge.Store
	rulesEngine *RulesEngine
	projectDir  string
}

// NewEngine creates a new policy engine with the knowledge store.
func NewEngine(store *knowledge.Store) *Engine {
	return NewEngineWithProject(store, ".")
}

// NewEngineWithProject creates a policy engine with project-specific rules.
func NewEngineWithProject(store *knowledge.Store, projectDir string) *Engine {
	// Load rules from project or use defaults
	ruleSet, err := LoadRulesFromProject(projectDir)
	if err != nil {
		// Fall back to defaults on error
		ruleSet = DefaultSecurityRules()
	}

	return &Engine{
		store:       store,
		rulesEngine: NewRulesEngine(ruleSet),
		projectDir:  projectDir,
	}
}

// ValidateAction checks if a tool execution is allowed by policy.
// Uses the rules engine for structured evaluation.
func (e *Engine) ValidateAction(ctx context.Context, toolName string, action string) (bool, string) {
	return e.ValidateActionWithContext(ctx, &EvaluationContext{
		Tool:   toolName,
		Action: action,
		Tier:   os.Getenv("OPENEXEC_MODE"),
	})
}

// ValidateActionWithContext performs full policy evaluation with rich context.
func (e *Engine) ValidateActionWithContext(ctx context.Context, evalCtx *EvaluationContext) (bool, string) {
	result := e.rulesEngine.Evaluate(evalCtx)

	switch result.Decision {
	case DecisionAllow:
		return true, ""
	case DecisionDeny:
		reason := result.Reason
		if result.MatchedRule != nil && result.MatchedRule.Description != "" {
			reason = result.MatchedRule.Description
		}
		return false, fmt.Sprintf("Policy violation: %s", reason)
	case DecisionAsk:
		// For "ask" decisions, we return true but include the reason
		// The caller is responsible for prompting the user
		reason := "User confirmation required"
		if result.MatchedRule != nil && result.MatchedRule.Description != "" {
			reason = result.MatchedRule.Description
		}
		return true, fmt.Sprintf("CONFIRM: %s", reason)
	default:
		return true, ""
	}
}

// ValidateTool validates a tool call with full context.
func (e *Engine) ValidateTool(ctx context.Context, toolName string, path string, args map[string]interface{}, tier string) *EvaluationResult {
	// Build action string from args for pattern matching
	action := toolName
	if cmd, ok := args["command"].(string); ok {
		action = cmd
	}

	evalCtx := &EvaluationContext{
		Tool:   toolName,
		Path:   path,
		Action: action,
		Tier:   tier,
		Args:   args,
		Env:    getEnvMap(),
	}

	return e.rulesEngine.Evaluate(evalCtx)
}

// RequiresConfirmation checks if the action requires user confirmation.
func (e *Engine) RequiresConfirmation(ctx context.Context, toolName string, action string) (bool, string) {
	result := e.rulesEngine.Evaluate(&EvaluationContext{
		Tool:   toolName,
		Action: action,
		Tier:   os.Getenv("OPENEXEC_MODE"),
	})

	if result.Decision == DecisionAsk {
		reason := result.Reason
		if result.MatchedRule != nil {
			reason = result.MatchedRule.Name
		}
		return true, reason
	}
	return false, ""
}

// ReloadRules reloads the rules from the project directory.
func (e *Engine) ReloadRules() error {
	ruleSet, err := LoadRulesFromProject(e.projectDir)
	if err != nil {
		return err
	}
	e.rulesEngine = NewRulesEngine(ruleSet)
	return nil
}

// getEnvMap returns relevant environment variables as a map.
func getEnvMap() map[string]string {
	env := make(map[string]string)
	relevantVars := []string{
		"OPENEXEC_MODE",
		"OPENEXEC_WORKSPACE_ROOT",
		"USER",
		"HOME",
	}
	for _, key := range relevantVars {
		if v := os.Getenv(key); v != "" {
			env[key] = v
		}
	}
	return env
}

// Legacy keyword-based validation (kept for backward compatibility)
func (e *Engine) validateLegacy(ctx context.Context, toolName string, action string) (bool, string) {
	if e.store == nil {
		return true, ""
	}

	policyKey := fmt.Sprintf("tool_%s", toolName)
	record, err := e.store.GetPolicy(policyKey)
	if err != nil {
		return false, fmt.Sprintf("failed to fetch policy: %v", err)
	}

	if record == nil {
		return true, ""
	}

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
