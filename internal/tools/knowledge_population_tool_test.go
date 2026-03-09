package tools

import (
	"context"
	"strings"
	"testing"

	"github.com/openexec/openexec/internal/knowledge"
)

func TestKnowledgePopulationTool(t *testing.T) {
	// Arrange
	tmpDir := t.TempDir()
	store, _ := knowledge.NewStore(tmpDir)
	defer store.Close()

	tool := NewKnowledgePopulationTool(store)
	ctx := context.Background()

	t.Run("Populate Environment", func(t *testing.T) {
		// Act
		_, err := tool.Execute(ctx, map[string]interface{}{
			"type":         "environment",
			"env":          "prod",
			"runtime_type": "k8s",
			"auth_steps":   `["gcloud auth login"]`,
			"topology":     `[{"ip": "10.0.0.1", "services": ["frontend", "backend"]}]`,
		})

		// Assert
		if err != nil {
			t.Fatalf("Execute failed: %v", err)
		}

		got, _ := store.GetEnvironment("prod")
		if got.RuntimeType != "k8s" {
			t.Errorf("got runtime type %q, want k8s", got.RuntimeType)
		}
		if !strings.Contains(got.Topology, "10.0.0.1") {
			t.Errorf("missing IP in topology: %s", got.Topology)
		}
	})

	t.Run("Populate API Doc", func(t *testing.T) {
		// Act
		_, err := tool.Execute(ctx, map[string]interface{}{
			"type":        "api_doc",
			"path":        "/api/v1/user",
			"method":      "POST",
			"description": "Create user",
		})

		// Assert
		if err != nil {
			t.Fatalf("Execute failed: %v", err)
		}

		got, _ := store.GetAPIDoc("/api/v1/user", "POST")
		if got.Description != "Create user" {
			t.Errorf("got description %q", got.Description)
		}
	})
}
