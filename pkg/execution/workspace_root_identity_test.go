package execution

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/openexec/openexec/pkg/agent"
)

func TestAPIWorkspaceReadableRootStaysBoundAfterAdmission(t *testing.T) {
	for _, scenario := range []struct {
		name            string
		swap, composite bool
	}{{"normal", false, false}, {"root replaced", true, false}, {"composite root replaced", true, true}} {
		t.Run(scenario.name, func(t *testing.T) {
			base, err := filepath.EvalSymlinks(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			working := filepath.Join(base, "candidate")
			sibling := filepath.Join(base, "selected")
			outside := filepath.Join(base, "unrelated")
			for _, dir := range []string{working, sibling, outside} {
				if err := os.Mkdir(dir, 0700); err != nil {
					t.Fatal(err)
				}
			}
			if err := os.WriteFile(filepath.Join(sibling, "witness"), []byte("authorized"), 0600); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(outside, "witness"), []byte("outside-secret"), 0600); err != nil {
				t.Fatal(err)
			}
			input, _ := json.Marshal(map[string]string{"path": filepath.Join(sibling, "witness")})
			adapter := &fakeAPIAdapter{responses: []*agent.Response{{Content: []agent.ContentBlock{{Type: agent.ContentTypeToolUse, ToolUseID: "read", ToolName: "read_file", ToolInput: input}}, StopReason: agent.StopReasonToolUse}, {Content: []agent.ContentBlock{{Type: agent.ContentTypeText, Text: "finished"}}, StopReason: agent.StopReasonEnd}}}
			var executor ToolExecutor = NewWorkspaceToolExecutor()
			if scenario.composite {
				executor, err = NewCompositeToolExecutor(executor, WorkspaceTools(Sandbox{Mode: SandboxReadOnly}), &GatewayToolExecutor{})
				if err != nil {
					t.Fatal(err)
				}
			}
			p, err := NewAPIProvider(APIProviderConfig{Adapter: adapter, Tools: WorkspaceTools(Sandbox{Mode: SandboxReadOnly}), ToolExecutor: executor})
			if err != nil {
				t.Fatal(err)
			}
			var toolOutput string
			result, err := p.Execute(context.Background(), Request{ID: "scope", WorkingDir: working, Prompt: "read witness", Model: "test", Sandbox: Sandbox{Mode: SandboxReadOnly}, ReadableRoots: []string{sibling}}, func(event Event) error {
				if event.Type == EventStarted && scenario.swap {
					if err := os.Rename(sibling, sibling+"-old"); err != nil {
						return err
					}
					return os.Symlink(outside, sibling)
				}
				if event.Type == EventToolCompleted {
					toolOutput = event.Text
				}
				return nil
			})
			if err != nil || result.Outcome != OutcomeSucceeded {
				t.Fatalf("outcome=%+v err=%v", result, err)
			}
			if strings.Contains(toolOutput, "outside-secret") {
				t.Fatalf("read escaped its admitted root: %q", toolOutput)
			}
			if !scenario.swap && toolOutput != "authorized" {
				t.Fatalf("authorized read=%q", toolOutput)
			}
		})
	}
}

func TestAPIWorkspaceRefusesSymlinkAtAdmission(t *testing.T) {
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(base, "target")
	if err := os.Mkdir(target, 0700); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(base, "alias")
	if err := os.Symlink(target, alias); err != nil {
		t.Fatal(err)
	}
	p, err := NewAPIProvider(APIProviderConfig{Adapter: &fakeAPIAdapter{}, ToolExecutor: NewWorkspaceToolExecutor()})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := p.Execute(context.Background(), Request{ID: "refuse", WorkingDir: target, Prompt: "read", Model: "test", Sandbox: Sandbox{Mode: SandboxReadOnly}, ReadableRoots: []string{alias}}, nil); err == nil {
		t.Fatal("symlink admitted as a registered root")
	}
}
