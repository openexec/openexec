// Package checklist implements the "boring infrastructure"
// production-readiness gate. It runs a set of focused checkers against
// a project root and returns findings in a shape that mirrors the
// no-stubs gate in internal/quality/nostubs.
//
// The default checkers ensure that:
//
//   - Every env var referenced in code is documented in .env.example.
//   - No common secret patterns are committed.
//   - Database migrations have a description and a down path.
//   - Every non-public API route handler checks the session.
//   - A health endpoint exists and returns 200.
//
// Each checker is independent and can be disabled via Config.Skip.
package checklist

import (
	"fmt"
	"sort"
)

// Severity represents the severity of a checklist finding.
type Severity string

const (
	// SeverityWarn is a warning — does not fail the gate by default.
	SeverityWarn Severity = "warn"
	// SeverityHigh is a blocking finding — fails the gate.
	SeverityHigh Severity = "high"
)

// Finding is a single production-readiness finding.
type Finding struct {
	Check    string   `json:"check"`
	Severity Severity `json:"severity"`
	File     string   `json:"file,omitempty"`
	Line     int      `json:"line,omitempty"`
	Message  string   `json:"message"`
}

// Config configures a checklist run.
type Config struct {
	// Skip lists check IDs to skip entirely.
	Skip []string
}

// Checker is a single production-readiness check.
type Checker interface {
	// ID is a stable identifier for the check, used in Finding.Check and
	// Config.Skip.
	ID() string
	// Run scans the project root and returns any findings.
	Run(root string) ([]Finding, error)
}

// DefaultCheckers returns the built-in set of checkers.
func DefaultCheckers() []Checker {
	return []Checker{
		&EnvVarsChecker{},
		&SecretsChecker{},
		&MigrationsChecker{},
		&AuthAnnotationsChecker{},
		&HealthCheckChecker{},
	}
}

// Run executes all default checkers (minus Config.Skip) against root
// and returns findings sorted by severity then file.
func Run(root string, cfg Config) ([]Finding, error) {
	skip := make(map[string]bool, len(cfg.Skip))
	for _, s := range cfg.Skip {
		skip[s] = true
	}

	var findings []Finding
	for _, c := range DefaultCheckers() {
		if skip[c.ID()] {
			continue
		}
		f, err := c.Run(root)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", c.ID(), err)
		}
		findings = append(findings, f...)
	}

	sort.SliceStable(findings, func(i, j int) bool {
		if findings[i].Severity != findings[j].Severity {
			return sevRank(findings[i].Severity) > sevRank(findings[j].Severity)
		}
		if findings[i].File != findings[j].File {
			return findings[i].File < findings[j].File
		}
		if findings[i].Line != findings[j].Line {
			return findings[i].Line < findings[j].Line
		}
		return findings[i].Check < findings[j].Check
	})

	return findings, nil
}

func sevRank(s Severity) int {
	switch s {
	case SeverityHigh:
		return 2
	case SeverityWarn:
		return 1
	}
	return 0
}

// severityCounts returns a map keyed by severity with the number of
// findings at that severity.
func severityCounts(findings []Finding) map[string]int {
	counts := map[string]int{
		string(SeverityHigh): 0,
		string(SeverityWarn): 0,
	}
	for _, f := range findings {
		counts[string(f.Severity)]++
	}
	return counts
}

// defaultExcludeDirs are directories the walkers skip. Kept conservative
// so we catch real source files but not generated or third-party code.
var defaultExcludeDirs = map[string]bool{
	"node_modules": true,
	".svelte-kit":  true,
	"dist":         true,
	"build":        true,
	".git":         true,
	"tests":        true,
	"__tests__":    true,
	".openexec":    true,
	".uaos":        true,
	".next":        true,
	"target":       true,
	"vendor":       true,
	"testdata":     true,
	"fixtures":     true,
	"mocks":        true,
	"coverage":     true,
}
