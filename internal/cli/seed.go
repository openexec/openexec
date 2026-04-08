package cli

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/openexec/openexec/internal/botseed"
	"github.com/openexec/openexec/internal/contracts"
	"github.com/spf13/cobra"

	// Register database drivers used by the --clear path.
	_ "github.com/lib/pq"
	_ "github.com/mattn/go-sqlite3"
)

var (
	seedBots     int
	seedRate     float64
	seedDuration time.Duration
	seedBaseURL  string
	seedClear    bool
	seedContract string
	seedRNG      int64
)

var seedCmd = &cobra.Command{
	Use:   "seed",
	Short: "Seed a running template app with synthetic bot users and traffic",
	Long: `Seed populates a running SvelteKit template app with a pool of
realistic synthetic users ("bots") that sign up through the real
/api/auth/* endpoints and then exercise operations from the active
blueprint contract using real HTTP calls.

Bots live in their own namespace — every email is prefixed with
"bot_" so they can be wiped with --clear without touching
human-authored rows.

Examples:
  openexec seed                                   # 20 bots for 2m at 1 rps
  openexec seed --bots 50 --rate 4 --duration 5m
  openexec seed --clear                           # delete bot_*@ users
  openexec seed --contract ./blueprint.yaml`,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		if ctx == nil {
			ctx = context.Background()
		}

		if seedClear {
			return runSeedClear(ctx, cmd)
		}
		return runSeedTraffic(ctx, cmd)
	},
}

func init() {
	seedCmd.Flags().IntVar(&seedBots, "bots", 20, "number of bot users to register")
	seedCmd.Flags().Float64Var(&seedRate, "rate", 1.0, "requests per second across the pool")
	seedCmd.Flags().DurationVar(&seedDuration, "duration", 2*time.Minute, "how long to generate traffic")
	seedCmd.Flags().StringVar(&seedBaseURL, "base-url", "http://localhost:3000", "base URL of the running app")
	seedCmd.Flags().BoolVar(&seedClear, "clear", false, "delete all bot users from DATABASE_URL and exit")
	seedCmd.Flags().StringVar(&seedContract, "contract", "", "path to blueprint YAML with feature_completeness_contracts")
	seedCmd.Flags().Int64Var(&seedRNG, "seed", 1, "RNG seed for deterministic persona generation")
	rootCmd.AddCommand(seedCmd)
}

// runSeedTraffic resolves the contract, runs a traffic pass, and
// prints a human-readable report to stdout.
func runSeedTraffic(ctx context.Context, cmd *cobra.Command) error {
	contractPath := seedContract
	if contractPath == "" {
		contractPath = resolveDefaultSeedContractPath()
	}
	if contractPath == "" {
		return fmt.Errorf("no contract found; pass --contract <path> or place blueprint.yaml under .openexec/")
	}
	if _, err := os.Stat(contractPath); err != nil {
		return fmt.Errorf("contract %q not accessible: %w", contractPath, err)
	}

	cf, err := contracts.LoadFromYAML(contractPath)
	if err != nil {
		return fmt.Errorf("load contract: %w", err)
	}
	if cf.IsEmpty() {
		return fmt.Errorf("contract %q has no feature_completeness_contracts entries", contractPath)
	}

	cmd.Printf("botseed: registering %d bot(s) against %s\n", seedBots, seedBaseURL)
	cmd.Printf("botseed: rate=%.2f rps duration=%s contract=%s\n", seedRate, seedDuration, contractPath)

	report, err := botseed.RunTraffic(ctx, botseed.TrafficConfig{
		Bots:     seedBots,
		Rate:     seedRate,
		Duration: seedDuration,
		BaseURL:  seedBaseURL,
		Contract: cf,
		Seed:     seedRNG,
	})
	if err != nil {
		return fmt.Errorf("run traffic: %w", err)
	}
	printReport(cmd, report)
	return nil
}

// runSeedClear opens DATABASE_URL with the sqlite3 driver (the only
// driver compiled into the binary today) and deletes bot_* users.
func runSeedClear(ctx context.Context, cmd *cobra.Command) error {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		return fmt.Errorf("DATABASE_URL is not set; cannot clear bot users")
	}
	driver, cleaned, err := resolveSeedDriver(dsn)
	if err != nil {
		return err
	}
	db, err := sql.Open(driver, cleaned)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer db.Close()
	if err := db.PingContext(ctx); err != nil {
		return fmt.Errorf("ping database: %w", err)
	}
	if err := botseed.Clear(ctx, db); err != nil {
		return err
	}
	cmd.Println("botseed: cleared bot_* users")
	return nil
}

// resolveSeedDriver picks a database/sql driver name based on the
// DATABASE_URL scheme. Supported: sqlite (file paths, sqlite:, file:)
// and postgres (postgres://, postgresql://). MySQL still surfaces a
// clear error.
func resolveSeedDriver(dsn string) (driver, cleaned string, err error) {
	lower := strings.ToLower(dsn)
	switch {
	case strings.HasPrefix(lower, "sqlite:"):
		return "sqlite3", strings.TrimPrefix(dsn, "sqlite:"), nil
	case strings.HasPrefix(lower, "sqlite3:"):
		return "sqlite3", strings.TrimPrefix(dsn, "sqlite3:"), nil
	case strings.HasPrefix(lower, "file:"):
		return "sqlite3", dsn, nil
	case strings.HasPrefix(lower, "postgres://"), strings.HasPrefix(lower, "postgresql://"):
		// lib/pq accepts postgres:// URLs directly.
		return "postgres", dsn, nil
	case strings.HasPrefix(lower, "mysql://"):
		return "", "", fmt.Errorf("mysql DATABASE_URL not supported by this build")
	default:
		// Assume a bare sqlite file path.
		return "sqlite3", dsn, nil
	}
}

// resolveDefaultSeedContractPath mirrors the fallback logic from the
// contract_tests action so `openexec seed` discovers the same
// blueprint files without forcing the user to pass --contract.
func resolveDefaultSeedContractPath() string {
	cwd, err := os.Getwd()
	if err != nil {
		return ""
	}
	candidates := []string{
		filepath.Join(cwd, ".openexec", "blueprints", "active.yaml"),
		filepath.Join(cwd, ".openexec", "blueprint.yaml"),
		filepath.Join(cwd, "blueprints", "active.yaml"),
		filepath.Join(cwd, "blueprint.yaml"),
	}
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			return c
		}
	}
	return ""
}

// printReport writes a compact summary of a traffic run to the
// cobra command's stdout.
func printReport(cmd *cobra.Command, report *botseed.TrafficReport) {
	if report == nil {
		return
	}
	cmd.Println()
	cmd.Println("botseed: run finished")
	cmd.Printf("  bots registered: %d\n", report.BotsRegistered)
	cmd.Printf("  total ops      : %d\n", report.Operations)
	cmd.Printf("  successes      : %d\n", report.Successes)
	cmd.Printf("  failures       : %d\n", report.Failures)
	cmd.Printf("  started at     : %s\n", report.StartedAt.Format(time.RFC3339))
	cmd.Printf("  finished at    : %s\n", report.FinishedAt.Format(time.RFC3339))

	if len(report.ByOperation) == 0 {
		return
	}
	cmd.Println("  by operation:")
	ids := make([]string, 0, len(report.ByOperation))
	for id := range report.ByOperation {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		s := report.ByOperation[id]
		cmd.Printf("    %-40s attempts=%-4d ok=%-4d fail=%-4d p50=%dms p95=%dms\n",
			id, s.Attempts, s.Successes, s.Failures, s.P50ms, s.P95ms)
	}
}
