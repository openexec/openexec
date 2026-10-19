package gates

import (
	"crypto/sha256"
	"encoding/hex"
	"testing"
)

func TestReceiptUnitMalformedChecks(t *testing.T) {
	for _, payload := range []string{"not json", "[]", `[{"gate":"test","exit_code":0}]`, `[{"gate":"test","exit_code":1,"terminal_id":"id"}]`} {
		digest := sha256.Sum256([]byte(payload))
		if ValidateVerificationFailureArtifacts(map[string]string{VerificationFailureReceiptKey: payload, VerificationFailureDigestKey: hex.EncodeToString(digest[:])}) {
			t.Fatalf("accepted %s", payload)
		}
	}
	// Typed evidence must still pass receipt shape validation at the native boundary.
	err := &verificationFailure{message: "bad", checks: []CheckFailure{{Gate: "", ExitCode: 1}}}
	if VerificationFailureArtifactsForStage(err, TerminalBinding{}) != nil {
		t.Fatal("malformed typed receipt accepted")
	}
}
