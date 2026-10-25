package actions

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/openexec/openexec/internal/types"
)

// wantApplyPatchRefusal is the refusal text apply_patch must return. It is
// spelled out rather than referencing the production constant, so this test
// still compiles against the unfixed action and fails on its assertions.
const wantApplyPatchRefusal = "action apply_patch is not implemented and applied no changes to the workspace; " +
	"apply the patch from an agentic stage with the git_apply_patch tool, or remove this stage from the blueprint"

// snapshotDir returns every regular file under dir with its contents, so a
// test can prove an action wrote, removed or created nothing.
func snapshotDir(t *testing.T, dir string) map[string]string {
	t.Helper()
	files := map[string]string{}
	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		b, err := os.ReadFile(path)
		files[path] = string(b)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return files
}

// The registered apply_patch has no patch engine. Given a valid diff it must
// refuse instead of reporting "Applied implementation changes" while the file
// stays untouched (design review 2026-10, entry 4).
func TestApplyPatchRefusesValidPatchWithoutEffect(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "probe.txt")
	if err := os.WriteFile(file, []byte("before\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	action, ok := DefaultRegistry(dir).Get("apply_patch")
	if !ok {
		t.Fatal("apply_patch is no longer registered")
	}
	if action.Name() != "apply_patch" {
		t.Fatalf("registered name = %q", action.Name())
	}
	before := snapshotDir(t, dir)

	diff := "diff --git a/probe.txt b/probe.txt\n--- a/probe.txt\n+++ b/probe.txt\n@@ -1 +1 @@\n-before\n+after\n"
	resp, err := action.Execute(context.Background(), ActionRequest{
		WorkspaceDir: dir,
		Inputs:       map[string]interface{}{"implementation": diff},
	})
	if err != nil {
		t.Fatalf("refusal must be a failed result, not a Go error: %v", err)
	}
	if resp.Status != types.StageStatusFailed {
		t.Fatalf("status = %q, want failed (output %q)", resp.Status, resp.Output)
	}
	if resp.Error != wantApplyPatchRefusal {
		t.Fatalf("error = %q, want the unsupported refusal", resp.Error)
	}
	for _, want := range []string{"apply_patch is not implemented", "applied no changes", "git_apply_patch", "remove this stage"} {
		if !strings.Contains(resp.Error, want) {
			t.Fatalf("refusal %q does not say %q", resp.Error, want)
		}
	}
	if strings.Contains(strings.ToLower(resp.Output), "applied") || resp.Artifacts["patch_applied"] != "" {
		t.Fatalf("refusal still claims an effect: output=%q artifacts=%v", resp.Output, resp.Artifacts)
	}
	b, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != "before\n" {
		t.Fatalf("target file changed: %q", b)
	}
	after := snapshotDir(t, dir)
	if len(after) != len(before) {
		t.Fatalf("workspace files changed: before=%v after=%v", before, after)
	}
	for path, content := range before {
		if after[path] != content {
			t.Fatalf("%s changed: %q -> %q", path, content, after[path])
		}
	}
}

// Bad input keeps its existing, distinct rejection: the refusal above must not
// replace it, and neither path may write anything.
func TestApplyPatchRejectsMissingInput(t *testing.T) {
	cases := []struct {
		name   string
		inputs map[string]interface{}
	}{
		{"missing", map[string]interface{}{"task_description": "x"}},
		{"nil-inputs", nil},
		{"empty", map[string]interface{}{"implementation": ""}},
		{"wrong-type", map[string]interface{}{"implementation": []byte("diff")}},
		{"wrong-type-int", map[string]interface{}{"implementation": 7}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, "probe.txt"), []byte("before\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			before := snapshotDir(t, dir)
			resp, err := NewApplyPatchAction(dir).Execute(context.Background(), ActionRequest{WorkspaceDir: dir, Inputs: tc.inputs})
			if err != nil {
				t.Fatal(err)
			}
			if resp.Status != types.StageStatusFailed || resp.Error != "no implementation data provided" {
				t.Fatalf("got status=%q error=%q, want the existing input rejection", resp.Status, resp.Error)
			}
			after := snapshotDir(t, dir)
			if len(after) != len(before) || after[filepath.Join(dir, "probe.txt")] != "before\n" {
				t.Fatalf("rejection touched the workspace: %v", after)
			}
		})
	}
}
