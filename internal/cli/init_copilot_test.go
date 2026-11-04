package cli

import (
	"bufio"
	"strings"
	"testing"
)

func TestGitHubCopilotPreset(t *testing.T) {
	var preset *providerPreset
	for i := range providerPresets {
		if providerPresets[i].Key == "githubcopilot" {
			preset = &providerPresets[i]
		}
	}
	if preset == nil {
		t.Fatal("githubcopilot preset missing")
	}
	if providerPresets[0].Key != "agenticsnz" {
		t.Error("default (first) preset must stay agenticsnz")
	}

	name, cfg := promptPresetProvider(bufio.NewReader(strings.NewReader("\n\n\n")), *preset)
	if name != "githubcopilot" || cfg == nil {
		t.Fatalf("got %q %+v", name, cfg)
	}
	if cfg.BaseURL != "https://api.githubcopilot.com" {
		t.Errorf("base url = %q", cfg.BaseURL)
	}
	if cfg.APIKey != "$GITHUB_COPILOT_API_KEY" {
		t.Errorf("api key = %q", cfg.APIKey)
	}
}
