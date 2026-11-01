package agent

import (
	"crypto/sha256"
	"encoding/json"
)

// Disposable admission metadata, not project memory or a capacity grant. Keep
// hashes rather than another copy of owner text or tool results. Only native
// successful usage can establish the baseline; estimates never become usage.
type nativeContextObservation struct {
	shape         [32]byte
	messages      [][32]byte
	tokens        int
	outputMessage [32]byte
	outputTokens  int
}

func nativeContextShape(body []byte) ([32]byte, []json.RawMessage, bool) {
	var fields map[string]json.RawMessage
	var messages []json.RawMessage
	if json.Unmarshal(body, &fields) != nil || json.Unmarshal(fields["messages"], &messages) != nil {
		return [32]byte{}, nil, false
	}
	delete(fields, "messages")
	// num_predict limits future output; it does not alter the serialized
	// prompt or its native token count. Bounded planning adjusts it between
	// calls to leave reply room, so keep that observation usable. Every other
	// option (including num_ctx), model, tool and message remains bound.
	var options map[string]json.RawMessage
	if json.Unmarshal(fields["options"], &options) == nil && options != nil {
		delete(options, "num_predict")
		fields["options"], _ = json.Marshal(options)
	}
	shape, err := json.Marshal(fields)
	return sha256.Sum256(shape), messages, err == nil
}

func (p *OpenAIProvider) observeNativeContext(body []byte, tokens int, outputMessage []byte, outputTokens int) {
	shape, messages, ok := nativeContextShape(body)
	if !ok || tokens <= 0 {
		return
	}
	o := &nativeContextObservation{shape: shape, tokens: tokens,
		outputMessage: sha256.Sum256(outputMessage), outputTokens: outputTokens}
	for _, message := range messages {
		o.messages = append(o.messages, sha256.Sum256(message))
	}
	p.contextObservationMu.Lock()
	p.contextObservation = o
	p.contextObservationMu.Unlock()
}

func (p *OpenAIProvider) admitObservedContext(body []byte, inputCap, outputCap, messages, tools int) error {
	// A first request or any changed system/model/tools/input options/prefix keeps
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
			// The retained prefix has a native count. Only its exact returned
			// assistant message may reuse the observed output count, including
			// reasoning and tool calls. Edited/generated elsewhere messages and
			// unique tool results retain the byte bound. Nothing is discarded.
			appended := o.tokens + 256
			added := wireMessages[len(o.messages):]
			if o.outputTokens > 0 && sha256.Sum256(added[0]) == o.outputMessage {
				appended += o.outputTokens + 32
				added = added[1:]
			}
			for _, message := range added {
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
