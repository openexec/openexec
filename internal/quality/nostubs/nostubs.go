// Package nostubs implements the "no-stubs" verifier — a static analyzer
// that detects mock constants, placeholder returns, TODO comments, and other
// stub patterns that commonly slip through LLM-generated code.
//
// It is used in two places:
//
//  1. As a quality gate wired into the execution pipeline (see gate.go).
//  2. As a standalone `openexec audit` command (see internal/cli/audit.go).
//
// The package is intentionally dependency-light: all scanners are regex- or
// AST-based and operate directly on the filesystem without requiring a build.
package nostubs

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
)

// Severity represents the severity of a finding.
type Severity string

const (
	// SeverityLow is informational.
	SeverityLow Severity = "low"
	// SeverityWarn is a warning — does not fail the build by default.
	SeverityWarn Severity = "warn"
	// SeverityHigh is a blocking finding — fails the quality gate.
	SeverityHigh Severity = "high"
	// SeverityOff disables the rule.
	SeverityOff Severity = "off"
)

// Finding is a single detected stub pattern.
type Finding struct {
	File     string   `json:"file"`
	Line     int      `json:"line"`
	Rule     string   `json:"rule"`
	Severity Severity `json:"severity"`
	Message  string   `json:"message"`
	Snippet  string   `json:"snippet,omitempty"`
}

// Scanner scans a directory tree for findings in a particular language.
type Scanner interface {
	// Scan walks `root` and returns all findings.
	Scan(root string) ([]Finding, error)
	// Language returns the short language identifier (e.g. "ts", "go", "py").
	Language() string
}

// Config configures a nostubs run.
type Config struct {
	// Languages restricts which scanners run. Empty means all.
	// Valid values: "ts", "go", "py".
	Languages []string
	// RuleOverrides maps a rule ID to a Severity. Use SeverityOff to disable.
	RuleOverrides map[string]Severity
	// ExcludeDirs is added to the default exclude list.
	ExcludeDirs []string
	// RuleIDs restricts the rules that run. Empty means all.
	RuleIDs []string
}

// defaultExcludes are directory names that are always skipped during a scan.
var defaultExcludes = []string{
	"node_modules", "dist", "build", ".git", ".svelte-kit",
	"target", "vendor", "tests", "__tests__", "__mocks__",
	"testdata", "fixtures", "mocks", ".next", ".venv", "venv",
	".openexec", ".uaos",
}

// isExcludedDir reports whether the given directory base name should be skipped.
func isExcludedDir(name string, extra []string) bool {
	for _, e := range defaultExcludes {
		if name == e {
			return true
		}
	}
	for _, e := range extra {
		if name == e {
			return true
		}
	}
	return false
}

// isTestPath reports whether the given path looks like a test file or a file
// located under a test directory. This is used to suppress rules that should
// only fire in production code.
func isTestPath(path string) bool {
	p := filepath.ToSlash(path)
	lower := strings.ToLower(p)
	testSegments := []string{
		"/tests/", "/test/", "/__tests__/", "/__mocks__/",
		"/testdata/", "/fixtures/", "/mocks/", "/e2e/",
		"/spec/", "/specs/",
	}
	for _, seg := range testSegments {
		if strings.Contains(lower, seg) {
			return true
		}
	}
	base := strings.ToLower(filepath.Base(p))
	if strings.HasSuffix(base, "_test.go") ||
		strings.HasSuffix(base, ".test.ts") ||
		strings.HasSuffix(base, ".test.tsx") ||
		strings.HasSuffix(base, ".test.js") ||
		strings.HasSuffix(base, ".test.jsx") ||
		strings.HasSuffix(base, ".spec.ts") ||
		strings.HasSuffix(base, ".spec.tsx") ||
		strings.HasSuffix(base, ".spec.js") ||
		strings.HasSuffix(base, ".spec.jsx") ||
		strings.HasSuffix(base, "_test.py") ||
		strings.HasPrefix(base, "test_") {
		return true
	}
	return false
}

// Run executes the configured scanners against `root` and returns all
// findings, sorted by file and line.
func Run(root string, cfg Config) ([]Finding, error) {
	scanners := []Scanner{
		NewTSScanner(cfg),
		NewGoScanner(cfg),
		NewPythonScanner(cfg),
	}

	var enabled []Scanner
	if len(cfg.Languages) == 0 {
		enabled = scanners
	} else {
		want := map[string]bool{}
		for _, l := range cfg.Languages {
			want[strings.ToLower(strings.TrimSpace(l))] = true
		}
		for _, s := range scanners {
			if want[s.Language()] {
				enabled = append(enabled, s)
			}
		}
	}

	var all []Finding
	for _, s := range enabled {
		f, err := s.Scan(root)
		if err != nil {
			return nil, fmt.Errorf("%s scanner: %w", s.Language(), err)
		}
		all = append(all, f...)
	}

	// Apply rule severity overrides.
	if len(cfg.RuleOverrides) > 0 {
		filtered := all[:0]
		for _, f := range all {
			if sev, ok := cfg.RuleOverrides[f.Rule]; ok {
				if sev == SeverityOff {
					continue
				}
				f.Severity = sev
			}
			filtered = append(filtered, f)
		}
		all = filtered
	}

	sort.Slice(all, func(i, j int) bool {
		if all[i].File != all[j].File {
			return all[i].File < all[j].File
		}
		if all[i].Line != all[j].Line {
			return all[i].Line < all[j].Line
		}
		return all[i].Rule < all[j].Rule
	})

	return all, nil
}

// HighestSeverity reports the most severe level found. Returns empty string
// when findings is empty.
func HighestSeverity(findings []Finding) Severity {
	rank := map[Severity]int{
		SeverityLow:  1,
		SeverityWarn: 2,
		SeverityHigh: 3,
	}
	var top Severity
	best := 0
	for _, f := range findings {
		if rank[f.Severity] > best {
			best = rank[f.Severity]
			top = f.Severity
		}
	}
	return top
}
