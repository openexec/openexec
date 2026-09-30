package gates

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"reflect"
	"testing"
	"time"
)

type retentionUnitTree []error

func (e retentionUnitTree) Error() string   { return "tree" }
func (e retentionUnitTree) Unwrap() []error { return e }

func TestRetentionUnitReferencesAndFingerprint(t *testing.T) {
	ctx := context.Background()
	exit := exec.Command("sh", "-c", "exit 2").Run()
	a := CommandFailureWithEvidence(ctx, "check", exit, "hash-a", "/private/a")
	b := CommandFailureWithEvidence(ctx, "check", exit, "hash-b", "/private/b")
	first, second := VerificationFailureArtifacts(a), VerificationFailureArtifacts(b)
	if first[VerificationFailureDigestKey] != second[VerificationFailureDigestKey] || first[VerificationFailureReceiptKey] != second[VerificationFailureReceiptKey] || first["hash-a"] != "/private/a" || second["hash-b"] != "/private/b" {
		t.Fatal("reference changed fingerprint")
	}
	raw, err := json.Marshal(first)
	if err != nil {
		t.Fatal(err)
	}
	var roundtrip map[string]string
	if err := json.Unmarshal(raw, &roundtrip); err != nil || !reflect.DeepEqual(roundtrip, first) {
		t.Fatal("reference serialization", err)
	}
	joined := VerificationFailureArtifacts(errors.Join(fmt.Errorf("wrapped: %w", a), b))
	if joined["hash-a"] != "/private/a" || joined["hash-b"] != "/private/b" || !ValidateVerificationFailureArtifacts(joined) {
		t.Fatal("joined references lost")
	}
	for _, err := range []error{nil, errors.New("refusal"), retentionUnitTree{}, retentionUnitTree{nil}, &verificationFailure{}, errors.Join(a, context.Canceled), NewFailure(nil), NewFailure(&GateReport{})} {
		if VerificationFailureArtifacts(err) != nil {
			t.Fatal("unclassified tree accepted", err)
		}
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	for _, err := range []error{nil, context.Canceled, exit} {
		wrapped := CommandFailureWithEvidence(cancelled, "check", err, "hash", "path")
		if !errors.Is(wrapped, err) || VerificationFailureArtifacts(wrapped) != nil {
			t.Fatal("cancellation classification")
		}
	}
	report := &GateReport{Results: []GateResult{{Passed: true}, {IsWarning: true}, {Name: "check", ExitCode: 2, verifiedExit: true, artifacts: map[string]string{"hash-a": "/private/a"}}}}
	if got := VerificationFailureArtifacts(NewFailure(report)); !reflect.DeepEqual(got, first) {
		t.Fatal("report references", got)
	}
}

func TestRetentionUnitGateUnknownAndStderr(t *testing.T) {
	r := &Runner{projectDir: t.TempDir(), timeout: time.Second, config: &Config{Quality: QualityConfig{Custom: []CustomGate{{Name: "check", Command: "printf 'stderr only' >&2; exit 2"}}}}}
	if result := r.RunGate(context.Background(), "missing"); result.Passed || result.Error == "" {
		t.Fatal("unknown gate passed")
	}
	result := r.RunGate(context.Background(), "check")
	if result.Output != "stderr only" || result.Passed || !result.verifiedExit {
		t.Fatalf("stderr evidence: %+v", result)
	}
}
