package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// ErrReadinessUnsupported means configuration may be valid but this adapter or
// endpoint cannot verify metadata without inference. It is not an auth failure.
var ErrReadinessUnsupported = errors.New("non-inference readiness metadata unavailable")

// MetadataReadiness is optional; callers must never substitute a completion
// when an adapter cannot implement a non-inference readiness check.
type MetadataReadiness interface {
	CheckModelMetadata(context.Context, string) error
}

func (p *OpenAIProvider) CheckModelMetadata(ctx context.Context, model string) error {
	if strings.TrimSpace(model) == "" {
		return &ProviderError{Code: ErrCodeInvalidRequest, Message: "configured model is empty"}
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(p.config.BaseURL, "/")+"/models", nil)
	if err != nil {
		return fmt.Errorf("invalid model metadata endpoint")
	}
	p.setHeaders(req)
	client := *p.httpClient
	// A health request must not forward configured credentials to a redirect.
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	response, err := client.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return fmt.Errorf("model metadata request cancelled or timed out")
		}
		return fmt.Errorf("model metadata endpoint unavailable")
	}
	defer response.Body.Close()
	switch response.StatusCode {
	case http.StatusUnauthorized, http.StatusForbidden:
		return &ProviderError{Code: ErrCodeAuthentication, Message: "model metadata authentication refused"}
	case http.StatusNotFound, http.StatusMethodNotAllowed, http.StatusNotImplemented:
		return ErrReadinessUnsupported
	case http.StatusOK:
	default:
		return fmt.Errorf("model metadata returned HTTP %d", response.StatusCode)
	}
	const bodyLimit = 1 << 20
	body, err := io.ReadAll(io.LimitReader(response.Body, bodyLimit+1))
	if err != nil || len(body) > bodyLimit {
		return fmt.Errorf("model metadata response unreadable or exceeds limit")
	}
	var catalog struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &catalog); err != nil || catalog.Data == nil {
		return fmt.Errorf("model metadata response has no valid catalog")
	}
	// Only a negotiated native Ollama endpoint has the documented implicit
	// :latest tag (https://github.com/ollama/ollama/blob/main/docs/api.md).
	// Hosted OpenAI-compatible catalogs and explicit tags remain exact.
	nativeDefault := ""
	if p.ollamaBudgetURL != "" && !strings.Contains(model[strings.LastIndex(model, "/")+1:], ":") && !strings.Contains(model, "@") {
		nativeDefault = model + ":latest"
	}
	for _, entry := range catalog.Data {
		if entry.ID == model || (nativeDefault != "" && entry.ID == nativeDefault) {
			return nil
		}
	}
	return &ProviderError{Code: ErrCodeNotFound, Message: "configured model absent from metadata catalog"}
}
