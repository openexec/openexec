package cli

import (
	"context"
	"fmt"
	"os"
	"os/exec"

	"github.com/spf13/cobra"
)

// govAutopilotCmd runs one autonomous tick of the delivery loop: connect (sync
// the AI-Fix-labeled issues), then advance the single next actionable change
// through the non-destructive governance steps. It is SAFE BY DEFAULT — it
// triages and reviews, and PARKS at approval and execution for a human. The
// review step still posts a clarification comment when it blocks (see the
// clarification loop), so a tick either advances a change to plan_ready, parks it
// awaiting your answer, or reports there is nothing to do.
//
// Execution/PR is intentionally gated behind --auto-execute (and, later, a
// low-risk auto-approve policy) so a cron cannot ship code unattended without an
// explicit opt-in. The loop never merges — humans merge.
//
// One tick advances one change as far as the flags allow, then exits, so it is
// cron-friendly: schedule it and each fire moves the queue forward. Single-slot
// is enforced by NextActionable (an implementing change occupies the slot).
var govAutopilotCmd = &cobra.Command{
	Use:   "autopilot",
	Short: "One autonomous tick: sync AI-Fix issues, then advance the next actionable change (safe by default)",
	RunE: func(cmd *cobra.Command, args []string) error {
		svc, _, db, err := newGovService(cmd)
		if err != nil {
			return err
		}
		defer db.Close()

		project, _ := cmd.Flags().GetString("project")
		repo, _ := cmd.Flags().GetString("repo")
		label, _ := cmd.Flags().GetString("label")
		autoExecute, _ := cmd.Flags().GetBool("auto-execute")
		maxSteps, _ := cmd.Flags().GetInt("max-steps")
		if project == "" || repo == "" {
			return fmt.Errorf("--project and --repo are required")
		}
		baseDir, _ := govBaseDir(cmd)
		self, err := os.Executable()
		if err != nil || self == "" {
			self = os.Args[0]
		}

		// 1. Connect: import (label-gated) issues. --no-triage so the loop below
		//    triages each change under its own single-slot control.
		cmd.Printf("[autopilot] sync %s (label=%q)\n", repo, label)
		if out, serr := runSelf(cmd.Context(), self, baseDir,
			"governance", "github", "sync", "--project", project, "--repo", repo,
			"--label", label, "--no-triage", "--project-dir", baseDir); serr != nil {
			return fmt.Errorf("sync failed: %w — %s", serr, out)
		}

		// 2. Drive the next actionable change, one step at a time, until it parks,
		//    the slot is busy, or there is nothing to do.
		for step := 0; step < maxSteps; step++ {
			ch, action, err := svc.NextActionable(cmd.Context(), project)
			if err != nil {
				return err
			}
			if ch == nil {
				cmd.Println("[autopilot] no actionable work; done.")
				return nil
			}
			cmd.Printf("[autopilot] %s [%s] → %s\n", ch.ID, ch.Status, action)

			switch action {
			case "in-progress":
				cmd.Printf("[autopilot] slot busy (%s is implementing); nothing new started.\n", ch.ID)
				return nil
			case "triage":
				if out, e := runSelf(cmd.Context(), self, baseDir,
					"governance", "work", "triage", ch.ID, "--deep", "--project-dir", baseDir); e != nil {
					return fmt.Errorf("triage %s: %w — %s", ch.ID, e, out)
				}
			case "review":
				// review-plan posts a clarification comment + parks if it blocks.
				if out, e := runSelf(cmd.Context(), self, baseDir,
					"governance", "work", "review-plan", ch.ID, "--reviewer", "bugbot", "--project-dir", baseDir); e != nil {
					return fmt.Errorf("review %s: %w — %s", ch.ID, e, out)
				}
				cmd.Printf("[autopilot] reviewed %s; parked for human approval or clarification.\n", ch.ID)
				return nil // review always parks (plan_ready or changes_requested)
			case "execute":
				if !autoExecute {
					cmd.Printf("[autopilot] %s is approved; parked (execution needs --auto-execute).\n", ch.ID)
					return nil
				}
				if out, e := runSelf(cmd.Context(), self, baseDir,
					"governance", "work", "execute", ch.ID, "--agent", "claude", "--mode", "workspace-write", "--project-dir", baseDir); e != nil {
					return fmt.Errorf("execute %s: %w — %s", ch.ID, e, out)
				}
				cmd.Printf("[autopilot] executed %s; a human opens/merges the PR (work open-pr).\n", ch.ID)
				return nil
			default:
				cmd.Printf("[autopilot] no automated step for action %q; stopping.\n", action)
				return nil
			}
		}
		cmd.Printf("[autopilot] reached max-steps (%d); more work remains next tick.\n", maxSteps)
		return nil
	},
}

// runSelf shells out to this same openexec binary for a subcommand, inheriting
// the current environment, so the autopilot reuses the exact tested commands.
func runSelf(ctx context.Context, self, dir string, args ...string) (string, error) {
	c := exec.CommandContext(ctx, self, args...)
	c.Dir = dir
	out, err := c.CombinedOutput()
	return string(out), err
}

func init() {
	governanceCmd.AddCommand(govAutopilotCmd)
	govAutopilotCmd.Flags().String("project", "", "Project ID")
	govAutopilotCmd.Flags().String("repo", "", "GitHub repo (owner/repo)")
	govAutopilotCmd.Flags().String("label", "AI Fix", "Control label — only issues with it are worked (deny-by-default)")
	govAutopilotCmd.Flags().Bool("auto-execute", false, "Allow the tick to run execution on an approved change (default: park for a human)")
	govAutopilotCmd.Flags().Int("max-steps", 6, "Max steps to advance in one tick")
}
