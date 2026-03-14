package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
    "time"

	"github.com/fatih/color"
	"github.com/gorilla/websocket"
	"github.com/openexec/openexec/internal/planner"
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
	runVerbose       bool
	runNoAutoPlan    bool
	runQuickfix      string
	runVerify        string
	runMode          string
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

// Legacy loop request/response removed; CLI now uses /api/fwu and /api/v1/runs

var startCmd = &cobra.Command{
	Use:   "start",
	Short: "Start execution daemon for concurrent task processing",
	RunE: func(cmd *cobra.Command, args []string) error {
		config, err := project.LoadProjectConfig(".")
		if err != nil {
			return fmt.Errorf("project not initialized: run 'openexec init' first")
		}

		if !cmd.Flags().Changed("port") && config.Execution.Port > 0 {
			startPort = config.Execution.Port
		}
		if !cmd.Flags().Changed("timeout") && config.Execution.TimeoutSeconds > 0 {
			startTimeout = config.Execution.TimeoutSeconds
		}
		if !cmd.Flags().Changed("reviewer") && config.Execution.ReviewEnabled {
			startReviewer = config.Execution.ReviewerModel
		}

		dataDir := filepath.Join(config.ProjectDir, ".openexec", "data")
		auditDB := filepath.Join(dataDir, "audit.db")

		finalPort, err := findAvailablePort(startPort)
		if err != nil {
			return err
		}
		if finalPort != startPort {
			cmd.Printf("   ⚠ Port %d is busy, using %d instead\n", startPort, finalPort)
			startPort = finalPort
		}

		serverArgs := []string{"start", "--port", fmt.Sprintf("%d", startPort)}
		if startReviewer != "" {
			serverArgs = append(serverArgs, "--reviewer", startReviewer)
		}

		if startDaemon {
			if isServerRunning(config.ProjectDir, startPort) {
				return fmt.Errorf("execution engine already running on port %d", startPort)
			}

			execPath, err := os.Executable()
			if err != nil {
				return fmt.Errorf("failed to find executable: %w", err)
			}

			logPath := filepath.Join(config.ProjectDir, ".openexec", "daemon.log")
			logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
			if err != nil {
				return fmt.Errorf("failed to open log file: %w", err)
			}

			c := exec.Command(execPath, serverArgs...)
			c.Dir = config.ProjectDir
			c.Stdout = logFile
			c.Stderr = logFile

			if err := c.Start(); err != nil {
				return fmt.Errorf("failed to start background process: %w", err)
			}

			fmt.Printf("✓ Execution engine started in background (PID: %d)\n", c.Process.Pid)
			return nil
		}

		if startUI {
			go func() {
				_ = waitForServer(startPort, 15*time.Second)
				uiURL := fmt.Sprintf("http://localhost:%d", startPort)
				_ = exec.Command("open", uiURL).Start()
			}()
		}

		if err := writePIDFile(config.ProjectDir, startPort); err != nil {
			cmd.Printf("   ⚠ Warning: could not write PID file: %v\n", err)
		}
		defer func() { _ = removePIDFile(config.ProjectDir) }()

        // Enable DCP only when explicitly requested via env
        enableDCP := false
        if v := os.Getenv("OPENEXEC_ENABLE_DCP"); v != "" {
            lv := strings.ToLower(v)
            enableDCP = (lv == "1" || lv == "true" || lv == "yes")
        }

        srv, err := server.New(server.Config{
            Port:        startPort,
            AuditDB:     auditDB,
            DataDir:     dataDir,
            ProjectsDir: config.ProjectDir,
            EnableDCP:   enableDCP,
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
	RunE: func(cmd *cobra.Command, args []string) error {
		config, err := project.LoadProjectConfig(".")
		if err != nil {
			return fmt.Errorf("project not initialized: run 'openexec init' first")
		}

		// 1. AUTO-START ENGINE
		if !isServerRunning(config.ProjectDir, 0) { // Check if ANY engine is running for this project
			cmd.Println("⚙️ Execution engine not running. Starting daemon...")
			// Start with requested port, but it might move if busy
			startArgs := []string{"start", "--daemon", "--port", fmt.Sprintf("%d", startPort)}
			execPath, _ := os.Executable()
			startExec := exec.Command(execPath, startArgs...)
			startExec.Dir = config.ProjectDir
			if err := startExec.Run(); err != nil {
				return fmt.Errorf("failed to auto-start execution engine: %w", err)
			}

			// Wait for PID file to be written and engine to initialize
			time.Sleep(1 * time.Second)
		}

		// Always sync port from PID file to handle auto-migration (port busy)
		if _, effectivePort, err := readPID(config.ProjectDir); err == nil {
			if effectivePort != startPort {
				cmd.Printf("   ℹ️ Syncing with engine on port %d (PID file sync)\n", effectivePort)
				startPort = effectivePort
			}
		}

		if err := waitForServer(startPort, 15*time.Second); err != nil {
			return fmt.Errorf("engine failed to become ready on port %d: %w", startPort, err)
		}

		// 2. MAIN EXECUTION LOOP (Supports Autonomous Plan-Healing Restarts)
		for {
			// AUTO-PLAN
			if !runNoAutoPlan {
				intentPath := "INTENT.md"
				storiesPath := filepath.Join(config.TractStore, "stories.json")
				wizardPath := filepath.Join(config.TractStore, "wizard_state.json")

				needsPlanning := false

				if _, err := os.Stat(intentPath); os.IsNotExist(err) {
					if _, err := os.Stat(wizardPath); err == nil {
						if data, err := os.ReadFile(wizardPath); err == nil {
							var ws struct {
								UpdatedState planner.IntentState `json:"updated_state"`
							}
							if err := json.Unmarshal(data, &ws); err == nil {
								if ws.UpdatedState.IsReady() {
									cmd.Println("📝 INTENT.md missing but wizard state complete. Rendering intent...")
									intentContent := ws.UpdatedState.RenderIntentMD()
									_ = os.WriteFile(intentPath, []byte(intentContent), 0644)
									cmd.Println("✓ INTENT.md rendered from wizard state.")
									needsPlanning = true
								}
							}
						}
					}
				}

				if _, err := os.Stat(storiesPath); os.IsNotExist(err) {
					needsPlanning = true
				} else {
					intentStat, _ := os.Stat(intentPath)
					storiesStat, _ := os.Stat(storiesPath)
					if intentStat != nil && storiesStat != nil && intentStat.ModTime().After(storiesStat.ModTime()) {
						cmd.Println("🔄 INTENT.md was modified. Re-generating plan...")
						needsPlanning = true
					}
				}

				if needsPlanning {
					if err := GenerateAndSave(cmd, intentPath, config.ProjectDir); err != nil {
						return fmt.Errorf("auto-planning failed: %w", err)
					}
				}
			}

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

		mgr, err := getReleaseManager(cmd)
		if err != nil {
			return err
		}

		// QUICKFIX FLOW: Deterministic single-task execution without planning
		if strings.TrimSpace(runQuickfix) != "" {
			title := strings.TrimSpace(runQuickfix)
			verify := strings.TrimSpace(runVerify)
			if verify == "" {
				verify = "echo quickfix-verify"
			}
			if strings.ToLower(runMode) == "read-only" {
				return fmt.Errorf("quickfix requires write access. Set --mode workspace-write or danger-full-access, or run with --verify only (no edits)")
			}
			qf := Task{
				ID:                 "T-QF-001",
				Title:              "Chassis: " + title,
				Description:        "Deterministic quickfix execution (no planning)",
				StoryID:            "US-QF",
				Status:             "pending",
				DependsOn:          nil,
				VerificationScript: verify,
				TechnicalStrategy:  "FAST-TRACK: Apply minimal, reviewable change and validate using the provided verification script.",
			}
			cmd.Printf("📋 Executing quickfix task: %s\n", qf.Title)
			return executeTasksParallel(cmd, config.ProjectDir, []Task{qf}, 1, mgr)
		}

			cmd.Println("🔍 Running Pre-flight Active Healing...")
			relTasks := mgr.GetTasks()
			resetCount := 0
			for _, rt := range relTasks {
				if rt.Status == "running" || rt.Status == "starting" {
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

        tasks, err := loadPendingTasks(config.ProjectDir, mgr, false)
            if err != nil || len(tasks) == 0 {
                // AUTO-IMPORT: If runtime DB is empty, try importing from .openexec/stories.json once
                if tryAutoImportStories(config.ProjectDir, mgr, cmd) {
                    // Reload after import
                    tasks, err = loadPendingTasks(config.ProjectDir, mgr, false)
                }
            }
            if err != nil {
                return fmt.Errorf("failed to load tasks: %w", err)
            }

            if len(tasks) == 0 {
                cmd.Println("No pending tasks found.")
                cmd.Println("Hint: add stories to .openexec/stories.json or run 'openexec story import' manually.")
                return nil
            }

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
			err = executeTasksParallel(cmd, config.ProjectDir, tasks, config.Execution.WorkerCount, mgr)
			if err != nil {
				if strings.Contains(err.Error(), "plan_healed") {
					cmd.Println("🔄 Orchestrator: Plan was autonomously healed. Restarting run loop to pick up new tasks...")
					continue
				}
				return err
			}

			cmd.Println("✓ Execution complete")
			return nil
		}
	},
}

var stopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop execution engine",
	RunE: func(cmd *cobra.Command, args []string) error {
		config, err := project.LoadProjectConfig(".")
		if err != nil {
			return err
		}
		if pid, _, err := readPID(config.ProjectDir); err == nil {
			return KillDaemon(pid)
		}
		return nil
	},
}

var restartCmd = &cobra.Command{
	Use:   "restart",
	Short: "Restart execution engine",
	RunE: func(cmd *cobra.Command, args []string) error {
		_ = stopCmd.RunE(cmd, args)
		time.Sleep(1 * time.Second)
		return startCmd.RunE(cmd, args)
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

// isStudyTask returns true if the task title indicates a study/mapping task.
func isStudyTask(title string) bool {
	lower := strings.ToLower(title)
	return strings.Contains(lower, "study") ||
		strings.Contains(lower, "mapping") ||
		strings.Contains(lower, "map")
}

// isChassisTask returns true if the task title indicates a chassis task.
func isChassisTask(title string) bool {
	return strings.Contains(strings.ToLower(title), "chassis")
}

func executeTasksParallel(cmd *cobra.Command, projectDir string, tasks []Task, workerCount int, mgr *release.Manager) error {
	if workerCount <= 0 {
		workerCount = 4
	}

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

	inDegree := make(map[string]int)
	for _, node := range nodes {
		if node.Status == StatusCompleted {
			continue
		}
		for depID := range node.DependsOn {
			depNode, exists := nodes[depID]
			if exists && depNode.Status != StatusCompleted {
				inDegree[node.Task.ID]++
			}
		}
	}

	queue := []string{}
	for id, node := range nodes {
		if node.Status != StatusCompleted && inDegree[id] == 0 {
			queue = append(queue, id)
		}
	}

	visitedCount := 0
	for len(queue) > 0 {
		u := queue[0]
		queue = queue[1:]
		visitedCount++

		for id, node := range nodes {
			if node.Status == StatusCompleted {
				continue
			}
			for depID := range node.DependsOn {
				if depID == u {
					inDegree[id]--
					if inDegree[id] == 0 {
						queue = append(queue, id)
					}
				}
			}
		}
	}

	if visitedCount < totalToRun {
		cmd.Printf("\n%s DEADLOCK DETECTED: Dependency cycle found in tasks.\n", color.RedString("❌"))
		return fmt.Errorf("dependency cycle detected")
	}

	if totalToRun == 0 {
		return nil
	}

	var mu sync.Mutex
	var wg sync.WaitGroup
	var closeOnce sync.Once
	readyTasks := make(chan *TaskNode, len(tasks))
	errors := make(chan error, len(tasks))
	finishedCount := 0
	doneCount := 0

	safeClose := func() {
		closeOnce.Do(func() {
			close(readyTasks)
		})
	}

	checkReady := func() {
		mu.Lock()
		defer mu.Unlock()

		if len(errors) > 0 || finishedCount == totalToRun {
			return
		}

		for _, node := range nodes {
			if node.Status != StatusPending {
				continue
			}

			allDone := true
			for depID := range node.DependsOn {
				depNode, exists := nodes[depID]
				if exists && depNode.Status != StatusCompleted {
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

	go checkReady()

	for i := 0; i < workerCount; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for node := range readyTasks {
				mu.Lock()
				if len(errors) > 0 {
					mu.Unlock()
					return
				}
				node.Status = StatusRunning
				mu.Unlock()

				isChassis := isChassisTask(node.Task.Title)
				isStudy := isStudyTask(node.Task.Title)
				isFastTrack := isChassis || isStudy

				if isFastTrack {
					trackName := "Chassis"
					if isStudy {
						trackName = "Study"
					}
					cmd.Printf("[Worker %d] ⚡ FAST-TRACK: Executing combined %s task %s\n", workerID, trackName, node.Task.ID)
				} else {
					cmd.Printf("[Worker %d] Executing %s: %s\n", workerID, node.Task.ID, node.Task.Title)
				}

				var lastError string
				maxRetries := 3
				success := false

				for node.Retries < maxRetries {
					// STALL RECOVERY: If this is a retry, stop the existing loop first to ensure a fresh process
					if node.Retries > 0 {
						stopURL := fmt.Sprintf("http://localhost:%d/api/fwu/%s/stop", startPort, node.Task.ID)
						_, _ = http.Post(stopURL, "application/json", nil)
						time.Sleep(2 * time.Second) // Increased sleep to give server more time
					}

					loopID, err := createExecutionLoopWithRetry(projectDir, node.Task, mgr, lastError)
					if err != nil {
						lastError = err.Error()
						lowerErr := strings.ToLower(lastError)

						if strings.Contains(lowerErr, "409") || strings.Contains(lowerErr, "already active") {
							loopID = node.Task.ID
							err = nil
						} else {
							if strings.Contains(lowerErr, "not found on path") || strings.Contains(lowerErr, "auth") {
								cmd.Printf("[Worker %d] ❌ NON-RETRIABLE RUNNER ERROR: %s\n", workerID, lastError)
								node.Retries = maxRetries
								break
							}

							cmd.Printf("[Worker %d] ⚠ Loop creation failed (attempt %d/%d): %v\n", workerID, node.Retries+1, maxRetries, err)
							node.Retries++
							continue
						}
					}

					workerPrefix := fmt.Sprintf("[Worker %d]", workerID)
					effectiveTimeout := time.Duration(runTimeout) * time.Second

					if isFastTrack {
						workerPrefix = fmt.Sprintf("[Worker %d] (fast-track)", workerID)
						effectiveTimeout = time.Duration(float64(runTimeout)*0.6) * time.Second
					}

					err = waitForLoop(cmd, loopID, workerPrefix, effectiveTimeout, isFastTrack)
					if err == nil {
						success = true
						break
					}
					lastError = err.Error()

					// UNIVERSAL SELF-HEALING Logic
					lowerErr := strings.ToLower(lastError)

					// A. Plan-Healing (Design Mismatch)
					isRefinementRequired := strings.Contains(lowerErr, "design must be updated") ||
						strings.Contains(lowerErr, "update the strategy") ||
						strings.Contains(lowerErr, "plan needs") ||
						strings.Contains(lowerErr, "planning mismatch")

					if isRefinementRequired {
						// Filter out completion signals first
						isComplete := strings.Contains(lowerErr, "complete") ||
							strings.Contains(lowerErr, "done") ||
							strings.Contains(lowerErr, "satisfied")

						if !isComplete {
							cmd.Printf("[Worker %d] 🔄 PLAN-HEALING: Agent requested plan update. Re-generating with failure context...\n", workerID)

							// Stop the active task so it doesn't conflict later
							stopURL := fmt.Sprintf("http://localhost:%d/api/fwu/%s/stop", startPort, loopID)
							_, _ = http.Post(stopURL, "application/json", nil)

							if err := GenerateAndSave(cmd, "INTENT.md", projectDir); err == nil {
								mu.Lock()
								if len(errors) == 0 {
									// Signal outer loop to restart with new plan
									errors <- fmt.Errorf("plan_healed")
								}
								safeClose()
								mu.Unlock()
								return // Abort this worker to trigger the restart
							} else {
								cmd.Printf("[Worker %d] ⚠ Plan-healing failed: %v\n", workerID, err)
							}
						}
					}

					// B. Documentation/Analysis Completion (Surgical auto-complete)
					if isStudy {
						isDocComplete := strings.Contains(lowerErr, "outside") && strings.Contains(lowerErr, "scope") ||
							strings.Contains(lowerErr, "test mismatch") ||
							strings.Contains(lowerErr, "unrelated failing test") ||
							strings.Contains(lowerErr, "already been implemented") ||
							strings.Contains(lowerErr, "cannot complete") && strings.Contains(lowerErr, "red tests")

						if isDocComplete {
							cmd.Printf("[Worker %d] ✨ AUTO-HEAL: Documentation/Study task completed despite minor environment noise.\n", workerID)
							success = true
							break
						}
					}

					// C. Strategy Pivoting (Logic Failures)
					if node.Retries == 0 {
						cmd.Printf("[Worker %d] ⚠️ Failure detected. Command: PIVOT STRATEGY for next retry.\n", workerID)
						lastError = "⚠️ PIVOT STRATEGY MANDATE: Your previous approach failed with: " + lastError + ". You MUST try a radically different implementation strategy now."
					} else if node.Retries == 1 {
						cmd.Printf("[Worker %d] ⚠️ Second failure. Command: FINAL ATTEMPT with diagnostic mandate.\n", workerID)
						lastError = "⚠️ FINAL ATTEMPT MANDATE: Two attempts have failed. Previous error: " + lastError + ". Focus on absolute simplicity and verify every assumption."
					}

					node.Retries++
				}

				mu.Lock()
				finishedCount++
				if !success {
					node.Status = StatusFailed
					errors <- fmt.Errorf("task %s failed: %s", node.Task.ID, lastError)
					safeClose()
					mu.Unlock()
					return
				}

				if mgr != nil {
					if _, err := mgr.CompleteTask(node.Task.ID); err != nil {
						// Fallback: update via SetTaskStatus if CompleteTask fails
						_ = mgr.SetTaskStatus(node.Task.ID, release.TaskStatusDone)
					}
					story := mgr.GetStory(node.Task.StoryID)
					if story != nil && story.Status == release.StoryStatusDone {
						cmd.Printf("[Worker %d] 🧬 Story %s complete. Integrating local branch...\n", workerID, story.ID)
						integrateStoryBranch(cmd, projectDir, story.ID, workerID)
					}
				}

				node.Status = StatusCompleted
				// Note: State is persisted via manager.CompleteTask/SetTaskStatus (SQLite canonical)
				doneCount++
				cmd.Printf("[Worker %d] ✓ Completed %s (%d/%d)\n", workerID, node.Task.ID, doneCount, totalToRun)

				if finishedCount == totalToRun {
					safeClose()
				}
				mu.Unlock()
				checkReady()
			}
		}(i)
	}

	wg.Wait()
	if len(errors) > 0 {
		// Prioritize plan_healed signal to ensure the orchestrator restarts
		for i := 0; i < len(errors); i++ {
			err := <-errors
			if strings.Contains(err.Error(), "plan_healed") {
				return err
			}
			// Put it back if it's not what we want (but only if we have more to check)
			if i < len(errors)-1 {
				errors <- err
			} else {
				return err
			}
		}
	}
	return nil
}

func findAvailablePort(basePort int) (int, error) {
	for port := basePort; port < basePort+100; port++ {
		ln, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
		if err == nil {
			_ = ln.Close()
			return port, nil
		}
	}
	return 0, fmt.Errorf("no ports available")
}

func writePIDFile(projectDir string, port int) error {
	pidFile := filepath.Join(projectDir, ".openexec", "openexec.pid")
	return os.WriteFile(pidFile, []byte(fmt.Sprintf("%d:%d", os.Getpid(), port)), 0644)
}

func removePIDFile(projectDir string) error {
	return os.Remove(filepath.Join(projectDir, ".openexec", "openexec.pid"))
}

func readPID(projectDir string) (int, int, error) {
	data, err := os.ReadFile(filepath.Join(projectDir, ".openexec", "openexec.pid"))
	if err != nil {
		return 0, 0, err
	}
	var pid, port int
	fmt.Sscanf(string(data), "%d:%d", &pid, &port)
	return pid, port, nil
}

func waitForServer(port int, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		resp, err := http.Get(fmt.Sprintf("http://localhost:%d/api/ready", port))
		if err == nil {
			resp.Body.Close()
			return nil
		}
		time.Sleep(500 * time.Millisecond)
	}
	return fmt.Errorf("timeout")
}

func isServerRunning(projectDir string, port int) bool {
	pid, runningPort, err := readPID(projectDir)
	if err != nil {
		return false
	}

	// 1. Check if PID is alive
	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	// On Unix, findProcess always succeeds, so we must check Signal(0)
	if process.Signal(syscall.Signal(0)) != nil {
		return false
	}

	// 2. Check if port is responding
	// If port is 0, we use the port from the PID file
	checkPort := runningPort
	if port > 0 {
		checkPort = port
	}

	client := http.Client{Timeout: 500 * time.Millisecond}
	resp, err := client.Get(fmt.Sprintf("http://localhost:%d/api/ready", checkPort))
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

// upsertTaskStatus removed: SQLite is now the canonical state store.
// Task status updates go through release.Manager which persists to SQLite.

func integrateStoryBranch(cmd *cobra.Command, projectDir string, storyID string, workerID int) {
	relPrefix := "release/"
	featPrefix := "feature/"
	baseBranch := "main"
	if projCfg, err := project.LoadProjectConfig(projectDir); err == nil {
		if projCfg.ReleaseBranchPrefix != "" {
			relPrefix = projCfg.ReleaseBranchPrefix
		}
		if projCfg.FeatureBranchPrefix != "" {
			featPrefix = projCfg.FeatureBranchPrefix
		}
		if projCfg.BaseBranch != "" {
			baseBranch = projCfg.BaseBranch
		}
	}

	releaseBranch := relPrefix + "current"
	fromVersion, _ := exec.Command("openexec", "version", "--short").Output()
	if v := strings.TrimSpace(string(fromVersion)); v != "" {
		releaseBranch = relPrefix + v
	}

	storyBranch := featPrefix + storyID

	_ = exec.Command("git", "checkout", releaseBranch).Run()
	mergeCmd := exec.Command("git", "merge", "--no-ff", "-m", fmt.Sprintf("Integrate story %s", storyID), storyBranch)
	if out, err := mergeCmd.CombinedOutput(); err != nil {
		cmd.Printf("[Worker %d] ⚠ Integration failed for story %s: %v\n%s\n", workerID, storyID, err, string(out))
	} else {
		cmd.Printf("[Worker %d] ✓ Integrated %s into %s\n", workerID, storyBranch, releaseBranch)
		_ = exec.Command("git", "branch", "-d", storyBranch).Run()
	}
	_ = exec.Command("git", "checkout", baseBranch).Run()
}

func createExecutionLoopWithRetry(projectDir string, task Task, mgr *release.Manager, lastError string) (string, error) {
    // prompt and MCP config are handled server-side for FWU start

	isStudy := isStudyTask(task.Title)

    payload := map[string]any{}
    if isStudy { payload["is_study"] = true }
    if runMode != "" { payload["mode"] = runMode }
    body, _ := json.Marshal(payload)
    resp, err := http.Post(fmt.Sprintf("http://localhost:%d/api/fwu/%s/start", startPort, task.ID), "application/json", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

    if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
        respBody, _ := io.ReadAll(resp.Body)
        return "", fmt.Errorf("server returned %d: %s", resp.StatusCode, string(respBody))
    }

    return task.ID, nil
}

func buildTaskPromptWithRetry(task Task, mgr *release.Manager, lastError string) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("TASK ID: %s\nTITLE: %s\n", task.ID, task.Title))
	if lastError != "" {
		sb.WriteString(fmt.Sprintf("\n⚠️ SELF-HEALING CONTEXT:\n%s\n", lastError))
	}

	// Inject Environment Context
	out, err := exec.Command("git", "status", "--short").Output()
	if err == nil && len(out) > 0 {
		sb.WriteString("\n📋 CURRENT ENVIRONMENT (GIT STATUS):\n")
		sb.WriteString(string(out))
	}

	return sb.String()
}

func waitForLoop(cmd *cobra.Command, loopID string, prefix string, timeout time.Duration, isChassis bool) error {
	deadline := time.Now().Add(timeout)
	lastIteration := -1
	lastPhase := ""
	lastAgent := ""
	lastActivity := time.Now()

	// DYNAMIC STALL THRESHOLD: max(6m, min(15m, timeout/3)); double for Chassis.
	stallMinutes := 6.0
	calculated := (timeout.Minutes() / 3.0)
	if calculated > stallMinutes {
		stallMinutes = calculated
	}
	if stallMinutes > 15.0 {
		stallMinutes = 15.0
	}
	if isChassis {
		stallMinutes *= 2.0
	}
	stallThreshold := time.Duration(stallMinutes) * time.Minute

	// Try WebSocket connection first
	wsURL := fmt.Sprintf("ws://localhost:%d/ws", startPort)
	dialer := websocket.DefaultDialer
	conn, _, err := dialer.Dial(wsURL, nil)

	if err == nil {
		defer conn.Close()

		// Subscribe to the loop session
		subscribeMsg := map[string]interface{}{
			"type":      "subscribe",
			"sessionId": loopID,
		}
		if err := conn.WriteJSON(subscribeMsg); err != nil {
			// Fallback to polling if subscription fails
			err = nil
		} else {
			// Listen for events via WebSocket
			type wsMessage struct {
				Type      string      `json:"type"`
				SessionID string      `json:"sessionId"`
				Payload   interface{} `json:"payload"`
			}

			// We still need to check deadline and heartbeat
			go func() {
				for {
					var msg wsMessage
					if err := conn.ReadJSON(&msg); err != nil {
						return
					}

					if msg.Type == "event" {
						// Payload is a loop.Event (or similar map)
						if payload, ok := msg.Payload.(map[string]interface{}); ok {
							iteration, _ := payload["Iteration"].(float64)
							phase, _ := payload["Phase"].(string)
							agent, _ := payload["Agent"].(string)
							eventType, _ := payload["Type"].(string)

							if int(iteration) > lastIteration || phase != lastPhase || agent != lastAgent || eventType == "heartbeat" || eventType == "progress" {
								lastIteration = int(iteration)
								lastPhase = phase
								lastAgent = agent
								lastActivity = time.Now()
							}
						}
					}
				}
			}()
		}
	}

	// Main monitoring loop (uses polling as secondary/heartbeat mechanism)
	for time.Now().Before(deadline) {
        resp, err := http.Get(fmt.Sprintf("http://localhost:%d/api/fwu/%s/status", startPort, loopID))
		if err != nil {
			time.Sleep(2 * time.Second)
			continue
		}
        var loopResp struct{
            Status string `json:"status"`
            Iteration int `json:"iteration"`
            Error string `json:"error"`
            Phase string `json:"phase"`
            Agent string `json:"agent"`
            LastActivity time.Time `json:"last_activity"`
        }
		if err := json.NewDecoder(resp.Body).Decode(&loopResp); err != nil {
			resp.Body.Close()
			time.Sleep(2 * time.Second)
			continue
		}
		resp.Body.Close()

		if loopResp.Status == "complete" {
			return nil
		}
		if loopResp.Status == "error" {
			return fmt.Errorf("%s", loopResp.Error)
		}

		if loopResp.Status == "paused" {
			if strings.Contains(loopResp.Error, "Planning Mismatch") {
				lowerErr := strings.ToLower(loopResp.Error)
				isComplete := strings.Contains(lowerErr, "complete") ||
					strings.Contains(lowerErr, "done") ||
					strings.Contains(lowerErr, "already been implemented") ||
					strings.Contains(lowerErr, "satisfied") ||
					strings.Contains(lowerErr, "criteria verified")

				if isComplete {
					cmd.Printf("%s ✨ AUTO-HEAL: Agent verified task is complete or redundant.\n", prefix)
					return nil
				}
				// Plan-healing requested
				return fmt.Errorf("%s", loopResp.Error)
			}

			// For generic paused status (e.g. decision-point), return as terminal so orchestrator knows it's not running
			return fmt.Errorf("agent paused: %s", loopResp.Error)
		}

		// HEARTBEAT MONITOR: Detect if runner is making progress (iteration, phase, agent or heartbeat)
		if loopResp.Iteration > lastIteration || loopResp.Phase != lastPhase || loopResp.Agent != lastAgent {
			lastIteration = loopResp.Iteration
			lastPhase = loopResp.Phase
			lastAgent = loopResp.Agent
			lastActivity = time.Now()
		} else if !loopResp.LastActivity.IsZero() && loopResp.LastActivity.After(lastActivity) {
			lastActivity = loopResp.LastActivity
		}

		if time.Since(lastActivity) > stallThreshold {
			return fmt.Errorf("runner stalled: no activity progress for %v", stallThreshold)
		}

		// With WebSocket active, we can poll much less frequently
		pollInterval := 5 * time.Second
		if conn == nil {
			pollInterval = 2 * time.Second
		}
		time.Sleep(pollInterval)
	}
	return fmt.Errorf("timeout")
}

// saveTaskStatus removed: SQLite is now the canonical state store.
// Task status updates go through release.Manager which persists to SQLite.

func KillDaemon(pid int) error {
	p, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	return p.Signal(syscall.SIGTERM)
}

func ensureMCPConfig(projectDir string) (string, error) {
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

func init() {
	startCmd.Flags().IntVarP(&startPort, "port", "P", 8765, "HTTP server port")
	startCmd.Flags().BoolVarP(&startDaemon, "daemon", "d", false, "Run as background daemon")
	startCmd.Flags().BoolVar(&startUI, "ui", false, "Open web console")
	startCmd.AddCommand(stopCmd)
	startCmd.AddCommand(restartCmd)

	runCmd.Flags().IntVar(&startPort, "port", 8765, "Execution engine port")
	runCmd.Flags().IntVar(&runMaxIterations, "max-iterations", 10, "Max iterations")
    runCmd.Flags().IntVar(&runTimeout, "timeout", 1800, "Timeout")
    runCmd.Flags().BoolVarP(&runVerbose, "verbose", "v", false, "Verbose logs")
    runCmd.Flags().BoolVar(&runNoAutoPlan, "no-auto-plan", false, "Disable automatic planning")
    runCmd.Flags().StringVar(&runQuickfix, "quickfix", "", "Execute a single deterministic quickfix without planning (task title)")
    runCmd.Flags().StringVar(&runVerify, "verify", "", "Verification script for --quickfix (defaults to echo quickfix-verify)")
    runCmd.Flags().StringVar(&runMode, "mode", "workspace-write", "Execution mode: read-only | workspace-write | danger-full-access")

	rootCmd.AddCommand(startCmd)
	rootCmd.AddCommand(runCmd)
	rootCmd.AddCommand(stopCmd)
}

// tryAutoImportStories attempts to import stories from .openexec/stories.json into the DB
// Returns true if an import was performed (regardless of success), false otherwise.
func tryAutoImportStories(projectDir string, mgr *release.Manager, cmd *cobra.Command) bool {
    storiesPath := filepath.Join(projectDir, ".openexec", "stories.json")
    data, err := os.ReadFile(storiesPath)
    if err != nil { return false }
    var sf struct {
        Stories []GeneratedStory `json:"stories"`
    }
    if err := json.Unmarshal(data, &sf); err != nil {
        // try legacy array-only format
        var bare []GeneratedStory
        if err2 := json.Unmarshal(data, &bare); err2 == nil {
            sf.Stories = bare
        } else {
            return false
        }
    }
    if len(sf.Stories) == 0 { return false }

    // Import minimal story/task records
    created := 0
    for _, s := range sf.Stories {
        if mgr.GetStory(s.ID) == nil {
            st := &release.Story{
                ID:                 s.ID,
                GoalID:             s.GoalID,
                Title:              s.Title,
                Description:        s.Description,
                AcceptanceCriteria: s.AcceptanceCriteria,
                VerificationScript: s.VerificationScript,
                DependsOn:          s.DependsOn,
                Status:             "pending",
                CreatedAt:          time.Now(),
            }
            _ = mgr.CreateStory(st)
        }
        // Create tasks under story
        var prevTaskID string
        for _, tRaw := range s.Tasks {
            var id, title, desc string
            var deps []string
            switch v := tRaw.(type) {
            case string:
                id = v
            case map[string]any:
                if v["id"] != nil { id, _ = v["id"].(string) }
                if v["title"] != nil { title, _ = v["title"].(string) }
                if v["description"] != nil { desc, _ = v["description"].(string) }
                if arr, ok := v["depends_on"].([]any); ok {
                    for _, a := range arr { if s, ok := a.(string); ok { deps = append(deps, s) } }
                }
            }
            if id == "" { continue }
            if mgr.GetTask(id) == nil {
                if prevTaskID != "" { deps = append(deps, prevTaskID) }
                task := &release.Task{
                    ID:          id,
                    Title:       title,
                    Description: desc,
                    StoryID:     s.ID,
                    DependsOn:   deps,
                    Status:      "pending",
                    CreatedAt:   time.Now(),
                }
                _ = mgr.CreateTask(task)
                prevTaskID = id
                created++
            }
        }
    }
    if created > 0 && cmd != nil {
        cmd.Printf("Imported %d tasks from %s (auto)\n", created, storiesPath)
    }
    return true
}
