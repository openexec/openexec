package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"sync"

	"github.com/openexec/openexec/internal/loop"
	pagent "github.com/openexec/openexec/pkg/agent"
)

// CoordinatorConfig holds configuration for the task coordinator.
type CoordinatorConfig struct {
	// Provider is the frontier model used for planning and merging.
	Provider pagent.ProviderAdapter
	// WorkerProvider is the model used for worker agents (can be cheaper).
	WorkerProvider pagent.ProviderAdapter
	// MaxWorkers is the maximum number of concurrent worker agents.
	MaxWorkers int
	// WorkDir is the project working directory.
	WorkDir string
	// Model is the model ID for the coordinator provider.
	Model string
	// WorkerModel is the model ID for the worker provider.
	WorkerModel string
}

// WorkPlan is the decomposition of a task into subtasks.
type WorkPlan struct {
	Subtasks []Subtask `json:"subtasks"`
}

// Subtask is a single unit of work within a work plan.
type Subtask struct {
	ID           string   `json:"id"`
	Description  string   `json:"description"`
	Files        []string `json:"files"`
	Dependencies []string `json:"dependencies"`
}

// WorkerResult holds the outcome of a single worker's execution.
type WorkerResult struct {
	SubtaskID string
	Status    string // "completed", "failed"
	Output    string
	Patches   string // git diff output
	Error     string
}

// TaskCoordinator decomposes tasks into subtasks, runs workers in parallel,
// and merges their results. It uses a frontier model for planning/merging
// and (optionally cheaper) worker models for execution.
type TaskCoordinator struct {
	config CoordinatorConfig
}

// NewTaskCoordinator creates a new coordinator with the given configuration.
func NewTaskCoordinator(config CoordinatorConfig) *TaskCoordinator {
	if config.MaxWorkers <= 0 {
		config.MaxWorkers = 4
	}
	if config.WorkerProvider == nil {
		config.WorkerProvider = config.Provider
	}
	if config.WorkerModel == "" {
		config.WorkerModel = config.Model
	}
	return &TaskCoordinator{config: config}
}

// Plan asks the frontier model to decompose a task into independent subtasks.
// It validates that subtasks don't have overlapping file assignments.
func (c *TaskCoordinator) Plan(ctx context.Context, task string, files []string) (*WorkPlan, error) {
	fileList := strings.Join(files, "\n")

	prompt := fmt.Sprintf(`You are a task coordinator. Decompose this task into independent subtasks.

Task: %s

Available files:
%s

Respond with JSON only (no markdown fences):
{
  "subtasks": [
    {"id": "1", "description": "...", "files": ["path/to/file"], "dependencies": []},
    {"id": "2", "description": "...", "files": ["other/file"], "dependencies": ["1"]}
  ]
}

Rules:
- Each subtask should modify different files (no overlapping file assignments)
- Use dependencies only when one subtask's output is needed by another
- Keep subtasks focused and independent where possible
- Return at least one subtask`, task, fileList)

	req := pagent.Request{
		Model: c.config.Model,
		Messages: []pagent.Message{
			pagent.NewTextMessage(pagent.RoleUser, prompt),
		},
		MaxTokens: 4096,
	}

	resp, err := c.config.Provider.Complete(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("coordinator plan failed: %w", err)
	}

	text := resp.GetText()
	// Strip markdown code fences if present
	text = stripCodeFences(text)

	var plan WorkPlan
	if err := json.Unmarshal([]byte(text), &plan); err != nil {
		return nil, fmt.Errorf("failed to parse work plan: %w (response: %s)", err, text)
	}

	if len(plan.Subtasks) == 0 {
		return nil, fmt.Errorf("coordinator returned empty work plan")
	}

	// Validate: check for file conflicts between subtasks
	if err := validateNoFileConflicts(&plan); err != nil {
		return nil, err
	}

	return &plan, nil
}

// Execute runs worker agents for each subtask, respecting dependency ordering.
// Independent subtasks run in parallel; dependent subtasks wait for their dependencies.
func (c *TaskCoordinator) Execute(ctx context.Context, plan *WorkPlan, briefing string) ([]*WorkerResult, error) {
	// Build dependency graph
	order, err := topologicalSort(plan.Subtasks)
	if err != nil {
		return nil, fmt.Errorf("dependency cycle detected: %w", err)
	}

	// Track completed subtask IDs
	completedMu := sync.Mutex{}
	completed := make(map[string]*WorkerResult)

	// Group subtasks into waves based on dependency ordering
	waves := buildWaves(order, plan.Subtasks)

	for _, wave := range waves {
		// Run all subtasks in this wave in parallel
		var wg sync.WaitGroup
		resultsCh := make(chan *WorkerResult, len(wave))
		sem := make(chan struct{}, c.config.MaxWorkers)

		for _, subtask := range wave {
			wg.Add(1)
			go func(st Subtask) {
				defer wg.Done()
				sem <- struct{}{}
				defer func() { <-sem }()

				result := c.runWorker(ctx, st, briefing)

				completedMu.Lock()
				completed[st.ID] = result
				completedMu.Unlock()

				resultsCh <- result
			}(subtask)
		}

		wg.Wait()
		close(resultsCh)

		// Check for failures in this wave
		for result := range resultsCh {
			if result.Status == "failed" {
				// Continue executing other waves despite failures
				// The merge step will report which subtasks failed
			}
		}
	}

	// Collect results in dependency order
	results := make([]*WorkerResult, 0, len(plan.Subtasks))
	for _, id := range order {
		if r, ok := completed[id]; ok {
			results = append(results, r)
		}
	}

	return results, nil
}

// Merge combines worker results by applying patches sequentially in dependency order.
// Returns a combined output summary or error if patches conflict.
func (c *TaskCoordinator) Merge(ctx context.Context, results []*WorkerResult) (string, error) {
	var summaries []string
	var failedIDs []string

	for _, r := range results {
		if r.Status == "failed" {
			failedIDs = append(failedIDs, r.SubtaskID)
			summaries = append(summaries, fmt.Sprintf("Subtask %s: FAILED - %s", r.SubtaskID, r.Error))
			continue
		}

		// Apply patch if present
		if r.Patches != "" {
			if err := applyPatch(ctx, c.config.WorkDir, r.Patches); err != nil {
				return "", fmt.Errorf("conflict applying patch for subtask %s: %w", r.SubtaskID, err)
			}
		}

		summaries = append(summaries, fmt.Sprintf("Subtask %s: completed\n%s", r.SubtaskID, r.Output))
	}

	summary := strings.Join(summaries, "\n\n---\n\n")
	if len(failedIDs) > 0 {
		summary += fmt.Sprintf("\n\nWarning: %d subtask(s) failed: %s", len(failedIDs), strings.Join(failedIDs, ", "))
	}

	return summary, nil
}

// runWorker executes a single subtask using an API runner with tool execution.
func (c *TaskCoordinator) runWorker(ctx context.Context, subtask Subtask, briefing string) *WorkerResult {
	result := &WorkerResult{
		SubtaskID: subtask.ID,
		Status:    "completed",
	}

	// Build focused prompt for this subtask
	prompt := fmt.Sprintf(`You are a worker agent. Complete the following subtask.

Subtask: %s

Files to work with:
%s

%s

Instructions:
- Focus only on the files listed above
- Make minimal, targeted changes
- Do not modify files outside your assignment`, subtask.Description, strings.Join(subtask.Files, "\n"), briefing)

	// Build tool definitions
	tools := loop.BuildAPIToolDefinitions()

	// Create event channel and runner
	ch := make(chan loop.Event, 100)
	runner := loop.NewAPIRunner(loop.APIRunnerConfig{
		Provider: c.config.WorkerProvider,
		Model:    c.config.WorkerModel,
		Prompt:   prompt,
		WorkDir:  c.config.WorkDir,
		MaxTurns: 30,
		Tools:    tools,
	}, ch)

	// Consume events in background to capture output
	var lastOutput string
	eventsDone := make(chan struct{})
	go func() {
		defer close(eventsDone)
		for event := range ch {
			if event.Text != "" {
				lastOutput = event.Text
			}
		}
	}()

	// Run the agent
	if err := runner.Run(ctx); err != nil {
		result.Status = "failed"
		result.Error = err.Error()
		<-eventsDone
		return result
	}

	<-eventsDone
	result.Output = lastOutput

	// Capture git diff for this worker's changes
	diff, err := captureGitDiff(ctx, c.config.WorkDir)
	if err == nil && diff != "" {
		result.Patches = diff
	}

	return result
}

// stripCodeFences removes markdown code fences from JSON responses.
func stripCodeFences(s string) string {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "```json") {
		s = strings.TrimPrefix(s, "```json")
	} else if strings.HasPrefix(s, "```") {
		s = strings.TrimPrefix(s, "```")
	}
	if strings.HasSuffix(s, "```") {
		s = strings.TrimSuffix(s, "```")
	}
	return strings.TrimSpace(s)
}

// validateNoFileConflicts checks that no two subtasks modify the same file.
func validateNoFileConflicts(plan *WorkPlan) error {
	fileOwner := make(map[string]string) // file -> subtask ID
	for _, st := range plan.Subtasks {
		for _, f := range st.Files {
			if owner, exists := fileOwner[f]; exists {
				return fmt.Errorf("file conflict: %q is assigned to both subtask %s and %s", f, owner, st.ID)
			}
			fileOwner[f] = st.ID
		}
	}
	return nil
}

// topologicalSort returns subtask IDs in dependency order.
// Returns error if a cycle is detected.
func topologicalSort(subtasks []Subtask) ([]string, error) {
	// Build adjacency and in-degree maps
	inDegree := make(map[string]int)
	dependents := make(map[string][]string) // id -> list of IDs that depend on it
	subtaskMap := make(map[string]Subtask)

	for _, st := range subtasks {
		subtaskMap[st.ID] = st
		if _, exists := inDegree[st.ID]; !exists {
			inDegree[st.ID] = 0
		}
		for _, dep := range st.Dependencies {
			dependents[dep] = append(dependents[dep], st.ID)
			inDegree[st.ID]++
		}
	}

	// Kahn's algorithm
	var queue []string
	for id, deg := range inDegree {
		if deg == 0 {
			queue = append(queue, id)
		}
	}

	var order []string
	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]
		order = append(order, id)

		for _, depID := range dependents[id] {
			inDegree[depID]--
			if inDegree[depID] == 0 {
				queue = append(queue, depID)
			}
		}
	}

	if len(order) != len(subtasks) {
		return nil, fmt.Errorf("dependency cycle detected: only %d of %d subtasks could be ordered", len(order), len(subtasks))
	}

	return order, nil
}

// buildWaves groups subtasks into parallel execution waves.
// Each wave contains subtasks whose dependencies are all in previous waves.
func buildWaves(order []string, subtasks []Subtask) [][]Subtask {
	subtaskMap := make(map[string]Subtask)
	for _, st := range subtasks {
		subtaskMap[st.ID] = st
	}

	completed := make(map[string]bool)
	var waves [][]Subtask

	remaining := make(map[string]bool)
	for _, id := range order {
		remaining[id] = true
	}

	for len(remaining) > 0 {
		var wave []Subtask
		for _, id := range order {
			if !remaining[id] {
				continue
			}
			st := subtaskMap[id]
			// Check if all dependencies are completed
			ready := true
			for _, dep := range st.Dependencies {
				if !completed[dep] {
					ready = false
					break
				}
			}
			if ready {
				wave = append(wave, st)
			}
		}

		if len(wave) == 0 {
			// Safety: should not happen after successful topological sort
			break
		}

		for _, st := range wave {
			completed[st.ID] = true
			delete(remaining, st.ID)
		}

		waves = append(waves, wave)
	}

	return waves
}

// captureGitDiff runs git diff in the working directory and returns the output.
func captureGitDiff(ctx context.Context, workDir string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", "diff")
	cmd.Dir = workDir
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// applyPatch applies a unified diff patch to the working directory.
func applyPatch(ctx context.Context, workDir string, patch string) error {
	cmd := exec.CommandContext(ctx, "git", "apply", "--verbose", "-")
	cmd.Dir = workDir
	cmd.Stdin = strings.NewReader(patch)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git apply failed: %s\n%w", string(out), err)
	}
	return nil
}
