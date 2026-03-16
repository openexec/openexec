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
	if !report.Passed {
		status = types.StageStatusFailed
	}

	return ActionResponse{
		Status: status,
		Output: report.Summary,
		Error:  strings.Join(report.FailedGates, ", "),
	}, nil
}
