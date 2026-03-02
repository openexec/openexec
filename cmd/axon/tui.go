package main

import (
	"fmt"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/openexec/openexec/pkg/manager"
	"github.com/openexec/openexec/internal/pipeline"
	"github.com/openexec/openexec/internal/axon/tui"
	"github.com/spf13/cobra"
)

var (
	tuiPortFlag       int
	tuiHostFlag       string
	tuiWorkdirFlag    string
	tuiTractStoreFlag string
	tuiAgentsDirFlag  string
	tuiPipelineFlag   string
	tuiMaxIterFlag    int
	tuiMaxRetriesFlag int
	tuiMaxReviewFlag  int
)

func init() {
	tuiCmd.Flags().IntVar(&tuiPortFlag, "port", 0, "Connect to remote axon serve on this port (0 = embedded mode)")
	tuiCmd.Flags().StringVar(&tuiHostFlag, "host", "localhost", "Remote host (used with --port)")
	tuiCmd.Flags().StringVarP(&tuiWorkdirFlag, "workdir", "w", ".", "Working directory (embedded mode)")
	tuiCmd.Flags().StringVar(&tuiTractStoreFlag, "tract-store", "", "Tract store name (embedded mode, required unless --port is set)")
	tuiCmd.Flags().StringVar(&tuiAgentsDirFlag, "agents-dir", "./agents", "Agent definitions directory (embedded mode)")
	tuiCmd.Flags().StringVar(&tuiPipelineFlag, "pipeline", "default", "Pipeline configuration name (embedded mode)")
	tuiCmd.Flags().IntVar(&tuiMaxIterFlag, "max-iterations", 10, "Max iterations per phase (embedded mode)")
	tuiCmd.Flags().IntVar(&tuiMaxRetriesFlag, "max-retries", 3, "Retry attempts on crash (embedded mode)")
	tuiCmd.Flags().IntVar(&tuiMaxReviewFlag, "max-review-cycles", 3, "Max IM↔RV review cycles (embedded mode)")

	rootCmd.AddCommand(tuiCmd)
}

var tuiCmd = &cobra.Command{
	Use:   "tui",
	Short: "Launch the Axon TUI dashboard",
	Long: `Launches an interactive terminal dashboard for observing and controlling
FWU pipelines.

Two modes:
  Remote:   axon tui --port 8080
            Connects to a running axon serve instance via HTTP API + SSE.

  Embedded: axon tui --workdir /path/to/repo --tract-store myproject
            Starts a Manager internally. Pipelines are started via the TUI.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		var source axontui.Source

		if tuiPortFlag > 0 {
			// Remote mode: connect to axon serve.
			source = axontui.NewRemoteSource(tuiHostFlag, tuiPortFlag)
		} else {
			// Embedded mode: start a Manager.
			if tuiTractStoreFlag == "" {
				return fmt.Errorf("--tract-store is required in embedded mode (or use --port for remote mode)")
			}
			pipelineDef, err := pipeline.LoadPipelineConfig(tuiAgentsDirFlag, tuiPipelineFlag)
			if err != nil {
				return fmt.Errorf("load pipeline config: %w", err)
			}
			cfg := manager.Config{
				WorkDir:              tuiWorkdirFlag,
				TractStore:           tuiTractStoreFlag,
				AgentsDir:            tuiAgentsDirFlag,
				Pipeline:             pipelineDef,
				DefaultMaxIterations: tuiMaxIterFlag,
				MaxRetries:           tuiMaxRetriesFlag,
				MaxReviewCycles:      tuiMaxReviewFlag,
				RetryBackoff:         []time.Duration{0, 5 * time.Second, 15 * time.Second},
				ThrashThreshold:      3,
			}
			mgr := manager.New(cfg)
			source = axontui.NewEmbeddedSource(mgr)
		}

		app := axontui.NewApp(source)
		p := tea.NewProgram(app, tea.WithAltScreen())
		_, err := p.Run()
		return err
	},
}
