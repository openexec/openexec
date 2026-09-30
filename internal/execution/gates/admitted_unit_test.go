package gates

import (
	"crypto/sha256"
	"encoding/hex"
	"testing"
)

func TestAdmittedUnitReceiptShape(t *testing.T) {
	// A valid digest proves integrity only; malformed content must still fail.
	for _, payload := range []string{"", "null", "[]", "not json", `[{"gate":" ","exit_code":2}]`, `[{"gate":"lint","exit_code":0}]`, `[{"gate":"lint","exit_code":-1}]`, `[{"gate":"lint","exit_code":126}]`, `[{"gate":"lint","exit_code":127}]`} {
		t.Run(payload, func(t *testing.T) {
			digest := sha256.Sum256([]byte(payload))
			if ValidateVerificationFailureArtifacts(map[string]string{VerificationFailureReceiptKey: payload, VerificationFailureDigestKey: hex.EncodeToString(digest[:])}) {
				t.Fatal("invalid receipt accepted")
			}
		})
	}
}
