package execution

import (
	"context"
	"fmt"

	"github.com/openexec/openexec/pkg/agent"
)

// CompositeToolExecutor runs OpenExec's workspace tools beside verbs that
// belong to the caller.
//
// It exists for one shape of run: an agent editing a repository that must also
// be able to build it, test it, and push it. OpenExec owns the file tools and
// bounds them by sandbox and roots. It does not own a shell, deliberately —
// running commands is the caller's to authorize, under the caller's approval,
// on the caller's terms — so those verbs arrive over a gateway instead.
//
// This is not the console-state gateway, and the difference is the whole
// reason ToolGatewayScope is named on the wire. That one stands alone by
// construction: console state and a repository in one turn is a pairing with
// no defensible story. This one grants an API provider exactly what every
// agent CLI on this contract already had.
//
// Routing is by name, decided once at construction from what the gateway
// actually offered. A verb the gateway does not offer is a workspace tool or
// it is nothing.
type CompositeToolExecutor struct {
	workspace ToolExecutor
	gateway   *GatewayToolExecutor
	forwarded map[string]bool
	tools     []agent.ToolDefinition
}

// NewCompositeToolExecutor combines workspace tools with a gateway's verbs.
//
// A name offered by both would be ambiguous at the point where authority is
// decided, which is the one place ambiguity may not be resolved by ordering.
// Refused at construction rather than shadowed silently.
func NewCompositeToolExecutor(workspace ToolExecutor, workspaceTools []agent.ToolDefinition, gateway *GatewayToolExecutor) (*CompositeToolExecutor, error) {
	if workspace == nil {
		return nil, fmt.Errorf("composite tool executor requires a workspace executor")
	}
	if gateway == nil {
		return nil, fmt.Errorf("composite tool executor requires a gateway executor")
	}
	owned := map[string]bool{}
	for _, tool := range workspaceTools {
		owned[tool.Name] = true
	}
	forwarded := map[string]bool{}
	combined := append([]agent.ToolDefinition(nil), workspaceTools...)
	for _, tool := range gateway.Tools() {
		if owned[tool.Name] {
			return nil, fmt.Errorf("tool %q is offered by both the workspace and the gateway", tool.Name)
		}
		forwarded[tool.Name] = true
		combined = append(combined, tool)
	}
	return &CompositeToolExecutor{workspace: workspace, gateway: gateway, forwarded: forwarded, tools: combined}, nil
}

// Tools is every verb this run may call, workspace and forwarded together.
func (e *CompositeToolExecutor) Tools() []agent.ToolDefinition {
	return append([]agent.ToolDefinition(nil), e.tools...)
}

// SupportsWorkspaceWrite answers for the half that touches files. The gateway
// half has no filesystem and no opinion about one.
func (e *CompositeToolExecutor) SupportsWorkspaceWrite() bool {
	return supportsWorkspaceWrite(e.workspace)
}

func (e *CompositeToolExecutor) servesGateway() bool { return true }

// ValidateAccess is the workspace executor's answer alone.
//
// Not the gateway's: its rule is that the run is read-only with no writable
// roots, which is exactly the shape this executor exists to allow. Applying it
// here would refuse every run this type is for. What still holds is the
// workspace executor's bounding of the sandbox and the roots, which is the
// check that decides what a model may touch.
func (e *CompositeToolExecutor) ValidateAccess(workingDir string, sandbox Sandbox, writableRoots []string) error {
	return e.workspace.ValidateAccess(workingDir, sandbox, writableRoots)
}

// ExecuteTool routes one call to whichever half owns the name.
//
// Authority is checked before routing, every call, so a forwarded verb cannot
// be used to step around the sandbox the workspace half enforces — the gateway
// is reached only by a run that was already allowed to be here.
func (e *CompositeToolExecutor) ExecuteTool(ctx context.Context, request ToolRequest) (string, error) {
	if err := e.ValidateAccess(request.WorkingDir, request.Sandbox, request.WritableRoots); err != nil {
		return "", err
	}
	if e.forwarded[request.Name] {
		return e.gateway.executeAuthorized(ctx, request)
	}
	return e.workspace.ExecuteTool(ctx, request)
}
