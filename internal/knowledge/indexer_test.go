package knowledge

import (
	"os"
	"path/filepath"
	"testing"
)

func TestIndexer(t *testing.T) {
	// Arrange
	tmpDir := t.TempDir()
	store, _ := NewStore(tmpDir)
	defer store.Close()
	
	idx := NewIndexer(store)

	// Create a dummy Go file with comments
	goCode := `
package test
// CalculateSum adds two numbers.
func CalculateSum(a, b int) int {
	return a + b
}
`
	os.WriteFile(filepath.Join(tmpDir, "logic.go"), []byte(goCode), 0644)

	// Act
	err := idx.IndexProject(tmpDir)
	if err != nil {
		t.Fatalf("IndexProject failed: %v", err)
	}

	// Assert
	symbol, err := store.GetSymbol("CalculateSum")
	if err != nil {
		t.Fatal(err)
	}
	if symbol == nil {
		t.Fatal("expected CalculateSum symbol to be indexed")
	}
	if symbol.Purpose != "CalculateSum adds two numbers." {
		t.Errorf("got purpose %q", symbol.Purpose)
	}
}
