// Package knowledge provides polyglot source code indexing.
// This file defines the provider interface for multi-language support.
package knowledge

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// SymbolKind represents the type of a code symbol.
type SymbolKind string

const (
	KindFunction  SymbolKind = "function"
	KindMethod    SymbolKind = "method"
	KindClass     SymbolKind = "class"
	KindStruct    SymbolKind = "struct"
	KindInterface SymbolKind = "interface"
	KindVariable  SymbolKind = "variable"
	KindConstant  SymbolKind = "constant"
	KindType      SymbolKind = "type"
	KindModule    SymbolKind = "module"
	KindPackage   SymbolKind = "package"
)

// Symbol represents a normalized code symbol across all languages.
type Symbol struct {
	Name       string            `json:"name"`
	Kind       SymbolKind        `json:"kind"`
	Language   string            `json:"language"`
	FilePath   string            `json:"file_path"`
	StartLine  int               `json:"start_line"`
	EndLine    int               `json:"end_line"`
	Signature  string            `json:"signature"`
	DocComment string            `json:"doc_comment,omitempty"`
	Parent     string            `json:"parent,omitempty"`     // For methods: class/struct name
	Visibility string            `json:"visibility,omitempty"` // public, private, etc.
	Metadata   map[string]string `json:"metadata,omitempty"`   // Language-specific metadata
}

// LanguageProvider defines the interface for language-specific parsing.
type LanguageProvider interface {
	// Name returns the provider name (e.g., "go", "typescript", "python").
	Name() string

	// Extensions returns file extensions this provider handles (e.g., [".go"]).
	Extensions() []string

	// CanHandle returns true if this provider can parse the given file.
	CanHandle(path string) bool

	// ExtractSymbols parses a file and returns all symbols found.
	ExtractSymbols(path string) ([]*Symbol, error)
}

// ProviderRegistry manages language providers.
type ProviderRegistry struct {
	providers       []LanguageProvider
	extensionMap    map[string]LanguageProvider
	enabledLanguages map[string]bool
}

// NewProviderRegistry creates a new registry with default providers.
func NewProviderRegistry() *ProviderRegistry {
	registry := &ProviderRegistry{
		providers:       make([]LanguageProvider, 0),
		extensionMap:    make(map[string]LanguageProvider),
		enabledLanguages: make(map[string]bool),
	}

	// Register built-in providers
	registry.Register(&GoProvider{})
	registry.Register(&TypeScriptProvider{})
	registry.Register(&PythonProvider{})

	return registry
}

// Register adds a language provider to the registry.
func (r *ProviderRegistry) Register(provider LanguageProvider) {
	r.providers = append(r.providers, provider)
	for _, ext := range provider.Extensions() {
		r.extensionMap[ext] = provider
	}
	r.enabledLanguages[provider.Name()] = true
}

// GetProvider returns the appropriate provider for a file.
func (r *ProviderRegistry) GetProvider(path string) LanguageProvider {
	ext := filepath.Ext(path)
	if provider, ok := r.extensionMap[ext]; ok {
		if r.enabledLanguages[provider.Name()] {
			return provider
		}
	}
	return nil
}

// EnableLanguage enables a specific language provider.
func (r *ProviderRegistry) EnableLanguage(name string) {
	r.enabledLanguages[name] = true
}

// DisableLanguage disables a specific language provider.
func (r *ProviderRegistry) DisableLanguage(name string) {
	r.enabledLanguages[name] = false
}

// SupportedLanguages returns the list of supported languages.
func (r *ProviderRegistry) SupportedLanguages() []string {
	var languages []string
	for _, p := range r.providers {
		languages = append(languages, p.Name())
	}
	return languages
}

// =============================================================================
// Go Provider
// =============================================================================

// GoProvider implements LanguageProvider for Go source files.
type GoProvider struct{}

func (p *GoProvider) Name() string {
	return "go"
}

func (p *GoProvider) Extensions() []string {
	return []string{".go"}
}

func (p *GoProvider) CanHandle(path string) bool {
	return strings.HasSuffix(path, ".go")
}

func (p *GoProvider) ExtractSymbols(path string) ([]*Symbol, error) {
	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
	if err != nil {
		return nil, fmt.Errorf("failed to parse Go file: %w", err)
	}

	var symbols []*Symbol

	// Extract package-level declarations
	for _, decl := range node.Decls {
		switch d := decl.(type) {
		case *ast.FuncDecl:
			sym := &Symbol{
				Name:       d.Name.Name,
				Kind:       KindFunction,
				Language:   "go",
				FilePath:   path,
				StartLine:  fset.Position(d.Pos()).Line,
				EndLine:    fset.Position(d.End()).Line,
				DocComment: extractGoComment(d.Doc),
				Visibility: goVisibility(d.Name.Name),
			}

			// Check if it's a method
			if d.Recv != nil && len(d.Recv.List) > 0 {
				sym.Kind = KindMethod
				sym.Parent = extractReceiverType(d.Recv)
			}

			sym.Signature = formatGoSignature(d)
			symbols = append(symbols, sym)

		case *ast.GenDecl:
			for _, spec := range d.Specs {
				switch s := spec.(type) {
				case *ast.TypeSpec:
					sym := &Symbol{
						Name:       s.Name.Name,
						Language:   "go",
						FilePath:   path,
						StartLine:  fset.Position(s.Pos()).Line,
						EndLine:    fset.Position(s.End()).Line,
						DocComment: extractGoComment(d.Doc),
						Visibility: goVisibility(s.Name.Name),
					}

					switch s.Type.(type) {
					case *ast.StructType:
						sym.Kind = KindStruct
						sym.Signature = fmt.Sprintf("type %s struct { ... }", s.Name.Name)
					case *ast.InterfaceType:
						sym.Kind = KindInterface
						sym.Signature = fmt.Sprintf("type %s interface { ... }", s.Name.Name)
					default:
						sym.Kind = KindType
						sym.Signature = fmt.Sprintf("type %s = ...", s.Name.Name)
					}
					symbols = append(symbols, sym)

				case *ast.ValueSpec:
					for _, name := range s.Names {
						sym := &Symbol{
							Name:       name.Name,
							Language:   "go",
							FilePath:   path,
							StartLine:  fset.Position(s.Pos()).Line,
							EndLine:    fset.Position(s.End()).Line,
							DocComment: extractGoComment(d.Doc),
							Visibility: goVisibility(name.Name),
						}
						if d.Tok.String() == "const" {
							sym.Kind = KindConstant
						} else {
							sym.Kind = KindVariable
						}
						symbols = append(symbols, sym)
					}
				}
			}
		}
	}

	return symbols, nil
}

func extractGoComment(doc *ast.CommentGroup) string {
	if doc == nil {
		return ""
	}
	return strings.TrimSpace(doc.Text())
}

func goVisibility(name string) string {
	if len(name) == 0 {
		return "private"
	}
	if name[0] >= 'A' && name[0] <= 'Z' {
		return "public"
	}
	return "private"
}

func extractReceiverType(recv *ast.FieldList) string {
	if recv == nil || len(recv.List) == 0 {
		return ""
	}
	switch t := recv.List[0].Type.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.StarExpr:
		if ident, ok := t.X.(*ast.Ident); ok {
			return "*" + ident.Name
		}
	}
	return ""
}

func formatGoSignature(fn *ast.FuncDecl) string {
	var b strings.Builder
	b.WriteString("func ")

	// Add receiver
	if fn.Recv != nil && len(fn.Recv.List) > 0 {
		b.WriteString("(")
		b.WriteString(extractReceiverType(fn.Recv))
		b.WriteString(") ")
	}

	b.WriteString(fn.Name.Name)
	b.WriteString("(")

	// Add params (simplified)
	if fn.Type.Params != nil {
		var params []string
		for range fn.Type.Params.List {
			params = append(params, "...")
		}
		b.WriteString(strings.Join(params, ", "))
	}
	b.WriteString(")")

	// Add return type indicator
	if fn.Type.Results != nil && len(fn.Type.Results.List) > 0 {
		b.WriteString(" (...)")
	}

	return b.String()
}

// =============================================================================
// TypeScript Provider (Pattern-based, no Tree-sitter dependency)
// =============================================================================

// TypeScriptProvider implements LanguageProvider for TypeScript/JavaScript.
type TypeScriptProvider struct{}

func (p *TypeScriptProvider) Name() string {
	return "typescript"
}

func (p *TypeScriptProvider) Extensions() []string {
	return []string{".ts", ".tsx", ".js", ".jsx"}
}

func (p *TypeScriptProvider) CanHandle(path string) bool {
	ext := filepath.Ext(path)
	for _, e := range p.Extensions() {
		if ext == e {
			return true
		}
	}
	return false
}

func (p *TypeScriptProvider) ExtractSymbols(path string) ([]*Symbol, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	lines := strings.Split(string(content), "\n")
	var symbols []*Symbol

	// Regex patterns for TypeScript/JavaScript
	patterns := map[SymbolKind]*regexp.Regexp{
		KindFunction:  regexp.MustCompile(`(?:export\s+)?(?:async\s+)?function\s+(\w+)\s*\(`),
		KindClass:     regexp.MustCompile(`(?:export\s+)?(?:abstract\s+)?class\s+(\w+)`),
		KindInterface: regexp.MustCompile(`(?:export\s+)?interface\s+(\w+)`),
		KindType:      regexp.MustCompile(`(?:export\s+)?type\s+(\w+)\s*=`),
		KindConstant:  regexp.MustCompile(`(?:export\s+)?const\s+(\w+)\s*[=:]`),
		KindVariable:  regexp.MustCompile(`(?:export\s+)?(?:let|var)\s+(\w+)\s*[=:]`),
	}

	methodPattern := regexp.MustCompile(`^\s*(?:async\s+)?(\w+)\s*\([^)]*\)\s*[:{]`)
	arrowFnPattern := regexp.MustCompile(`(?:export\s+)?const\s+(\w+)\s*=\s*(?:async\s+)?\([^)]*\)\s*=>`)

	var docComment string
	var currentClass string

	for lineNum, line := range lines {
		// Capture doc comments
		if strings.Contains(line, "/**") {
			docComment = extractJSDocComment(lines, lineNum)
		}

		// Check for class/interface end
		if currentClass != "" && strings.TrimSpace(line) == "}" {
			currentClass = ""
		}

		for kind, pattern := range patterns {
			if matches := pattern.FindStringSubmatch(line); len(matches) > 1 {
				sym := &Symbol{
					Name:       matches[1],
					Kind:       kind,
					Language:   "typescript",
					FilePath:   path,
					StartLine:  lineNum + 1,
					EndLine:    lineNum + 1,
					DocComment: docComment,
					Visibility: tsVisibility(line),
					Signature:  strings.TrimSpace(line),
				}
				if kind == KindClass {
					currentClass = matches[1]
				}
				symbols = append(symbols, sym)
				docComment = ""
			}
		}

		// Check for arrow functions
		if matches := arrowFnPattern.FindStringSubmatch(line); len(matches) > 1 {
			symbols = append(symbols, &Symbol{
				Name:       matches[1],
				Kind:       KindFunction,
				Language:   "typescript",
				FilePath:   path,
				StartLine:  lineNum + 1,
				EndLine:    lineNum + 1,
				DocComment: docComment,
				Visibility: tsVisibility(line),
				Signature:  strings.TrimSpace(line),
			})
			docComment = ""
		}

		// Check for methods inside classes
		if currentClass != "" && !strings.Contains(line, "class ") {
			if matches := methodPattern.FindStringSubmatch(line); len(matches) > 1 {
				// Skip common non-method patterns
				name := matches[1]
				if name != "if" && name != "for" && name != "while" && name != "switch" {
					symbols = append(symbols, &Symbol{
						Name:       name,
						Kind:       KindMethod,
						Language:   "typescript",
						FilePath:   path,
						StartLine:  lineNum + 1,
						EndLine:    lineNum + 1,
						Parent:     currentClass,
						DocComment: docComment,
						Visibility: tsVisibility(line),
						Signature:  strings.TrimSpace(line),
					})
					docComment = ""
				}
			}
		}
	}

	return symbols, nil
}

func extractJSDocComment(lines []string, startLine int) string {
	var comment strings.Builder
	for i := startLine; i < len(lines); i++ {
		line := strings.TrimSpace(lines[i])
		if strings.HasPrefix(line, "/**") {
			line = strings.TrimPrefix(line, "/**")
		}
		if strings.HasSuffix(line, "*/") {
			line = strings.TrimSuffix(line, "*/")
			comment.WriteString(strings.TrimSpace(line))
			break
		}
		line = strings.TrimPrefix(line, "*")
		comment.WriteString(strings.TrimSpace(line))
		comment.WriteString(" ")
	}
	return strings.TrimSpace(comment.String())
}

func tsVisibility(line string) string {
	if strings.Contains(line, "private ") {
		return "private"
	}
	if strings.Contains(line, "protected ") {
		return "protected"
	}
	if strings.Contains(line, "export ") {
		return "public"
	}
	return "public" // TS default
}

// =============================================================================
// Python Provider (Pattern-based)
// =============================================================================

// PythonProvider implements LanguageProvider for Python.
type PythonProvider struct{}

func (p *PythonProvider) Name() string {
	return "python"
}

func (p *PythonProvider) Extensions() []string {
	return []string{".py", ".pyi"}
}

func (p *PythonProvider) CanHandle(path string) bool {
	ext := filepath.Ext(path)
	return ext == ".py" || ext == ".pyi"
}

func (p *PythonProvider) ExtractSymbols(path string) ([]*Symbol, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	lines := strings.Split(string(content), "\n")
	var symbols []*Symbol

	funcPattern := regexp.MustCompile(`^(\s*)(?:async\s+)?def\s+(\w+)\s*\(`)
	classPattern := regexp.MustCompile(`^(\s*)class\s+(\w+)`)
	varPattern := regexp.MustCompile(`^(\w+)\s*[:]?\s*=`)

	var currentClass string
	var currentIndent int
	var docstring string

	for lineNum, line := range lines {
		// Track class scope by indentation
		if currentClass != "" {
			indent := len(line) - len(strings.TrimLeft(line, " \t"))
			if indent <= currentIndent && strings.TrimSpace(line) != "" {
				currentClass = ""
				currentIndent = 0
			}
		}

		// Check for docstrings
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, `"""`) || strings.HasPrefix(trimmed, `'''`) {
			docstring = extractPythonDocstring(lines, lineNum)
		}

		// Class definition
		if matches := classPattern.FindStringSubmatch(line); len(matches) > 2 {
			currentClass = matches[2]
			currentIndent = len(matches[1])
			symbols = append(symbols, &Symbol{
				Name:       matches[2],
				Kind:       KindClass,
				Language:   "python",
				FilePath:   path,
				StartLine:  lineNum + 1,
				EndLine:    lineNum + 1,
				DocComment: docstring,
				Visibility: pyVisibility(matches[2]),
				Signature:  strings.TrimSpace(line),
			})
			docstring = ""
		}

		// Function/method definition
		if matches := funcPattern.FindStringSubmatch(line); len(matches) > 2 {
			sym := &Symbol{
				Name:       matches[2],
				Kind:       KindFunction,
				Language:   "python",
				FilePath:   path,
				StartLine:  lineNum + 1,
				EndLine:    lineNum + 1,
				DocComment: docstring,
				Visibility: pyVisibility(matches[2]),
				Signature:  strings.TrimSpace(line),
			}

			if currentClass != "" {
				sym.Kind = KindMethod
				sym.Parent = currentClass
			}

			symbols = append(symbols, sym)
			docstring = ""
		}

		// Module-level variables (simple heuristic)
		if currentClass == "" && !strings.HasPrefix(line, " ") && !strings.HasPrefix(line, "\t") {
			if matches := varPattern.FindStringSubmatch(line); len(matches) > 1 {
				name := matches[1]
				if !strings.HasPrefix(name, "#") && name != "def" && name != "class" && name != "import" && name != "from" {
					kind := KindVariable
					if name == strings.ToUpper(name) {
						kind = KindConstant
					}
					symbols = append(symbols, &Symbol{
						Name:       name,
						Kind:       kind,
						Language:   "python",
						FilePath:   path,
						StartLine:  lineNum + 1,
						EndLine:    lineNum + 1,
						Visibility: pyVisibility(name),
						Signature:  strings.TrimSpace(line),
					})
				}
			}
		}
	}

	return symbols, nil
}

func extractPythonDocstring(lines []string, startLine int) string {
	var docstring strings.Builder
	quote := `"""`
	if strings.Contains(lines[startLine], `'''`) {
		quote = `'''`
	}

	for i := startLine; i < len(lines); i++ {
		line := lines[i]
		if i == startLine {
			line = strings.TrimPrefix(strings.TrimSpace(line), quote)
		}
		if strings.Contains(line, quote) && i != startLine {
			line = strings.TrimSuffix(strings.TrimSpace(line), quote)
			docstring.WriteString(strings.TrimSpace(line))
			break
		}
		docstring.WriteString(strings.TrimSpace(line))
		docstring.WriteString(" ")
	}
	return strings.TrimSpace(docstring.String())
}

func pyVisibility(name string) string {
	if strings.HasPrefix(name, "__") && !strings.HasSuffix(name, "__") {
		return "private"
	}
	if strings.HasPrefix(name, "_") {
		return "protected"
	}
	return "public"
}
