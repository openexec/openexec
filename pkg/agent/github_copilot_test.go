package agent

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func fakeCopilot(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "copilot"), []byte("#!/bin/sh\n"+body), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return dir
}

func userMsg(text string) Message {
	return Message{Role: RoleUser, Content: []ContentBlock{{Type: ContentTypeText, Text: text}}}
}

func TestGitHubCopilotRegisteredAndModelLookup(t *testing.T) {
	fakeCopilot(t, "echo hi\n")
	r := NewProviderRegistry()
	RegisterDefaultFactories(r)
	if !r.HasProvider(ProviderGitHubCopilot) {
		t.Fatal("github-copilot not registered")
	}
	for _, m := range []string{"copilot", "copilot:gpt-4.1"} {
		p, err := r.GetForModel(m)
		if err != nil || p.GetName() != string(ProviderGitHubCopilot) {
			t.Fatalf("GetForModel(%q) = %v, %v", m, p, err)
		}
	}
	// Near misses must not resolve to Copilot.
	for _, m := range []string{"copilotx", "not-copilot", "gpt-4"} {
		if p, err := r.GetForModel(m); err == nil && p.GetName() == string(ProviderGitHubCopilot) {
			t.Fatalf("%q resolved to copilot", m)
		}
	}
}

func TestGitHubCopilotCompleteRoundTripAndCredentialScope(t *testing.T) {
	dir := fakeCopilot(t, "printf '%s|%s|%s\\n' \"$*\" \"$OPENAI_API_KEY\" \"$COPILOT_GITHUB_TOKEN\" > \"$0.out\"\necho answer\n")
	t.Setenv("OPENAI_API_KEY", "sk-no")
	t.Setenv("COPILOT_GITHUB_TOKEN", "gho_fixture")
	p, err := NewGitHubCopilotProvider()
	if err != nil {
		t.Fatal(err)
	}
	resp, err := p.Complete(context.Background(), Request{Model: "copilot:gpt-4.1", Messages: []Message{userMsg("hello")}})
	if err != nil || resp.GetText() != "answer" {
		t.Fatalf("resp = %v, err = %v", resp, err)
	}
	out, _ := os.ReadFile(filepath.Join(dir, "copilot.out"))
	got := strings.TrimSpace(string(out))
	if !strings.Contains(got, "-s --no-ask-user --model gpt-4.1") || !strings.HasSuffix(got, "||gho_fixture") || strings.Contains(got, "sk-no") {
		t.Fatalf("invocation = %q", got)
	}
}

func TestGitHubCopilotRefusals(t *testing.T) {
	// Missing executable.
	t.Setenv("PATH", t.TempDir())
	if _, err := NewGitHubCopilotProvider(); err == nil || !strings.Contains(err.Error(), "copilot") {
		t.Fatalf("missing binary err = %v", err)
	}
	// Classic PAT refused without echoing it.
	fakeCopilot(t, "echo hi\n")
	t.Setenv("GH_TOKEN", "ghp_classic")
	if _, err := NewGitHubCopilotProvider(); err == nil || strings.Contains(err.Error(), "ghp_classic") {
		t.Fatalf("classic PAT err = %v", err)
	}
}

func TestGitHubCopilotCLIAuthFailurePropagates(t *testing.T) {
	fakeCopilot(t, "echo 'not signed in: run /login' >&2\nexit 1\n")
	t.Setenv("GH_TOKEN", "")
	p, err := NewGitHubCopilotProvider()
	if err != nil {
		t.Fatal(err)
	}
	_, err = p.Complete(context.Background(), Request{Model: "copilot", Messages: []Message{userMsg("x")}})
	if err == nil || !strings.Contains(err.Error(), "run /login") {
		t.Fatalf("err = %v, want CLI message and /login hint", err)
	}
	// Wrong model is refused before running anything.
	if _, err := p.Complete(context.Background(), Request{Model: "gpt-4", Messages: []Message{userMsg("x")}}); err == nil {
		t.Fatal("non-copilot model accepted")
	}
}
