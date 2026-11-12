//go:build linux || darwin

package execution

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The real unsupported-platform functions, driven on a platform that supports
// identity by pointing the seams at them. No fallback is enabled: a platform
// that cannot verify a directory identity must refuse, not skip the check.
func TestUnsupportedPlatformRefusesBoundRootsWithoutFallback(t *testing.T) {
	f := newIdentityFixture(t)
	info, err := os.Stat(f.selected)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := unsupportedReadableRootIdentity(f.selected, info); err == nil || !strings.Contains(err.Error(), "unsupported") {
		t.Fatalf("unsupported identity err=%v", err)
	}

	provider, err := NewAPIProvider(APIProviderConfig{Adapter: &fakeAPIAdapter{}, ToolExecutor: NewWorkspaceToolExecutor()})
	if err != nil {
		t.Fatal(err)
	}
	supported, identityOf := readableRootIdentitySupported, readableRootIdentityOf
	t.Cleanup(func() { readableRootIdentitySupported, readableRootIdentityOf = supported, identityOf })
	readableRootIdentitySupported = func() bool { return false }
	readableRootIdentityOf = unsupportedReadableRootIdentity

	if provider.BoundReadableRootsAvailable() {
		t.Fatal("provider advertised bound roots on a platform without identity")
	}
	// Even a caller that ignores the advertisement gets no unverified root.
	adapter := &fakeAPIAdapter{}
	provider, err = NewAPIProvider(APIProviderConfig{Adapter: adapter, ToolExecutor: NewWorkspaceToolExecutor()})
	if err != nil {
		t.Fatal(err)
	}
	_, err = provider.Execute(t.Context(), Request{WorkingDir: f.working, Prompt: "x", Model: "test", Sandbox: Sandbox{Mode: SandboxReadOnly}, ReadableRoots: []string{f.selected}, ReadableRootIdentities: []ReadableRootIdentity{f.identity}}, nil)
	if err == nil || !strings.Contains(err.Error(), "changed since caller admission") {
		t.Fatalf("root admitted without a verifiable identity: %v", err)
	}
	if len(adapter.requests) != 0 {
		t.Fatal("inference dispatched despite the refusal")
	}
	if strings.Contains(err.Error(), "outside-secret") || strings.Contains(err.Error(), f.outside) {
		t.Fatalf("refusal disclosed outside data: %v", err)
	}
}

// A symlink inside an admitted root that points outside it is not followed, on
// the pinned descriptor, directly and through the composite executor.
func TestPinnedRootDoesNotFollowSymlinkToOutside(t *testing.T) {
	for _, composite := range []bool{false, true} {
		f := newIdentityFixture(t)
		if err := os.Symlink(filepath.Join(f.outside, "witness"), filepath.Join(f.selected, "escape")); err != nil {
			t.Fatal(err)
		}
		var base ToolExecutor = NewWorkspaceToolExecutor()
		if composite {
			var err error
			if base, err = NewCompositeToolExecutor(base, WorkspaceTools(Sandbox{Mode: SandboxReadOnly}), &GatewayToolExecutor{}); err != nil {
				t.Fatal(err)
			}
		}
		pinned, closeRoots, err := base.(rootPinner).pinReadableRoots([]string{f.selected}, []ReadableRootIdentity{f.identity})
		if err != nil {
			t.Fatal(err)
		}
		out, err := pinned.ExecuteTool(t.Context(), ToolRequest{Name: "read_file", Input: []byte(`{"path":"` + filepath.Join(f.selected, "escape") + `"}`), WorkingDir: f.working, Sandbox: Sandbox{Mode: SandboxReadOnly}, ReadableRoots: []string{f.selected}})
		closeRoots()
		if err == nil {
			t.Fatalf("composite=%v symlink escape was followed: out=%q", composite, out)
		}
		for _, leak := range []string{"outside-secret", f.outside} {
			if strings.Contains(out, leak) || strings.Contains(err.Error(), leak) {
				t.Fatalf("composite=%v symlink escape disclosed %q: out=%q err=%v", composite, leak, out, err)
			}
		}
	}
}
