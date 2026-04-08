package nostubs

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
)

// GoScanner scans Go source files for stub patterns. It uses go/parser +
// go/ast where the rule maps cleanly to AST shapes, and falls back to regex
// for everything else.
type GoScanner struct {
	cfg Config
}

// NewGoScanner creates a new Go scanner.
func NewGoScanner(cfg Config) *GoScanner {
	return &GoScanner{cfg: cfg}
}

// Language returns "go".
func (s *GoScanner) Language() string { return "go" }

// Scan walks the root directory and returns findings for all .go files.
func (s *GoScanner) Scan(root string) ([]Finding, error) {
	var out []Finding
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if isExcludedDir(d.Name(), s.cfg.ExcludeDirs) {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		findings, ferr := s.scanFile(path)
		if ferr != nil {
			return nil
		}
		out = append(out, findings...)
		return nil
	})
	return out, err
}

func (s *GoScanner) scanFile(path string) ([]Finding, error) {
	data, err := os.ReadFile(path) // #nosec G304
	if err != nil {
		return nil, err
	}
	content := string(data)
	lines := strings.Split(content, "\n")

	var findings []Finding
	test := isTestPath(path)
	if test {
		return nil, nil
	}

	fset := token.NewFileSet()
	file, parseErr := parser.ParseFile(fset, path, data, parser.ParseComments)

	// --- AST-based rules (if parse succeeded) ---
	handler := false
	hasDB := false
	if parseErr == nil && file != nil {
		// Detect DB-ish imports for rule 4.
		for _, imp := range file.Imports {
			if imp.Path != nil && reDBImport.MatchString(strings.ToLower(imp.Path.Value)) {
				hasDB = true
				break
			}
		}

		// Walk funcs to find HTTP handlers + check their bodies.
		ast.Inspect(file, func(n ast.Node) bool {
			fn, ok := n.(*ast.FuncDecl)
			if !ok || fn.Type == nil || fn.Type.Params == nil {
				return true
			}
			if !looksLikeHTTPHandler(fn) {
				return true
			}
			handler = true

			// Rule 9: empty_handler — body is only `return` or single return of nil.
			if fn.Body != nil && ruleActive(s.cfg, RuleEmptyHandler) {
				if isEmptyBody(fn.Body) {
					pos := fset.Position(fn.Body.Lbrace)
					findings = append(findings, Finding{
						File:     path,
						Line:     pos.Line,
						Rule:     RuleEmptyHandler,
						Severity: severityFor(s.cfg, RuleEmptyHandler, SeverityHigh),
						Message:  "handler body is empty or only returns nil",
					})
				}
			}

			// Rule 5: hardcoded literal return inside handler.
			if fn.Body != nil && ruleActive(s.cfg, RuleHandlerHardcoded) {
				ast.Inspect(fn.Body, func(nn ast.Node) bool {
					ret, ok := nn.(*ast.ReturnStmt)
					if !ok {
						return true
					}
					for _, r := range ret.Results {
						if lit, ok := r.(*ast.CompositeLit); ok && hasLiteralFields(lit) {
							pos := fset.Position(ret.Pos())
							findings = append(findings, Finding{
								File:     path,
								Line:     pos.Line,
								Rule:     RuleHandlerHardcoded,
								Severity: severityFor(s.cfg, RuleHandlerHardcoded, SeverityHigh),
								Message:  "handler returns a hardcoded composite literal",
							})
						}
					}
					return true
				})
			}
			return true
		})

		// Rule 1: mock_constants via AST constant/variable names.
		if ruleActive(s.cfg, RuleMockConstants) {
			for _, decl := range file.Decls {
				gd, ok := decl.(*ast.GenDecl)
				if !ok || (gd.Tok != token.CONST && gd.Tok != token.VAR) {
					continue
				}
				for _, spec := range gd.Specs {
					vs, ok := spec.(*ast.ValueSpec)
					if !ok {
						continue
					}
					for _, name := range vs.Names {
						if reMockConstGo.MatchString(name.Name) && looksMockName(name.Name) {
							pos := fset.Position(name.Pos())
							findings = append(findings, Finding{
								File:     path,
								Line:     pos.Line,
								Rule:     RuleMockConstants,
								Severity: severityFor(s.cfg, RuleMockConstants, SeverityHigh),
								Message:  "constant/variable name announces it is a mock: " + name.Name,
							})
						}
					}
				}
			}
		}

		// Rule 10: panic("TODO...") or NotImplementedError-style.
		if ruleActive(s.cfg, RuleNotImplementedThrow) {
			ast.Inspect(file, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				id, ok := call.Fun.(*ast.Ident)
				if !ok || id.Name != "panic" || len(call.Args) == 0 {
					return true
				}
				if lit, ok := call.Args[0].(*ast.BasicLit); ok && lit.Kind == token.STRING {
					if strings.Contains(strings.ToUpper(lit.Value), "TODO") ||
						strings.Contains(strings.ToLower(lit.Value), "not implemented") {
						pos := fset.Position(call.Pos())
						findings = append(findings, Finding{
							File:     path,
							Line:     pos.Line,
							Rule:     RuleNotImplementedThrow,
							Severity: severityFor(s.cfg, RuleNotImplementedThrow, SeverityHigh),
							Message:  "panic(\"TODO/not implemented\") marker",
							Snippet:  "panic(" + lit.Value + ")",
						})
					}
				}
				return true
			})
		}
	}

	// Also do a content-based handler check (covers files where AST parse
	// failed or the handler style is not AST-detectable).
	if !handler {
		handler = isHandlerFile(path, content)
	}

	// Rule 4: handler_no_db_import — fallback when AST didn't see DB import.
	if handler && !hasDB && ruleActive(s.cfg, RuleHandlerNoDBImport) && !hasDBImport(content) {
		findings = append(findings, Finding{
			File:     path,
			Line:     1,
			Rule:     RuleHandlerNoDBImport,
			Severity: severityFor(s.cfg, RuleHandlerNoDBImport, SeverityHigh),
			Message:  "HTTP handler has no database import — likely a stub",
		})
	}

	// --- Regex-based rules (run regardless of parse success) ---

	// Rule 3: todo_comments.
	if ruleActive(s.cfg, RuleTodoComments) {
		for i, line := range lines {
			if reTodoSlash.MatchString(line) {
				findings = append(findings, Finding{
					File:     path,
					Line:     i + 1,
					Rule:     RuleTodoComments,
					Severity: severityFor(s.cfg, RuleTodoComments, SeverityWarn),
					Message:  "TODO/FIXME/HACK/stub comment in production code",
					Snippet:  strings.TrimSpace(line),
				})
			}
		}
	}

	// Rule 8: hardcoded_array.
	if ruleActive(s.cfg, RuleHardcodedArray) {
		for i, line := range lines {
			if reHardcodedArray.MatchString(line) {
				findings = append(findings, Finding{
					File:     path,
					Line:     i + 1,
					Rule:     RuleHardcodedArray,
					Severity: severityFor(s.cfg, RuleHardcodedArray, SeverityWarn),
					Message:  "hardcoded array with >2 object literals",
					Snippet:  strings.TrimSpace(line),
				})
			}
		}
	}

	// Rule 10 fallback: regex for files that couldn't be parsed.
	if parseErr != nil && ruleActive(s.cfg, RuleNotImplementedThrow) {
		for i, line := range lines {
			if reNotImplemented.MatchString(line) {
				findings = append(findings, Finding{
					File:     path,
					Line:     i + 1,
					Rule:     RuleNotImplementedThrow,
					Severity: severityFor(s.cfg, RuleNotImplementedThrow, SeverityHigh),
					Message:  "'not implemented' panic marker",
					Snippet:  strings.TrimSpace(line),
				})
			}
		}
	}

	return findings, nil
}

// looksLikeHTTPHandler returns true for `func X(w http.ResponseWriter, r *http.Request)`.
func looksLikeHTTPHandler(fn *ast.FuncDecl) bool {
	if fn.Type.Params == nil || len(fn.Type.Params.List) < 2 {
		return false
	}
	// First param: http.ResponseWriter.
	p0 := exprString(fn.Type.Params.List[0].Type)
	p1 := exprString(fn.Type.Params.List[1].Type)
	return strings.Contains(p0, "ResponseWriter") && strings.Contains(p1, "Request")
}

// exprString renders an ast.Expr as a best-effort string for simple types.
func exprString(e ast.Expr) string {
	switch t := e.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.SelectorExpr:
		return exprString(t.X) + "." + t.Sel.Name
	case *ast.StarExpr:
		return "*" + exprString(t.X)
	default:
		return ""
	}
}

// isEmptyBody reports whether a function body is empty or just returns nil/zero values.
func isEmptyBody(body *ast.BlockStmt) bool {
	if body == nil || len(body.List) == 0 {
		return true
	}
	if len(body.List) > 1 {
		return false
	}
	ret, ok := body.List[0].(*ast.ReturnStmt)
	if !ok {
		return false
	}
	if len(ret.Results) == 0 {
		return true
	}
	for _, r := range ret.Results {
		switch v := r.(type) {
		case *ast.Ident:
			if v.Name != "nil" {
				return false
			}
		case *ast.CompositeLit:
			if v.Type != nil && len(v.Elts) > 0 {
				return false
			}
		default:
			return false
		}
	}
	return true
}

// hasLiteralFields reports whether a composite literal contains at least one
// field set directly to a string or integer literal.
func hasLiteralFields(lit *ast.CompositeLit) bool {
	for _, elt := range lit.Elts {
		kv, ok := elt.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		if bl, ok := kv.Value.(*ast.BasicLit); ok {
			if bl.Kind == token.STRING || bl.Kind == token.INT || bl.Kind == token.FLOAT {
				return true
			}
		}
	}
	return false
}

// looksMockName guards against matching plain words like "fake" inside longer
// unrelated names. It requires the match to be at the start of the identifier.
func looksMockName(name string) bool {
	lower := strings.ToLower(name)
	prefixes := []string{"mock", "sample_", "fake", "dummy", "stub_"}
	for _, p := range prefixes {
		if strings.HasPrefix(lower, p) {
			return true
		}
	}
	// SCREAMING_SNAKE prefixes.
	upperPrefixes := []string{"MOCK_", "SAMPLE_", "STUB_", "FAKE_", "DUMMY_"}
	for _, p := range upperPrefixes {
		if strings.HasPrefix(name, p) {
			return true
		}
	}
	return false
}
