//go:build linux || darwin

package execution

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
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

// A swap that is undone before the post-operation name check is refused by the
// identity watch bound at the descriptor operation, with the typed refusal and
// no bytes or paths. "no-wait" restores the name inside one kernel timestamp
// tick, which a change-time comparison cannot see; "cross-tick" holds the swap
// open across a tick.
func TestPinnedRootTransientSwapServesOnlyAdmittedDirectory(t *testing.T) {
	for _, hold := range []time.Duration{0, 50 * time.Millisecond} {
		name := "no-wait"
		if hold > 0 {
			name = "cross-tick"
		}
		t.Run(name, func(t *testing.T) {
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
				time.Sleep(hold)
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
			if err == nil {
				t.Fatalf("transient swap-and-restore was served: out=%q", out)
			}
			if !errors.Is(err, errPinnedRootChanged) {
				t.Fatalf("refusal is not the typed root-changed refusal: %v", err)
			}
			if out != "" {
				t.Fatalf("refusal still produced output %q", out)
			}
			for _, leak := range []string{"authorized-needle", "outside-secret-needle", f.outside, f.selected, f.selected + "-old"} {
				if strings.Contains(out, leak) || strings.Contains(err.Error(), leak) {
					t.Fatalf("refusal disclosed %q: out=%q err=%v", leak, out, err)
				}
			}
		})
	}
}

// Near miss: the same pinned read with no swap succeeds, including after
// unrelated activity in the parent directory.
func TestPinnedRootUnswappedReadSucceedsAfterSiblingActivity(t *testing.T) {
	f := newIdentityFixture(t)
	pinned, closeRoots, err := NewWorkspaceToolExecutor().pinReadableRoots([]string{f.selected}, []ReadableRootIdentity{f.identity})
	if err != nil {
		t.Fatal(err)
	}
	defer closeRoots()
	prev := afterPinnedRootCheckHook
	t.Cleanup(func() { afterPinnedRootCheckHook = prev })
	afterPinnedRootCheckHook = func(string) {
		if err := os.WriteFile(f.selected+"-sibling", []byte("x"), 0600); err != nil {
			t.Error(err)
		}
	}
	out, err := f.call(t, pinned, "read_file")
	if err != nil || out != "authorized-needle" {
		t.Fatalf("unswapped read refused: out=%q err=%v", out, err)
	}
}
