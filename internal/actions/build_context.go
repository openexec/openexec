package actions

import (
	"context"
	"fmt"

	ocontext "github.com/openexec/openexec/internal/context"
	"github.com/openexec/openexec/internal/types"
)

// BuildContextAction assembles the local context pack.
type BuildContextAction struct {
	projectDir string
}

func NewBuildContextAction(projectDir string) *BuildContextAction {
	return &BuildContextAction{projectDir: projectDir}
}

func (a *BuildContextAction) Name() string {
	return "build_context"
}

func (a *BuildContextAction) Execute(ctx context.Context, req ActionRequest) (ActionResponse, error) {
	// Simple v1 context assembly
	pack, err := ocontext.BuildContextWithRouting(
		ctx,
		a.projectDir,
		32000, // Default budget
		nil,   // No specific zones
		nil,   // No specific sources
		"low", // Sensitivity
	)
	if err != nil {
		return ActionResponse{}, err
	}

	return ActionResponse{
		Status:    types.StageStatusCompleted,
		Output:    fmt.Sprintf("Assembled context with %d items", len(pack.Items)),
		Artifacts: map[string]string{"context_assembled": "true"},
	}, nil
}
