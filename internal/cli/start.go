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
	RunE: func(cmd *cobra.Command, args []string) error {
		config, err := project.LoadProjectConfig(".")
		if err != nil {
			return fmt.Errorf("project not initialized: run 'openexec init' first")
		}

		// 1. AUTO-START ENGINE
		if !isServerRunning(config.ProjectDir, startPort) {
			cmd.Println("⚙️ Execution engine not running. Starting daemon...")
			startArgs := []string{"start", "--daemon", "--port", fmt.Sprintf("%d", startPort)}
			execPath, _ := os.Executable()
			startExec := exec.Command(execPath, startArgs...)
			startExec.Dir = config.ProjectDir
			if err := startExec.Run(); err != nil {
				return fmt.Errorf("failed to auto-start execution engine: %w", err)
			}
			
			time.Sleep(500 * time.Millisecond)
			if _, effectivePort, err := readPID(config.ProjectDir); err == nil {
				if effectivePort != startPort {
					cmd.Printf("   ℹ️ Engine started on effective port %d (PID file sync)\n", effectivePort)
					startPort = effectivePort
				}
			}

			if err := waitForServer(startPort, 15*time.Second); err != nil {
				return fmt.Errorf("engine failed to become ready on port %d: %w", startPort, err)
			}
			cmd.Println("✓ Execution engine started successfully.")
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
							var ws struct { UpdatedState planner.IntentState `json:"updated_state"` }
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

			tasks, err := loadPendingTasks(config.ProjectDir, mgr, true)
			if err != nil {
				return fmt.Errorf("failed to load tasks: %w", err)
			}

			if len(tasks) == 0 {
				cmd.Println("No pending tasks found.")
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
		if err != nil { return err }
		if pid, _, err := readPID(config.ProjectDir); err == nil {
			return KillDaemon(pid)
		}
		return nil
	},
}

var restartCmd = &cobra.Command{
	Use: "restart",
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
		if node.Status == StatusCompleted { continue }
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
			if node.Status == StatusCompleted { continue }
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
			if node.Status != StatusPending { continue }

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

				isChassis := strings.Contains(strings.ToLower(node.Task.Title), "chassis")
				if isChassis {
					cmd.Printf("[Worker %d] ⚡ FAST-TRACK: Executing combined Chassis task %s\n", workerID, node.Task.ID)
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
					
					if isChassis {
						workerPrefix = fmt.Sprintf("[Worker %d] (chassis)", workerID)
						effectiveTimeout = time.Duration(float64(runTimeout)*0.6) * time.Second
					}
					
					err = waitForLoop(cmd, loopID, workerPrefix, effectiveTimeout)
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

					// B. Strategy Pivoting (Logic Failures)
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
						_ = upsertTaskStatus(projectDir, node.Task.ID, "completed", node.Task.StoryID)
					}
					story := mgr.GetStory(node.Task.StoryID)
					if story != nil && story.Status == release.StoryStatusDone {
						cmd.Printf("[Worker %d] 🧬 Story %s complete. Integrating local branch...\n", workerID, story.ID)
						integrateStoryBranch(cmd, projectDir, story.ID, workerID)
					}
				}

				node.Status = StatusCompleted
				_ = saveTaskStatus(projectDir, node.Task.ID, "completed")
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
	if err != nil { return 0, 0, err }
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
	pid, _, err := readPID(projectDir)
	if err != nil { return false }
	process, err := os.FindProcess(pid)
	if err != nil { return false }
	return process.Signal(syscall.Signal(0)) == nil
}

func upsertTaskStatus(projectDir string, taskID string, status string, storyID string) error {
	paths := []string{
		filepath.Join(projectDir, ".openexec", "tasks.json"),
		filepath.Join(projectDir, "tasks.json"),
	}

	var data []byte
	var path string
	for _, p := range paths {
		if d, err := os.ReadFile(p); err == nil {
			data = d
			path = p
			break
		}
	}

	if data == nil {
		path = filepath.Join(projectDir, ".openexec", "tasks.json")
		_ = os.MkdirAll(filepath.Dir(path), 0750)
		data = []byte(`{"tasks":[]}`)
	}

	var tf struct {
		Tasks []map[string]interface{} `json:"tasks"`
	}
	_ = json.Unmarshal(data, &tf)

	found := false
	for _, t := range tf.Tasks {
		if id, _ := t["id"].(string); id == taskID {
			t["status"] = status
			found = true
			break
		}
	}

	if !found {
		tf.Tasks = append(tf.Tasks, map[string]interface{}{
			"id":       taskID,
			"status":   status,
			"story_id": storyID,
			"title":    "Healed Task " + taskID,
		})
	}

	newData, _ := json.MarshalIndent(tf, "", "  ")
	return os.WriteFile(path, newData, 0644)
}

func integrateStoryBranch(cmd *cobra.Command, projectDir string, storyID string, workerID int) {
	relPrefix := "release/"
	featPrefix := "feature/"
	baseBranch := "main"
	if projCfg, err := project.LoadProjectConfig(projectDir); err == nil {
		if projCfg.ReleaseBranchPrefix != "" { relPrefix = projCfg.ReleaseBranchPrefix }
		if projCfg.FeatureBranchPrefix != "" { featPrefix = projCfg.FeatureBranchPrefix }
		if projCfg.BaseBranch != "" { baseBranch = projCfg.BaseBranch }
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
	prompt := buildTaskPromptWithRetry(task, mgr, lastError)
	mcpPath, _ := ensureMCPConfig(projectDir)

	req := CreateLoopRequest{
		Prompt:        prompt,
		WorkDir:       projectDir,
		MaxIterations: runMaxIterations,
		TaskID:        task.ID,
		MCPConfigPath: mcpPath,
	}

	body, _ := json.Marshal(req)
	resp, err := http.Post(fmt.Sprintf("http://localhost:%d/api/v1/loops", startPort), "application/json", bytes.NewReader(body))
	if err != nil { return "", err }
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		respBody, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("server returned %d: %s", resp.StatusCode, string(respBody))
	}

	var loopResp LoopResponse
	_ = json.NewDecoder(resp.Body).Decode(&loopResp)
	return loopResp.ID, nil
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

func waitForLoop(cmd *cobra.Command, loopID string, prefix string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	lastIteration := -1
	lastProgressTime := time.Now()

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
							eventType, _ := payload["Type"].(string)
							iteration, _ := payload["Iteration"].(float64)
							
							if int(iteration) > lastIteration {
								lastIteration = int(iteration)
								lastProgressTime = time.Now()
							}
							
							if eventType == "complete" {
								// We'll let the main loop detect completion via polling or status check
								// to ensure consistency with the existing logic
							}
						}
					}
				}
			}()
		}
	}

	// Main monitoring loop (uses polling as secondary/heartbeat mechanism)
	for time.Now().Before(deadline) {
		resp, err := http.Get(fmt.Sprintf("http://localhost:%d/api/v1/loops/%s", startPort, loopID))
		if err != nil { 
			time.Sleep(2 * time.Second)
			continue 
		}
		var loopResp LoopResponse
		if err := json.NewDecoder(resp.Body).Decode(&loopResp); err != nil {
			resp.Body.Close()
			time.Sleep(2 * time.Second)
			continue
		}
		resp.Body.Close()
		
		if loopResp.Status == "complete" { return nil }
		if loopResp.Status == "error" { return fmt.Errorf("%s", loopResp.Error) }
		
		// HEARTBEAT MONITOR: Detect if runner is making progress
		if loopResp.Iteration > lastIteration {
			lastIteration = loopResp.Iteration
			lastProgressTime = time.Now()
		} else if time.Since(lastProgressTime) > 5*time.Minute {
			return fmt.Errorf("runner stalled: no iteration progress for 5 minutes")
		}

		if loopResp.Status == "paused" && strings.Contains(loopResp.Error, "Planning Mismatch") {
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
			return fmt.Errorf("%s", loopResp.Error)
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
		return fmt.Errorf("could not find tasks.json")
	}

	var data map[string]interface{}
	if err := json.Unmarshal(firstData, &data); err != nil {
		return err
	}

	updated := false
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

	if !updated {
		return fmt.Errorf("task %s not found", taskID)
	}

	newData, _ := json.MarshalIndent(data, "", "  ")
	for _, p := range paths {
		if _, err := os.Stat(p); err == nil {
			_ = os.WriteFile(p, newData, 0644)
		}
	}

	return nil
}

func KillDaemon(pid int) error {
	p, err := os.FindProcess(pid)
	if err != nil { return err }
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

	rootCmd.AddCommand(startCmd)
	rootCmd.AddCommand(runCmd)
	rootCmd.AddCommand(stopCmd)
}
