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
	"github.com/openexec/openexec/internal/execution/evidence"
	"github.com/openexec/openexec/internal/execution/gates"
	"github.com/openexec/openexec/internal/loop"
	"github.com/openexec/openexec/internal/router"
	"github.com/openexec/openexec/internal/skills"
	"github.com/openexec/openexec/internal/types"
)

func TestRetentionUnitAdmittedCombinations(t *testing.T) {
	for _, kind := range []types.StageType{types.StageTypeAgentic, types.StageTypeDeterministic} {
		for _, withResult := range []bool{false, true} {
			for _, withError := range []bool{false, true} {
				var result *blueprint.StageResult
				var original error
				if withResult {
					result = &blueprint.StageResult{Output: "safe", Diagnostics: "diagnostic"}
				}
				if withError {
					original = errors.New("refused")
				}
				runner := &gateRunnerAction{}
				got, err := (admittedStageExecutor{executor: retentionBoundaryExecutor{result, original}, evidence: runner}).Execute(context.Background(), &blueprint.Stage{Type: kind}, nil)
				if got != result || !errors.Is(err, original) || runner.receipt != nil {
					t.Fatal("pair changed or authority inferred")
				}
			}
		}
	}
	action := &gateRunnerAction{}
	bp := &blueprint.Blueprint{Stages: map[string]*blueprint.Stage{"check": {Name: "check", Type: types.StageTypeDeterministic}}}
	for _, run := range []*blueprint.Run{{}, {Results: []*blueprint.StageResult{{StageName: "missing", Status: types.StageStatusFailed}}}, {Results: []*blueprint.StageResult{{StageName: "check", Status: types.StageStatusCompleted}}}} {
		if action.terminalEvidence(bp, run) != nil {
			t.Fatal("invalid terminal evidence")
		}
	}
}

func TestRetentionUnitNativePipelinePaths(t *testing.T) {
	for _, scenario := range []string{"success", "failure", "malformed", "missing", "review-refusal", "quick-refusal"} {
		t.Run(scenario, func(t *testing.T) {
			dir := t.TempDir()
			mustWrite := func(path, contents string) {
				t.Helper()
				if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, []byte(contents), 0600); err != nil {
					t.Fatal(err)
				}
			}
			mustWrite(filepath.Join(dir, "AGENTS.md"), "Test function api context instructions.\n")
			command := "printf retained-marker"
			if scenario == "failure" {
				command = "printf retained-marker; exit 2"
			}
			mustWrite(filepath.Join(dir, ".openexec", "config.json"), `{"execution":{"test_commands":["`+command+`"],"lint_commands":["exit 0"]}}`)
			body := "id: retained\nname: retained\ninitial_stage: implement\nstages:\n  implement:\n    name: implement\n    type: deterministic\n    commands: ['printf prepared']\n    on_success: test\n    create_checkpoint: true\n  test:\n    name: test\n    type: deterministic\n    commands: ['exit 0']\n    on_success: complete\n"
			if scenario == "review-refusal" {
				body = "id: retained\nname: retained\ninitial_stage: review\nstages:\n  review:\n    name: review\n    type: agentic\n    on_success: complete\n"
			}
			if scenario == "malformed" {
				body = "[invalid"
			}
			if scenario != "missing" {
				mustWrite(filepath.Join(dir, ".openexec", "blueprints", "retained.yaml"), body)
			}
			mustWrite(filepath.Join(dir, "skills", "testing", "SKILL.md"), "---\nname: testing\ndescription: Test function api\ntags: [test, function]\n---\nKeep evidence intact.\n")
			registry := skills.NewRegistry()
			if err := registry.LoadFromDir(filepath.Join(dir, "skills"), "project"); err != nil {
				t.Fatal(err)
			}
			p := &Pipeline{cfg: Config{FWUID: "retention", WorkDir: dir, BlueprintID: "retained", TaskDescription: "test function api", TaskTimeout: time.Second, ContextTokenBudget: 2000, PreResolvedContext: "pre-resolved fixture", ReviewEnabled: true, ReviewerModel: "review-model", APIProvider: "fixture", APIKey: "", DefaultMaxIterations: 1}, events: make(chan loop.Event, 100), intentRouter: router.NewDeterministicRouter(), skillRegistry: registry, gateRunner: &evidenceGateRunner{}}
			if scenario == "quick-refusal" {
				p.cfg.BlueprintID = "quick_fix"
			}
			pruneCfg := ocontext.DefaultPrunerConfig()
			pruneCfg.MinRelevanceScore = 0
			pruner, err := ocontext.NewPruner(dir, nil, pruneCfg)
			if err != nil {
				t.Fatal(err)
			}
			defer pruner.Close()
			p.contextPruner = pruner
			err = p.runBlueprintMode(context.Background())
			if (err == nil) != (scenario == "success") {
				t.Fatalf("%s disposition: %v", scenario, err)
			}
			if len(p.cfg.RepoZones) == 0 || len(p.cfg.KnowledgeSources) == 0 || p.cfg.Sensitivity == "" || p.cfg.SelectedToolset == "" {
				t.Fatal("routing context missing", p.cfg)
			}
			terminal := loop.Event{}
			for len(p.events) > 0 {
				event := <-p.events
				if event.Type == loop.EventBlueprintFailed || event.Type == loop.EventBlueprintComplete {
					terminal = event
				}
			}
			switch scenario {
			case "success":
				if terminal.Type != loop.EventBlueprintComplete {
					t.Fatal("missing success")
				}
			case "failure":
				if terminal.Type != loop.EventBlueprintFailed || terminal.StageName != "test" || !strings.Contains(terminal.Text, "retained-marker") || !gates.ValidateVerificationFailureArtifacts(terminal.Artifacts) {
					t.Fatalf("failed evidence lost: %+v err=%v", terminal, err)
				}
				references := 0
				for hash := range terminal.Artifacts {
					if hash == gates.VerificationFailureReceiptKey || hash == gates.VerificationFailureDigestKey {
						continue
					}
					private, err := evidence.Read(dir, hash)
					if err != nil || private.ExitCode != 2 || private.Argv[2] != command {
						t.Fatal("private native evidence missing", err)
					}
					references++
				}
				if references != 1 {
					t.Fatal("missing reference", references)
				}
			case "review-refusal", "quick-refusal":
				if !strings.Contains(err.Error(), "API key is required") || terminal.Artifacts[gates.VerificationFailureReceiptKey] != "" {
					t.Fatal("provider refusal authorized repair", err)
				}
			}
		})
	}
}
