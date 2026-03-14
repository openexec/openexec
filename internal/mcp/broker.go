package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/openexec/openexec/pkg/telemetry"
	"go.opentelemetry.io/otel/trace"
)

// PermissionMode defines the execution tier.
type PermissionMode string

const (
	ModeSuggest           PermissionMode = "suggest"             // Read-only + Propose Patch
	ModeAutoEdit          PermissionMode = "auto-edit"           // Apply Patch + Read-only
	ModeFullAuto          PermissionMode = "danger-full-access"  // Command allowlist + write_file
)

// ToolBroker enforces tiered permissions across all MCP tools.
type ToolBroker struct {
	mode  PermissionMode
	runID string // Current run ID for tracing
}

// SetRunID sets the current run ID for tracing context.
func (b *ToolBroker) SetRunID(runID string) {
	b.runID = runID
}

// StartToolSpan creates a traced span for a tool invocation.
// Returns the context with span and a function to end the span.
func (b *ToolBroker) StartToolSpan(ctx context.Context, toolName string, args map[string]interface{}) (context.Context, trace.Span) {
	return telemetry.StartToolSpan(ctx, b.runID, toolName, args)
}

// Mode returns the current permission mode.
func (b *ToolBroker) Mode() PermissionMode {
	return b.mode
}

// NewToolBroker creates a broker based on the current environment or config.
func NewToolBroker(modeOverride string) *ToolBroker {
	modeStr := modeOverride
	if modeStr == "" {
		modeStr = os.Getenv("OPENEXEC_MODE")
	}
	mode := PermissionMode(modeStr)
	
	// Normalize and apply secure default
	switch mode {
	case ModeSuggest, ModeAutoEdit, ModeFullAuto:
		// valid
	default:
		mode = ModeAutoEdit // Secure default: Allow patches but no shell/write_file
	}
	
	return &ToolBroker{mode: mode}
}

// Authorize checks if a tool call is permitted in the current mode.
func (b *ToolBroker) Authorize(toolName string, arguments string) (bool, string) {
	switch toolName {
	case "read_file", "axon_signal", "openexec_signal", "get_fork_info", "list_session_forks", "fork_session":
		return true, "" // Always allowed (Read-only or control plane)

	case "git_apply_patch":
		if b.mode == ModeSuggest {
			// In suggest mode, only allow dry-run (check_only=true)
			var args struct {
				CheckOnly bool `json:"check_only"`
			}
			if err := json.Unmarshal([]byte(arguments), &args); err != nil || !args.CheckOnly {
				return false, "[Suggest Mode] git_apply_patch requires check_only=true; propose patches as reasoning text for manual apply"
			}
		}
		return true, ""

	case "write_file":
		if b.mode != ModeFullAuto {
			return false, "[Security] write_file is restricted to Full Auto (danger) mode; please use git_apply_patch for workspace edits"
		}
		return true, ""

	case "run_shell_command":
		if b.mode != ModeFullAuto {
			return false, fmt.Sprintf("[%s Mode] run_shell_command is disabled; switch to Full Auto (danger) mode to enable shell access", b.mode)
		}
		return b.validateShellCommand(arguments)

	default:
		// For safety, restrict unknown tools in all but Full Auto
		if b.mode != ModeFullAuto {
			return false, fmt.Sprintf("[%s Mode] unknown tool '%s' is restricted", b.mode, toolName)
		}
		return true, ""
	}
}

func (b *ToolBroker) validateShellCommand(arguments string) (bool, string) {
	// Implementation of allowlist logic
	allowedCommands := map[string]bool{
		"go": true, "npm": true, "ls": true, "pwd": true, "git": true,
		"cat": true, "grep": true, "find": true, "mkdir": true, "rm": true,
		"touch": true, "cp": true, "mv": true, "chmod": true, "chown": true,
		"pytest": true, "ruff": true, "mypy": true, "black": true, "isort": true,
		"tsc": true, "vitest": true, "eslint": true, "prettier": true, "python": true, "python3": true,
		"uv": true, "pip": true, "poetry": true, "make": true, "just": true,
		"echo": true, "printf": true, "wc": true, "sleep": true, "head": true, "tail": true,
		"bash": true, "sh": true, "date": true, "env": true, "exit": true,
	}

	// Parse JSON arguments to extract command
	var args struct {
		Command string `json:"command"`
	}
	if err := json.Unmarshal([]byte(arguments), &args); err != nil {
		return false, "invalid command arguments"
	}

	fields := strings.Fields(args.Command)
	if len(fields) == 0 {
		return false, "empty command"
	}

	baseCmd := fields[0]
	if !allowedCommands[baseCmd] {
		return false, "command '" + baseCmd + "' is not in the allowlist"
	}

	return true, ""
}
