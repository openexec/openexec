package actions

import (
	"context"

	"github.com/openexec/openexec/internal/types"
)

// Re-export types from internal/types for convenience in the actions package
type ActionRequest = types.ActionRequest
type ActionResponse = types.ActionResponse
type Action = types.Action

// Execute is a helper to run an action.
func Execute(ctx context.Context, a Action, req ActionRequest) (ActionResponse, error) {
	return a.Execute(ctx, req)
}
