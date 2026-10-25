package manager

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/openexec/openexec/pkg/audit"
	"github.com/openexec/openexec/pkg/db/state"
)

// wantApplyPatchRefusal is the refusal text apply_patch must return. It is
// spelled out rather than referencing the production constant, so this test
// still compiles against the unfixed action and fails on its assertions.
const wantApplyPatchRefusal = "action apply_patch is not implemented and applied no changes to the workspace; " +
	"apply the patch from an agentic stage with the git_apply_patch tool, or remove this stage from the blueprint"

// applyPatchChildEnv names the disposable project a child test process runs;
// the parent reads what that process left behind after it has exited.
const applyPatchChildEnv = "OPENEXEC_APPLY_PATCH_PERSIST_CHILD"

// applyPatchProbe is a custom blueprint whose only real stage dispatches the
// registered apply_patch action with a valid diff.
func applyPatchProbe(handler string) string {
	return "id: apply_probe\nname: Apply probe\ninitial_stage: apply\nstages:\n" +
		"  apply:\n    name: apply\n    type: deterministic\n    toolset: coding_backend\n    action: apply_patch\n" +
		"    inputs:\n      implementation: |\n        --- a/probe.txt\n        +++ b/probe.txt\n        @@ -1 +1 @@\n        -before\n        +after\n" +
		"    on_success: complete\n" + handler +
		"  recover:\n    name: recover\n    type: deterministic\n    toolset: coding_backend\n    commands: [\"true\"]\n    on_success: complete\n"
}

// TestApplyPatchPersistedChild is the child half of
// TestApplyPatchRefusalPersistsAfterReopen: it runs the custom blueprint through
// the native manager against the project's state and audit databases, then exits.
func TestApplyPatchPersistedChild(t *testing.T) {
	dir := os.Getenv(applyPatchChildEnv)
	if dir == "" {
		t.Skip("child process of TestApplyPatchRefusalPersistsAfterReopen")
	}
	store, err := state.NewStore(filepath.Join(dir, ".openexec", "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	logger, err := audit.NewLogger(filepath.Join(dir, ".openexec", "audit.db"))
	if err != nil {
		t.Fatal(err)
	}
	m, err := New(Config{WorkDir: dir, StateStore: store, AuditLogger: logger})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	if err := m.Start(ctx, "apply-probe", WithBlueprint("apply_probe")); err != nil {
		t.Fatal(err)
	}
	if err := m.Wait(context.Background(), "apply-probe"); err != nil {
		t.Fatal(err)
	}
	m.Close()
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	if err := logger.Close(); err != nil {
		t.Fatal(err)
	}
}

// TestApplyPatchRefusalPersistsAfterReopen runs a custom blueprint using
// apply_patch through the native manager in a separate process, and after that
// process exits reopens the existing runs table, run steps and audit log: the
// refusal must be recorded as noncompletion, never as a completed stage.
func TestApplyPatchRefusalPersistsAfterReopen(t *testing.T) {
	cases := []struct {
		name, handler, wantStatus string
	}{
		// No failure route: the run itself must end in error with the refusal.
		{"without-failure-handler", "", string(StatusError)},
		// A failure route recovers the run, but the apply stage stays failed.
		{"with-failure-handler", "    on_failure: recover\n    max_retries: 1\n", string(StatusComplete)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			probe := filepath.Join(dir, "probe.txt")
			if err := os.WriteFile(probe, []byte("before\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			bpDir := filepath.Join(dir, ".openexec", "blueprints")
			if err := os.MkdirAll(bpDir, 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(bpDir, "apply_probe.yaml"), []byte(applyPatchProbe(tc.handler)), 0o600); err != nil {
				t.Fatal(err)
			}

			child := exec.Command(os.Args[0], "-test.run=^TestApplyPatchPersistedChild$", "-test.count=1")
			child.Env = append(os.Environ(), applyPatchChildEnv+"="+dir)
			out, err := child.CombinedOutput()
			if err != nil {
				t.Fatalf("child run failed: %v\n%s", err, out)
			}
			if !strings.Contains(string(out), "PASS") {
				t.Fatalf("child did not run:\n%s", out)
			}

			if b, _ := os.ReadFile(probe); string(b) != "before\n" {
				t.Fatalf("probe.txt changed: %q", b)
			}

			store, err := state.NewStore(filepath.Join(dir, ".openexec", "state.db"))
			if err != nil {
				t.Fatal(err)
			}
			defer store.Close()
			ctx := context.Background()
			run, err := store.GetRun(ctx, "apply-probe")
			if err != nil || run == nil {
				t.Fatalf("persisted run missing: %v", err)
			}
			if run.Status != tc.wantStatus {
				t.Fatalf("persisted run status = %q (error %q), want %q", run.Status, run.ErrorMessage.String, tc.wantStatus)
			}
			if tc.handler == "" && !strings.Contains(run.ErrorMessage.String, wantApplyPatchRefusal) {
				t.Fatalf("persisted run error = %q, want the refusal", run.ErrorMessage.String)
			}
			steps, err := store.ListRunSteps(ctx, "apply-probe", 100, 0)
			if err != nil {
				t.Fatal(err)
			}
			sawApply := false
			for _, step := range steps {
				if step.Phase != "apply" {
					continue
				}
				sawApply = true
				if step.Status == "completed" {
					t.Fatalf("persisted run step records apply as completed: %+v", step)
				}
			}
			if !sawApply {
				t.Fatalf("no persisted step for the apply stage: %+v", steps)
			}

			logger, err := audit.NewLogger(filepath.Join(dir, ".openexec", "audit.db"))
			if err != nil {
				t.Fatal(err)
			}
			defer logger.Close()
			res, err := logger.Query(ctx, &audit.QueryFilter{EventTypes: []audit.EventType{audit.EventRunStep}, Limit: 200})
			if err != nil {
				t.Fatal(err)
			}
			stageFailed := false
			for _, e := range res.Entries {
				var md map[string]interface{}
				_ = e.GetMetadata(&md)
				if md["stage"] != "apply" {
					continue
				}
				switch md["type"] {
				case "stage_failed":
					if md["error"] != wantApplyPatchRefusal {
						t.Fatalf("persisted stage_failed error = %v, want the refusal", md["error"])
					}
					stageFailed = true
				case "stage_complete":
					t.Fatalf("audit log records apply as complete: %v", md)
				}
			}
			if !stageFailed {
				t.Fatalf("audit log has no stage_failed entry for apply (%d entries)", len(res.Entries))
			}
		})
	}
}
