package execution

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"unicode/utf8"

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

const recallTool = "recall_prior_tool_result"

// A run-local index into tool results already present in the transcript. The
// original messages remain intact; only the provider's request is compacted.
type priorResults map[string]string

func (p priorResults) compact(r agent.Request) (agent.Request, bool) {
	copyRequest := r
	copyRequest.Messages = append([]agent.Message(nil), r.Messages...)
	// Only a known workspace directory listing contains names alone. File
	// reads, searches and unknown tools can contain governing instructions.
	pending := map[string][]string{}
	ambiguous := map[string]bool{}
	eligible := map[[2]int]bool{}
	for i, message := range r.Messages {
		for j, block := range message.Content {
			switch block.Type {
			case agent.ContentTypeToolUse:
				if len(pending[block.ToolUseID]) > 0 {
					ambiguous[block.ToolUseID] = true
				}
				pending[block.ToolUseID] = append(pending[block.ToolUseID], block.ToolName)
			case agent.ContentTypeToolResult:
				calls := pending[block.ToolResultID]
				if len(calls) > 0 {
					eligible[[2]int{i, j}] = !ambiguous[block.ToolResultID] && calls[0] == "list_directory"
					pending[block.ToolResultID] = calls[1:]
					if len(calls) == 1 {
						delete(ambiguous, block.ToolResultID)
					}
				}
			}
		}
	}
	kept, changed := 0, false
	for i := len(r.Messages) - 1; i >= 0; i-- {
		copyRequest.Messages[i].Content = append([]agent.ContentBlock(nil), r.Messages[i].Content...)
		for j := len(r.Messages[i].Content) - 1; j >= 0; j-- {
			b := &copyRequest.Messages[i].Content[j]
			if b.Type != agent.ContentTypeToolResult || !eligible[[2]int{i, j}] || b.ToolError != "" || len(b.ToolOutput) < 256 {
				continue
			}
			kept++
			if kept <= 1 {
				continue
			}
			hash := sha256.Sum256([]byte(b.ToolOutput))
			id := hex.EncodeToString(hash[:])
			p[id] = b.ToolOutput
			b.ToolOutput = fmt.Sprintf("Prior directory listing retained as %s (%d bytes). Use %s with this id and an offset to inspect it.", id, len(b.ToolOutput), recallTool)
			changed = true
		}
	}
	if changed {
		copyRequest.Tools = append(append([]agent.ToolDefinition(nil), r.Tools...), recallDefinition())
	}
	return copyRequest, changed
}

func recallDefinition() agent.ToolDefinition {
	return agent.ToolDefinition{Name: recallTool, Description: "Read a page of a prior tool result retained from this execution after context compaction.", InputSchema: json.RawMessage(`{"type":"object","properties":{"id":{"type":"string"},"offset":{"type":"integer","minimum":0}},"required":["id","offset"],"additionalProperties":false}`)}
}

func (p priorResults) read(raw json.RawMessage) (string, error) {
	var input struct {
		ID     string `json:"id"`
		Offset int    `json:"offset"`
	}
	if err := json.Unmarshal(raw, &input); err != nil {
		return "", fmt.Errorf("invalid prior result request")
	}
	content, ok := p[input.ID]
	if !ok || input.Offset < 0 || input.Offset > len(content) || (input.Offset < len(content) && !utf8.RuneStart(content[input.Offset])) {
		return "", fmt.Errorf("unknown result or invalid byte offset")
	}
	end := min(input.Offset+4096, len(content))
	for end < len(content) && !utf8.RuneStart(content[end]) {
		end--
	}
	page, _ := json.Marshal(map[string]any{"id": input.ID, "offset": input.Offset, "nextOffset": end, "totalBytes": len(content), "content": content[input.Offset:end]})
	return string(page), nil
}
