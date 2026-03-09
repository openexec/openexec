package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/chzyer/readline"
	"github.com/fatih/color"
	"github.com/openexec/openexec/internal/project"
	"github.com/spf13/cobra"
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

		return runChatREPL(projectName)
	},
}

func runChatREPL(projectName string) error {
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

		// Send query to server
		response, err := sendChatQuery(line)
		if err != nil {
			fmt.Printf(color.RedString("Error: %v\n"), err)
			continue
		}

		fmt.Println()
		fmt.Print(color.GreenString("Agent: "))
		fmt.Println(response)
		fmt.Println()
	}

	return nil
}

func sendChatQuery(query string) (string, error) {
	url := fmt.Sprintf("http://localhost:%d/api/v1/dcp/query", startPort)
	
	payload := map[string]string{"query": query}
	body, _ := json.Marshal(payload)

	resp, err := http.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("failed to connect to server: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		var errData struct {
			Error string `json:"error"`
		}
		json.NewDecoder(resp.Body).Decode(&errData)
		if errData.Error != "" {
			return "", fmt.Errorf("server error: %s", errData.Error)
		}
		return "", fmt.Errorf("server returned status %d", resp.StatusCode)
	}

	var result struct {
		Response string `json:"response"`
		Result   string `json:"result"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("failed to decode response: %w", err)
	}

	// Server returns "result", but we'll check both for safety
	if result.Result != "" {
		return result.Result, nil
	}
	return result.Response, nil
}

func init() {
	chatCmd.Flags().IntVar(&startPort, "port", 8765, "Execution engine port")
	rootCmd.AddCommand(chatCmd)
}
