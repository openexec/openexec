package blueprint

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/openexec/openexec/internal/actions"
	"github.com/openexec/openexec/internal/types"
)

// DefaultExecutor executes blueprint stages.
// Deterministic stages run shell commands; agentic stages use bounded subloops.
type DefaultExecutor struct {
	// WorkDir is the working directory for command execution.
	WorkDir string

	// Timeout is the default timeout for commands. If zero, 5 minutes is used.
	Timeout time.Duration

	// ActionRegistry contains deterministic Go-native actions.
	ActionRegistry *actions.Registry

	// AgenticRunner runs agentic stages. If nil, agentic stages fail.
	AgenticRunner AgenticRunner

	// OnCommandStart is called when a command starts.
	OnCommandStart func(stage *Stage, cmd string)

	// OnCommandComplete is called when a command completes.
	OnCommandComplete func(stage *Stage, cmd string, output string, err error)
}

// AgenticRunner executes agentic stages using an AI provider.
type AgenticRunner interface {
	// RunAgentic executes an agentic stage with the given prompt and returns output.
	RunAgentic(ctx context.Context, stage *Stage, input *StageInput) (string, map[string]string, error)
}

// NewDefaultExecutor creates a new default executor.
func NewDefaultExecutor(workDir string) *DefaultExecutor {
	return &DefaultExecutor{
		WorkDir: workDir,
		Timeout: 5 * time.Minute,
	}
}

// Execute runs a stage and returns the result.
func (e *DefaultExecutor) Execute(ctx context.Context, stage *Stage, input *StageInput) (*StageResult, error) {
	result := NewStageResult(stage.Name, 1)

	switch stage.Type {
	case types.StageTypeDeterministic:
		return e.executeDeterministic(ctx, stage, input, result)
	case types.StageTypeAgentic:
		return e.executeAgentic(ctx, stage, input, result)
	default:
		result.Fail(fmt.Sprintf("unknown stage type: %s", stage.Type))
		return result, fmt.Errorf("unknown stage type: %s", stage.Type)
	}
}

// executeDeterministic runs Go-native actions or shell commands for deterministic stages.
func (e *DefaultExecutor) executeDeterministic(ctx context.Context, stage *Stage, input *StageInput, result *StageResult) (*StageResult, error) {
	// 1. Try Action Registry first (Go-native logic)
	if stage.Action != "" && e.ActionRegistry != nil {
		if action, ok := e.ActionRegistry.Get(stage.Action); ok {
			resp, err := action.Execute(ctx, actions.ActionRequest{
				RunID:        input.RunID,
				WorkspaceDir: e.WorkDir,
				Inputs:       map[string]any{"task_description": input.TaskDescription},
			})
			if err != nil {
				result.Fail(err.Error())
				return result, nil
			}
			result.Status = resp.Status
			result.Output = resp.Output
			result.Error = resp.Error
			result.Artifacts = resp.Artifacts

			// If action succeeded, still run quality gates if it wasn't the gates action itself
			if result.Status == types.StageStatusCompleted && stage.Action != "run_gates" {
				e.runQualityGates(ctx, input, result)
			}
			return result, nil
		}
	}

	// 2. Fallback to shell commands
	if len(stage.Commands) > 0 {
		timeout := e.Timeout
		if stage.Timeout > 0 {
			timeout = stage.Timeout
		}

		var outputs []string
		for _, cmdStr := range stage.Commands {
			cmdCtx, cancel := context.WithTimeout(ctx, timeout)
			defer cancel()

			if e.OnCommandStart != nil {
				e.OnCommandStart(stage, cmdStr)
			}

			output, err := e.runCommand(cmdCtx, cmdStr)
			outputs = append(outputs, output)

			if e.OnCommandComplete != nil {
				e.OnCommandComplete(stage, cmdStr, output, err)
			}

			if err != nil {
				result.Output = strings.Join(outputs, "\n---\n")
				result.Fail(fmt.Sprintf("command failed: %s: %v", cmdStr, err))
				return result, nil
			}
		}
		result.Output = strings.Join(outputs, "\n---\n")
		result.Complete("all commands succeeded")
	} else {
		// No commands = automatic success
		result.Complete("no commands to execute")
	}

	// 3. Run Quality Gates (if configured and Action Registry available)
	e.runQualityGates(ctx, input, result)

	return result, nil
}

// runQualityGates executes configured quality gates and merges results into the stage result.
func (e *DefaultExecutor) runQualityGates(ctx context.Context, input *StageInput, result *StageResult) {
	if e.ActionRegistry == nil {
		return
	}

	runGates, ok := e.ActionRegistry.Get("run_gates")
	if !ok {
		return
	}

	gateResp, err := runGates.Execute(ctx, actions.ActionRequest{
		RunID:        input.RunID,
		WorkspaceDir: e.WorkDir,
		Inputs:       map[string]any{"task_description": input.TaskDescription},
	})
	if err != nil {
		// Internal error running gates, don't fail the stage but log it
		result.Output += fmt.Sprintf("\n\n⚠ Warning: failed to run quality gates: %v", err)
		return
	}

	// Merge gate results
	if gateResp.Status == types.StageStatusFailed {
		result.Status = types.StageStatusFailed
		result.Error = fmt.Sprintf("Quality gates failed: %s", gateResp.Error)
		result.Output += "\n\n=== QUALITY GATE FAILURE ===\n" + gateResp.Output
	} else if gateResp.Output != "" {
		result.Output += "\n\n=== QUALITY GATES PASSED ===\n" + gateResp.Output
	}
}

// runCommand executes a shell command and returns its output.
func (e *DefaultExecutor) runCommand(ctx context.Context, cmdStr string) (string, error) {
	workDir := e.WorkDir
	if workDir == "" {
		workDir = "."
	}

	// Use sh -c to handle shell features like pipes and redirects
	cmd := exec.CommandContext(ctx, "sh", "-c", cmdStr) // #nosec G204
	cmd.Dir = workDir

	// Propagate environment
	cmd.Env = os.Environ()

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()

	output := stdout.String()
	if stderr.Len() > 0 {
		output += "\n[stderr]\n" + stderr.String()
	}

	if err != nil {
		// Include stderr in error for better diagnostics
		if stderr.Len() > 0 {
			return output, fmt.Errorf("%w: %s", err, strings.TrimSpace(stderr.String()))
		}
		return output, err
	}

	return output, nil
}

// executeAgentic runs an agentic stage using the AI provider.
func (e *DefaultExecutor) executeAgentic(ctx context.Context, stage *Stage, input *StageInput, result *StageResult) (*StageResult, error) {
	if e.AgenticRunner == nil {
		result.Fail("agentic runner not configured")
		return result, fmt.Errorf("agentic runner not configured")
	}

	timeout := e.Timeout
	if stage.Timeout > 0 {
		timeout = stage.Timeout
	}

	agenticCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	output, artifacts, err := e.AgenticRunner.RunAgentic(agenticCtx, stage, input)
	if err != nil {
		result.Fail(err.Error())
		return result, nil // Return result, not error
	}

	// Copy artifacts to result
	for k, v := range artifacts {
		result.AddArtifact(k, v)
	}

	result.Complete(output)
	return result, nil
}

// SimpleAgenticRunner is a basic agentic runner that uses a callback function.
type SimpleAgenticRunner struct {
	// RunFunc is called to execute the agentic stage.
	RunFunc func(ctx context.Context, stage *Stage, input *StageInput) (string, map[string]string, error)
}

// RunAgentic implements AgenticRunner.
func (r *SimpleAgenticRunner) RunAgentic(ctx context.Context, stage *Stage, input *StageInput) (string, map[string]string, error) {
	if r.RunFunc == nil {
		return "", nil, fmt.Errorf("RunFunc not set")
	}
	return r.RunFunc(ctx, stage, input)
}

// LoopAgenticRunner runs agentic stages using a bounded loop with the Loop infrastructure.
type LoopAgenticRunner struct {
	// LoopFactory creates a new loop for each agentic stage.
	// stageName is passed to allow model tiering (e.g. using Opus for 'review' stage).
	LoopFactory func(stageName string, prompt string, workDir string, maxIterations int) (AgenticLoop, error)

	// MaxIterations is the maximum iterations for the bounded subloop. Default 10.
	MaxIterations int
}

// AgenticLoop is a minimal interface for loop execution.
type AgenticLoop interface {
	Run(ctx context.Context) error
	GetResult() (string, map[string]string, error)
}

// RunAgentic implements AgenticRunner using a bounded loop.
func (r *LoopAgenticRunner) RunAgentic(ctx context.Context, stage *Stage, input *StageInput) (string, map[string]string, error) {
	if r.LoopFactory == nil {
		return "", nil, fmt.Errorf("LoopFactory not set")
	}

	maxIter := r.MaxIterations
	if maxIter <= 0 {
		maxIter = 10
	}

	// Build prompt from stage and input
	prompt := buildAgenticPrompt(stage, input)

	loop, err := r.LoopFactory(stage.Name, prompt, input.WorkingDirectory, maxIter)
	if err != nil {
		return "", nil, fmt.Errorf("failed to create loop: %w", err)
	}

	if err := loop.Run(ctx); err != nil {
		return "", nil, fmt.Errorf("loop execution failed: %w", err)
	}

	return loop.GetResult()
}

// buildAgenticPrompt constructs the prompt for an agentic stage.
func buildAgenticPrompt(stage *Stage, input *StageInput) string {
	var sb strings.Builder

	sb.WriteString("## Stage: ")
	sb.WriteString(stage.Name)
	sb.WriteString("\n\n")

	if stage.Description != "" {
		sb.WriteString(stage.Description)
		sb.WriteString("\n\n")
	}

	if stage.Prompt != "" {
		sb.WriteString(stage.Prompt)
		sb.WriteString("\n\n")
	}

	sb.WriteString("## Task\n")
	sb.WriteString(input.TaskDescription)
	sb.WriteString("\n\n")

	if input.Briefing != "" {
		sb.WriteString("## Project Context & Briefing\n")
		sb.WriteString(input.Briefing)
		sb.WriteString("\n\n")
	}

	// Add context from previous stages
	if len(input.PreviousStages) > 0 {
		sb.WriteString("## Previous Stage Results\n")
		for _, prev := range input.PreviousStages {
			sb.WriteString(fmt.Sprintf("- **%s** (%s): %s\n", prev.StageName, prev.Status, truncate(prev.Output, 500)))
		}
		sb.WriteString("\n")
	}

	// Add context files
	if len(input.ContextPack) > 0 {
		sb.WriteString("## Context Files\n")
		for path := range input.ContextPack {
			sb.WriteString("- ")
			sb.WriteString(path)
			sb.WriteString("\n")
		}
		sb.WriteString("\n")
	}

	// Stage-specific instructions
	switch stage.Name {
	case "implement":
		sb.WriteString("Implement the requested changes. Create new files with your Write tool, and use git_apply_patch or Edit for modifying existing files.\n")
	case "fix_lint":
		sb.WriteString("Fix the linting errors from the previous stage. Use git_apply_patch or Edit for code modifications.\n")
	case "fix_tests":
		sb.WriteString("Fix the failing tests from the previous stage. Use git_apply_patch or Edit for code modifications.\n")
	case "review":
		sb.WriteString("Review the changes made in previous stages. Provide a summary of what was done and any concerns.\n")
	}

	sb.WriteString("\nWhen complete, emit an openexec_signal with type 'phase-complete'.\n")

	return sb.String()
}

// truncate shortens a string to maxLen, adding "..." if truncated.
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}
