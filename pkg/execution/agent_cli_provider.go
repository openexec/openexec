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
	if config.Kind != "claude" && config.Kind != "codex" {
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
			Streaming: true, Resume: true, Cancellation: true, ReadOnly: true,
			WorkspaceWrite: true, CommandNetwork: p.config.Kind == "codex", ToolCalling: true,
		},
	}
}

func (p *AgentCLIProvider) Probe(ctx context.Context, dir string) Readiness {
	bin, err := executable(p.config.Binary, p.config.SearchPath)
	if err != nil {
		return Readiness{State: ReadinessNotInstalled, Problem: err.Error()}
	}
	probeCtx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	var args []string
	if p.config.Kind == "claude" {
		args = []string{"-p", "Reply with exactly: ok", "--output-format", "text", "--permission-mode", "plan"}
	} else {
		args = []string{"exec", "-C", dir, "--sandbox", "read-only", "--skip-git-repo-check", "Reply with exactly: ok"}
	}
	cmd := exec.CommandContext(probeCtx, bin, args...)
	cmd.Dir, cmd.Env = dir, safeCLIEnv(p.config.SearchPath)
	output, err := cmd.CombinedOutput()
	if err == nil {
		return Readiness{State: ReadinessReady}
	}
	problem := compactOutput(output, err)
	lower := strings.ToLower(problem)
	state := ReadinessUnhealthy
	for _, marker := range []string{"login", "log in", "auth", "credential", "unauthorized", "api key"} {
		if strings.Contains(lower, marker) {
			state = ReadinessNeedsLogin
			break
		}
	}
	return Readiness{State: state, Problem: problem}
}

func (p *AgentCLIProvider) Execute(ctx context.Context, req Request, sink EventSink) (Result, error) {
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
	cmd.Dir, cmd.Env = req.WorkingDir, safeCLIEnv(p.config.SearchPath)
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

func safeCLIEnv(path string) []string {
	allowed := map[string]bool{"HOME": true, "USER": true, "LOGNAME": true, "TMPDIR": true, "LANG": true, "LC_ALL": true, "TERM": true, "XDG_CONFIG_HOME": true, "XDG_DATA_HOME": true, "CODEX_HOME": true, "CLAUDE_CONFIG_DIR": true}
	out := []string{"PATH=" + path, "NO_COLOR=1"}
	for _, entry := range os.Environ() {
		name, _, ok := strings.Cut(entry, "=")
		if ok && allowed[name] {
			out = append(out, entry)
		}
	}
	return out
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
