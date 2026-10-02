package pipeline

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/openexec/openexec/internal/blueprint"
	ocontext "github.com/openexec/openexec/internal/context"
	"github.com/openexec/openexec/internal/loop"
	"github.com/openexec/openexec/internal/quality"
	"github.com/openexec/openexec/internal/router"
	"github.com/openexec/openexec/internal/skills"
)

func unitFile(t *testing.T, dir, path, content string) {
	t.Helper()
	full := filepath.Join(dir, path)
	if err := os.MkdirAll(filepath.Dir(full), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
}

// Exercise the orchestration unit with an in-memory executor, inspecting the
// exact inputs and emitted events; no task queue, persisted terminal or process.
func TestBlueprintUnitInputsAndOutcomes(t *testing.T) {
	for _, id := range []string{"standard_task", "quick_fix", "unit", "broken", "missing"} {
		t.Run(id, func(t *testing.T) {
			dir := t.TempDir()
			unitFile(t, dir, ".openexec/blueprints/unit.yaml", `id: unit
name: unit
initial_stage: implement
stages:
  implement:
    name: implement
    type: deterministic
    create_checkpoint: true
    on_success: complete
`)
			unitFile(t, dir, ".openexec/blueprints/broken.yaml", "[")
			unitFile(t, dir, ".openexec/config.json", `{"execution":{"lint_commands":["never execute lint"],"test_commands":["never execute test"]}}`)
			unitFile(t, dir, "skills/unit/SKILL.md", "---\nname: unit\ndescription: repair unit\n---\nUnit guidance\n")
			sr := skills.NewRegistry()
			if err := sr.LoadFromDir(filepath.Join(dir, "skills"), "project"); err != nil {
				t.Fatal(err)
			}
			calls := 0
			executor := terminalExecutor(func(_ context.Context, s *blueprint.Stage, i *blueprint.StageInput) (*blueprint.StageResult, error) {
				calls++
				if i.TaskAttempt != 3 || i.StageAttempt != 1 || !strings.Contains(i.Briefing, "resolved") || !strings.Contains(i.Briefing, "Unit guidance") {
					t.Fatalf("lost input: %+v", i)
				}
				if s.Name == "implement" && s.Timeout != time.Second {
					t.Fatal("timeout not forwarded")
				}
				if s.Name == "lint" && s.Commands[0] != "never execute lint" {
					t.Fatal("lint config lost")
				}
				r := blueprint.NewStageResult(s.Name, i.StageAttempt)
				r.Complete("unit")
				r.Artifacts = map[string]string{"unit": "value"}
				return r, nil
			})
			p := &Pipeline{cfg: Config{FWUID: "A", WorkDir: dir, BlueprintID: id, StageExecutor: executor, TaskAttempt: 3, TaskTimeout: time.Second, TaskDescription: "repair unit", PreResolvedContext: "resolved", ReviewEnabled: true, ReviewerModel: "unit"}, events: make(chan loop.Event, 100), skillRegistry: sr}
			err := p.runBlueprintMode(context.Background())
			if id == "broken" || id == "missing" {
				if err == nil || calls != 0 {
					t.Fatalf("invalid blueprint dispatched: %v", err)
				}
				return
			}
			if err != nil || calls == 0 {
				t.Fatalf("execution: calls=%d err=%v", calls, err)
			}
			found := false
			for len(p.events) > 0 {
				e := <-p.events
				if e.Type == loop.EventBlueprintComplete {
					found = true
					if e.Result.Artifacts["unit"] != "value" {
						t.Fatal("lost artifacts")
					}
				}
			}
			if !found {
				t.Fatal("no completion event")
			}
			p.cfg.ReviewEnabled = false
			p.cfg.StageExecutor = terminalExecutor(func(context.Context, *blueprint.Stage, *blueprint.StageInput) (*blueprint.StageResult, error) {
				return nil, errors.New("unit failure")
			})
			if err := p.runBlueprintMode(context.Background()); err == nil {
				t.Fatal("executor error passed")
			}
		})
	}
}

// Native configuration is exercised using the injected no-op gate action. The
// quality manager has no commands. Context and symbol inputs are local fixtures.
func TestBlueprintUnitNativeContext(t *testing.T) {
	dir := t.TempDir()
	unitFile(t, dir, ".openexec/blueprints/unit.yaml", `id: unit
name: unit
initial_stage: implement
stages:
  implement:
    name: implement
    type: deterministic
    action: run_gates
    run_quality_gates: true
    create_checkpoint: true
    on_success: complete
`)
	unitFile(t, dir, "AGENTS.md", "Greet backend API instructions")
	unitFile(t, dir, "greet.go", "package fixture\nfunc Greet() {}\n")
	db := setupTestDB(t)
	defer db.Close()
	insertSymbol(t, db, "Greet", "function", "greet.go", 2, 2, "func Greet()")
	pruneConfig := ocontext.DefaultPrunerConfig()
	pruneConfig.MinRelevanceScore = 0
	pruner, err := ocontext.NewPruner(dir, nil, pruneConfig)
	if err != nil {
		t.Fatal(err)
	}
	defer pruner.Close()
	p := &Pipeline{cfg: Config{FWUID: "A", WorkDir: dir, BlueprintID: "unit", TaskDescription: "Greet backend API", LocalPreResolveEnabled: true, StateDB: db, ContextTokenBudget: 8000}, events: make(chan loop.Event, 100), intentRouter: router.NewDeterministicRouter(), contextPruner: pruner, gateRunner: &evidenceGateRunner{}, qualityManager: quality.NewManager(dir, []quality.Gate{})}
	if err := p.runBlueprintMode(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(p.cfg.PreResolvedContext, "Greet") || p.cfg.SelectedToolset == "" || len(p.cfg.KnowledgeSources) == 0 {
		t.Fatalf("missing routed input: %+v", p.cfg)
	}
	// Wait for the asynchronous quality callback before returning the fixture.
	timer := time.NewTimer(5 * time.Second)
	defer timer.Stop()
	for {
		select {
		case e := <-p.events:
			if e.Type == loop.EventGatesPassed {
				// Creating the review loop must reject a missing API key before
				// provider dispatch, and propagate that refusal as failure.
				unitFile(t, dir, ".openexec/blueprints/unit.yaml", `id: unit
name: unit
initial_stage: review
stages:
  review:
    name: review
    type: agentic
    on_success: complete
`)
				p.cfg.APIProvider = "unit"
				p.cfg.ReviewerModel = "review-unit"
				p.cfg.ReviewEnabled = true
				p.cfg.ContextTokenBudget = 0
				if err := p.runBlueprintMode(context.Background()); err == nil || !strings.Contains(err.Error(), "API key is required") {
					t.Fatalf("missing key must refuse review dispatch: %v", err)
				}
				return
			}
		case <-timer.C:
			t.Fatal("quality callback missing")
		}
	}
}
