package cli

import (
	"testing"
	"time"

	"github.com/openexec/openexec/internal/project"
	"github.com/openexec/openexec/pkg/agent"
)

// A model on this machine is given the unattended budget; a hosted one keeps
// the interactive default, where two silent minutes still mean it is broken.
func TestLoopbackEndpointGetsUnattendedRequestBudget(t *testing.T) {
	for baseURL, want := range map[string]time.Duration{
		"http://127.0.0.1:11436/v1":          unattendedRequestBudget,
		"http://localhost:11434/v1":          unattendedRequestBudget,
		"http://[::1]:11436/v1":              unattendedRequestBudget,
		"https://api.moonshot.ai/v1":         agent.DefaultOpenAITimeout,
		"https://api.agentics.org.nz/v1":     agent.DefaultOpenAITimeout,
		"http://192.168.1.20:11436/v1":       agent.DefaultOpenAITimeout,
		"http://127.0.0.1.example.com:80/v1": agent.DefaultOpenAITimeout,
	} {
		adapter, err := configuredOpenAIAdapter("fixture",
			project.ProviderConfig{BaseURL: baseURL, APIKey: "local", Model: liveOllamaModel}, "local")
		if err != nil {
			t.Fatal(err)
		}
		if got := adapter.RequestTimeout(); got != want {
			t.Errorf("%s: request timeout = %v, want %v", baseURL, got, want)
		}
	}
}
