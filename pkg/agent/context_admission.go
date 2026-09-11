package agent

import (
	"encoding/json"
	"fmt"
)

// ContextOverflowError distinguishes request capability from cumulative grant
// exhaustion. Dispatched must remain true for a provider refusal: the caller
// cannot infer zero usage merely from an HTTP status.
type ContextOverflowError struct {
	EstimatedTokens int
	ContextTokens   int
	RequestBytes    int
	Dispatched      bool
}

func (e *ContextOverflowError) Error() string {
	return fmt.Sprintf("context_overflow: assembled request bytes=%d estimated_tokens=%d context=%d dispatched=%t", e.RequestBytes, e.EstimatedTokens, e.ContextTokens, e.Dispatched)
}

// This is a deliberately padded estimate, NOT an exact native tokenizer or a
// proof of fit. Count the complete serialized native request (including tools,
// reasoning metadata and escaping), weight non-ASCII bytes at one token/byte,
// ASCII at three bytes/token plus 25%, and reserve framing overhead. Native
// truncate=false remains the final refusal boundary. Real provider evidence
// is required in addition to this preflight, particularly for dense code.
func assembledContextEstimate(body []byte, messages, tools int) int {
	ascii, other := 0, 0
	for _, b := range body {
		if b < 128 {
			ascii++
		} else {
			other++
		}
	}
	return (ascii*5+11)/12 + other + 256 + messages*32 + tools*32
}

func admitAssembledContext(body []byte, inputCap, outputCap, messages, tools int) error {
	estimate := assembledContextEstimate(body, messages, tools)
	if estimate+outputCap > inputCap {
		return &ContextOverflowError{EstimatedTokens: estimate, ContextTokens: inputCap, RequestBytes: len(body)}
	}
	return nil
}

// Inspect the already serialized request, rather than estimating only the
// user's text and missing schemas/tool results added by the adapter.
func boundedRequestShape(body []byte) (int, int) {
	var wire struct {
		Messages []json.RawMessage `json:"messages"`
		Tools    []json.RawMessage `json:"tools"`
	}
	_ = json.Unmarshal(body, &wire)
	return len(wire.Messages), len(wire.Tools)
}
