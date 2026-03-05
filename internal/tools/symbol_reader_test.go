package tools

import (
	"context"
	"strings"
	"testing"

	"github.com/openexec/openexec/internal/knowledge"
)

func TestSymbolReaderTool(t *testing.T) {
	// Arrange
	tmpDir := t.TempDir()
	store, _ := knowledge.NewStore(tmpDir)
	defer store.Close()

	tool := NewSymbolReaderTool(store)
	ctx := context.Background()

	t.Run("Execute Success", func(t *testing.T) {
		// Arrange
		store.SetSymbol(&knowledge.SymbolRecord{
			Name: "MyFunc",
			Kind: "func",
			Purpose: "Testing",
		})

		// Act
		res, err := tool.Execute(ctx, map[string]interface{}{"name": "MyFunc"})

		// Assert
		if err != nil {
			t.Fatalf("Execute failed: %v", err)
		}
		if !strings.Contains(res.(string), "MyFunc") || !strings.Contains(res.(string), "func") {
			t.Errorf("unexpected result: %v", res)
		}
	})

	t.Run("Execute Symbol Not Found", func(t *testing.T) {
		// Act
		_, err := tool.Execute(ctx, map[string]interface{}{"name": "Missing"})

		// Assert
		if err == nil || !strings.Contains(err.Error(), "not found") {
			t.Errorf("expected 'not found' error, got %v", err)
		}
	})
}
