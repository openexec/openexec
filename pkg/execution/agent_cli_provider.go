package execution

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const maxCLIEventBytes = 16 << 20

// CopilotKind is the GitHub Copilot CLI (`copilot`). GitHub documents no direct
// chat-completions HTTP API for Copilot subscriptions: the supported routes are
// the Copilot CLI/SDK (sign-in, or a token in COPILOT_GITHUB_TOKEN, GH_TOKEN or
// GITHUB_TOKEN) and BYOK. The api.githubcopilot.com session-token exchange is
// an undocumented editor-client endpoint and is deliberately not used.
// See docs/ADDING_A_PROVIDER.md and docs/CONFIGURATION.md.
const CopilotKind = "copilot"

// copilotTokenEnv lists the token variables the Copilot CLI documents, in its
// priority order.
var copilotTokenEnv = []string{"COPILOT_GITHUB_TOKEN", "GH_TOKEN", "GITHUB_TOKEN"}

// classifyCopilotToken applies GitHub's documented prefix list: gho_ (OAuth
// user token), ghu_ (GitHub App user token) and github_pat_ (fine-grained PAT)
// are supported; ghp_ (classic PAT) is not.
func classifyCopilotToken(token string) (supported, known bool) {
	switch {
	case strings.HasPrefix(token, "gho_"), strings.HasPrefix(token, "ghu_"), strings.HasPrefix(token, "github_pat_"):
		return true, true
	case strings.HasPrefix(token, "ghp_"):
		return false, true
	}
	return false, false
}

// copilotEnvToken returns the first documented token variable that is set.
func copilotEnvToken() (name, value string) {
	for _, n := range copilotTokenEnv {
		if v := strings.TrimSpace(os.Getenv(n)); v != "" {
			return n, v
		}
	}
	return "", ""
}

const copilotClassicPATProblem = "GitHub Copilot does not accept classic personal access tokens (ghp_); sign in with 'copilot' (/login) or set COPILOT_GITHUB_TOKEN to an OAuth (gho_), GitHub App user (ghu_) or fine-grained (github_pat_) token"

type AgentCLIConfig struct {
	Kind       string
	Binary     string
	SearchPath string
	Models     []string
}

// AgentCLIProvider owns Claude Code and Codex process construction so callers
// do not duplicate sandbox, resume, streaming, or cancellation behavior.
type AgentCLIProvider struct {
	config AgentCLIConfig
}

func NewAgentCLIProvider(config AgentCLIConfig) (*AgentCLIProvider, error) {
	if config.Kind != "claude" && config.Kind != "codex" && config.Kind != CopilotKind {
		return nil, fmt.Errorf("unsupported agent CLI kind %q", config.Kind)
	}
	if strings.TrimSpace(config.Binary) == "" {
		config.Binary = config.Kind
	}
	return &AgentCLIProvider{config: config}, nil
}

func (p *AgentCLIProvider) Descriptor() ProviderDescriptor {
	return ProviderDescriptor{
		ID: p.config.Kind, Runtime: "cli", Models: append([]string(nil), p.config.Models...),
		Capabilities: Capability{
			NonInferenceReadiness: true,
			Streaming:             true, Resume: p.config.Kind != CopilotKind, Cancellation: true, ReadOnly: true,
			WorkspaceWrite: true, CommandNetwork: p.config.Kind == "codex", ToolCalling: true,
		},
	}
}

func (p *AgentCLIProvider) Probe(ctx context.Context, dir string) Readiness {
	bin, err := executable(p.config.Binary, p.config.SearchPath)
	if err != nil {
		return Readiness{State: ReadinessNotInstalled, Problem: err.Error()}
	}
	if p.config.Kind == CopilotKind {
		return probeCopilotCredential()
	}
	probeCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	var args []string
	if p.config.Kind == "claude" {
		args = []string{"auth", "status", "--json"}
	} else {
		args = []string{"login", "status"}
	}
	cmd := exec.CommandContext(probeCtx, bin, args...)
	cmd.Dir, cmd.Env = dir, safeCLIEnv(p.config.Kind, p.config.SearchPath)
	var output bytes.Buffer
	capture := &boundedWriter{writer: &output, remaining: 8192}
	cmd.Stdout, cmd.Stderr = capture, capture
	err = cmd.Run()
	if err == nil {
		if p.config.Kind == "claude" {
			var status struct {
				LoggedIn *bool `json:"loggedIn"`
			}
			if json.Unmarshal(output.Bytes(), &status) != nil || status.LoggedIn == nil {
				return Readiness{State: ReadinessUnknown, Check: "authentication-status", Problem: "authentication status format unavailable; inference not tested"}
			}
			if !*status.LoggedIn {
				return Readiness{State: ReadinessNeedsLogin, Check: "authentication-status", Problem: "provider is not signed in"}
			}
		}
		return Readiness{State: ReadinessReady, Check: "authentication-status"}
	}
	if probeCtx.Err() != nil {
		return Readiness{State: ReadinessUnhealthy, Check: "authentication-status", Problem: "authentication status cancelled or timed out"}
	}
	lower := strings.ToLower(output.String())
	for _, marker := range []string{"unknown", "unrecognized", "unsupported", "unexpected argument"} {
		if strings.Contains(lower, marker) {
			return Readiness{State: ReadinessUnknown, Check: "authentication-status", Problem: "authentication status command unavailable; inference not tested"}
		}
	}
	for _, marker := range []string{"login", "log in", "not logged in", "unauthorized", "credential"} {
		if strings.Contains(lower, marker) {
			return Readiness{State: ReadinessNeedsLogin, Check: "authentication-status", Problem: "provider is not signed in"}
		}
	}
	return Readiness{State: ReadinessUnknown, Check: "authentication-status", Problem: "authentication status unavailable; inference not tested"}
}

// probeCopilotCredential reports readiness without inference. The Copilot CLI
// documents no authentication-status command and keeps an interactive sign-in
// in the system keychain, which cannot be inspected from here; only a token in
// the documented environment variables can be judged.
func probeCopilotCredential() Readiness {
	name, token := copilotEnvToken()
	if token == "" {
		return Readiness{State: ReadinessUnknown, Check: "credential-environment",
			Problem: "no COPILOT_GITHUB_TOKEN, GH_TOKEN or GITHUB_TOKEN set; an interactive 'copilot' sign-in cannot be checked without inference"}
	}
	supported, known := classifyCopilotToken(token)
	switch {
	case known && !supported:
		return Readiness{State: ReadinessNeedsLogin, Check: "credential-environment", Problem: name + ": " + copilotClassicPATProblem}
	case supported:
		return Readiness{State: ReadinessReady, Check: "credential-environment"}
	}
	return Readiness{State: ReadinessUnknown, Check: "credential-environment", Problem: name + " has an unrecognised token prefix; inference not tested"}
}

func (p *AgentCLIProvider) Execute(ctx context.Context, req Request, sink EventSink) (Result, error) {
	if req.NonThinking {
		return Result{}, fmt.Errorf("CLI cannot select validated native non-thinking mode")
	}
	if req.ContextTokenLimit != 0 {
		return Result{}, fmt.Errorf("CLI cannot enforce a hard context limit")
	}
	if req.TokenBudget != 0 {
		return Result{}, fmt.Errorf("CLI cannot enforce a hard total-token grant")
	}
	if p.config.Kind == CopilotKind {
		if len(req.WritableRoots) != 0 {
			return Result{}, fmt.Errorf("copilot CLI cannot enforce writable roots")
		}
		if req.NetworkAccess {
			return Result{}, fmt.Errorf("copilot CLI cannot enforce command network policy")
		}
		if req.NativeSessionID != "" {
			return Result{}, fmt.Errorf("copilot CLI non-interactive mode cannot resume a session")
		}
		if name, token := copilotEnvToken(); token != "" {
			if supported, known := classifyCopilotToken(token); known && !supported {
				return Result{}, fmt.Errorf("%s: %s", name, copilotClassicPATProblem)
			}
		}
	}
	started := time.Now().UTC()
	result := Result{Executor: p.config.Kind, Model: req.Model, Sandbox: req.Sandbox, StartedAt: started, Outcome: OutcomeFailed}
	finish := func() { result.EndedAt = time.Now().UTC() }
	if sink == nil {
		sink = func(Event) error { return nil }
	}
	if err := validateProviderRequest(req); err != nil {
		finish()
		return result, err
	}
	bin, err := executable(p.config.Binary, p.config.SearchPath)
	if err != nil {
		finish()
		return result, err
	}
	result.Executor = bin
	args := p.arguments(req)
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Dir, cmd.Env = req.WorkingDir, safeCLIEnv(p.config.Kind, p.config.SearchPath)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		finish()
		return result, err
	}
	var stderr bytes.Buffer
	cmd.Stderr = &boundedWriter{writer: &stderr, remaining: 64 << 10}
	if err := sink(Event{Type: EventStarted}); err != nil {
		finish()
		return result, err
	}
	if err := cmd.Start(); err != nil {
		finish()
		return result, err
	}
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 64<<10), maxCLIEventBytes)
	var final strings.Builder
	for scanner.Scan() {
		event, nativeID, ok := parseCLIEvent(p.config.Kind, scanner.Bytes())
		if nativeID != "" {
			result.NativeSessionID = nativeID
		}
		if ok {
			final.WriteString(event.Text)
			if err := sink(event); err != nil {
				_ = cmd.Process.Kill()
				_ = cmd.Wait()
				finish()
				return result, err
			}
		}
	}
	result.FinalText = final.String()
	scanErr, waitErr := scanner.Err(), cmd.Wait()
	finish()
	if scanErr != nil {
		_ = sink(Event{Type: EventFailed, Text: scanErr.Error()})
		return result, scanErr
	}
	if waitErr != nil {
		if errors.Is(ctx.Err(), context.Canceled) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
			result.Outcome = OutcomeCancelled
			_ = sink(Event{Type: EventCancelled})
			return result, ctx.Err()
		}
		problem := compactOutput(stderr.Bytes(), waitErr)
		_ = sink(Event{Type: EventFailed, Text: problem})
		return result, fmt.Errorf("%s failed: %s", filepath.Base(bin), problem)
	}
	result.Outcome = OutcomeSucceeded
	if result.NativeSessionID == "" {
		result.NativeSessionID = req.NativeSessionID
	}
	if err := sink(Event{Type: EventCompleted}); err != nil {
		return result, err
	}
	return result, nil
}

func (p *AgentCLIProvider) arguments(req Request) []string {
	if p.config.Kind == CopilotKind {
		// -p runs one prompt non-interactively, -s prints only the answer and
		// --no-ask-user stops clarifying questions. Unattended runs deny any
		// tool that was not pre-approved, so read-only grants none and
		// workspace-write grants only file writes (no shell, no URLs).
		args := []string{"-p", req.Prompt, "-s", "--no-ask-user"}
		if req.Model != "" && req.Model != "default" {
			args = append(args, "--model", req.Model)
		}
		if req.Sandbox.Mode == "workspace-write" {
			args = append(args, "--allow-tool=write")
		}
		return args
	}
	if p.config.Kind == "claude" {
		args := []string{"-p", req.Prompt, "--output-format", "stream-json", "--verbose"}
		if req.NativeSessionID == "" {
			args = append(args, "--session-id", req.ID)
		} else {
			args = append(args, "--resume", req.NativeSessionID)
		}
		if req.Model != "" && req.Model != "default" {
			args = append(args, "--model", req.Model)
		}
		permission := "plan"
		if req.Sandbox.Mode == "workspace-write" {
			permission = "acceptEdits"
			for _, root := range req.WritableRoots {
				args = append(args, "--add-dir", root)
			}
		}
		return append(args, "--permission-mode", permission)
	}
	args := []string{"exec"}
	if req.NativeSessionID != "" {
		args = append(args, "resume", req.NativeSessionID, "-c", `sandbox_mode="`+req.Sandbox.Mode+`"`)
	} else {
		args = append(args, "-C", req.WorkingDir, "--sandbox", req.Sandbox.Mode)
	}
	if req.Sandbox.Mode == "workspace-write" && len(req.WritableRoots) > 0 {
		quoted := make([]string, len(req.WritableRoots))
		for i, root := range req.WritableRoots {
			quoted[i] = strconv.Quote(root)
		}
		args = append(args, "-c", "sandbox_workspace_write.writable_roots=["+strings.Join(quoted, ",")+"]")
	}
	if req.Sandbox.Mode == "workspace-write" {
		args = append(args, "-c", "sandbox_workspace_write.network_access="+strconv.FormatBool(req.NetworkAccess))
	}
	args = append(args, "--json")
	if req.Model != "" && req.Model != "default" {
		args = append(args, "--model", req.Model)
	}
	return append(args, req.Prompt)
}

func validateProviderRequest(req Request) error {
	if strings.TrimSpace(req.ID) == "" || strings.TrimSpace(req.WorkingDir) == "" || strings.TrimSpace(req.Prompt) == "" {
		return errors.New("execution ID, working directory, and prompt are required")
	}
	if req.Sandbox.Mode != "read-only" && req.Sandbox.Mode != "workspace-write" {
		return fmt.Errorf("agent CLI provider cannot enforce sandbox mode %q", req.Sandbox.Mode)
	}
	if req.Sandbox.Mode == "read-only" && len(req.WritableRoots) != 0 {
		return errors.New("read-only execution cannot declare writable roots")
	}
	if req.NetworkAccess && req.Sandbox.Mode != "workspace-write" {
		return errors.New("network access requires workspace-write mode")
	}
	for _, root := range req.WritableRoots {
		if !filepath.IsAbs(root) {
			return fmt.Errorf("writable root must be absolute: %q", root)
		}
	}
	return nil
}

func parseCLIEvent(kind string, line []byte) (Event, string, bool) {
	if kind == CopilotKind {
		// Silent mode prints plain answer text; there is no session id.
		text := string(line) + "\n"
		return Event{Type: EventAssistantDelta, Text: text}, "", len(strings.TrimSpace(text)) > 0
	}
	if kind == "codex" {
		var value struct {
			Type     string `json:"type"`
			ThreadID string `json:"thread_id"`
			Item     struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"item"`
		}
		if json.Unmarshal(line, &value) == nil && value.Type == "item.completed" && value.Item.Type == "agent_message" {
			return Event{Type: EventAssistantDelta, Text: value.Item.Text}, value.ThreadID, value.Item.Text != ""
		}
		return Event{}, value.ThreadID, false
	}
	var value struct {
		Type      string `json:"type"`
		SessionID string `json:"session_id"`
		Message   struct {
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		} `json:"message"`
	}
	if json.Unmarshal(line, &value) != nil || value.Type != "assistant" {
		return Event{}, value.SessionID, false
	}
	var text strings.Builder
	for _, block := range value.Message.Content {
		if block.Type == "text" {
			text.WriteString(block.Text)
		}
	}
	return Event{Type: EventAssistantDelta, Text: text.String()}, value.SessionID, text.Len() > 0
}

func executable(binary, searchPath string) (string, error) {
	if strings.ContainsRune(binary, filepath.Separator) {
		if info, err := os.Stat(binary); err == nil && !info.IsDir() && info.Mode()&0o111 != 0 {
			return binary, nil
		}
		return "", fmt.Errorf("provider executable %q is not executable", binary)
	}
	for _, dir := range filepath.SplitList(searchPath) {
		candidate := filepath.Join(dir, binary)
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() && info.Mode()&0o111 != 0 {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("provider executable %q was not found", binary)
}

func safeCLIEnv(kind, path string) []string {
	allowed := map[string]bool{"HOME": true, "USER": true, "LOGNAME": true, "TMPDIR": true, "LANG": true, "LC_ALL": true, "TERM": true, "XDG_CONFIG_HOME": true, "XDG_DATA_HOME": true, "CODEX_HOME": true, "CLAUDE_CONFIG_DIR": true}
	if kind == CopilotKind {
		// Only the Copilot CLI receives its documented credential variables;
		// claude and codex runs keep a GitHub token out of their environment.
		for _, name := range append([]string{"COPILOT_HOME"}, copilotTokenEnv...) {
			allowed[name] = true
		}
	}
	out := []string{"PATH=" + path, "NO_COLOR=1"}
	for _, entry := range os.Environ() {
		name, value, ok := strings.Cut(entry, "=")
		if ok && (allowed[name] || name == "GOCACHE" && isConfiguredGoCache(value)) {
			out = append(out, entry)
		}
	}
	return out
}

// isConfiguredGoCache reports whether a caller-configured GOCACHE is carried
// to the agent. Dropping it sent every nested go build back to the default
// cache under HOME, outside the caller's cache placement. It is a location,
// not a credential, and grants no writable root: the agent can write it only
// where the request's sandbox already allows. Go refuses a relative GOCACHE,
// so one is dropped and the agent keeps Go's default instead of failing.
func isConfiguredGoCache(value string) bool {
	return filepath.IsAbs(value)
}

func compactOutput(output []byte, fallback error) string {
	text := strings.TrimSpace(string(output))
	if len(text) > 400 {
		text = text[len(text)-400:]
	}
	if text == "" && fallback != nil {
		text = fallback.Error()
	}
	return text
}

type boundedWriter struct {
	writer    io.Writer
	remaining int
}

func (w *boundedWriter) Write(p []byte) (int, error) {
	size := len(p)
	if w.remaining > 0 {
		chunk := p
		if len(chunk) > w.remaining {
			chunk = chunk[:w.remaining]
		}
		_, _ = w.writer.Write(chunk)
		w.remaining -= len(chunk)
	}
	return size, nil
}

var _ Provider = (*AgentCLIProvider)(nil)
