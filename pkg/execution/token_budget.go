package execution

import (
	"context"
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
}

func (p *boundedAPIAdapter) Complete(ctx context.Context, r agent.Request) (*agent.Response, error) {
	// Ollama may raise vision-capable model contexts to 2048 even for text.
	// Keep the input reservation above that native floor before any inference.
	if p.remaining < 4096 {
		return nil, fmt.Errorf("hard token grant cannot admit another inference")
	}
	output := min(int64(2048), p.remaining/4)
	input := min(int64(32768), p.remaining-output)
	// Debit the worst case before inference. Unknown usage never returns capacity.
	p.remaining -= input + output
	res, err := p.ProviderAdapter.(agent.HardTokenAdapter).CompleteBounded(ctx, r, int(input), int(output))
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
