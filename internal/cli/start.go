package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
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
	startPort        int
	startWorkers     int
	startTimeout     int
	startExecutor    string
	startReviewer    string
	startDaemon      bool
	startUI          bool
	executionBinary  string
	runNoReview      bool
	runMaxIterations int
	runTimeout       int
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
	Error     string `json:"error,omitempty"`
	Phase     string `json:"phase,omitempty"`
	Agent     string `json:"agent,omitempty"`
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
		if !cmd.Flags().Changed("timeout") && config.Execution.TimeoutSeconds > 0 {
			startTimeout = config.Execution.TimeoutSeconds
		}
		if !cmd.Flags().Changed("reviewer") && config.Execution.ReviewEnabled {
			startReviewer = config.Execution.ReviewerModel
		}

		// Prepare execution arguments for the integrated server
		dataDir := filepath.Join(config.ProjectDir, ".openexec", "data")
		auditDB := filepath.Join(dataDir, "audit.db")

		// Find available port if default is busy
		finalPort, err := findAvailablePort(startPort)
		if err != nil {
			return err
		}
		if finalPort != startPort {
			cmd.Printf("   ⚠ Port %d is busy, using %d instead\n", startPort, finalPort)
			startPort = finalPort
		}

		// Map our cobra flags to server flags
		serverArgs := []string{
			"start",
			"--port", fmt.Sprintf("%d", startPort),
		}
		if startReviewer != "" {
			serverArgs = append(serverArgs, "--reviewer", startReviewer)
		}

		// If daemon mode requested, fork a background process
		if startDaemon {
			// Check if already running
			if isServerRunning(config.ProjectDir, startPort) {
				return fmt.Errorf("execution engine already running on port %d", startPort)
			}

			// Prepare command to run ourselves without the --daemon flag
			execPath, err := os.Executable()
			if err != nil {
				return fmt.Errorf("failed to find executable: %w", err)
			}

			logPath := filepath.Join(config.ProjectDir, ".openexec", "daemon.log")
			logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
			if err != nil {
				return fmt.Errorf("failed to open log file: %w", err)
			}

			cmd := exec.Command(execPath, serverArgs...)
			cmd.Dir = config.ProjectDir
			cmd.Stdout = logFile
			cmd.Stderr = logFile

			if err := cmd.Start(); err != nil {
				return fmt.Errorf("failed to start background process: %w", err)
			}

			fmt.Printf("✓ Execution engine started in background (PID: %d)\n", cmd.Process.Pid)
			fmt.Printf("  Logs: .openexec/daemon.log\n")
			return nil
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

		// Write PID file
		if err := writePIDFile(config.ProjectDir, startPort); err != nil {
			cmd.Printf("   ⚠ Warning: could not write PID file: %v\n", err)
		}
		defer func() {
			_ = removePIDFile(config.ProjectDir)
		}()

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
		if !cmd.Flags().Changed("timeout") && config.Execution.TimeoutSeconds > 0 {
			runTimeout = config.Execution.TimeoutSeconds
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
		if !isServerRunning(config.ProjectDir, startPort) {
			return fmt.Errorf("execution engine not running\n\nStart it with:\n  openexec start --daemon")
		}

		// Load release manager for status updates and as source of truth for tasks
		mgr, err := getReleaseManager(cmd)
		if err != nil {
			return err
		}

		// PRE-FLIGHT ACTIVE HEALING:
		// Ensure any existing loops are stopped and ghost states are reset
		// This makes 'openexec run' safe to call from cron or after a crash.
		cmd.Println("🔍 Running Pre-flight Active Healing...")
		relTasks := mgr.GetTasks()
		resetCount := 0
		for _, rt := range relTasks {
			if rt.Status == "running" || rt.Status == "starting" {
				// Force stop on the server if it happens to be running
				stopURL := fmt.Sprintf("http://localhost:%d/api/fwu/%s/stop", startPort, rt.ID)
				_, _ = http.Post(stopURL, "application/json", nil)
				
				rt.Status = "pending"
				_ = mgr.UpdateTask(rt)
				resetCount++
			}
		}
		if resetCount > 0 {
			cmd.Printf("   ✨ Self-Healed: Reset %d ghost tasks to pending\n", resetCount)
		}

		// Load tasks (now includes implicit reconciliation)
		tasks, err := loadPendingTasks(config.ProjectDir, mgr)
		if err != nil {
			return fmt.Errorf("failed to load tasks: %w", err)
		}

		// Materialize tasks.json so saveTaskStatus can persist completions.
		// Only write if no tasks.json exists yet (i.e., tasks came from stories.json).
		tasksPath := filepath.Join(config.ProjectDir, ".openexec", "tasks.json")
		if _, statErr := os.Stat(tasksPath); statErr != nil && len(tasks) > 0 {
			tf := TasksFile{Tasks: tasks}
			if data, err := json.MarshalIndent(tf, "", "  "); err == nil {
				_ = os.WriteFile(tasksPath, data, 0644)
			}
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
						
						if strings.Contains(lastError, "Planning Mismatch") {
							// If the agent also mentioned implementation is done, AUTO-HEAL!
							lowerErr := strings.ToLower(lastError)
							if strings.Contains(lowerErr, "complete") || strings.Contains(lowerErr, "done") || strings.Contains(lowerErr, "criteria appear to be met") {
								cmd.Printf("[Worker %d] ✨ AUTO-HEAL: Agent verified task is actually complete. Syncing state...\n", workerID)
								_, _ = mgr.CompleteTask(node.Task.ID)
								mu.Lock()
								node.Status = StatusCompleted
								mu.Unlock()
								break 
							}

							cmd.Printf("\n💡 REPAIR TIP: The orchestrator's state is inconsistent with the filesystem.\n")
							cmd.Printf("   1. Verify task status in .openexec/tasks.json and .openexec/stories.json\n")
							cmd.Printf("   2. Once aligned, run 'openexec run' again.\n\n")
							
							node.Retries = maxRetries // Force permanent failure
							mu.Lock()
							node.Status = StatusFailed
							mu.Unlock()
							errors <- fmt.Errorf("task %s failed permanently: %s", node.Task.ID, lastError)
							break
						}
						
						node.Retries++
						continue
					}

					workerPrefix := fmt.Sprintf("[Worker %d]", workerID)
					cmd.Printf("%s    Loop: %s\n", workerPrefix, loopID)

					// Wait for loop to complete
					err = waitForLoop(cmd, loopID, workerPrefix, time.Duration(runTimeout)*time.Second)

					if err == nil && node.Task.VerificationScript != "" {
						cmd.Printf("[Worker %d] Running autonomous verification: %s\n", workerID, node.Task.VerificationScript)

						// Execute the verification script with node_modules/.bin on PATH
						verifyCmd := exec.Command("bash", "-c", node.Task.VerificationScript)
						verifyCmd.Dir = projectDir

						// Add node_modules/.bin to PATH cross-platform
						newPath := fmt.Sprintf("%s%c%s",
							filepath.Join(projectDir, "node_modules", ".bin"),
							filepath.ListSeparator,
							os.Getenv("PATH"),
						)
						verifyCmd.Env = append(os.Environ(), "PATH="+newPath)

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
						
						// Check for non-retriable errors (e.g. Planning Mismatch)
						if strings.Contains(lastError, "Planning Mismatch") {
							// If the agent also mentioned implementation is done, AUTO-HEAL!
							lowerErr := strings.ToLower(lastError)
							if strings.Contains(lowerErr, "complete") || strings.Contains(lowerErr, "done") || strings.Contains(lowerErr, "criteria appear to be met") {
								cmd.Printf("[Worker %d] ✨ AUTO-HEAL: Agent verified task is actually complete. Syncing state...\n", workerID)
								_, _ = mgr.CompleteTask(node.Task.ID)
								mu.Lock()
								node.Status = StatusCompleted
								mu.Unlock()
								break 
							}

							cmd.Printf("[Worker %d] ❌ NON-RETRIABLE ERROR: %s\n", workerID, lastError)
							cmd.Printf("\n💡 REPAIR TIP: The orchestrator's state is inconsistent with the filesystem.\n")
							cmd.Printf("   1. Verify task status in .openexec/tasks.json and .openexec/stories.json\n")
							cmd.Printf("   2. Check if .openexec/stories/%s.md matches the current state\n", node.Task.ID)
							cmd.Printf("   3. Once aligned, run 'openexec run' again.\n\n")
							
							node.Retries = maxRetries // Force permanent failure
							break
						}
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

				newTasks, err := loadPendingTasks(projectDir, mgr)
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
		// Load project configuration
		config, err := project.LoadProjectConfig(".")
		if err != nil {
			return fmt.Errorf("project not initialized: run 'openexec init' first")
		}

		// Apply config port if not overridden
		if !cmd.Flags().Changed("port") && config.Execution.Port > 0 {
			startPort = config.Execution.Port
		}

		// Check if server is running
		if !isServerRunning(config.ProjectDir, startPort) {
			fmt.Println("Execution daemon is not running.")
			return nil
		}

		// Find and kill the process
		fmt.Printf("Stopping execution daemon on port %d...\n", startPort)

		// 1. Try killing by PID file first
		if pid, _, err := readPID(config.ProjectDir); err == nil {
			if err := KillDaemon(pid); err == nil {
				// Wait a moment and verify
				time.Sleep(500 * time.Millisecond)
				if !isServerRunning(config.ProjectDir, startPort) {
					fmt.Println("✓ Execution daemon stopped (via PID)")
					return nil
				}
			}
		}

		// 2. Fallback to pkill
		killCmd := exec.Command("pkill", "-f", "openexec-execution")
		if err := killCmd.Run(); err != nil {
			killCmd = exec.Command("pkill", "openexec-execution")
			_ = killCmd.Run()
		}

		// Wait a moment and verify
		time.Sleep(500 * time.Millisecond)

		if isServerRunning(config.ProjectDir, startPort) {
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
		// Load project configuration
		config, err := project.LoadProjectConfig(".")
		if err != nil {
			return fmt.Errorf("project not initialized: run 'openexec init' first")
		}

		fmt.Println("♻️ Restarting OpenExec Execution Engine...")

		// Stop if running
		if isServerRunning(config.ProjectDir, startPort) {
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
	runCmd.Flags().IntVar(&runMaxIterations, "max-iterations", 10, "Maximum iterations per task")
	runCmd.Flags().IntVar(&runTimeout, "timeout", 1800, "Task timeout in seconds")
	runCmd.Flags().StringVar(&startExecutor, "executor", "", "Executor model for task execution (overrides config)")
	runCmd.Flags().StringVar(&startReviewer, "reviewer", "", "Reviewer model for code review (overrides config)")
	runCmd.Flags().BoolVar(&runNoReview, "no-review", false, "Disable code review (overrides config)")

	// stop command flags
	stopCmd.Flags().IntVar(&startPort, "port", 8765, "Execution engine port")

	rootCmd.AddCommand(startCmd)
	rootCmd.AddCommand(runCmd)
	rootCmd.AddCommand(stopCmd)
}

// findAvailablePort tries to find an available port starting from basePort
func findAvailablePort(basePort int) (int, error) {
	for port := basePort; port < basePort+100; port++ {
		ln, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
		if err == nil {
			_ = ln.Close()
			return port, nil
		}
	}
	return 0, fmt.Errorf("could not find available port in range %d-%d", basePort, basePort+99)
}

// writePIDFile writes the current process ID and port to a file
func writePIDFile(projectDir string, port int) error {
	pid := os.Getpid()
	pidFile := filepath.Join(projectDir, ".openexec", "openexec.pid")
	return os.WriteFile(pidFile, []byte(fmt.Sprintf("%d:%d", pid, port)), 0644)
}

// removePIDFile removes the PID file
func removePIDFile(projectDir string) error {
	pidFile := filepath.Join(projectDir, ".openexec", "openexec.pid")
	return os.Remove(pidFile)
}

// readPID reads the process ID and optional port from the PID file
func readPID(projectDir string) (int, int, error) {
	pidFile := filepath.Join(projectDir, ".openexec", "openexec.pid")
	data, err := os.ReadFile(pidFile)
	if err != nil {
		return 0, 0, err
	}
	var pid, port int
	content := string(data)
	if strings.Contains(content, ":") {
		_, err = fmt.Sscanf(content, "%d:%d", &pid, &port)
	} else {
		_, err = fmt.Sscanf(content, "%d", &pid)
	}
	return pid, port, err
}

// waitForServer waits for the server to be ready
func waitForServer(port int, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	client := &http.Client{Timeout: 2 * time.Second}

	for time.Now().Before(deadline) {
		resp, err := client.Get(fmt.Sprintf("http://localhost:%d/api/health", port))
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

// isServerRunning checks if the execution server is running.
// If port is 0, it tries to discover the port from the PID file.
func isServerRunning(projectDir string, port int) bool {
	checkPort := port

	// 1. Check PID file first to see if we can discover the actual port
	pid, discoveredPort, err := readPID(projectDir)
	if err == nil {
		if checkPort == 0 || checkPort == 8765 || checkPort == 8080 { // only override if default or unspecified
			if discoveredPort > 0 {
				checkPort = discoveredPort
			}
		}

		// Verify process is actually alive
		process, err := os.FindProcess(pid)
		if err == nil {
			if err := process.Signal(syscall.Signal(0)); err != nil {
				return false // process is dead
			}
		}
	}

	// 2. Try HTTP health check (most reliable)
	if checkPort > 0 {
		client := &http.Client{Timeout: 2 * time.Second}
		resp, err := client.Get(fmt.Sprintf("http://localhost:%d/api/health", port))
		if err == nil {
			defer resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return true
			}
		}

		// If explicit port failed, but we discovered a DIFFERENT port, try that too
		if checkPort != port {
			resp, err := client.Get(fmt.Sprintf("http://localhost:%d/api/health", checkPort))
			if err == nil {
				defer resp.Body.Close()
				if resp.StatusCode == http.StatusOK {
					// Update global startPort so CLI uses the discovered one
					startPort = checkPort
					return true
				}
			}
		}
	}

	return false
}

// loadPendingTasks loads all tasks from the release manager (Source of Truth), tasks.json, or stories.json
func loadPendingTasks(projectDir string, mgr *release.Manager) ([]Task, error) {
	// Pre-flight: Load stories.json to use as a "map of intent" for reconciliation
	incomingTaskStories := make(map[string]string)
	storiesFile := filepath.Join(projectDir, ".openexec", "stories.json")
	if data, err := os.ReadFile(storiesFile); err == nil {
		var sf struct {
			Stories []struct {
				ID    string `json:"id"`
				Tasks []any  `json:"tasks"`
			} `json:"stories"`
		}
		if err := json.Unmarshal(data, &sf); err == nil {
			for _, s := range sf.Stories {
				for _, tRaw := range s.Tasks {
					id := ""
					switch v := tRaw.(type) {
					case string: id = v
					case map[string]any: id, _ = v["id"].(string)
					}
					if id != "" {
						incomingTaskStories[id] = s.ID
					}
				}
			}
		}
	}

	// 1. Try Release Manager first (Source of Truth)
	if mgr != nil {
		relTasks := mgr.GetTasks()
		if len(relTasks) > 0 {
			var tasks []Task
			for _, rt := range relTasks {
				// RECONCILIATION: Check for StoryID mismatch or missing
				expectedStoryID, inPlan := incomingTaskStories[rt.ID]
				if inPlan && rt.StoryID != expectedStoryID {
					rt.StoryID = expectedStoryID
					_ = mgr.UpdateTask(rt)
				}

				// STATUS SYNC: Check if markdown file says it's done
				storyPath := filepath.Join(projectDir, ".openexec", "stories", rt.StoryID+".md")
				if data, err := os.ReadFile(storyPath); err == nil {
					content := strings.ToLower(string(data))
					if strings.Contains(content, "status: completed") || strings.Contains(content, "status: done") {
						if rt.Status == "pending" {
							rt.Status = "completed"
							_ = mgr.UpdateTask(rt)
						}
					}
				}

				// MATERIALIZATION: Ensure the agent's work file exists (Self-Healing)
				fwuDir := filepath.Join(projectDir, ".openexec", "fwu")
				_ = os.MkdirAll(fwuDir, 0750)
				taskFile := filepath.Join(fwuDir, rt.ID+".md")
				if _, err := os.Stat(taskFile); os.IsNotExist(err) {
					content := fmt.Sprintf("# Task %s: %s\n\n%s\n\nStatus: pending\n", rt.ID, rt.Title, rt.Description)
					_ = os.WriteFile(taskFile, []byte(content), 0644)
				}

				// Only include tasks that are actually in the current Plan of Record
				// (Non-destructive pruning: we don't delete them from DB, just don't schedule them)
				if !inPlan {
					continue
				}

				tasks = append(tasks, Task{
					ID:                 rt.ID,
					Title:              rt.Title,
					Description:        rt.Description,
					StoryID:            rt.StoryID,
					Status:             rt.Status,
					DependsOn:          rt.DependsOn,
					VerificationScript: rt.VerificationScript,
				})
			}

			// POST-RECONCILIATION AUDIT: Ensure all tasks from stories.json are actually loaded
			missingIDs := []string{}
			loadedMap := make(map[string]bool)
			for _, t := range tasks {
				loadedMap[t.ID] = true
			}
			for tid := range incomingTaskStories {
				if !loadedMap[tid] {
					missingIDs = append(missingIDs, tid)
				}
			}

			// ACTIVE DEEP HEALING: If tasks are missing, try to re-import them automatically
			if len(missingIDs) > 0 {
				fmt.Printf("  ⚠ Warning: %d task(s) missing from database. Attempting active deep healing...\n", len(missingIDs))
				
				// Re-read stories.json to get full task data for missing items
				data, err := os.ReadFile(storiesFile)
				if err == nil {
					// We'll reuse the struct from the fallback logic later
					type GeneratedTask struct {
						ID                 string   `json:"id"`
						Title              string   `json:"title"`
						Description        string   `json:"description"`
						DependsOn          []string `json:"depends_on"`
						VerificationScript string   `json:"verification_script,omitempty"`
					}
					var sf struct {
						Stories []struct {
							ID    string          `json:"id"`
							Tasks []GeneratedTask `json:"tasks"`
						} `json:"stories"`
					}
					if err := json.Unmarshal(data, &sf); err == nil {
						for _, s := range sf.Stories {
							for _, gt := range s.Tasks {
								isMissing := false
								for _, mid := range missingIDs {
									if gt.ID == mid { isMissing = true; break }
								}
								if isMissing {
									// Create the missing task
									newTask := &release.Task{
										ID:                 gt.ID,
										Title:              gt.Title,
										Description:        gt.Description,
										StoryID:            s.ID,
										DependsOn:          gt.DependsOn,
										VerificationScript: gt.VerificationScript,
										Status:             release.TaskStatusPending,
									}
									if err := mgr.CreateTask(newTask); err == nil {
										fmt.Printf("  ✨ Deep-Healed: Restored missing task %s\n", gt.ID)
										// Add to local set so it's runnable immediately
										tasks = append(tasks, Task{
											ID:                 gt.ID,
											Title:              gt.Title,
											Description:        gt.Description,
											StoryID:            s.ID,
											Status:             "pending",
											DependsOn:          gt.DependsOn,
											VerificationScript: gt.VerificationScript,
										})
									}
								}
							}
						}
					}
				}
			}

			return tasks, nil
		}
	}

	// 2. Fallback to root tasks.json
	rootTasksFile := filepath.Join(projectDir, "tasks.json")
	if data, err := os.ReadFile(rootTasksFile); err == nil {
		var tf TasksFile
		if err := json.Unmarshal(data, &tf); err == nil {
			return tf.Tasks, nil
		}
	}

	// 3. Fallback to .openexec/tasks.json
	hiddenTasksFile := filepath.Join(projectDir, ".openexec", "tasks.json")
	if data, err := os.ReadFile(hiddenTasksFile); err == nil {
		var tf TasksFile
		if err := json.Unmarshal(data, &tf); err == nil {
			return tf.Tasks, nil
		}
	}

	// 4. Fallback to stories.json (Planner Output)
	storiesFile = filepath.Join(projectDir, ".openexec", "stories.json")
	storiesData, err := os.ReadFile(storiesFile)
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

	if err := json.Unmarshal(storiesData, &sf); err != nil {
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

	var tasks []Task
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
					contracts, hasContracts := storyContractTaskIDs[depStoryID]
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
					Status:             story.Status,
					DependsOn:          deps,
					VerificationScript: genTask.VerificationScript,
				})

				// Move to next task in story
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
		MaxIterations: runMaxIterations,
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

	if resp.StatusCode == http.StatusConflict {
		// Pipeline already active! During self-healing, we should aggressively stop it.
		stopURL := fmt.Sprintf("http://localhost:%d/api/fwu/%s/stop", startPort, task.ID)
		_, _ = http.Post(stopURL, "application/json", nil)
		time.Sleep(1 * time.Second) // give server a moment to cleanup

		// Retry the create request
		resp, err = http.Post(
			fmt.Sprintf("http://localhost:%d/api/v1/loops", startPort),
			"application/json",
			bytes.NewReader(body),
		)
		if err != nil {
			return "", fmt.Errorf("failed to retry after stop: %w", err)
		}
		defer resp.Body.Close()
	}

	if resp.StatusCode != http.StatusCreated {
		respBody, _ := io.ReadAll(resp.Body)
		var errData struct {
			Error      string `json:"error"`
			Suggestion string `json:"suggestion"`
		}
		if err := json.Unmarshal(respBody, &errData); err == nil {
			if errData.Suggestion != "" {
				return "", fmt.Errorf("server error: %s\n💡 Suggestion: %s", errData.Error, errData.Suggestion)
			}
			return "", fmt.Errorf("server error: %s", errData.Error)
		}
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

	if story != nil {
		sb.WriteString(fmt.Sprintf("STORY ID: %s\n", story.ID))
		sb.WriteString(fmt.Sprintf("STORY TITLE: %s\n", story.Title))
		sb.WriteString(fmt.Sprintf("STORY FILE: .openexec/stories/%s.md\n", story.ID))
	}

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

// waitForLoop polls the loop status until completion or timeout.
// prefix is prepended to all output lines (e.g. "[Worker 1] ").
func waitForLoop(cmd *cobra.Command, loopID string, prefix string, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	client := &http.Client{Timeout: 5 * time.Second}
	lastIteration := 0
	lastPhase := ""
	startTime := time.Now()

	for {
		select {
		case <-ctx.Done():
			cmd.Printf("%s   ⚠ Timeout reached after %v. Aborting server-side execution...\n", prefix, timeout)
			// Explicitly stop the loop on the server so retries can proceed
			stopURL := fmt.Sprintf("http://localhost:%d/api/fwu/%s/stop", startPort, loopID)
			_, _ = http.Post(stopURL, "application/json", nil)
			return fmt.Errorf("loop timed out after %v", timeout)
		default:
			// continue
		}

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

		// Show phase transitions
		if loop.Phase != "" && loop.Phase != lastPhase {
			cmd.Printf("%s   → Phase %s (%s)\n", prefix, loop.Phase, loop.Agent)
			lastPhase = loop.Phase
		}

		// Show iteration progress
		if loop.Iteration > lastIteration {
			cmd.Printf("%s   → Iteration %d\n", prefix, loop.Iteration)
			lastIteration = loop.Iteration
		}

		// Check terminal states
		switch loop.Status {
		case "complete":
			elapsed := time.Since(startTime).Truncate(time.Second)
			cmd.Printf("%s   ✓ Complete (%s)\n", prefix, elapsed)
			return nil
		case "error":
			if loop.Error != "" {
				cmd.Printf("%s   ❌ Error: %s\n", prefix, loop.Error)
				return fmt.Errorf("loop failed: %s", loop.Error)
			}
			cmd.Printf("%s   ❌ Error\n", prefix)
			return fmt.Errorf("loop failed")
		case "max_iterations":
			cmd.Printf("%s   ⚠ Max iterations reached (%d)\n", prefix, loop.Iteration)
			return nil
		case "paused":
			if loop.Error != "" {
				cmd.Printf("%s   ⏸ Paused: %s\n", prefix, loop.Error)
				return fmt.Errorf("loop paused: %s", loop.Error)
			}
			cmd.Printf("%s   ⏸ Paused\n", prefix)
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
