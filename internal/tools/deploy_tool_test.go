package tools

import (
	"context"
	"strings"
	"testing"

	"github.com/openexec/openexec/internal/knowledge"
)

func TestDeployTool(t *testing.T) {
	// Arrange
	tmpDir := t.TempDir()
	store, _ := knowledge.NewStore(tmpDir)
	defer store.Close()

	tool := NewDeployTool(store)
	ctx := context.Background()

	t.Run("Execute with Records", func(t *testing.T) {
		// Arrange
		store.SetEnvironment(&knowledge.EnvironmentRecord{
			Env:         "prod",
			RuntimeType: "k8s",
			AuthSteps:   `["gcloud auth login"]`,
			Topology:    `[{"ip": "10.0.0.5", "services": ["backend"]}]`,
		})

		// Act
		res, err := tool.Execute(ctx, map[string]interface{}{"env": "prod", "action": "push"})

		// Assert
		if err != nil {
			t.Fatalf("Execute failed: %v", err)
		}
		out := res.(string)
		if !strings.Contains(out, "k8s runtime") || !strings.Contains(out, "gcloud auth login") {
			t.Errorf("unexpected output: %s", out)
		}
	})

	t.Run("Missing Records", func(t *testing.T) {
		// Act
		res, err := tool.Execute(ctx, map[string]interface{}{"env": "dev"})

		// Assert
		if err != nil {
			t.Fatalf("did not expect error, got: %v", err)
		}
		if !strings.Contains(res.(string), "KNOWLEDGE_MISSING") {
			t.Errorf("expected KNOWLEDGE_MISSING prompt, got %v", res)
		}
	})
}
