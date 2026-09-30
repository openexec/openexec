package pipeline

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/openexec/openexec/internal/execution/gates"
	"github.com/openexec/openexec/internal/loop"
	"github.com/openexec/openexec/internal/testutil/admittedevidence"
	"github.com/openexec/openexec/pkg/runtime"
)

func TestPublicSilentFailureEvent(t *testing.T) {
	for _, gate := range []string{"lint", "test"} {
		t.Run(gate, func(t *testing.T) {
			f := &admittedevidence.Executor{Dir: t.TempDir(), Gate: gate}
			p, events := New(Config{FWUID: "silent", WorkDir: f.Dir, BlueprintID: "standard_task", StageExecutor: f})
			defer p.Close()
			if err := p.runBlueprintMode(context.Background()); err == nil {
				t.Fatal("silent exit 2 accepted")
			}
			var terminal *loop.Event
			for len(events) > 0 {
				event := <-events
				if event.Type == loop.EventBlueprintFailed {
					terminal = &event
				}
			}
			if terminal == nil || terminal.StageName != gate || !gates.ValidateVerificationFailureArtifacts(terminal.Artifacts) {
				t.Fatalf("missing typed terminal failure: %+v", terminal)
			}
			hash, err := hex.DecodeString(f.Hash)
			if err != nil || len(hash) != 32 || terminal.Artifacts[f.Hash] != f.Path {
				t.Fatal("terminal event lost wrapper evidence attachment")
			}
			if terminal.Text != "" || terminal.Result.Diagnostics != "" || f.Calls != 1 {
				t.Fatal("silent fixture produced diagnostics or repeated execution")
			}
		})
	}
}

func TestPublicEvidenceFailureClassification(t *testing.T) {
	ctx := context.Background()
	exitTwo := exec.Command("/bin/sh", "-c", "exit 2").Run()
	if exitTwo == nil {
		t.Fatal("expected process failure")
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	refusal := errors.New("admission refused")
	hash, path := strings.Repeat("a", 64), "/private/reference"
	for _, tc := range []struct {
		name       string
		ctx        context.Context
		err        error
		classified bool
	}{
		{"failure", ctx, exitTwo, true},
		{"success", ctx, nil, false},
		{"cancelled", cancelled, exitTwo, false},
		{"transport", ctx, refusal, false},
		{"launch", ctx, exec.Command("/nonexistent-openexec-fixture").Run(), false},
		{"reserved-exit", ctx, exec.Command("/bin/sh", "-c", "exit 127").Run(), false},
		{"mixed", ctx, errors.Join(exitTwo, refusal), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			failure := runtime.VerificationCommandFailureWithEvidence(tc.ctx, "lint", tc.err, hash, path)
			artifacts := gates.VerificationFailureArtifacts(failure)
			if !tc.classified {
				if failure != tc.err || artifacts != nil {
					t.Fatal("reference changed refusal/success classification")
				}
				return
			}
			plain := gates.VerificationFailureArtifacts(runtime.VerificationCommandFailure(tc.ctx, "lint", tc.err))
			if !gates.ValidateVerificationFailureArtifacts(artifacts) || artifacts[hash] != path || artifacts[gates.VerificationFailureDigestKey] != plain[gates.VerificationFailureDigestKey] {
				t.Fatal("wrapper lost reference or changed classification digest")
			}
			if gates.VerificationFailureArtifacts(errors.Join(failure, refusal)) != nil {
				t.Fatal("mixed typed failure authorized repair")
			}
		})
	}
}

func TestPublicDiagnosticFailureEvent(t *testing.T) {
	var fingerprint string
	for _, nilResult := range []bool{false, true} {
		t.Run(fmt.Sprintf("nil=%t", nilResult), func(t *testing.T) {
			f := &admittedevidence.Executor{Dir: t.TempDir(), Gate: "test", Diagnostic: true, NilResult: nilResult}
			p, events := New(Config{FWUID: "diagnostic", WorkDir: f.Dir, BlueprintID: "standard_task", StageExecutor: f})
			defer p.Close()
			if err := p.runBlueprintMode(context.Background()); err == nil {
				t.Fatal("failed check accepted")
			}
			var terminal *loop.Event
			for len(events) > 0 {
				e := <-events
				if e.Type == loop.EventBlueprintFailed {
					terminal = &e
				}
			}
			if terminal == nil || terminal.Artifacts[f.Hash] != f.Path || !gates.ValidateVerificationFailureArtifacts(terminal.Artifacts) {
				t.Fatal("missing diagnostic evidence")
			}
			receipt := terminal.Artifacts[gates.VerificationFailureReceiptKey]
			if receipt != `[{"gate":"test","exit_code":2}]` {
				t.Fatal("classification contains command diagnostics", receipt)
			}
			digest := terminal.Artifacts[gates.VerificationFailureDigestKey]
			if fingerprint != "" && digest != fingerprint {
				t.Fatal("private capture changed public fingerprint")
			}
			fingerprint = digest
			raw, err := json.Marshal(terminal)
			if err != nil || strings.Contains(string(raw), "SENTINEL") {
				t.Fatal("public event exposed secret", err)
			}
			if !nilResult && (!strings.Contains(terminal.Text, "DIAGNOSTIC_TAIL") || !strings.Contains(terminal.Result.Diagnostics, "STDERR_TAIL")) {
				t.Fatal("terminal lost diagnostic tails")
			}
		})
	}
}

func TestPublicNilResultRefusals(t *testing.T) {
	for _, name := range []string{"launch", "cancelled", "transport"} {
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			var original error
			switch name {
			case "launch":
				original = exec.Command(filepath.Join(t.TempDir(), "missing")).Run()
			case "cancelled":
				cmd := exec.CommandContext(ctx, "/bin/sh", "-c", "while :; do :; done")
				if err := cmd.Start(); err != nil {
					t.Fatal(err)
				}
				cancel()
				original = cmd.Wait()
			default:
				original = errors.New("admission refused token=PRIVATE_SENTINEL")
			}
			classified := runtime.VerificationCommandFailureWithEvidence(ctx, "test", original, strings.Repeat("a", 64), "/private/reference")
			runner := &gateRunnerAction{}
			adapter := admittedStageExecutor{executor: retentionBoundaryExecutor{nil, classified}, evidence: runner}
			result, err := adapter.Execute(ctx, &runtime.Stage{Name: "test", Type: runtime.StageTypeDeterministic}, nil)
			if result != nil || err == nil || !errors.Is(err, original) || runner.receipt != nil || strings.Contains(err.Error(), "SENTINEL") {
				t.Fatal("nil result refusal lost identity, leaked, or authorized repair")
			}
		})
	}
}
