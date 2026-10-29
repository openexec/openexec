package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// HardTokenAdapter bounds each inference before submitting it. The execution
// controller reserves the returned context+output allowance before calling it.
// CLI telemetry and prompt instructions cannot implement this interface.
type HardTokenAdapter interface {
	SupportsHardTokenBudget() bool
	CompleteBounded(context.Context, Request, int, int) (*Response, error)
}

// NativeCompletionObservation contains only bounded protocol facts, never
// response text, private reasoning, tool arguments or request content.
type NativeCompletionObservation struct {
	NonThinkingRequested bool   `json:"nonThinkingRequested,omitempty"`
	Reason               string `json:"reason"`
	Input                int    `json:"input"`
	Output               int    `json:"output"`
	InputCap             int    `json:"inputCap"`
	OutputCap            int    `json:"outputCap"`
	HasText              bool   `json:"hasText"`
	HasReasoning         bool   `json:"hasReasoning"`
	ToolCalls            int    `json:"toolCalls"`
}

// EnableLocalOllamaBounds negotiates only with the already configured loopback
// endpoint. It does not probe with inference or contact a hosted fallback.
func (p *OpenAIProvider) EnableLocalOllamaBounds(ctx context.Context) bool {
	u, err := url.Parse(p.config.BaseURL)
	if err != nil || u.Scheme != "http" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return false
	}
	ip := net.ParseIP(u.Hostname())
	if ip == nil || !ip.IsLoopback() {
		return false
	}
	if strings.TrimRight(u.Path, "/") != "/v1" {
		return false
	}
	u.Path = "/api/version"
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	client := *p.httpClient
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	res, err := client.Do(req)
	if err != nil {
		return false
	}
	defer res.Body.Close()
	var v struct {
		Version string `json:"version"`
	}
	if res.StatusCode != 200 || json.NewDecoder(io.LimitReader(res.Body, 4096)).Decode(&v) != nil {
		return false
	}
	// Pin the reviewed native protocol. Unknown versions must be validated before
	// claiming input/output admission, even if their endpoint looks compatible.
	if v.Version != "0.32.13" {
		return false
	}
	u.Path = "/api/chat"
	p.ollamaBudgetURL = u.String()
	return true
}
func (p *OpenAIProvider) SupportsHardTokenBudget() bool { return p.ollamaBudgetURL != "" }

func (p *OpenAIProvider) CompleteBounded(ctx context.Context, req Request, inputCap, outputCap int) (*Response, error) {
	if !p.SupportsHardTokenBudget() || inputCap < 2048 || outputCap < 1 {
		return nil, fmt.Errorf("hard token admission unavailable")
	}
	return p.completeLocalOllama(ctx, req, inputCap, outputCap)
}

// Zero caps preserve the configured model's context and generation defaults;
// they do not create a grant or claim bounded usage. Both routes retain the
// native no-truncation refusal and report actual usage through the same parser.
func (p *OpenAIProvider) completeLocalOllama(ctx context.Context, req Request, inputCap, outputCap int) (*Response, error) {
	messages := []map[string]any{}
	if req.System != "" {
		messages = append(messages, map[string]any{"role": "system", "content": req.System})
	}
	names := map[string]string{}
	for _, m := range req.Messages {
		msg := map[string]any{"role": string(m.Role), "content": m.GetText()}
		calls := []map[string]any{}
		for _, b := range m.Content {
			switch b.Type {
			case ContentTypeImage:
				return nil, fmt.Errorf("bounded route does not admit media tokens")
			case ContentTypeToolUse:
				var args map[string]any
				if json.Unmarshal(b.ToolInput, &args) != nil {
					return nil, fmt.Errorf("invalid bounded tool arguments")
				}
				calls = append(calls, map[string]any{"function": map[string]any{"name": b.ToolName, "arguments": args}})
				names[b.ToolUseID] = b.ToolName
			case ContentTypeToolResult:
				name := names[b.ToolResultID]
				if name == "" {
					return nil, fmt.Errorf("unbound tool result")
				}
				msg["tool_name"] = name
				msg["content"] = b.ToolOutput
				if b.ToolError != "" {
					msg["content"] = b.ToolError
				}
			}
		}
		if len(calls) > 0 {
			msg["tool_calls"] = calls
		}
		if thinking, ok := m.Metadata["thinking"].(string); ok {
			msg["thinking"] = thinking
		}
		messages = append(messages, msg)
	}
	tools := []map[string]any{}
	if req.ToolChoice != "none" {
		for _, t := range req.Tools {
			tools = append(tools, map[string]any{"type": "function", "function": map[string]any{"name": t.Name, "description": t.Description, "parameters": t.InputSchema}})
		}
	}
	options := map[string]any{}
	if inputCap > 0 {
		options["num_ctx"], options["num_predict"] = inputCap, outputCap
	} else {
		if req.MaxTokens > 0 {
			options["num_predict"] = req.MaxTokens
		}
		if req.Temperature != nil {
			options["temperature"] = req.Temperature
		}
		if req.TopP != nil {
			options["top_p"] = req.TopP
		}
		if len(req.StopSequences) > 0 {
			options["stop"] = req.StopSequences
		}
	}
	wireBody := map[string]any{"model": req.Model, "messages": messages, "tools": tools, "stream": false, "truncate": false, "options": options}
	if req.NonThinking {
		if err := p.ValidateNativeNonThinking(ctx, req.Model); err != nil {
			return nil, err
		}
		wireBody["think"] = false
	}
	body, err := json.Marshal(wireBody)
	if err != nil {
		return nil, err
	}
	messageCount, toolCount := boundedRequestShape(body)
	if inputCap > 0 {
		if err := p.admitObservedContext(body, inputCap, outputCap, messageCount, toolCount); err != nil {
			return nil, err
		}
	}
	wire, err := http.NewRequestWithContext(ctx, http.MethodPost, p.ollamaBudgetURL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	wire.Header.Set("Content-Type", "application/json")
	client := *p.httpClient
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	sent := time.Now()
	p.remember(body, wire)
	res, err := client.Do(wire)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, p.handleHTTPError(err, time.Since(sent), len(body))
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		// Retain only the classification, never raw provider content that may
		// contain prompt text or credentials. The bounded body is diagnostic
		// input, not evidence that an unsuccessful request consumed nothing.
		diagnostic, _ := io.ReadAll(io.LimitReader(res.Body, 8192))
		lower := strings.ToLower(string(diagnostic))
		if res.StatusCode == 400 && strings.Contains(lower, "context") && (strings.Contains(lower, "exceed") || strings.Contains(lower, "too long")) {
			return nil, &ContextOverflowError{EstimatedTokens: assembledContextEstimate(body, messageCount, toolCount), ContextTokens: inputCap, RequestBytes: len(body), Dispatched: true}
		}
		// Reuse the compatibility error contract without exposing native error
		// bodies, which may contain private prompt text.
		sanitized := *res
		sanitized.Body = io.NopCloser(strings.NewReader(fmt.Sprintf(`{"error":{"message":"local inference HTTP %d"}}`, res.StatusCode)))
		return nil, p.parseErrorResponse(&sanitized)
	}
	var data struct {
		Done       bool   `json:"done"`
		DoneReason string `json:"done_reason"`
		Prompt     *int   `json:"prompt_eval_count"`
		Output     *int   `json:"eval_count"`
		Message    struct {
			Content  string `json:"content"`
			Thinking string `json:"thinking"`
			Calls    []struct {
				Function struct {
					Name      string          `json:"name"`
					Arguments json.RawMessage `json:"arguments"`
				} `json:"function"`
			} `json:"tool_calls"`
		} `json:"message"`
	}
	if err = json.NewDecoder(io.LimitReader(res.Body, 8<<20)).Decode(&data); err != nil {
		return nil, err
	}
	if !data.Done || data.Prompt == nil || data.Output == nil || *data.Prompt < 0 || *data.Output < 0 || (inputCap > 0 && (*data.Prompt > inputCap || *data.Output > outputCap)) {
		return nil, fmt.Errorf("bounded local inference returned invalid usage")
	}
	p.observeNativeContext(body, *data.Prompt)
	result := &Response{Model: req.Model, Usage: Usage{PromptTokens: *data.Prompt, CompletionTokens: *data.Output, TotalTokens: *data.Prompt + *data.Output}, Metadata: map[string]any{"thinking": data.Message.Thinking}}
	// Whitelist native reasons: an unexpected provider string is not safe
	// diagnostic text and must never escape into an owner-visible error.
	reason := "unknown"
	switch data.DoneReason {
	case "stop", "length", "load", "unload":
		reason = data.DoneReason
	}
	result.StopReason = StopReasonEnd
	if data.DoneReason == "length" {
		result.StopReason = StopReasonMaxTokens
	} else if len(data.Message.Calls) > 0 {
		result.StopReason = StopReasonToolUse
	}
	result.Metadata["native_completion"] = NativeCompletionObservation{
		NonThinkingRequested: req.NonThinking,
		Reason:               reason, Input: *data.Prompt, Output: *data.Output,
		InputCap: inputCap, OutputCap: outputCap,
		HasText:      strings.TrimSpace(data.Message.Content) != "",
		HasReasoning: strings.TrimSpace(data.Message.Thinking) != "", ToolCalls: len(data.Message.Calls),
	}
	if data.Message.Content != "" {
		result.Content = append(result.Content, ContentBlock{Type: ContentTypeText, Text: data.Message.Content})
	}
	for i, c := range data.Message.Calls {
		result.Content = append(result.Content, ContentBlock{Type: ContentTypeToolUse, ToolUseID: fmt.Sprintf("bounded-%d-%d", time.Now().UnixNano(), i), ToolName: c.Function.Name, ToolInput: c.Function.Arguments})
	}
	return result, nil
}
