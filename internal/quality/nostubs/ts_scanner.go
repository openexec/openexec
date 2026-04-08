package nostubs

import (
	"os"
	"path/filepath"
	"strings"
)

// TSScanner scans TypeScript, JavaScript, and Svelte files for stub patterns.
type TSScanner struct {
	cfg Config
}

// NewTSScanner creates a new TypeScript/JavaScript/Svelte scanner.
func NewTSScanner(cfg Config) *TSScanner {
	return &TSScanner{cfg: cfg}
}

// Language returns "ts".
func (s *TSScanner) Language() string { return "ts" }

var tsExts = map[string]bool{
	".ts":     true,
	".tsx":    true,
	".js":     true,
	".jsx":    true,
	".mjs":    true,
	".cjs":    true,
	".svelte": true,
}

// Scan walks the root directory and returns findings for all TS/JS/Svelte files.
func (s *TSScanner) Scan(root string) ([]Finding, error) {
	var out []Finding
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			// Silently skip unreadable entries — scanner should be resilient.
			return nil
		}
		if d.IsDir() {
			if isExcludedDir(d.Name(), s.cfg.ExcludeDirs) {
				return filepath.SkipDir
			}
			return nil
		}
		ext := strings.ToLower(filepath.Ext(path))
		if !tsExts[ext] {
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

func (s *TSScanner) scanFile(path string) ([]Finding, error) {
	data, err := os.ReadFile(path) // #nosec G304 - user-supplied audit paths
	if err != nil {
		return nil, err
	}
	content := string(data)
	lines := strings.Split(content, "\n")

	var findings []Finding
	test := isTestPath(path)

	// Rule 1: mock_constants (production code only).
	if !test && ruleActive(s.cfg, RuleMockConstants) {
		for i, line := range lines {
			if m := reMockConstJS.FindString(line); m != "" {
				findings = append(findings, Finding{
					File:     path,
					Line:     i + 1,
					Rule:     RuleMockConstants,
					Severity: severityFor(s.cfg, RuleMockConstants, SeverityHigh),
					Message:  "constant name starts with a mock/stub/sample/fake/dummy prefix",
					Snippet:  strings.TrimSpace(line),
				})
			}
		}
	}

	// Rule 2: mock_imports (production code only).
	if !test && ruleActive(s.cfg, RuleMockImports) {
		for i, line := range lines {
			if m := reMockImport.FindStringSubmatch(line); m != nil {
				findings = append(findings, Finding{
					File:     path,
					Line:     i + 1,
					Rule:     RuleMockImports,
					Severity: severityFor(s.cfg, RuleMockImports, SeverityHigh),
					Message:  "import pulled from mock/fixture module: " + m[1],
					Snippet:  strings.TrimSpace(line),
				})
			}
		}
	}

	// Rule 3: todo_comments (production code only).
	if !test && ruleActive(s.cfg, RuleTodoComments) {
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

	handler := isHandlerFile(path, content)
	stateless := isStatelessHandlerPath(path)

	// Rule 4: handler_no_db_import.
	if !test && handler && !stateless && ruleActive(s.cfg, RuleHandlerNoDBImport) && !hasDBImport(content) {
		findings = append(findings, Finding{
			File:     path,
			Line:     1,
			Rule:     RuleHandlerNoDBImport,
			Severity: severityFor(s.cfg, RuleHandlerNoDBImport, SeverityHigh),
			Message:  "HTTP handler has no database import — likely a stub",
		})
	}

	// Rule 5: handler_hardcoded_return.
	if !test && handler && !stateless && ruleActive(s.cfg, RuleHandlerHardcoded) {
		for i, line := range lines {
			if reHardcodedReturnJS.MatchString(line) {
				findings = append(findings, Finding{
					File:     path,
					Line:     i + 1,
					Rule:     RuleHandlerHardcoded,
					Severity: severityFor(s.cfg, RuleHandlerHardcoded, SeverityHigh),
					Message:  "handler returns a hardcoded literal object",
					Snippet:  strings.TrimSpace(line),
				})
			}
		}
	}

	// Rule 6: server_no_await for SvelteKit +server.ts files.
	if !test && !stateless && ruleActive(s.cfg, RuleServerNoAwait) {
		lower := strings.ToLower(filepath.ToSlash(path))
		if strings.Contains(lower, "/routes/") && strings.HasSuffix(lower, "+server.ts") {
			if !strings.Contains(content, "await") {
				findings = append(findings, Finding{
					File:     path,
					Line:     1,
					Rule:     RuleServerNoAwait,
					Severity: severityFor(s.cfg, RuleServerNoAwait, SeverityWarn),
					Message:  "+server.ts contains no await — likely synchronous stub",
				})
			}
		}
	}

	// Rule 7: promise_resolve_literal.
	if !test && ruleActive(s.cfg, RulePromiseResolveLit) {
		for i, line := range lines {
			if rePromiseResolveLit.MatchString(line) {
				findings = append(findings, Finding{
					File:     path,
					Line:     i + 1,
					Rule:     RulePromiseResolveLit,
					Severity: severityFor(s.cfg, RulePromiseResolveLit, SeverityWarn),
					Message:  "Promise.resolve wraps a literal object",
					Snippet:  strings.TrimSpace(line),
				})
			}
		}
	}

	// Rule 8: hardcoded_array.
	if !test && ruleActive(s.cfg, RuleHardcodedArray) {
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

	// Rule 9: empty_handler.
	if !test && handler && !stateless && ruleActive(s.cfg, RuleEmptyHandler) {
		for i, line := range lines {
			if reEmptyReturn.MatchString(line) {
				findings = append(findings, Finding{
					File:     path,
					Line:     i + 1,
					Rule:     RuleEmptyHandler,
					Severity: severityFor(s.cfg, RuleEmptyHandler, SeverityHigh),
					Message:  "handler body is an empty return",
					Snippet:  strings.TrimSpace(line),
				})
			}
		}
	}

	// Rule 10: not_implemented_throw.
	if !test && ruleActive(s.cfg, RuleNotImplementedThrow) {
		for i, line := range lines {
			if reNotImplemented.MatchString(line) {
				findings = append(findings, Finding{
					File:     path,
					Line:     i + 1,
					Rule:     RuleNotImplementedThrow,
					Severity: severityFor(s.cfg, RuleNotImplementedThrow, SeverityHigh),
					Message:  "'not implemented' throw/panic/raise",
					Snippet:  strings.TrimSpace(line),
				})
			}
		}
	}

	return findings, nil
}
