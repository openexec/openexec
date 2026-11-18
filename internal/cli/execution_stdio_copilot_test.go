package cli

import (
	"context"
	"testing"

	"github.com/openexec/openexec/pkg/execution"
)

func TestExecutionStdioSelectsCopilotCLI(t *testing.T) {
	old := executionProviderKind
	t.Cleanup(func() { executionProviderKind = old })

	executionProviderKind = "copilot"
	provider, err := executionProviderFor(context.Background(), t.TempDir(), execution.Sandbox{Mode: "read-only"}, toolGateway{})
	if err != nil {
		t.Fatal(err)
	}
	if got := provider.Descriptor().ID; got != "copilot" {
		t.Fatalf("provider id = %q", got)
	}

	executionProviderKind = "githubcopilot"
	if _, err := executionProviderFor(context.Background(), t.TempDir(), execution.Sandbox{Mode: "read-only"}, toolGateway{}); err == nil {
		t.Fatal("unknown provider kind was accepted")
	}
}
