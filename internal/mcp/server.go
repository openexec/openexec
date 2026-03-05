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

// Server is a minimal MCP server that exposes the openexec_signal tool.
type Server struct {
	in              io.Reader
	out             io.Writer
	pythonValidator *PythonValidator
	forkManager     *SessionForkManager
}

// NewServer creates an MCP server reading from in, writing to out.
func NewServer(in io.Reader, out io.Writer) *Server {
	return &Server{
		in:              in,
		out:             out,
		pythonValidator: NewPythonValidatorWithConfig(&PythonValidatorConfig{
			PythonPath:     "python3",
			Timeout:        5 * time.Second,
			SkipIfNoPython: true, // Don't fail writes if Python is unavailable
		}),
	}
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
	tools := []interface{}{
		axonSignalToolDef(),
		ReadFileToolDef(),
		WriteFileToolDef(),
		RunShellCommandToolDef(),
		GitApplyPatchToolDef(),
	}

	// Add fork tools if fork manager is configured
	if s.forkManager != nil {
		tools = append(tools,
			ForkSessionToolDef(),
			GetForkInfoToolDef(),
			ListSessionForksToolDef(),
		)
	}

	s.writeResult(req.ID, map[string]interface{}{
		"tools": tools,
	})
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

	switch params.Name {
	case "openexec_signal":
		s.handleOpenExecSignal(req, params)
	case "read_file":
		s.handleReadFile(req, params)
	case "write_file":
		s.handleWriteFile(req, params)
	case "run_shell_command":
		s.handleRunShellCommand(req, params)
	case "git_apply_patch":
		s.handleGitApplyPatch(req, params)
	case "fork_session":
		s.handleForkSession(req, params)
	case "get_fork_info":
		s.handleGetForkInfo(req, params)
	case "list_session_forks":
		s.handleListSessionForks(req, params)
	default:
		s.writeError(req.ID, -32602, fmt.Sprintf("unknown tool: %s", params.Name))
	}
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

	// Validate path security (no root restrictions for now)
	validPath, err := ValidatePathForRead(rfReq.Path, nil)
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

	// Validate path security (no root restrictions for now)
	validPath, err := ValidatePathForWrite(wfReq.Path, nil)
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

	// Build result
	stdoutStr := stdout.String()
	stderrStr := stderr.String()

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
				"id":          fork.ID,
				"title":       fork.Title,
				"provider":    fork.Provider,
				"model":       fork.Model,
				"status":      string(fork.Status),
				"created_at":  fork.CreatedAt.Format(time.RFC3339),
				"fork_point":  fork.GetForkPointMessageID(),
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
