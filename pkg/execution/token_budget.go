package execution

import (
	"context"
	"errors"
	"fmt"
	"github.com/openexec/openexec/pkg/agent"
)

func supportsHardTokens(p agent.ProviderAdapter) bool {
	b, ok := p.(agent.HardTokenAdapter)
	return ok && b.SupportsHardTokenBudget()
}

func supportsNativeNonThinking(p agent.ProviderAdapter) bool {
	b, ok := p.(agent.NativeNonThinkingAdapter)
	return ok && b.SupportsNativeNonThinking()
}

type boundedAPIAdapter struct {
	agent.ProviderAdapter
	remaining     int64
	input, output int64
	unknown       bool
	sink          EventSink
	contextCap    int64
	contextLimit  int64
	nonThinking   bool
	prior         priorResults
}

var errHardTokenGrantExhausted = errors.New("hard token grant cannot admit another inference")

func (p *boundedAPIAdapter) Complete(ctx context.Context, r agent.Request) (*agent.Response, error) {
	r.NonThinking = p.nonThinking
	// Ollama may raise vision-capable model contexts to 2048 even for text.
	// Keep the input reservation above that native floor before any inference.
	if p.remaining < 4096 {
		return nil, errHardTokenGrantExhausted
	}
	const minimumOutput int64 = 2048
	if p.contextCap == 0 {
		// Admit one stable context for this execution, using native-sized
		// powers of two. Do not shrink it as prior requests consume capacity:
		// that made a valid 23K prompt reach a newly reduced 16K context.
		p.contextCap = 2048
		ceiling := MaxAPIContextTokens
		if p.contextLimit > 0 && p.contextLimit < ceiling {
			ceiling = p.contextLimit
		}
		for next := p.contextCap * 2; next <= ceiling && next <= p.remaining-minimumOutput; next *= 2 {
			p.contextCap = next
		}
	}
	input := p.contextCap
	if p.remaining < input+minimumOutput {
		// Conservative admission is intentional: preserve all request content
		// and yield known usage before HTTP, rather than guess token counts,
		// truncate authority, or enlarge the remaining grant.
		return nil, errHardTokenGrantExhausted
	}
	// Reasoning and the visible answer share this reservation. A native
	// planner exhausted a fixed 2K reply twice without returning any tasks.
	// Admit more output within the SAME grant and context ceiling; keep an
	// explicit caller maximum restrictive. The adapter's preflight below
	// also leaves room for the complete serialized prompt.
	output := min(input, p.remaining-input)
	if r.MaxTokens > 0 {
		output = min(output, int64(r.MaxTokens))
	}
	// Debit the worst case before inference. Unknown usage never returns capacity.
	p.remaining -= input + output
	prepare := func(request agent.Request) (*agent.Response, error) {
		res, err := p.ProviderAdapter.(agent.HardTokenAdapter).CompleteBounded(ctx, request, int(input), int(output))
		var overflow *agent.ContextOverflowError
		if errors.As(err, &overflow) && !overflow.Dispatched {
			room := input - int64(overflow.EstimatedTokens)
			if room >= minimumOutput && room < output {
				// No inference was submitted. Return only the unused output
				// reservation, then prepare once with room for the whole prompt.
				// Never shrink input, truncate authority, or retry a dispatched call.
				p.remaining += output - room
				output = room
				return p.ProviderAdapter.(agent.HardTokenAdapter).CompleteBounded(ctx, request, int(input), int(output))
			}
		}
		return res, err
	}
	if p.prior != nil {
		if compacted, changed := p.prior.compact(r); changed {
			r = compacted
		} else {
			r.Tools = append(append([]agent.ToolDefinition(nil), r.Tools...), recallDefinition())
		}
	}
	res, err := prepare(r)
	var overflow *agent.ContextOverflowError
	if errors.As(err, &overflow) && !overflow.Dispatched {
		// No inference was submitted. Rebuild once by removing only exact
		// duplicate results. Unique obligations and owner authority stay intact.
		if compacted, changed := compactRepeatedResults(r); changed {
			res, err = prepare(compacted)
		}
		overflow = nil
		if errors.As(err, &overflow) && !overflow.Dispatched && p.prior == nil && r.ToolChoice != "none" && len(r.Tools) > 0 {
			collision := false
			for _, tool := range r.Tools {
				if tool.Name == recallTool {
					collision = true
				}
			}
			if !collision {
				prior := priorResults{}
				if compacted, changed := prior.compact(r); changed {
					res, err = prepare(compacted)
					if err == nil {
						p.prior = prior
					}
				}
			}
		}
		overflow = nil
		if errors.As(err, &overflow) && !overflow.Dispatched {
			p.remaining += input + output
			return nil, err
		}
	}
	if err != nil {
		p.unknown = true
		return nil, err
	}
	if res == nil || res.Usage.PromptTokens < 0 || res.Usage.CompletionTokens < 0 || int64(res.Usage.PromptTokens) > input || int64(res.Usage.CompletionTokens) > output {
		p.unknown = true
		return nil, fmt.Errorf("provider violated token reservation")
	}
	// The native protocol reports complete prompt counts, including cached input.
	usedInput, usedOutput := int64(res.Usage.PromptTokens), int64(res.Usage.CompletionTokens)
	p.remaining += input + output - usedInput - usedOutput
	p.input += usedInput
	p.output += usedOutput
	if p.sink != nil {
		if err = p.sink(Event{Type: EventUsage, InputTokens: p.input, OutputTokens: p.output}); err != nil {
			return nil, err
		}
	}
	if p.nonThinking {
		thinking, _ := res.Metadata["thinking"].(string)
		if thinking != "" {
			return nil, fmt.Errorf("native provider ignored non-thinking mode")
		}
	}
	return res, nil
}
func (p *boundedAPIAdapter) Stream(ctx context.Context, r agent.Request) (<-chan agent.StreamEvent, error) {
	res, err := p.Complete(ctx, r)
	if err != nil {
		return nil, err
	}
	ch := make(chan agent.StreamEvent, 2)
	ch <- agent.StreamEvent{Type: agent.StreamEventContentDelta, Delta: &agent.StreamDelta{Text: res.GetText()}}
	close(ch)
	return ch, nil
}
