package gates

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/openexec/openexec/internal/execution/evidence"
)

func TestRetentionBoundariesGateCapture(t *testing.T) {
	dir := t.TempDir()
	command := `TOKEN=COMMAND_SECRET; printf '%s\n' "$TOKEN"; printf 'token=DIAGNOSTIC_SECRET\n' >&2; head -c 20000 /dev/zero; head -c 20000 /dev/zero >&2; exit 2`
	runner := &Runner{projectDir: dir, timeout: time.Second * 10, config: &Config{Quality: QualityConfig{Gates: []string{"test"}, Custom: []CustomGate{{Name: "test", Command: command}}}}}
	report := runner.RunAll(context.Background())
	if report.Passed || len(report.Results) != 1 {
		t.Fatal("failed check accepted")
	}
	result := report.Results[0]
	if strings.Contains(result.Output, "SECRET") || len(result.Output) > 2*evidence.StreamLimit+1 {
		t.Fatal("public output leaked or unbounded")
	}
	refs := VerificationFailureArtifacts(NewFailure(report))
	if !ValidateVerificationFailureArtifacts(refs) || len(refs) != 3 {
		t.Fatal("private reference/classification lost")
	}
	digest := refs[VerificationFailureDigestKey]
	for hash := range result.artifacts {
		private, err := evidence.Read(dir, hash)
		if err != nil {
			t.Fatal(err)
		}
		if private.Argv[2] != command || private.Cwd != dir || len(private.Stdout) != evidence.StreamLimit || len(private.Stderr) != evidence.StreamLimit {
			t.Fatal("private capture identity/bound lost")
		}
		if !strings.Contains(private.Stderr, "DIAGNOSTIC_SECRET") {
			t.Fatal("private diagnostic missing")
		}
	}
	runner.config.Quality.Custom[0].Command = "printf 'different diagnostic'; exit 2"
	next := runner.RunAll(context.Background())
	if VerificationFailureArtifacts(NewFailure(next))[VerificationFailureDigestKey] != digest {
		t.Fatal("diagnostic changed classification fingerprint")
	}
	runner.config.Quality.Custom[0].Command = "exit 0"
	if report := runner.RunAll(context.Background()); !report.Passed || VerificationFailureArtifacts(NewFailure(report)) != nil {
		t.Fatal("success became repair authority")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	runner.config.Quality.Custom[0].Command = "exit 2"
	if refs := VerificationFailureArtifacts(NewFailure(runner.RunAll(ctx))); refs != nil {
		t.Fatal("cancellation became repair authority")
	}
	// A refusal to retain private evidence cannot mint an authoritative receipt.
	if err := os.Chmod(filepath.Join(dir, ".openexec", "data", "verification"), 0755); err != nil {
		t.Fatal(err)
	}
	if refs := VerificationFailureArtifacts(NewFailure(runner.RunAll(context.Background()))); refs != nil {
		t.Fatal("capture refusal authorized repair")
	}
}
