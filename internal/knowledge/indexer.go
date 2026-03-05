package knowledge

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
)

// Indexer handles automatic extraction of knowledge from source code
type Indexer struct {
	store *Store
}

// Syncer defines the interface for triggering file re-indexing
type Syncer interface {
	SyncFile(filePath string) error
}

func NewIndexer(store *Store) *Indexer {
	return &Indexer{store: store}
}

// IndexProject scans the directory and populates symbol and API records
func (idx *Indexer) IndexProject(projectDir string) error {
	return filepath.Walk(projectDir, func(path string, info os.FileInfo, err error) error {
		if err != nil { return err }
		
		// Skip hidden directories and node_modules
		if info.IsDir() && (strings.HasPrefix(info.Name(), ".") || info.Name() == "node_modules") {
			return filepath.SkipDir
		}

		// Currently handles Go files. Can be extended with Tree-sitter for polyglot support.
		if !info.IsDir() && strings.HasSuffix(path, ".go") {
			return idx.IndexFile(path)
		}
		return nil
	})
}

// IndexFile atomically updates all symbols for a specific file
func (idx *Indexer) IndexFile(path string) error {
	// 1. Parse File
	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
	if err != nil {
		return fmt.Errorf("failed to parse %s: %w", path, err)
	}

	// 2. Clear old records for this file to handle Line Drift
	if err := idx.store.DeleteSymbolsByFile(path); err != nil {
		return fmt.Errorf("failed to clear old symbols for %s: %w", path, err)
	}

	// 3. Extract and set new symbols
	for _, decl := range node.Decls {
		if fn, ok := decl.(*ast.FuncDecl); ok {
			symbol := &SymbolRecord{
				Name:      fn.Name.Name,
				Kind:      "func",
				FilePath:  path,
				StartLine: fset.Position(fn.Pos()).Line,
				EndLine:   fset.Position(fn.End()).Line,
				Signature: idx.getSignature(fn),
				Purpose:   idx.extractComment(fn.Doc),
			}
			idx.store.SetSymbol(symbol)
		}
	}
	return nil
}

func (idx *Indexer) getSignature(fn *ast.FuncDecl) string {
	// Simplified signature extraction
	return fmt.Sprintf("func %s(...)", fn.Name.Name)
}

func (idx *Indexer) extractComment(doc *ast.CommentGroup) string {
	if doc == nil {
		return "No description provided."
	}
	return strings.TrimSpace(doc.Text())
}
