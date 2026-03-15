package loop

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/openexec/openexec/internal/runner"
)

// Process represents a running Claude Code instance.
type Process struct {
	cmd    *exec.Cmd
	Stdout io.ReadCloser
	Stderr io.ReadCloser
}

// StartProcess spawns Claude Code (or a mock command) with the given config.
// Optional writers can be provided to tee stdout and stderr.
// Middleware wraps stdin/stdout/stderr for Deep-Trace interception if provided.
func StartProcess(ctx context.Context, cfg Config, stdoutW, stderrW io.Writer, m Middleware) (*Process, error) {
    name, args := buildCommand(cfg)
    // name is either "claude" or a test-controlled override
    cmd := exec.CommandContext(ctx, name, args...) // #nosec G204
    cmd.Dir = cfg.WorkDir
    // Propagate execution mode and workspace root to the child process
    env := os.Environ()
    if cfg.ExecMode != "" {
        env = append(env, "OPENEXEC_MODE="+cfg.ExecMode)
    }
    if cfg.WorkDir != "" {
        env = append(env, "WORKSPACE_ROOT="+cfg.WorkDir)
    }
    cmd.Env = env

	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("stdout pipe: %w", err)
	}

	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start process: %w", err)
	}

	finalStdout := io.ReadCloser(stdoutPipe)
	if stdoutW != nil {
		finalStdout = teeReadCloser{
			r: io.TeeReader(stdoutPipe, stdoutW),
			c: stdoutPipe,
		}
	}
	// Wrap stdout with middleware for tracing
	if m != nil {
		finalStdout = m.WrapStdout(finalStdout)
	}

	finalStderr := io.ReadCloser(stderrPipe)
	if stderrW != nil {
		finalStderr = teeReadCloser{
			r: io.TeeReader(stderrPipe, stderrW),
			c: stderrPipe,
		}
	}
	// Wrap stderr with middleware for tracing
	if m != nil {
		finalStderr = m.WrapStderr(finalStderr)
	}

	return &Process{
		cmd:    cmd,
		Stdout: finalStdout,
		Stderr: finalStderr,
	}, nil
}

type teeReadCloser struct {
	r io.Reader
	c io.Closer
}

func (t teeReadCloser) Read(p []byte) (n int, err error) {
	return t.r.Read(p)
}

func (t teeReadCloser) Close() error {
	return t.c.Close()
}

// Wait waits for the process to exit and returns the exit error (nil = clean exit).
func (p *Process) Wait() error {
	return p.cmd.Wait()
}

// Kill sends SIGKILL to the process.
func (p *Process) Kill() error {
	if p.cmd.Process == nil {
		return nil
	}
	return p.cmd.Process.Kill()
}

// CaptureStderr copies stderr to a log file in dir and a memory buffer.
// Blocks until EOF. Removes the file on clean exit if it's empty.
// Returns the captured stderr content (tail, up to 4KB) for error diagnostics.
func CaptureStderr(r io.Reader, dir string) (string, error) {
	if dir == "" {
		dir = "."
	}
	name := fmt.Sprintf("claude-%s.log", time.Now().Format("20060102-150405"))
	path := filepath.Join(dir, name)

	// path is constructed from a timestamp-based filename, safe to create
	f, err := os.Create(path) // #nosec G304
	if err != nil {
		return "", err
	}

	// Capture into both file and a tail buffer for diagnostics
	var buf stderrTailBuffer
	w := io.MultiWriter(f, &buf)
	n, copyErr := io.Copy(w, r)
	_ = f.Close()

	// Remove empty log files — they provide no diagnostic value.
	if n == 0 {
		_ = os.Remove(path)
	}

	return buf.String(), copyErr
}

// stderrTailBuffer keeps the last 4KB of stderr for error diagnostics.
type stderrTailBuffer struct {
	data []byte
}

const stderrTailMax = 4096

func (b *stderrTailBuffer) Write(p []byte) (int, error) {
	b.data = append(b.data, p...)
	if len(b.data) > stderrTailMax {
		b.data = b.data[len(b.data)-stderrTailMax:]
	}
	return len(p), nil
}

func (b *stderrTailBuffer) String() string {
	return strings.TrimSpace(string(b.data))
}

// autonomousPreamble is prepended to every prompt to ensure Claude operates
// autonomously without attempting interactive workflows.
const autonomousPreamble = `IMPORTANT: You are running autonomously in a non-interactive pipeline. ` +
    `There is no human operator present. Do NOT plan — proceed directly with implementation. ` +
    `Work independently and make reasonable decisions. ` +
    `For code edits, prefer the git_apply_patch MCP tool for unified diffs, but you may use your built-in Write and Edit tools when creating new files or when patches are impractical. ` +
    `If you are genuinely blocked, use the openexec_signal tool with type "blocked" or "decision-point".

` + `If the project does not yet have a .gitignore file, create an appropriate one for the tech stack before writing other code.

`

// disabledTools lists tools that require interactive approval and are useless
// in non-interactive (-p) mode. If called, they block the agent in a loop.
var disabledTools = []string{
	"EnterPlanMode",
	"ExitPlanMode",
	"AskUserQuestion",
}

func buildCommand(cfg Config) (string, []string) {
	// If the server/caller already provided a command name and args, use them.
	if cfg.CommandName != "" {
		return cfg.CommandName, cfg.CommandArgs
	}

	// Resolve runner using centralized logic.
	cliCmd, cmdArgs, err := runner.Resolve(
		cfg.ExecutorModel,
		cfg.RunnerCommand,
		cfg.RunnerArgs,
	)
	if err != nil {
		// Fallback to internal Claude default if resolution fails
		return "claude", buildClaudeArgs(cfg)
	}

	// For Claude, ensuring we use our specific flags if it was using defaults.
	if strings.Contains(strings.ToLower(cliCmd), "claude") {
		return cliCmd, buildClaudeArgs(cfg)
	}

    // For Gemini, gate --yolo by execution mode (danger only)
    if strings.Contains(strings.ToLower(cliCmd), "gemini") {
        danger := cfg.ExecMode == "danger-full-access" || os.Getenv("OPENEXEC_MODE") == "danger-full-access"
        if danger {
            hasYolo := false
            for _, a := range cmdArgs {
                if a == "--yolo" { hasYolo = true; break }
            }
            if !hasYolo { cmdArgs = append(cmdArgs, "--yolo") }
        } else {
            filtered := make([]string, 0, len(cmdArgs))
            for _, a := range cmdArgs { if a != "--yolo" { filtered = append(filtered, a) } }
            cmdArgs = filtered
        }
    }

	return cliCmd, cmdArgs
}

func buildClaudeArgs(cfg Config) []string {
	prompt := autonomousPreamble + cfg.Prompt

    args := []string{
        "-p", prompt,
        "--output-format", "stream-json",
        "--verbose",
        "--max-turns", "50",
        "--disallowedTools", strings.Join(disabledTools, ","),
    }
    // Skip permission prompts for non-interactive execution.
    // In -p mode, there is no user to approve tool calls, so the agent
    // would block forever on Write/Edit/Bash permission prompts.
    // Safety is enforced by the MCP toolset and execution mode constraints.
    if cfg.ExecMode == "workspace-write" || cfg.ExecMode == "danger-full-access" || os.Getenv("OPENEXEC_MODE") == "danger-full-access" {
        args = append([]string{"--dangerously-skip-permissions"}, args...)
    }
	if cfg.MCPConfigPath != "" {
		args = append(args, "--mcp-config", cfg.MCPConfigPath)
	}
	return args
}
