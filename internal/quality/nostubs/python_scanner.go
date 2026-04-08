package nostubs

import (
	"os"
	"path/filepath"
	"strings"
)

// PythonScanner scans Python source files for stub patterns using regex only.
type PythonScanner struct {
	cfg Config
}

// NewPythonScanner creates a new Python scanner.
func NewPythonScanner(cfg Config) *PythonScanner {
	return &PythonScanner{cfg: cfg}
}

// Language returns "py".
func (s *PythonScanner) Language() string { return "py" }

// Scan walks the root directory and returns findings for all .py files.
func (s *PythonScanner) Scan(root string) ([]Finding, error) {
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
		if !strings.HasSuffix(path, ".py") {
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

func (s *PythonScanner) scanFile(path string) ([]Finding, error) {
	data, err := os.ReadFile(path) // #nosec G304
	if err != nil {
		return nil, err
	}
	content := string(data)
	lines := strings.Split(content, "\n")

	if isTestPath(path) {
		return nil, nil
	}

	var findings []Finding

	// Rule 1: mock_constants.
	if ruleActive(s.cfg, RuleMockConstants) {
		for i, line := range lines {
			if reMockConstPy.MatchString(line) {
				findings = append(findings, Finding{
					File:     path,
					Line:     i + 1,
					Rule:     RuleMockConstants,
					Severity: severityFor(s.cfg, RuleMockConstants, SeverityHigh),
					Message:  "module-level binding with mock/stub/sample name",
					Snippet:  strings.TrimSpace(line),
				})
			}
		}
	}

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

	handler := isHandlerFile(path, content)

	// Rule 4: handler_no_db_import.
	if handler && ruleActive(s.cfg, RuleHandlerNoDBImport) && !hasDBImport(content) {
		findings = append(findings, Finding{
			File:     path,
			Line:     1,
			Rule:     RuleHandlerNoDBImport,
			Severity: severityFor(s.cfg, RuleHandlerNoDBImport, SeverityHigh),
			Message:  "HTTP handler file has no database import — likely a stub",
		})
	}

	// Rule 5: handler_hardcoded_return.
	if handler && ruleActive(s.cfg, RuleHandlerHardcoded) {
		for i, line := range lines {
			if reHardcodedReturnPy.MatchString(line) {
				findings = append(findings, Finding{
					File:     path,
					Line:     i + 1,
					Rule:     RuleHandlerHardcoded,
					Severity: severityFor(s.cfg, RuleHandlerHardcoded, SeverityHigh),
					Message:  "handler returns a hardcoded literal dict",
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
					Message:  "hardcoded list with >2 dict literals",
					Snippet:  strings.TrimSpace(line),
				})
			}
		}
	}

	// Rule 10: not_implemented_throw (raise NotImplementedError).
	if ruleActive(s.cfg, RuleNotImplementedThrow) {
		for i, line := range lines {
			if reNotImplemented.MatchString(line) {
				findings = append(findings, Finding{
					File:     path,
					Line:     i + 1,
					Rule:     RuleNotImplementedThrow,
					Severity: severityFor(s.cfg, RuleNotImplementedThrow, SeverityHigh),
					Message:  "raise NotImplementedError marker",
					Snippet:  strings.TrimSpace(line),
				})
			}
		}
	}

	return findings, nil
}
