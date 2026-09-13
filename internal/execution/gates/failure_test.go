package gates

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestVerificationFailureEvidence(t *testing.T) {
	for _, tc := range []struct {
		name, command string
		want          bool
	}{
		{"failed", "printf secret-output; exit 1", true},
		{"launch", "command_that_does_not_exist_987", false},
		{"signal", "kill -TERM $$", false},
		{"passed", "exit 0", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := &Runner{projectDir: t.TempDir(), timeout: time.Second, config: &Config{}}
			r.config.Quality.Gates = []string{"test"}
			r.config.Quality.Custom = []CustomGate{{Name: "test", Command: tc.command}}
			report := r.RunAll(context.Background())
			err := NewFailure(report)
			artifacts := VerificationFailureArtifacts(err)
			if (artifacts != nil) != tc.want {
				t.Fatalf("evidence=%v report=%+v", artifacts, report)
			}
			if tc.want {
				if !ValidateVerificationFailureArtifacts(artifacts) || strings.Contains(artifacts[VerificationFailureReceiptKey], "secret-output") {
					t.Fatal("invalid or unsanitized receipt")
				}
				if VerificationFailureArtifacts(errors.Join(err, context.Canceled)) != nil {
					t.Fatal("mixed refusal classified")
				}
				if VerificationFailureArtifacts(fmt.Errorf("wrapped: %w", err)) == nil {
					t.Fatal("typed wrapping lost")
				}
				artifacts[VerificationFailureReceiptKey] += " "
				if ValidateVerificationFailureArtifacts(artifacts) {
					t.Fatal("modified receipt accepted")
				}
			}
		})
	}
	report := &GateReport{Summary: "untrusted", Results: []GateResult{{Name: "test", ExitCode: 1}}}
	if VerificationFailureArtifacts(NewFailure(report)) != nil {
		t.Fatal("fabricated report classified")
	}
}
