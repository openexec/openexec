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
	"github.com/openexec/openexec/internal/server"
	"github.com/spf13/cobra"
)

var (
	startPort       int
	startWorkers    int
	startTimeout    int
	startExecutor   string
	startReviewer   string
	startDaemon     bool
	startUI         bool
	executionBinary string
	runNoReview     bool
)

// Task represents a task to execute
type Task struct {
	ID                 string   `json:"id"`
	Title              string   `json:"title"`
	Description        string   `json:"description"`
	StoryID            string   `json:"story_id,omitempty"`
	Status             string   `json:"status"`
	DependsOn          []string `json:"depends_on,omitempty"`
	VerificationScript string   `json:"verification_script,omitempty"`
	TechnicalStrategy  string   `json:"technical_strategy,omitempty"`
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

		// Prepare execution arguments for the integrated server
		dataDir := filepath.Join(config.ProjectDir, ".openexec", "data")
		auditDB := filepath.Join(dataDir, "audit.db")
		
		// Map our cobra flags to server flags
		os.Args = []string{
			"openexec-server",
			"--port", fmt.Sprintf("%d", startPort),
			"--data-dir", dataDir,
			"--audit-db", auditDB,
		}
		if startReviewer != "" {
			os.Args = append(os.Args, "--reviewer", startReviewer)
		}
		if startDaemon {
			os.Args = append(os.Args, "--daemon")
		}

		// Handle UI launch if requested
		if startUI {
			go func() {
				// Wait for server to be ready
				_ = waitForServer(startPort, 15*time.Second)
				uiURL := fmt.Sprintf("http://localhost:%d", startPort)
				cmd.Printf("🌐 Opening web console at %s\n", uiURL)

				var openCmd string
				var openArgs []string

				// Note: Using GOOS from the runtime environment
				switch strings.ToLower(os.Getenv("GOOS")) {
				case "darwin":
					openCmd = "open"
				case "windows":
					openCmd = "cmd"
					openArgs = []string{"/c", "start"}
				default: // linux
					openCmd = "xdg-open"
				}

				if openCmd == "" && os.PathSeparator == '/' {
					// Fallback for macOS if GOOS env is not set
					if _, err := os.Stat("/usr/bin/open"); err == nil {
						openCmd = "open"
					} else {
						openCmd = "xdg-open"
					}
				}

				if openCmd != "" {
					args := append(openArgs, uiURL)
					_ = exec.Command(openCmd, args...).Start()
				}
			}()
		}

		cmd.Printf("🚀 Starting Integrated OpenExec Server\n")
		cmd.Printf("   Project: %s\n", config.Name)
		cmd.Printf("   Port: %d\n", startPort)
		
		// Initialize the new unified server
		srv, err := server.New(server.Config{
			Port:        startPort,
			AuditDB:     auditDB,
			DataDir:     dataDir,
			ProjectsDir: config.ProjectDir,
		})
		if err != nil {
			return err
		}

		return srv.Start(cmd.Context())
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
			cmd.Println("No pending tasks found.")
			cmd.Println()
			cmd.Println("Generate tasks from intent:")
			cmd.Println("  openexec plan INTENT.md")
			cmd.Println("  openexec story import")
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

		pendingCount := 0
		for _, t := range tasks {
			if t.Status != "completed" && t.Status != "done" {
				pendingCount++
			}
		}

		if pendingCount == 0 {
			cmd.Println("No pending tasks found.")
			return nil
		}

		cmd.Printf("📋 Executing %d task(s)\n", pendingCount)
		if startExecutor != "" {
			cmd.Printf("   Executor: %s\n", startExecutor)
		}
		if startReviewer != "" {
			cmd.Printf("   Reviewer: %s\n", startReviewer)
		}
		cmd.Printf("   Workers:  %d\n", startWorkers)
		cmd.Println()

		// Load release manager for status updates
		mgr, err := getReleaseManager(cmd)
		if err != nil {
			return err
		}

		// Execute tasks in parallel using DAG scheduler
		err = executeTasksParallel(cmd, config.ProjectDir, tasks, startWorkers, mgr)
		if err != nil {
			return err
		}

		cmd.Println("✓ Execution complete")
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
	Retries   int
}

func executeTasksParallel(cmd *cobra.Command, projectDir string, tasks []Task, workerCount int, mgr *release.Manager) error {
	if workerCount <= 0 {
		workerCount = 4
	}

	// 1. Build the graph
	nodes := make(map[string]*TaskNode)
	totalToRun := 0
	
	for _, t := range tasks {
		status := StatusPending
		if t.Status == "completed" || t.Status == "done" {
			status = StatusCompleted
		} else {
			totalToRun++
		}
		
		node := &TaskNode{
			Task:      t,
			Status:    status,
			DependsOn: make(map[string]bool),
		}
		for _, dep := range t.DependsOn {
			node.DependsOn[dep] = true
		}
		nodes[t.ID] = node
	}

	if totalToRun == 0 {
		cmd.Println("No pending tasks to execute.")
		return nil
	}

	var mu sync.Mutex
	var wg sync.WaitGroup
	readyTasks := make(chan *TaskNode, len(tasks))
	errors := make(chan error, len(tasks))
	doneCount := 0
	totalCount := totalToRun

	// Helper to check and enqueue ready tasks
	checkReady := func() {
		mu.Lock()
		defer mu.Unlock()

		for _, node := range nodes {
			// Only consider tasks that are currently pending
			if node.Status != StatusPending {
				continue
			}

			// Check if all dependencies are completed
			allDone := true
			for depID := range node.DependsOn {
				depNode, exists := nodes[depID]
				if !exists {
					// Dependency not in current set
					continue
				}
				if depNode.Status != StatusCompleted {
					allDone = false
					break
				}
			}

			if allDone {
				// Mark as ready FIRST to prevent double-enqueuing in concurrent checkReady calls
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

				cmd.Printf("[Worker %d] Executing %s: %s\n", workerID, node.Task.ID, node.Task.Title)

				// Start execution loop
				var lastError string
				maxRetries := 3
				success := false

				for node.Retries < maxRetries {
					if node.Retries > 0 {
						cmd.Printf("[Worker %d] 🔄 Self-healing attempt %d/%d for %s\n", workerID, node.Retries, maxRetries, node.Task.ID)
					}

					// Create execution loop (passing last error if this is a retry)
					loopID, err := createExecutionLoopWithRetry(projectDir, node.Task, mgr, lastError)
					if err != nil {
						cmd.Printf("[Worker %d] ❌ Failed to create loop for %s: %v\n", workerID, node.Task.ID, err)
						lastError = err.Error()
						node.Retries++
						continue
					}

					cmd.Printf("[Worker %d]    Loop: %s\n", workerID, loopID)

					// Wait for loop to complete
					err = waitForLoop(cmd, loopID)
					
					if err == nil && node.Task.VerificationScript != "" {
						cmd.Printf("[Worker %d] Running autonomous verification: %s\n", workerID, node.Task.VerificationScript)
						
						// Execute the verification script
						verifyCmd := exec.Command("bash", "-c", node.Task.VerificationScript)
						verifyCmd.Dir = projectDir
						
						output, verifyErr := verifyCmd.CombinedOutput()
						if verifyErr != nil {
							cmd.Printf("[Worker %d] ✗ Verification failed for %s:\n%s\n", workerID, node.Task.ID, string(output))
							err = fmt.Errorf("verification script failed: %s\nOutput:\n%s", verifyErr, string(output))
						} else {
							cmd.Printf("[Worker %d] ✓ Verification passed for %s\n", workerID, node.Task.ID)
							success = true
							break
						}
					} else if err == nil {
						// No verification script, but loop finished naturally
						success = true
						break
					}

					// If we're here, either loop failed or verification failed
					if err != nil {
						lastError = err.Error()
					} else {
						lastError = "unknown execution failure"
					}
					node.Retries++
				}

				mu.Lock()
				if !success {
					cmd.Printf("[Worker %d] ⚠ Task %s permanently failed after %d attempts\n", workerID, node.Task.ID, node.Retries)
					node.Status = StatusFailed
					mu.Unlock()
					errors <- fmt.Errorf("task %s failed permanently: %s", node.Task.ID, lastError)
					continue
				}

				// Update status in release manager (SQLite)
				if mgr != nil {
					_, updateErr := mgr.CompleteTask(node.Task.ID)
					if updateErr != nil {
						cmd.Printf("[Worker %d] ⚠ Warning: failed to update status for %s: %v\n", workerID, node.Task.ID, updateErr)
					}
				}

				node.Status = StatusCompleted
				
				// Persist completion back to tasks.json (already done, but let's be safe)
				_ = saveTaskStatus(projectDir, node.Task.ID, "completed")
				
				// ADAPTIVE DISCOVERY: Re-scan for new tasks
				doneCount++
				cmd.Printf("[Worker %d] ✓ Completed %s (%d/%d)\n", workerID, node.Task.ID, doneCount, totalCount)
				
				newTasks, err := loadPendingTasks(projectDir)
				if err == nil {
					for _, nt := range newTasks {
						if _, exists := nodes[nt.ID]; !exists {
							cmd.Printf("[Worker %d] ✨ Discovered new task: %s\n", workerID, nt.ID)
							newNode := &TaskNode{
								Task:      nt,
								Status:    StatusPending,
								DependsOn: make(map[string]bool),
							}
							if nt.Status == "completed" || nt.Status == "done" {
								newNode.Status = StatusCompleted
							}
							for _, dep := range nt.DependsOn {
								newNode.DependsOn[dep] = true
							}
							nodes[nt.ID] = newNode
							totalCount++
						}
					}
				}
				
				// If all tasks are done, signal completion
				finished := (doneCount == totalCount)
				mu.Unlock()

				if finished {
					// Don't close yet, let WaitGroup handle termination if workers are still active
					// Actually, with range readyTasks, we MUST close it to stop workers.
					// We'll close it once after all tasks are finished.
				}

				// Re-check for new ready tasks (OUTSIDE of lock)
				checkReady()
			}
		}(i)
	}

	var once sync.Once
	// Monitor for completion to close the channel
	go func() {
		for {
			mu.Lock()
			finished := (doneCount == totalCount && totalCount > 0)
			mu.Unlock()
			
			if finished {
				once.Do(func() {
					close(readyTasks)
				})
				return
			}
			time.Sleep(100 * time.Millisecond)
		}
	}()

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

var restartCmd = &cobra.Command{
	Use:   "restart",
	Short: "Restart the execution daemon",
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println("♻️ Restarting OpenExec Execution Engine...")
		
		// Stop if running
		if isServerRunning(startPort) {
			stopArgs := []string{"start", "stop", "--port", fmt.Sprintf("%d", startPort)}
			stopExec := exec.Command(os.Args[0], stopArgs...)
			_ = stopExec.Run()
			time.Sleep(1 * time.Second)
		}
		
		// Start again
		startArgs := []string{"start", "--daemon", "--port", fmt.Sprintf("%d", startPort)}
		if startReviewer != "" {
			startArgs = append(startArgs, "--reviewer", startReviewer)
		}
		
		startExec := exec.Command(os.Args[0], startArgs...)
		startExec.Stdout = os.Stdout
		startExec.Stderr = os.Stderr
		return startExec.Run()
	},
}

func init() {
	// start command flags
	startCmd.Flags().IntVarP(&startPort, "port", "P", 8765, "HTTP server port")
	startCmd.Flags().IntVarP(&startWorkers, "workers", "w", 4, "Number of concurrent workers")
	startCmd.Flags().IntVarP(&startTimeout, "timeout", "t", 600, "Task timeout in seconds")
	startCmd.Flags().StringVar(&startReviewer, "reviewer", "", "Reviewer model for code review (e.g., claude-3-opus-20240229)")
	startCmd.Flags().BoolVarP(&startDaemon, "daemon", "d", false, "Run as background daemon")
	startCmd.Flags().BoolVar(&startUI, "ui", false, "Open web console in browser")
	startCmd.Flags().BoolVar(&startUI, "console", false, "Open web console in browser (alias for --ui)")

	// Subcommands for start
	startCmd.AddCommand(stopCmd)
	startCmd.AddCommand(restartCmd)

	// run command flags
	runCmd.Flags().IntVar(&startPort, "port", 8765, "Execution engine port")
	runCmd.Flags().IntVar(&startTimeout, "max-iterations", 10, "Maximum iterations per task")
	runCmd.Flags().IntVar(&startTimeout, "timeout", 600, "Task timeout in seconds")
	runCmd.Flags().StringVar(&startExecutor, "executor", "", "Executor model for task execution (overrides config)")
	runCmd.Flags().StringVar(&startReviewer, "reviewer", "", "Reviewer model for code review (overrides config)")
	runCmd.Flags().BoolVar(&runNoReview, "no-review", false, "Disable code review (overrides config)")

	// stop command flags
	stopCmd.Flags().IntVar(&startPort, "port", 8765, "Execution engine port")

	rootCmd.AddCommand(startCmd)
	rootCmd.AddCommand(runCmd)
	rootCmd.AddCommand(stopCmd)
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

// loadPendingTasks loads all tasks from tasks.json or .openexec/stories.json
func loadPendingTasks(projectDir string) ([]Task, error) {
	var tasks []Task

	// Try root tasks.json first
	rootTasksFile := filepath.Join(projectDir, "tasks.json")
	if data, err := os.ReadFile(rootTasksFile); err == nil {
		var tf TasksFile
		if err := json.Unmarshal(data, &tf); err == nil {
			// LOAD ALL TASKS to preserve DAG integrity
			return tf.Tasks, nil
		}
	}

	// Try .openexec/tasks.json next
	tasksFile := filepath.Join(projectDir, ".openexec", "tasks.json")
	if data, err := os.ReadFile(tasksFile); err == nil {
		var tf TasksFile
		if err := json.Unmarshal(data, &tf); err == nil {
			// LOAD ALL TASKS to preserve DAG integrity
			return tf.Tasks, nil
		}
	}

	// Fall back to stories.json
	storiesFile := filepath.Join(projectDir, ".openexec", "stories.json")
	data, err := os.ReadFile(storiesFile)
	if err != nil {
		return nil, fmt.Errorf("no tasks found: %w", err)
	}

	// Reuse GeneratedTask from release.go if possible, or define here
	type GeneratedTask struct {
		ID                 string   `json:"id"`
		Title              string   `json:"title"`
		Description        string   `json:"description"`
		DependsOn          []string `json:"depends_on"`
		VerificationScript string   `json:"verification_script,omitempty"`
		TechnicalStrategy  string   `json:"technical_strategy,omitempty"`
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

	if err := json.Unmarshal(data, &sf); err != nil {
		return nil, fmt.Errorf("invalid stories.json: %w", err)
	}

	// Sort stories by ID first
	sort.Slice(sf.Stories, func(i, j int) bool {
		return sf.Stories[i].ID < sf.Stories[j].ID
	})

	// Map story ID to its task IDs for barrier injection
	storyTaskIDs := make(map[string][]string)
	storyContractTaskIDs := make(map[string][]string)
	
	for _, story := range sf.Stories {
		for _, task := range story.Tasks {
			if task.ID != "" {
				storyTaskIDs[story.ID] = append(storyTaskIDs[story.ID], task.ID)
				
				// Identify Contract/Schema tasks for Interface-First Parallelism
				titleLower := strings.ToLower(task.Title)
				if strings.Contains(titleLower, "contract") || strings.Contains(titleLower, "schema") {
					storyContractTaskIDs[story.ID] = append(storyContractTaskIDs[story.ID], task.ID)
				}
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
				
				// 2. Interface-First / Story Barrier injection:
				for _, depStoryID := range story.DependsOn {
					contracts, hasContracts := storyContractTaskIDs[depStoryID]; 
					if hasContracts && len(contracts) > 0 {
						// INTERFACE-FIRST: This story only waits for the CONTRACT tasks of the prerequisite story.
						deps = append(deps, contracts...)
					} else if prerequisiteTasks, ok := storyTaskIDs[depStoryID]; ok {
						// FALLBACK: Wait for all tasks (Story Barrier) if no explicit contract task identified.
						deps = append(deps, prerequisiteTasks...)
					}
				}
				
				tasks = append(tasks, Task{
					ID:                 taskID,
					Title:              genTask.Title,
					Description:        genTask.Description,
					StoryID:            story.ID,
					Status:             "pending",
					DependsOn:          deps,
					VerificationScript: genTask.VerificationScript,
					TechnicalStrategy:  genTask.TechnicalStrategy,
				})
				
				prevTaskInStory = taskID
			}
		}
	}

	return tasks, nil
}

// createExecutionLoopWithRetry creates a new execution loop for a task with optional retry context
func createExecutionLoopWithRetry(projectDir string, task Task, mgr *release.Manager, lastError string) (string, error) {
	// Build prompt for the task
	prompt := buildTaskPromptWithRetry(task, mgr, lastError)

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

// buildTaskPromptWithRetry builds a Claude Code prompt for a task, including failure context if it's a retry
func buildTaskPromptWithRetry(task Task, mgr *release.Manager, lastError string) string {
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

	if task.TechnicalStrategy != "" {
		sb.WriteString(fmt.Sprintf("TECHNICAL STRATEGY: %s\n", task.TechnicalStrategy))
	}

	if lastError != "" {
		sb.WriteString("\n⚠️ SELF-HEALING CONTEXT:\n")
		sb.WriteString("A previous attempt to complete this task failed. Use the error information below to fix the implementation.\n")
		sb.WriteString(fmt.Sprintf("PREVIOUS ERROR/FAILURE:\n%s\n", lastError))
	}

	if story != nil {
		sb.WriteString("\nSTORY CONTEXT:\n")
		sb.WriteString(fmt.Sprintf("ID:    %s\n", story.ID))
		if story.GoalID != "" {
			sb.WriteString(fmt.Sprintf("Goal:  %s\n", story.GoalID))
		}
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

		if story.Contract != "" {
			sb.WriteString("\nINTERFACE CONTRACT:\n")
			sb.WriteString(fmt.Sprintf("%s\n", story.Contract))
		}
	}

	sb.WriteString("\nINSTRUCTIONS:\n")
	sb.WriteString("1. Analyze the task requirements and story context.\n")
	sb.WriteString("2. Implement the necessary changes idiomatic to the project.\n")
	sb.WriteString("3. Verify your implementation works using existing tests or by creating new ones.\n")
	
	if task.VerificationScript != "" {
		sb.WriteString(fmt.Sprintf("   -> MANDATORY VERIFICATION SCRIPT: Run '%s' to prove the task is complete.\n", task.VerificationScript))
	}

	sb.WriteString("4. When complete and verified, signal completion using the openexec_signal tool with type 'phase-complete'.\n")
	sb.WriteString("\n")
	sb.WriteString("Work autonomously and make reasonable decisions. Do not ask for clarification.\n")

	return sb.String()
}

// waitForLoop polls the loop status until completion
func waitForLoop(cmd *cobra.Command, loopID string) error {
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
			cmd.Printf("   → Iteration %d\n", loop.Iteration)
			lastIteration = loop.Iteration
		}

		// Check terminal states
		switch loop.Status {
		case "complete":
			cmd.Printf("   ✓ Complete (iteration %d)\n", loop.Iteration)
			return nil
		case "error":
			cmd.Printf("   ❌ Error\n")
			return fmt.Errorf("loop failed")
		case "max_iterations":
			cmd.Printf("   ⚠ Max iterations reached (%d)\n", loop.Iteration)
			return nil
		case "paused":
			cmd.Printf("   ⏸ Paused\n")
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

// saveTaskStatus persists task status to .openexec/tasks.json and root tasks.json
func saveTaskStatus(projectDir string, taskID string, status string) error {
	paths := []string{
		filepath.Join(projectDir, "tasks.json"),
		filepath.Join(projectDir, ".openexec", "tasks.json"),
	}

	var firstData []byte
	for _, p := range paths {
		if data, err := os.ReadFile(p); err == nil {
			firstData = data
			break
		}
	}

	if firstData == nil {
		return fmt.Errorf("could not find tasks.json in root or .openexec")
	}

	// We need a generic map to preserve all fields (phases, etc.)
	var data map[string]interface{}
	if err := json.Unmarshal(firstData, &data); err != nil {
		return err
	}

	updated := false

	// Update in flat 'tasks' list if exists
	if tasks, ok := data["tasks"].([]interface{}); ok {
		for _, t := range tasks {
			if taskMap, ok := t.(map[string]interface{}); ok {
				if id, ok := taskMap["id"].(string); ok && id == taskID {
					taskMap["status"] = status
					updated = true
				}
			}
		}
	}

	// Update in 'phases' too if present
	if phases, ok := data["phases"].([]interface{}); ok {
		for _, phase := range phases {
			if phaseMap, ok := phase.(map[string]interface{}); ok {
				if tasks, ok := phaseMap["tasks"].([]interface{}); ok {
					for _, t := range tasks {
						if taskMap, ok := t.(map[string]interface{}); ok {
							if id, ok := taskMap["id"].(string); ok && id == taskID {
								taskMap["status"] = status
								updated = true
							}
						}
					}
				}
			}
		}
	}

	if !updated {
		return fmt.Errorf("task %s not found in tasks.json", taskID)
	}

	newData, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}

	// Write to all paths that exist
	for _, p := range paths {
		if _, err := os.Stat(p); err == nil {
			_ = os.WriteFile(p, newData, 0644)
		}
	}

	return nil
}
