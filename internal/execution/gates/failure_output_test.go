package gates

import (
	"context"
	"encoding/json"
	"os/exec"
	"strings"
	"testing"
)

// A repair is built from this receipt. Given only "test, exit 2" one stopped
// on "stored evidence lacks the test command and diagnostics"; the receipt now
// carries what ran and the tail of what it printed.
func TestACommandFailureCarriesItsCommandAndOutput(t *testing.T) {
	runErr := exec.Command("sh", "-c", "exit 2").Run()
	long := strings.Repeat("x", maxFailureOutput+100) + "--- FAIL: TestThatBroke"
	failure := NewCommandFailureWithOutput(context.Background(), "test", runErr, "make test", long)
	artifacts := VerificationFailureArtifacts(failure)
	if !ValidateVerificationFailureArtifacts(artifacts) {
		t.Fatalf("receipt invalid: %v", artifacts)
	}
	var checks []CheckFailure
	if err := json.Unmarshal([]byte(artifacts[VerificationFailureReceiptKey]), &checks); err != nil || len(checks) != 1 {
		t.Fatalf("receipt: %v %v", checks, err)
	}
	got := checks[0]
	if got.Gate != "test" || got.ExitCode != 2 || got.Command != "make test" {
		t.Fatalf("check = %+v", got)
	}
	if !strings.HasSuffix(got.Output, "--- FAIL: TestThatBroke") || len(got.Output) > maxFailureOutput+len("…") {
		t.Fatalf("output was not kept as its bounded tail: %d chars", len(got.Output))
	}
	// Without output the receipt is what it always was.
	plain := VerificationFailureArtifacts(NewCommandFailure(context.Background(), "lint", runErr))
	if !ValidateVerificationFailureArtifacts(plain) || strings.Contains(plain[VerificationFailureReceiptKey], "command") {
		t.Fatalf("a plain failure changed shape: %v", plain)
	}
}
