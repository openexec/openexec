package actions

import (
	"context"
	"fmt"
	"strings"

	"github.com/openexec/openexec/internal/quality/checklist"
	"github.com/openexec/openexec/internal/types"
)

// ChecklistRunAction runs the production-readiness checklist against
// the current project root and emits a markdown summary to the stage
// output. It is registered under the name "checklist.Run" and wired
// into the default blueprint after `test` and before `review`.
//
// This action is non-blocking by design: the stage OnFailure path loops
// back into `review`, mirroring the contract-tests stage. The build is
// failed by the manager-side gate in pkg/manager (see the production-
// ready gate wiring), not by this stage.
type ChecklistRunAction struct {
	projectDir string
}

// NewChecklistRunAction constructs a new action bound to the given
// project directory.
func NewChecklistRunAction(projectDir string) *ChecklistRunAction {
	return &ChecklistRunAction{projectDir: projectDir}
}

// Name implements types.Action.
func (a *ChecklistRunAction) Name() string {
	return "checklist.Run"
}

// Execute runs the checklist. The stage input may carry a "skip" key
// containing a comma-separated list of check IDs to skip.
func (a *ChecklistRunAction) Execute(ctx context.Context, req ActionRequest) (ActionResponse, error) {
	_ = ctx
	root := req.WorkspaceDir
	if root == "" {
		root = a.projectDir
	}

	var cfg checklist.Config
	if skip := stringInput(req.Inputs, "skip"); skip != "" {
		parts := strings.Split(skip, ",")
		for _, p := range parts {
			if p = strings.TrimSpace(p); p != "" {
				cfg.Skip = append(cfg.Skip, p)
			}
		}
	}

	findings, err := checklist.Run(root, cfg)
	if err != nil {
		return ActionResponse{
			Status: types.StageStatusFailed,
			Error:  fmt.Sprintf("checklist run: %v", err),
		}, nil
	}

	var md strings.Builder
	if werr := checklist.WriteMarkdown(findings, &md); werr != nil {
		return ActionResponse{
			Status: types.StageStatusFailed,
			Error:  fmt.Sprintf("checklist render: %v", werr),
		}, nil
	}

	var high, warn int
	for _, f := range findings {
		switch f.Severity {
		case checklist.SeverityHigh:
			high++
		case checklist.SeverityWarn:
			warn++
		}
	}

	status := types.StageStatusCompleted
	summary := fmt.Sprintf("production-ready: %d HIGH, %d warnings", high, warn)

	return ActionResponse{
		Status: status,
		Output: summary + "\n\n" + md.String(),
		Artifacts: map[string]string{
			"checklist_high_count": fmt.Sprintf("%d", high),
			"checklist_warn_count": fmt.Sprintf("%d", warn),
		},
	}, nil
}
