package execution

import (
	"context"
	"strings"
	"testing"

	"github.com/openexec/openexec/pkg/agent"
)

// recordingWorkspace stands in for the real workspace executor: it answers for
// the file half and records what reached it, so routing can be observed rather
// than inferred from a success.
type recordingWorkspace struct {
	called    []string
	refuse    error
	validated []Sandbox
}

func (w *recordingWorkspace) ValidateAccess(_ string, sandbox Sandbox, _ []string) error {
	w.validated = append(w.validated, sandbox)
	return w.refuse
}

func (w *recordingWorkspace) ExecuteTool(_ context.Context, request ToolRequest) (string, error) {
	w.called = append(w.called, request.Name)
	return "workspace ran " + request.Name, nil
}

func (w *recordingWorkspace) SupportsWorkspaceWrite() bool { return true }

func gatewayStub(tools ...string) *GatewayToolExecutor {
	defined := make([]agent.ToolDefinition, 0, len(tools))
	for _, name := range tools {
		defined = append(defined, agent.ToolDefinition{Name: name})
	}
	return &GatewayToolExecutor{tools: defined}
}

func TestCompositeRoutesEachVerbToTheHalfThatOwnsIt(t *testing.T) {
	workspace := &recordingWorkspace{}
	executor, err := NewCompositeToolExecutor(workspace,
		[]agent.ToolDefinition{{Name: "read_file"}, {Name: "write_file"}}, gatewayStub("run_command", "git_push"))
	if err != nil {
		t.Fatal(err)
	}

	names := map[string]bool{}
	for _, tool := range executor.Tools() {
		names[tool.Name] = true
	}
	for _, want := range []string{"read_file", "write_file", "run_command", "git_push"} {
		if !names[want] {
			t.Errorf("%s is not offered; a composite run must see both halves", want)
		}
	}

	request := ToolRequest{Name: "write_file", Sandbox: Sandbox{Mode: SandboxWorkspaceWrite}, WritableRoots: []string{"/repo"}}
	if _, err := executor.ExecuteTool(context.Background(), request); err != nil {
		t.Fatalf("workspace verb was refused: %v", err)
	}
	if len(workspace.called) != 1 || workspace.called[0] != "write_file" {
		t.Fatalf("workspace calls = %v, want the write routed to the file half", workspace.called)
	}
}

// The point of the type: workspace-write is exactly the shape the standalone
// gateway refuses, and the composite must allow it or it delivers nothing.
func TestCompositeAllowsWorkspaceWriteThatAStandaloneGatewayRefuses(t *testing.T) {
	sandbox := Sandbox{Mode: SandboxWorkspaceWrite}
	roots := []string{"/repo"}

	if err := gatewayStub("run_command").ValidateAccess("/repo", sandbox, roots); err == nil {
		t.Fatal("the standalone gateway accepted workspace-write, so this test proves nothing")
	}

	executor, err := NewCompositeToolExecutor(&recordingWorkspace{},
		[]agent.ToolDefinition{{Name: "read_file"}}, gatewayStub("run_command"))
	if err != nil {
		t.Fatal(err)
	}
	if err := executor.ValidateAccess("/repo", sandbox, roots); err != nil {
		t.Fatalf("composite refused the mode it exists to allow: %v", err)
	}
	if !executor.SupportsWorkspaceWrite() {
		t.Error("composite must report workspace-write, or a caller will not choose it for editing work")
	}
}

// Authority is the workspace half's, on every call — including the forwarded
// ones. A verb reached over the gateway must not be a way around the sandbox.
func TestCompositeAppliesWorkspaceAuthorityToForwardedVerbs(t *testing.T) {
	workspace := &recordingWorkspace{refuse: errUnauthorizedForTest{}}
	executor, err := NewCompositeToolExecutor(workspace,
		[]agent.ToolDefinition{{Name: "read_file"}}, gatewayStub("run_command"))
	if err != nil {
		t.Fatal(err)
	}
	_, err = executor.ExecuteTool(context.Background(),
		ToolRequest{Name: "run_command", Sandbox: Sandbox{Mode: SandboxWorkspaceWrite}})
	if err == nil {
		t.Fatal("a forwarded verb ran while the workspace executor refused the run")
	}
	if len(workspace.validated) == 0 {
		t.Error("the workspace executor was never asked about a forwarded call")
	}
}

// A name on both sides has no answer at the point authority is decided, and
// resolving it by ordering would make which half runs an accident of
// construction.
func TestCompositeRefusesAVerbClaimedByBothHalves(t *testing.T) {
	_, err := NewCompositeToolExecutor(&recordingWorkspace{},
		[]agent.ToolDefinition{{Name: "run_command"}}, gatewayStub("run_command"))
	if err == nil {
		t.Fatal("a duplicated tool name was accepted")
	}
	if !strings.Contains(err.Error(), "run_command") {
		t.Errorf("error does not name the clashing verb: %v", err)
	}
}

// The provider decides it serves a gateway from behaviour, not concrete type —
// a composite forwards some verbs and would otherwise be told it has none,
// then refuse the very requests it was built for.
func TestCompositeIsRecognisedAsServingAGateway(t *testing.T) {
	executor, err := NewCompositeToolExecutor(&recordingWorkspace{},
		[]agent.ToolDefinition{{Name: "read_file"}}, gatewayStub("run_command"))
	if err != nil {
		t.Fatal(err)
	}
	provider, err := NewAPIProvider(APIProviderConfig{
		Adapter: &fakeAPIAdapter{}, Tools: executor.Tools(), ToolExecutor: executor,
	})
	if err != nil {
		t.Fatal(err)
	}
	descriptor := provider.Descriptor()
	if !descriptor.Capabilities.ToolGateway {
		t.Error("a composite executor was not reported as serving a gateway")
	}
	if !descriptor.Capabilities.WorkspaceWrite {
		t.Error("a composite executor was not reported as able to write the workspace")
	}
}

type errUnauthorizedForTest struct{}

func (errUnauthorizedForTest) Error() string { return "not authorized for this run" }
