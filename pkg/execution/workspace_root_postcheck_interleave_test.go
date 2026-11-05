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
