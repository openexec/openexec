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
			return idx.indexGoFile(path)
		}
		return nil
	})
}

func (idx *Indexer) indexGoFile(path string) error {
	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
	if err != nil {
		return nil // Skip files that don't parse
	}

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

			// Detect API routes if they look like handlers
			if strings.Contains(symbol.Signature, "http.ResponseWriter") {
				idx.store.SetAPIDoc(&APIDocRecord{
					Path:        "Unknown (check implementation)",
					Method:      "HTTP",
					Description: fmt.Sprintf("Handler for %s", symbol.Name),
				})
			}
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
