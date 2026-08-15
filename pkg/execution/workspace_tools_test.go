package execution

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// workspace builds a repository root with one file, plus a sibling directory
// outside every granted root to attempt escapes into.
func workspace(t *testing.T) (root, outside string) {
	t.Helper()
	base := t.TempDir()
	root = filepath.Join(base, "repo")
	outside = filepath.Join(base, "secrets")
	for _, dir := range []string{root, outside} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "main.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outside, "token"), []byte("s3cret\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	// t.TempDir() is under /tmp, which is a symlink on some systems; resolving
	// here keeps the test asserting containment rather than string equality.
	resolvedRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	resolvedOutside, err := filepath.EvalSymlinks(outside)
	if err != nil {
		t.Fatal(err)
	}
	return resolvedRoot, resolvedOutside
}

func writeRequest(root, path, content string, roots []string) ToolRequest {
	input, _ := json.Marshal(map[string]any{"path": path, "content": content})
	return ToolRequest{
		Name: "write_file", Input: input, WorkingDir: root,
		Sandbox: Sandbox{Mode: SandboxWorkspaceWrite}, WritableRoots: roots,
	}
}

func readRequest(root, path string, sandbox Sandbox, roots []string) ToolRequest {
	input, _ := json.Marshal(map[string]any{"path": path})
	return ToolRequest{
		Name: "read_file", Input: input, WorkingDir: root,
		Sandbox: sandbox, WritableRoots: roots,
	}
}

func TestWorkspaceToolsReadAndWriteInsideRoot(t *testing.T) {
	root, _ := workspace(t)
	executor := NewWorkspaceToolExecutor()

	out, err := executor.ExecuteTool(context.Background(),
		readRequest(root, "main.go", Sandbox{Mode: SandboxReadOnly}, nil))
	if err != nil {
		t.Fatalf("read inside the working directory: %v", err)
	}
	if !strings.Contains(out, "package main") {
		t.Errorf("read returned %q", out)
	}

	if _, err := executor.ExecuteTool(context.Background(),
		writeRequest(root, "new.go", "package new\n", []string{root})); err != nil {
		t.Fatalf("write inside a writable root: %v", err)
	}
	written, err := os.ReadFile(filepath.Join(root, "new.go"))
	if err != nil || string(written) != "package new\n" {
		t.Fatalf("file = %q, err = %v", written, err)
	}
}

// Each case is an escape the containment check exists to stop. Remove
// resolution or the root check in containedPath and these pass.
func TestWorkspaceToolsRefuseEscapes(t *testing.T) {
	root, outside := workspace(t)
	executor := NewWorkspaceToolExecutor()

	linked := filepath.Join(root, "link")
	if err := os.Symlink(outside, linked); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name    string
		request ToolRequest
	}{
		{"absolute path outside", readRequest(root, filepath.Join(outside, "token"), Sandbox{Mode: SandboxReadOnly}, nil)},
		{"parent traversal", readRequest(root, "../secrets/token", Sandbox{Mode: SandboxReadOnly}, nil)},
		{"symlinked directory out", readRequest(root, "link/token", Sandbox{Mode: SandboxReadOnly}, nil)},
		{"write through traversal", writeRequest(root, "../secrets/owned", "x", []string{root})},
		{"write behind a symlink", writeRequest(root, "link/owned", "x", []string{root})},
		{"write to an absolute path outside", writeRequest(root, filepath.Join(outside, "owned"), "x", []string{root})},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			if _, err := executor.ExecuteTool(context.Background(), test.request); err == nil {
				t.Fatal("escape was allowed")
			}
		})
	}
	if entries, err := os.ReadDir(outside); err == nil {
		for _, entry := range entries {
			if entry.Name() == "owned" {
				t.Fatal("a refused write still landed outside the roots")
			}
		}
	}
}

// The model is not offered write_file in read-only mode, and inventing the
// call anyway must not write.
func TestWorkspaceToolsRefuseWriteInReadOnly(t *testing.T) {
	root, _ := workspace(t)
	request := writeRequest(root, "sneaky.go", "package x\n", nil)
	request.Sandbox = Sandbox{Mode: SandboxReadOnly}

	if _, err := NewWorkspaceToolExecutor().ExecuteTool(context.Background(), request); err == nil {
		t.Fatal("write_file succeeded in read-only mode")
	}
	if _, err := os.Stat(filepath.Join(root, "sneaky.go")); !os.IsNotExist(err) {
		t.Fatal("read-only mode produced a file")
	}
}

// The working directory is readable because the run happens there. That is not
// a grant to write into it.
func TestWorkspaceToolsWorkingDirectoryIsNotWritable(t *testing.T) {
	root, outside := workspace(t)
	request := writeRequest(root, "in-workdir.go", "package x\n", []string{outside})

	if _, err := NewWorkspaceToolExecutor().ExecuteTool(context.Background(), request); err == nil {
		t.Fatal("wrote into the working directory without it being a writable root")
	}
}

func TestWorkspaceToolsAdvertiseByMode(t *testing.T) {
	readOnly := WorkspaceTools(Sandbox{Mode: SandboxReadOnly})
	for _, tool := range readOnly {
		if tool.Name == "write_file" {
			t.Fatal("read-only sessions were offered write_file")
		}
	}
	var found bool
	for _, tool := range WorkspaceTools(Sandbox{Mode: SandboxWorkspaceWrite}) {
		if tool.Name == "write_file" {
			found = true
		}
	}
	if !found {
		t.Fatal("workspace-write sessions were not offered write_file")
	}
	for _, tool := range WorkspaceTools(Sandbox{Mode: SandboxWorkspaceWrite}) {
		if tool.Name == "run_shell_command" {
			t.Fatal("a shell tool is offered; that is a separate decision with its own approval path")
		}
	}
}

func TestWorkspaceToolsValidateAccessFailsClosed(t *testing.T) {
	root, _ := workspace(t)
	executor := NewWorkspaceToolExecutor()

	if err := executor.ValidateAccess("", Sandbox{Mode: SandboxReadOnly}, nil); err == nil {
		t.Error("empty working directory was accepted")
	}
	if err := executor.ValidateAccess(filepath.Join(root, "absent"), Sandbox{Mode: SandboxReadOnly}, nil); err == nil {
		t.Error("a working directory that does not exist was accepted")
	}
	if err := executor.ValidateAccess(root, Sandbox{Mode: SandboxWorkspaceWrite}, nil); err == nil {
		t.Error("workspace-write with no writable root was accepted")
	}
	if err := executor.ValidateAccess(root, Sandbox{Mode: SandboxWorkspaceWrite},
		[]string{filepath.Join(root, "absent")}); err == nil {
		t.Error("a writable root that does not exist was accepted")
	}
	if err := executor.ValidateAccess(root, Sandbox{Mode: "yolo"}, nil); err == nil {
		t.Error("an unknown sandbox mode was accepted")
	}
}

// An API provider with this executor satisfies the guard that refuses
// workspace-write without a bounded one.
func TestAPIProviderAcceptsWorkspaceWriteWithExecutor(t *testing.T) {
	provider, err := NewAPIProvider(APIProviderConfig{
		Adapter:      &fakeAPIAdapter{},
		Tools:        WorkspaceTools(Sandbox{Mode: SandboxWorkspaceWrite}),
		ToolExecutor: NewWorkspaceToolExecutor(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if !provider.Descriptor().Capabilities.WorkspaceWrite {
		t.Error("descriptor denies workspace-write despite a bounded executor")
	}
}
