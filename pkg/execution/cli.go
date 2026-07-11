package execution

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/openexec/openexec/pkg/runner"
)

// AgentCLIExecutor is the core-owned adapter for local coding-agent CLIs.
// Callers use this interface instead of constructing CLI processes themselves.
type AgentCLIExecutor struct {
	OverrideCommand string
	Streams         Streams
}

func (e *AgentCLIExecutor) Execute(ctx context.Context, req Request) (Result, error) {
	started := time.Now().UTC()
	result := Result{Model: req.Model, Sandbox: req.Sandbox, StartedAt: started, Outcome: OutcomeFailed}
	finish := func() { result.EndedAt = time.Now().UTC() }

	if strings.TrimSpace(req.WorkingDir) == "" {
		finish()
		return result, fmt.Errorf("execution working directory is required")
	}
	if strings.TrimSpace(req.Prompt) == "" {
		finish()
		return result, fmt.Errorf("execution prompt is required")
	}
	if req.Sandbox.Mode != "danger-full-access" {
		finish()
		return result, fmt.Errorf("agent CLI executor cannot enforce sandbox mode %q", req.Sandbox.Mode)
	}

	command, _, err := runner.Resolve(req.Model, e.OverrideCommand, nil)
	if err != nil {
		finish()
		return result, err
	}
	result.Executor = command

	var cmd *exec.Cmd
	var stdin string
	if strings.Contains(strings.ToLower(command), "claude") {
		cmd = exec.CommandContext(ctx, command, "--dangerously-skip-permissions", "-p", req.Prompt)
	} else {
		cmd = exec.CommandContext(ctx, command, "--prompt", "-")
		stdin = req.Prompt
	}
	cmd.Dir = req.WorkingDir
	cmd.Stdout = e.Streams.Stdout
	cmd.Stderr = e.Streams.Stderr
	if cmd.Stdout == nil {
		cmd.Stdout = os.Stdout
	}
	if cmd.Stderr == nil {
		cmd.Stderr = os.Stderr
	}
	if stdin != "" {
		cmd.Stdin = strings.NewReader(stdin)
	}
	err = cmd.Run()
	finish()
	if err != nil {
		return result, fmt.Errorf("executor %s failed: %w", command, err)
	}
	result.Outcome = OutcomeSucceeded
	return result, nil
}
