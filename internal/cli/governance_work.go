package cli

// Work-item subcommands for the governance group (see governance.go). These are
// thin adapters over internal/governance/service: each parses input, calls one
// Service method, and renders the result.

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

var governanceWorkCmd = &cobra.Command{
	Use:   "work",
	Short: "Manage change records through the work lifecycle (import, triage, review, claim, evidence)",
}

var govWorkImportGitHubCmd = &cobra.Command{
	Use:   "import-github",
	Short: "Import a GitHub issue as a change record (idempotent)",
	RunE: func(cmd *cobra.Command, args []string) error {
		svc, _, db, err := newGovService(cmd)
		if err != nil {
			return err
		}
		defer db.Close()

		projectID, _ := cmd.Flags().GetString("project")
		repo, _ := cmd.Flags().GetString("repo")
		issue, _ := cmd.Flags().GetInt("issue")
		if projectID == "" || repo == "" || issue == 0 {
			return fmt.Errorf("--project, --repo (owner/repo), and --issue are required")
		}

		ch, err := svc.ImportGitHubIssue(cmd.Context(), projectID, repo, issue)
		if err != nil {
			return err
		}
		cmd.Printf("Imported change %s [%s]: %s\n", ch.ID, ch.Status, ch.Title)
		return nil
	},
}

var govWorkAttachCmd = &cobra.Command{
	Use:   "attach <change-id>",
	Short: "Attach a change record to a release",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		svc, _, db, err := newGovService(cmd)
		if err != nil {
			return err
		}
		defer db.Close()

		releaseID, _ := cmd.Flags().GetString("release")
		if releaseID == "" {
			return fmt.Errorf("--release <releaseID> is required")
		}
		priority, _ := cmd.Flags().GetInt("priority")
		required, _ := cmd.Flags().GetBool("required")

		if err := svc.AttachChange(cmd.Context(), releaseID, args[0], priority, required); err != nil {
			return err
		}
		cmd.Printf("Attached change %s to release %s (priority %d, required %t)\n", args[0], releaseID, priority, required)
		return nil
	},
}

var govWorkTriageCmd = &cobra.Command{
	Use:   "triage <change-id>",
	Short: "Run the planner AI over a change and apply the resulting plan",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		svc, _, db, err := newGovService(cmd)
		if err != nil {
			return err
		}
		defer db.Close()

		repoContext, _ := cmd.Flags().GetString("context")
		actor, _ := cmd.Flags().GetString("actor")

		// Deep triage: run the real planner to decompose intent into stories +
		// vertical-slice tasks (owned by this change), instead of a flat plan.
		if deep, _ := cmd.Flags().GetBool("deep"); deep {
			res, err := svc.TriageDeep(cmd.Context(), args[0], repoContext, actor)
			if err != nil {
				return err
			}
			if jsonOut, _ := cmd.Flags().GetBool("json"); jsonOut {
				return printJSON(cmd, res)
			}
			cmd.Printf("Deep-triaged change %s into %d stor(ies): %s\n", args[0], len(res.StoryIDs), strings.Join(res.StoryIDs, ", "))
			cmd.Printf("Review the decomposition: openexec governance work stories %s\n", args[0])
			return nil
		}

		out, err := svc.Triage(cmd.Context(), args[0], repoContext, actor)
		if err != nil {
			return err
		}
		if jsonOut, _ := cmd.Flags().GetBool("json"); jsonOut {
			return printJSON(cmd, out)
		}
		cmd.Printf("Triaged change %s\n", args[0])
		cmd.Printf("  Summary: %s\n", out.Summary)
		cmd.Printf("  Kind: %s  Risk: %s\n", out.Kind, out.Risk)
		if len(out.AcceptanceCriteria) > 0 {
			cmd.Println("  Acceptance criteria:")
			for _, ac := range out.AcceptanceCriteria {
				cmd.Printf("    - %s\n", ac)
			}
		}
		if len(out.VerificationPlan) > 0 {
			cmd.Println("  Verification plan:")
			for _, v := range out.VerificationPlan {
				cmd.Printf("    - %s\n", v)
			}
		}
		return nil
	},
}

var govWorkExecuteCmd = &cobra.Command{
	Use:   "execute <change-id>",
	Short: "Run an approved change's tasks through the execution engine",
	Long: `Claim an approved change and run each of its tasks through the existing
execution engine (openexec run), producing commits/PRs. Refused unless the
change is approved for AI in an approved release. After execution, link the PR
with 'work record-pr' and move to 'work ready-for-test'.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		svc, _, db, err := newGovService(cmd)
		if err != nil {
			return err
		}
		defer db.Close()

		agent, _ := cmd.Flags().GetString("agent")
		mode, _ := cmd.Flags().GetString("mode")
		rep, err := svc.ExecuteChange(cmd.Context(), args[0], agent, mode)
		if err != nil {
			return err
		}
		cmd.Printf("Executed change %s: %d task(s) dispatched", rep.ChangeID, len(rep.DispatchedTasks))
		if len(rep.Failures) > 0 {
			cmd.Printf(", %d failed", len(rep.Failures))
		}
		cmd.Println()
		for _, tid := range rep.DispatchedTasks {
			cmd.Printf("  ok   %s\n", tid)
		}
		for tid, msg := range rep.Failures {
			cmd.Printf("  fail %s: %s\n", tid, msg)
		}
		cmd.Printf("Next: link the PR (work record-pr %s --url ...) then work ready-for-test %s\n", args[0], args[0])
		return nil
	},
}

var govWorkStoriesCmd = &cobra.Command{
	Use:   "stories <change-id>",
	Short: "Show the stories/tasks a change was decomposed into (deep triage)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		svc, _, db, err := newGovService(cmd)
		if err != nil {
			return err
		}
		defer db.Close()

		stories, err := svc.ChangeStories(cmd.Context(), args[0])
		if err != nil {
			return err
		}
		if jsonOut, _ := cmd.Flags().GetBool("json"); jsonOut {
			return printJSON(cmd, stories)
		}
		if len(stories) == 0 {
			cmd.Printf("Change %s has no linked stories (run: work triage %s --deep)\n", args[0], args[0])
			return nil
		}
		cmd.Printf("Change %s decomposes into %d stor(ies):\n", args[0], len(stories))
		for _, st := range stories {
			cmd.Printf("  - %s [%s] %s\n", st.ID, st.Status, st.Title)
			for _, taskID := range st.Tasks {
				cmd.Printf("      task %s\n", taskID)
			}
		}
		return nil
	},
}

var govWorkReviewPlanCmd = &cobra.Command{
	Use:   "review-plan <change-id>",
	Short: "Run the reviewer AI over a change's plan (records a review, never approves)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		svc, _, db, err := newGovService(cmd)
		if err != nil {
			return err
		}
		defer db.Close()

		reviewer, _ := cmd.Flags().GetString("reviewer")
		if reviewer == "" {
			return fmt.Errorf("--reviewer <authorityID> is required (e.g. bugbot)")
		}
		actor, _ := cmd.Flags().GetString("actor")
		if actor == "" {
			actor = reviewer
		}

		out, err := svc.ReviewPlan(cmd.Context(), args[0], reviewer, actor)
		if err != nil {
			return err
		}
		if jsonOut, _ := cmd.Flags().GetBool("json"); jsonOut {
			return printJSON(cmd, out)
		}
		cmd.Printf("Reviewed change %s by %s\n", args[0], reviewer)
		cmd.Printf("  Decision: %s\n", out.Decision)
		for _, c := range out.Concerns {
			cmd.Printf("  Concern: %s\n", c)
		}
		return nil
	},
}

var govWorkApproveCmd = &cobra.Command{
	Use:   "approve <change-id>",
	Short: "Approve a change's plan for AI execution",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		svc, _, db, err := newGovService(cmd)
		if err != nil {
			return err
		}
		defer db.Close()

		by, _ := cmd.Flags().GetString("by")
		if by == "" {
			return fmt.Errorf("--by <authorityID> is required (e.g. pm)")
		}
		if err := svc.ApproveChange(cmd.Context(), args[0], by); err != nil {
			return err
		}
		cmd.Printf("Change %s approved by %s\n", args[0], by)
		return nil
	},
}

var govWorkNextCmd = &cobra.Command{
	Use:   "next",
	Short: "List approved, unclaimed work available to executors",
	RunE: func(cmd *cobra.Command, args []string) error {
		svc, _, db, err := newGovService(cmd)
		if err != nil {
			return err
		}
		defer db.Close()

		projectID, _ := cmd.Flags().GetString("project")
		if projectID == "" {
			return fmt.Errorf("--project <id> is required")
		}

		work, err := svc.ListApprovedWork(cmd.Context(), projectID)
		if err != nil {
			return err
		}
		if jsonOut, _ := cmd.Flags().GetBool("json"); jsonOut {
			return printJSON(cmd, work)
		}
		if len(work) == 0 {
			cmd.Println("No approved work available.")
			return nil
		}
		cmd.Printf("Approved work (%d):\n", len(work))
		for _, ch := range work {
			cmd.Printf("  - %s [%s] %s (release=%s, risk=%s)\n", ch.ID, ch.Status, ch.Title, ch.ReleaseID, ch.Risk)
		}
		return nil
	},
}

var govWorkClaimCmd = &cobra.Command{
	Use:   "claim <change-id>",
	Short: "Claim a change record for an executor with a lease",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		svc, _, db, err := newGovService(cmd)
		if err != nil {
			return err
		}
		defer db.Close()

		agent, _ := cmd.Flags().GetString("agent")
		if agent == "" {
			return fmt.Errorf("--agent <name> is required")
		}
		leaseStr, _ := cmd.Flags().GetString("lease")
		lease, err := time.ParseDuration(leaseStr)
		if err != nil {
			return fmt.Errorf("invalid --lease %q: %w", leaseStr, err)
		}
		if lease <= 0 {
			return fmt.Errorf("invalid --lease %q: must be a positive duration (a non-positive lease claims work already expired)", leaseStr)
		}
		if err := svc.ClaimWork(cmd.Context(), args[0], agent, lease); err != nil {
			return err
		}
		cmd.Printf("Change %s claimed by %s (lease %s)\n", args[0], agent, lease)
		return nil
	},
}

var govWorkBriefCmd = &cobra.Command{
	Use:   "brief <change-id>",
	Short: "Print the executor brief for a change (approved scope + reporting contract)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		svc, _, db, err := newGovService(cmd)
		if err != nil {
			return err
		}
		defer db.Close()

		repoPath, _ := cmd.Flags().GetString("repo")
		brief, err := svc.WorkBrief(cmd.Context(), args[0], repoPath)
		if err != nil {
			return err
		}
		cmd.Println(brief)
		return nil
	},
}

var govWorkRecordPRCmd = &cobra.Command{
	Use:   "record-pr <change-id>",
	Short: "Record the pull request opened for a change",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		svc, _, db, err := newGovService(cmd)
		if err != nil {
			return err
		}
		defer db.Close()

		url, _ := cmd.Flags().GetString("url")
		branch, _ := cmd.Flags().GetString("branch")
		if url == "" {
			return fmt.Errorf("--url is required")
		}
		if err := svc.RecordPR(cmd.Context(), args[0], url, branch); err != nil {
			return err
		}
		cmd.Printf("Recorded PR for change %s: %s\n", args[0], url)
		return nil
	},
}

var govWorkRecordEvidenceCmd = &cobra.Command{
	Use:   "record-evidence <change-id>",
	Short: "Attach a structured evidence record to a change",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		svc, _, db, err := newGovService(cmd)
		if err != nil {
			return err
		}
		defer db.Close()

		kind, _ := cmd.Flags().GetString("kind")
		source, _ := cmd.Flags().GetString("source")
		summary, _ := cmd.Flags().GetString("summary")
		url, _ := cmd.Flags().GetString("url")
		raw, _ := cmd.Flags().GetString("raw")
		if kind == "" || source == "" || summary == "" {
			return fmt.Errorf("--kind, --source, and --summary are required")
		}
		if err := svc.RecordEvidence(cmd.Context(), args[0], kind, source, summary, url, raw); err != nil {
			return err
		}
		cmd.Printf("Recorded %s evidence for change %s\n", kind, args[0])
		return nil
	},
}

var govWorkReadyForTestCmd = &cobra.Command{
	Use:   "ready-for-test <change-id>",
	Short: "Move a change to ready_for_test (requires a PR or manual/test/CI evidence)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		svc, _, db, err := newGovService(cmd)
		if err != nil {
			return err
		}
		defer db.Close()

		if err := svc.ReadyForTest(cmd.Context(), args[0]); err != nil {
			return err
		}
		cmd.Printf("Change %s is ready for test\n", args[0])
		return nil
	},
}

var govWorkDoneCmd = &cobra.Command{
	Use:   "done <change-id>",
	Short: "Mark a change done (requires verification evidence)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		svc, _, db, err := newGovService(cmd)
		if err != nil {
			return err
		}
		defer db.Close()

		by, _ := cmd.Flags().GetString("by")
		if by == "" {
			return fmt.Errorf("--by <authorityID> is required (e.g. tester_ai, ci_verifier, pm)")
		}
		if err := svc.MarkDone(cmd.Context(), args[0], by); err != nil {
			return err
		}
		cmd.Printf("Change %s marked done by %s\n", args[0], by)
		return nil
	},
}

var govWorkHistoryCmd = &cobra.Command{
	Use:   "history <change-id>",
	Short: "Show the full audit trail (decisions + evidence) for a change",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		svc, _, db, err := newGovService(cmd)
		if err != nil {
			return err
		}
		defer db.Close()

		events, evidence, err := svc.History(cmd.Context(), args[0])
		if err != nil {
			return err
		}
		if jsonOut, _ := cmd.Flags().GetBool("json"); jsonOut {
			return printJSON(cmd, map[string]any{
				"events":   events,
				"evidence": evidence,
			})
		}
		cmd.Printf("History for change %s\n", args[0])
		cmd.Printf("\nDecision events (%d):\n", len(events))
		for _, ev := range events {
			cmd.Printf("  - [%s] %s by %s (v%d): %s\n",
				ev.CreatedAt.Format(time.RFC3339), ev.Decision, ev.Actor, ev.ProposalVersion, ev.Comment)
		}
		cmd.Printf("\nEvidence (%d):\n", len(evidence))
		for _, e := range evidence {
			cmd.Printf("  - [%s] %s/%s: %s\n", e.CreatedAt.Format(time.RFC3339), e.Kind, e.Source, e.Summary)
		}
		return nil
	},
}

var govWorkSyncGitHubCmd = &cobra.Command{
	Use:   "sync-github <change-id>",
	Short: "Mirror a github-sourced change's status to its issue (label + comment)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		svc, _, db, err := newGovService(cmd)
		if err != nil {
			return err
		}
		defer db.Close()

		if err := svc.SyncGitHubState(cmd.Context(), args[0]); err != nil {
			return err
		}
		cmd.Printf("Synced change %s status to its GitHub issue\n", args[0])
		return nil
	},
}

func init() {
	// work import-github
	governanceWorkCmd.AddCommand(govWorkImportGitHubCmd)
	govWorkImportGitHubCmd.Flags().String("project", "", "Project ID")
	govWorkImportGitHubCmd.Flags().String("repo", "", "GitHub repo (owner/repo)")
	govWorkImportGitHubCmd.Flags().Int("issue", 0, "GitHub issue number")

	// work sync-github
	governanceWorkCmd.AddCommand(govWorkSyncGitHubCmd)

	// work attach
	governanceWorkCmd.AddCommand(govWorkAttachCmd)
	govWorkAttachCmd.Flags().String("release", "", "Release ID to attach to")
	govWorkAttachCmd.Flags().Int("priority", 0, "Item priority")
	govWorkAttachCmd.Flags().Bool("required", false, "Mark the item as required")

	// work triage
	governanceWorkCmd.AddCommand(govWorkTriageCmd)
	govWorkTriageCmd.Flags().String("context", "", "Repo context to give the planner")
	govWorkTriageCmd.Flags().String("actor", "planner_ai", "Actor recorded as the proposer")
	govWorkTriageCmd.Flags().Bool("deep", false, "Decompose intent into real stories + tasks via the planner (owned by this change)")
	govWorkTriageCmd.Flags().Bool("json", false, "Output as JSON")

	// work stories
	governanceWorkCmd.AddCommand(govWorkStoriesCmd)
	govWorkStoriesCmd.Flags().Bool("json", false, "Output as JSON")

	// work execute
	governanceWorkCmd.AddCommand(govWorkExecuteCmd)
	govWorkExecuteCmd.Flags().String("agent", "openexec-executor", "Executor name recorded on the claim")
	govWorkExecuteCmd.Flags().String("mode", "workspace-write", "Execution mode (read-only|workspace-write|danger-full-access)")

	// work review-plan
	governanceWorkCmd.AddCommand(govWorkReviewPlanCmd)
	govWorkReviewPlanCmd.Flags().String("reviewer", "", "Reviewer authority ID (e.g. bugbot)")
	govWorkReviewPlanCmd.Flags().String("actor", "", "Actor recorded for the review (default: reviewer)")
	govWorkReviewPlanCmd.Flags().Bool("json", false, "Output as JSON")

	// work approve
	governanceWorkCmd.AddCommand(govWorkApproveCmd)
	govWorkApproveCmd.Flags().String("by", "", "Approving authority ID (e.g. pm)")

	// work next
	governanceWorkCmd.AddCommand(govWorkNextCmd)
	govWorkNextCmd.Flags().String("project", "", "Project ID")
	govWorkNextCmd.Flags().Bool("json", false, "Output as JSON")

	// work claim
	governanceWorkCmd.AddCommand(govWorkClaimCmd)
	govWorkClaimCmd.Flags().String("agent", "", "Executor agent name")
	govWorkClaimCmd.Flags().String("lease", "30m", "Lease duration (e.g. 30m, 1h)")

	// work brief
	governanceWorkCmd.AddCommand(govWorkBriefCmd)
	govWorkBriefCmd.Flags().String("repo", "", "Repo path to embed in the brief")

	// work record-pr
	governanceWorkCmd.AddCommand(govWorkRecordPRCmd)
	govWorkRecordPRCmd.Flags().String("url", "", "Pull request URL")
	govWorkRecordPRCmd.Flags().String("branch", "", "Branch name")

	// work record-evidence
	governanceWorkCmd.AddCommand(govWorkRecordEvidenceCmd)
	govWorkRecordEvidenceCmd.Flags().String("kind", "", "Evidence kind (test|ci|review|deploy|monitoring|manual)")
	govWorkRecordEvidenceCmd.Flags().String("source", "", "Evidence source (cli|github|jira|webhook|human)")
	govWorkRecordEvidenceCmd.Flags().String("summary", "", "Evidence summary")
	govWorkRecordEvidenceCmd.Flags().String("url", "", "Evidence URL")
	govWorkRecordEvidenceCmd.Flags().String("raw", "", "Raw evidence pointer/output")

	// work ready-for-test
	governanceWorkCmd.AddCommand(govWorkReadyForTestCmd)

	// work done
	governanceWorkCmd.AddCommand(govWorkDoneCmd)
	govWorkDoneCmd.Flags().String("by", "", "Review authority ID marking the work done (must hold mark_done, e.g. tester_ai, ci_verifier, pm)")

	// work history
	governanceWorkCmd.AddCommand(govWorkHistoryCmd)
	govWorkHistoryCmd.Flags().Bool("json", false, "Output as JSON")
}
