package loop

import (
	"context"
	"fmt"

	"github.com/openexec/openexec/internal/gates"
)

// GateValidation holds the result of gate validation.
type GateValidation struct {
	PreflightReport *gates.PreflightReport
	GateReport      *gates.GateReport
	Passed          bool
	FixPrompt       string
}

// runPreflightChecks runs preflight validation before task execution.
func (l *Loop) runPreflightChecks() *gates.PreflightReport {
	if !l.cfg.PreflightChecks {
		return nil
	}

	l.emit(Event{Type: EventPreflightStart})

	// Get gate names from config
	gateRunner, err := gates.NewRunner(l.cfg.WorkDir, l.cfg.GateTimeout)
	if err != nil {
		return nil
	}

	var gateNames []string
	if gateRunner != nil {
		cfg, _ := gates.LoadConfig(l.cfg.WorkDir)
		if cfg != nil {
			gateNames = cfg.GetEnabledGates()
		}
	}

	report := gates.RunPreflightChecks(l.cfg.TaskTitle, gateNames)

	if report.Passed {
		l.emit(Event{
			Type: EventPreflightPassed,
			Text: report.Summary,
		})
	} else {
		l.emit(Event{
			Type:    EventPreflightFailed,
			Text:    report.Summary,
			ErrText: gates.FormatPreflightReport(report),
		})
	}

	return report
}

// runQualityGates runs quality gates after task signals completion.
func (l *Loop) runQualityGates(ctx context.Context) *gates.GateReport {
	if !l.cfg.QualityGates {
		return nil
	}

	l.emit(Event{Type: EventGatesStart})

	runner, err := gates.NewRunner(l.cfg.WorkDir, l.cfg.GateTimeout)
	if err != nil {
		l.emit(Event{
			Type:    EventError,
			ErrText: fmt.Sprintf("failed to create gate runner: %v", err),
		})
		return nil
	}

	report := runner.RunAll(ctx)

	if report.Passed {
		l.emit(Event{
			Type: EventGatesPassed,
			Text: report.Summary,
		})
	} else {
		l.emit(Event{
			Type:    EventGatesFailed,
			Text:    report.Summary,
			ErrText: runner.FormatForExecutor(report),
		})
	}

	return report
}

// buildGateFixPrompt creates a prompt telling the executor to fix gate failures.
func (l *Loop) buildGateFixPrompt(report *gates.GateReport, attempt int) string {
	runner, _ := gates.NewRunner(l.cfg.WorkDir, l.cfg.GateTimeout)

	prompt := fmt.Sprintf(`
QUALITY GATES FAILED - FIX REQUIRED (Attempt %d/%d)

%s

Your previous implementation caused quality gate failures. You MUST fix these issues.

DO NOT just signal completion again. Actually fix the code so the gates pass.

After fixing, the quality gates will automatically re-run. They must pass before the task is considered complete.
`, attempt, l.cfg.MaxGateRetries, runner.FormatForExecutor(report))

	return prompt
}
