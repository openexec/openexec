package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

type NativeNonThinkingAdapter interface {
	SupportsNativeNonThinking() bool
	ValidateNativeNonThinking(context.Context, string) error
}

func (p *OpenAIProvider) SupportsNativeNonThinking() bool { return p.SupportsHardTokenBudget() }

// ValidateNativeNonThinking is metadata-only. The reviewed native version and
// structured architecture/template must support disabling thinking. A model
// display name (including a renamed GPT-OSS) is not a capability declaration.
func (p *OpenAIProvider) ValidateNativeNonThinking(ctx context.Context, model string) error {
	if !p.SupportsNativeNonThinking() || strings.TrimSpace(model) == "" {
		return fmt.Errorf("native non-thinking model support unavailable")
	}
	body, _ := json.Marshal(map[string]string{"model": model})
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimSuffix(p.ollamaBudgetURL, "/chat")+"/show", bytes.NewReader(body))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	client := *p.httpClient
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	response, err := client.Do(request)
	if err != nil {
		return fmt.Errorf("native non-thinking metadata unavailable")
	}
	defer response.Body.Close()
	var metadata struct {
		Details struct {
			Family string `json:"family"`
		} `json:"details"`
		Capabilities []string `json:"capabilities"`
		Template     string   `json:"template"`
	}
	if response.StatusCode != http.StatusOK || json.NewDecoder(io.LimitReader(response.Body, 4<<20)).Decode(&metadata) != nil {
		return fmt.Errorf("native non-thinking metadata invalid")
	}
	thinking := false
	for _, capability := range metadata.Capabilities {
		thinking = thinking || capability == "thinking"
	}
	// The initial supported contract is the inspected qwen35 native template.
	// Unknown families/templates need review, never a name-prefix fallback.
	if metadata.Details.Family != "qwen35" || !thinking || !strings.Contains(metadata.Template, "enable_thinking is defined and enable_thinking is false") {
		return fmt.Errorf("model does not declare reviewed native non-thinking support")
	}
	return nil
}
