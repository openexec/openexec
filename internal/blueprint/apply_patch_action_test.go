package blueprint

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/openexec/openexec/internal/actions"
	"github.com/openexec/openexec/internal/types"
)

// applyPatchBlueprint is a custom blueprint whose first stage dispatches the
// registered apply_patch action with a valid diff. handler is the YAML for the
// stage's failure route ("" means none).
func applyPatchBlueprint(handler string) string {
	return "id: apply_probe\nname: Apply probe\ninitial_stage: apply\nstages:\n" +
		"  apply:\n    name: apply\n    type: deterministic\n    toolset: coding_backend\n    action: apply_patch\n" +
		"    inputs:\n      implementation: |\n        --- a/probe.txt\n        +++ b/probe.txt\n        @@ -1 +1 @@\n        -before\n        +after\n" +
		"    on_success: complete\n" + handler +
		"  recover:\n    name: recover\n    type: deterministic\n    toolset: coding_backend\n    commands: [\"true\"]\n    on_success: complete\n"
}

// TestCustomBlueprintApplyPatchRefusalTakesFailurePath loads a custom
// blueprint from a disposable project and runs it through the real engine,
// DefaultExecutor and action registry: the refusal must enter the existing
// failure/retry route and never complete the stage or change the file.
func TestCustomBlueprintApplyPatchRefusalTakesFailurePath(t *testing.T) {
	cases := []struct {
		name    string
		handler string
	}{
		{"with-failure-handler", "    on_failure: recover\n    max_retries: 1\n"},
		{"without-failure-handler", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			probe := filepath.Join(dir, "probe.txt")
			if err := os.WriteFile(probe, []byte("before\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			bpPath := filepath.Join(dir, ".openexec", "blueprints", "apply_probe.yaml")
			if err := os.MkdirAll(filepath.Dir(bpPath), 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(bpPath, []byte(applyPatchBlueprint(tc.handler)), 0o600); err != nil {
				t.Fatal(err)
			}
			bp, err := NewRegistry().LoadFromFile(bpPath)
			if err != nil {
				t.Fatal(err)
			}

			executor := NewDefaultExecutor(dir)
			executor.ActionRegistry = actions.DefaultRegistry(dir)
			var completed []*StageResult
			var visited []string
			cfg := DefaultEngineConfig()
			cfg.OnStageStart = func(_ *Run, stage string) { visited = append(visited, stage) }
			cfg.OnStageComplete = func(_ *Run, result *StageResult) { completed = append(completed, result) }
			engine, err := NewEngine(bp, executor, cfg)
			if err != nil {
				t.Fatal(err)
			}
			run, err := engine.StartRun(context.Background(), "apply-probe", nil)
			if err != nil {
				t.Fatal(err)
			}
			execErr := engine.Execute(context.Background(), run, NewStageInput("apply-probe", "probe", dir))

			if len(completed) == 0 || completed[0].StageName != "apply" {
				t.Fatalf("apply stage never reported: %+v", completed)
			}
			apply := completed[0]
			if apply.Status != types.StageStatusFailed {
				t.Fatalf("apply stage status = %q (output %q), want failed", apply.Status, apply.Output)
			}
			if apply.Error != actions.ApplyPatchUnsupported {
				t.Fatalf("stage-failed callback error = %q, want the refusal", apply.Error)
			}
			if b, _ := os.ReadFile(probe); string(b) != "before\n" {
				t.Fatalf("probe.txt changed: %q", b)
			}

			if tc.handler != "" {
				if execErr != nil {
					t.Fatalf("failure handler route returned %v", execErr)
				}
				if len(visited) != 2 || visited[1] != "recover" || run.GetRetries("apply") != 1 {
					t.Fatalf("refusal did not move to the failure handler: visited=%v retries=%d", visited, run.GetRetries("apply"))
				}
				return
			}
			if execErr == nil || !strings.Contains(execErr.Error(), actions.ApplyPatchUnsupported) {
				t.Fatalf("run without a handler returned %v, want the refusal", execErr)
			}
			if run.Status != RunStatusFailed || run.Error != actions.ApplyPatchUnsupported {
				t.Fatalf("run status=%q error=%q, want failed with the refusal", run.Status, run.Error)
			}
			if len(visited) != 1 {
				t.Fatalf("run continued past the refused stage: %v", visited)
			}
		})
	}
}
