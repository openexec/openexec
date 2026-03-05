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

	t.Run("ValidateCodeChange - Deny Secrets", func(t *testing.T) {
		// Arrange
		store.SetPolicy(&knowledge.PolicyRecord{
			Key:   "safety_code",
			Value: "no_secrets",
		})

		// Act
		allowed, _ := engine.ValidateCodeChange(ctx, "main.go", "const API_KEY = 'secret'")

		// Assert
		if allowed {
			t.Error("expected denied for hardcoded secret")
		}
	})
}
