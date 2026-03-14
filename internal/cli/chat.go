package cli

import (
    "encoding/json"
    "fmt"
    "io"
    "net/http"
    "os"
    "strings"
    "time"

    "github.com/chzyer/readline"
    "github.com/fatih/color"
    "github.com/openexec/openexec/internal/project"
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

		// Check if server is running, if not, start it in background
		if !isServerRunning(projectDir, startPort) {
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
				return fmt.Errorf("engine failed to become ready on port %d: %w", startPort, err)
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

		return runChatREPL(cmd, projectName)
	},
}

func runChatREPL(cmd *cobra.Command, projectName string) error {
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

    // V5: Create a conversational session on start
    sessionID := fmt.Sprintf("session-%d", time.Now().Unix())
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

        // Conversational V5: Create a run for the message
        runReq := map[string]any{
            "session_id":     sessionID,
            "quickfix_title": line,
            "mode":           os.Getenv("OPENEXEC_MODE"),
        }
        body, _ := json.Marshal(runReq)
        resp, err := http.Post(fmt.Sprintf("http://localhost:%d/api/v1/runs", startPort), "application/json", strings.NewReader(string(body)))
        if err != nil {
            fmt.Printf("Error: %v\n", err)
            continue
        }
        
        var runResp struct {
            RunID string `json:"run_id"`
        }
        _ = json.NewDecoder(resp.Body).Decode(&runResp)
        resp.Body.Close()

        // Start the run
        http.Post(fmt.Sprintf("http://localhost:%d/api/v1/runs/%s/start", startPort, runResp.RunID), "application/json", nil)

        // Monitor the run
        fmt.Print(color.GreenString("Agent: "))
        _ = waitForLoop(cmd, runResp.RunID, "[Chat]", 5*time.Minute, false)
        fmt.Println()
    }

	return nil
}

func init() {
	chatCmd.Flags().IntVar(&startPort, "port", 8765, "Execution engine port")
	chatCmd.Flags().BoolVar(&chatDebug, "debug", false, "Show raw HTTP debug information")
	rootCmd.AddCommand(chatCmd)
}
