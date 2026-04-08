package nostubs

import (
	"context"
	"fmt"
	"strings"
)

// GateReport summarizes a gate invocation in a form the pipeline can consume.
type GateReport struct {
	Passed      bool      `json:"passed"`
	Summary     string    `json:"summary"`
	Findings    []Finding `json:"findings"`
	HighCount   int       `json:"high_count"`
	WarnCount   int       `json:"warn_count"`
	LowCount    int       `json:"low_count"`
	ProjectRoot string    `json:"project_root"`
}

// Gate wraps a nostubs scan so it can be used as a pipeline quality gate.
// It fails (Passed=false) whenever there is at least one HIGH severity finding.
type Gate struct {
	ProjectRoot string
	Config      Config
}

// NewGate creates a new no-stubs gate rooted at projectRoot with the given config.
func NewGate(projectRoot string, cfg Config) *Gate {
	return &Gate{ProjectRoot: projectRoot, Config: cfg}
}

// Name returns the gate's display name.
func (g *Gate) Name() string { return "no-stubs" }

// Run executes the gate and returns a GateReport. The context is accepted for
// interface compatibility; the underlying scanners are synchronous and ignore
// it.
func (g *Gate) Run(ctx context.Context) (*GateReport, error) {
	_ = ctx
	findings, err := Run(g.ProjectRoot, g.Config)
	if err != nil {
		return nil, err
	}
	counts := severityCounts(findings)
	report := &GateReport{
		Findings:    findings,
		HighCount:   counts[string(SeverityHigh)],
		WarnCount:   counts[string(SeverityWarn)],
		LowCount:    counts[string(SeverityLow)],
		ProjectRoot: g.ProjectRoot,
	}
	report.Passed = report.HighCount == 0
	report.Summary = g.summarize(report)
	return report, nil
}

// RunAll satisfies the pipeline.GateRunner-style interface used by the
// manager. It returns an error if any HIGH-severity finding is present.
func (g *Gate) RunAll(ctx context.Context) error {
	report, err := g.Run(ctx)
	if err != nil {
		return fmt.Errorf("no-stubs gate: %w", err)
	}
	if !report.Passed {
		return fmt.Errorf("%s", report.Summary)
	}
	return nil
}

func (g *Gate) summarize(report *GateReport) string {
	if report.Passed {
		if report.WarnCount == 0 {
			return "no-stubs: clean"
		}
		return fmt.Sprintf("no-stubs: clean (%d warnings)", report.WarnCount)
	}
	var top []string
	seen := map[string]bool{}
	for _, f := range report.Findings {
		if f.Severity != SeverityHigh {
			continue
		}
		key := f.File
		if seen[key] {
			continue
		}
		seen[key] = true
		top = append(top, fmt.Sprintf("%s:%d [%s]", f.File, f.Line, f.Rule))
		if len(top) >= 5 {
			break
		}
	}
	return fmt.Sprintf("no-stubs: %d HIGH findings, %d warnings (top: %s)",
		report.HighCount, report.WarnCount, strings.Join(top, ", "))
}
