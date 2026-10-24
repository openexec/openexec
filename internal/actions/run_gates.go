package actions

import (
	"context"
	"strings"
	"time"

	"github.com/openexec/openexec/internal/execution/gates"
	"github.com/openexec/openexec/internal/types"
)

// RunGatesAction executes configured quality gates (lint/test).
type RunGatesAction struct {
	projectDir string
}

func NewRunGatesAction(projectDir string) *RunGatesAction {
	return &RunGatesAction{projectDir: projectDir}
}

func (a *RunGatesAction) Name() string {
	return "run_gates"
}

func (a *RunGatesAction) Execute(ctx context.Context, req ActionRequest) (ActionResponse, error) {
	runner, err := gates.NewRunner(a.projectDir, 5*time.Minute)
	if err != nil {
		return ActionResponse{}, err
	}

	report := runner.RunAll(ctx)

	status := types.StageStatusCompleted
	failure := strings.Join(report.FailedGates, ", ")
	if !report.Passed {
		status = types.StageStatusFailed
		if failure == "" {
			// A refusal with no failed gate, e.g. none declared.
			failure = report.Summary
		}
	}

	return ActionResponse{
		Status: status,
		Output: report.Summary,
		Error:  failure,
	}, nil
}
