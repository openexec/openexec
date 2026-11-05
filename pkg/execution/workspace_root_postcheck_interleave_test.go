//go:build linux || darwin

package execution

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The root is swapped after the pinned-root name check passed and before the
// descriptor operation completes. The operation must be refused, with neither
// the original data nor any path in the output or error.
func TestPinnedRootSwappedAfterNameCheckIsRefused(t *testing.T) {
	for _, swap := range []bool{false, true} {
		name := "unswapped-near-miss"
		if swap {
			name = "swapped-after-name-check"
		}
		t.Run(name, func(t *testing.T) {
			prev := afterPinnedRootCheckHook
			t.Cleanup(func() { afterPinnedRootCheckHook = prev })
			f := newIdentityFixture(t)
			hookCalls := 0
			afterPinnedRootCheckHook = func(string) {
				hookCalls++
				if swap {
					f.swapToOutside(t)
				}
			}
			pinned, closeRoots, err := NewWorkspaceToolExecutor().pinReadableRoots([]string{f.selected}, []ReadableRootIdentity{f.identity})
			if err != nil {
				t.Fatal(err)
			}
			defer closeRoots()
			out, err := pinned.ExecuteTool(t.Context(), ToolRequest{Name: "read_file", Input: []byte(`{"path":"` + filepath.Join(f.selected, "witness") + `"}`), WorkingDir: f.working, Sandbox: Sandbox{Mode: SandboxReadOnly}, ReadableRoots: []string{f.selected}})
			if hookCalls == 0 {
				t.Fatal("pinned boundary was never reached")
			}
			if !swap {
				if err != nil || out != "authorized-needle" {
					t.Fatalf("valid read refused: out=%q err=%v", out, err)
				}
				return
			}
			if err == nil {
				t.Fatalf("swap served: out=%q", out)
			}
			for _, leak := range []string{"authorized-needle", "outside-secret-needle", f.outside, f.selected + "-old"} {
				if strings.Contains(out, leak) || strings.Contains(err.Error(), leak) {
					t.Fatalf("refusal disclosed %q: out=%q err=%v", leak, out, err)
				}
			}
			_ = os.Remove(f.selected)
		})
	}
}

// A swap that is undone before the post-operation check cannot be seen by any
// name check, and does not need to be: the operation runs against the
// descriptor pinned at admission, whose identity the first check proved. The
// served bytes therefore can only come from the admitted directory, never from
// the directory the transient swap named.
func TestPinnedRootTransientSwapServesOnlyAdmittedDirectory(t *testing.T) {
	prev := afterPinnedRootCheckHook
	t.Cleanup(func() { afterPinnedRootCheckHook = prev })
	f := newIdentityFixture(t)
	calls := 0
	afterPinnedRootCheckHook = func(string) {
		calls++
		f.swapToOutside(t)
		// Read through the swapped name to prove the swap was live.
		if data, err := os.ReadFile(filepath.Join(f.selected, "witness")); err != nil || string(data) != "outside-secret-needle" {
			t.Errorf("swap was not live: %q %v", data, err)
		}
		if err := os.Remove(f.selected); err != nil {
			t.Error(err)
		}
		if err := os.Rename(f.selected+"-old", f.selected); err != nil {
			t.Error(err)
		}
	}
	pinned, closeRoots, err := NewWorkspaceToolExecutor().pinReadableRoots([]string{f.selected}, []ReadableRootIdentity{f.identity})
	if err != nil {
		t.Fatal(err)
	}
	defer closeRoots()
	out, err := f.call(t, pinned, "read_file")
	if calls == 0 {
		t.Fatal("pinned boundary was never reached")
	}
	f.requireNoOutsideLeak(t, out)
	if err != nil {
		f.requireNoOutsideLeak(t, err.Error())
		return
	}
	if out != "authorized-needle" {
		t.Fatalf("transient swap changed the answer: %q", out)
	}
}
