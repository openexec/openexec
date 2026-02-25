package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/openexec/openexec/internal/pipeline"
	"github.com/spf13/cobra"
)

var (
	pipelineFWUFlag           string
	pipelineWorkdirFlag       string
	pipelineTractStoreFlag    string
	pipelineAgentsDirFlag     string
	pipelineBriefingFileFlag  string
	pipelineNameFlag          string
	pipelineMaxIterationsFlag int
	pipelineMaxRetriesFlag    int
	pipelineMaxReviewFlag     int

	// Evidence flags
	pipelineEvidenceDir      string
	pipelineEvidenceBucket   string
	pipelineEvidenceRegion   string
	pipelineEvidenceEndpoint string
	pipelineEvidencePrefix   string
)

func init() {
	runPipelineCmd.Flags().StringVar(&pipelineFWUFlag, "fwu", "", "FWU ID (required)")
	runPipelineCmd.Flags().StringVarP(&pipelineWorkdirFlag, "workdir", "w", ".", "Working directory for Claude Code")
	runPipelineCmd.Flags().StringVar(&pipelineTractStoreFlag, "tract-store", "", "Tract store name (required unless briefing-file is set)")
	runPipelineCmd.Flags().StringVar(&pipelineAgentsDirFlag, "agents-dir", "./agents", "Directory containing agent definitions")
	runPipelineCmd.Flags().StringVar(&pipelineBriefingFileFlag, "briefing-file", "", "Path to file containing briefing text (overrides tract-store)")
	runPipelineCmd.Flags().StringVar(&pipelineNameFlag, "pipeline", "default", "Pipeline configuration name (loaded from agents-dir/pipelines/)")
	runPipelineCmd.Flags().IntVar(&pipelineMaxIterationsFlag, "max-iterations", 10, "Maximum iterations per phase (0 = unlimited)")
	runPipelineCmd.Flags().IntVar(&pipelineMaxRetriesFlag, "max-retries", 3, "Retry attempts on crash per phase")
	runPipelineCmd.Flags().IntVar(&pipelineMaxReviewFlag, "max-review-cycles", 3, "Maximum IM↔RV review cycles")

	// Evidence flags
	runPipelineCmd.Flags().StringVar(&pipelineEvidenceDir, "evidence-dir", ".axon/evidence", "Local directory for evidence capture")
	runPipelineCmd.Flags().StringVar(&pipelineEvidenceBucket, "evidence-bucket", "", "S3 bucket for evidence upload (enables upload)")
	runPipelineCmd.Flags().StringVar(&pipelineEvidenceRegion, "evidence-region", "us-east-1", "AWS region for evidence bucket")
	runPipelineCmd.Flags().StringVar(&pipelineEvidenceEndpoint, "evidence-endpoint", "", "Custom S3 endpoint for evidence bucket")
	runPipelineCmd.Flags().StringVar(&pipelineEvidencePrefix, "evidence-prefix", "evidence/", "Key prefix for uploaded evidence")

	_ = runPipelineCmd.MarkFlagRequired("fwu")
	// tract-store is required unless briefing-file is provided. Checked in RunE.

	rootCmd.AddCommand(runPipelineCmd)
}

var runPipelineCmd = &cobra.Command{
	Use:   "run-pipeline",
	Short: "Run a full FWU pipeline (TD → IM → RV → RF → FL)",
	Long: `Drives an FWU through the full agent workflow: Technical Design, Implementation,
Review, Refactor, and Feedback Loop. Each phase creates a new Loop with the
appropriate agent and workflow prompt.

Events are printed as JSON lines to stdout, enriched with phase context.

Signal handling:
  First SIGINT  → pause (finish current iteration, then exit)
  Second SIGINT → stop (kill process immediately)
  SIGTERM       → stop (kill process immediately)`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if pipelineTractStoreFlag == "" && pipelineBriefingFileFlag == "" {
			return fmt.Errorf("either --tract-store or --briefing-file is required")
		}

		pipelineDef, err := pipeline.LoadPipelineConfig(pipelineAgentsDirFlag, pipelineNameFlag)
		if err != nil {
			return fmt.Errorf("load pipeline config: %w", err)
		}

		var briefingFn pipeline.BriefingFunc
		if pipelineBriefingFileFlag != "" {
			content, err := os.ReadFile(pipelineBriefingFileFlag)
			if err != nil {
				return fmt.Errorf("read briefing file: %w", err)
			}
			briefingFn = func(ctx context.Context, fwuID string) (string, error) {
				return string(content), nil
			}
		}

		cfg := pipeline.Config{
			FWUID:                pipelineFWUFlag,
			WorkDir:              pipelineWorkdirFlag,
			TractStore:           pipelineTractStoreFlag,
			AgentsDir:            pipelineAgentsDirFlag,
			Pipeline:             pipelineDef,
			BriefingFunc:         briefingFn,
			DefaultMaxIterations: pipelineMaxIterationsFlag,
			MaxRetries:           pipelineMaxRetriesFlag,
			MaxReviewCycles:      pipelineMaxReviewFlag,
			RetryBackoff:         []time.Duration{0, 5 * time.Second, 15 * time.Second},
			ThrashThreshold:      3,
			EvidenceDir:          pipelineEvidenceDir,
			EvidenceBucket:       pipelineEvidenceBucket,
			EvidenceRegion:       pipelineEvidenceRegion,
			EvidenceEndpoint:     pipelineEvidenceEndpoint,
			EvidencePrefix:       pipelineEvidencePrefix,
		}

		p, events := pipeline.New(cfg)

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
		go handlePipelineSignals(p)

		return p.Run(cmd.Context())
	},
}

func handlePipelineSignals(p *pipeline.Pipeline) {
	sigCh := make(chan os.Signal, 2)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	var sigintCount atomic.Int32

	for sig := range sigCh {
		switch sig {
		case syscall.SIGINT:
			n := sigintCount.Add(1)
			if n == 1 {
				fmt.Fprintln(os.Stderr, "\naxon: received SIGINT, pausing after current iteration...")
				p.Pause()
			} else {
				fmt.Fprintln(os.Stderr, "\naxon: received second SIGINT, stopping immediately...")
				p.Stop()
				return
			}
		case syscall.SIGTERM:
			fmt.Fprintln(os.Stderr, "\naxon: received SIGTERM, stopping immediately...")
			p.Stop()
			return
		}
	}
}
