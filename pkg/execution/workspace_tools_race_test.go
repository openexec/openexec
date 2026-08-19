package execution

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// Reads must not reach the configuration that holds this feature's own
// credentials, even though it sits inside the checkout by construction.
func TestWorkspaceToolsRefuseWorkspaceConfiguration(t *testing.T) {
	root, _ := workspace(t)
	for _, directory := range []string{".openexec", ".uaos", ".git"} {
		if err := os.MkdirAll(filepath.Join(root, directory), 0o755); err != nil {
			t.Fatal(err)
		}
		secret := filepath.Join(root, directory, "config.json")
		if err := os.WriteFile(secret, []byte(`{"api_key":"sk-live-do-not-leak"}`), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	executor := NewWorkspaceToolExecutor()

	for _, path := range []string{
		".openexec/config.json",
		".uaos/project.json",
		".git/config",
		filepath.Join(root, ".openexec", "config.json"),
		"./.openexec/../.openexec/config.json",
	} {
		out, err := executor.ExecuteTool(context.Background(),
			readRequest(root, path, Sandbox{Mode: SandboxReadOnly}, nil))
		if err == nil {
			t.Errorf("read %q was allowed, returning %q", path, out)
		}
		if strings.Contains(out, "sk-live") {
			t.Fatalf("credential returned for %q", path)
		}
	}

	// The policy follows the opened target, not only the path spelling. These
	// aliases are pre-existing checkout content; write_file itself has no
	// symlink operation.
	if err := os.Symlink(".openexec/config.json", filepath.Join(root, "notes.txt")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(".openexec", filepath.Join(root, "project-notes")); err != nil {
		t.Fatal(err)
	}
	if out, err := executor.ExecuteTool(context.Background(),
		readRequest(root, "notes.txt", Sandbox{Mode: SandboxReadOnly}, nil)); err == nil || strings.Contains(out, "sk-live") {
		t.Fatalf("read through denied symlink returned %q, err=%v", out, err)
	}
	listInput, _ := json.Marshal(map[string]string{"path": "project-notes"})
	if out, err := executor.ExecuteTool(context.Background(), ToolRequest{
		Name: "list_directory", Input: listInput, WorkingDir: root, Sandbox: Sandbox{Mode: SandboxReadOnly},
	}); err == nil || strings.Contains(out, "config.json") {
		t.Fatalf("list through denied symlink returned %q, err=%v", out, err)
	}
	writeInput, _ := json.Marshal(map[string]string{"path": "notes.txt", "content": "overwritten"})
	if out, err := executor.ExecuteTool(context.Background(), ToolRequest{
		Name: "write_file", Input: writeInput, WorkingDir: root, Sandbox: Sandbox{Mode: SandboxWorkspaceWrite},
		WritableRoots: []string{root},
	}); err == nil {
		t.Fatalf("write through denied symlink returned %q", out)
	}
	secret, err := os.ReadFile(filepath.Join(root, ".openexec", "config.json"))
	if err != nil || !strings.Contains(string(secret), "sk-live-do-not-leak") {
		t.Fatalf("refused symlink write changed credential file: %q, err=%v", secret, err)
	}
	if err := os.Symlink(".git", filepath.Join(root, "git-alias")); err != nil {
		t.Fatal(err)
	}
	lockInput, _ := json.Marshal(map[string]any{"path": "git-alias/index.lock", "content": "x"})
	if _, err := executor.ExecuteTool(context.Background(), ToolRequest{
		Name: "write_file", Input: lockInput, WorkingDir: root, Sandbox: Sandbox{Mode: SandboxWorkspaceWrite},
		WritableRoots: []string{root},
	}); err == nil {
		t.Fatal("write through a denied parent symlink was allowed")
	}
	if _, err := os.Lstat(filepath.Join(root, ".git", "index.lock")); !os.IsNotExist(err) {
		t.Fatalf("refused write left .git/index.lock behind: %v", err)
	}
	nestedInput, _ := json.Marshal(map[string]any{
		"path": "project-notes/x/y.txt", "content": "x", "create_directories": true,
	})
	if _, err := executor.ExecuteTool(context.Background(), ToolRequest{
		Name: "write_file", Input: nestedInput, WorkingDir: root, Sandbox: Sandbox{Mode: SandboxWorkspaceWrite},
		WritableRoots: []string{root},
	}); err == nil {
		t.Fatal("directory creation through a denied parent symlink was allowed")
	}
	if _, err := os.Lstat(filepath.Join(root, ".openexec", "x")); !os.IsNotExist(err) {
		t.Fatalf("refused write created a directory in .openexec: %v", err)
	}

	// Listing the checkout still works; only entering the directory is refused.
	if _, err := executor.ExecuteTool(context.Background(),
		readRequest(root, "main.go", Sandbox{Mode: SandboxReadOnly}, nil)); err != nil {
		t.Errorf("ordinary read broke: %v", err)
	}
}

func TestWorkspaceToolsIgnoreDeniedNamesAboveTheGrantedRoot(t *testing.T) {
	base := t.TempDir()
	root := filepath.Join(base, ".openexec", "projects", "ordinary-checkout")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "main.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	executor := NewWorkspaceToolExecutor()
	if out, err := executor.ExecuteTool(context.Background(),
		readRequest(root, "main.go", Sandbox{Mode: SandboxReadOnly}, nil)); err != nil || !strings.Contains(out, "package main") {
		t.Fatalf("ordinary read under a denied-named ancestor = %q, %v", out, err)
	}
	input, _ := json.Marshal(map[string]string{"path": "new.txt", "content": "written"})
	if out, err := executor.ExecuteTool(context.Background(), ToolRequest{
		Name: "write_file", Input: input, WorkingDir: root, Sandbox: Sandbox{Mode: SandboxWorkspaceWrite},
		WritableRoots: []string{root},
	}); err != nil {
		t.Fatalf("ordinary write under a denied-named ancestor = %q, %v", out, err)
	}
}

// A read-only session carries no writable roots, so without readable ones a
// reviewer can see only its own checkout — while the console has already told
// it otherwise.
func TestWorkspaceToolsHonourReadableRoots(t *testing.T) {
	root, sibling := workspace(t)
	if err := os.WriteFile(filepath.Join(sibling, "api.go"), []byte("package api\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	executor := NewWorkspaceToolExecutor()

	denied := readRequest(root, filepath.Join(sibling, "api.go"), Sandbox{Mode: SandboxReadOnly}, nil)
	if _, err := executor.ExecuteTool(context.Background(), denied); err == nil {
		t.Fatal("an ungranted sibling was readable")
	}

	granted := denied
	granted.ReadableRoots = []string{sibling}
	out, err := executor.ExecuteTool(context.Background(), granted)
	if err != nil {
		t.Fatalf("a granted sibling was not readable: %v", err)
	}
	if !strings.Contains(out, "package api") {
		t.Errorf("read returned %q", out)
	}

	// Readable is not writable, in either mode.
	write, _ := json.Marshal(map[string]any{"path": filepath.Join(sibling, "api.go"), "content": "x"})
	attempt := ToolRequest{
		Name: "write_file", Input: write, WorkingDir: root,
		Sandbox: Sandbox{Mode: SandboxWorkspaceWrite}, WritableRoots: []string{root},
		ReadableRoots: []string{sibling},
	}
	if _, err := executor.ExecuteTool(context.Background(), attempt); err == nil {
		t.Error("a read-only grant was written through")
	}
}

// The escape that string comparison cannot stop: the path is legal when
// checked and points elsewhere when opened. os.Root resolves beneath a held
// descriptor, so there is no window between the two.
//
// The loop is the test. A single attempt proves nothing about a race, and a
// failure here is a real escape rather than a flake — the assertion is that
// no iteration ever reads the file outside the root.
func TestWorkspaceToolsSurviveSymlinkSwapRace(t *testing.T) {
	root, outside := workspace(t)
	executor := NewWorkspaceToolExecutor()

	inside := filepath.Join(root, "swap")
	if err := os.MkdirAll(inside, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(inside, "token"), []byte("harmless\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	stop := make(chan struct{})
	var swapping sync.WaitGroup
	swapping.Add(1)
	go func() {
		defer swapping.Done()
		link := filepath.Join(root, "swap-link")
		for {
			select {
			case <-stop:
				return
			default:
			}
			_ = os.Remove(link)
			_ = os.Symlink(inside, link)
			_ = os.Remove(link)
			_ = os.Symlink(outside, link)
		}
	}()

	var escapes int
	for attempt := 0; attempt < 3000; attempt++ {
		out, err := executor.ExecuteTool(context.Background(),
			readRequest(root, "swap-link/token", Sandbox{Mode: SandboxReadOnly}, nil))
		if err == nil && strings.Contains(out, "s3cret") {
			escapes++
		}
	}
	close(stop)
	swapping.Wait()

	if escapes > 0 {
		t.Fatalf("%d reads escaped the root through a swapped symlink", escapes)
	}
}
