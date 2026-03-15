package runner

import (
	"fmt"
	"os"
	"path/filepath"
)

// PermissionTier defines the access level for a run.
// This provides a unified model across all runners (Claude, Codex, Gemini).
type PermissionTier string

const (
	// TierReadOnly allows only reading files and running read-only commands.
	// No file writes, no network access beyond APIs, no code execution with side effects.
	TierReadOnly PermissionTier = "read-only"

	// TierWorkspaceWrite allows writing within the workspace directory.
	// This is the default tier for most development tasks.
	TierWorkspaceWrite PermissionTier = "workspace-write"

	// TierDangerFull allows unrestricted access including:
	// - Writing anywhere on the filesystem
	// - Running arbitrary shell commands
	// - Network access
	// Use only for trusted, fully automated scenarios.
	TierDangerFull PermissionTier = "danger-full-access"
)

// ParseTier converts a string to a PermissionTier, returning TierWorkspaceWrite as default.
func ParseTier(s string) PermissionTier {
	switch s {
	case "read-only":
		return TierReadOnly
	case "workspace-write", "suggest", "":
		return TierWorkspaceWrite
	case "danger-full-access", "full", "unrestricted":
		return TierDangerFull
	default:
		return TierWorkspaceWrite
	}
}

// String returns the string representation of the tier.
func (t PermissionTier) String() string {
	return string(t)
}

// IsReadOnly returns true if this tier only allows read operations.
func (t PermissionTier) IsReadOnly() bool {
	return t == TierReadOnly
}

// AllowsWrites returns true if this tier allows any write operations.
func (t PermissionTier) AllowsWrites() bool {
	return t == TierWorkspaceWrite || t == TierDangerFull
}

// AllowsFullAccess returns true if this tier has unrestricted access.
func (t PermissionTier) AllowsFullAccess() bool {
	return t == TierDangerFull
}

// ToRunnerEnv converts the permission tier to environment variables for a specific runner.
// This enables consistent permission enforcement across Claude, Codex, and Gemini CLIs.
func (t PermissionTier) ToRunnerEnv(runner, workspacePath string) map[string]string {
	env := make(map[string]string)

	// Absolute workspace path for jailing
	absWorkspace, err := filepath.Abs(workspacePath)
	if err != nil {
		absWorkspace = workspacePath
	}

	// Common environment variables for all runners
	env["OPENEXEC_PERMISSION_TIER"] = string(t)
	env["OPENEXEC_WORKSPACE_ROOT"] = absWorkspace

	switch runner {
	case "claude":
		env = t.claudeEnv(env, absWorkspace)
	case "codex":
		env = t.codexEnv(env, absWorkspace)
	case "gemini":
		env = t.geminiEnv(env, absWorkspace)
	}

	return env
}

// claudeEnv configures Claude Code specific environment.
func (t PermissionTier) claudeEnv(env map[string]string, workspace string) map[string]string {
	switch t {
	case TierReadOnly:
		// Claude Code doesn't have a built-in read-only mode,
		// but we can set environment hints for hooks/wrappers
		env["CLAUDE_CODE_READ_ONLY"] = "true"
		env["CLAUDE_CODE_ALLOWED_PATHS"] = workspace
	case TierWorkspaceWrite:
		env["CLAUDE_CODE_ALLOWED_PATHS"] = workspace
	case TierDangerFull:
		// No restrictions
		env["CLAUDE_CODE_ALLOW_ALL"] = "true"
	}
	return env
}

// codexEnv configures Codex CLI specific environment.
func (t PermissionTier) codexEnv(env map[string]string, workspace string) map[string]string {
	switch t {
	case TierReadOnly:
		env["CODEX_SANDBOX"] = "strict"
		env["CODEX_WRITABLE_PATHS"] = ""
	case TierWorkspaceWrite:
		env["CODEX_SANDBOX"] = "workspace"
		env["CODEX_WRITABLE_PATHS"] = workspace
	case TierDangerFull:
		env["CODEX_SANDBOX"] = "off"
	}
	return env
}

// geminiEnv configures Gemini CLI specific environment.
func (t PermissionTier) geminiEnv(env map[string]string, workspace string) map[string]string {
	switch t {
	case TierReadOnly:
		env["GEMINI_SAFETY_LEVEL"] = "strict"
		env["GEMINI_ALLOWED_TOOLS"] = "read"
	case TierWorkspaceWrite:
		env["GEMINI_SAFETY_LEVEL"] = "standard"
		env["GEMINI_WORKSPACE"] = workspace
	case TierDangerFull:
		env["GEMINI_SAFETY_LEVEL"] = "minimal"
	}
	return env
}

// ToArgs converts the permission tier to CLI arguments for a specific runner.
func (t PermissionTier) ToArgs(runner, workspacePath string) []string {
	var args []string

	switch runner {
	case "claude":
		args = t.claudeArgs(workspacePath)
	case "codex":
		args = t.codexArgs(workspacePath)
	case "gemini":
		args = t.geminiArgs(workspacePath)
	}

	return args
}

// claudeArgs returns Claude-specific CLI arguments for this tier.
func (t PermissionTier) claudeArgs(workspace string) []string {
	switch t {
	case TierReadOnly:
		return []string{"--allowedTools", "Read,Glob,Grep,WebSearch,WebFetch"}
	case TierWorkspaceWrite:
		return []string{"--allowedTools", "Read,Write,Edit,Glob,Grep,Bash,WebSearch,WebFetch"}
	case TierDangerFull:
		return []string{"--dangerouslySkipPermissions"}
	default:
		return nil
	}
}

// codexArgs returns Codex-specific CLI arguments for this tier.
func (t PermissionTier) codexArgs(workspace string) []string {
	switch t {
	case TierReadOnly:
		return []string{"--readonly"}
	case TierWorkspaceWrite:
		return []string{"--workspace", workspace}
	case TierDangerFull:
		return []string{"--no-sandbox"}
	default:
		return nil
	}
}

// geminiArgs returns Gemini-specific CLI arguments for this tier.
func (t PermissionTier) geminiArgs(workspace string) []string {
	switch t {
	case TierReadOnly:
		return []string{"--safety", "strict"}
	case TierWorkspaceWrite:
		return []string{"--workspace", workspace}
	case TierDangerFull:
		return []string{"--safety", "minimal"}
	default:
		return nil
	}
}

// ValidatePathAccess checks if a path is accessible under the given tier and workspace.
// Returns an error if access would be denied.
func ValidatePathAccess(tier PermissionTier, workspace, path string, isWrite bool) error {
	// Read-only tier denies all writes
	if tier == TierReadOnly && isWrite {
		return fmt.Errorf("write access denied: tier is read-only")
	}

	// Full access tier allows everything
	if tier == TierDangerFull {
		return nil
	}

	// Workspace write tier: validate path is within workspace
	if tier == TierWorkspaceWrite {
		absWorkspace, err := filepath.Abs(workspace)
		if err != nil {
			return fmt.Errorf("failed to resolve workspace path: %w", err)
		}

		absPath, err := filepath.Abs(path)
		if err != nil {
			return fmt.Errorf("failed to resolve target path: %w", err)
		}

		// Resolve symlinks to prevent escapes
		realWorkspace, err := filepath.EvalSymlinks(absWorkspace)
		if err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("failed to resolve workspace symlinks: %w", err)
		}
		if realWorkspace == "" {
			realWorkspace = absWorkspace
		}

		realPath, err := filepath.EvalSymlinks(absPath)
		if err != nil && !os.IsNotExist(err) {
			// For new files, check parent directory
			realPath = absPath
		}
		if realPath == "" {
			realPath = absPath
		}

		// Check containment
		rel, err := filepath.Rel(realWorkspace, realPath)
		if err != nil || len(rel) > 2 && rel[:3] == ".." + string(filepath.Separator) || rel == ".." {
			return fmt.Errorf("path %q is outside workspace %q", path, workspace)
		}
	}

	return nil
}

// ApplyToEnvironment merges permission environment variables into an existing environment slice.
func ApplyToEnvironment(tier PermissionTier, runner, workspace string, environ []string) []string {
	permEnv := tier.ToRunnerEnv(runner, workspace)
	for k, v := range permEnv {
		environ = append(environ, k+"="+v)
	}
	return environ
}
