package infra

import (
	"context"
	"errors"
	"os/exec"
	"time"
)

// Runner executes a resolved Command. The interface exists so MCP handler
// tests can capture argv instead of spawning real infrastructure tooling.
type Runner interface {
	// Run executes binary with args and returns combined output and the
	// process exit code. err is non-nil for spawn failures, timeouts, and
	// non-zero exits (output is still returned for diagnostics).
	Run(ctx context.Context, binary string, args []string) (output string, exitCode int, err error)
}

// DefaultTimeout bounds a single infra command. Playbooks and plans are
// minutes-long by nature; anything past this is hung.
const DefaultTimeout = 10 * time.Minute

// ExecRunner runs commands via exec.CommandContext with a strict argv
// array. There is no shell anywhere in this path: no sh -c, no string
// joining, no expansion. An injected "staging; rm -rf /" is one literal
// argument the target binary will reject.
type ExecRunner struct {
	// Timeout per command; zero means DefaultTimeout.
	Timeout time.Duration
}

// Run implements Runner.
func (e *ExecRunner) Run(ctx context.Context, binary string, args []string) (string, int, error) {
	timeout := e.Timeout
	if timeout == 0 {
		timeout = DefaultTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, binary, args...)
	out, err := cmd.CombinedOutput()

	exitCode := 0
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			exitCode = ee.ExitCode()
		} else {
			exitCode = -1 // spawn failure or kill-by-timeout
		}
	}
	return string(out), exitCode, err
}
