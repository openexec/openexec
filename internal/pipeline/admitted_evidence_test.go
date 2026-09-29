package pipeline

import (
	"context"
	"encoding/hex"
	"errors"
	"os/exec"
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
