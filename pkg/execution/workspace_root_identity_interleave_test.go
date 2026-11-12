//go:build linux || darwin

package execution

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The demonstrated no-proc interleaving: a name that is a symlink to the
// protected config is opened (the descriptor is the protected file), then the
// name is swapped for an ordinary file while the check looks at it, and swapped
// back to the symlink when the check looks again. A verification built from a
// separate Lstat and Stat of the final path is satisfied by that sequence and
// blesses the protected descriptor. The production walk opens every component
// once with no-follow beneath the root descriptor; the hook runs the swap at
// the point the walk touches the name, through the production traversal.
func TestNoProcInterleavedLstatStatSwapIsRefused(t *testing.T) {
	for _, tc := range []struct {
		name    string
		symlink bool // state of "link" at the moment the walk opens it
	}{
		{"ordinary-file-at-open", false},
		{"symlink-at-open", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			prevPath, prevHook := descriptorPath, noProcWalkHook
			t.Cleanup(func() { descriptorPath, noProcWalkHook = prevPath, prevHook })
			descriptorPath = func(*os.File) (string, bool) { return "", false }

			f := newIdentityFixture(t)
			protected := filepath.Join(f.selected, ".openexec")
			if err := os.Mkdir(protected, 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(protected, "config"), []byte("protected-needle"), 0600); err != nil {
				t.Fatal(err)
			}
			replaceLink(t, f.selected, true)

			calls := 0
			noProcWalkHook = func(component string) {
				calls++
				if component != "link" {
					t.Errorf("walk opened %q, want link", component)
				}
				replaceLink(t, f.selected, tc.symlink)
			}
			out, err := NewWorkspaceToolExecutor().ExecuteTool(t.Context(), ToolRequest{Name: "read_file", Input: []byte(`{"path":"` + filepath.Join(f.selected, "link") + `"}`), WorkingDir: f.working, Sandbox: Sandbox{Mode: SandboxReadOnly}, ReadableRoots: []string{f.selected}})
			if calls == 0 {
				t.Fatal("production walk was never reached")
			}
			if err == nil {
				t.Fatalf("protected target served through interleaved swap: out=%q", out)
			}
			for _, leak := range []string{"protected-needle", "outside-secret", f.outside, protected} {
				if strings.Contains(out, leak) || strings.Contains(err.Error(), leak) {
					t.Fatalf("refusal disclosed %q: out=%q err=%v", leak, out, err)
				}
			}
		})
	}
}

func replaceLink(t *testing.T, selected string, symlink bool) {
	t.Helper()
	link := filepath.Join(selected, "link")
	if err := os.Remove(link); err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	if symlink {
		if err := os.Symlink(filepath.Join(".openexec", "config"), link); err != nil {
			t.Fatal(err)
		}
		return
	}
	if err := os.WriteFile(link, []byte("innocent"), 0600); err != nil {
		t.Fatal(err)
	}
}

// Near miss: a plain file under a plain directory is read normally without /proc.
func TestNoProcPlainFileStillReadable(t *testing.T) {
	prev := descriptorPath
	t.Cleanup(func() { descriptorPath = prev })
	descriptorPath = func(*os.File) (string, bool) { return "", false }
	f := newIdentityFixture(t)
	if err := os.MkdirAll(filepath.Join(f.selected, "sub"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(f.selected, "sub", "ok"), []byte("plain-needle"), 0600); err != nil {
		t.Fatal(err)
	}
	out, err := NewWorkspaceToolExecutor().ExecuteTool(t.Context(), ToolRequest{Name: "read_file", Input: []byte(`{"path":"` + filepath.Join(f.selected, "sub", "ok") + `"}`), WorkingDir: f.working, Sandbox: Sandbox{Mode: SandboxReadOnly}, ReadableRoots: []string{f.selected}})
	if err != nil || !strings.Contains(out, "plain-needle") {
		t.Fatalf("plain file refused: out=%q err=%v", out, err)
	}
}
