package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/chzyer/readline"
	"github.com/fatih/color"
	"github.com/openexec/openexec/internal/project"
	"github.com/openexec/openexec/internal/release"
	"github.com/spf13/cobra"
)

var (
	chatDebug bool
)

var chatCmd = &cobra.Command{
	Use:   "chat",
	Short: "Start an interactive conversational session",
	Long: `Start a real-time conversational session with the OpenExec agent.
Talk to your project, ask questions about the codebase, or trigger automated tasks.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Check for updates in background
		go checkForUpdate()

		// Try to load project configuration
		config, err := project.LoadProjectConfig(".")
		projectName := "global"
		projectDir := "."

		if err == nil {
			projectName = config.Name
			projectDir = config.ProjectDir

			// Apply config port if not overridden
			if !cmd.Flags().Changed("port") && config.Execution.Port > 0 {
				startPort = config.Execution.Port
			}
		}

		// Phase-aware guidance: once the initial heavy build is complete,
		// chat (which boots the full engine) is usually the wrong lane for
		// small tasks — say so before paying the daemon startup cost.
		printPhaseHint(projectDir)

		// Check if server is running, if not, start it in background
		if !isServerRunning(projectDir, startPort) {
			// Preflight: verify the configured runner CLI is available
			if config != nil {
				if err := preflightRunnerCheck(config); err != nil {
					return err
				}
			}

			fmt.Println(color.CyanString("🚀 Starting execution engine in background..."))

			// Set daemon flag and run start command
			startDaemon = true
			if err := startCmd.RunE(cmd, args); err != nil {
				return fmt.Errorf("failed to start background engine: %w", err)
			}

			// Re-read the port from PID file because it might have shifted (discovery)
			_, actualPort, err := readPID(projectDir)
			if err == nil {
				startPort = actualPort
			}

			// Wait for server to be ready on the discovered port
			fmt.Printf("⏳ Waiting for engine to initialize (port %d)...", startPort)
			if err := waitForServer(startPort, 15*time.Second); err != nil {
				fmt.Println(color.RedString(" ✗ Failed!"))
				hint := daemonDiagnostic(projectDir)
				if hint != "" {
					return fmt.Errorf("engine failed to become ready on port %d: %w\n\n  Daemon log (last lines):\n  %s", startPort, err, strings.ReplaceAll(hint, "\n", "\n  "))
				}
				return fmt.Errorf("engine failed to become ready on port %d: %w\n\n  Check .openexec/daemon.log for details", startPort, err)
			}
			fmt.Println(color.GreenString(" ✓ Ready"))
			fmt.Println()
		} else {
			// Even if it was already running, discover the correct port from PID file
			_, actualPort, err := readPID(projectDir)
			if err == nil {
				startPort = actualPort
			}
		}

		return runChatREPL(cmd, projectName, projectDir)
	},
}

// printPhaseHint inspects the backlog (cheap, read-only — no daemon) and, when
// the initial build is already complete, recommends the lightweight lane.
// Errors are deliberately swallowed: this is guidance, never a gate.
func printPhaseHint(projectDir string) {
	if _, err := os.Stat(filepath.Join(projectDir, ".openexec")); err != nil {
		return
	}
	mgr, err := release.NewManager(projectDir, nil)
	if err != nil {
		return
	}
	defer func() { _ = mgr.Close() }()
	if err := mgr.Load(); err != nil {
		return
	}
	if mgr.Phase() == release.PhaseMaintaining {
		fmt.Println(color.YellowString("💡 Initial build is complete (all stories done)."))
		fmt.Println(color.YellowString("   For quick tasks and questions, light mode is faster: connect Claude Code"))
		fmt.Println(color.YellowString("   via `openexec mcp-serve` (see docs/LIGHT_MODE.md) instead of booting the engine."))
		fmt.Println()
	}
}

func runChatREPL(cmd *cobra.Command, projectName, projectDir string) error {
	l, err := readline.NewEx(&readline.Config{
		Prompt:          color.CyanString("openexec(%s) > ", projectName),
		HistoryFile:     "/tmp/openexec-chat.tmp",
		InterruptPrompt: "^C",
		EOFPrompt:       "exit",
	})
	if err != nil {
		return err
	}
	defer l.Close()

	fmt.Println(color.HiBlueString("Welcome to OpenExec Conversational Mode"))
	fmt.Println("Type your intent or 'exit' to quit.")
	fmt.Println()

	// Create session via daemon API (daemon-owned orchestration)
	sessionID, err := createChatSession(startPort, projectName)
	if err != nil {
		fmt.Println(color.YellowString("   ⚠️ The engine did not create a chat session (%v); requests will be refused.", err))
	}
	fmt.Printf("   ℹ️ Session: %s\n\n", sessionID)

	for {
		line, err := l.Readline()
		if err == readline.ErrInterrupt {
			if len(line) == 0 {
				break
			} else {
				continue
			}
		} else if err == io.EOF {
			break
		}

		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if line == "exit" || line == "quit" {
			break
		}

		// Escalate the line through the session's run route. A refusal is
		// shown and nothing runs, so there is no run to wait on.
		runID, err := escalateChatLine(startPort, projectDir, sessionID, line)
		if err != nil {
			fmt.Println(color.RedString("Error: %v", err))
			continue
		}

		// Monitor the run
		fmt.Print(color.GreenString("Agent: "))
		_ = waitForLoop(cmd, runID, "[Chat]", 5*time.Minute, false)
		fmt.Println()
	}

	return nil
}

// createChatSession creates the chat session through the daemon API. When the
// daemon refuses, it returns a local ID, which the run route then refuses,
// together with the reason.
func createChatSession(port int, projectName string) (string, error) {
	// Sessions start in chat mode by default
	sessionReq := map[string]any{
		"projectPath": projectName,
		"provider":    os.Getenv("OPENEXEC_PROVIDER"),
		"model":       os.Getenv("OPENEXEC_MODEL"),
		"title":       fmt.Sprintf("Chat %s", time.Now().Format("2006-01-02 15:04")),
		"mode":        "chat",
	}
	sessionBody, _ := json.Marshal(sessionReq)
	sessionResp, err := http.Post(fmt.Sprintf("http://localhost:%d/api/v1/sessions", port), "application/json", bytes.NewReader(sessionBody))
	if err == nil {
		defer sessionResp.Body.Close()
		var resp struct {
			ID    string `json:"id"`
			Error string `json:"error"`
		}
		_ = json.NewDecoder(sessionResp.Body).Decode(&resp)
		if sessionResp.StatusCode == http.StatusCreated && resp.ID != "" {
			return resp.ID, nil
		}
		err = fmt.Errorf("HTTP %d: %s", sessionResp.StatusCode, resp.Error)
	}
	// Fallback to local ID if daemon session creation fails
	return fmt.Sprintf("session-%d", time.Now().Unix()), err
}

// chatExecMode resolves the execution mode a chat escalation runs with:
// OPENEXEC_MODE when set, otherwise the project's configured mode. It is
// always sent explicitly, so the run route's broad fallback never applies.
// Anything that is not an execution mode is refused before a run exists.
func chatExecMode(projectDir string) (string, error) {
	mode, source := os.Getenv("OPENEXEC_MODE"), "OPENEXEC_MODE"
	if mode == "" {
		cfg, err := project.LoadProjectConfig(projectDir)
		if err != nil {
			return "", fmt.Errorf("OPENEXEC_MODE is unset and the project execution mode is unavailable: %w", err)
		}
		mode, source = cfg.Execution.EffectiveExecMode(), "project exec_mode"
	}
	switch mode {
	case "read-only", "workspace-write", "danger-full-access":
		return mode, nil
	}
	return "", fmt.Errorf("%s %q is not an execution mode (valid: read-only, workspace-write, danger-full-access); no run was started", source, mode)
}

// escalateChatLine starts one run for a chat line through
// POST /api/v1/sessions/{id}/run, with the line as the task description and
// the resolved execution mode. It returns the started run's ID.
func escalateChatLine(port int, projectDir, sessionID, line string) (string, error) {
	if strings.TrimSpace(line) == "" {
		return "", fmt.Errorf("empty request; no run was started")
	}
	mode, err := chatExecMode(projectDir)
	if err != nil {
		return "", err
	}
	body, _ := json.Marshal(map[string]string{"task_description": line, "mode": mode})
	resp, err := http.Post(fmt.Sprintf("http://localhost:%d/api/v1/sessions/%s/run", port, url.PathEscape(sessionID)), "application/json", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	var out struct {
		RunID string `json:"run_id"`
		Error string `json:"error"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&out)
	if resp.StatusCode != http.StatusCreated {
		if out.Error == "" {
			out.Error = http.StatusText(resp.StatusCode)
		}
		return "", fmt.Errorf("run not started (HTTP %d): %s", resp.StatusCode, out.Error)
	}
	if out.RunID == "" {
		return "", fmt.Errorf("run route returned no run_id")
	}
	return out.RunID, nil
}

func init() {
	chatCmd.Flags().IntVar(&startPort, "port", 8765, "Execution engine port")
	chatCmd.Flags().BoolVar(&chatDebug, "debug", false, "Show raw HTTP debug information")
	rootCmd.AddCommand(chatCmd)
}
