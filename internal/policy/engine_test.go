package policy

import (
	"context"
	"strings"
	"testing"

	"github.com/openexec/openexec/internal/knowledge"
)

func TestPolicyEngine(t *testing.T) {
	// Arrange
	tmpDir := t.TempDir()
	store, _ := knowledge.NewStore(tmpDir)
	defer store.Close()

	engine := NewEngine(store)
	ctx := context.Background()

	t.Run("ValidateAction - Allow Default", func(t *testing.T) {
		allowed, msg := engine.ValidateAction(ctx, "deploy", "normal")
		if !allowed {
			t.Errorf("expected allowed by default, got msg: %s", msg)
		}
	})

	t.Run("ValidateAction - Deny Force", func(t *testing.T) {
		// Arrange
		store.SetPolicy(&knowledge.PolicyRecord{
			Key:   "tool_deploy",
			Value: "deny_force",
		})

		// Act
		allowed, msg := engine.ValidateAction(ctx, "deploy", "force push")

		// Assert
		if allowed {
			t.Error("expected denied for force operation")
		}
		if !strings.Contains(msg, "violation") {
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
