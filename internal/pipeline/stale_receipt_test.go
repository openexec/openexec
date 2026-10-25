package pipeline

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/openexec/openexec/internal/execution/gates"
	"github.com/openexec/openexec/internal/loop"
	"github.com/openexec/openexec/internal/router"
)

// A failed check's receipt belongs to that stage execution only. When the
// blueprint routes the failure to another deterministic stage that then fails
// without a classified check, the run must not end carrying the earlier
// stage's receipt as if the later stage had produced it.
func TestFailedCheckReceiptDoesNotOutliveItsStage(t *testing.T) {
	for _, tc := range []struct {
		name, body, terminalStage string
		receipt                   bool
	}{
		{"check-is-terminal", "id: stale\nname: stale\ninitial_stage: test\nstages:\n  test:\n    name: test\n    type: deterministic\n    commands: ['exit 0']\n", "test", true},
		{"later-unclassified-stage", "id: stale\nname: stale\ninitial_stage: test\nstages:\n  test:\n    name: test\n    type: deterministic\n    commands: ['exit 0']\n    max_retries: 1\n    on_failure: probe\n  probe:\n    name: probe\n    type: deterministic\n    commands: ['exit 127']\n", "probe", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			for path, contents := range map[string]string{
				filepath.Join(dir, ".openexec", "config.json"):              `{"execution":{"test_commands":["printf 'FAIL TestX'; exit 1"]}}`,
				filepath.Join(dir, ".openexec", "blueprints", "stale.yaml"): tc.body,
			} {
				if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, []byte(contents), 0600); err != nil {
					t.Fatal(err)
				}
			}
			p := &Pipeline{cfg: Config{FWUID: "stale", WorkDir: dir, BlueprintID: "stale", TaskDescription: "check", TaskTimeout: 5 * time.Second, DefaultMaxIterations: 1}, events: make(chan loop.Event, 100), intentRouter: router.NewDeterministicRouter(), gateRunner: &evidenceGateRunner{}}
			if err := p.runBlueprintMode(context.Background()); err == nil {
				t.Fatal("failed blueprint reported success")
			}
			terminal := loop.Event{}
			for len(p.events) > 0 {
				if event := <-p.events; event.Type == loop.EventBlueprintFailed {
					terminal = event
				}
			}
			if terminal.Type != loop.EventBlueprintFailed || terminal.StageName != tc.terminalStage {
				t.Fatalf("unexpected terminal event: %+v", terminal)
			}
			_, carried := terminal.Artifacts[gates.VerificationFailureReceiptKey]
			if carried != tc.receipt || (tc.receipt && !gates.ValidateVerificationFailureArtifacts(terminal.Artifacts)) {
				t.Fatalf("receipt carried=%v want %v: %+v", carried, tc.receipt, terminal.Artifacts)
			}
		})
	}
}
