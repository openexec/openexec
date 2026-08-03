package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/openexec/openexec/internal/knowledge"
	"github.com/openexec/openexec/internal/repository"
)

// SymbolReaderTool implements the surgical code reading via pointers
type SymbolReaderTool struct {
	store          *knowledge.Store
	repositoryRoot string
}

// NewSymbolReaderToolForRepository enables graph-backed, hash-validated source
// reads for a specific authorized repository root.
func NewSymbolReaderToolForRepository(store *knowledge.Store, repositoryRoot string) *SymbolReaderTool {
	return &SymbolReaderTool{store: store, repositoryRoot: repositoryRoot}
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
			"symbol_id": {
				"type": "string",
				"description": "Opaque symbol ID returned by a prior resolution"
			},
			"name": {
				"type": "string",
				"description": "The name of the symbol to read"
			},
			"file": {
				"type": "string",
				"description": "Optional repository-relative file used to disambiguate"
			},
			"kind": {
				"type": "string",
				"description": "Optional symbol kind used to disambiguate"
			}
		}
	}`
}

func (t *SymbolReaderTool) Execute(ctx context.Context, args map[string]interface{}) (any, error) {
	name, _ := args["name"].(string)
	symbolID, _ := args["symbol_id"].(string)
	if name == "" && symbolID == "" {
		return nil, fmt.Errorf("missing name or symbol_id argument")
	}
	if t.repositoryRoot != "" {
		identity, err := t.store.EnsureRepositoryIdentity(ctx, t.repositoryRoot, "")
		if err != nil {
			return nil, fmt.Errorf("resolve repository identity: %w", err)
		}
		if symbolID == "" {
			file, _ := args["file"].(string)
			kind, _ := args["kind"].(string)
			resolved, err := t.store.ResolveGraphSymbol(ctx, identity, name, file, kind, 20)
			if err != nil {
				return nil, fmt.Errorf("resolve graph symbol: %w", err)
			}
			if resolved.Result.Candidate == nil {
				if len(resolved.Result.Candidates) > 0 {
					var candidates []string
					for _, candidate := range resolved.Result.Candidates {
						candidates = append(candidates, fmt.Sprintf("%s %s at %s:%d [%s]", candidate.Symbol.Kind, candidate.Symbol.QualifiedName, candidate.Occurrence.FilePath, candidate.Occurrence.StartLine, candidate.Symbol.ID))
					}
					return nil, fmt.Errorf("symbol %q is ambiguous; select a symbol_id, file, or kind:\n%s", name, strings.Join(candidates, "\n"))
				}
				return nil, fmt.Errorf("symbol %q not found in current repository graph", name)
			}
			symbolID = resolved.Result.Candidate.Symbol.ID
		}
		reader, err := repository.NewRootedReader(t.repositoryRoot, identity.RepositoryID, identity.WorktreeID, knowledge.DefaultGraphLimits().MaxBytes)
		if err != nil {
			return nil, err
		}
		source, err := t.store.ReadGraphSymbol(ctx, identity, symbolID, reader)
		if err != nil {
			return nil, fmt.Errorf("read current symbol source: %w", err)
		}
		return fmt.Sprintf("Symbol: %s (%s)\nID: %s\nFile: %s\nRange: L%d-L%d\nFreshness: %s\nResolution: %s\n\n%s",
			source.Result.Symbol.DisplayName, source.Result.Symbol.Kind, source.Result.Symbol.ID,
			source.Result.Occurrence.FilePath, source.Result.Occurrence.StartLine,
			source.Result.Occurrence.EndLine, source.Generation.Freshness,
			source.Result.Occurrence.Resolution, source.Result.Source.Content), nil
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
