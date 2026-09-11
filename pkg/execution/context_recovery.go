package execution

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"

	"github.com/openexec/openexec/pkg/agent"
)

// Replace repeated copies, never unique facts, system/owner instructions or
// tool-call/result pairs. Retained request history is not mutated. The last
// exact copy remains in the next request; earlier results point to that copy.
// This is one deterministic attempt, not an inference-powered summary loop.
func compactRepeatedResults(r agent.Request) (agent.Request, bool) {
	copyRequest := r
	copyRequest.Messages = append([]agent.Message(nil), r.Messages...)
	seen := map[string]string{}
	changed := false
	for i := len(r.Messages) - 1; i >= 0; i-- {
		copyRequest.Messages[i].Content = append([]agent.ContentBlock(nil), r.Messages[i].Content...)
		for j := len(r.Messages[i].Content) - 1; j >= 0; j-- {
			b := &copyRequest.Messages[i].Content[j]
			if b.Type != agent.ContentTypeToolResult || b.ToolError != "" || len(b.ToolOutput) < 1024 {
				continue
			}
			sum := sha256.Sum256([]byte(b.ToolOutput))
			digest := hex.EncodeToString(sum[:])
			if later, ok := seen[digest]; ok {
				ref, _ := json.Marshal(map[string]string{"duplicateOfToolResult": later, "sha256": digest, "notice": "Exact duplicate detail omitted here; the complete identical result remains later in this request. No task completion or authority is inferred."})
				b.ToolOutput = string(ref)
				changed = true
			} else {
				seen[digest] = b.ToolResultID
			}
		}
	}
	return copyRequest, changed
}
