package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"time"

	"github.com/openexec/openexec/internal/governance"
	"github.com/openexec/openexec/internal/governance/service"
	"github.com/openexec/openexec/internal/project"
	"github.com/spf13/cobra"
)

// autopilotExecuteToPR is the autonomous execute→commit→PR path. It runs the
// change's tasks through the UNGATED single-task endpoint (POST
// /runs/{id}/start → Manager.Start, which the batch executor's hitl gate does
// not touch), commits the result deterministically (the agent does not reliably
// self-commit), and opens a PR. hitl study/QA tasks are run too — the PR review
// IS the human checkpoint in this model — and flagged in the PR for verification.
func autopilotExecuteToPR(cmd *cobra.Command, self, baseDir, changeID string, svc *service.Service) error {
	pc, _ := project.LoadProjectConfig(baseDir)
	port := 8765
	base := "main"
	if pc != nil {
		if pc.Execution.Port > 0 {
			port = pc.Execution.Port
		}
		if pc.BaseBranch != "" {
			base = pc.BaseBranch
		}
	}
	if !daemonHealthy(port) {
		return fmt.Errorf("execution daemon not reachable on port %d — start it first: (cd %s && openexec start)", port, baseDir)
	}

	// Isolate on a per-change branch off base.
	branch := "gov/" + changeID
	if out, err := gitIn(baseDir, "checkout", "-B", branch, base); err != nil {
		return fmt.Errorf("branch %s: %w — %s", branch, err, out)
	}

	stories, err := svc.ChangeStories(cmd.Context(), changeID)
	if err != nil {
		return fmt.Errorf("list tasks for %s: %w", changeID, err)
	}
	var taskIDs []string
	for _, st := range stories {
		taskIDs = append(taskIDs, st.Tasks...)
	}
	if len(taskIDs) == 0 {
		return fmt.Errorf("change %s has no tasks", changeID)
	}

	// Run each task ungated, in story order, and wait for it to finish.
	for _, tid := range taskIDs {
		cmd.Printf("[autopilot]   run %s (ungated)...\n", tid)
		if err := startRunUngated(port, tid, "workspace-write"); err != nil {
			return fmt.Errorf("start %s: %w", tid, err)
		}
		st, err := waitRunTerminal(cmd.Context(), port, tid, 30*time.Minute)
		if err != nil {
			return fmt.Errorf("run %s: %w", tid, err)
		}
		cmd.Printf("[autopilot]   %s → %s\n", tid, st)
	}

	// Deterministic commit — the agent does not reliably call safe_commit.
	if out, err := gitIn(baseDir, "add", "-A"); err != nil {
		return fmt.Errorf("git add: %w — %s", err, out)
	}
	msg := fmt.Sprintf("feat: implement %s (autopilot)", changeID)
	out, err := gitIn(baseDir, "-c", "user.name=openexec", "-c", "user.email=openexec@local", "commit", "-m", msg)
	if err != nil {
		// "nothing to commit" means the run produced no changes — surface it.
		if bytesContains(out, "nothing to commit") {
			cmd.Printf("[autopilot] %s produced no changes; nothing to PR.\n", changeID)
			return nil
		}
		return fmt.Errorf("git commit: %w — %s", err, out)
	}

	// Open the PR (push + gh pr create + assessment).
	self2 := self
	pout, perr := runSelf(cmd.Context(), self2, baseDir,
		"governance", "work", "open-pr", changeID, "--branch", branch, "--project-dir", baseDir)
	if perr != nil {
		return fmt.Errorf("open-pr %s: %w — %s", changeID, perr, pout)
	}
	cmd.Print(pout)
	return nil
}

func daemonHealthy(port int) bool {
	resp, err := http.Get(fmt.Sprintf("http://localhost:%d/api/v1/runs", port))
	if err != nil {
		return false
	}
	_ = resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

func startRunUngated(port int, taskID, mode string) error {
	body, _ := json.Marshal(map[string]string{"mode": mode})
	resp, err := http.Post(
		fmt.Sprintf("http://localhost:%d/api/v1/runs/%s/start", port, taskID),
		"application/json", bytes.NewReader(body))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 && resp.StatusCode != http.StatusConflict {
		return fmt.Errorf("daemon returned %d", resp.StatusCode)
	}
	return nil
}

// waitRunTerminal polls the run status until it reaches a terminal state.
func waitRunTerminal(ctx context.Context, port int, taskID string, timeout time.Duration) (string, error) {
	deadline := time.Now().Add(timeout)
	for {
		resp, err := http.Get(fmt.Sprintf("http://localhost:%d/api/v1/runs/%s", port, taskID))
		if err == nil {
			var b struct {
				Status string `json:"status"`
			}
			_ = json.NewDecoder(resp.Body).Decode(&b)
			_ = resp.Body.Close()
			switch b.Status {
			case "complete", "completed", "failed", "error", "paused":
				return b.Status, nil
			}
		}
		if time.Now().After(deadline) {
			return "", fmt.Errorf("timed out after %s", timeout)
		}
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(3 * time.Second):
		}
	}
}

func gitIn(dir string, args ...string) (string, error) {
	c := exec.Command("git", append([]string{"-C", dir}, args...)...)
	out, err := c.CombinedOutput()
	return string(out), err
}

func bytesContains(s string, sub string) bool { return bytes.Contains([]byte(s), []byte(sub)) }

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
		svc, store, db, err := newGovService(cmd)
		if err != nil {
			return err
		}
		defer db.Close()

		project, _ := cmd.Flags().GetString("project")
		repo, _ := cmd.Flags().GetString("repo")
		label, _ := cmd.Flags().GetString("label")
		autoApproveLow, _ := cmd.Flags().GetBool("auto-approve-low-risk")
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
				// Re-read: review left the change plan_ready or changes_requested.
				ch2, e := store.GetChangeRecord(cmd.Context(), ch.ID)
				if e != nil {
					return e
				}
				if ch2.Status == governance.ChangeStatusChangesRequested {
					cmd.Printf("[autopilot] %s parked — clarification posted to the issue; awaiting your answer.\n", ch.ID)
					return nil
				}
				// plan_ready. Labeling the issue is the authorization to IMPLEMENT
				// (the human still approves at the PR/merge, not here), so auto-
				// approve within the risk ceiling; higher-risk plans park for a
				// human `/openexec approve`.
				if autoApproveLow && ch2.Risk == governance.RiskLow {
					if out, ae := runSelf(cmd.Context(), self, baseDir,
						"governance", "work", "approve", ch.ID, "--by", "pm", "--project-dir", baseDir); ae != nil {
						return fmt.Errorf("auto-approve %s: %w — %s", ch.ID, ae, out)
					}
					cmd.Printf("[autopilot] %s reviewed + auto-approved (low risk); queued for execution.\n", ch.ID)
					continue // NextActionable returns it as "execute" next
				}
				cmd.Printf("[autopilot] %s reviewed → plan_ready (%s risk); parked for your approval.\n", ch.ID, ch2.Risk)
				return nil
			case "execute":
				if !autoExecute {
					cmd.Printf("[autopilot] %s is approved; parked (execution needs --auto-execute).\n", ch.ID)
					return nil
				}
				cmd.Printf("[autopilot] executing %s → PR (ungated run → commit → open-pr)...\n", ch.ID)
				if e := autopilotExecuteToPR(cmd, self, baseDir, ch.ID, svc); e != nil {
					return fmt.Errorf("execute→PR %s: %w", ch.ID, e)
				}
				cmd.Printf("[autopilot] %s done; the slot is free for the next change.\n", ch.ID)
				continue // after PR the change is pr_open; the loop picks the next actionable
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
	govAutopilotCmd.Flags().Bool("auto-approve-low-risk", false, "Auto-approve low-risk plans in-loop (labeling is the authorization; you still approve at the PR)")
	govAutopilotCmd.Flags().Bool("auto-execute", false, "Allow the tick to run execution on an approved change (default: park for a human)")
	govAutopilotCmd.Flags().Int("max-steps", 6, "Max steps to advance in one tick")
}
