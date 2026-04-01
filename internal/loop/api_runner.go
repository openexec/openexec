package loop

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/openexec/openexec/pkg/agent"
)

// DefaultMaxTurns is the default limit for agentic conversation turns.
const DefaultMaxTurns = 50

// APIRunnerConfig holds configuration for the API-based agentic runner.
type APIRunnerConfig struct {
	Provider agent.ProviderAdapter
	Model    string
	Prompt   string // System prompt
	WorkDir  string
	MaxTurns int // Max conversation turns (default 50)
	Tools    []agent.ToolDefinition
}

// APIRunner executes an agentic loop using an OpenAI-compatible HTTP API
// instead of a CLI subprocess. It emits the same Event types as the CLI loop.
type APIRunner struct {
	config APIRunnerConfig
	events chan<- Event
	tools  *MCPToolHandler
}

// NewAPIRunner creates a new API-based agentic runner.
func NewAPIRunner(cfg APIRunnerConfig, events chan<- Event) *APIRunner {
	if cfg.MaxTurns == 0 {
		cfg.MaxTurns = DefaultMaxTurns
	}
	return &APIRunner{
		config: cfg,
		events: events,
		tools: &MCPToolHandler{
			workDir: cfg.WorkDir,
		},
	}
}

// Run executes the agentic loop: send messages to the API, execute tool calls
// locally, and continue until the model stops requesting tools or max turns is reached.
func (r *APIRunner) Run(ctx context.Context) error {
	defer close(r.events)

	messages := []agent.Message{
		agent.NewTextMessage(agent.RoleUser, r.config.Prompt),
	}

	for turn := 0; turn < r.config.MaxTurns; turn++ {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		r.emit(Event{
			Type:      EventIterationStart,
			Iteration: turn + 1,
		})

		req := agent.Request{
			Model:    r.config.Model,
			Messages: messages,
			Tools:    r.config.Tools,
			MaxTokens: 16384,
		}

		resp, err := r.config.Provider.Complete(ctx, req)
		if err != nil {
			r.emit(Event{
				Type:    EventError,
				ErrText: fmt.Sprintf("API error: %v", err),
			})
			return fmt.Errorf("API provider error: %w", err)
		}

		// Emit text content
		text := resp.GetText()
		if text != "" {
			r.emit(Event{
				Type: EventAssistantText,
				Text: text,
			})
		}

		// Check for tool calls
		toolCalls := resp.GetToolCalls()
		if len(toolCalls) == 0 {
			// No tool calls: model is done
			r.emit(Event{
				Type: EventComplete,
				Text: text,
				Result: &StepResult{
					Status:     "complete",
					Reason:     "end_turn",
					Confidence: 1.0,
				},
			})
			return nil
		}

		// Build assistant message with tool calls for conversation history
		assistantMsg := agent.Message{
			Role:    agent.RoleAssistant,
			Content: resp.Content,
		}
		messages = append(messages, assistantMsg)

		// Execute each tool call
		for _, tc := range toolCalls {
			r.emit(Event{
				Type: EventToolStart,
				Tool: tc.ToolName,
			})

			output, toolErr := r.tools.ExecuteTool(ctx, tc.ToolName, tc.ToolInput)

			if toolErr != nil {
				r.emit(Event{
					Type:    EventToolResult,
					Tool:    tc.ToolName,
					Text:    fmt.Sprintf("Error: %v", toolErr),
					ErrText: toolErr.Error(),
				})
			} else {
				r.emit(Event{
					Type: EventToolResult,
					Tool: tc.ToolName,
					Text: truncateOutput(output, 2000),
				})
			}

			// Add tool result to conversation
			messages = append(messages, agent.NewToolResultMessage(tc.ToolUseID, output, toolErr))
		}
	}

	// Max turns reached
	r.emit(Event{
		Type:    EventMaxIterationsReached,
		Text:    fmt.Sprintf("Reached maximum of %d turns", r.config.MaxTurns),
	})
	r.emit(Event{
		Type: EventComplete,
		Result: &StepResult{
			Status: "complete",
			Reason: "max_turns",
		},
	})
	return nil
}

func (r *APIRunner) emit(e Event) {
	r.events <- e
}

// MCPToolHandler executes tools locally, reusing the same tool semantics
// as the MCP server (read_file, write_file, run_shell_command, git_apply_patch).
type MCPToolHandler struct {
	workDir string
}

// ExecuteTool dispatches a tool call to the appropriate local handler.
func (h *MCPToolHandler) ExecuteTool(ctx context.Context, name string, input json.RawMessage) (string, error) {
	switch name {
	case "read_file":
		return h.readFile(input)
	case "write_file":
		return h.writeFile(input)
	case "run_shell_command":
		return h.runShellCommand(ctx, input)
	case "git_apply_patch":
		return h.gitApplyPatch(ctx, input)
	default:
		return "", fmt.Errorf("unknown tool: %s", name)
	}
}

func (h *MCPToolHandler) readFile(input json.RawMessage) (string, error) {
	var req struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal(input, &req); err != nil {
		return "", fmt.Errorf("invalid input: %w", err)
	}
	if req.Path == "" {
		return "", fmt.Errorf("path is required")
	}

	path := h.resolvePath(req.Path)
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read file: %w", err)
	}
	return string(data), nil
}

func (h *MCPToolHandler) writeFile(input json.RawMessage) (string, error) {
	var req struct {
		Path              string `json:"path"`
		Content           string `json:"content"`
		CreateDirectories bool   `json:"create_directories"`
	}
	if err := json.Unmarshal(input, &req); err != nil {
		return "", fmt.Errorf("invalid input: %w", err)
	}
	if req.Path == "" {
		return "", fmt.Errorf("path is required")
	}

	path := h.resolvePath(req.Path)

	if req.CreateDirectories {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return "", fmt.Errorf("create directories: %w", err)
		}
	}

	if err := os.WriteFile(path, []byte(req.Content), 0o644); err != nil {
		return "", fmt.Errorf("write file: %w", err)
	}
	return fmt.Sprintf("Wrote %d bytes to %s", len(req.Content), req.Path), nil
}

func (h *MCPToolHandler) runShellCommand(ctx context.Context, input json.RawMessage) (string, error) {
	var req struct {
		Command          string `json:"command"`
		WorkingDirectory string `json:"working_directory"`
		TimeoutMs        int64  `json:"timeout_ms"`
	}
	if err := json.Unmarshal(input, &req); err != nil {
		return "", fmt.Errorf("invalid input: %w", err)
	}
	if req.Command == "" {
		return "", fmt.Errorf("command is required")
	}

	timeout := 30 * time.Second
	if req.TimeoutMs > 0 {
		timeout = time.Duration(req.TimeoutMs) * time.Millisecond
	}

	cmdCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	workDir := h.workDir
	if req.WorkingDirectory != "" {
		workDir = h.resolvePath(req.WorkingDirectory)
	}

	cmd := exec.CommandContext(cmdCtx, "sh", "-c", req.Command)
	cmd.Dir = workDir
	out, err := cmd.CombinedOutput()

	result := string(out)
	if err != nil {
		result += fmt.Sprintf("\nExit code: %v", err)
		return result, nil // Return output even on non-zero exit
	}
	return result, nil
}

func (h *MCPToolHandler) gitApplyPatch(ctx context.Context, input json.RawMessage) (string, error) {
	var req struct {
		Patch            string `json:"patch"`
		WorkingDirectory string `json:"working_directory"`
	}
	if err := json.Unmarshal(input, &req); err != nil {
		return "", fmt.Errorf("invalid input: %w", err)
	}
	if req.Patch == "" {
		return "", fmt.Errorf("patch is required")
	}

	workDir := h.workDir
	if req.WorkingDirectory != "" {
		workDir = h.resolvePath(req.WorkingDirectory)
	}

	cmd := exec.CommandContext(ctx, "git", "apply", "--verbose", "-")
	cmd.Dir = workDir
	cmd.Stdin = strings.NewReader(req.Patch)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git apply failed: %s\n%v", string(out), err)
	}
	return fmt.Sprintf("Patch applied successfully.\n%s", string(out)), nil
}

func (h *MCPToolHandler) resolvePath(path string) string {
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(h.workDir, path)
}

// truncateOutput limits output to maxLen characters.
func truncateOutput(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "\n... (truncated)"
}

// BuildAPIToolDefinitions returns the standard tool definitions for API-based execution.
func BuildAPIToolDefinitions() []agent.ToolDefinition {
	return []agent.ToolDefinition{
		{
			Name:        "read_file",
			Description: "Read the contents of a file at the specified path. Returns the file content as text.",
			InputSchema: json.RawMessage(`{
				"type": "object",
				"properties": {
					"path": {
						"type": "string",
						"description": "The absolute or relative path to the file to read."
					}
				},
				"required": ["path"]
			}`),
		},
		{
			Name:        "write_file",
			Description: "Write content to a file at the specified path. Creates the file if it doesn't exist, or overwrites it if it does.",
			InputSchema: json.RawMessage(`{
				"type": "object",
				"properties": {
					"path": {
						"type": "string",
						"description": "The absolute or relative path to the file to write."
					},
					"content": {
						"type": "string",
						"description": "The content to write to the file."
					},
					"create_directories": {
						"type": "boolean",
						"description": "If true, create parent directories if they don't exist. Defaults to false.",
						"default": false
					}
				},
				"required": ["path", "content"]
			}`),
		},
		{
			Name:        "run_shell_command",
			Description: "Execute a shell command and return its output. Returns stdout, stderr, and exit code.",
			InputSchema: json.RawMessage(`{
				"type": "object",
				"properties": {
					"command": {
						"type": "string",
						"description": "The shell command to execute."
					},
					"working_directory": {
						"type": "string",
						"description": "The working directory for command execution."
					},
					"timeout_ms": {
						"type": "integer",
						"description": "Maximum execution time in milliseconds. Defaults to 30000.",
						"default": 30000
					}
				},
				"required": ["command"]
			}`),
		},
		{
			Name:        "git_apply_patch",
			Description: "Apply a unified diff/patch to files in a git repository.",
			InputSchema: json.RawMessage(`{
				"type": "object",
				"properties": {
					"patch": {
						"type": "string",
						"description": "The patch content in unified diff format."
					},
					"working_directory": {
						"type": "string",
						"description": "The working directory (git repository root) where the patch should be applied."
					}
				},
				"required": ["patch"]
			}`),
		},
	}
}
