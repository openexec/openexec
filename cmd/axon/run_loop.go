package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"sync/atomic"
	"syscall"

	"github.com/openexec/openexec/internal/loop"
	"github.com/openexec/openexec/internal/prompt"
	"github.com/openexec/openexec/internal/tract"
	"github.com/spf13/cobra"
)

var (
	promptFlag        string
	workdirFlag       string
	maxIterationsFlag int
	maxRetriesFlag    int
	fwuFlag           string
	agentFlag         string
	workflowFlag      string
	tractStoreFlag    string
	agentsDirFlag     string
)

func init() {
	runLoopCmd.Flags().StringVarP(&promptFlag, "prompt", "p", "", "System prompt for Claude Code")
	runLoopCmd.Flags().StringVarP(&workdirFlag, "workdir", "w", ".", "Working directory for Claude Code")
	runLoopCmd.Flags().IntVar(&maxIterationsFlag, "max-iterations", 10, "Maximum loop iterations (0 = unlimited)")
	runLoopCmd.Flags().IntVar(&maxRetriesFlag, "max-retries", 3, "Retry attempts on crash")
	runLoopCmd.Flags().StringVar(&fwuFlag, "fwu", "", "FWU ID for context-driven execution")
	runLoopCmd.Flags().StringVar(&agentFlag, "agent", "", "Agent name (required with --fwu)")
	runLoopCmd.Flags().StringVar(&workflowFlag, "workflow", "", "Workflow prompt ID (required with --fwu)")
	runLoopCmd.Flags().StringVar(&tractStoreFlag, "tract-store", "", "Tract store name (required with --fwu)")
	runLoopCmd.Flags().StringVar(&agentsDirFlag, "agents-dir", "./agents", "Directory containing agent definitions")

	rootCmd.AddCommand(runLoopCmd)
}

var runLoopCmd = &cobra.Command{
	Use:   "run-loop",
	Short: "Run a single Loop iteration executor",
	Long: `Spawns Claude Code in a loop, parsing stream-JSON output into typed events.
Events are printed as JSON lines to stdout. The loop runs until max iterations,
crash (retries exhausted), or OS signal.

Signal handling:
  First SIGINT  → pause (finish current iteration, then exit)
  Second SIGINT → stop (kill process immediately)
  SIGTERM       → stop (kill process immediately)`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Validate mutually exclusive flags.
		if promptFlag != "" && fwuFlag != "" {
			return fmt.Errorf("--prompt and --fwu are mutually exclusive")
		}
		if promptFlag == "" && fwuFlag == "" {
			return fmt.Errorf("either --prompt or --fwu is required")
		}
		if fwuFlag != "" {
			if agentFlag == "" || workflowFlag == "" || tractStoreFlag == "" {
				return fmt.Errorf("--agent, --workflow, and --tract-store are required with --fwu")
			}
		}

		defaults := loop.DefaultConfig()

		if fwuFlag != "" {
			// FWU mode: fetch briefing, compose prompt, configure MCP servers.

			// 1. Fetch briefing from Tract.
			tractClient, err := tract.StartSubprocess(cmd.Context(), tractStoreFlag)
			if err != nil {
				return fmt.Errorf("start tract: %w", err)
			}
			brief, err := tractClient.Brief(fwuFlag)
			_ = tractClient.Close()
			if err != nil {
				return fmt.Errorf("fetch briefing: %w", err)
			}

			// 2. Format briefing.
			briefingText := prompt.FormatBriefing(brief)

			// 3. Compose prompt.
			assembler := prompt.NewAssembler(agentsDirFlag)
			composedPrompt, err := assembler.Compose(agentFlag, workflowFlag, briefingText)
			if err != nil {
				return fmt.Errorf("compose prompt: %w", err)
			}

			// 4. Build MCP config (axon-signal + tract).
			axonBin, _ := os.Executable()
			servers := loop.BuildMCPServers(axonBin, tractStoreFlag)
			mcpPath, err := loop.WriteMCPConfig(servers)
			if err != nil {
				return fmt.Errorf("write MCP config: %w", err)
			}
			defer func() { _ = os.Remove(mcpPath) }()

			cfg := loop.Config{
				Prompt:          composedPrompt,
				WorkDir:         workdirFlag,
				MaxIterations:   maxIterationsFlag,
				MaxRetries:      maxRetriesFlag,
				RetryBackoff:    defaults.RetryBackoff,
				MCPConfigPath:   mcpPath,
				ThrashThreshold: defaults.ThrashThreshold,
			}

			l, events := loop.New(cfg)
			go func() {
				enc := json.NewEncoder(os.Stdout)
				for event := range events {
					if err := enc.Encode(event); err != nil {
						fmt.Fprintf(os.Stderr, "axon: encode event: %v\n", err)
					}
				}
			}()
			go handleSignals(l)
			return l.Run(cmd.Context())
		}

		// Legacy mode: use prompt directly.
		axonBin, _ := os.Executable()
		servers := loop.BuildMCPServers(axonBin, "")
		mcpPath, err := loop.WriteMCPConfig(servers)
		if err != nil {
			return fmt.Errorf("write MCP config: %w", err)
		}
		defer func() { _ = os.Remove(mcpPath) }()

		cfg := loop.Config{
			Prompt:          promptFlag,
			WorkDir:         workdirFlag,
			MaxIterations:   maxIterationsFlag,
			MaxRetries:      maxRetriesFlag,
			RetryBackoff:    defaults.RetryBackoff,
			MCPConfigPath:   mcpPath,
			ThrashThreshold: defaults.ThrashThreshold,
		}

		l, events := loop.New(cfg)

		// Print events as JSON lines to stdout.
		go func() {
			enc := json.NewEncoder(os.Stdout)
			for event := range events {
				if err := enc.Encode(event); err != nil {
					fmt.Fprintf(os.Stderr, "axon: encode event: %v\n", err)
				}
			}
		}()

		// Handle OS signals.
		go handleSignals(l)

		return l.Run(cmd.Context())
	},
}

func handleSignals(l *loop.Loop) {
	sigCh := make(chan os.Signal, 2)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	var sigintCount atomic.Int32

	for sig := range sigCh {
		switch sig {
		case syscall.SIGINT:
			n := sigintCount.Add(1)
			if n == 1 {
				fmt.Fprintln(os.Stderr, "\naxon: received SIGINT, pausing after current iteration...")
				l.Pause()
			} else {
				fmt.Fprintln(os.Stderr, "\naxon: received second SIGINT, stopping immediately...")
				l.Stop()
				return
			}
		case syscall.SIGTERM:
			fmt.Fprintln(os.Stderr, "\naxon: received SIGTERM, stopping immediately...")
			l.Stop()
			return
		}
	}
}
