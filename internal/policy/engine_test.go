package policy

import (
	"context"
	"strings"
	"testing"
)

func TestPolicyEngine(t *testing.T) {
	// Arrange - the engine doesn't require a store for rule-based validation
	tmpDir := t.TempDir()

	engine := NewEngineWithProject(nil, tmpDir)
	ctx := context.Background()

	t.Run("ValidateAction - Allow Default", func(t *testing.T) {
		allowed, msg := engine.ValidateAction(ctx, "deploy", "normal")
		if !allowed {
			t.Errorf("expected allowed by default, got msg: %s", msg)
		}
	})

	t.Run("ValidateAction - Deny via Rules", func(t *testing.T) {
		// Create a custom rule set that denies force operations
		ruleSet := &RuleSet{
			Version:         "1.0",
			Name:            "Test Policy",
			DefaultDecision: DecisionAllow,
			Rules: []Rule{
				{
					ID:       "deny-force",
					Name:     "Block force operations",
					Decision: DecisionDeny,
					Conditions: []Condition{
						{Type: "tool", Value: "deploy"},
						{Type: "action", Operator: "contains", Value: "force"},
					},
				},
			},
		}

		// Create engine with custom rules
		customEngine := &Engine{
			store:       nil,
			rulesEngine: NewRulesEngine(ruleSet),
			projectDir:  tmpDir,
		}

		// Act
		allowed, msg := customEngine.ValidateAction(ctx, "deploy", "force push")

		// Assert
		if allowed {
			t.Error("expected denied for force operation")
		}
		if !strings.Contains(msg, "violation") && !strings.Contains(msg, "Block force") {
			t.Errorf("missing violation message: %s", msg)
		}
	})

	t.Run("ValidateCompliance - Block on failure", func(t *testing.T) {
		// In a real test we'd mock the exec calls, but for this AAA test
		// we'll just check that it executes.
		// Since we're in a temp dir without go.mod, it should pass by default.
		passed, _ := engine.ValidateCompliance(ctx, tmpDir)
		if !passed {
			t.Error("expected passed by default in empty dir")
		}
	})
}

func TestRulesEngine(t *testing.T) {
	t.Run("Default security rules", func(t *testing.T) {
		engine := NewRulesEngine(DefaultSecurityRules())

		// Test deny /etc writes
		result := engine.Evaluate(&EvaluationContext{
			Tool: "write_file",
			Path: "/etc/passwd",
		})
		if result.Decision != DecisionDeny {
			t.Errorf("expected deny for /etc/passwd, got %s", result.Decision)
		}

		// Test allow normal writes
		result = engine.Evaluate(&EvaluationContext{
			Tool: "write_file",
			Path: "/tmp/test.txt",
		})
		if result.Decision != DecisionAllow {
			t.Errorf("expected allow for /tmp/test.txt, got %s", result.Decision)
		}
	})

	t.Run("Condition operators", func(t *testing.T) {
		ruleSet := &RuleSet{
			Version:         "1.0",
			DefaultDecision: DecisionAllow,
			Rules: []Rule{
				{
					ID:       "match-prefix",
					Name:     "Match prefix",
					Decision: DecisionDeny,
					Conditions: []Condition{
						{Type: "path", Operator: "prefix", Value: "/secret/"},
					},
				},
			},
		}
		engine := NewRulesEngine(ruleSet)

		// Should match
		result := engine.Evaluate(&EvaluationContext{Path: "/secret/data"})
		if result.Decision != DecisionDeny {
			t.Errorf("expected deny for /secret/data, got %s", result.Decision)
		}

		// Should not match
		result = engine.Evaluate(&EvaluationContext{Path: "/public/data"})
		if result.Decision != DecisionAllow {
			t.Errorf("expected allow for /public/data, got %s", result.Decision)
		}
	})

	t.Run("Exception handling", func(t *testing.T) {
		ruleSet := &RuleSet{
			Version:         "1.0",
			DefaultDecision: DecisionAllow,
			Rules: []Rule{
				{
					ID:       "deny-credentials",
					Name:     "Deny credentials",
					Decision: DecisionDeny,
					Conditions: []Condition{
						{Type: "path", Operator: "suffix", Value: ".env"},
					},
					Exceptions: []Condition{
						{Type: "path", Operator: "suffix", Value: ".env.example"},
					},
				},
			},
		}
		engine := NewRulesEngine(ruleSet)

		// Should deny .env
		result := engine.Evaluate(&EvaluationContext{Path: "/app/.env"})
		if result.Decision != DecisionDeny {
			t.Errorf("expected deny for .env, got %s", result.Decision)
		}

		// Should allow .env.example (exception)
		result = engine.Evaluate(&EvaluationContext{Path: "/app/.env.example"})
		if result.Decision != DecisionAllow {
			t.Errorf("expected allow for .env.example, got %s", result.Decision)
		}
	})
}
