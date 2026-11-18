package agent

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// ProviderGitHubCopilot is GitHub Copilot, reached through GitHub's own
// `copilot` CLI (programmatic mode: copilot -p <prompt> -s). GitHub documents
// no direct chat-completions API for Copilot subscriptions, so sign-in and
// token refresh stay with the CLI; this adapter only runs it and reports its
// failure. See docs/CONFIGURATION.md.
const ProviderGitHubCopilot ProviderName = "github-copilot"

// copilotModelPrefix selects a CLI model: "copilot" (the CLI default) or
// "copilot:<model>", matching internal/runner.IsCopilotModel.
const copilotModelPrefix = "copilot"

type copilotAdapter struct {
	bin string
}

// NewGitHubCopilotProvider returns the adapter, or an error when the copilot
// binary is not installed or a token variable holds a classic PAT (ghp_).
// No OpenAI/Anthropic key is ever consulted.
func NewGitHubCopilotProvider() (ProviderAdapter, error) {
	bin, err := exec.LookPath("copilot")
	if err != nil {
		return nil, &ProviderError{Code: ErrCodeNotFound, Message: "runner \"copilot\" not found on PATH; install GitHub Copilot CLI (npm install -g @github/copilot) and sign in with /login"}
	}
	if _, err := copilotChildEnv(os.Environ()); err != nil {
		return nil, &ProviderError{Code: ErrCodeAuthentication, Message: err.Error()}
	}
	return &copilotAdapter{bin: bin}, nil
}

// copilotChildEnv drops other vendors' keys and refuses classic PATs.
func copilotChildEnv(environ []string) ([]string, error) {
	drop := map[string]bool{"OPENAI_API_KEY": true, "ANTHROPIC_API_KEY": true, "CODEX_API_KEY": true, "GEMINI_API_KEY": true}
	out := make([]string, 0, len(environ))
	for _, e := range environ {
		name, value, _ := strings.Cut(e, "=")
		if drop[name] {
			continue
		}
		switch name {
		case "COPILOT_GITHUB_TOKEN", "GH_TOKEN", "GITHUB_TOKEN":
			if strings.HasPrefix(value, "ghp_") {
				return nil, fmt.Errorf("%s: GitHub Copilot does not accept classic personal access tokens (ghp_); sign in with 'copilot' (/login) or use a gho_, ghu_ or github_pat_ token", name)
			}
		}
		out = append(out, e)
	}
	return out, nil
}

func (a *copilotAdapter) GetName() string { return string(ProviderGitHubCopilot) }

// GetModels lists only the CLI-default selector; any "copilot:<model>" is
// accepted and passed to --model, since the installed CLI owns its model list.
func (a *copilotAdapter) GetModels() []string { return []string{copilotModelPrefix} }

func (a *copilotAdapter) GetModelInfo(modelID string) (*ModelInfo, error) {
	if !isCopilotModel(modelID) {
		return nil, &ProviderError{Code: ErrCodeNotFound, Message: fmt.Sprintf("model %q is not a GitHub Copilot model", modelID)}
	}
	caps, _ := a.GetCapabilities(modelID)
	return &ModelInfo{ID: modelID, Name: "GitHub Copilot CLI", Provider: a.GetName(), Capabilities: *caps}, nil
}

func (a *copilotAdapter) GetCapabilities(modelID string) (*ProviderCapabilities, error) {
	return &ProviderCapabilities{Streaming: true, SystemPrompt: true}, nil
}

func isCopilotModel(m string) bool {
	m = strings.ToLower(strings.TrimSpace(m))
	return m == copilotModelPrefix || strings.HasPrefix(m, copilotModelPrefix+":")
}

// SupportsModel accepts "copilot" and "copilot:<model>" for registry lookup.
func (a *copilotAdapter) SupportsModel(modelID string) bool { return isCopilotModel(modelID) }

func (a *copilotAdapter) ValidateRequest(req Request) error {
	if !isCopilotModel(req.Model) {
		return &ProviderError{Code: ErrCodeInvalidRequest, Message: fmt.Sprintf("model %q is not a GitHub Copilot model", req.Model)}
	}
	if len(req.Tools) > 0 {
		return &ProviderError{Code: ErrCodeInvalidRequest, Message: "GitHub Copilot CLI adapter does not accept caller-supplied tools"}
	}
	if len(req.Messages) == 0 {
		return &ProviderError{Code: ErrCodeInvalidRequest, Message: "at least one message is required"}
	}
	return nil
}

func (a *copilotAdapter) EstimateTokens(content string) int { return len(content) / 4 }

func (a *copilotAdapter) Complete(ctx context.Context, req Request) (*Response, error) {
	if err := a.ValidateRequest(req); err != nil {
		return nil, err
	}
	env, err := copilotChildEnv(os.Environ())
	if err != nil {
		return nil, &ProviderError{Code: ErrCodeAuthentication, Message: err.Error()}
	}
	var prompt strings.Builder
	if req.System != "" {
		prompt.WriteString(req.System + "\n\n")
	}
	for _, m := range req.Messages {
		prompt.WriteString(m.GetText() + "\n")
	}
	args := []string{"-p", prompt.String(), "-s", "--no-ask-user"}
	if _, name, ok := strings.Cut(req.Model, ":"); ok && strings.TrimSpace(name) != "" {
		args = append(args, "--model", strings.TrimSpace(name))
	}
	cmd := exec.CommandContext(ctx, a.bin, args...) // #nosec G204
	cmd.Env = env
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		// The CLI's own message is the actionable part; pass it through.
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = strings.TrimSpace(stdout.String())
		}
		return nil, &ProviderError{Code: ErrCodeAuthentication, Message: fmt.Sprintf("copilot CLI failed: %v: %s (if this is a sign-in problem, run 'copilot' and use /login)", err, msg)}
	}
	return &Response{
		Model:      req.Model,
		Content:    []ContentBlock{{Type: ContentTypeText, Text: strings.TrimSpace(stdout.String())}},
		StopReason: StopReasonEnd,
	}, nil
}

// Stream runs the CLI to completion and delivers its text as one delta; the
// CLI's silent mode has no incremental protocol.
func (a *copilotAdapter) Stream(ctx context.Context, req Request) (<-chan StreamEvent, error) {
	resp, err := a.Complete(ctx, req)
	if err != nil {
		return nil, err
	}
	ch := make(chan StreamEvent, 3)
	ch <- StreamEvent{Type: StreamEventStart}
	ch <- StreamEvent{Type: StreamEventContentDelta, Delta: &StreamDelta{Text: resp.GetText()}}
	ch <- StreamEvent{Type: StreamEventStop, StopReason: StopReasonEnd}
	close(ch)
	return ch, nil
}
