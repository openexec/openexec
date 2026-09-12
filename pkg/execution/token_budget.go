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

type boundedAPIAdapter struct {
	agent.ProviderAdapter
	remaining     int64
	input, output int64
	unknown       bool
	sink          EventSink
	contextCap    int64
	contextLimit  int64
}

var errHardTokenGrantExhausted = errors.New("hard token grant cannot admit another inference")

func (p *boundedAPIAdapter) Complete(ctx context.Context, r agent.Request) (*agent.Response, error) {
	// Ollama may raise vision-capable model contexts to 2048 even for text.
	// Keep the input reservation above that native floor before any inference.
	if p.remaining < 4096 {
		return nil, errHardTokenGrantExhausted
	}
	const output int64 = 2048
	if p.contextCap == 0 {
		// Admit one stable context for this execution, using native-sized
		// powers of two. Do not shrink it as prior requests consume capacity:
		// that made a valid 23K prompt reach a newly reduced 16K context.
		p.contextCap = 2048
		ceiling := int64(32768)
		if p.contextLimit > 0 && p.contextLimit < ceiling {
			ceiling = p.contextLimit
		}
		for next := p.contextCap * 2; next <= ceiling && next <= p.remaining-output; next *= 2 {
			p.contextCap = next
		}
	}
	input := p.contextCap
	if p.remaining < input+output {
		// Conservative admission is intentional: preserve all request content
		// and yield known usage before HTTP, rather than guess token counts,
		// truncate authority, or enlarge the remaining grant.
		return nil, errHardTokenGrantExhausted
	}
	// Debit the worst case before inference. Unknown usage never returns capacity.
	p.remaining -= input + output
	res, err := p.ProviderAdapter.(agent.HardTokenAdapter).CompleteBounded(ctx, r, int(input), int(output))
	var overflow *agent.ContextOverflowError
	if errors.As(err, &overflow) && !overflow.Dispatched {
		// No inference was submitted. Rebuild once by removing only exact
		// duplicate results. Unique obligations and owner authority stay intact.
		if compacted, changed := compactRepeatedResults(r); changed {
			res, err = p.ProviderAdapter.(agent.HardTokenAdapter).CompleteBounded(ctx, compacted, int(input), int(output))
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
