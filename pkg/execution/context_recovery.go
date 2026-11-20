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
	return p.compactWith(r, true)
}

// guardsOff lets a test switch one named guard off to show, in the declared
// suite, what that guard prevents (a guard-removal negative control). Nothing
// outside tests ever writes to it, so every guard is on in production.
var guardsOff = map[string]bool{}

func guardOn(name string) bool { return !guardsOff[name] }

type pendingCall struct {
	name       string
	input      json.RawMessage
	occurrence int
}

// compactWith omits a result only by structured provenance, never by what its
// text says (and never one that is not valid UTF-8, which recall's JSON pages
// could not return byte-for-byte): a directory listing from a trusted executor (names alone), or a
// search whose own scan reported complete. File reads, failed results, partial
// scans and unknown tools can hold governing instructions and stay verbatim.
func (p priorResults) compactWith(r agent.Request, trustDirectories bool) (agent.Request, bool) {
	copyRequest := r
	copyRequest.Messages = append([]agent.Message(nil), r.Messages...)
	pending := map[string][]pendingCall{}
	seenCalls := map[string]int{}
	ambiguous := map[string]bool{}
	eligible := map[[2]int]pendingCall{}
	lastAssistant := -1
	for i, message := range r.Messages {
		if message.Role == agent.RoleAssistant {
			lastAssistant = i
		}
		for j, block := range message.Content {
			switch block.Type {
			case agent.ContentTypeToolUse:
				if len(pending[block.ToolUseID]) > 0 {
					ambiguous[block.ToolUseID] = true
				}
				pending[block.ToolUseID] = append(pending[block.ToolUseID], pendingCall{block.ToolName, block.ToolInput, seenCalls[block.ToolUseID]})
				seenCalls[block.ToolUseID]++
			case agent.ContentTypeToolResult:
				calls := pending[block.ToolResultID]
				if len(calls) > 0 {
					if !ambiguous[block.ToolResultID] || !guardOn("ambiguity") {
						eligible[[2]int{i, j}] = calls[0]
					}
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
			call, ok := eligible[[2]int{i, j}]
			if b.Type != agent.ContentTypeToolResult || !ok || b.ToolError != "" || len(b.ToolOutput) < 256 || (!utf8.ValidString(b.ToolOutput) && guardOn("utf8")) {
				continue
			}
			switch {
			case call.name == "list_directory" && trustDirectories:
				kept++
				if kept <= 1 {
					continue
				}
				hash := sha256.Sum256([]byte(b.ToolOutput))
				id := hex.EncodeToString(hash[:])
				p[id] = b.ToolOutput
				b.ToolOutput = fmt.Sprintf("Prior directory listing retained as %s (%d bytes). Use %s with this id and an offset to inspect it.", id, len(b.ToolOutput), recallTool)
			case call.name == "search_files" && (b.ScanComplete || !guardOn("scan-complete")) && i < lastAssistant:
				// Results the model has not yet answered stay in view. The id
				// binds this exact call position and result, so a reused call
				// ID or an identical output elsewhere cannot stand in for it.
				hash := sha256.Sum256([]byte(fmt.Sprintf("search\x00%d\x00%s\x00%s\x00%s", call.occurrence, b.ToolResultID, call.input, b.ToolOutput)))
				id := hex.EncodeToString(hash[:])
				p[id] = b.ToolOutput
				b.ToolOutput = fmt.Sprintf("Omitted complete search_files result for call %q (use #%d, %d bytes) retained as %s. It has NOT been inspected in this context: use %s with this id and offset 0, then each nextOffset, before relying on it.", b.ToolResultID, call.occurrence, len(b.ToolOutput), id, recallTool)
			default:
				continue
			}
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
	output, _, err := p.page(raw)
	return output, err
}

// recalledPage says how far into one omitted result a page reached.
type recalledPage struct {
	id         string
	offset     int
	next, size int
}

func (p priorResults) page(raw json.RawMessage) (string, recalledPage, error) {
	var input struct {
		ID     string `json:"id"`
		Offset int    `json:"offset"`
	}
	if err := json.Unmarshal(raw, &input); err != nil {
		return "", recalledPage{}, fmt.Errorf("invalid prior result request")
	}
	content, ok := p[input.ID]
	if !ok || input.Offset < 0 || input.Offset > len(content) || (input.Offset < len(content) && !utf8.RuneStart(content[input.Offset])) {
		return "", recalledPage{}, fmt.Errorf("unknown result or invalid byte offset")
	}
	end := min(input.Offset+4096, len(content))
	for end < len(content) && !utf8.RuneStart(content[end]) {
		end--
	}
	page, _ := json.Marshal(map[string]any{"id": input.ID, "offset": input.Offset, "nextOffset": end, "totalBytes": len(content), "content": content[input.Offset:end]})
	return string(page), recalledPage{input.ID, input.Offset, end, len(content)}, nil
}
