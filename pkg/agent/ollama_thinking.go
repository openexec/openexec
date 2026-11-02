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

// NativeContextWindow reads model metadata from the already configured local
// endpoint. Unknown or ambiguous metadata is not a model capability claim.
func (p *OpenAIProvider) NativeContextWindow(ctx context.Context, model string) (int64, error) {
	if !p.SupportsHardTokenBudget() || strings.TrimSpace(model) == "" {
		return 0, fmt.Errorf("native context metadata unavailable")
	}
	body, _ := json.Marshal(map[string]string{"model": model})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimSuffix(p.ollamaBudgetURL, "/chat")+"/show", bytes.NewReader(body))
	if err != nil {
		return 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	client := *p.httpClient
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	var data struct {
		ModelInfo map[string]json.RawMessage `json:"model_info"`
	}
	if resp.StatusCode != http.StatusOK || json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&data) != nil {
		return 0, fmt.Errorf("native context metadata invalid")
	}
	var window int64
	for key, raw := range data.ModelInfo {
		if !strings.HasSuffix(key, ".context_length") {
			continue
		}
		var n int64
		if json.Unmarshal(raw, &n) != nil || n <= 0 || (window != 0 && window != n) {
			return 0, fmt.Errorf("native context metadata ambiguous")
		}
		window = n
	}
	if window == 0 {
		return 0, fmt.Errorf("native context metadata unavailable")
	}
	return window, nil
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
