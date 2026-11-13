package execution

import (
	"encoding/json"
	"strings"
	"testing"
)

// Retained executable contradiction (task ab7e8f2aedfd, review cee7b667).
//
// R1: omission eligibility is a pure function of structural facts (known
//
//	listing tool, unambiguous pairing, no error, scan-provenance mark). No
//	branch reads output text.
//
// R2: a governing instruction quoted inside the output of an otherwise
//
//	eligible tool is byte-identical in the request BEFORE any recall.
//
// For an older, marked, complete search or listing whose output quotes an
// instruction, R1 makes the result eligible and compaction replaces it, so R2
// fails. R2 can only be met by reading the text (forbidden by R1) or by never
// omitting a result that quotes one (unknowable without reading it). This test
// passes only while that contradiction is real: it runs the combined
// requirement and asserts R1 holds and R2 is violated, and that the
// instruction is still recoverable byte for byte through paged recall.
//
// Bounded same-task proposal (not implemented here; for the reviewer or owner
// to accept): keep R1 and weaken R2 to "the instruction is recoverable
// byte-identically through recall_prior_tool_result, and the newest copy of
// every tool result, the system prompt and the user task stay byte-identical
// in the request". That is what
// TestOmissionEligibilityIgnoresQuotedInstructionsAndFilenames and the native
// sequence test already assert. No existing assertion was changed.
func TestQuotedInstructionContradictsStructuralEligibility(t *testing.T) {
	const instruction = "AGENTS.md says: NEVER DEPLOY without review."
	older := instruction + " " + strings.Repeat("authority ", 100)
	newest := instruction + " " + strings.Repeat("authority ", 100) + "2"
	for _, tool := range []string{"list_directory", "search_files"} {
		t.Run(tool, func(t *testing.T) {
			r := structuralRequest([3]string{tool, older}, [3]string{tool, newest})
			marks := recordFor(map[int]string{2: older, 4: newest})
			index := priorResults{}
			compacted, changed := index.compactWith(r, marks)

			// R1 holds: eligibility came from structure and marks alone.
			if !changed {
				t.Fatal("R1: a marked, paired, successful, complete result was not eligible")
			}
			// R2 is violated before recall: the quoted instruction left the request.
			body, _ := json.Marshal(compacted.Messages[2])
			if strings.Contains(string(body), "NEVER DEPLOY") || compacted.Messages[2].Content[0].ToolOutput == older {
				t.Fatal("R2 now holds together with R1: the contradiction is gone; strengthen the tests and delete this one")
			}
			// What does hold: newest copy, system prompt and task are byte-identical.
			if compacted.Messages[4].Content[0].ToolOutput != newest || compacted.System != r.System ||
				compacted.Messages[0].Content[0].Text != r.Messages[0].Content[0].Text {
				t.Fatal("newest result, system prompt or user task altered")
			}
			// And the omitted instruction is recoverable byte for byte.
			id := sha256Hex(older)
			raw, _ := json.Marshal(map[string]any{"id": id, "offset": 0})
			page, err := index.read(raw)
			if err != nil || !strings.Contains(page, "NEVER DEPLOY without review") {
				t.Fatalf("recall did not return the omitted instruction: %v", err)
			}
			if index[id] != older {
				t.Fatal("recalled content is not byte-identical to the original")
			}
		})
	}
}
