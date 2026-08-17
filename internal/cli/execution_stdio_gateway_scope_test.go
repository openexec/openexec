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
