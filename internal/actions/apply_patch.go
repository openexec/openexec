package actions

import (
	"context"

	"github.com/openexec/openexec/internal/types"
)

// ApplyPatchUnsupported is the refusal apply_patch returns for every valid
// request. The action has no patch engine, so it must not report an effect it
// never had; patches are applied by agentic stages through git_apply_patch.
const ApplyPatchUnsupported = "action apply_patch is not implemented and applied no changes to the workspace; " +
	"apply the patch from an agentic stage with the git_apply_patch tool, or remove this stage from the blueprint"

// ApplyPatchAction is the registered apply_patch action. It validates its
// input and then refuses, because applying patches is not implemented here.
type ApplyPatchAction struct {
	projectDir string
}

func NewApplyPatchAction(projectDir string) *ApplyPatchAction {
	return &ApplyPatchAction{projectDir: projectDir}
}

func (a *ApplyPatchAction) Name() string {
	return "apply_patch"
}

func (a *ApplyPatchAction) Execute(ctx context.Context, req ActionRequest) (ActionResponse, error) {
	implementation, ok := req.Inputs["implementation"].(string)
	if !ok || implementation == "" {
		return ActionResponse{
			Status: types.StageStatusFailed,
			Error:  "no implementation data provided",
		}, nil
	}

	return ActionResponse{
		Status: types.StageStatusFailed,
		Error:  ApplyPatchUnsupported,
	}, nil
}
