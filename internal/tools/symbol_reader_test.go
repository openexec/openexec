package tools

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/openexec/openexec/internal/knowledge"
)

func TestSymbolReaderTool(t *testing.T) {
	// Arrange
	tmpDir := t.TempDir()
	os.MkdirAll(filepath.Join(tmpDir, ".openexec"), 0755)
	store, err := knowledge.NewStore(tmpDir)
	if err != nil {
		t.Fatalf("failed to create knowledge store: %v", err)
	}
	defer store.Close()

	tool := NewSymbolReaderTool(store)
	ctx := context.Background()

	t.Run("Execute Success", func(t *testing.T) {
		// Arrange
		store.SetSymbol(&knowledge.SymbolRecord{
			Name:    "MyFunc",
			Kind:    "func",
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

func TestGraphSymbolReaderToolReadsSourceAndRefreshesStalePointer(t *testing.T) {
	tmpDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(tmpDir, ".openexec"), 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(tmpDir, "service.go")
	if err := os.WriteFile(path, []byte("package service\n\nfunc Run() string { return \"ok\" }\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	store, err := knowledge.NewStore(tmpDir)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if _, err := store.ScanRepository(context.Background(), tmpDir); err != nil {
		t.Fatal(err)
	}
	tool := NewSymbolReaderToolForRepository(store, tmpDir)
	result, err := tool.Execute(context.Background(), map[string]interface{}{"name": "Run"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result.(string), "func Run() string") || !strings.Contains(result.(string), "Freshness: current") {
		t.Fatalf("graph source was not returned: %v", result)
	}
	if err := os.WriteFile(path, []byte("package service\n\nfunc Run() string { return \"changed\" }\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	result, err = tool.Execute(context.Background(), map[string]interface{}{"name": "Run"})
	if err != nil {
		t.Fatalf("refresh current symbol source: %v", err)
	}
	if !strings.Contains(result.(string), `return "changed"`) || !strings.Contains(result.(string), "Freshness: current") {
		t.Fatalf("current source was not returned after refresh: %v", result)
	}
}
