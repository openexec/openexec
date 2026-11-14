//go:build linux || darwin

package execution

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func noProc(t *testing.T) {
	t.Helper()
	prev := descriptorPath
	t.Cleanup(func() { descriptorPath = prev })
	descriptorPath = func(*os.File) (string, bool) { return "", false }
}

// Without /proc (macOS, restricted containers) the denied-component check has
// no kernel name for the pinned directory. A swap must not let the check judge
// the original by the replacement: the original holds `link -> .openexec/...`
// and the replacement holds an innocent file of the same name. Reading `link`
// through the pinned descriptor opens the protected file, so it must be refused.
func TestNoProcSwapPlusSymlinkDoesNotBlessProtectedTarget(t *testing.T) {
	noProc(t)

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

// The race the Lstat/Stat pairing could not close: the descriptor is opened on a
// symlink that reaches the protected file, and the name is then flipped between
// an ordinary file and the symlink around the check. The decision rests on the
// descriptor's identity, so it is refused in every state of the name, and a
// plain file (near miss) is still accepted.
func TestNoProcRefusesProtectedDescriptorWhateverTheNameNowIs(t *testing.T) {
	noProc(t)

	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, ".openexec"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".openexec", "config"), []byte("protected-needle"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "plain"), []byte("ok"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(".openexec", "config"), filepath.Join(dir, "link")); err != nil {
		t.Fatal(err)
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()

	plain, err := root.Open("plain")
	if err != nil {
		t.Fatal(err)
	}
	defer plain.Close()
	if err := refuseDeniedOpenedTarget(root, plain, "plain", "plain"); err != nil {
		t.Fatalf("plain file refused: %v", err)
	}

	file, err := root.Open("link")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	check := func(state string) {
		t.Helper()
		err := refuseDeniedOpenedTarget(root, file, "link", "link")
		if err == nil {
			t.Fatalf("protected descriptor served while name is %s", state)
		}
		if strings.Contains(err.Error(), dir) || strings.Contains(err.Error(), "protected-needle") {
			t.Fatalf("refusal discloses data in state %s: %v", state, err)
		}
	}
	check("the symlink")
	if err := os.Remove(filepath.Join(dir, "link")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "link"), []byte("ordinary"), 0600); err != nil {
		t.Fatal(err)
	}
	check("an ordinary file")
	if err := os.Remove(filepath.Join(dir, "link")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(".openexec", "config"), filepath.Join(dir, "link")); err != nil {
		t.Fatal(err)
	}
	check("the symlink again")
}

// A symlink carrying a protected name cannot be judged without /proc, so the
// check fails closed; an ordinary symlink elsewhere is a near miss and does
// not stop an unrelated plain file.
func TestNoProcFailsClosedOnProtectedNameSymlink(t *testing.T) {
	noProc(t)

	dir := t.TempDir()
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "plain"), []byte("ok"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(dir, "harmless")); err != nil {
		t.Fatal(err)
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	file, err := root.Open("plain")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if err := refuseDeniedOpenedTarget(root, file, "plain", "plain"); err != nil {
		t.Fatalf("near miss refused: %v", err)
	}
	if err := os.Symlink(outside, filepath.Join(dir, ".git")); err != nil {
		t.Fatal(err)
	}
	err = refuseDeniedOpenedTarget(root, file, "plain", "plain")
	if err == nil || strings.Contains(err.Error(), outside) {
		t.Fatalf("protected-name symlink accepted or error leaks path: %v", err)
	}
}
