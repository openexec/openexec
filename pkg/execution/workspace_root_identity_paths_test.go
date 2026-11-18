//go:build linux || darwin

package execution

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type identityFixture struct {
	working, selected, outside string
	identity                   ReadableRootIdentity
}

func newIdentityFixture(t *testing.T) identityFixture {
	t.Helper()
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	f := identityFixture{working: filepath.Join(base, "candidate"), selected: filepath.Join(base, "selected"), outside: filepath.Join(base, "unrelated")}
	for _, dir := range []string{f.working, f.selected, f.outside} {
		if err := os.Mkdir(dir, 0700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(f.selected, "witness"), []byte("authorized-needle"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(f.outside, "witness"), []byte("outside-secret-needle"), 0600); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(f.selected)
	if err != nil {
		t.Fatal(err)
	}
	if f.identity, err = ReadableRootIdentityOf(f.selected, info); err != nil {
		t.Fatal(err)
	}
	return f
}

func (f identityFixture) swapToOutside(t *testing.T) {
	t.Helper()
	if err := os.Rename(f.selected, f.selected+"-old"); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(f.outside, f.selected); err != nil {
		t.Fatal(err)
	}
}

func (f identityFixture) call(t *testing.T, executor ToolExecutor, tool string) (string, error) {
	t.Helper()
	input := map[string]string{"path": filepath.Join(f.selected, "witness")}
	switch tool {
	case "list_directory":
		input = map[string]string{"path": f.selected}
	case "search_files":
		input = map[string]string{"path": f.selected, "text": "needle"}
	}
	raw, _ := json.Marshal(input)
	return executor.ExecuteTool(context.Background(), ToolRequest{Name: tool, Input: raw, WorkingDir: f.working, Sandbox: Sandbox{Mode: SandboxReadOnly}, ReadableRoots: []string{f.selected}})
}

type rootPinner interface {
	pinReadableRoots([]string, []ReadableRootIdentity) (ToolExecutor, func(), error)
}

// A valid root works, and after a post-admission swap the pinned descriptor
// still serves the original directory for read, list and search, directly and
// through the composite executor, never the swapped-in outside one.
func TestPinnedReadableRootServesOriginalForReadListSearch(t *testing.T) {
	for _, composite := range []bool{false, true} {
		for _, tool := range []string{"read_file", "list_directory", "search_files"} {
			name := tool
			if composite {
				name += "/composite"
			}
			t.Run(name, func(t *testing.T) {
				f := newIdentityFixture(t)
				var base ToolExecutor = NewWorkspaceToolExecutor()
				if composite {
					var err error
					base, err = NewCompositeToolExecutor(base, WorkspaceTools(Sandbox{Mode: SandboxReadOnly}), &GatewayToolExecutor{})
					if err != nil {
						t.Fatal(err)
					}
				}
				pinned, closeRoots, err := base.(rootPinner).pinReadableRoots([]string{f.selected}, []ReadableRootIdentity{f.identity})
				if err != nil {
					t.Fatal(err)
				}
				defer closeRoots()
				before, err := f.call(t, pinned, tool)
				if err != nil || !strings.Contains(before, "witness") && !strings.Contains(before, "authorized-needle") {
					t.Fatalf("valid root: %q %v", before, err)
				}
				f.swapToOutside(t)
				after, err := f.call(t, pinned, tool)
				if strings.Contains(after, "outside-secret") {
					t.Fatalf("%s disclosed outside data after swap: %q", tool, after)
				}
				// Either the original directory is served or the call is refused
				// (the post-open path verification sees the swap); never other data.
				if err == nil && after != before {
					t.Fatalf("pinned %s changed after swap: %q want %q", tool, after, before)
				}
			})
		}
	}
}

func TestPinReadableRootsRefusesBadIdentities(t *testing.T) {
	f := newIdentityFixture(t)
	wrongInode := f.identity
	wrongInode.Inode++
	wrongDevice := f.identity
	wrongDevice.Device++
	otherPath := f.identity
	otherPath.Path = f.outside
	for _, tc := range []struct {
		name       string
		roots      []string
		identities []ReadableRootIdentity
		want       string
	}{
		{"wrong inode", []string{f.selected}, []ReadableRootIdentity{wrongInode}, "changed since caller admission"},
		{"wrong device", []string{f.selected}, []ReadableRootIdentity{wrongDevice}, "changed since caller admission"},
		{"identity for a different path", []string{f.selected}, []ReadableRootIdentity{otherPath}, "changed since caller admission"},
		{"duplicate identity", []string{f.selected}, []ReadableRootIdentity{f.identity, f.identity}, "duplicate readable root identity"},
		{"missing identity for second root", []string{f.selected, f.outside}, []ReadableRootIdentity{f.identity}, "do not match roots"},
		{"relative root", []string{"selected"}, []ReadableRootIdentity{{Path: "selected"}}, "not a stable canonical directory"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, closeRoots, err := NewWorkspaceToolExecutor().pinReadableRoots(tc.roots, tc.identities)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err=%v want %q", err, tc.want)
			}
			if closeRoots != nil {
				t.Fatal("close function returned alongside an error")
			}
		})
	}
}

// A root reached through a symlinked parent still names the same directory
// (so SameFile alone passes), but its path is not canonical and must be refused.
func TestPinReadableRootsRefusesSymlinkedParent(t *testing.T) {
	f := newIdentityFixture(t)
	alias := filepath.Join(filepath.Dir(f.selected), "alias")
	if err := os.Symlink(filepath.Dir(f.selected), alias); err != nil {
		t.Fatal(err)
	}
	viaAlias := filepath.Join(alias, filepath.Base(f.selected))
	identity := f.identity
	identity.Path = viaAlias
	_, closeRoots, err := NewWorkspaceToolExecutor().pinReadableRoots([]string{viaAlias}, []ReadableRootIdentity{identity})
	if err == nil || !strings.Contains(err.Error(), "not a stable canonical directory") || closeRoots != nil {
		t.Fatalf("err=%v", err)
	}
}

// Near miss: a correct identity for the exact root pins cleanly.
func TestPinReadableRootsAcceptsMatchingIdentity(t *testing.T) {
	f := newIdentityFixture(t)
	_, closeRoots, err := NewWorkspaceToolExecutor().pinReadableRoots([]string{f.selected}, []ReadableRootIdentity{f.identity})
	if err != nil {
		t.Fatal(err)
	}
	closeRoots()
}

func TestAPIProviderRefusesIdentitiesItCannotBind(t *testing.T) {
	f := newIdentityFixture(t)
	p, err := NewAPIProvider(APIProviderConfig{Adapter: &fakeAPIAdapter{}, ToolExecutor: unpinnedExecutor{}})
	if err != nil {
		t.Fatal(err)
	}
	if p.BoundReadableRootsAvailable() {
		t.Fatal("executor without pinning reported bound-root support")
	}
	_, err = p.Execute(context.Background(), Request{WorkingDir: f.working, Prompt: "x", Model: "test", Sandbox: Sandbox{Mode: SandboxReadOnly}, ReadableRoots: []string{f.selected}, ReadableRootIdentities: []ReadableRootIdentity{f.identity}}, nil)
	if err == nil || !strings.Contains(err.Error(), "cannot bind") {
		t.Fatalf("err=%v", err)
	}
	pinning, err := NewAPIProvider(APIProviderConfig{Adapter: &fakeAPIAdapter{}, ToolExecutor: NewWorkspaceToolExecutor()})
	if err != nil {
		t.Fatal(err)
	}
	if !pinning.BoundReadableRootsAvailable() {
		t.Fatal("workspace executor should support bound roots")
	}
	_, err = pinning.Execute(context.Background(), Request{WorkingDir: f.working, Prompt: "x", Model: "test", Sandbox: Sandbox{Mode: SandboxReadOnly}, ReadableRootIdentities: []ReadableRootIdentity{f.identity}}, nil)
	if err == nil || !strings.Contains(err.Error(), "without roots") {
		t.Fatalf("err=%v", err)
	}
}

// A composite whose workspace cannot pin refuses; a pinning one is the near miss.
func TestCompositeRefusesWorkspaceThatCannotPin(t *testing.T) {
	f := newIdentityFixture(t)
	build := func(workspace ToolExecutor) *CompositeToolExecutor {
		composite, err := NewCompositeToolExecutor(workspace, WorkspaceTools(Sandbox{Mode: SandboxReadOnly}), &GatewayToolExecutor{})
		if err != nil {
			t.Fatal(err)
		}
		return composite
	}
	_, closeRoots, err := build(unpinnedExecutor{}).pinReadableRoots([]string{f.selected}, []ReadableRootIdentity{f.identity})
	if err == nil || !strings.Contains(err.Error(), "cannot pin") || closeRoots != nil {
		t.Fatalf("err=%v", err)
	}
	_, closeRoots, err = build(NewWorkspaceToolExecutor()).pinReadableRoots([]string{f.selected}, []ReadableRootIdentity{f.identity})
	if err != nil {
		t.Fatal(err)
	}
	closeRoots()
}

type unpinnedExecutor struct{}

func (unpinnedExecutor) ExecuteTool(context.Context, ToolRequest) (string, error) { return "", nil }
func (unpinnedExecutor) ValidateAccess(string, Sandbox, []string) error           { return nil }
