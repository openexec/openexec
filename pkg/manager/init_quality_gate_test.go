package manager

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/openexec/openexec/internal/blueprint"
	"github.com/openexec/openexec/internal/execution/gates"
	"github.com/openexec/openexec/internal/project"
	"github.com/openexec/openexec/pkg/db/state"
)

// oldInitTemplate is the openexec.yaml project.Initialize wrote before this
// fix: a lint gate with no quality.custom entry.
const oldInitTemplate = "project:\n  name: fixture\n  type: fullstack-webapp\n\nquality:\n  gates:\n    - lint\n"

// verifyProbe is a one-stage blueprint holding exactly quick_fix's verify
// stage, so the run needs no agentic implement stage and no provider.
const verifyProbe = "id: verify_probe\nname: Verify probe\ninitial_stage: verify\nstages:\n  verify:\n    name: verify\n    type: deterministic\n    toolset: coding_backend\n    action: run_gates\n    run_quality_gates: true\n    on_success: complete\n"

// TestFreshInitQualityGateNativeRun drives project.Initialize, rereads the
// persisted openexec.yaml, then runs quick_fix's verify stage through the
// native manager (the gates.Runner adapter it wires when no external
// executor is admitted). Only the blueprint is a fixture.
func TestFreshInitQualityGateNativeRun(t *testing.T) {
	stage := blueprint.QuickFixBlueprint.Stages["verify"]
	if stage.Action != "run_gates" || !stage.RunQualityGates || stage.Type != "deterministic" {
		t.Fatalf("verify_probe no longer mirrors quick_fix verify: %+v", stage)
	}
	goMod := "module example.com/fixture\n\ngo 1.21\n"
	clean := "package fixture\n\nfunc Add(a, b int) int { return a + b }\n"
	vetError := "package fixture\n\nimport \"fmt\"\n\nfunc Show() string { return fmt.Sprintf(\"%d\", \"not a number\") }\n"
	cases := []struct {
		name       string
		files      map[string]string
		yaml       string // written over the initialized openexec.yaml when set
		removeYAML bool
		command    string // the gate init must declare; "" means none
		complete   bool
		wantError  string
	}{
		{name: "go-clean", files: map[string]string{"go.mod": goMod, "fixture.go": clean}, command: "go vet ./...", complete: true},
		{name: "go-vet-error", files: map[string]string{"go.mod": goMod, "fixture.go": vetError}, command: "go vet ./...", wantError: "lint"},
		{name: "npm-lint-script", files: map[string]string{"package.json": `{"scripts":{"lint":"exit 0"}}`}, command: "npm run lint", complete: true},
		{name: "npm-lint-failing", files: map[string]string{"package.json": `{"scripts":{"lint":"echo lint-broke; exit 3"}}`}, command: "npm run lint", wantError: "lint"},
		{name: "npm-without-lint-script", files: map[string]string{"package.json": `{"scripts":{"test":"exit 0"}}`}, wantError: gates.NoGatesDeclared},
		{name: "empty-folder", wantError: gates.NoGatesDeclared},
		{name: "go-module-without-packages", files: map[string]string{"go.mod": goMod}, wantError: gates.NoGatesDeclared},
		{name: "explicit-unknown", yaml: "project:\n  name: fixture\nquality:\n  gates: [typecheck]\n", wantError: "not defined under quality.custom in openexec.yaml: typecheck"},
		{name: "explicit-custom", yaml: "project:\n  name: fixture\nquality:\n  gates: [marker]\n  custom:\n    - name: marker\n      command: test -f marker.txt\n      mode: blocking\n", files: map[string]string{"marker.txt": "x"}, complete: true},
		{name: "no-openexec-yaml", removeYAML: true, complete: true},
		// Guard-removal control: the old template must fail the run that
		// go-clean completes, with the refusal this fix removes.
		{name: "old-template-control", files: map[string]string{"go.mod": goMod, "fixture.go": clean}, command: "go vet ./...", yaml: oldInitTemplate, wantError: "not defined under quality.custom in openexec.yaml: lint"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			for name, body := range tc.files {
				if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			cfg, err := project.Initialize("fixture", dir)
			if err != nil {
				t.Fatal(err)
			}
			// Isolate the command gate: the no-stubs and production-readiness
			// gates the manager also composes judge unrelated properties.
			off := false
			cfg.QualityGates.NoStubs, cfg.QualityGates.ProductionReady = &off, &off
			if err := project.SaveProjectConfig(cfg); err != nil {
				t.Fatal(err)
			}

			// Reload what init persisted.
			persisted, err := gates.LoadConfig(dir)
			if err != nil {
				t.Fatal(err)
			}
			raw, _ := os.ReadFile(filepath.Join(dir, "openexec.yaml"))
			if tc.command == "" {
				if len(persisted.Quality.Gates) != 0 || len(persisted.Quality.Custom) != 0 {
					t.Fatalf("init declared a gate it could not infer:\n%s", raw)
				}
			} else {
				if len(persisted.Quality.Gates) != 1 {
					t.Fatalf("init gates = %v:\n%s", persisted.Quality.Gates, raw)
				}
				gate := persisted.GetGate(persisted.Quality.Gates[0])
				if gate == nil || gate.Command != tc.command || gate.Mode != "blocking" {
					t.Fatalf("declared gate does not resolve to %q: %+v\n%s", tc.command, gate, raw)
				}
			}
			// Every gate a fresh init declares resolves.
			for _, name := range persisted.GetEnabledGates() {
				if persisted.GetGate(name) == nil {
					t.Fatalf("init declared unresolvable gate %q:\n%s", name, raw)
				}
			}
			if tc.yaml != "" {
				if err := os.WriteFile(filepath.Join(dir, "openexec.yaml"), []byte(tc.yaml), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			if tc.removeYAML {
				if err := os.Remove(filepath.Join(dir, "openexec.yaml")); err != nil {
					t.Fatal(err)
				}
			}

			info, failure := runVerifyProbe(t, dir)
			if tc.complete {
				if info.Status != StatusComplete {
					t.Fatalf("status=%s error=%q failure=%q", info.Status, info.Error, failure)
				}
			} else {
				if info.Status != StatusError {
					t.Fatalf("status=%s, want error", info.Status)
				}
				if !strings.Contains(failure, tc.wantError) {
					t.Fatalf("failure %q does not name %q", failure, tc.wantError)
				}
			}

			// The real gate result behind the run: exit code and evidence.
			runner, err := gates.NewRunner(dir, time.Minute)
			if err != nil {
				t.Fatal(err)
			}
			report := runner.RunAll(context.Background())
			adapterErr := (&gateRunnerAdapter{runner: runner}).RunAll(context.Background())
			if (adapterErr == nil) != tc.complete || report.Passed != tc.complete {
				t.Fatalf("adapter err=%v report=%+v", adapterErr, report)
			}
			switch tc.name {
			case "go-clean":
				if len(report.Results) != 1 || report.Results[0].ExitCode != 0 {
					t.Fatalf("go vet did not run cleanly: %+v", report.Results)
				}
				t.Logf("go vet ./... exit=%d", report.Results[0].ExitCode)
			case "go-vet-error":
				res := report.Results[0]
				if res.ExitCode == 0 || !strings.Contains(res.Output, "Sprintf") {
					t.Fatalf("vet failure not observed: %+v", res)
				}
				// A real command failure is repair evidence, not a refusal.
				if gates.VerificationFailureArtifacts(adapterErr) == nil {
					t.Fatalf("vet failure not classified as verification failure: %v", adapterErr)
				}
				t.Logf("go vet ./... exit=%d output=%q", res.ExitCode, res.Output)
			case "npm-lint-failing":
				res := report.Results[0]
				if res.ExitCode == 0 || !strings.Contains(res.Output, "lint-broke") || gates.VerificationFailureArtifacts(adapterErr) == nil {
					t.Fatalf("npm lint failure not observed as command failure: %+v %v", res, adapterErr)
				}
				t.Logf("npm run lint exit=%d", res.ExitCode)
			case "empty-folder", "go-module-without-packages", "npm-without-lint-script":
				if len(report.Results) != 0 || gates.VerificationFailureArtifacts(adapterErr) != nil || adapterErr.Error() != gates.NoGatesDeclared {
					t.Fatalf("missing verification not refused as configuration: %v %+v", adapterErr, report)
				}
				if !strings.Contains(string(raw), "quality.custom") && !strings.Contains(string(raw), "custom:") {
					t.Fatalf("openexec.yaml does not say where to declare a gate:\n%s", raw)
				}
			case "explicit-unknown":
				res := report.Results[0]
				if !strings.Contains(res.Error, "gate 'typecheck' not found") || !strings.Contains(res.FixHint, "quality.custom") || !strings.Contains(res.FixHint, "name: typecheck") {
					t.Fatalf("unknown gate refusal lacks hint: %+v", res)
				}
				if gates.VerificationFailureArtifacts(adapterErr) != nil {
					t.Fatal("an undefined gate must not become command-failure evidence")
				}
			case "old-template-control":
				if len(report.Results) != 1 || !strings.Contains(report.Results[0].Error, "gate 'lint' not found in config") {
					t.Fatalf("old template did not reproduce the phantom gate: %+v", report.Results)
				}
				t.Logf("old template: %s", report.Results[0].Error)
			case "no-openexec-yaml":
				if report.Summary != "No openexec.yaml found, skipping quality gates" {
					t.Fatalf("no-yaml behaviour changed: %q", report.Summary)
				}
			}
		})
	}
}

func runVerifyProbe(t *testing.T, dir string) (PipelineInfo, string) {
	t.Helper()
	bpDir := filepath.Join(dir, ".openexec", "blueprints")
	if err := os.MkdirAll(bpDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bpDir, "verify_probe.yaml"), []byte(verifyProbe), 0o600); err != nil {
		t.Fatal(err)
	}
	store, err := state.NewStore(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	m, err := New(Config{WorkDir: dir, StateStore: store})
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	if err := m.Start(ctx, "verify-probe", WithBlueprint("verify_probe")); err != nil {
		t.Fatal(err)
	}
	if err := m.Wait(context.Background(), "verify-probe"); err != nil {
		t.Fatal(err)
	}
	info, err := m.Status("verify-probe")
	if err != nil {
		t.Fatal(err)
	}
	return info, info.Error
}
