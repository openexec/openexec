package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/openexec/openexec/internal/approval"
	"github.com/openexec/openexec/internal/infra"
)

// fakeRunner records the argv it was asked to run instead of executing.
type fakeRunner struct {
	binary string
	args   []string
	output string
	calls  int
}

func (f *fakeRunner) Run(_ context.Context, binary string, args []string) (string, int, error) {
	f.binary = binary
	f.args = append([]string{}, args...)
	f.calls++
	return f.output, 0, nil
}

// fakeGate auto-approves (or auto-rejects) every request.
type fakeGate struct {
	approve  bool
	requests []*approval.GateRequest
}

func (g *fakeGate) RequestApproval(_ context.Context, req *approval.GateRequest) (*approval.GateRequest, error) {
	g.requests = append(g.requests, req)
	if g.approve {
		req.Status = approval.RequestStatusApproved
	} else {
		req.Status = approval.RequestStatusRejected
		req.RejectReason = "operator said no"
	}
	return req, nil
}
func (g *fakeGate) GetPendingApprovals(string) []*approval.GateRequest { return nil }
func (g *fakeGate) GetAllPendingApprovals() []*approval.GateRequest    { return nil }
func (g *fakeGate) Approve(string, string) error                       { return nil }
func (g *fakeGate) Reject(string, string, string) error                { return nil }
func (g *fakeGate) CancelForRun(string) int                            { return 0 }

func newInfraTestServer(t *testing.T, mode string) (*Server, *bytes.Buffer, *fakeRunner) {
	t.Helper()
	out := new(bytes.Buffer)
	srv, err := NewServerWithConfig(strings.NewReader(""), out, ServerConfig{
		WorkDir: t.TempDir(),
		Mode:    mode,
	})
	if err != nil {
		t.Fatalf("NewServerWithConfig: %v", err)
	}
	reg := auditInfraRegistry() // staging env with all four engines
	srv.SetInfraRegistry(reg)
	runner := &fakeRunner{output: "ok\n"}
	srv.SetInfraRunner(runner)
	return srv, out, runner
}

func TestInfraToolsAdvertisedOnlyInFullAuto(t *testing.T) {
	srv, out, _ := newInfraTestServer(t, "danger-full-access")
	srv.dispatch(Request{JSONRPC: "2.0", ID: json.RawMessage(`1`), Method: "tools/list"})
	if !strings.Contains(out.String(), "ansible_run_playbook") ||
		!strings.Contains(out.String(), "terraform_plan") {
		t.Error("full-auto tools/list should advertise infra tools when a registry is set")
	}
	// Enums from the allowlist must be visible to the model.
	if !strings.Contains(out.String(), "deploy.yml") || !strings.Contains(out.String(), "check_disk") {
		t.Error("infra tool schemas should carry allowlist enums")
	}

	srvRestricted, outR, _ := newInfraTestServer(t, "auto-edit")
	srvRestricted.dispatch(Request{JSONRPC: "2.0", ID: json.RawMessage(`1`), Method: "tools/list"})
	if strings.Contains(outR.String(), "ansible_run_playbook") {
		t.Error("infra tools must not be advertised outside danger-full-access mode")
	}
}

func TestInfraToolsModeGated(t *testing.T) {
	for _, mode := range []string{"suggest", "auto-edit"} {
		srv, out, runner := newInfraTestServer(t, mode)
		result := callTool(t, srv, out, "ssh_run_query", map[string]interface{}{
			"environment": "staging", "host": "10.0.1.5", "query": "check_disk",
		})
		if !isToolError(result) {
			t.Errorf("[%s] infra tool should be refused outside full-auto", mode)
		}
		if runner.calls != 0 {
			t.Errorf("[%s] runner must not be invoked on a refused call", mode)
		}
	}
}

func TestSSHRunQuery_EndToEnd(t *testing.T) {
	srv, out, runner := newInfraTestServer(t, "danger-full-access")
	result := callTool(t, srv, out, "ssh_run_query", map[string]interface{}{
		"environment": "staging", "host": "10.0.1.5", "query": "check_disk",
	})
	if isToolError(result) {
		t.Fatalf("expected success, got error: %s", resultText(result))
	}
	if runner.binary != "ssh" {
		t.Errorf("binary = %q, want ssh", runner.binary)
	}
	want := []string{"-o", "BatchMode=yes", "-o", "StrictHostKeyChecking=accept-new", "10.0.1.5", "df", "-h"}
	if strings.Join(runner.args, " ") != strings.Join(want, " ") {
		t.Errorf("argv = %q, want %q", runner.args, want)
	}
	if ac, _ := result["apply_class"].(bool); ac {
		t.Error("ssh query must be read-class")
	}
}

func TestInfraDenyByDefaultSurfacesAsToolError(t *testing.T) {
	srv, out, runner := newInfraTestServer(t, "danger-full-access")
	result := callTool(t, srv, out, "ansible_run_playbook", map[string]interface{}{
		"environment": "staging", "playbook": "../../etc/passwd",
	})
	if !isToolError(result) || !strings.Contains(resultText(result), "allowlist") {
		t.Errorf("hostile playbook should be an allowlist rejection, got: %s", resultText(result))
	}
	if runner.calls != 0 {
		t.Error("runner must not be invoked for a rejected resolution")
	}
}

func TestApplyClassFailsClosedWithoutGate(t *testing.T) {
	srv, out, runner := newInfraTestServer(t, "danger-full-access")
	// No approval gate wired: a real playbook run must refuse.
	result := callTool(t, srv, out, "ansible_run_playbook", map[string]interface{}{
		"environment": "staging", "playbook": "deploy.yml",
	})
	if !isToolError(result) || !strings.Contains(resultText(result), "fail-closed") {
		t.Errorf("apply-class without a gate must fail closed, got: %s", resultText(result))
	}
	if runner.calls != 0 {
		t.Error("runner must not be invoked without approval")
	}

	// The --check dry-run is read-class and runs without a gate.
	result = callTool(t, srv, out, "ansible_run_playbook", map[string]interface{}{
		"environment": "staging", "playbook": "deploy.yml", "check": true,
	})
	if isToolError(result) {
		t.Fatalf("--check dry-run should run without a gate: %s", resultText(result))
	}
	if runner.calls != 1 || runner.args[len(runner.args)-1] != "--check" {
		t.Errorf("dry-run argv = %q (calls=%d)", runner.args, runner.calls)
	}
}

func TestApplyClassWithGate(t *testing.T) {
	srv, out, runner := newInfraTestServer(t, "danger-full-access")
	gate := &fakeGate{approve: true}
	SetApprovalGateForServer(srv, gate)
	t.Cleanup(func() { CleanupApprovalGateForServer(srv) })

	result := callTool(t, srv, out, "salt_apply_state", map[string]interface{}{
		"environment": "staging", "state": "app.deploy", "target": "web*",
	})
	if isToolError(result) {
		t.Fatalf("approved apply should run: %s", resultText(result))
	}
	if len(gate.requests) != 1 || gate.requests[0].ToolName != "salt_apply_state" {
		t.Fatalf("expected one approval request for salt_apply_state, got %+v", gate.requests)
	}
	if runner.binary != "salt" || strings.Join(runner.args, " ") != "web* state.apply app.deploy" {
		t.Errorf("argv = %q %q", runner.binary, runner.args)
	}

	// Rejection path: operator says no → no execution.
	gate.approve = false
	runner.calls = 0
	result = callTool(t, srv, out, "salt_apply_state", map[string]interface{}{
		"environment": "staging", "state": "app.deploy", "target": "web*",
	})
	if !isToolError(result) || !strings.Contains(resultText(result), "operator said no") {
		t.Errorf("rejected apply should surface the rejection: %s", resultText(result))
	}
	if runner.calls != 0 {
		t.Error("runner must not be invoked after rejection")
	}
}

func TestInfraToolWithoutRegistryRefuses(t *testing.T) {
	out := new(bytes.Buffer)
	srv, err := NewServerWithConfig(strings.NewReader(""), out, ServerConfig{
		WorkDir: t.TempDir(),
		Mode:    "danger-full-access",
	})
	if err != nil {
		t.Fatalf("NewServerWithConfig: %v", err)
	}
	result := callTool(t, srv, out, "terraform_plan", map[string]interface{}{"environment": "staging"})
	if !isToolError(result) || !strings.Contains(resultText(result), "not enabled") {
		t.Errorf("infra tool without a loaded allowlist must refuse, got: %s", resultText(result))
	}
}

func TestTerraformPlan_VarsValidated(t *testing.T) {
	srv, out, runner := newInfraTestServer(t, "danger-full-access")
	result := callTool(t, srv, out, "terraform_plan", map[string]interface{}{
		"environment": "staging",
		"vars":        map[string]interface{}{"instance_count": "3"},
	})
	if isToolError(result) {
		t.Fatalf("allowlisted var should pass: %s", resultText(result))
	}
	if !strings.Contains(strings.Join(runner.args, " "), "-var=instance_count=3") {
		t.Errorf("argv = %q", runner.args)
	}

	result = callTool(t, srv, out, "terraform_plan", map[string]interface{}{
		"environment": "staging",
		"vars":        map[string]interface{}{"admin_password": "x"},
	})
	if !isToolError(result) {
		t.Error("non-allowlisted var must be rejected")
	}
}

// Guard against accidental re-introduction of generic-path gating for infra
// tools (it would double-prompt and cannot distinguish dry-runs).
func TestInfraToolsSkipGenericApprovalPath(t *testing.T) {
	srv, _, _ := newInfraTestServer(t, "danger-full-access")
	gate := &fakeGate{approve: true}
	SetApprovalGateForServer(srv, gate)
	SetApprovalEnabledForServer(srv, true)
	t.Cleanup(func() { CleanupApprovalGateForServer(srv) })

	if RequiresApprovalForTool(srv, "ansible_run_playbook") {
		t.Error("infra tools must be exempt from the generic approval path; their handlers gate apply-class invocations themselves")
	}

	infraReg := srv.infraRegistry
	if infraReg == nil {
		t.Fatal("test setup: registry missing")
	}
	if _, err := infra.NewRegistry(nil); err == nil {
		t.Error("sanity: nil config must not produce a registry")
	}
}
