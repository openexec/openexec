package knowledge

import (
	"testing"
)

func TestKnowledgeStore_Specialized(t *testing.T) {
	// Arrange
	tmpDir := t.TempDir()
	store, _ := NewStore(tmpDir)
	defer store.Close()

	t.Run("Symbol CRUD", func(t *testing.T) {
		// Arrange
		s := &SymbolRecord{
			Name: "Execute",
			Kind: "func",
			Purpose: "Main entry point",
			FilePath: "main.go",
		}

		// Act
		store.SetSymbol(s)
		got, _ := store.GetSymbol("Execute")

		// Assert
		if got.Purpose != s.Purpose {
			t.Errorf("got %q, want %q", got.Purpose, s.Purpose)
		}
	})

	t.Run("Environment CRUD", func(t *testing.T) {
		// Arrange
		d := &EnvironmentRecord{
			Env:         "prod",
			RuntimeType: "k8s",
			AuthSteps:   `["gcloud auth login"]`,
		}

		// Act
		store.SetEnvironment(d)
		got, _ := store.GetEnvironment("prod")

		// Assert
		if got.RuntimeType != d.RuntimeType {
			t.Errorf("got %q, want %q", got.RuntimeType, d.RuntimeType)
		}
	})

	t.Run("APIDoc CRUD", func(t *testing.T) {
		// Arrange
		a := &APIDocRecord{
			Path: "/health",
			Method: "GET",
			Description: "Health check",
		}

		// Act
		store.SetAPIDoc(a)
		got, _ := store.GetAPIDoc("/health", "GET")

		// Assert
		if got.Description != a.Description {
			t.Errorf("got %q, want %q", got.Description, a.Description)
		}
	})
}
