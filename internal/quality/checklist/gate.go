package checklist

import (
	"context"
	"fmt"
	"strings"
)

// GateReport summarizes a gate invocation for pipeline consumption.
type GateReport struct {
	Passed      bool      `json:"passed"`
	Summary     string    `json:"summary"`
	Findings    []Finding `json:"findings"`
	HighCount   int       `json:"high_count"`
	WarnCount   int       `json:"warn_count"`
	ProjectRoot string    `json:"project_root"`
}

// Gate runs the production-readiness checklist as a pipeline gate.
// It fails (Passed=false) whenever there is at least one HIGH severity
// finding. Warnings never fail the build.
type Gate struct {
	root string
	cfg  Config
}

// NewGate constructs a new Gate bound to a project root.
func NewGate(root string, cfg Config) *Gate {
	return &Gate{root: root, cfg: cfg}
}

// Name returns the gate's display name.
func (g *Gate) Name() string { return "production-ready" }

// Run executes the gate and returns a GateReport.
func (g *Gate) Run(ctx context.Context) (*GateReport, error) {
	_ = ctx
	findings, err := Run(g.root, g.cfg)
	if err != nil {
		return nil, err
	}
	counts := severityCounts(findings)
	report := &GateReport{
		Findings:    findings,
		HighCount:   counts[string(SeverityHigh)],
		WarnCount:   counts[string(SeverityWarn)],
		ProjectRoot: g.root,
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
		return fmt.Errorf("production-ready gate: %w", err)
	}
	if !report.Passed {
		return fmt.Errorf("%s", report.Summary)
	}
	return nil
}

func (g *Gate) summarize(report *GateReport) string {
	if report.Passed {
		if report.WarnCount == 0 {
			return "production-ready: clean"
		}
		return fmt.Sprintf("production-ready: clean (%d warnings)", report.WarnCount)
	}
	var top []string
	seen := map[string]bool{}
	for _, f := range report.Findings {
		if f.Severity != SeverityHigh {
			continue
		}
		key := f.Check + "|" + f.File
		if seen[key] {
			continue
		}
		seen[key] = true
		if f.File != "" {
			top = append(top, fmt.Sprintf("%s [%s]", f.File, f.Check))
		} else {
			top = append(top, f.Check)
		}
		if len(top) >= 5 {
			break
		}
	}
	return fmt.Sprintf("production-ready: %d HIGH findings, %d warnings (top: %s)",
		report.HighCount, report.WarnCount, strings.Join(top, ", "))
}
