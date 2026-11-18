package agent

import (
	"strings"
	"testing"
)

func TestGetForModelReportsWhyOwnFamilyProviderIsUnavailable(t *testing.T) {
	reg := NewProviderRegistry()
	reg.RegisterFactory(ProviderGitHubCopilot, func() (ProviderAdapter, error) {
		return nil, &ProviderError{Code: ErrCodeAuthentication, Message: "factory-refusal-reason"}
	})
	if _, err := reg.GetForModel("copilot:gpt-4.1"); err == nil || !strings.Contains(err.Error(), "factory-refusal-reason") {
		t.Fatalf("own-family refusal hidden: %v", err)
	}
	// Near miss: another family's lookup must not surface the copilot refusal.
	if _, err := reg.GetForModel("gpt-4.1"); err == nil || strings.Contains(err.Error(), "factory-refusal-reason") {
		t.Fatalf("unrelated model reported copilot refusal: %v", err)
	}
}
