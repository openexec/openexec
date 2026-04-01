package cli

import (
	"bufio"
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
	"syscall"
	"time"

	"github.com/fatih/color"
	"github.com/gorilla/websocket"
	"github.com/openexec/openexec/internal/planner"
	"github.com/openexec/openexec/internal/project"
	"github.com/openexec/openexec/internal/release"
	"github.com/openexec/openexec/internal/runner"
	"github.com/openexec/openexec/internal/server"
	"github.com/spf13/cobra"
)

// Legacy FWU flow removed in Phase Four. All orchestration is daemon-owned.

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

	// Blueprint command flags
	blueprintID      string
	blueprintClarify bool
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
	Long: `Start the execution daemon that handles all orchestration.

The daemon exposes /api/v1/runs endpoints for run management:
  POST /api/v1/runs        Create a new run
  POST /api/v1/runs:plan   Generate a plan from INTENT.md
  POST /api/v1/runs:execute Execute pending tasks
  GET  /api/v1/runs        List runs (supports ?limit=&offset= paging)
  GET  /api/v1/runs/{id}   Get run status

WebSocket events are available at /ws for real-time monitoring.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Check for updates in background
		go checkForUpdate()

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

		dataDir := filepath.Join(config.ProjectDir, ".openexec", "data")
		auditDB := filepath.Join(config.ProjectDir, ".openexec", "openexec.db")

		finalPort, err := findAvailablePort(startPort)
		if err != nil {
			return err
		}
		if finalPort != startPort {
			cmd.Printf("   ⚠ Port %d is busy, using %d instead\n", startPort, finalPort)
			startPort = finalPort
		}

		serverArgs := []string{"start", "--port", fmt.Sprintf("%d", startPort)}

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
            UnifiedDB:   auditDB,
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
	Short: "Execute tasks using the execution engine (daemon-orchestrated)",
	Long: `Execute tasks using the execution engine.

All orchestration is handled by the daemon via /api/v1/runs endpoints.
The CLI is a thin client that triggers and monitors server-side execution.

Examples:
  openexec run             # Execute all pending tasks
  openexec run T-001       # Execute specific task
  openexec run --quickfix "Fix typo" --verify "make test"`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Check for updates in background
		go checkForUpdate()

		config, err := project.LoadProjectConfig(".")
		if err != nil {
			return fmt.Errorf("project not initialized: run 'openexec init' first")
		}

		// Use config port if not explicitly set via flag
		if !cmd.Flags().Changed("port") && config.Execution.Port > 0 {
			startPort = config.Execution.Port
		}

		// 1. AUTO-START ENGINE
		if !isServerRunning(config.ProjectDir, 0) { // Check if ANY engine is running for this project
			// Preflight: verify the configured runner CLI is available
			if err := preflightRunnerCheck(config); err != nil {
				return err
			}

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
			hint := daemonDiagnostic(config.ProjectDir)
			if hint != "" {
				return fmt.Errorf("engine failed to become ready on port %d: %w\n\n  Daemon log (last lines):\n  %s", startPort, err, strings.ReplaceAll(hint, "\n", "\n  "))
			}
			return fmt.Errorf("engine failed to become ready on port %d: %w\n\n  Check .openexec/daemon.log for details", startPort, err)
		}

		// 2. TRIGGER SERVER-SIDE EXECUTION
		// Thin client: CLI only initiates and monitors.
		
		// Plan first if needed
		if !runNoAutoPlan {
			planReq := map[string]any{
				"intent_file": "INTENT.md",
				"auto_import": true,
			}
			body, _ := json.Marshal(planReq)
			_, err := http.Post(fmt.Sprintf("http://localhost:%d/api/v1/runs:plan", startPort), "application/json", bytes.NewReader(body))
			if err != nil {
				return fmt.Errorf("auto-planning failed: %w", err)
			}
		}

		// Execute all pending tasks
		runOpts := map[string]any{
			"worker_count": config.Execution.WorkerCount,
			"mode":         runMode,
		}
		if len(args) > 0 {
			runOpts["task_ids"] = []string{args[0]}
		}
		
		body, _ := json.Marshal(runOpts)
		resp, err := http.Post(fmt.Sprintf("http://localhost:%d/api/v1/runs:execute", startPort), "application/json", bytes.NewReader(body))
		if err != nil {
			return fmt.Errorf("execution trigger failed: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusAccepted {
			return fmt.Errorf("server rejected execution request: %d", resp.StatusCode)
		}

		cmd.Println("📋 Execution orchestrated by daemon.")
		cmd.Println("   Monitor: openexec status | openexec tui | http://localhost:" + fmt.Sprintf("%d", startPort))
		return nil
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
		pid, port, err := readPID(config.ProjectDir)
		if err != nil {
			cmd.Println("Daemon is not running (no PID file found)")
			return nil
		}

		// Check if process is actually running
		process, err := os.FindProcess(pid)
		if err != nil || process.Signal(syscall.Signal(0)) != nil {
			cmd.Println("Daemon is not running (stale PID file)")
			_ = removePIDFile(config.ProjectDir)
			return nil
		}

		cmd.Printf("Stopping daemon (PID %d, port %d)...\n", pid, port)
		if err := KillDaemon(pid); err != nil {
			return fmt.Errorf("failed to stop daemon: %w", err)
		}

		// Wait briefly for process to exit
		time.Sleep(500 * time.Millisecond)

		// Clean up PID file
		_ = removePIDFile(config.ProjectDir)
		cmd.Println("Daemon stopped")
		return nil
	},
}

var restartCmd = &cobra.Command{
	Use:   "restart",
	Short: "Restart execution engine",
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := stopCmd.RunE(cmd, args); err != nil {
			// Ignore stop errors (daemon may not be running)
			_ = err
		}
		return startCmd.RunE(cmd, args)
	},
}

var blueprintCmd = &cobra.Command{
	Use:   "blueprint <task-description>",
	Short: "Execute a task using blueprint-based orchestration",
	Long: `Execute a task using blueprint-based stage orchestration.

Blueprints define sequences of deterministic and agentic stages:
  gather_context → implement → lint → test → review

Available blueprints:
  - standard_task: Full workflow with lint/test validation (default)
  - quick_fix:     Simplified workflow for small fixes

Examples:
  openexec blueprint "Add user authentication"
  openexec blueprint --blueprint-id quick_fix "Fix typo in README"`,
	Args: cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		taskDescription := strings.Join(args, " ")

		config, err := project.LoadProjectConfig(".")
		if err != nil {
			return fmt.Errorf("project not initialized: run 'openexec init' first")
		}

		// Handle interactive clarification if requested
		if blueprintClarify {
			clarified, err := runMiniInterview(cmd, taskDescription, config)
			if err != nil {
				return err
			}
			if clarified == "" {
				return nil // User cancelled
			}
			taskDescription = clarified
		}

		// Use config port if not explicitly set via flag
		if !cmd.Flags().Changed("port") && config.Execution.Port > 0 {
			startPort = config.Execution.Port
		}

		// Auto-start engine if not running
		if !isServerRunning(config.ProjectDir, 0) {
			if err := preflightRunnerCheck(config); err != nil {
				return err
			}

			cmd.Println("⚙️ Execution engine not running. Starting daemon...")
			startArgs := []string{"start", "--daemon", "--port", fmt.Sprintf("%d", startPort)}
			execPath, _ := os.Executable()
			startExec := exec.Command(execPath, startArgs...)
			startExec.Dir = config.ProjectDir
			if err := startExec.Run(); err != nil {
				return fmt.Errorf("failed to auto-start execution engine: %w", err)
			}
			time.Sleep(1 * time.Second)
		}

		// Sync port from PID file
		if _, effectivePort, err := readPID(config.ProjectDir); err == nil {
			if effectivePort != startPort {
				cmd.Printf("   ℹ️ Syncing with engine on port %d\n", effectivePort)
				startPort = effectivePort
			}
		}

		if err := waitForServer(startPort, 15*time.Second); err != nil {
			hint := daemonDiagnostic(config.ProjectDir)
			if hint != "" {
				return fmt.Errorf("engine failed to become ready on port %d: %w\n\n  Daemon log (last lines):\n  %s", startPort, err, strings.ReplaceAll(hint, "\n", "\n  "))
			}
			return fmt.Errorf("engine failed to become ready on port %d: %w\n\n  Check .openexec/daemon.log for details", startPort, err)
		}

		// Trigger blueprint run via API
		// Blueprint runs use "run" mode for full automation
		blueprintRunMode := runMode
		if blueprintRunMode == "" || blueprintRunMode == "workspace-write" {
			// Map execution permission mode to operational mode
			// Default to "run" mode for blueprint execution
			blueprintRunMode = "run"
		}
		payload := map[string]any{
			"blueprint_id":     blueprintID,
			"task_description": taskDescription,
			"mode":             blueprintRunMode,
		}
		body, _ := json.Marshal(payload)

		resp, err := http.Post(
			fmt.Sprintf("http://localhost:%d/api/v1/runs:blueprint", startPort),
			"application/json",
			bytes.NewReader(body),
		)
		if err != nil {
			return fmt.Errorf("failed to start blueprint run: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusCreated {
			respBody, _ := io.ReadAll(resp.Body)
			return fmt.Errorf("server rejected blueprint run: %s", string(respBody))
		}

		var result map[string]interface{}
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			return fmt.Errorf("failed to decode response: %w", err)
		}

		runID, _ := result["run_id"].(string)
		bpID, _ := result["blueprint_id"].(string)

		cmd.Printf("🎯 Blueprint run started\n")
		cmd.Printf("   Run ID:    %s\n", runID)
		cmd.Printf("   Blueprint: %s\n", bpID)
		cmd.Printf("   Task:      %s\n", taskDescription)
		cmd.Println()
		cmd.Printf("Monitor progress with: openexec status %s\n", runID)
		cmd.Printf("Or view timeline:      curl http://localhost:%d/api/v1/runs/%s/timeline\n", startPort, runID)

		return nil
	},
}

// isStudyTask returns true if the task title indicates a study/research task.
func isStudyTask(title string) bool {
	lower := strings.ToLower(title)
	return strings.Contains(lower, "study") || strings.Contains(lower, "research") || strings.Contains(lower, "investigate")
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

// preflightRunnerCheck validates that the configured executor CLI is available
// before attempting to start the daemon. Returns a user-friendly error with
// install guidance if the runner is missing.
func preflightRunnerCheck(config *project.ProjectConfig) error {
	model := config.Execution.ExecutorModel
	overrideCmd := config.Execution.RunnerCommand
	overrideArgs := config.Execution.RunnerArgs

	_, _, err := runner.Resolve(model, overrideCmd, overrideArgs)
	if err != nil {
		// Build a helpful error message
		cliName := "claude"
		if overrideCmd != "" {
			cliName = overrideCmd
		} else if model != "" {
			m := strings.ToLower(model)
			switch {
			case strings.HasPrefix(m, "gpt-") || strings.Contains(m, "codex") || strings.Contains(m, "openai"):
				cliName = "codex"
			case strings.HasPrefix(m, "gemini"):
				cliName = "gemini"
			}
		}

		modelDisplay := model
		if modelDisplay == "" {
			modelDisplay = "default (claude)"
		}

		return fmt.Errorf("%q CLI not found on PATH (configured model: %s)\n\n"+
			"  Supported runners:\n"+
			"    claude  — Claude (Anthropic)    npm i -g @anthropic-ai/claude-code\n"+
			"    codex   — Codex (OpenAI)        npm i -g @openai/codex\n"+
			"    gemini  — Gemini (Google)        npm i -g @google/gemini-cli\n\n"+
			"  Or reconfigure:  openexec init --force",
			cliName, modelDisplay)
	}
	return nil
}

// daemonDiagnostic reads the last lines of daemon.log to surface errors
// that caused the background engine to crash before becoming ready.
func daemonDiagnostic(projectDir string) string {
	logPath := filepath.Join(projectDir, ".openexec", "daemon.log")
	data, err := os.ReadFile(logPath)
	if err != nil {
		return ""
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	// Return last 5 lines (or fewer)
	start := len(lines) - 5
	if start < 0 {
		start = 0
	}
	return strings.Join(lines[start:], "\n")
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

// createExecutionLoopWithRetry triggers a task execution via daemon using /api/v1/runs endpoints.
func createExecutionLoopWithRetry(projectDir string, task Task, mgr *release.Manager, lastError string) (string, error) {
	isStudy := isStudyTask(task.Title)

	// Determine operational mode - default to "run" for task execution
	execMode := runMode
	if execMode == "" {
		execMode = "run"
	}

	payload := map[string]any{
		"mode": execMode,
	}
	if isStudy {
		payload["is_study"] = true
	}
	body, _ := json.Marshal(payload)

	endpoint := fmt.Sprintf("http://localhost:%d/api/v1/runs/%s/start", startPort, task.ID)
	resp, err := http.Post(endpoint, "application/json", bytes.NewReader(body))
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

// waitForLoop monitors a run via /api/v1/runs/{id} until completion or timeout.
// Uses WebSocket events for real-time updates with HTTP polling fallback.
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

	// Main monitoring loop - uses v1 runs endpoint
	for time.Now().Before(deadline) {
		resp, err := http.Get(fmt.Sprintf("http://localhost:%d/api/v1/runs/%s", startPort, loopID))
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
	startCmd.Flags().StringVar(&startReviewer, "reviewer", "", "Model for code review")
	startCmd.AddCommand(stopCmd)
	startCmd.AddCommand(restartCmd)

	runCmd.Flags().IntVar(&startPort, "port", 8765, "Execution engine port")
	runCmd.Flags().IntVar(&runMaxIterations, "max-iterations", 10, "Max iterations")
    runCmd.Flags().IntVar(&runTimeout, "timeout", 1800, "Timeout")
    runCmd.Flags().BoolVarP(&runVerbose, "verbose", "v", false, "Verbose logs")
    runCmd.Flags().BoolVar(&runNoAutoPlan, "no-auto-plan", false, "Disable automatic planning")
    runCmd.Flags().StringVar(&runQuickfix, "quickfix", "", "Execute a single deterministic quickfix without planning (task title)")
    runCmd.Flags().StringVar(&runVerify, "verify", "", "Verification script for --quickfix (defaults to echo quickfix-verify)")
    runCmd.Flags().StringVar(&runMode, "mode", "danger-full-access", "Execution mode: read-only | workspace-write | danger-full-access")

	blueprintCmd.Flags().IntVar(&startPort, "port", 8765, "Execution engine port")
	blueprintCmd.Flags().StringVar(&blueprintID, "blueprint-id", "standard_task", "Blueprint to execute (standard_task, quick_fix)")
	blueprintCmd.Flags().BoolVar(&blueprintClarify, "clarify", false, "Start interactive clarification interview before execution")
	blueprintCmd.Flags().StringVar(&runMode, "mode", "danger-full-access", "Execution mode: read-only | workspace-write | danger-full-access")

	rootCmd.AddCommand(startCmd)
	rootCmd.AddCommand(runCmd)
	rootCmd.AddCommand(stopCmd)
	rootCmd.AddCommand(blueprintCmd)
}

func runMiniInterview(cmd *cobra.Command, initialIntent string, config *project.ProjectConfig) (string, error) {
	model := config.Execution.PlannerModel
	if model == "" {
		model = config.Execution.ExecutorModel
	}
	if model == "" {
		model = "sonnet"
	}

	cmd.Println(color.CyanString("\n=== Task Clarification Interview ==="))
	cmd.Printf("   Model: %s\n", model)
	cmd.Println("   (Type 'done' to start execution, or 'exit' to cancel)")
	cmd.Println()

	reader := bufio.NewReader(os.Stdin)
	p := planner.New(&cliLLMProvider{model: model, cmd: cmd})
	stateJSON := "{}"

	// Initial message
	message := initialIntent
	
	for {
		if message != "" {
			cmd.Print(color.CyanString("Thinking... "))
			resp, err := p.ProcessWizardMessage(cmd.Context(), message, stateJSON)
			cmd.Print("\r") // Clear Thinking line
			if err != nil {
				return "", err
			}

			// Update state
			stateBytes, _ := json.Marshal(resp.UpdatedState)
			stateJSON = string(stateBytes)

			if resp.Acknowledgement != "" {
				cmd.Println(color.BlueString("🤖 %s", resp.Acknowledgement))
			}

			if resp.IsComplete {
				cmd.Println(color.GreenString("\n✔ Intent clarified."))
				return initialIntent + "\n\nContext:\n" + resp.Acknowledgement, nil
			}

			cmd.Println(color.GreenString("\n? %s", resp.NextQuestion))
		}

		cmd.Print(color.GreenString("> "))
		input, err := reader.ReadString('\n')
		if err != nil {
			return "", err
		}

		message = strings.TrimSpace(input)
		if message == "exit" || message == "quit" {
			return "", nil
		}
		if message == "done" {
			return initialIntent, nil
		}
		if message == "" {
			continue
		}
	}
}

// tryAutoImportStories attempts to import stories from .openexec/stories.json into the DB
//
// JSON IMPORT GUARD:
// This function performs a ONE-TIME IMPORT from JSON to SQLite.
// It is ONLY called when SQLite has no stories (legacy project migration).
// After import, JSON is NEVER read again - SQLite is the canonical source.
//
// This is NOT a runtime data source - it's a migration path for legacy projects.
// Returns true if an import was performed (regardless of success), false otherwise.
func tryAutoImportStories(projectDir string, mgr *release.Manager, cmd *cobra.Command) bool {
    storiesPath := filepath.Join(projectDir, ".openexec", "stories.json")
    data, err := os.ReadFile(storiesPath)
    if err != nil { return false }

    // Log import operation for drift tracking
    fmt.Fprintf(os.Stderr, "[IMPORT] Loading from JSON (%s) for one-time migration. SQLite is the canonical store.\n", storiesPath)
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
