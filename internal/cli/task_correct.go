package cli

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"

	"github.com/openexec/openexec/internal/execution/gates"
	"github.com/openexec/openexec/internal/knowledge"
	"github.com/openexec/openexec/internal/release"
	"github.com/openexec/openexec/pkg/db/state"
	"github.com/openexec/openexec/pkg/manager"
	"github.com/spf13/cobra"
)

// Capture the trusted launcher boundary once, as for operator MCP sessions.
// A decision reference or command flag cannot turn an agent into an operator.
func newTaskCorrectCommand(operator bool) *cobra.Command {
	var decision, plan, directory string
	var authorize bool
	cmd := &cobra.Command{
		Use: "correct <task-id>", Short: "Authorize one native verification pass of an exhausted retained candidate",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !operator {
				return fmt.Errorf("task correction requires an operator session")
			}
			if !authorize {
				return fmt.Errorf("explicit --authorize-correction is required")
			}
			if !regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:/@+-]{0,255}$`).MatchString(decision) {
				return fmt.Errorf("a valid --decision-ref is required (1-256 reference characters, no whitespace)")
			}
			root, err := filepath.Abs(directory)
			if err != nil {
				return err
			}
			dbPath := filepath.Join(root, ".openexec", "openexec.db")
			if _, err := os.Stat(dbPath); err != nil {
				return err
			}
			store, err := state.NewStore(dbPath)
			if err != nil {
				return err
			}
			defer store.Close()
			mgr, err := manager.New(manager.Config{WorkDir: root, StateStore: store})
			if err != nil {
				return err
			}
			defer mgr.Close()
			rel, err := release.NewSQLiteStore(store.GetDB())
			if err != nil {
				return err
			}
			task, err := rel.GetTask(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			evidence, _ := task.Metadata["verification_failure_evidence"].(string)
			step, err := store.GetRunStep(cmd.Context(), evidence)
			if err != nil {
				return err
			}
			var artifacts map[string]string
			if step == nil || step.RunID != task.ID || step.Status != "failed" || step.Agent.String != "deterministic-verification" || json.Unmarshal([]byte(step.Metadata), &artifacts) != nil || !gates.ValidateVerificationFailureArtifacts(artifacts) {
				return fmt.Errorf("correction requires the retained task failure receipt")
			}
			path, branch, digest, err := mgr.CorrectionCandidate(cmd.Context())
			if err != nil {
				return err
			}
			c := release.TaskCorrection{TaskID: task.ID, DecisionRef: decision, EvidenceID: evidence, CandidatePath: path, Branch: branch, CandidateDigest: digest}
			err = store.GetDB().QueryRowContext(cmd.Context(), `SELECT id FROM validation_plan_revisions WHERE task_id=? AND status='accepted' ORDER BY revision DESC LIMIT 1`, task.ID).Scan(&c.PlanID)
			if err != nil && err != sql.ErrNoRows {
				return err
			}
			if cmd.Flags().Changed("plan") && (plan == "" || plan != c.PlanID) {
				return fmt.Errorf("--plan must match the current accepted task plan")
			}
			if c.PlanID != "" {
				manifest, err := knowledge.BuildScanManifest(path)
				if err != nil {
					return err
				}
				c.StateHash = manifest.WorktreeStateHash
			}
			if err := mgr.AuthorizeTaskCorrection(cmd.Context(), c); err != nil {
				return err
			}
			cmd.Printf("Authorized task %s correction with decision %s; execution remains with the native queue.\n", task.ID, decision)
			return nil
		},
	}
	cmd.Flags().StringVar(&directory, "dir", ".", "Retained candidate root")
	cmd.Flags().StringVar(&decision, "decision-ref", "", "Explicit operator correction decision reference")
	cmd.Flags().StringVar(&plan, "plan", "", "Require this current accepted plan")
	cmd.Flags().BoolVar(&authorize, "authorize-correction", false, "Explicitly authorize one correction verification pass")
	return cmd
}

func init() { taskCmd.AddCommand(newTaskCorrectCommand(os.Getenv("OPENEXEC_OPERATOR_SESSION") == "1")) }
