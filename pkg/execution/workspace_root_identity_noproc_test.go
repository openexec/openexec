//go:build linux || darwin

package execution

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Without /proc (macOS, restricted containers) the denied-component check has
// no kernel name for the pinned directory. A swap must not let the check judge
// the original by the replacement: the original holds `link -> .openexec/...`
// and the replacement holds an innocent file of the same name. Reading `link`
// through the pinned descriptor opens the protected file, so it must be refused.
func TestNoProcSwapPlusSymlinkDoesNotBlessProtectedTarget(t *testing.T) {
	prev := descriptorPath
	t.Cleanup(func() { descriptorPath = prev })
	descriptorPath = func(*os.File) (string, bool) { return "", false }

	f := newIdentityFixture(t)
	protected := filepath.Join(f.selected, ".openexec")
	if err := os.Mkdir(protected, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(protected, "config"), []byte("protected-needle"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(".openexec", "config"), filepath.Join(f.selected, "link")); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(f.selected)
	if err != nil {
		t.Fatal(err)
	}
	identity, err := ReadableRootIdentityOf(f.selected, info)
	if err != nil {
		t.Fatal(err)
	}
	pinned, closeRoots, err := NewWorkspaceToolExecutor().pinReadableRoots([]string{f.selected}, []ReadableRootIdentity{identity})
	if err != nil {
		t.Fatal(err)
	}
	defer closeRoots()

	read := func() (string, error) {
		return pinned.ExecuteTool(t.Context(), ToolRequest{Name: "read_file", Input: []byte(`{"path":"` + filepath.Join(f.selected, "link") + `"}`), WorkingDir: f.working, Sandbox: Sandbox{Mode: SandboxReadOnly}, ReadableRoots: []string{f.selected}})
	}

	// Near miss: unswapped, the protected target is refused by name.
	if out, err := read(); err == nil || strings.Contains(out, "protected-needle") {
		t.Fatalf("unswapped protected symlink was served: out=%q err=%v", out, err)
	}

	// Swap: the path now names a directory whose `link` is an ordinary file.
	if err := os.Rename(f.selected, f.selected+"-old"); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(f.selected, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(f.selected, "link"), []byte("replacement"), 0600); err != nil {
		t.Fatal(err)
	}
	out, err := read()
	if err == nil {
		t.Fatalf("protected target served through a swapped root: out=%q", out)
	}
	for _, leak := range []string{"protected-needle", "outside-secret", f.outside} {
		if strings.Contains(out, leak) || strings.Contains(err.Error(), leak) {
			t.Fatalf("refusal disclosed %q: out=%q err=%v", leak, out, err)
		}
	}
}

// Without /proc the opened file must still be the object the root names: a
// plain file swapped for another after open is refused, and an unchanged one
// is accepted (near miss).
func TestNoProcRefusesFileReplacedAfterOpen(t *testing.T) {
	prev := descriptorPath
	t.Cleanup(func() { descriptorPath = prev })
	descriptorPath = func(*os.File) (string, bool) { return "", false }

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "note"), []byte("original"), 0600); err != nil {
		t.Fatal(err)
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	file, err := root.Open("note")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if err := refuseDeniedOpenedTarget(root, file, "note", "note"); err != nil {
		t.Fatalf("unchanged plain file refused: %v", err)
	}
	if err := os.Remove(filepath.Join(dir, "note")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "note"), []byte("replacement"), 0600); err != nil {
		t.Fatal(err)
	}
	err = refuseDeniedOpenedTarget(root, file, "note", "note")
	if err == nil || strings.Contains(err.Error(), dir) {
		t.Fatalf("replaced file served or error leaks path: %v", err)
	}
}
