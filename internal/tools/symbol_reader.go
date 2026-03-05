package tools

import (
	"context"
	"fmt"

	"github.com/openexec/openexec/internal/knowledge"
)

// SymbolReaderTool implements the surgical code reading via pointers
type SymbolReaderTool struct {
	store *knowledge.Store
}

func NewSymbolReaderTool(store *knowledge.Store) *SymbolReaderTool {
	return &SymbolReaderTool{store: store}
}

func (t *SymbolReaderTool) Name() string {
	return "read_symbol"
}

func (t *SymbolReaderTool) Description() string {
	return "Reads the exact implementation of a symbol (function/struct) using a local AST pointer."
}

func (t *SymbolReaderTool) InputSchema() string {
	return `{
		"type": "object",
		"properties": {
			"name": {
				"type": "string",
				"description": "The name of the symbol to read"
			}
		},
		"required": ["name"]
	}`
}

func (t *SymbolReaderTool) Execute(ctx context.Context, args map[string]interface{}) (any, error) {
	name, ok := args["name"].(string)
	if !ok {
		return nil, fmt.Errorf("missing name argument")
	}

	symbol, err := t.store.GetSymbol(name)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch pointer: %w", err)
	}
	if symbol == nil {
		return nil, fmt.Errorf("symbol %q not found in local index", name)
	}

	// Format result for LLM
	result := fmt.Sprintf("Symbol: %s (%s)\nFile: %s\nRange: L%d-L%d\nPurpose: %s\nSignature: %s\n",
		symbol.Name, symbol.Kind, symbol.FilePath, symbol.StartLine, symbol.EndLine, symbol.Purpose, symbol.Signature)

	// In production, we'd read the actual file bytes here.
	return result, nil
}
