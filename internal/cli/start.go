package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/openexec/openexec/internal/project"
	"github.com/openexec/openexec/internal/release"
	"github.com/spf13/cobra"
)

var (
	startPort       int
	startWorkers    int
	startTimeout    int
	startExecutor   string
	startReviewer   string
	startDaemon     bool
	executionBinary string
	runNoReview     bool
)

// Task represents a task to execute
type Task struct {
	ID          string   `json:"id"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	StoryID     string   `json:"story_id,omitempty"`
	Status      string   `json:"status"`
	DependsOn   []string `json:"depends_on,omitempty"`
}

// TasksFile represents the tasks.json structure
type TasksFile struct {
	Tasks []Task `json:"tasks"`
}

// CreateLoopRequest is the request body for creating a loop
type CreateLoopRequest struct {
	Prompt        string `json:"prompt"`
	WorkDir       string `json:"work_dir"`
	MaxIterations int    `json:"max_iterations,omitempty"`
	ReviewerModel string `json:"reviewer_model,omitempty"`
	TaskID        string `json:"task_id,omitempty"`
	MCPConfigPath string `json:"mcp_config_path,omitempty"`
}

// LoopResponse is the API response for loop operations
type LoopResponse struct {
	ID        string `json:"id"`
	Status    string `json:"status"`
	Iteration int    `json:"iteration"`
	StartedAt string `json:"started_at,omitempty"`
}

// SSEEvent represents a Server-Sent Event
type SSEEvent struct {
	Type      string `json:"type"`
	ID        string `json:"id"`
	Status    string `json:"status"`
	Iteration int    `json:"iteration"`
	Text      string `json:"text,omitempty"`
}

var startCmd = &cobra.Command{
	Use:   "start",
	Short: "Start execution daemon for concurrent task processing",
	Long: `Start the execution daemon that processes tasks concurrently using Claude Code.

This command starts the openexec-execution server and optionally processes
pending tasks from .openexec/stories.json or .openexec/tasks.json.

Examples:
  # Start the execution daemon
  openexec start

  # Start with custom port and workers
  openexec start --port 8081 --workers 4

  # Start with reviewer model for code review
  openexec start --reviewer claude-3-opus-20240229

  # Start as background daemon
  openexec start --daemon`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Load project configuration
		config, err := project.LoadProjectConfig(".")
		if err != nil {
			return fmt.Errorf("project not initialized: run 'openexec init' first")
		}

		// Apply config defaults (if not overridden by flags)
		if !cmd.Flags().Changed("port") && config.Execution.Port > 0 {
			startPort = config.Execution.Port
		}
		if !cmd.Flags().Changed("reviewer") && config.Execution.ReviewEnabled {
			startReviewer = config.Execution.ReviewerModel
		}

		// Find execution binary
		execBin := findExecutionBinary()
		if execBin == "" {
			return fmt.Errorf("execution engine not found\n\nInstall with:\n  cd ../openexec-execution && go build -o bin/openexec-execution ./cmd/server\n\nOr add openexec-execution to your PATH")
		}

		fmt.Printf("🚀 Starting OpenExec Execution Engine\n")
		fmt.Printf("   Project: %s\n", config.Name)
		fmt.Printf("   Port: %d\n", startPort)
		if startReviewer != "" {
			fmt.Printf("   Reviewer: %s (enabled)\n", startReviewer)
		} else {
			fmt.Printf("   Reviewer: disabled\n")
		}
		fmt.Println()

		// Prepare execution command
		dataDir := filepath.Join(config.ProjectDir, ".openexec", "data")
		auditDB := filepath.Join(dataDir, "audit.db")
		execArgs := []string{
			"--port", fmt.Sprintf("%d", startPort),
			"--data-dir", dataDir,
			"--audit-db", auditDB,
		}

		// Start execution server
		// #nosec G204 - execBin is from findExecutionBinary which returns known paths
		execCmd := exec.Command(execBin, execArgs...)
		execCmd.Dir = config.ProjectDir
		execCmd.Env = os.Environ()

		if startDaemon {
			// Run as daemon - detach and return
			execCmd.Stdout = nil
			execCmd.Stderr = nil
			if err := execCmd.Start(); err != nil {
				return fmt.Errorf("failed to start execution daemon: %w", err)
			}
			fmt.Printf("✓ Execution daemon started (PID: %d)\n", execCmd.Process.Pid)
			fmt.Printf("  API: http://localhost:%d\n", startPort)
			fmt.Printf("  Health: http://localhost:%d/health\n", startPort)
			fmt.Println()
			fmt.Println("To process tasks:")
			fmt.Println("  openexec run")
			return nil
		}

		// Run in foreground with output
		execCmd.Stdout = os.Stdout
		execCmd.Stderr = os.Stderr

		// Start the server
		if err := execCmd.Start(); err != nil {
			return fmt.Errorf("failed to start execution engine: %w", err)
		}

		pid := execCmd.Process.Pid
		fmt.Printf("✓ Execution engine started (PID: %d)\n", pid)
		fmt.Printf("  API: http://localhost:%d\n", startPort)
		fmt.Printf("  Health: http://localhost:%d/health\n", startPort)
		fmt.Println()

		// Wait for server to be ready
		if err := waitForServer(startPort, 10*time.Second); err != nil {
			execCmd.Process.Kill()
			return fmt.Errorf("execution engine failed to start: %w", err)
		}

		fmt.Println("✓ Execution engine is ready")
		fmt.Println()
		fmt.Println("Press Ctrl+C to stop")
		fmt.Println()

		// Wait for the process
		return execCmd.Wait()
	},
}

var runCmd = &cobra.Command{
	Use:   "run [task-id]",
	Short: "Execute tasks using the execution engine",
	Long: `Execute pending tasks from .openexec/stories.json or .openexec/tasks.json.

Without arguments, executes all pending tasks. With a task ID, executes only that task.

The execution engine must be running (openexec start) or will be started automatically.

Reviewer settings are loaded from project config (.openexec/config.json) but can be
overridden with --reviewer or --no-review flags.

Examples:
  # Execute all pending tasks (uses config settings)
  openexec run

  # Execute a specific task
  openexec run T-US-001-001

  # Override to disable review
  openexec run --no-review

  # Override reviewer model
  openexec run --reviewer claude-3-opus-20240229`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Load project configuration
		config, err := project.LoadProjectConfig(".")
		if err != nil {
			return fmt.Errorf("project not initialized: run 'openexec init' first")
		}

		// Apply config defaults (if not overridden by flags)
		if !cmd.Flags().Changed("executor") && config.Execution.ExecutorModel != "" {
			startExecutor = config.Execution.ExecutorModel
		}
		if !cmd.Flags().Changed("reviewer") && !cmd.Flags().Changed("no-review") {
			if config.Execution.ReviewEnabled {
				startReviewer = config.Execution.ReviewerModel
			}
		}
		if runNoReview {
			startReviewer = ""
		}
		if !cmd.Flags().Changed("port") && config.Execution.Port > 0 {
			startPort = config.Execution.Port
		}

		// Use parallel settings from config if not explicitly set
		parallelEnabled := config.Execution.ParallelEnabled
		if !cmd.Flags().Changed("workers") && config.Execution.WorkerCount > 0 {
			startWorkers = config.Execution.WorkerCount
		}
		if !parallelEnabled && !cmd.Flags().Changed("workers") {
			startWorkers = 1 // Sequential if parallel disabled and no workers flag
		}

		// Check if server is running
		if !isServerRunning(startPort) {
			return fmt.Errorf("execution engine not running\n\nStart it with:\n  openexec start --daemon")
		}

		// Load tasks
		tasks, err := loadPendingTasks(config.ProjectDir)
		if err != nil {
			return fmt.Errorf("failed to load tasks: %w", err)
		}

		if len(tasks) == 0 {
			fmt.Println("No pending tasks found.")
			fmt.Println()
			fmt.Println("Generate tasks from intent:")
			fmt.Println("  openexec plan INTENT.md")
			fmt.Println("  openexec story import")
			return nil
		}

		// Filter by task ID if specified
		if len(args) > 0 {
			taskID := args[0]
			var filtered []Task
			for _, t := range tasks {
				if t.ID == taskID {
					filtered = append(filtered, t)
					break
				}
			}
			if len(filtered) == 0 {
				return fmt.Errorf("task %s not found", taskID)
			}
			tasks = filtered
		}

		fmt.Printf("📋 Executing %d task(s)\n", len(tasks))
		if startExecutor != "" {
			fmt.Printf("   Executor: %s\n", startExecutor)
		}
		if startReviewer != "" {
			fmt.Printf("   Reviewer: %s\n", startReviewer)
		}
		fmt.Printf("   Workers:  %d\n", startWorkers)
		fmt.Println()

		// Load release manager for status updates
		mgr, err := getReleaseManager(cmd)
		if err != nil {
			return err
		}

		// Execute tasks in parallel using DAG scheduler
		err = executeTasksParallel(config.ProjectDir, tasks, startWorkers, mgr)
		if err != nil {
			return err
		}

		fmt.Println("✓ Execution complete")
		return nil
	},
}

type TaskStatus string

const (
	StatusPending   TaskStatus = "pending"
	StatusReady     TaskStatus = "ready"
	StatusRunning   TaskStatus = "running"
	StatusCompleted TaskStatus = "completed"
	StatusFailed    TaskStatus = "failed"
)

type TaskNode struct {
	Task      Task
	Status    TaskStatus
	DependsOn map[string]bool
}

func executeTasksParallel(projectDir string, tasks []Task, workerCount int, mgr *release.Manager) error {
	if workerCount <= 0 {
		workerCount = 4
	}

	// 1. Build the graph
	nodes := make(map[string]*TaskNode)
	for _, t := range tasks {
		node := &TaskNode{
			Task:      t,
			Status:    StatusPending,
			DependsOn: make(map[string]bool),
		}
		for _, dep := range t.DependsOn {
			node.DependsOn[dep] = true
		}
		nodes[t.ID] = node
	}

	var mu sync.Mutex
	var wg sync.WaitGroup
	readyTasks := make(chan *TaskNode, len(tasks))
	errors := make(chan error, len(tasks))
	doneCount := 0
	totalCount := len(tasks)

	// Helper to check and enqueue ready tasks
	checkReady := func() {
		mu.Lock()
		defer mu.Unlock()

		for _, node := range nodes {
			if node.Status != StatusPending {
				continue
			}

			// Check if all dependencies are completed
			allDone := true
			for depID := range node.DependsOn {
				depNode, exists := nodes[depID]
				if !exists {
					// Dependency not in current set (maybe already completed in previous run)
					continue
				}
				if depNode.Status != StatusCompleted {
					allDone = false
					break
				}
			}

			if allDone {
				node.Status = StatusReady
				readyTasks <- node
			}
		}
	}

	// Initial check
	go checkReady()

	// Start workers
	for i := 0; i < workerCount; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for node := range readyTasks {
				// Check if we should abort due to other worker errors
				mu.Lock()
				if len(errors) > 0 {
					mu.Unlock()
					return
				}
				node.Status = StatusRunning
				mu.Unlock()

				fmt.Printf("[Worker %d] Executing %s: %s\n", workerID, node.Task.ID, node.Task.Title)

				// Create execution loop
				loopID, err := createExecutionLoop(projectDir, node.Task, mgr)
				if err != nil {
					fmt.Printf("[Worker %d] ❌ Failed to create loop for %s: %v\n", workerID, node.Task.ID, err)
					mu.Lock()
					node.Status = StatusFailed
					mu.Unlock()
					errors <- fmt.Errorf("task %s failed to start: %w", node.Task.ID, err)
					continue
				}

				fmt.Printf("[Worker %d]    Loop: %s\n", workerID, loopID)

				// Wait for loop to complete
				err = waitForLoop(loopID)
				
				mu.Lock()
				if err != nil {
					fmt.Printf("[Worker %d] ⚠ Error in %s: %v\n", workerID, node.Task.ID, err)
					node.Status = StatusFailed
					mu.Unlock()
					errors <- fmt.Errorf("task %s failed: %w", node.Task.ID, err)
					continue
				}

				// Update status in release manager (SQLite)
				if mgr != nil {
					_, updateErr := mgr.CompleteTask(node.Task.ID)
					if updateErr != nil {
						fmt.Printf("[Worker %d] ⚠ Warning: failed to update status for %s: %v\n", workerID, node.Task.ID, updateErr)
					}
				}

				node.Status = StatusCompleted
				doneCount++
				fmt.Printf("[Worker %d] ✓ Completed %s (%d/%d)\n", workerID, node.Task.ID, doneCount, totalCount)
				mu.Unlock()

				// Re-check for new ready tasks
				checkReady()

				// If all tasks are done, close the channel to stop workers
				mu.Lock()
				if doneCount == totalCount {
					close(readyTasks)
				}
				mu.Unlock()
			}
		}(i)
	}

	// Wait for workers to finish
	wg.Wait()

	// Check for errors
	select {
	case <-errors:
		// We have errors. Collect them all.
		// Note: we can't iterate over channel if it's not closed, 
		// but since waitgroup is done, we know no more errors will be sent.
		count := len(errors) + 1 // +1 for the one we just read
		return fmt.Errorf("%d task(s) failed during parallel execution", count)
	default:
		return nil
	}
}

var stopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop the execution daemon",
	Long: `Stop the running execution daemon.

Examples:
  openexec stop`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Check if server is running
		if !isServerRunning(startPort) {
			fmt.Println("Execution daemon is not running.")
			return nil
		}

		// Find and kill the process
		fmt.Printf("Stopping execution daemon on port %d...\n", startPort)

		// Use pkill to find and kill the process
		killCmd := exec.Command("pkill", "-f", "openexec-execution")
		if err := killCmd.Run(); err != nil {
			// Try alternative method
			killCmd = exec.Command("pkill", "openexec-execution")
			_ = killCmd.Run()
		}

		// Wait a moment and verify
		time.Sleep(500 * time.Millisecond)

		if isServerRunning(startPort) {
			return fmt.Errorf("failed to stop daemon, it may still be running")
		}

		fmt.Println("✓ Execution daemon stopped")
		return nil
	},
}

func init() {
	// start command flags
	startCmd.Flags().IntVarP(&startPort, "port", "P", 8080, "HTTP server port")
	startCmd.Flags().IntVarP(&startWorkers, "workers", "w", 4, "Number of concurrent workers")
	startCmd.Flags().IntVarP(&startTimeout, "timeout", "t", 600, "Task timeout in seconds")
	startCmd.Flags().StringVar(&startReviewer, "reviewer", "", "Reviewer model for code review (e.g., claude-3-opus-20240229)")
	startCmd.Flags().BoolVarP(&startDaemon, "daemon", "d", false, "Run as background daemon")

	// run command flags
	runCmd.Flags().IntVar(&startPort, "port", 8080, "Execution engine port")
	runCmd.Flags().IntVar(&startTimeout, "max-iterations", 10, "Maximum iterations per task")
	runCmd.Flags().IntVar(&startTimeout, "timeout", 600, "Task timeout in seconds")
	runCmd.Flags().StringVar(&startExecutor, "executor", "", "Executor model for task execution (overrides config)")
	runCmd.Flags().StringVar(&startReviewer, "reviewer", "", "Reviewer model for code review (overrides config)")
	runCmd.Flags().BoolVar(&runNoReview, "no-review", false, "Disable code review (overrides config)")

	// stop command flags
	stopCmd.Flags().IntVar(&startPort, "port", 8080, "Execution engine port")

	rootCmd.AddCommand(startCmd)
	rootCmd.AddCommand(runCmd)
	rootCmd.AddCommand(stopCmd)
}

// findExecutionBinary locates the openexec-execution binary
func findExecutionBinary() string {
	// Check common locations
	locations := []string{
		// In PATH
		"openexec-execution",
		// Relative to CLI
		"../openexec-execution/bin/openexec-execution",
		// Development locations
		filepath.Join(os.Getenv("HOME"), "go/bin/openexec-execution"),
	}

	// First try PATH
	if path, err := exec.LookPath("openexec-execution"); err == nil {
		return path
	}

	// Try relative paths
	for _, loc := range locations[1:] {
		if _, err := os.Stat(loc); err == nil {
			abs, _ := filepath.Abs(loc)
			return abs
		}
	}

	return ""
}

// waitForServer waits for the server to be ready
func waitForServer(port int, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	client := &http.Client{Timeout: 2 * time.Second}

	for time.Now().Before(deadline) {
		resp, err := client.Get(fmt.Sprintf("http://localhost:%d/health", port))
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return nil
			}
		}
		time.Sleep(500 * time.Millisecond)
	}

	return fmt.Errorf("server did not become ready within %v", timeout)
}

// isServerRunning checks if the execution server is running
func isServerRunning(port int) bool {
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(fmt.Sprintf("http://localhost:%d/health", port))
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

// loadPendingTasks loads tasks from stories.json or tasks.json
func loadPendingTasks(projectDir string) ([]Task, error) {
	var tasks []Task

	// Try tasks.json first
	tasksFile := filepath.Join(projectDir, ".openexec", "tasks.json")
	if data, err := os.ReadFile(tasksFile); err == nil {
		var tf TasksFile
		if err := json.Unmarshal(data, &tf); err == nil {
			for _, t := range tf.Tasks {
				if t.Status == "pending" || t.Status == "" {
					tasks = append(tasks, t)
				}
			}
			return tasks, nil
		}
	}

	// Fall back to stories.json
	storiesFile := filepath.Join(projectDir, ".openexec", "stories.json")
	data, err := os.ReadFile(storiesFile)
	if err != nil {
		return nil, fmt.Errorf("no tasks found: %w", err)
	}

	var sf struct {
		Stories []struct {
			ID        string          `json:"id"`
			Title     string          `json:"title"`
			Status    string          `json:"status"`
			DependsOn []string        `json:"depends_on"`
			Tasks     []GeneratedTask `json:"tasks"`
		} `json:"stories"`
	}

	// Reuse GeneratedTask from release.go if possible, or define here
	type GeneratedTask struct {
		ID          string   `json:"id"`
		Title       string   `json:"title"`
		Description string   `json:"description"`
		DependsOn   []string `json:"depends_on"`
	}

	if err := json.Unmarshal(data, &sf); err != nil {
		return nil, fmt.Errorf("invalid stories.json: %w", err)
	}

	// Sort stories by ID first
	sort.Slice(sf.Stories, func(i, j int) bool {
		return sf.Stories[i].ID < sf.Stories[j].ID
	})

	// Map story ID to its task IDs for barrier injection
	storyTaskIDs := make(map[string][]string)
	for _, story := range sf.Stories {
		for _, task := range story.Tasks {
			if task.ID != "" {
				storyTaskIDs[story.ID] = append(storyTaskIDs[story.ID], task.ID)
			}
		}
	}

	// Extract tasks from stories
	for _, story := range sf.Stories {
		if story.Status == "pending" || story.Status == "" {
			var prevTaskInStory string
			
			for _, genTask := range story.Tasks {
				taskID := genTask.ID
				if taskID == "" {
					continue
				}

				// Start with explicit task-level dependencies
				deps := genTask.DependsOn
				if deps == nil {
					deps = []string{}
				}
				
				// 1. Intra-story sequence: Each task depends on the previous task in the SAME story
				// This ensures story progress is linear by default.
				if prevTaskInStory != "" {
					deps = append(deps, prevTaskInStory)
				}
				
				// 2. Inter-story barriers: This task depends on ALL tasks of prerequisite stories.
				// This ensures architectural layers are respected.
				for _, depStoryID := range story.DependsOn {
					if prerequisiteTasks, ok := storyTaskIDs[depStoryID]; ok {
						deps = append(deps, prerequisiteTasks...)
					}
				}
				
				tasks = append(tasks, Task{
					ID:          taskID,
					Title:       genTask.Title,
					Description: genTask.Description,
					StoryID:     story.ID,
					Status:      "pending",
					DependsOn:   deps,
				})
				
				prevTaskInStory = taskID
			}
		}
	}

	return tasks, nil
}

// createExecutionLoop creates a new execution loop for a task
func createExecutionLoop(projectDir string, task Task, mgr *release.Manager) (string, error) {
	// Build prompt for the task
	prompt := buildTaskPrompt(task, mgr)

	// Ensure MCP config is present
	mcpPath, err := ensureMCPConfig(projectDir)
	if err != nil {
		fmt.Printf("   ⚠ Warning: could not setup MCP tools: %v\n", err)
	}

	req := CreateLoopRequest{
		Prompt:        prompt,
		WorkDir:       projectDir,
		MaxIterations: 10,
		TaskID:        task.ID,
		MCPConfigPath: mcpPath,
	}

	// Add reviewer model if configured
	if startReviewer != "" {
		req.ReviewerModel = startReviewer
	}

	body, err := json.Marshal(req)
	if err != nil {
		return "", err
	}

	resp, err := http.Post(
		fmt.Sprintf("http://localhost:%d/api/v1/loops", startPort),
		"application/json",
		bytes.NewReader(body),
	)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		respBody, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("server returned %d: %s", resp.StatusCode, string(respBody))
	}

	var loopResp LoopResponse
	if err := json.NewDecoder(resp.Body).Decode(&loopResp); err != nil {
		return "", err
	}

	return loopResp.ID, nil
}

// buildTaskPrompt builds a Claude Code prompt for a task
func buildTaskPrompt(task Task, mgr *release.Manager) string {
	var sb strings.Builder

	// Try to get rich story context
	var story *release.Story
	if mgr != nil {
		story = mgr.GetStory(task.StoryID)
	}

	sb.WriteString("You are executing a development task within the OpenExec orchestration engine.\n\n")
	sb.WriteString(fmt.Sprintf("TASK ID: %s\n", task.ID))
	sb.WriteString(fmt.Sprintf("TITLE:   %s\n", task.Title))
	
	if task.Description != "" {
		sb.WriteString(fmt.Sprintf("DESCRIPTION: %s\n", task.Description))
	}

	if story != nil {
		sb.WriteString("\nSTORY CONTEXT:\n")
		sb.WriteString(fmt.Sprintf("ID:    %s\n", story.ID))
		sb.WriteString(fmt.Sprintf("Title: %s\n", story.Title))
		if story.Description != "" {
			sb.WriteString(fmt.Sprintf("Scope: %s\n", story.Description))
		}
		
		if len(story.AcceptanceCriteria) > 0 {
			sb.WriteString("\nACCEPTANCE CRITERIA (Definition of Done):\n")
			for _, ac := range story.AcceptanceCriteria {
				sb.WriteString(fmt.Sprintf("- %s\n", ac))
			}
		}
	}

	sb.WriteString("\nINSTRUCTIONS:\n")
	sb.WriteString("1. Analyze the task requirements and story context.\n")
	sb.WriteString("2. Implement the necessary changes idiomatic to the project.\n")
	sb.WriteString("3. Verify your implementation works using existing tests or by creating new ones.\n")
	sb.WriteString("4. When complete and verified, signal completion using the axon_signal tool with type 'phase-complete'.\n")
	sb.WriteString("\n")
	sb.WriteString("Work autonomously and make reasonable decisions. Do not ask for clarification.\n")

	return sb.String()
}

// waitForLoop polls the loop status until completion
func waitForLoop(loopID string) error {
	client := &http.Client{Timeout: 5 * time.Second}
	lastIteration := 0

	for {
		resp, err := client.Get(fmt.Sprintf("http://localhost:%d/api/v1/loops/%s", startPort, loopID))
		if err != nil {
			return fmt.Errorf("failed to check loop status: %w", err)
		}

		var loop LoopResponse
		if err := json.NewDecoder(resp.Body).Decode(&loop); err != nil {
			resp.Body.Close()
			return fmt.Errorf("failed to decode loop status: %w", err)
		}
		resp.Body.Close()

		// Show iteration progress
		if loop.Iteration > lastIteration {
			fmt.Printf("   → Iteration %d\n", loop.Iteration)
			lastIteration = loop.Iteration
		}

		// Check terminal states
		switch loop.Status {
		case "complete":
			fmt.Printf("   ✓ Complete (iteration %d)\n", loop.Iteration)
			return nil
		case "error":
			fmt.Printf("   ❌ Error\n")
			return fmt.Errorf("loop failed")
		case "max_iterations":
			fmt.Printf("   ⚠ Max iterations reached (%d)\n", loop.Iteration)
			return nil
		case "paused":
			fmt.Printf("   ⏸ Paused\n")
			return nil
		}

		// Still running, wait and poll again
		time.Sleep(2 * time.Second)
	}
}

// KillDaemon kills the execution daemon by process ID
func KillDaemon(pid int) error {
	process, err := os.FindProcess(pid)
	if err != nil {
		return fmt.Errorf("process not found: %w", err)
	}

	// Send SIGTERM signal
	if err := process.Signal(syscall.SIGTERM); err != nil {
		return fmt.Errorf("failed to kill daemon: %w", err)
	}

	return nil
}

// ensureMCPConfig generates an mcp.json file that points to the openexec binary
func ensureMCPConfig(projectDir string) (string, error) {
	// Find the path to the current executable
	execPath, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("failed to find current executable: %w", err)
	}

	mcpDir := filepath.Join(projectDir, ".openexec")
	if err := os.MkdirAll(mcpDir, 0750); err != nil {
		return "", err
	}

	mcpPath := filepath.Join(mcpDir, "mcp.json")

	config := map[string]interface{}{
		"mcpServers": map[string]interface{}{
			"axon-signal": map[string]interface{}{
				"command": execPath,
				"args":    []string{"mcp-serve"},
			},
		},
	}

	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return "", err
	}

	if err := os.WriteFile(mcpPath, data, 0600); err != nil {
		return "", err
	}

	return mcpPath, nil
}
