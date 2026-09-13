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
	if err == nil {
		return nil
	}
	exit, ok := err.(*exec.ExitError)
	if !ok || ctx.Err() != nil || exit.ExitCode() <= 0 || exit.ExitCode() >= 126 {
		return err
	}
	return &verificationFailure{message: err.Error(), checks: []CheckFailure{{Gate: name, ExitCode: exit.ExitCode()}}}
}

const VerificationFailureReceiptKey = "verification_failure_receipt"
const VerificationFailureDigestKey = "verification_failure_digest"

// CheckFailure records a locally observed failed verification, not its cause.
type CheckFailure struct {
	Gate     string `json:"gate"`
	ExitCode int    `json:"exit_code"`
}
type verificationFailure struct {
	message string
	checks  []CheckFailure
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
	for _, result := range report.Results {
		if result.Passed || result.IsWarning {
			continue
		}
		if !result.verifiedExit {
			return errors.New(report.Summary)
		}
		checks = append(checks, CheckFailure{Gate: result.Name, ExitCode: result.ExitCode})
	}
	if report.Passed || len(checks) == 0 {
		return errors.New(report.Summary)
	}
	return &verificationFailure{message: report.Summary, checks: checks}
}

// VerificationFailureArtifacts refuses mixed error trees: wrapping or joining
// a check failure with an unclassified refusal cannot authorize repair work.
func VerificationFailureArtifacts(err error) map[string]string {
	var checks []CheckFailure
	var visit func(error) bool
	visit = func(e error) bool {
		if e == nil {
			return false
		}
		if f, ok := e.(*verificationFailure); ok {
			checks = append(checks, f.checks...)
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
	return map[string]string{VerificationFailureReceiptKey: string(payload), VerificationFailureDigestKey: hex.EncodeToString(digest[:])}
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
