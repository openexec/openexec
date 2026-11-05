//go:build linux || darwin

package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/openexec/openexec/pkg/agent"
	"github.com/openexec/openexec/pkg/execution"
)

// scriptedAdapter is the model endpoint: it replays one tool call and then
// finishes. before runs when the model is first asked, i.e. after admission.
type scriptedAdapter struct {
	tool   string
	input  json.RawMessage
	before func()
	calls  int
}

func (a *scriptedAdapter) GetName() string     { return "openai-compatible" }
func (a *scriptedAdapter) GetModels() []string { return []string{"test-model"} }
func (a *scriptedAdapter) GetModelInfo(string) (*agent.ModelInfo, error) {
	return &agent.ModelInfo{}, nil
}
func (a *scriptedAdapter) GetCapabilities(string) (*agent.ProviderCapabilities, error) {
	return &agent.ProviderCapabilities{Streaming: true, ToolUse: true}, nil
}
func (a *scriptedAdapter) Complete(context.Context, agent.Request) (*agent.Response, error) {
	a.calls++
	if a.calls == 1 {
		if a.before != nil {
			a.before()
		}
		return &agent.Response{Content: []agent.ContentBlock{{Type: agent.ContentTypeToolUse, ToolUseID: "t1", ToolName: a.tool, ToolInput: a.input}}, StopReason: agent.StopReasonToolUse}, nil
	}
	return &agent.Response{Content: []agent.ContentBlock{{Type: agent.ContentTypeText, Text: "finished"}}, StopReason: agent.StopReasonEnd}, nil
}
func (a *scriptedAdapter) Stream(context.Context, agent.Request) (<-chan agent.StreamEvent, error) {
	return nil, errors.New("not streamed")
}
func (a *scriptedAdapter) ValidateRequest(agent.Request) error { return nil }
func (a *scriptedAdapter) EstimateTokens(string) int           { return 1 }

// unpinnedExecutor serves tools but cannot hold a directory descriptor.
type unpinnedExecutor struct{}

func (unpinnedExecutor) ExecuteTool(context.Context, execution.ToolRequest) (string, error) {
	return "", nil
}
func (unpinnedExecutor) ValidateAccess(string, execution.Sandbox, []string) error { return nil }

const (
	authorizedBytes = "authorized-needle"
	outsideBytes    = "outside-secret-needle"
)

type rootFixture struct {
	working, selected, outside string
}

func newRootFixture(t *testing.T) rootFixture {
	t.Helper()
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	f := rootFixture{filepath.Join(base, "candidate"), filepath.Join(base, "selected"), filepath.Join(base, "unrelated")}
	for _, dir := range []string{f.working, f.selected, f.outside} {
		if err := os.Mkdir(dir, 0700); err != nil {
			t.Fatal(err)
		}
	}
	for dir, text := range map[string]string{f.selected: authorizedBytes, f.outside: outsideBytes} {
		if err := os.WriteFile(filepath.Join(dir, "witness"), []byte(text), 0600); err != nil {
			t.Fatal(err)
		}
	}
	return f
}

// capture is the caller's step: open the directory and record what it is.
func (f rootFixture) capture(t *testing.T, path string) execution.ReadableRootIdentity {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	identity, err := execution.ReadableRootIdentityOf(path, info)
	if err != nil {
		t.Fatal(err)
	}
	return identity
}

func (f rootFixture) swapToOutside(t *testing.T) {
	t.Helper()
	if err := os.Rename(f.selected, f.selected+"-old"); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(f.outside, f.selected); err != nil {
		t.Fatal(err)
	}
}

// replaceWithOutside puts the outside directory itself at the root's name, so a
// path-based re-resolution would find a real, consistent directory.
func (f rootFixture) replaceWithOutside(t *testing.T) {
	t.Helper()
	if err := os.Rename(f.selected, f.selected+"-old"); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(f.outside, f.selected); err != nil {
		t.Fatal(err)
	}
}

func (f rootFixture) toolCall(tool string) (string, json.RawMessage) {
	input := map[string]string{"path": filepath.Join(f.selected, "witness")}
	switch tool {
	case "list_directory":
		input = map[string]string{"path": f.selected}
	case "search_files":
		input = map[string]string{"path": f.selected, "text": "needle"}
	}
	raw, _ := json.Marshal(input)
	return tool, raw
}

func (f rootFixture) executor(t *testing.T, composite bool) execution.ToolExecutor {
	t.Helper()
	var executor execution.ToolExecutor = execution.NewWorkspaceToolExecutor()
	if composite {
		var err error
		executor, err = execution.NewCompositeToolExecutor(executor, execution.WorkspaceTools(execution.Sandbox{Mode: execution.SandboxReadOnly}), &execution.GatewayToolExecutor{})
		if err != nil {
			t.Fatal(err)
		}
	}
	return executor
}

type stdioRun struct {
	stdout, toolOutput, failure string
	err                         error
	providerBuilt               bool
}

// visible is everything the caller could see: the stream, the result error and
// the serve error.
func (r stdioRun) visible() string {
	text := r.stdout + r.failure
	if r.err != nil {
		text += r.err.Error()
	}
	return text
}

// serve runs the real stdio reader, the real API provider and the real tool
// executor. Only the model endpoint is scripted.
func (f rootFixture) serve(t *testing.T, newest, version int, roots []string, identities []execution.ReadableRootIdentity, adapter *scriptedAdapter, executor execution.ToolExecutor) stdioRun {
	t.Helper()
	provider, err := execution.NewAPIProvider(execution.APIProviderConfig{Adapter: adapter, Tools: execution.WorkspaceTools(execution.Sandbox{Mode: execution.SandboxReadOnly}), ToolExecutor: executor})
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(executionEnvelope{Version: version, Operation: "execute", Request: &execution.Request{ID: "connected", WorkingDir: f.working, Prompt: "read it", Model: "test-model", Sandbox: execution.Sandbox{Mode: execution.SandboxReadOnly}, ReadableRoots: roots, ReadableRootIdentities: identities}})
	if err != nil {
		t.Fatal(err)
	}
	var run stdioRun
	var out bytes.Buffer
	run.err = serveExecutionProtocolUpTo(context.Background(), newest, bytes.NewReader(data), &out, func(context.Context, string, execution.Sandbox, toolGateway) (execution.Provider, error) {
		run.providerBuilt = true
		return provider, nil
	})
	run.stdout = out.String()
	for _, line := range strings.Split(strings.TrimSpace(run.stdout), "\n") {
		var envelope executionEnvelope
		if json.Unmarshal([]byte(line), &envelope) != nil {
			continue
		}
		if envelope.Event != nil && envelope.Event.Type == execution.EventToolCompleted {
			run.toolOutput = envelope.Event.Text
		}
		run.failure += envelope.Error
	}
	return run
}

func (f rootFixture) noOutsideDisclosure(t *testing.T, run stdioRun) {
	t.Helper()
	for _, secret := range []string{outsideBytes, "outside-secret", f.outside} {
		if strings.Contains(run.visible(), secret) {
			t.Fatalf("outside data %q reached the caller: %s", secret, run.visible())
		}
	}
}

func TestConnectedValidRootServesReadListSearchThroughPinnedDescriptor(t *testing.T) {
	for _, composite := range []bool{false, true} {
		for _, tool := range []string{"read_file", "list_directory", "search_files"} {
			for _, swap := range []string{"", "symlink", "directory"} {
				name := tool
				if composite {
					name += "/composite"
				}
				if swap != "" {
					name += "/" + swap + "-swapped-after-admission"
				}
				t.Run(name, func(t *testing.T) {
					f := newRootFixture(t)
					identity := f.capture(t, f.selected)
					name, input := f.toolCall(tool)
					adapter := &scriptedAdapter{tool: name, input: input}
					switch swap {
					case "symlink":
						adapter.before = func() { f.swapToOutside(t) }
					case "directory":
						adapter.before = func() { f.replaceWithOutside(t) }
					}
					run := f.serve(t, executionProtocolBoundRoots, 4, []string{f.selected}, []execution.ReadableRootIdentity{identity}, adapter, f.executor(t, composite))
					if swap != "" {
						// A root swapped after admission is refused, not served from
						// the old descriptor, and nothing outside reaches the caller.
						f.noOutsideDisclosure(t, run)
						if !strings.Contains(run.visible(), "readable root changed after admission") {
							t.Fatalf("swap after admission was not refused: err=%v failure=%q stdout=%q", run.err, run.failure, run.stdout)
						}
						if strings.Contains(run.visible(), authorizedBytes) || strings.Contains(run.toolOutput, "witness") {
							t.Fatalf("swapped root still served: %q", run.visible())
						}
						if _, err := os.Lstat(f.selected + "-old"); err != nil {
							t.Fatalf("the swap did not happen, so the refusal is unproven: %v", err)
						}
						return
					}
					if run.err != nil || run.failure != "" {
						t.Fatalf("valid root refused: %v %q", run.err, run.failure)
					}
					f.noOutsideDisclosure(t, run)
					// After the swap the path names the outside directory. Only the
					// descriptor opened at admission can still answer with the
					// original data.
					want := authorizedBytes
					if tool == "list_directory" {
						want = "witness"
					}
					// Swapped or not, the answer is the original directory's content.
					// After the swap the path names the outside directory, so only the
					// descriptor held since admission can produce it. A refusal is not
					// accepted: it would not show which object served.
					if !strings.Contains(run.toolOutput, want) {
						t.Fatalf("%s output %q (calls=%d)", tool, run.toolOutput, adapter.calls)
					}
					if swap != "" {
						if _, err := os.Lstat(f.selected + "-old"); err != nil {
							t.Fatalf("the swap did not happen, so descriptor use is unproven: %v", err)
						}
					}
				})
			}
		}
	}
}

func TestConnectedUnsafeRootsFailSafely(t *testing.T) {
	for _, composite := range []bool{false, true} {
		for _, tc := range []struct {
			name  string
			setup func(t *testing.T, f rootFixture) ([]string, []execution.ReadableRootIdentity)
			want  string
		}{
			{"swap before admission to a symlink", func(t *testing.T, f rootFixture) ([]string, []execution.ReadableRootIdentity) {
				id := f.capture(t, f.selected)
				f.swapToOutside(t)
				return []string{f.selected}, []execution.ReadableRootIdentity{id}
			}, "not a stable canonical directory"},
			{"swap before admission to another directory", func(t *testing.T, f rootFixture) ([]string, []execution.ReadableRootIdentity) {
				id := f.capture(t, f.selected)
				if err := os.Rename(f.selected, f.selected+"-old"); err != nil {
					t.Fatal(err)
				}
				if err := os.Rename(f.outside, f.selected); err != nil {
					t.Fatal(err)
				}
				return []string{f.selected}, []execution.ReadableRootIdentity{id}
			}, "changed since caller admission"},
			{"symlink root", func(t *testing.T, f rootFixture) ([]string, []execution.ReadableRootIdentity) {
				alias := filepath.Join(filepath.Dir(f.selected), "alias")
				if err := os.Symlink(f.outside, alias); err != nil {
					t.Fatal(err)
				}
				return []string{alias}, []execution.ReadableRootIdentity{f.capture(t, alias)}
			}, "not a stable canonical directory"},
			{"non-matching identity", func(t *testing.T, f rootFixture) ([]string, []execution.ReadableRootIdentity) {
				id := f.capture(t, f.selected)
				id.Inode++
				return []string{f.selected}, []execution.ReadableRootIdentity{id}
			}, "changed since caller admission"},
			{"duplicate identity", func(t *testing.T, f rootFixture) ([]string, []execution.ReadableRootIdentity) {
				id := f.capture(t, f.selected)
				return []string{f.selected, f.selected}, []execution.ReadableRootIdentity{id, id}
			}, "duplicate readable root identity"},
			{"identity for a different root", func(t *testing.T, f rootFixture) ([]string, []execution.ReadableRootIdentity) {
				return []string{f.selected}, []execution.ReadableRootIdentity{f.capture(t, f.outside)}
			}, "changed since caller admission"},
		} {
			name := tc.name
			if composite {
				name += "/composite"
			}
			t.Run(name, func(t *testing.T) {
				f := newRootFixture(t)
				roots, identities := tc.setup(t, f)
				tool, input := f.toolCall("read_file")
				adapter := &scriptedAdapter{tool: tool, input: input}
				run := f.serve(t, executionProtocolBoundRoots, 4, roots, identities, adapter, f.executor(t, composite))
				if !strings.Contains(run.visible(), tc.want) {
					t.Fatalf("want refusal %q, got err=%v failure=%q", tc.want, run.err, run.failure)
				}
				if adapter.calls != 0 || run.toolOutput != "" {
					t.Fatalf("inference or a tool ran despite the refusal: calls=%d tool=%q", adapter.calls, run.toolOutput)
				}
				f.noOutsideDisclosure(t, run)
			})
		}
	}
}

func TestConnectedV4RefusesMissingIdentityAndProviderThatCannotEnforce(t *testing.T) {
	f := newRootFixture(t)
	id := f.capture(t, f.selected)
	tool, input := f.toolCall("read_file")
	for _, tc := range []struct {
		name       string
		roots      []string
		identities []execution.ReadableRootIdentity
		executor   execution.ToolExecutor
		want       string
	}{
		{"missing identity", []string{f.selected}, nil, f.executor(t, false), "requires an identity for every readable root"},
		{"fewer identities than roots", []string{f.selected, f.outside}, []execution.ReadableRootIdentity{id}, f.executor(t, false), "requires an identity for every readable root"},
		{"provider cannot enforce identity", []string{f.selected}, []execution.ReadableRootIdentity{id}, unpinnedExecutor{}, "provider cannot enforce bound readable roots"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			adapter := &scriptedAdapter{tool: tool, input: input}
			run := f.serve(t, executionProtocolBoundRoots, 4, tc.roots, tc.identities, adapter, tc.executor)
			if run.err == nil || !strings.Contains(run.err.Error(), tc.want) {
				t.Fatalf("err=%v want %q", run.err, tc.want)
			}
			if adapter.calls != 0 || run.toolOutput != "" || run.stdout != "" {
				t.Fatalf("work was dispatched despite the refusal: calls=%d out=%q", adapter.calls, run.stdout)
			}
			f.noOutsideDisclosure(t, run)
		})
	}
}

// A runtime that predates version 4 is the same reader that only understands
// up to version 3. It must refuse before building a provider, not ignore the
// identity fields it has no name for.
func TestConnectedRuntimeWithoutV4RefusesV4Request(t *testing.T) {
	f := newRootFixture(t)
	id := f.capture(t, f.selected)
	tool, input := f.toolCall("read_file")
	adapter := &scriptedAdapter{tool: tool, input: input}
	run := f.serve(t, executionProtocolVersion, 4, []string{f.selected}, []execution.ReadableRootIdentity{id}, adapter, f.executor(t, false))
	if run.err == nil || !strings.Contains(run.err.Error(), "unsupported execution protocol version 4") {
		t.Fatalf("old runtime did not refuse v4: %v", run.err)
	}
	// Same bytes-on-the-wire and same refusal wording as the pre-v4 reader in
	// TestLegacyReaderRefusesV4Request: the identity fields were not stripped,
	// and the refusal does not echo any root path.
	if strings.Contains(run.err.Error(), f.selected) || strings.Contains(run.err.Error(), "Inode") {
		t.Fatalf("refusal echoes request content: %v", run.err)
	}
	var refusal *unsupportedProtocolVersionError
	if !errors.As(run.err, &refusal) || refusal.Version != 4 {
		t.Fatalf("refusal is not typed: %T %v", run.err, run.err)
	}
	if run.providerBuilt || adapter.calls != 0 || run.stdout != "" {
		t.Fatalf("old runtime did work for a v4 request: built=%v calls=%d out=%q", run.providerBuilt, adapter.calls, run.stdout)
	}
}

// v1-v3 callers send roots without identities and are served exactly as before;
// they cannot smuggle identities in under an older version.
func TestConnectedV1ToV3KeepTheirBehaviour(t *testing.T) {
	for _, version := range []int{1, 2, 3} {
		t.Run("without identities", func(t *testing.T) {
			f := newRootFixture(t)
			tool, input := f.toolCall("read_file")
			adapter := &scriptedAdapter{tool: tool, input: input}
			run := f.serve(t, executionProtocolBoundRoots, version, []string{f.selected}, nil, adapter, f.executor(t, false))
			if run.err != nil || run.toolOutput != authorizedBytes {
				t.Fatalf("v%d changed: err=%v tool=%q", version, run.err, run.toolOutput)
			}
		})
		t.Run("with identities", func(t *testing.T) {
			f := newRootFixture(t)
			tool, input := f.toolCall("read_file")
			adapter := &scriptedAdapter{tool: tool, input: input}
			run := f.serve(t, executionProtocolBoundRoots, version, []string{f.selected}, []execution.ReadableRootIdentity{f.capture(t, f.selected)}, adapter, f.executor(t, false))
			if run.err == nil || !strings.Contains(run.err.Error(), "require protocol version 4") || adapter.calls != 0 {
				t.Fatalf("v%d accepted identities: err=%v calls=%d", version, run.err, adapter.calls)
			}
		})
	}
}
