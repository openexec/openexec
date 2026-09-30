package gates

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os/exec"
	"strings"
)

// NewCommandFailure is called at a configured verification command boundary.
// Unknown process and cancellation errors retain their original classification.
func NewCommandFailure(ctx context.Context, name string, err error) error {
	return NewCommandFailureWithOutput(ctx, name, err, "", "")
}

// NewCommandFailureWithOutput is NewCommandFailure carrying the command that
// ran and its output, so the evidence a repair is built from can be reproduced.
func NewCommandFailureWithOutput(ctx context.Context, name string, err error, command, output string) error {
	if err == nil {
		return nil
	}
	exit, ok := err.(*exec.ExitError)
	if !ok || ctx.Err() != nil || exit.ExitCode() <= 0 || exit.ExitCode() >= 126 {
		return err
	}
	if len(output) > maxFailureOutput {
		output = "…" + output[len(output)-maxFailureOutput:]
	}
	return &verificationFailure{message: err.Error(), checks: []CheckFailure{{Gate: name, ExitCode: exit.ExitCode(), Command: command, Output: output}}}
}

// CommandFailureWithEvidence attaches private references only after normal-exit
// classification. References never grant authority and never enter its digest.
func CommandFailureWithEvidence(ctx context.Context, name string, err error, hash, path string) error {
	classified := NewCommandFailure(ctx, name, err)
	if failure, ok := classified.(*verificationFailure); ok {
		failure.artifacts = map[string]string{hash: path}
	}
	return classified
}

const VerificationFailureReceiptKey = "verification_failure_receipt"
const VerificationFailureDigestKey = "verification_failure_digest"

// CheckFailure records a locally observed failed verification, not its cause.
type CheckFailure struct {
	Gate     string `json:"gate"`
	ExitCode int    `json:"exit_code"`
	// Command and Output are what the failure looked like: the argv that ran
	// and the tail of what it printed. Without them a repair task was handed
	// "test, exit 2" and nothing to reproduce, and stopped on "stored evidence
	// lacks the test command and diagnostics".
	Command string `json:"command,omitempty"`
	Output  string `json:"output,omitempty"`
}

// maxFailureOutput bounds the output a receipt carries: the tail, where the
// failing test and its assertion are.
const maxFailureOutput = 6000

type verificationFailure struct {
	message   string
	checks    []CheckFailure
	artifacts map[string]string
}

func (f *verificationFailure) Error() string { return f.message }

// NewFailure preserves the report's error text. Only runner-observed normal
// failed exits qualify as verification evidence. Transport, cancellation,
// launch, configuration and unknown failures remain unclassified.
func NewFailure(report *GateReport) error {
	if report == nil {
		return errors.New("missing gate report")
	}
	var checks []CheckFailure
	artifacts := map[string]string{}
	for _, result := range report.Results {
		if result.Passed || result.IsWarning {
			continue
		}
		if !result.verifiedExit {
			return errors.New(report.Summary)
		}
		checks = append(checks, CheckFailure{Gate: result.Name, ExitCode: result.ExitCode})
		for hash, path := range result.artifacts {
			artifacts[hash] = path
		}
	}
	if report.Passed || len(checks) == 0 {
		return errors.New(report.Summary)
	}
	return &verificationFailure{message: report.Summary, checks: checks, artifacts: artifacts}
}

// VerificationFailureArtifacts refuses mixed error trees: wrapping or joining
// a check failure with an unclassified refusal cannot authorize repair work.
func VerificationFailureArtifacts(err error) map[string]string {
	var checks []CheckFailure
	artifacts := map[string]string{}
	var visit func(error) bool
	visit = func(e error) bool {
		if e == nil {
			return false
		}
		if f, ok := e.(*verificationFailure); ok {
			checks = append(checks, f.checks...)
			for hash, path := range f.artifacts {
				artifacts[hash] = path
			}
			return len(f.checks) > 0
		}
		if u, ok := e.(interface{ Unwrap() []error }); ok {
			children := u.Unwrap()
			if len(children) == 0 {
				return false
			}
			for _, child := range children {
				if !visit(child) {
					return false
				}
			}
			return true
		}
		if u, ok := e.(interface{ Unwrap() error }); ok {
			return visit(u.Unwrap())
		}
		return false
	}
	if !visit(err) {
		return nil
	}
	payload, _ := json.Marshal(checks)
	digest := sha256.Sum256(payload)
	artifacts[VerificationFailureReceiptKey] = string(payload)
	artifacts[VerificationFailureDigestKey] = hex.EncodeToString(digest[:])
	return artifacts
}

// Validation checks integrity/shape, not provenance. Callers must obtain these
// artifacts from the trusted deterministic execution boundary, not workers.
func ValidateVerificationFailureArtifacts(artifacts map[string]string) bool {
	payload := artifacts[VerificationFailureReceiptKey]
	digest := sha256.Sum256([]byte(payload))
	if artifacts[VerificationFailureDigestKey] != hex.EncodeToString(digest[:]) {
		return false
	}
	var checks []CheckFailure
	if json.Unmarshal([]byte(payload), &checks) != nil || len(checks) == 0 {
		return false
	}
	for _, check := range checks {
		if strings.TrimSpace(check.Gate) == "" || check.ExitCode <= 0 || check.ExitCode >= 126 {
			return false
		}
	}
	return true
}
