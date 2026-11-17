//go:build linux || darwin

package execution

import (
	"strings"
	"testing"
)

// A swap landing after the pre-operation root check and before the operation
// must be refused, not answered from the old descriptor. The hook runs inside
// the production withinRoots, in that window. The unswapped run is the near
// miss: the same call serves the admitted root.
func TestRootSwapBetweenCheckAndOperationIsRefused(t *testing.T) {
	for _, tool := range []string{"read_file", "list_directory", "search_files"} {
		t.Run(tool, func(t *testing.T) {
			t.Cleanup(func() { rootUseHook = nil })

			f := newIdentityFixture(t)
			pinned, closeRoots, err := NewWorkspaceToolExecutor().pinReadableRoots([]string{f.selected}, []ReadableRootIdentity{f.identity})
			if err != nil {
				t.Fatal(err)
			}
			defer closeRoots()

			// Near miss: no swap, the admitted root is served.
			calls := 0
			rootUseHook = func() { calls++ }
			out, err := f.call(t, pinned, tool)
			if err != nil || !strings.Contains(out, "witness") && !strings.Contains(out, "authorized-needle") {
				t.Fatalf("unswapped valid root was not served: out=%q err=%v", out, err)
			}
			if calls == 0 {
				t.Fatal("the window between check and operation was never reached")
			}

			rootUseHook = func() { f.swapToOutside(t) }
			out, err = f.call(t, pinned, tool)
			if err == nil {
				t.Fatalf("swap between check and operation was served: out=%q", out)
			}
			if !strings.Contains(err.Error(), "changed after admission") {
				t.Fatalf("refusal is not the root-changed refusal: %v", err)
			}
			for _, leak := range []string{"outside-secret-needle", "authorized-needle", f.outside, f.selected, "unrelated"} {
				if strings.Contains(out, leak) || strings.Contains(err.Error(), leak) {
					t.Fatalf("refusal disclosed %q: out=%q err=%v", leak, out, err)
				}
			}
		})
	}
}
