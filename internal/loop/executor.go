// Package loop provides the agent loop execution engine and event types
// for orchestrating AI agent interactions with tool execution.
package loop

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/openexec/openexec/internal/agent"
	"github.com/openexec/openexec/internal/approval"
	"github.com/openexec/openexec/internal/mcp"
)

// ToolHandler is a function that executes a tool and returns the result.
type ToolHandler func(ctx context.Context, input json.RawMessage) (*ToolResult, error)

// ToolResult represents the result of a tool execution.
type ToolResult struct {
	// Output is the tool's output content.
	Output string `json:"output"`

	// IsError indicates if the tool execution resulted in an error.
	IsError bool `json:"is_error"`

	// Metadata contains additional result information.
	Metadata map[string]interface{} `json:"metadata,omitempty"`
}

// ExecutorConfig configures the tool executor.
type ExecutorConfig struct {
	// WorkDir is the working directory for tool execution.
	WorkDir string

	// SessionID is the session identifier for approval tracking.
	SessionID string

	// ProjectPath is the project path for approval policy lookup.
	ProjectPath string

	// ApprovalManager handles tool approval workflow.
	ApprovalManager *approval.Manager

	// ApprovalTimeout is how long to wait for approval decisions.
	// If zero, uses a default of 5 minutes.
	ApprovalTimeout time.Duration

	// ApprovalCheckInterval is how often to check for approval decisions.
	// If zero, uses a default of 500ms.
	ApprovalCheckInterval time.Duration

	// AllowedRoots restricts file operations to these directories.
	// If empty, no restrictions are applied.
	AllowedRoots []string

	// EventCallback is called for each tool execution event.
	// May be nil if no event tracking is needed.
	EventCallback func(*LoopEvent)

	// MaxShellTimeout is the maximum timeout for shell commands.
	// If zero, uses the MCP default of 10 minutes.
	MaxShellTimeout time.Duration
}

// Executor manages tool registration and execution with approval workflow.
type Executor struct {
	cfg      ExecutorConfig
	handlers map[string]ToolHandler
	mu       sync.RWMutex

	// builtinTools are the standard MCP tools available by default.
	builtinTools map[string]bool
}

// NewExecutor creates a new tool executor with the given configuration.
func NewExecutor(cfg ExecutorConfig) *Executor {
	if cfg.ApprovalTimeout == 0 {
		cfg.ApprovalTimeout = 5 * time.Minute
	}
	if cfg.ApprovalCheckInterval == 0 {
		cfg.ApprovalCheckInterval = 500 * time.Millisecond
	}
	if cfg.MaxShellTimeout == 0 {
		cfg.MaxShellTimeout = 10 * time.Minute
	}

	e := &Executor{
		cfg:      cfg,
		handlers: make(map[string]ToolHandler),
		builtinTools: map[string]bool{
			"read_file":         true,
			"write_file":        true,
			"run_shell_command": true,
			"git_apply_patch":   true,
			"axon_signal":       true,
		},
	}

	// Register built-in tool handlers
	e.registerBuiltinTools()

	return e
}

// RegisterTool registers a custom tool handler.
// Returns an error if a tool with the same name is already registered.
func (e *Executor) RegisterTool(name string, handler ToolHandler) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if _, exists := e.handlers[name]; exists {
		return fmt.Errorf("tool already registered: %s", name)
	}

	e.handlers[name] = handler
	return nil
}

// UnregisterTool removes a tool handler.
// Built-in tools cannot be unregistered.
func (e *Executor) UnregisterTool(name string) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.builtinTools[name] {
		return fmt.Errorf("cannot unregister built-in tool: %s", name)
	}

	delete(e.handlers, name)
	return nil
}

// ListTools returns the names of all registered tools.
func (e *Executor) ListTools() []string {
	e.mu.RLock()
	defer e.mu.RUnlock()

	names := make([]string, 0, len(e.handlers))
	for name := range e.handlers {
		names = append(names, name)
	}
	return names
}

// GetToolDefinitions returns MCP tool definitions for all registered tools.
func (e *Executor) GetToolDefinitions() []map[string]interface{} {
	e.mu.RLock()
	defer e.mu.RUnlock()

	defs := make([]map[string]interface{}, 0, len(e.handlers))

	// Add built-in tool definitions
	if e.handlers["read_file"] != nil {
		defs = append(defs, mcp.ReadFileToolDef())
	}
	if e.handlers["write_file"] != nil {
		defs = append(defs, mcp.WriteFileToolDef())
	}
	if e.handlers["run_shell_command"] != nil {
		defs = append(defs, mcp.RunShellCommandToolDef())
	}
	if e.handlers["git_apply_patch"] != nil {
		defs = append(defs, mcp.GitApplyPatchToolDef())
	}
	if e.handlers["axon_signal"] != nil {
		defs = append(defs, axonSignalToolDef())
	}

	return defs
}

// axonSignalToolDef returns the MCP tool definition for axon_signal.
func axonSignalToolDef() map[string]interface{} {
	return map[string]interface{}{
		"name":        "axon_signal",
		"description": "Send a structured signal to the Axon orchestrator. Use this to report progress, signal phase completion, request routing, or flag issues.",
		"inputSchema": map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"type": map[string]interface{}{
					"type": "string",
					"enum": []string{
						"phase-complete", "blocked", "decision-point", "progress",
						"planning-mismatch", "scope-discovery", "route",
					},
					"description": "The signal type.",
				},
				"reason": map[string]interface{}{
					"type":        "string",
					"description": "Human-readable reason for the signal.",
				},
				"target": map[string]interface{}{
					"type":        "string",
					"description": "Target agent for route signals (e.g. 'spark', 'hon').",
				},
				"metadata": map[string]interface{}{
					"type":        "object",
					"description": "Additional structured metadata.",
				},
			},
			"required": []string{"type"},
		},
	}
}

// Execute executes a tool call from an AI provider response.
// It handles approval workflow, execution, and event emission.
func (e *Executor) Execute(ctx context.Context, toolCall agent.ContentBlock) (*ToolResult, error) {
	if toolCall.Type != agent.ContentTypeToolUse {
		return nil, fmt.Errorf("expected tool_use content block, got %s", toolCall.Type)
	}

	toolName := toolCall.ToolName
	toolID := toolCall.ToolUseID
	if toolID == "" {
		toolID = uuid.New().String()
	}

	// Create tool call info for tracking
	toolInfo := NewToolCallInfo(toolID, toolName)
	if err := toolInfo.SetInput(toolCall.ToolInput); err != nil {
		return nil, fmt.Errorf("failed to set tool input: %w", err)
	}

	// Emit tool call requested event
	e.emitToolEvent(ToolCallRequested, toolInfo, "")

	// Get handler
	e.mu.RLock()
	handler, exists := e.handlers[toolName]
	e.mu.RUnlock()

	if !exists {
		toolInfo.Status = ToolCallStatusFailed
		toolInfo.Error = fmt.Sprintf("unknown tool: %s", toolName)
		e.emitToolEvent(ToolCallError, toolInfo, toolInfo.Error)
		return &ToolResult{
			Output:  fmt.Sprintf("Error: unknown tool '%s'", toolName),
			IsError: true,
		}, nil
	}

	// Handle approval workflow
	approved, err := e.handleApproval(ctx, toolInfo)
	if err != nil {
		toolInfo.Status = ToolCallStatusFailed
		toolInfo.Error = err.Error()
		e.emitToolEvent(ToolCallError, toolInfo, err.Error())
		return &ToolResult{
			Output:  fmt.Sprintf("Approval error: %v", err),
			IsError: true,
		}, nil
	}

	if !approved {
		toolInfo.Status = ToolCallStatusRejected
		e.emitToolEvent(ToolCallRejected, toolInfo, "Tool call was rejected")
		return &ToolResult{
			Output:  "Tool call was rejected by approval policy",
			IsError: true,
		}, nil
	}

	// Execute the tool
	toolInfo.Status = ToolCallStatusRunning
	now := time.Now()
	toolInfo.StartedAt = &now
	e.emitToolEvent(ToolCallStart, toolInfo, "")

	result, err := handler(ctx, toolCall.ToolInput)
	if err != nil {
		toolInfo.Status = ToolCallStatusFailed
		toolInfo.Error = err.Error()
		completed := time.Now()
		toolInfo.CompletedAt = &completed
		toolInfo.DurationMs = completed.Sub(now).Milliseconds()
		e.emitToolEvent(ToolCallError, toolInfo, err.Error())

		return &ToolResult{
			Output:  fmt.Sprintf("Tool execution error: %v", err),
			IsError: true,
		}, nil
	}

	// Success
	toolInfo.Status = ToolCallStatusCompleted
	toolInfo.Output = result.Output
	completed := time.Now()
	toolInfo.CompletedAt = &completed
	toolInfo.DurationMs = completed.Sub(now).Milliseconds()
	e.emitToolEvent(ToolCallComplete, toolInfo, "")

	return result, nil
}

// ExecuteBatch executes multiple tool calls in sequence.
// Returns results in the same order as the input.
func (e *Executor) ExecuteBatch(ctx context.Context, toolCalls []agent.ContentBlock) ([]*ToolResult, error) {
	results := make([]*ToolResult, len(toolCalls))
	for i, tc := range toolCalls {
		result, err := e.Execute(ctx, tc)
		if err != nil {
			return results, fmt.Errorf("tool %d (%s) failed: %w", i, tc.ToolName, err)
		}
		results[i] = result
	}
	return results, nil
}

// ToContentBlock converts a tool result to a tool_result content block.
func (e *Executor) ToContentBlock(toolID string, result *ToolResult) agent.ContentBlock {
	block := agent.ContentBlock{
		Type:         agent.ContentTypeToolResult,
		ToolResultID: toolID,
		ToolOutput:   result.Output,
	}
	if result.IsError {
		block.ToolError = result.Output
	}
	return block
}

// handleApproval checks if the tool call is approved and waits if necessary.
func (e *Executor) handleApproval(ctx context.Context, toolInfo *ToolCallInfo) (bool, error) {
	mgr := e.cfg.ApprovalManager
	if mgr == nil {
		// No approval manager configured - auto-approve everything
		toolInfo.Status = ToolCallStatusAutoApproved
		e.emitToolEvent(ToolAutoApproved, toolInfo, "No approval manager configured")
		return true, nil
	}

	// Create approval request
	toolInputStr := string(toolInfo.Input)
	request, canProceed, err := mgr.RequestApproval(
		ctx,
		e.cfg.SessionID,
		toolInfo.ID,
		toolInfo.Name,
		toolInputStr,
		"agent",
		e.cfg.ProjectPath,
	)
	if err != nil {
		return false, fmt.Errorf("failed to request approval: %w", err)
	}

	if canProceed {
		// Auto-approved by policy
		toolInfo.Status = ToolCallStatusAutoApproved
		toolInfo.ApprovedBy = "policy"
		e.emitToolEvent(ToolAutoApproved, toolInfo, "Auto-approved by policy")
		return true, nil
	}

	// Need to wait for approval
	toolInfo.Status = ToolCallStatusPending
	e.emitToolEvent(ToolCallQueued, toolInfo, "Waiting for approval")

	// Create timeout context
	approvalCtx, cancel := context.WithTimeout(ctx, e.cfg.ApprovalTimeout)
	defer cancel()

	// Wait for approval decision
	result, err := mgr.WaitForApproval(approvalCtx, request.ID, e.cfg.ApprovalCheckInterval)
	if err != nil {
		if ctx.Err() != nil {
			return false, ctx.Err()
		}
		return false, fmt.Errorf("approval wait failed: %w", err)
	}

	if result.Approved {
		toolInfo.Status = ToolCallStatusApproved
		if result.Decision != nil {
			toolInfo.ApprovedBy = result.Decision.DecidedBy
		}
		e.emitToolEvent(ToolCallApproved, toolInfo, result.Reason)
		return true, nil
	}

	toolInfo.Status = ToolCallStatusRejected
	if result.Decision != nil {
		toolInfo.RejectedBy = result.Decision.DecidedBy
		toolInfo.RejectionReason = result.Decision.Reason
	}
	return false, nil
}

// emitToolEvent emits a tool event if an event callback is configured.
func (e *Executor) emitToolEvent(eventType LoopEventType, toolInfo *ToolCallInfo, message string) {
	if e.cfg.EventCallback == nil {
		return
	}

	builder, err := NewLoopEvent(eventType)
	if err != nil {
		return
	}

	event, err := builder.
		WithSession(e.cfg.SessionID).
		WithToolCall(toolInfo).
		WithMessage(message).
		Build()
	if err != nil {
		return
	}

	e.cfg.EventCallback(event)
}

// registerBuiltinTools registers the standard MCP tool handlers.
func (e *Executor) registerBuiltinTools() {
	e.handlers["read_file"] = e.handleReadFile
	e.handlers["write_file"] = e.handleWriteFile
	e.handlers["run_shell_command"] = e.handleRunShellCommand
	e.handlers["git_apply_patch"] = e.handleGitApplyPatch
	e.handlers["axon_signal"] = e.handleAxonSignal
}

// handleReadFile implements the read_file tool.
func (e *Executor) handleReadFile(ctx context.Context, input json.RawMessage) (*ToolResult, error) {
	var req mcp.ReadFileRequest
	if err := json.Unmarshal(input, &req); err != nil {
		return &ToolResult{Output: fmt.Sprintf("Invalid input: %v", err), IsError: true}, nil
	}

	if err := mcp.ValidateReadFileRequest(&req); err != nil {
		return &ToolResult{Output: fmt.Sprintf("Validation error: %v", err), IsError: true}, nil
	}

	// Validate path security
	validPath, err := mcp.ValidatePathForRead(req.Path, e.cfg.AllowedRoots)
	if err != nil {
		return &ToolResult{Output: fmt.Sprintf("Path error: %v", err), IsError: true}, nil
	}

	// Make path absolute relative to workdir if needed
	if !filepath.IsAbs(validPath) && e.cfg.WorkDir != "" {
		validPath = filepath.Join(e.cfg.WorkDir, validPath)
	}

	// Open the file
	file, err := os.Open(validPath)
	if err != nil {
		return &ToolResult{Output: fmt.Sprintf("Failed to open file: %v", err), IsError: true}, nil
	}
	defer file.Close()

	// Seek to offset if specified
	if req.Offset > 0 {
		if _, err := file.Seek(req.Offset, io.SeekStart); err != nil {
			return &ToolResult{Output: fmt.Sprintf("Failed to seek: %v", err), IsError: true}, nil
		}
	}

	// Read the file content
	var content []byte
	if req.Length > 0 {
		content = make([]byte, req.Length)
		n, err := io.ReadFull(file, content)
		if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
			return &ToolResult{Output: fmt.Sprintf("Failed to read file: %v", err), IsError: true}, nil
		}
		content = content[:n]
	} else {
		content, err = io.ReadAll(file)
		if err != nil {
			return &ToolResult{Output: fmt.Sprintf("Failed to read file: %v", err), IsError: true}, nil
		}
	}

	// Encode content based on encoding type
	var textContent string
	if req.Encoding == "binary" {
		textContent = base64.StdEncoding.EncodeToString(content)
	} else {
		textContent = string(content)
	}

	return &ToolResult{Output: textContent}, nil
}

// handleWriteFile implements the write_file tool.
func (e *Executor) handleWriteFile(ctx context.Context, input json.RawMessage) (*ToolResult, error) {
	var req mcp.WriteFileRequest
	if err := json.Unmarshal(input, &req); err != nil {
		return &ToolResult{Output: fmt.Sprintf("Invalid input: %v", err), IsError: true}, nil
	}

	if err := mcp.ValidateWriteFileRequest(&req); err != nil {
		return &ToolResult{Output: fmt.Sprintf("Validation error: %v", err), IsError: true}, nil
	}

	// Validate path security
	validPath, err := mcp.ValidatePathForWrite(req.Path, e.cfg.AllowedRoots)
	if err != nil {
		return &ToolResult{Output: fmt.Sprintf("Path error: %v", err), IsError: true}, nil
	}

	// Make path absolute relative to workdir if needed
	if !filepath.IsAbs(validPath) && e.cfg.WorkDir != "" {
		validPath = filepath.Join(e.cfg.WorkDir, validPath)
	}

	// Determine content to write
	var contentBytes []byte
	if req.Encoding == "binary" {
		contentBytes, err = base64.StdEncoding.DecodeString(req.Content)
		if err != nil {
			return &ToolResult{Output: fmt.Sprintf("Failed to decode base64 content: %v", err), IsError: true}, nil
		}
	} else {
		contentBytes = []byte(req.Content)
	}

	// Create parent directories if requested
	if req.CreateDirectories {
		dir := filepath.Dir(validPath)
		if err := os.MkdirAll(dir, 0755); err != nil {
			return &ToolResult{Output: fmt.Sprintf("Failed to create directories: %v", err), IsError: true}, nil
		}
	} else {
		if err := mcp.ValidateParentDirectoryForWrite(validPath); err != nil {
			return &ToolResult{Output: fmt.Sprintf("Parent directory error: %v", err), IsError: true}, nil
		}
	}

	// Determine flags based on mode
	var flags int
	if req.Mode == mcp.WriteModeAppend {
		flags = os.O_WRONLY | os.O_CREATE | os.O_APPEND
	} else {
		flags = os.O_WRONLY | os.O_CREATE | os.O_TRUNC
	}

	// Open/create the file
	file, err := os.OpenFile(validPath, flags, 0644)
	if err != nil {
		return &ToolResult{Output: fmt.Sprintf("Failed to open file for writing: %v", err), IsError: true}, nil
	}
	defer file.Close()

	// Write the content
	n, err := file.Write(contentBytes)
	if err != nil {
		return &ToolResult{Output: fmt.Sprintf("Failed to write to file: %v", err), IsError: true}, nil
	}

	return &ToolResult{
		Output: fmt.Sprintf("Successfully wrote %d bytes to %s", n, validPath),
		Metadata: map[string]interface{}{
			"bytes_written": n,
			"path":          validPath,
		},
	}, nil
}

// handleRunShellCommand implements the run_shell_command tool.
func (e *Executor) handleRunShellCommand(ctx context.Context, input json.RawMessage) (*ToolResult, error) {
	var req mcp.RunShellCommandRequest
	if err := json.Unmarshal(input, &req); err != nil {
		return &ToolResult{Output: fmt.Sprintf("Invalid input: %v", err), IsError: true}, nil
	}

	if err := mcp.ValidateRunShellCommandRequest(&req); err != nil {
		return &ToolResult{Output: fmt.Sprintf("Validation error: %v", err), IsError: true}, nil
	}

	// Cap timeout at max
	maxTimeoutMs := int64(e.cfg.MaxShellTimeout.Milliseconds())
	if req.TimeoutMs > maxTimeoutMs {
		req.TimeoutMs = maxTimeoutMs
	}

	// Create context with timeout
	cmdCtx, cancel := context.WithTimeout(ctx, time.Duration(req.TimeoutMs)*time.Millisecond)
	defer cancel()

	// Prepare the command
	var cmd *exec.Cmd
	if len(req.Args) > 0 {
		cmd = exec.CommandContext(cmdCtx, req.Command, req.Args...)
	} else {
		cmd = exec.CommandContext(cmdCtx, "bash", "-c", req.Command)
	}

	// Set working directory
	workDir := req.WorkingDirectory
	if workDir == "" {
		workDir = e.cfg.WorkDir
	}
	if workDir != "" {
		cmd.Dir = workDir
	}

	// Set environment variables
	if len(req.Env) > 0 {
		cmd.Env = os.Environ()
		for key, value := range req.Env {
			cmd.Env = append(cmd.Env, key+"="+value)
		}
	}

	// Set up stdin if provided
	if req.Stdin != "" {
		cmd.Stdin = strings.NewReader(req.Stdin)
	}

	// Capture stdout and stderr
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	// Execute the command
	err := cmd.Run()

	// Determine exit code
	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else if cmdCtx.Err() == context.DeadlineExceeded {
			exitCode = -1
		} else {
			exitCode = 1
		}
	}

	// Build output text
	var outputText strings.Builder
	stdoutStr := stdout.String()
	stderrStr := stderr.String()

	if stdoutStr != "" {
		outputText.WriteString(stdoutStr)
	}
	if stderrStr != "" {
		if outputText.Len() > 0 && !strings.HasSuffix(stdoutStr, "\n") {
			outputText.WriteString("\n")
		}
		outputText.WriteString(stderrStr)
	}

	// If no output and there's an error, include the error message
	if outputText.Len() == 0 && err != nil {
		if cmdCtx.Err() == context.DeadlineExceeded {
			outputText.WriteString(fmt.Sprintf("command timed out after %d ms", req.TimeoutMs))
		} else {
			outputText.WriteString(err.Error())
		}
	}

	return &ToolResult{
		Output:  outputText.String(),
		IsError: err != nil,
		Metadata: map[string]interface{}{
			"exit_code": exitCode,
			"stdout":    stdoutStr,
			"stderr":    stderrStr,
		},
	}, nil
}

// handleGitApplyPatch implements the git_apply_patch tool.
func (e *Executor) handleGitApplyPatch(ctx context.Context, input json.RawMessage) (*ToolResult, error) {
	var req mcp.GitApplyPatchRequest
	if err := json.Unmarshal(input, &req); err != nil {
		return &ToolResult{Output: fmt.Sprintf("Invalid input: %v", err), IsError: true}, nil
	}

	if err := mcp.ValidateGitApplyPatchRequest(&req); err != nil {
		return &ToolResult{Output: fmt.Sprintf("Validation error: %v", err), IsError: true}, nil
	}

	// Parse the patch to get statistics
	patch, _ := mcp.ParsePatch(req.Patch)
	var patchStats mcp.PatchStats
	var affectedFiles []string
	if patch != nil {
		patchStats = patch.Stats()
		affectedFiles = patch.GetFilePaths()
	}

	// Determine working directory
	workDir := req.WorkingDirectory
	if workDir == "" {
		workDir = e.cfg.WorkDir
	}
	if workDir == "" {
		var err error
		workDir, err = os.Getwd()
		if err != nil {
			return &ToolResult{Output: fmt.Sprintf("Failed to get working directory: %v", err), IsError: true}, nil
		}
	}

	// Validate that working directory is a git repository
	gitDir := filepath.Join(workDir, ".git")
	if _, err := os.Stat(gitDir); os.IsNotExist(err) {
		return &ToolResult{Output: fmt.Sprintf("Not a git repository: %s", workDir), IsError: true}, nil
	}

	// Build git apply command arguments
	args := []string{"apply"}

	if req.CheckOnly {
		args = append(args, "--check")
	}
	if req.Reverse {
		args = append(args, "--reverse")
	}
	if req.ThreeWay {
		args = append(args, "--3way")
	}
	if req.IgnoreWhitespace {
		args = append(args, "--ignore-whitespace")
	}
	if req.ContextLines != mcp.DefaultContextLines {
		args = append(args, fmt.Sprintf("-C%d", req.ContextLines))
	}

	// Apply patch from stdin
	args = append(args, "-")

	// Create context with timeout (30 seconds for patch application)
	cmdCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	// Execute git apply
	cmd := exec.CommandContext(cmdCtx, "git", args...)
	cmd.Dir = workDir
	cmd.Stdin = strings.NewReader(req.Patch)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()

	// Build result
	stdoutStr := stdout.String()
	stderrStr := stderr.String()

	var resultText string
	if req.CheckOnly {
		if err == nil {
			resultText = fmt.Sprintf("Patch can be applied cleanly (%d file(s), +%d/-%d lines)",
				patchStats.FilesChanged, patchStats.Additions, patchStats.Deletions)
		} else {
			resultText = fmt.Sprintf("Patch cannot be applied cleanly:\n%s", stderrStr)
		}
	} else {
		if err == nil {
			if req.Reverse {
				resultText = fmt.Sprintf("Patch unapplied successfully (%d file(s), +%d/-%d lines)",
					patchStats.FilesChanged, patchStats.Deletions, patchStats.Additions)
			} else {
				resultText = fmt.Sprintf("Patch applied successfully (%d file(s), +%d/-%d lines)",
					patchStats.FilesChanged, patchStats.Additions, patchStats.Deletions)
			}
			if stdoutStr != "" {
				resultText += "\n" + stdoutStr
			}
		} else {
			resultText = fmt.Sprintf("Failed to apply patch:\n%s", stderrStr)
		}
	}

	return &ToolResult{
		Output:  resultText,
		IsError: err != nil,
		Metadata: map[string]interface{}{
			"stats": map[string]interface{}{
				"files_changed": patchStats.FilesChanged,
				"additions":     patchStats.Additions,
				"deletions":     patchStats.Deletions,
				"hunks":         patchStats.Hunks,
			},
			"affected_files": affectedFiles,
		},
	}, nil
}

// handleAxonSignal implements the axon_signal tool.
func (e *Executor) handleAxonSignal(ctx context.Context, input json.RawMessage) (*ToolResult, error) {
	var sig mcp.Signal
	if err := json.Unmarshal(input, &sig); err != nil {
		return &ToolResult{Output: fmt.Sprintf("Invalid signal arguments: %v", err), IsError: true}, nil
	}

	if err := sig.Validate(); err != nil {
		return &ToolResult{Output: fmt.Sprintf("Invalid signal: %v", err), IsError: true}, nil
	}

	return &ToolResult{
		Output: fmt.Sprintf("Signal received: %s", sig.Type),
		Metadata: map[string]interface{}{
			"signal_type": string(sig.Type),
			"reason":      sig.Reason,
			"target":      sig.Target,
			"metadata":    sig.Metadata,
		},
	}, nil
}

// IsAxonSignal checks if a tool call is an axon_signal.
func IsAxonSignal(toolCall agent.ContentBlock) bool {
	return toolCall.ToolName == "axon_signal"
}

// GetSignalFromToolCall extracts a Signal from an axon_signal tool call.
func GetSignalFromToolCall(toolCall agent.ContentBlock) (*mcp.Signal, error) {
	if !IsAxonSignal(toolCall) {
		return nil, fmt.Errorf("not an axon_signal tool call")
	}

	var sig mcp.Signal
	if err := json.Unmarshal(toolCall.ToolInput, &sig); err != nil {
		return nil, fmt.Errorf("failed to parse signal: %w", err)
	}

	return &sig, nil
}
