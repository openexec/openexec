package main

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/openexec/openexec/pkg/api"
	"github.com/openexec/openexec/pkg/audit"
	"github.com/openexec/openexec/pkg/db/session"
	"github.com/openexec/openexec/pkg/manager"
	"github.com/openexec/openexec/internal/pipeline"
	_ "modernc.org/sqlite"
	"github.com/spf13/cobra"
)

var (
	servePortFlag          int
	serveWorkdirFlag       string
	serveTractStoreFlag    string
	serveAgentsDirFlag     string
	servePipelineFlag      string
	serveMaxIterationsFlag int
	serveMaxRetriesFlag    int
	serveMaxReviewFlag     int
	serveAuditDBFlag       string
	serveProjectsDirFlag   string
)

func init() {
	serveCmd.Flags().IntVar(&servePortFlag, "port", 8080, "HTTP server port")
	serveCmd.Flags().StringVarP(&serveWorkdirFlag, "workdir", "w", ".", "Working directory for pipelines")
	serveCmd.Flags().StringVar(&serveTractStoreFlag, "tract-store", "", "Tract store name (required)")
	serveCmd.Flags().StringVar(&serveAgentsDirFlag, "agents-dir", "./agents", "Directory containing agent definitions")
	serveCmd.Flags().StringVar(&servePipelineFlag, "pipeline", "default", "Pipeline configuration name (loaded from agents-dir/pipelines/)")
	serveCmd.Flags().IntVar(&serveMaxIterationsFlag, "max-iterations", 10, "Maximum iterations per phase (0 = unlimited)")
	serveCmd.Flags().IntVar(&serveMaxRetriesFlag, "max-retries", 3, "Retry attempts on crash per phase")
	serveCmd.Flags().IntVar(&serveMaxReviewFlag, "max-review-cycles", 3, "Maximum IM↔RV review cycles")
	serveCmd.Flags().StringVar(&serveAuditDBFlag, "audit-db", ".openexec/data/audit.db", "Path to audit database")
	serveCmd.Flags().StringVar(&serveProjectsDirFlag, "projects-dir", "..", "Path to projects root directory for multi-project discovery")

	_ = serveCmd.MarkFlagRequired("tract-store")

	rootCmd.AddCommand(serveCmd)
}

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start Axon HTTP API server",
	Long: `Starts an HTTP API server for managing multiple FWU pipelines concurrently.
Pipelines are started, paused, stopped, and monitored via REST endpoints.
Server-Sent Events (SSE) provide real-time pipeline event streaming.

Endpoints:
  POST /api/fwu/{id}/start   Start a pipeline
  GET  /api/fwu/{id}/status   Get pipeline status
  GET  /api/fwus              List all pipelines
  POST /api/fwu/{id}/pause    Pause a pipeline
  POST /api/fwu/{id}/stop     Stop a pipeline
  GET  /api/fwu/{id}/events   SSE event stream`,
	RunE: func(cmd *cobra.Command, args []string) error {
		pipelineDef, err := pipeline.LoadPipelineConfig(serveAgentsDirFlag, servePipelineFlag)
		if err != nil {
			return fmt.Errorf("load pipeline config: %w", err)
		}

		// Initialize Database and Repository
		dbPath := serveAuditDBFlag
		if err := os.MkdirAll(filepath.Dir(dbPath), 0755); err != nil {
			return fmt.Errorf("create audit db directory: %w", err)
		}

		db, err := sql.Open("sqlite", dbPath)
		if err != nil {
			return fmt.Errorf("open sqlite db: %w", err)
		}
		defer db.Close()

		sessionRepo, err := session.NewSQLiteRepository(db)
		if err != nil {
			return fmt.Errorf("create session repo: %w", err)
		}

		auditLogger, err := audit.NewLogger(dbPath)
		if err != nil {
			// If NewLogger fails, we can still run with partial functionality
			fmt.Printf("Warning: failed to initialize audit logger: %v\n", err)
		}

		cfg := manager.Config{
			WorkDir:              serveWorkdirFlag,
			TractStore:           serveTractStoreFlag,
			AgentsDir:            serveAgentsDirFlag,
			Pipeline:             pipelineDef,
			DefaultMaxIterations: serveMaxIterationsFlag,
			MaxRetries:           serveMaxRetriesFlag,
			MaxReviewCycles:      serveMaxReviewFlag,
			RetryBackoff:         []time.Duration{0, 5 * time.Second, 15 * time.Second},
			ThrashThreshold:      3,
		}

		mgr := manager.New(cfg)
		addr := fmt.Sprintf(":%d", servePortFlag)
		srv := api.New(mgr, sessionRepo, auditLogger, serveProjectsDirFlag, addr)

		ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
		defer stop()

		_, _ = fmt.Fprintf(cmd.OutOrStderr(), "axon: serving on %s\n", addr)
		return srv.ListenAndServe(ctx)
	},
}
