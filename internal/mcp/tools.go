// Package mcp provides MCP (Model Context Protocol) server functionality.
// This file defines MCP tool schemas for file operations.
package mcp

import (
	"fmt"
	"strings"
)

// ReadFileToolDef returns the MCP tool definition for the read_file tool.
// This tool allows reading the contents of a file at a specified path.
func ReadFileToolDef() map[string]interface{} {
	return map[string]interface{}{
		"name":        "read_file",
		"description": "Read the contents of a file at the specified path. Returns the file content as text. Supports reading text files of any size. For binary files, content will be returned as base64-encoded string.",
		"inputSchema": map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"path": map[string]interface{}{
					"type":        "string",
					"description": "The absolute or relative path to the file to read.",
				},
				"encoding": map[string]interface{}{
					"type":        "string",
					"description": "The encoding to use when reading the file. Defaults to 'utf-8'.",
					"default":     "utf-8",
					"enum":        []string{"utf-8", "utf-16", "ascii", "binary"},
				},
				"offset": map[string]interface{}{
					"type":        "integer",
					"description": "The byte offset to start reading from. Defaults to 0 (beginning of file).",
					"default":     0,
					"minimum":     0,
				},
				"length": map[string]interface{}{
					"type":        "integer",
					"description": "The maximum number of bytes to read. If not specified, reads the entire file.",
					"minimum":     1,
				},
			},
			"required": []string{"path"},
		},
	}
}

// ReadFileRequest represents the parsed arguments for a read_file tool call.
type ReadFileRequest struct {
	Path     string `json:"path"`
	Encoding string `json:"encoding,omitempty"`
	Offset   int64  `json:"offset,omitempty"`
	Length   int64  `json:"length,omitempty"`
}

// DefaultEncoding is the default file encoding for read_file.
const DefaultEncoding = "utf-8"

// ValidateReadFileRequest validates a ReadFileRequest and sets defaults.
func ValidateReadFileRequest(req *ReadFileRequest) error {
	if req.Path == "" {
		return &ValidationError{Field: "path", Message: "path is required"}
	}

	// Set default encoding if not provided
	if req.Encoding == "" {
		req.Encoding = DefaultEncoding
	}

	// Validate encoding
	validEncodings := map[string]bool{
		"utf-8":  true,
		"utf-16": true,
		"ascii":  true,
		"binary": true,
	}
	if !validEncodings[req.Encoding] {
		return &ValidationError{Field: "encoding", Message: "invalid encoding: must be one of utf-8, utf-16, ascii, binary"}
	}

	// Validate offset
	if req.Offset < 0 {
		return &ValidationError{Field: "offset", Message: "offset must be non-negative"}
	}

	// Validate length
	if req.Length < 0 {
		return &ValidationError{Field: "length", Message: "length must be positive"}
	}

	return nil
}

// ValidationError represents a validation error for tool inputs.
type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string {
	return e.Field + ": " + e.Message
}

// WriteFileToolDef returns the MCP tool definition for the write_file tool.
// This tool allows writing content to a file at a specified path.
func WriteFileToolDef() map[string]interface{} {
	return map[string]interface{}{
		"name":        "write_file",
		"description": "Write content to a file at the specified path. Creates the file if it doesn't exist, or overwrites it if it does. Parent directories must exist. For binary content, provide base64-encoded data with encoding set to 'binary'.",
		"inputSchema": map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"path": map[string]interface{}{
					"type":        "string",
					"description": "The absolute or relative path to the file to write.",
				},
				"content": map[string]interface{}{
					"type":        "string",
					"description": "The content to write to the file. For binary files, this should be base64-encoded.",
				},
				"encoding": map[string]interface{}{
					"type":        "string",
					"description": "The encoding to use when writing the file. Defaults to 'utf-8'. Use 'binary' for base64-encoded binary content.",
					"default":     "utf-8",
					"enum":        []string{"utf-8", "utf-16", "ascii", "binary"},
				},
				"mode": map[string]interface{}{
					"type":        "string",
					"description": "Write mode: 'overwrite' replaces existing content, 'append' adds to end of file. Defaults to 'overwrite'.",
					"default":     "overwrite",
					"enum":        []string{"overwrite", "append"},
				},
				"create_directories": map[string]interface{}{
					"type":        "boolean",
					"description": "If true, create parent directories if they don't exist. Defaults to false.",
					"default":     false,
				},
			},
			"required": []string{"path", "content"},
		},
	}
}

// WriteFileRequest represents the parsed arguments for a write_file tool call.
type WriteFileRequest struct {
	Path              string `json:"path"`
	Content           string `json:"content"`
	Encoding          string `json:"encoding,omitempty"`
	Mode              string `json:"mode,omitempty"`
	CreateDirectories bool   `json:"create_directories,omitempty"`
}

// WriteMode constants
const (
	WriteModeOverwrite = "overwrite"
	WriteModeAppend    = "append"
)

// ValidateWriteFileRequest validates a WriteFileRequest and sets defaults.
func ValidateWriteFileRequest(req *WriteFileRequest) error {
	if req.Path == "" {
		return &ValidationError{Field: "path", Message: "path is required"}
	}

	// Content can be empty (writing empty file is valid)

	// Set default encoding if not provided
	if req.Encoding == "" {
		req.Encoding = DefaultEncoding
	}

	// Validate encoding
	validEncodings := map[string]bool{
		"utf-8":  true,
		"utf-16": true,
		"ascii":  true,
		"binary": true,
	}
	if !validEncodings[req.Encoding] {
		return &ValidationError{Field: "encoding", Message: "invalid encoding: must be one of utf-8, utf-16, ascii, binary"}
	}

	// Set default mode if not provided
	if req.Mode == "" {
		req.Mode = WriteModeOverwrite
	}

	// Validate mode
	validModes := map[string]bool{
		WriteModeOverwrite: true,
		WriteModeAppend:    true,
	}
	if !validModes[req.Mode] {
		return &ValidationError{Field: "mode", Message: "invalid mode: must be one of overwrite, append"}
	}

	return nil
}

// RunShellCommandToolDef returns the MCP tool definition for the run_shell_command tool.
// This tool allows executing shell commands in a controlled environment.
func RunShellCommandToolDef() map[string]interface{} {
	return map[string]interface{}{
		"name":        "run_shell_command",
		"description": "Execute a shell command and return its output. Runs the command in a subprocess with configurable timeout and working directory. Returns stdout, stderr, and exit code. Use with caution as commands run with the same permissions as the server process.",
		"inputSchema": map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"command": map[string]interface{}{
					"type":        "string",
					"description": "The shell command to execute. Supports standard shell syntax including pipes, redirects, and command chaining.",
				},
				"args": map[string]interface{}{
					"type":        "array",
					"description": "Optional array of arguments to pass to the command. If provided, command is executed directly without shell interpretation (safer for untrusted input).",
					"items": map[string]interface{}{
						"type": "string",
					},
				},
				"working_directory": map[string]interface{}{
					"type":        "string",
					"description": "The working directory for command execution. Defaults to the current working directory of the server process.",
				},
				"timeout_ms": map[string]interface{}{
					"type":        "integer",
					"description": "Maximum execution time in milliseconds. Command is terminated if it exceeds this limit. Defaults to 30000 (30 seconds). Maximum is 600000 (10 minutes).",
					"default":     30000,
					"minimum":     100,
					"maximum":     600000,
				},
				"env": map[string]interface{}{
					"type":        "object",
					"description": "Additional environment variables to set for the command. Merged with the server process environment.",
					"additionalProperties": map[string]interface{}{
						"type": "string",
					},
				},
				"stdin": map[string]interface{}{
					"type":        "string",
					"description": "Optional input to pass to the command's standard input.",
				},
			},
			"required": []string{"command"},
		},
	}
}

// RunShellCommandRequest represents the parsed arguments for a run_shell_command tool call.
type RunShellCommandRequest struct {
	Command          string            `json:"command"`
	Args             []string          `json:"args,omitempty"`
	WorkingDirectory string            `json:"working_directory,omitempty"`
	TimeoutMs        int64             `json:"timeout_ms,omitempty"`
	Env              map[string]string `json:"env,omitempty"`
	Stdin            string            `json:"stdin,omitempty"`
}

// Default timeout constants for run_shell_command
const (
	DefaultTimeoutMs = 30000  // 30 seconds
	MinTimeoutMs     = 100    // 100 milliseconds
	MaxTimeoutMs     = 600000 // 10 minutes
)

// ValidateRunShellCommandRequest validates a RunShellCommandRequest and sets defaults.
func ValidateRunShellCommandRequest(req *RunShellCommandRequest) error {
	if req.Command == "" {
		return &ValidationError{Field: "command", Message: "command is required"}
	}

	// Set default timeout if not provided
	if req.TimeoutMs == 0 {
		req.TimeoutMs = DefaultTimeoutMs
	}

	// Validate timeout range
	if req.TimeoutMs < MinTimeoutMs {
		return &ValidationError{Field: "timeout_ms", Message: "timeout_ms must be at least 100 milliseconds"}
	}
	if req.TimeoutMs > MaxTimeoutMs {
		return &ValidationError{Field: "timeout_ms", Message: "timeout_ms must not exceed 600000 milliseconds (10 minutes)"}
	}

	return nil
}

// GitApplyPatchToolDef returns the MCP tool definition for the git_apply_patch tool.
// This tool applies a unified diff/patch to files in a git repository.
func GitApplyPatchToolDef() map[string]interface{} {
	return map[string]interface{}{
		"name":        "git_apply_patch",
		"description": "Apply a unified diff/patch to files in a git repository. The patch should be in unified diff format (as produced by 'git diff' or 'diff -u'). This tool validates the patch format before applying and can optionally check the patch without applying it.",
		"inputSchema": map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"patch": map[string]interface{}{
					"type":        "string",
					"description": "The patch content in unified diff format. Must include proper diff headers (--- a/file and +++ b/file) and hunk headers (@@ -start,count +start,count @@).",
				},
				"working_directory": map[string]interface{}{
					"type":        "string",
					"description": "The working directory (git repository root) where the patch should be applied. Defaults to the current working directory.",
				},
				"check_only": map[string]interface{}{
					"type":        "boolean",
					"description": "If true, only check if the patch can be applied cleanly without actually applying it. Useful for validation before making changes. Defaults to false.",
					"default":     false,
				},
				"reverse": map[string]interface{}{
					"type":        "boolean",
					"description": "If true, apply the patch in reverse (unapply). Useful for reverting previously applied patches. Defaults to false.",
					"default":     false,
				},
				"three_way": map[string]interface{}{
					"type":        "boolean",
					"description": "If true, attempt a three-way merge when the patch does not apply cleanly. This can help resolve conflicts by using git's merge machinery. Defaults to false.",
					"default":     false,
				},
				"ignore_whitespace": map[string]interface{}{
					"type":        "boolean",
					"description": "If true, ignore whitespace differences when applying the patch. Defaults to false.",
					"default":     false,
				},
				"context_lines": map[string]interface{}{
					"type":        "integer",
					"description": "Number of context lines to use when applying the patch. Reduces this when patches fail due to context mismatch. Defaults to 3 (standard unified diff context).",
					"default":     3,
					"minimum":     0,
					"maximum":     10,
				},
			},
			"required": []string{"patch"},
		},
	}
}

// GitApplyPatchRequest represents the parsed arguments for a git_apply_patch tool call.
type GitApplyPatchRequest struct {
	Patch            string `json:"patch"`
	WorkingDirectory string `json:"working_directory,omitempty"`
	CheckOnly        bool   `json:"check_only,omitempty"`
	Reverse          bool   `json:"reverse,omitempty"`
	ThreeWay         bool   `json:"three_way,omitempty"`
	IgnoreWhitespace bool   `json:"ignore_whitespace,omitempty"`
	ContextLines     int    `json:"context_lines,omitempty"`
}

// Default constants for git_apply_patch
const (
	DefaultContextLines = 3
	MinContextLines     = 0
	MaxContextLines     = 10
)

// ForkSessionToolDef returns the MCP tool definition for the fork_session tool.
// This tool creates a new session forked from an existing session at a specified message.
func ForkSessionToolDef() map[string]interface{} {
	return map[string]interface{}{
		"name":        "fork_session",
		"description": "Create a new session forked from an existing session at a specified message. The forked session inherits the conversation history up to and including the fork point message, allowing exploration of alternative approaches without modifying the original conversation. Useful for trying different solutions, A/B testing prompts, or creating checkpoints in long conversations.",
		"inputSchema": map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"parent_session_id": map[string]interface{}{
					"type":        "string",
					"description": "The ID of the session to fork from. Must be a valid existing session UUID.",
				},
				"fork_point_message_id": map[string]interface{}{
					"type":        "string",
					"description": "The ID of the message at which to fork. All messages up to and including this message are inherited by the forked session. Must belong to the parent session.",
				},
				"title": map[string]interface{}{
					"type":        "string",
					"description": "Optional title for the forked session. If not provided, a default title will be generated based on the parent session.",
				},
				"provider": map[string]interface{}{
					"type":        "string",
					"description": "Optional LLM provider override for the forked session (e.g., 'openai', 'anthropic', 'gemini'). If not provided, inherits from parent session.",
				},
				"model": map[string]interface{}{
					"type":        "string",
					"description": "Optional model override for the forked session (e.g., 'gpt-4', 'claude-3-opus'). If not provided, inherits from parent session.",
				},
				"copy_messages": map[string]interface{}{
					"type":        "boolean",
					"description": "If true, copies all messages from parent up to fork point into the new session. If false (default), the fork references parent history (more space efficient but requires traversal).",
					"default":     false,
				},
				"copy_tool_calls": map[string]interface{}{
					"type":        "boolean",
					"description": "If true and copy_messages is true, also copies tool call records associated with the copied messages. Default is false.",
					"default":     false,
				},
				"copy_summaries": map[string]interface{}{
					"type":        "boolean",
					"description": "If true and copy_messages is true, also copies session summaries. Default is false.",
					"default":     false,
				},
			},
			"required": []string{"parent_session_id", "fork_point_message_id"},
		},
	}
}

// ForkSessionRequest represents the parsed arguments for a fork_session tool call.
type ForkSessionRequest struct {
	ParentSessionID    string `json:"parent_session_id"`
	ForkPointMessageID string `json:"fork_point_message_id"`
	Title              string `json:"title,omitempty"`
	Provider           string `json:"provider,omitempty"`
	Model              string `json:"model,omitempty"`
	CopyMessages       bool   `json:"copy_messages,omitempty"`
	CopyToolCalls      bool   `json:"copy_tool_calls,omitempty"`
	CopySummaries      bool   `json:"copy_summaries,omitempty"`
}

// ValidateForkSessionRequest validates a ForkSessionRequest.
func ValidateForkSessionRequest(req *ForkSessionRequest) error {
	if req.ParentSessionID == "" {
		return &ValidationError{Field: "parent_session_id", Message: "parent_session_id is required"}
	}
	if req.ForkPointMessageID == "" {
		return &ValidationError{Field: "fork_point_message_id", Message: "fork_point_message_id is required"}
	}
	return nil
}

// ForkSessionResult represents the result of a fork_session operation.
type ForkSessionResult struct {
	ForkedSessionID    string   `json:"forked_session_id"`
	ParentSessionID    string   `json:"parent_session_id"`
	ForkPointMessageID string   `json:"fork_point_message_id"`
	Title              string   `json:"title"`
	Provider           string   `json:"provider"`
	Model              string   `json:"model"`
	MessagesCopied     int      `json:"messages_copied,omitempty"`
	ToolCallsCopied    int      `json:"tool_calls_copied,omitempty"`
	SummariesCopied    int      `json:"summaries_copied,omitempty"`
	ForkDepth          int      `json:"fork_depth"`
	AncestorChain      []string `json:"ancestor_chain"`
}

// GetForkInfoToolDef returns the MCP tool definition for the get_fork_info tool.
// This tool retrieves detailed fork relationship information for a session.
func GetForkInfoToolDef() map[string]interface{} {
	return map[string]interface{}{
		"name":        "get_fork_info",
		"description": "Retrieve detailed fork relationship information for a session, including ancestor chain, fork depth, and descendant counts. Useful for understanding the structure of forked session hierarchies.",
		"inputSchema": map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"session_id": map[string]interface{}{
					"type":        "string",
					"description": "The ID of the session to get fork information for.",
				},
			},
			"required": []string{"session_id"},
		},
	}
}

// GetForkInfoRequest represents the parsed arguments for a get_fork_info tool call.
type GetForkInfoRequest struct {
	SessionID string `json:"session_id"`
}

// ValidateGetForkInfoRequest validates a GetForkInfoRequest.
func ValidateGetForkInfoRequest(req *GetForkInfoRequest) error {
	if req.SessionID == "" {
		return &ValidationError{Field: "session_id", Message: "session_id is required"}
	}
	return nil
}

// ListSessionForksToolDef returns the MCP tool definition for the list_session_forks tool.
// This tool lists all sessions forked from a given parent session.
func ListSessionForksToolDef() map[string]interface{} {
	return map[string]interface{}{
		"name":        "list_session_forks",
		"description": "List all sessions that were directly forked from a given parent session. Returns only direct children, not descendants of descendants.",
		"inputSchema": map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"session_id": map[string]interface{}{
					"type":        "string",
					"description": "The ID of the parent session to list forks for.",
				},
			},
			"required": []string{"session_id"},
		},
	}
}

// ListSessionForksRequest represents the parsed arguments for a list_session_forks tool call.
type ListSessionForksRequest struct {
	SessionID string `json:"session_id"`
}

// ValidateListSessionForksRequest validates a ListSessionForksRequest.
func ValidateListSessionForksRequest(req *ListSessionForksRequest) error {
	if req.SessionID == "" {
		return &ValidationError{Field: "session_id", Message: "session_id is required"}
	}
	return nil
}

// ValidateGitApplyPatchRequest validates a GitApplyPatchRequest and sets defaults.
func ValidateGitApplyPatchRequest(req *GitApplyPatchRequest) error {
	if req.Patch == "" {
		return &ValidationError{Field: "patch", Message: "patch is required"}
	}

	// Parse and validate the patch using the full parser
	patch, validationResult, err := ValidatePatchString(req.Patch)
	if err != nil {
		if parseErr, ok := err.(*ParseError); ok {
			return &ValidationError{
				Field:   "patch",
				Message: parseErr.Error(),
			}
		}
		return &ValidationError{Field: "patch", Message: fmt.Sprintf("failed to parse patch: %v", err)}
	}

	// Store the parsed patch for later use (could be used by handler)
	_ = patch

	// Check for validation errors
	if !validationResult.Valid {
		// Build error message from all validation errors
		var errMsgs []string
		for _, e := range validationResult.Errors {
			errMsgs = append(errMsgs, e.Error())
		}
		return &ValidationError{
			Field:   "patch",
			Message: fmt.Sprintf("invalid patch: %s", strings.Join(errMsgs, "; ")),
		}
	}

	// Set default context lines if not provided (0 is valid, so check if explicitly set)
	// Since Go defaults to 0, we rely on the caller or use a separate "set" flag
	// For simplicity, we keep 0 as valid (no context lines)
	if req.ContextLines < MinContextLines {
		req.ContextLines = MinContextLines
	}
	if req.ContextLines > MaxContextLines {
		return &ValidationError{Field: "context_lines", Message: "context_lines must not exceed 10"}
	}

	return nil
}

