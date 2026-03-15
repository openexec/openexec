package mcp

import (
    "bufio"
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
    "syscall"
    "time"
    "crypto/sha256"
    "encoding/hex"

    "github.com/openexec/openexec/internal/mode"
    "github.com/openexec/openexec/internal/toolset"
    "github.com/openexec/openexec/pkg/telemetry"
)

const protocolVersion = "2024-11-05"

// Request is a JSON-RPC 2.0 request or notification.
type Request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// Response is a JSON-RPC 2.0 response.
type Response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  interface{}     `json:"result,omitempty"`
	Error   *RPCError       `json:"error,omitempty"`
}

// RPCError is a JSON-RPC 2.0 error object.
type RPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// ServerConfig holds configuration for the MCP server.
type ServerConfig struct {
	// WorkDir is the workspace directory for path scoping.
	// Falls back to WORKSPACE_ROOT env var, then current working directory.
	WorkDir string

	// Mode overrides OPENEXEC_MODE env var for permission tier.
	Mode string
}

// Server is a minimal MCP server that exposes the openexec_signal tool.
type Server struct {
	in                  io.Reader
	out                 io.Writer
	pythonValidator     *PythonValidator
	forkManager         *SessionForkManager
	broker              *ToolBroker
	workspaceRoots      []string
	ctx                 context.Context     // Tracing context
	runID               string              // Current run ID for tracing
	idempotencyChecker  IdempotencyChecker  // Resume support
	resumeMode          bool                // True when resuming a prior run
	toolsetRegistry     *toolset.Registry   // Toolset-based tool filtering
	currentMode         mode.Mode           // Current operational mode (chat/task/run)
	activeToolset       string              // Currently active toolset name
}

// SetContext sets the tracing context and run ID for the server.
func (s *Server) SetContext(ctx context.Context, runID string) {
	s.ctx = ctx
	s.runID = runID
	s.broker.SetRunID(runID)
}

// SetIdempotencyChecker sets the checker for resume support.
func (s *Server) SetIdempotencyChecker(checker IdempotencyChecker) {
	s.idempotencyChecker = checker
}

// SetResumeMode enables or disables resume mode.
// When enabled, tool calls are checked against prior applications.
func (s *Server) SetResumeMode(enabled bool) {
	s.resumeMode = enabled
}

// SetMode sets the current operational mode (chat, task, run).
// This affects which tools are available and whether writes are allowed.
func (s *Server) SetMode(m mode.Mode) {
	s.currentMode = m
	// Auto-select toolset based on mode
	switch m {
	case mode.ModeChat:
		s.activeToolset = "repo_readonly"
	case mode.ModeTask, mode.ModeRun:
		s.activeToolset = "coding_backend"
	}
}

// GetMode returns the current operational mode.
func (s *Server) GetMode() mode.Mode {
	return s.currentMode
}

// SetToolset sets the active toolset for filtering available tools.
func (s *Server) SetToolset(name string) error {
	if _, ok := s.toolsetRegistry.Get(name); !ok {
		return fmt.Errorf("toolset %q not found", name)
	}
	s.activeToolset = name
	return nil
}

// GetToolset returns the currently active toolset.
func (s *Server) GetToolset() string {
	return s.activeToolset
}

// GetToolsetRegistry returns the toolset registry for external configuration.
func (s *Server) GetToolsetRegistry() *toolset.Registry {
	return s.toolsetRegistry
}

// ErrNoWorkspaceRoot is returned when no valid workspace root can be determined.
var ErrNoWorkspaceRoot = fmt.Errorf("no valid workspace root: set WORKSPACE_ROOT or provide WorkDir in config")

// NewServer creates a new MCP server reading from in and writing to out.
// Returns error if no workspace root can be determined (fail-fast).
func NewServer(in io.Reader, out io.Writer) (*Server, error) {
	return NewServerWithConfig(in, out, ServerConfig{})
}

// NewServerWithConfig creates a new MCP server with explicit configuration.
// Returns error if no workspace root can be determined (fail-fast).
// CRITICAL: Workspace root is mandatory for path scoping security.
func NewServerWithConfig(in io.Reader, out io.Writer, cfg ServerConfig) (*Server, error) {
	// Determine workspace roots with fallback chain
	var roots []string
	if envRoot := os.Getenv("WORKSPACE_ROOT"); envRoot != "" {
		roots = []string{envRoot}
	} else if cfg.WorkDir != "" {
		roots = []string{cfg.WorkDir}
	} else if cwd, err := os.Getwd(); err == nil && cwd != "" {
		roots = []string{cwd}
	}

	// FAIL FAST: No workspace root means no path scoping security
	if len(roots) == 0 || roots[0] == "" {
		return nil, ErrNoWorkspaceRoot
	}

	return &Server{
		in:              in,
		out:             out,
		broker:          NewToolBroker(cfg.Mode),
		workspaceRoots:  roots,
		toolsetRegistry: toolset.NewRegistry(),
		currentMode:     mode.ModeChat, // Start in chat mode
		activeToolset:   "repo_readonly", // Default to read-only toolset
		pythonValidator: NewPythonValidatorWithConfig(&PythonValidatorConfig{
			PythonPath:     "python3",
			Timeout:        5 * time.Second,
			SkipIfNoPython: true, // Don't fail writes if Python is unavailable
		}),
	}, nil
}

// ValidateConfig checks that the server has valid workspace configuration.
// DEPRECATED: NewServerWithConfig now fails fast if workspace root is missing.
// This method is kept for backward compatibility but always returns nil.
func (s *Server) ValidateConfig() error {
	// Validation now happens in constructor (fail-fast)
	return nil
}

// WorkspaceRoots returns the configured workspace roots for path scoping.
func (s *Server) WorkspaceRoots() []string {
	return s.workspaceRoots
}

// WithForkManager sets the session fork manager for the server.
// This enables fork_session, get_fork_info, and list_session_forks tools.
func (s *Server) WithForkManager(fm *SessionForkManager) *Server {
	s.forkManager = fm
	return s
}

// Serve reads JSON-RPC requests from in, dispatches them, and writes responses to out.
// Blocks until in is closed (EOF). Returns nil on clean EOF.
func (s *Server) Serve() error {
	scanner := bufio.NewScanner(s.in)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var req Request
		if err := json.Unmarshal(line, &req); err != nil {
			s.writeError(nil, -32700, "parse error")
			continue
		}

		s.dispatch(req)
	}

	return scanner.Err()
}

func (s *Server) dispatch(req Request) {
	switch req.Method {
	case "initialize":
		s.handleInitialize(req)
	case "notifications/initialized":
		// Notification — no response.
	case "tools/list":
		s.handleToolsList(req)
	case "tools/call":
		s.handleToolsCall(req)
	default:
		if req.ID != nil {
			s.writeError(req.ID, -32601, fmt.Sprintf("method not found: %s", req.Method))
		}
	}
}

func (s *Server) handleInitialize(req Request) {
	s.writeResult(req.ID, map[string]interface{}{
		"protocolVersion": protocolVersion,
		"capabilities": map[string]interface{}{
			"tools": map[string]interface{}{},
		},
		"serverInfo": map[string]interface{}{
			"name":    "axon-signal",
			"version": "1.0.0",
		},
	})
}

func (s *Server) handleToolsList(req Request) {
    // Core tools always available in all modes
    tools := []interface{}{
        axonSignalToolDef(),        // Always available (control plane)
        ReadFileToolDef(),          // Read is always allowed
        GitApplyPatchToolDef(),     // Patch tool available (with mode restrictions in Authorize)
        OpenExecResultToolDef(),    // Typed step results
        OpenExecActionToolDef(),    // Unified action envelope
    }

    // Dangerous tools only advertised in danger-full-access mode
    // This prevents clients from seeing tools they can't use
    if s.broker.Mode() == ModeFullAuto {
        tools = append(tools,
            WriteFileToolDef(),
            RunShellCommandToolDef(),
        )
    }

    // Add fork tools if fork manager is configured
    if s.forkManager != nil {
        tools = append(tools,
            ForkSessionToolDef(),
            GetForkInfoToolDef(),
            ListSessionForksToolDef(),
        )
    }

    // Include toolset info in response for clients that support it
    result := map[string]interface{}{
        "tools": tools,
    }

    // Add toolset metadata if configured
    if s.toolsetRegistry != nil && s.activeToolset != "" {
        result["active_toolset"] = s.activeToolset
        result["mode"] = string(s.currentMode)

        // Include list of tools in active toolset for client filtering
        if ts, ok := s.toolsetRegistry.Get(s.activeToolset); ok {
            result["toolset_tools"] = ts.Tools
            result["toolset_risk"] = string(ts.RiskLevel)
        }
    }

    s.writeResult(req.ID, result)
}

func axonSignalToolDef() map[string]interface{} {
	return map[string]interface{}{
		"name":        "openexec_signal",
		"description": "Send a structured signal to the OpenExec orchestrator. Use this to report progress, signal phase completion, request routing, or flag issues.",
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

// toolsCallParams is the shape of params for tools/call.
type toolsCallParams struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

func (s *Server) handleToolsCall(req Request) {
	var params toolsCallParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		s.writeError(req.ID, -32602, "invalid params")
		return
	}

	// Security: Check permissions via Tool Broker (V1 Alignment)
	if allowed, reason := s.broker.Authorize(params.Name, string(params.Arguments)); !allowed {
		s.writeToolError(req.ID, reason)
		return
	}

	// Parse arguments for tracing and idempotency
	var argsMap map[string]interface{}
	_ = json.Unmarshal(params.Arguments, &argsMap)

	// Resume support: Check if this tool call was already applied
    var idempotencyKey string
    // Determine idempotency eligibility (tool-specific)
    eligible := false
    switch params.Name {
    case "write_file", "git_apply_patch":
        eligible = true
    case "run_shell_command":
        // Best-effort: consider idempotent only for safe, read-only commands
        // Extract command/args
        var shellArgs struct{ Command string `json:"command"` }
        _ = json.Unmarshal(params.Arguments, &shellArgs)
        fields := strings.Fields(shellArgs.Command)
        if len(fields) > 0 {
            cmd := fields[0]
            args := []string{}
            if len(fields) > 1 { args = fields[1:] }
            if IsShellIdempotent(cmd, args) {
                eligible = true
            }
        }
    }

    if s.resumeMode && s.idempotencyChecker != nil && eligible {
        // Per-run scoped key to avoid global deduplication
        idempotencyKey = GenerateIdempotencyKey(s.runID, params.Name, argsMap)
        wasApplied, err := s.idempotencyChecker.WasApplied(idempotencyKey)
        if err == nil && wasApplied {
			// Skip execution and return cached result indicator
			s.writeResult(req.ID, map[string]interface{}{
				"content": []interface{}{
					map[string]interface{}{
						"type": "text",
						"text": fmt.Sprintf("[Resume] Tool call '%s' was already applied (key: %s...); skipping re-execution", params.Name, idempotencyKey[:12]),
					},
				},
				"idempotency": map[string]interface{}{
					"skipped": true,
					"key":     idempotencyKey,
					"run_id":  s.runID,
				},
			})
			return
		}
	}

	// Start OTel span for tool invocation
	ctx := s.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	_, span := telemetry.StartToolSpan(ctx, s.runID, params.Name, argsMap)
	defer span.End()

	// Execute the tool
	switch params.Name {
	case "openexec_signal":
		s.handleOpenExecSignal(req, params)
		telemetry.RecordToolSuccess(span, "")
	case "read_file":
		s.handleReadFile(req, params)
		telemetry.RecordToolSuccess(span, "")
	case "write_file":
		s.handleWriteFile(req, params)
		telemetry.RecordToolSuccess(span, "")
		s.markToolApplied(idempotencyKey, params.Name)
	case "run_shell_command":
		// Shell commands are NOT marked as applied - they are non-idempotent by design.
		// Re-running shell commands on resume is safer than skipping them, as partial
		// side effects from a previous run may have left state inconsistent.
		s.handleRunShellCommand(req, params)
		telemetry.RecordToolSuccess(span, "")
		// NOTE: Intentionally NOT calling s.markToolApplied() for shell commands
	case "git_apply_patch":
		s.handleGitApplyPatch(req, params)
		telemetry.RecordToolSuccess(span, "")
		s.markToolApplied(idempotencyKey, params.Name)
	case "fork_session":
		s.handleForkSession(req, params)
		telemetry.RecordToolSuccess(span, "")
	case "get_fork_info":
		s.handleGetForkInfo(req, params)
		telemetry.RecordToolSuccess(span, "")
	case "list_session_forks":
		s.handleListSessionForks(req, params)
		telemetry.RecordToolSuccess(span, "")
	case "openexec_result":
		s.handleOpenExecResult(req, params)
		telemetry.RecordToolSuccess(span, "")
	case "openexec_action":
		s.handleOpenExecAction(req, params)
		telemetry.RecordToolSuccess(span, "")
	default:
		s.writeError(req.ID, -32602, fmt.Sprintf("unknown tool: %s", params.Name))
		telemetry.RecordToolError(span, fmt.Errorf("unknown tool: %s", params.Name))
	}
}

// isIdempotentTool returns true if the tool has side effects that can be safely deduplicated.
// Shell commands are explicitly NOT idempotent - they should always re-run on resume
// to avoid issues with partial side effects from previous runs.
func isIdempotentTool(name string) bool {
    switch name {
    case "write_file", "git_apply_patch":
        return true
    default:
        return false
    }
}

// markToolApplied records that a tool was successfully applied.
func (s *Server) markToolApplied(key, toolName string) {
	if key == "" || s.idempotencyChecker == nil {
		return
	}
	_ = s.idempotencyChecker.MarkApplied(key, toolName, "")
}

func (s *Server) handleOpenExecSignal(req Request, params toolsCallParams) {
	var sig Signal
	if err := json.Unmarshal(params.Arguments, &sig); err != nil {
		s.writeError(req.ID, -32602, fmt.Sprintf("invalid signal arguments: %v", err))
		return
	}

	if err := sig.Validate(); err != nil {
		s.writeToolError(req.ID, fmt.Sprintf("invalid signal: %v", err))
		return
	}

	s.writeResult(req.ID, map[string]interface{}{
		"content": []interface{}{
			map[string]interface{}{
				"type": "text",
				"text": fmt.Sprintf("Signal received: %s", sig.Type),
			},
		},
	})
}

func (s *Server) handleReadFile(req Request, params toolsCallParams) {
	var rfReq ReadFileRequest
	if err := json.Unmarshal(params.Arguments, &rfReq); err != nil {
		s.writeError(req.ID, -32602, fmt.Sprintf("invalid read_file arguments: %v", err))
		return
	}

	// Validate the request
	if err := ValidateReadFileRequest(&rfReq); err != nil {
		s.writeToolError(req.ID, fmt.Sprintf("validation error: %v", err))
		return
	}

    // Validate path security against workspace root (centralized in server config)
    allowed := s.workspaceRoots
    if len(allowed) == 0 {
        s.writeToolError(req.ID, "no workspace root configured; cannot validate path")
        return
    }
    validPath, err := ValidatePathForRead(rfReq.Path, allowed)
	if err != nil {
		s.writeToolError(req.ID, fmt.Sprintf("path error: %v", err))
		return
	}

	// Open the file
	file, err := os.Open(validPath)
	if err != nil {
		s.writeToolError(req.ID, fmt.Sprintf("failed to open file: %v", err))
		return
	}
	defer file.Close()

	// Seek to offset if specified
	if rfReq.Offset > 0 {
		_, err = file.Seek(rfReq.Offset, io.SeekStart)
		if err != nil {
			s.writeToolError(req.ID, fmt.Sprintf("failed to seek: %v", err))
			return
		}
	}

	// Read the file content
	var content []byte
	if rfReq.Length > 0 {
		content = make([]byte, rfReq.Length)
		n, err := io.ReadFull(file, content)
		if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
			s.writeToolError(req.ID, fmt.Sprintf("failed to read file: %v", err))
			return
		}
		content = content[:n]
	} else {
		content, err = io.ReadAll(file)
		if err != nil {
			s.writeToolError(req.ID, fmt.Sprintf("failed to read file: %v", err))
			return
		}
	}

	// Encode content based on encoding type
	var textContent string
	if rfReq.Encoding == "binary" {
		textContent = base64.StdEncoding.EncodeToString(content)
	} else {
		textContent = string(content)
	}

	s.writeResult(req.ID, map[string]interface{}{
		"content": []interface{}{
			map[string]interface{}{
				"type": "text",
				"text": textContent,
			},
		},
	})
}

func (s *Server) handleWriteFile(req Request, params toolsCallParams) {
	var wfReq WriteFileRequest
	if err := json.Unmarshal(params.Arguments, &wfReq); err != nil {
		s.writeError(req.ID, -32602, fmt.Sprintf("invalid write_file arguments: %v", err))
		return
	}

	// Validate the request
	if err := ValidateWriteFileRequest(&wfReq); err != nil {
		s.writeToolError(req.ID, fmt.Sprintf("validation error: %v", err))
		return
	}

    // Enforce workspace-scoped writes (centralized in server config)
    if os.Getenv("OPENEXEC_MODE") == "read-only" {
        s.writeToolError(req.ID, "write is not allowed in read-only mode")
        return
    }
    allowed := s.workspaceRoots
    if len(allowed) == 0 {
        s.writeToolError(req.ID, "no workspace root configured; cannot validate path")
        return
    }
    validPath, err := ValidatePathForWrite(wfReq.Path, allowed)
	if err != nil {
		s.writeToolError(req.ID, fmt.Sprintf("path error: %v", err))
		return
	}

	// Determine content to write
	var contentBytes []byte
	if wfReq.Encoding == "binary" {
		// Decode base64 content for binary mode
		contentBytes, err = base64.StdEncoding.DecodeString(wfReq.Content)
		if err != nil {
			s.writeToolError(req.ID, fmt.Sprintf("failed to decode base64 content: %v", err))
			return
		}
	} else {
		contentBytes = []byte(wfReq.Content)
	}

	// Create parent directories if requested
	if wfReq.CreateDirectories {
		dir := filepath.Dir(validPath)
		if err := os.MkdirAll(dir, 0755); err != nil {
			s.writeToolError(req.ID, fmt.Sprintf("failed to create directories: %v", err))
			return
		}
	} else {
		// Validate parent directory exists
		if err := ValidateParentDirectoryForWrite(validPath); err != nil {
			s.writeToolError(req.ID, fmt.Sprintf("parent directory error: %v", err))
			return
		}
	}

	// Validate Python syntax before writing Python files
	if ShouldValidatePythonSyntax(validPath) && wfReq.Encoding != "binary" {
		syntaxResult := s.pythonValidator.ValidateCode(wfReq.Content, validPath)
		if !syntaxResult.Valid {
			var errMsgs []string
			for _, syntaxErr := range syntaxResult.Errors {
				errMsgs = append(errMsgs, syntaxErr.Error())
			}
			s.writeToolError(req.ID, fmt.Sprintf("Python syntax error: %s", strings.Join(errMsgs, "; ")))
			return
		}
	}

	// Determine flags based on mode
	var flags int
	if wfReq.Mode == WriteModeAppend {
		flags = os.O_WRONLY | os.O_CREATE | os.O_APPEND
	} else {
		flags = os.O_WRONLY | os.O_CREATE | os.O_TRUNC
	}

	// Open/create the file
	file, err := os.OpenFile(validPath, flags, 0644)
	if err != nil {
		s.writeToolError(req.ID, fmt.Sprintf("failed to open file for writing: %v", err))
		return
	}
	defer file.Close()

	// Write the content
	n, err := file.Write(contentBytes)
	if err != nil {
		s.writeToolError(req.ID, fmt.Sprintf("failed to write to file: %v", err))
		return
	}

	s.writeResult(req.ID, map[string]interface{}{
		"content": []interface{}{
			map[string]interface{}{
				"type": "text",
				"text": fmt.Sprintf("Successfully wrote %d bytes to %s", n, validPath),
			},
		},
	})
}

func (s *Server) writeResult(id json.RawMessage, result interface{}) {
	resp := Response{
		JSONRPC: "2.0",
		ID:      id,
		Result:  result,
	}
	s.writeJSON(resp)
}

func (s *Server) writeError(id json.RawMessage, code int, message string) {
	resp := Response{
		JSONRPC: "2.0",
		ID:      id,
		Error:   &RPCError{Code: code, Message: message},
	}
	s.writeJSON(resp)
}

func (s *Server) writeToolError(id json.RawMessage, message string) {
	resp := Response{
		JSONRPC: "2.0",
		ID:      id,
		Result: map[string]interface{}{
			"content": []interface{}{
				map[string]interface{}{
					"type": "text",
					"text": message,
				},
			},
			"isError": true,
		},
	}
	s.writeJSON(resp)
}

func (s *Server) writeJSON(v interface{}) {
	data, err := json.Marshal(v)
	if err != nil {
		return
	}
	data = append(data, '\n')
	_, _ = s.out.Write(data)
}

func (s *Server) handleRunShellCommand(req Request, params toolsCallParams) {
	var rscReq RunShellCommandRequest
	if err := json.Unmarshal(params.Arguments, &rscReq); err != nil {
		s.writeError(req.ID, -32602, fmt.Sprintf("invalid run_shell_command arguments: %v", err))
		return
	}

	// Validate the request (sets defaults like timeout)
	if err := ValidateRunShellCommandRequest(&rscReq); err != nil {
		s.writeToolError(req.ID, fmt.Sprintf("validation error: %v", err))
		return
	}

	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(rscReq.TimeoutMs)*time.Millisecond)
	defer cancel()

	// Prepare the command - use args mode or shell mode
	var cmd *exec.Cmd
	if len(rscReq.Args) > 0 {
		// Direct execution mode (safer for untrusted input)
		cmd = exec.CommandContext(ctx, rscReq.Command, rscReq.Args...)
	} else {
		// Shell interpretation mode
		cmd = exec.CommandContext(ctx, "bash", "-c", rscReq.Command)
	}

	// Set working directory if specified
	if rscReq.WorkingDirectory != "" {
		cmd.Dir = rscReq.WorkingDirectory
	}

	// Set environment variables
	if len(rscReq.Env) > 0 {
		// Start with current environment and add custom vars
		cmd.Env = os.Environ()
		for key, value := range rscReq.Env {
			cmd.Env = append(cmd.Env, key+"="+value)
		}
	}

	// Set up stdin if provided
	if rscReq.Stdin != "" {
		cmd.Stdin = strings.NewReader(rscReq.Stdin)
	}

	// Capture stdout and stderr separately
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	// Execute the command
	err := cmd.Run()

	// Determine exit code
	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			if status, ok := exitErr.Sys().(syscall.WaitStatus); ok {
				exitCode = status.ExitStatus()
			} else {
				exitCode = 1
			}
		} else if ctx.Err() == context.DeadlineExceeded {
			exitCode = -1 // Special code for timeout
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
		if ctx.Err() == context.DeadlineExceeded {
			outputText.WriteString(fmt.Sprintf("command timed out after %d ms", rscReq.TimeoutMs))
		} else {
			outputText.WriteString(err.Error())
		}
	}

	result := map[string]interface{}{
		"content": []interface{}{
			map[string]interface{}{
				"type": "text",
				"text": outputText.String(),
			},
		},
		"stdout":    stdoutStr,
		"stderr":    stderrStr,
		"exit_code": exitCode,
	}

	if err != nil {
		result["isError"] = true
	}

	s.writeResult(req.ID, result)
}

func (s *Server) handleGitApplyPatch(req Request, params toolsCallParams) {
	var gapReq GitApplyPatchRequest
	if err := json.Unmarshal(params.Arguments, &gapReq); err != nil {
		s.writeError(req.ID, -32602, fmt.Sprintf("invalid git_apply_patch arguments: %v", err))
		return
	}

	// Validate the request (this also parses and validates the patch format)
	if err := ValidateGitApplyPatchRequest(&gapReq); err != nil {
		s.writeToolError(req.ID, fmt.Sprintf("validation error: %v", err))
		return
	}

	// Parse the patch to get statistics for the response
	patch, _ := ParsePatch(gapReq.Patch)
	var patchStats PatchStats
	var affectedFiles []string
	if patch != nil {
		patchStats = patch.Stats()
		affectedFiles = patch.GetFilePaths()
	}

	// Determine working directory
	workDir := gapReq.WorkingDirectory
	if workDir == "" {
		var err error
		workDir, err = os.Getwd()
		if err != nil {
			s.writeToolError(req.ID, fmt.Sprintf("failed to get working directory: %v", err))
			return
		}
	}

	// Validate that working directory exists and is a git repository
	gitDir := filepath.Join(workDir, ".git")
	if _, err := os.Stat(gitDir); os.IsNotExist(err) {
		s.writeToolError(req.ID, fmt.Sprintf("not a git repository: %s", workDir))
		return
	}

	// SECURITY: Validate that all patch file paths are within workspace_root (Goal G-004)
	// This prevents patches from modifying files outside the workspace boundary.
	if patch != nil {
		if err := ValidatePatchPathsInWorkspace(patch, workDir); err != nil {
			s.writeToolError(req.ID, fmt.Sprintf("security error: %v", err))
			return
		}
	}

	// Build git apply command arguments
	args := []string{"apply"}

	if gapReq.CheckOnly {
		args = append(args, "--check")
	}
	if gapReq.Reverse {
		args = append(args, "--reverse")
	}
	if gapReq.ThreeWay {
		args = append(args, "--3way")
	}
	if gapReq.IgnoreWhitespace {
		args = append(args, "--ignore-whitespace")
	}
	if gapReq.ContextLines != DefaultContextLines {
		args = append(args, fmt.Sprintf("-C%d", gapReq.ContextLines))
	}

	// Apply patch from stdin
	args = append(args, "-")

	// Create context with timeout (30 seconds for patch application)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Execute git apply
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = workDir
	cmd.Stdin = strings.NewReader(gapReq.Patch)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()

    // Build result and persist patch artifact
    stdoutStr := stdout.String()
    stderrStr := stderr.String()

    // Persist patch artifact under .openexec/artifacts/patches
    workspace := workDir
    if len(s.workspaceRoots) > 0 && s.workspaceRoots[0] != "" {
        workspace = s.workspaceRoots[0]
    }
    artHash := ""
    artPath := ""
    if workspace != "" {
        sum := sha256.Sum256([]byte(gapReq.Patch))
        artHash = hex.EncodeToString(sum[:])
        dir := filepath.Join(workspace, ".openexec", "artifacts", "patches")
        _ = os.MkdirAll(dir, 0o755)
        p := filepath.Join(dir, artHash+".patch")
        _ = os.WriteFile(p, []byte(gapReq.Patch), 0o644)
        artPath = p
    }

    var resultText string
    if gapReq.CheckOnly {
        if err == nil {
            resultText = fmt.Sprintf("Patch can be applied cleanly (%d file(s), +%d/-%d lines)",
                patchStats.FilesChanged, patchStats.Additions, patchStats.Deletions)
        } else {
            resultText = fmt.Sprintf("Patch cannot be applied cleanly:\n%s", stderrStr)
        }
    } else {
        if err == nil {
            if gapReq.Reverse {
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
    if artHash != "" {
        resultText += "\nARTIFACT:patch " + artHash + " " + artPath
    }

    result := map[string]interface{}{
        "content": []interface{}{
            map[string]interface{}{
                "type": "text",
                "text": resultText,
            },
        },
        "stats": map[string]interface{}{
            "files_changed": patchStats.FilesChanged,
            "additions":     patchStats.Additions,
            "deletions":     patchStats.Deletions,
            "hunks":         patchStats.Hunks,
        },
        "affected_files": affectedFiles,
        "artifact": map[string]interface{}{
            "type": "patch",
            "hash": artHash,
            "path": artPath,
        },
    }

	if err != nil {
		result["isError"] = true
		result["stderr"] = stderrStr
	}

	s.writeResult(req.ID, result)
}

func (s *Server) handleForkSession(req Request, params toolsCallParams) {
	// Check if fork manager is available
	if s.forkManager == nil {
		s.writeToolError(req.ID, "fork_session tool is not available: session repository not configured")
		return
	}

	var fsReq ForkSessionRequest
	if err := json.Unmarshal(params.Arguments, &fsReq); err != nil {
		s.writeError(req.ID, -32602, fmt.Sprintf("invalid fork_session arguments: %v", err))
		return
	}

	// Validate the request
	if err := ValidateForkSessionRequest(&fsReq); err != nil {
		s.writeToolError(req.ID, fmt.Sprintf("validation error: %v", err))
		return
	}

	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Perform the fork operation
	result, err := s.forkManager.ForkSession(ctx, &fsReq)
	if err != nil {
		s.writeToolError(req.ID, fmt.Sprintf("fork operation failed: %v", err))
		return
	}

	// Build success message
	var msgBuilder strings.Builder
	msgBuilder.WriteString(fmt.Sprintf("Successfully forked session.\n"))
	msgBuilder.WriteString(fmt.Sprintf("New session ID: %s\n", result.ForkedSessionID))
	msgBuilder.WriteString(fmt.Sprintf("Title: %s\n", result.Title))
	msgBuilder.WriteString(fmt.Sprintf("Provider: %s, Model: %s\n", result.Provider, result.Model))
	msgBuilder.WriteString(fmt.Sprintf("Fork depth: %d\n", result.ForkDepth))

	if result.MessagesCopied > 0 {
		msgBuilder.WriteString(fmt.Sprintf("Messages copied: %d\n", result.MessagesCopied))
	}
	if result.ToolCallsCopied > 0 {
		msgBuilder.WriteString(fmt.Sprintf("Tool calls copied: %d\n", result.ToolCallsCopied))
	}
	if result.SummariesCopied > 0 {
		msgBuilder.WriteString(fmt.Sprintf("Summaries copied: %d\n", result.SummariesCopied))
	}

	s.writeResult(req.ID, map[string]interface{}{
		"content": []interface{}{
			map[string]interface{}{
				"type": "text",
				"text": msgBuilder.String(),
			},
		},
		"forked_session_id":     result.ForkedSessionID,
		"parent_session_id":     result.ParentSessionID,
		"fork_point_message_id": result.ForkPointMessageID,
		"title":                 result.Title,
		"provider":              result.Provider,
		"model":                 result.Model,
		"messages_copied":       result.MessagesCopied,
		"tool_calls_copied":     result.ToolCallsCopied,
		"summaries_copied":      result.SummariesCopied,
		"fork_depth":            result.ForkDepth,
		"ancestor_chain":        result.AncestorChain,
	})
}

func (s *Server) handleGetForkInfo(req Request, params toolsCallParams) {
	// Check if fork manager is available
	if s.forkManager == nil {
		s.writeToolError(req.ID, "get_fork_info tool is not available: session repository not configured")
		return
	}

	var gfiReq GetForkInfoRequest
	if err := json.Unmarshal(params.Arguments, &gfiReq); err != nil {
		s.writeError(req.ID, -32602, fmt.Sprintf("invalid get_fork_info arguments: %v", err))
		return
	}

	// Validate the request
	if err := ValidateGetForkInfoRequest(&gfiReq); err != nil {
		s.writeToolError(req.ID, fmt.Sprintf("validation error: %v", err))
		return
	}

	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Get fork info
	forkInfo, err := s.forkManager.GetForkInfo(ctx, &gfiReq)
	if err != nil {
		s.writeToolError(req.ID, fmt.Sprintf("failed to get fork info: %v", err))
		return
	}

	// Build info message
	var msgBuilder strings.Builder
	msgBuilder.WriteString(fmt.Sprintf("Session: %s\n", forkInfo.SessionID))
	if forkInfo.ParentSessionID != "" {
		msgBuilder.WriteString(fmt.Sprintf("Parent session: %s\n", forkInfo.ParentSessionID))
	} else {
		msgBuilder.WriteString("This is a root session (not a fork)\n")
	}
	msgBuilder.WriteString(fmt.Sprintf("Root session: %s\n", forkInfo.RootSessionID))
	if forkInfo.ForkPointMessageID != "" {
		msgBuilder.WriteString(fmt.Sprintf("Fork point message: %s\n", forkInfo.ForkPointMessageID))
	}
	msgBuilder.WriteString(fmt.Sprintf("Fork depth: %d\n", forkInfo.ForkDepth))
	msgBuilder.WriteString(fmt.Sprintf("Direct children: %d\n", forkInfo.ChildCount))
	msgBuilder.WriteString(fmt.Sprintf("Total descendants: %d\n", forkInfo.TotalDescendants))
	if len(forkInfo.AncestorChain) > 1 {
		msgBuilder.WriteString(fmt.Sprintf("Ancestor chain: %v\n", forkInfo.AncestorChain))
	}

	s.writeResult(req.ID, map[string]interface{}{
		"content": []interface{}{
			map[string]interface{}{
				"type": "text",
				"text": msgBuilder.String(),
			},
		},
		"session_id":            forkInfo.SessionID,
		"parent_session_id":     forkInfo.ParentSessionID,
		"root_session_id":       forkInfo.RootSessionID,
		"fork_point_message_id": forkInfo.ForkPointMessageID,
		"fork_depth":            forkInfo.ForkDepth,
		"child_count":           forkInfo.ChildCount,
		"total_descendants":     forkInfo.TotalDescendants,
		"ancestor_chain":        forkInfo.AncestorChain,
	})
}

func (s *Server) handleListSessionForks(req Request, params toolsCallParams) {
	// Check if fork manager is available
	if s.forkManager == nil {
		s.writeToolError(req.ID, "list_session_forks tool is not available: session repository not configured")
		return
	}

	var lsfReq ListSessionForksRequest
	if err := json.Unmarshal(params.Arguments, &lsfReq); err != nil {
		s.writeError(req.ID, -32602, fmt.Sprintf("invalid list_session_forks arguments: %v", err))
		return
	}

	// Validate the request
	if err := ValidateListSessionForksRequest(&lsfReq); err != nil {
		s.writeToolError(req.ID, fmt.Sprintf("validation error: %v", err))
		return
	}

	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// List session forks
	forks, err := s.forkManager.ListSessionForks(ctx, &lsfReq)
	if err != nil {
		s.writeToolError(req.ID, fmt.Sprintf("failed to list session forks: %v", err))
		return
	}

	// Build list of forks for response
	var forksList []map[string]interface{}
	var msgBuilder strings.Builder

	if len(forks) == 0 {
		msgBuilder.WriteString(fmt.Sprintf("No forks found for session %s\n", lsfReq.SessionID))
	} else {
		msgBuilder.WriteString(fmt.Sprintf("Found %d fork(s) of session %s:\n\n", len(forks), lsfReq.SessionID))
		for _, fork := range forks {
			forkData := map[string]interface{}{
				"id":         fork.ID,
				"title":      fork.Title,
				"provider":   fork.Provider,
				"model":      fork.Model,
				"status":     string(fork.Status),
				"created_at": fork.CreatedAt.Format(time.RFC3339),
				"fork_point": fork.GetForkPointMessageID(),
			}
			forksList = append(forksList, forkData)

			msgBuilder.WriteString(fmt.Sprintf("- %s\n", fork.ID))
			if fork.Title != "" {
				msgBuilder.WriteString(fmt.Sprintf("  Title: %s\n", fork.Title))
			}
			msgBuilder.WriteString(fmt.Sprintf("  Provider/Model: %s/%s\n", fork.Provider, fork.Model))
			msgBuilder.WriteString(fmt.Sprintf("  Status: %s\n", fork.Status))
			msgBuilder.WriteString(fmt.Sprintf("  Created: %s\n", fork.CreatedAt.Format(time.RFC3339)))
			msgBuilder.WriteString("\n")
		}
	}

	s.writeResult(req.ID, map[string]interface{}{
		"content": []interface{}{
			map[string]interface{}{
				"type": "text",
				"text": msgBuilder.String(),
			},
		},
		"parent_session_id": lsfReq.SessionID,
		"fork_count":        len(forks),
		"forks":             forksList,
	})
}

// handleOpenExecResult processes typed step result submissions from agents.
// This replaces the legacy text-based "STEP_RESULT: {JSON}" pattern with
// schema-validated structured output.
func (s *Server) handleOpenExecResult(req Request, params toolsCallParams) {
	var resultReq OpenExecResultRequest
	if err := json.Unmarshal(params.Arguments, &resultReq); err != nil {
		s.writeError(req.ID, -32602, fmt.Sprintf("invalid openexec_result arguments: %v", err))
		return
	}

	// Validate the result against schema constraints
	if err := ValidateOpenExecResultRequest(&resultReq); err != nil {
		s.writeToolError(req.ID, fmt.Sprintf("validation error: %v", err))
		return
	}

	// The result is returned to the agent; the orchestrator's parser will
	// extract typed signals from tool_result messages.
	s.writeResult(req.ID, map[string]interface{}{
		"content": []interface{}{
			map[string]interface{}{
				"type": "text",
				"text": fmt.Sprintf("STEP_RESULT: %s", mustMarshalJSON(map[string]interface{}{
					"status":      resultReq.Status,
					"reason":      resultReq.Reason,
					"next_phase":  resultReq.NextPhase,
					"artifacts":   resultReq.Artifacts,
					"confidence":  resultReq.Confidence,
					"diagnostics": resultReq.Diagnostics,
				})),
			},
		},
		"result": map[string]interface{}{
			"status":      resultReq.Status,
			"reason":      resultReq.Reason,
			"next_phase":  resultReq.NextPhase,
			"artifacts":   resultReq.Artifacts,
			"confidence":  resultReq.Confidence,
			"diagnostics": resultReq.Diagnostics,
		},
		"typed": true,
	})
}

// handleOpenExecAction processes unified typed agent actions.
// Supports step results, tool calls, plan updates, and progress reports.
func (s *Server) handleOpenExecAction(req Request, params toolsCallParams) {
	var actionReq map[string]interface{}
	if err := json.Unmarshal(params.Arguments, &actionReq); err != nil {
		s.writeError(req.ID, -32602, fmt.Sprintf("invalid openexec_action arguments: %v", err))
		return
	}

	actionType, _ := actionReq["type"].(string)
	if actionType == "" {
		s.writeToolError(req.ID, "action type is required")
		return
	}

	// Process based on action type
	switch actionType {
	case "complete", "error", "pivot", "retry":
		// Extract step_result and validate
		stepResult, ok := actionReq["step_result"].(map[string]interface{})
		if !ok {
			s.writeToolError(req.ID, fmt.Sprintf("step_result is required for action type %q", actionType))
			return
		}

		// Convert to typed struct and validate
		resultReq := OpenExecResultRequest{
			Status:    getString(stepResult, "status"),
			Reason:    getString(stepResult, "reason"),
			NextPhase: getString(stepResult, "next_phase"),
		}
		if artifacts, ok := stepResult["artifacts"].(map[string]interface{}); ok {
			resultReq.Artifacts = make(map[string]string)
			for k, v := range artifacts {
				if str, ok := v.(string); ok {
					resultReq.Artifacts[k] = str
				}
			}
		}
		if conf, ok := stepResult["confidence"].(float64); ok {
			resultReq.Confidence = conf
		}
		resultReq.Diagnostics = getString(stepResult, "diagnostics")

		if err := ValidateOpenExecResultRequest(&resultReq); err != nil {
			s.writeToolError(req.ID, fmt.Sprintf("validation error: %v", err))
			return
		}

		// Return the result in a format the parser can extract
		s.writeResult(req.ID, map[string]interface{}{
			"content": []interface{}{
				map[string]interface{}{
					"type": "text",
					"text": fmt.Sprintf("STEP_RESULT: %s", mustMarshalJSON(stepResult)),
				},
			},
			"action_type": actionType,
			"result":      stepResult,
			"typed":       true,
		})

	case "progress":
		text, _ := actionReq["text"].(string)
		s.writeResult(req.ID, map[string]interface{}{
			"content": []interface{}{
				map[string]interface{}{
					"type": "text",
					"text": fmt.Sprintf("Progress: %s", text),
				},
			},
			"action_type": actionType,
			"typed":       true,
		})

	case "tool_call":
		// Tool calls through this action type are informational only.
		// Actual tool execution must go through tools/call.
		toolCall, _ := actionReq["tool_call"].(map[string]interface{})
		toolName := getString(toolCall, "tool")

		s.writeResult(req.ID, map[string]interface{}{
			"content": []interface{}{
				map[string]interface{}{
					"type": "text",
					"text": fmt.Sprintf("Tool call intent recorded: %s (use tools/call for execution)", toolName),
				},
			},
			"action_type": actionType,
			"tool_call":   toolCall,
			"typed":       true,
		})

	case "plan_update":
		planUpdate, _ := actionReq["plan_update"].(map[string]interface{})
		updateAction := getString(planUpdate, "action")
		reason := getString(planUpdate, "reason")

		s.writeResult(req.ID, map[string]interface{}{
			"content": []interface{}{
				map[string]interface{}{
					"type": "text",
					"text": fmt.Sprintf("Plan update: action=%s, reason=%s", updateAction, reason),
				},
			},
			"action_type": actionType,
			"plan_update": planUpdate,
			"typed":       true,
		})

	default:
		s.writeToolError(req.ID, fmt.Sprintf("unknown action type: %s", actionType))
	}
}

// getString safely extracts a string value from a map.
func getString(m map[string]interface{}, key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}

// mustMarshalJSON marshals a value to JSON, returning "{}" on error.
func mustMarshalJSON(v interface{}) string {
	b, err := json.Marshal(v)
	if err != nil {
		return "{}"
	}
	return string(b)
}
