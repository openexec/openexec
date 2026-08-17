package cli

import (
	"context"
	"strings"
	"testing"

	"github.com/openexec/openexec/pkg/execution"
)

// The standalone gateway is the default, and stays refused in workspace-write.
//
// Console state and the repository in one turn is the pairing with no
// defensible story. A caller that names an endpoint without saying what it is
// for gets that rule — so a console with a bug, and an older console that has
// never heard of the other scope, both fail closed rather than quietly
// receiving file tools alongside console verbs.
func TestGatewayWithoutAScopeStaysReadOnly(t *testing.T) {
	root := writeProjectConfig(t, "qwen-gpu0")
	_, err := newConfiguredAPIProvider(context.Background(), root, "qwen-gpu0",
		execution.Sandbox{Mode: execution.SandboxWorkspaceWrite},
		toolGateway{Endpoint: "http://127.0.0.1:9/mcp/secret/run"})
	if err == nil {
		t.Fatal("a workspace-write run with an unscoped gateway was accepted")
	}
	if !strings.Contains(err.Error(), "read-only") {
		t.Fatalf("refusal does not name the rule that applied: %v", err)
	}
}

// And the strict rule is about the pairing, not about gateways: read-only with
// no scope still builds, exactly as the admin lane has always used it.
func TestUnscopedGatewayStillServesTheReadOnlyAdminShape(t *testing.T) {
	root := writeProjectConfig(t, "qwen-gpu0")
	_, err := newConfiguredAPIProvider(context.Background(), root, "qwen-gpu0",
		execution.Sandbox{Mode: execution.SandboxReadOnly},
		toolGateway{Endpoint: "http://127.0.0.1:9/mcp/secret/run"})
	// The endpoint is unreachable in a test, so construction fails at the
	// listing rather than at the rule. What must not appear is the read-only
	// refusal: that would mean the admin lane had been broken by this change.
	if err != nil && strings.Contains(err.Error(), "read-only") {
		t.Fatalf("the read-only rule fired on a read-only run: %v", err)
	}
}

// The composite scope is for editing work, and the runtime says so itself.
//
// A read-only run handed forwarded verbs would be read-only in name only: the
// workspace executor validates paths, not modes, so run_command and git_push
// would have been offered and executed with nothing downstream objecting.
func TestWithWorkspaceScopeRequiresWorkspaceWrite(t *testing.T) {
	root := writeProjectConfig(t, "qwen-gpu0")
	_, err := newConfiguredAPIProvider(context.Background(), root, "qwen-gpu0",
		execution.Sandbox{Mode: execution.SandboxReadOnly},
		toolGateway{Endpoint: "http://127.0.0.1:9/mcp/secret/run", Scope: execution.GatewayScopeWithWorkspace})
	if err == nil {
		t.Fatal("a read-only run with the composite scope was accepted")
	}
	if !strings.Contains(err.Error(), "workspace-write") {
		t.Fatalf("refusal does not name the mode the scope requires: %v", err)
	}
	// Refused before the endpoint is contacted: the tools must not even be
	// listed for a run that may not have them.
	if strings.Contains(err.Error(), "connection refused") || strings.Contains(err.Error(), "dial") {
		t.Fatalf("the gateway was contacted before the mode was checked: %v", err)
	}
}

// The runtime declares composite support as its own bit, so a caller can tell
// a build that has it from one that merely forwards console state.
func TestRuntimeDeclaresCompositeSupportSeparately(t *testing.T) {
	var capability execution.Capability
	if capability.ToolGatewayWithWorkspace {
		t.Fatal("the zero value claims composite support; older runtimes must read as false")
	}
}
