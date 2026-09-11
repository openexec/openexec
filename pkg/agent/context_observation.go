package agent

import (
	"crypto/sha256"
	"encoding/json"
)

// Disposable admission metadata, not project memory or a capacity grant. Keep
// hashes rather than another copy of owner text or tool results. Only native
// successful usage can establish the baseline; estimates never become usage.
type nativeContextObservation struct {
	shape    [32]byte
	messages [][32]byte
	tokens   int
}

func nativeContextShape(body []byte) ([32]byte, []json.RawMessage, bool) {
	var fields map[string]json.RawMessage
	var messages []json.RawMessage
	if json.Unmarshal(body, &fields) != nil || json.Unmarshal(fields["messages"], &messages) != nil {
		return [32]byte{}, nil, false
	}
	delete(fields, "messages")
	shape, err := json.Marshal(fields)
	return sha256.Sum256(shape), messages, err == nil
}

func (p *OpenAIProvider) observeNativeContext(body []byte, tokens int) {
	shape, messages, ok := nativeContextShape(body)
	if !ok || tokens <= 0 {
		return
	}
	o := &nativeContextObservation{shape: shape, tokens: tokens}
	for _, message := range messages {
		o.messages = append(o.messages, sha256.Sum256(message))
	}
	p.contextObservationMu.Lock()
	p.contextObservation = o
	p.contextObservationMu.Unlock()
}

func (p *OpenAIProvider) admitObservedContext(body []byte, inputCap, outputCap, messages, tools int) error {
	// A first request or any changed system/model/tools/options/prefix keeps
	// the existing conservative whole-request preflight. Never subtract an
	// estimate from actual accounting or assume edited content costs the same.
	estimate := assembledContextEstimate(body, messages, tools)
	shape, wireMessages, ok := nativeContextShape(body)
	p.contextObservationMu.Lock()
	o := p.contextObservation
	p.contextObservationMu.Unlock()
	if ok && o != nil && shape == o.shape && len(wireMessages) > len(o.messages) {
		unchanged := true
		for i, hash := range o.messages {
			if sha256.Sum256(wireMessages[i]) != hash {
				unchanged = false
				break
			}
		}
		if unchanged {
			// The retained prefix has a native count. Charge every serialized
			// added byte as a token, plus framing slack. This includes thinking,
			// tool arguments/results and escaping; no content is discarded.
			appended := o.tokens + 256
			for _, message := range wireMessages[len(o.messages):] {
				appended += len(message) + 32
			}
			if appended < estimate {
				estimate = appended
			}
		}
	}
	if estimate+outputCap > inputCap {
		return &ContextOverflowError{EstimatedTokens: estimate, ContextTokens: inputCap, RequestBytes: len(body)}
	}
	return nil
}
