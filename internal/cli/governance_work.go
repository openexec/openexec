package cli

// Work-item subcommands for the governance group (see governance.go). These are
// thin adapters over internal/governance/service: each parses input, calls one
// Service method, and renders the result.

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/openexec/openexec/internal/governance"
	"github.com/openexec/openexec/internal/governance/service"
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

var govWorkCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "File a change record from free text (no GitHub issue needed)",
	RunE: func(cmd *cobra.Command, args []string) error {
		svc, _, db, err := newGovService(cmd)
		if err != nil {
			return err
		}
		defer db.Close()

		projectID, _ := cmd.Flags().GetString("project")
		title, _ := cmd.Flags().GetString("title")
		body, _ := cmd.Flags().GetString("body")
		if title == "" {
			return fmt.Errorf("--title is required")
		}
		ch, err := svc.CreateManualChange(cmd.Context(), projectID, title, body)
		if err != nil {
			return err
		}
		cmd.Printf("Created change %s [%s]: %s\n", ch.ID, ch.Status, ch.Title)
		cmd.Printf("Next: openexec governance work triage %s --deep\n", ch.ID)
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
		svc, store, db, err := newGovService(cmd)
		if err != nil {
			return err
		}
		defer db.Close()

		repoContext, _ := cmd.Flags().GetString("context")
		actor, _ := cmd.Flags().GetString("actor")

		// When no explicit context is given, gather relevant code so deep triage
		// references real files ("affects this and that"): rank the repo's files
		// by the change's intent and feed the top files' contents to the planner
		// and the impact analysis. --repo-context scans a code repo separate from
		// the governance project dir (so governance state stays isolated from the
		// code repo's backlog); it defaults to the project dir. Best-effort.
		if repoContext == "" {
			codeDir, _ := cmd.Flags().GetString("repo-context")
			if codeDir == "" {
				codeDir, _ = govBaseDir(cmd)
			}
			if codeDir != "" {
				intent := ""
				if ch, e := store.GetChangeRecord(cmd.Context(), args[0]); e == nil {
					intent = ch.Title + " " + ch.RawText
				}
				repoContext = gatherRelevantFiles(codeDir, intent, 10, 16000)
			}
		}

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

var govWorkQuickplanCmd = &cobra.Command{
	Use:   "quickplan <change-id>",
	Short: "Lightweight lane: prepare a TRIVIAL change as a single task, no planner",
	Long: `Prepare a trivial change for approval without running the planner. Instead of
decomposing intent into a full story/task tree (and a round of planner-output
review), it builds one story with one task whose description is the change's
intent. The change is marked light, so a human operator can approve it without
the AI review its risk tier would otherwise require — the operator is the
reviewer. Refused for high/critical risk; use 'work triage --deep' for those.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		svc, _, db, err := newGovService(cmd)
		if err != nil {
			return err
		}
		defer db.Close()

		actor, _ := cmd.Flags().GetString("actor")
		res, err := svc.TriageLight(cmd.Context(), args[0], actor)
		if err != nil {
			return err
		}
		if jsonOut, _ := cmd.Flags().GetBool("json"); jsonOut {
			return printJSON(cmd, res)
		}
		cmd.Printf("Light-triaged change %s into a single task (%d stor(y): %s)\n", args[0], len(res.StoryIDs), strings.Join(res.StoryIDs, ", "))
		cmd.Printf("Next: openexec governance work approve %s --by <operator> (OPERATOR_SESSION), then work execute %s\n", args[0], args[0])
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

var govWorkOperabilityCmd = &cobra.Command{
	Use:   "operability <change-id>",
	Short: "Show the operability review (rollback, DB migration, deploy risk)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		svc, _, db, err := newGovService(cmd)
		if err != nil {
			return err
		}
		defer db.Close()

		rep, err := svc.ChangeOperability(cmd.Context(), args[0])
		if err != nil {
			return err
		}
		if jsonOut, _ := cmd.Flags().GetBool("json"); jsonOut {
			return printJSON(cmd, rep)
		}
		if rep == nil {
			cmd.Printf("No operability review for %s (re-run: work triage %s --deep --repo-context <path>)\n", args[0], args[0])
			return nil
		}
		cmd.Printf("Operability review for %s:\n", args[0])
		cmd.Printf("  rollback safe: %s\n", rep.RollbackSafe)
		cmd.Printf("  db migration:  %s\n", rep.DBMigration)
		cmd.Printf("  deploy risk:   %s\n", rep.DeployRisk)
		if len(rep.Mitigations) > 0 {
			cmd.Println("  mitigations:")
			for _, m := range rep.Mitigations {
				cmd.Printf("    - %s\n", m)
			}
		}
		if len(rep.Monitoring) > 0 {
			cmd.Println("  monitor after deploy:")
			for _, m := range rep.Monitoring {
				cmd.Printf("    - %s\n", m)
			}
		}
		if rep.Notes != "" {
			cmd.Printf("  notes: %s\n", rep.Notes)
		}
		safe := "NO — a human operator must merge"
		if rep.AutoMergeSafe() {
			safe = "yes (rollback-safe, non-destructive, low/medium risk)"
		}
		cmd.Printf("  auto-merge safe: %s\n", safe)
		return nil
	},
}

var govWorkMergeCmd = &cobra.Command{
	Use:   "merge <change-id>",
	Short: "Merge a change's PR (the safety gate — never auto-merges by default)",
	Long: `Merge the pull request for a change. This is the safety gate: it refuses unless
either a human operator session (OPENEXEC_OPERATOR_SESSION=1) with an approve
authority runs it, or policy explicitly opts the change's risk tier into
auto-merge AND verification evidence exists. By default NOTHING auto-merges, so
a change can never accidentally trigger a CI/CD production deploy.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		svc, _, db, err := newGovService(cmd)
		if err != nil {
			return err
		}
		defer db.Close()

		by, _ := cmd.Flags().GetString("by")
		method, _ := cmd.Flags().GetString("method")
		if err := svc.MergeChange(cmd.Context(), args[0], by, method); err != nil {
			return err
		}
		cmd.Printf("Merged change %s (%s)\n", args[0], method)
		return nil
	},
}

var govWorkImpactCmd = &cobra.Command{
	Use:   "impact <change-id>",
	Short: "Show the file-level impact analysis (which files the change touches)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		svc, _, db, err := newGovService(cmd)
		if err != nil {
			return err
		}
		defer db.Close()

		rep, err := svc.ChangeImpact(cmd.Context(), args[0])
		if err != nil {
			return err
		}
		if jsonOut, _ := cmd.Flags().GetBool("json"); jsonOut {
			return printJSON(cmd, rep)
		}
		if rep == nil || len(rep.Files) == 0 {
			cmd.Printf("No file-level impact recorded for %s (re-run: work triage %s --deep --repo-context <path>)\n", args[0], args[0])
			return nil
		}
		cmd.Printf("Change %s affects %d file(s):\n", args[0], len(rep.Files))
		for _, f := range rep.Files {
			cmd.Printf("  [%s] %s\n        %s\n", f.Action, f.Path, f.Reason)
		}
		if rep.Notes != "" {
			cmd.Printf("Notes: %s\n", rep.Notes)
		}
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

// assessAndPostToPR generates the governance risk assessment (file-level impact
// + operability) for a change, records it in the audit trail, and posts it to
// the change's PR. Repo context is gathered from the project dir by the change's
// intent so the model reads real files. Shared by record-pr (automatic, so every
// PR carries the assessment) and the standalone pr-assess command.
func assessAndPostToPR(cmd *cobra.Command, svc *service.Service, store governance.Store, changeID string) error {
	ch, err := store.GetChangeRecord(cmd.Context(), changeID)
	if err != nil {
		return err
	}
	baseDir, _ := govBaseDir(cmd)
	repoCtx := gatherRelevantFiles(baseDir, ch.Title+" "+ch.RawText, 10, 16000)
	if _, _, err := svc.AssessChange(cmd.Context(), changeID, repoCtx); err != nil {
		return err
	}
	return svc.PostPRAssessment(cmd.Context(), changeID)
}

var govWorkRecordPRCmd = &cobra.Command{
	Use:   "record-pr <change-id>",
	Short: "Record the pull request opened for a change (auto-posts the governance assessment)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		svc, store, db, err := newGovService(cmd)
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

		// Every PR carries the AI risk assessment (impact + operability), recorded
		// in the audit trail and posted to the PR. Best-effort: a missing completer
		// or gh must not undo a recorded PR — warn and continue.
		cmd.Println("Assessing risk (impact + operability) and posting to the PR...")
		if err := assessAndPostToPR(cmd, svc, store, args[0]); err != nil {
			cmd.Printf("  warning: assessment not posted: %v\n", err)
		} else {
			cmd.Println("  posted governance assessment to the PR")
		}
		return nil
	},
}

var govWorkPRAssessCmd = &cobra.Command{
	Use:   "pr-assess <change-id>",
	Short: "Generate the governance impact + operability assessment, record it, and post it to the PR",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		svc, store, db, err := newGovService(cmd)
		if err != nil {
			return err
		}
		defer db.Close()

		if printOnly, _ := cmd.Flags().GetBool("print"); printOnly {
			ch, err := store.GetChangeRecord(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			baseDir, _ := govBaseDir(cmd)
			repoCtx := gatherRelevantFiles(baseDir, ch.Title+" "+ch.RawText, 10, 16000)
			if _, _, err := svc.AssessChange(cmd.Context(), args[0], repoCtx); err != nil {
				return err
			}
			md, err := svc.PRAssessmentMarkdown(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			cmd.Println(md)
			return nil
		}
		if err := assessAndPostToPR(cmd, svc, store, args[0]); err != nil {
			return err
		}
		cmd.Printf("Posted governance assessment to the PR for change %s\n", args[0])
		return nil
	},
}

var govWorkSyncChecksCmd = &cobra.Command{
	Use:   "sync-checks <change-id>",
	Short: "Fetch the PR's GitHub CI checks and record them as trusted evidence when green",
	Long: `Fetch the check-runs for the change's PR head commit from GitHub and, when they
are all green, record a TRUSTED verification evidence row (source github) with
the commit SHA and a hash of the fetched payload. This is the only way to create
trusted evidence — manual record-evidence cannot claim a github/webhook source —
so the auto-merge trust signal is always grounded in a real external check.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		svc, _, db, err := newGovService(cmd)
		if err != nil {
			return err
		}
		defer db.Close()
		if err := svc.SyncGitHubChecks(cmd.Context(), args[0]); err != nil {
			return err
		}
		cmd.Printf("Recorded verified GitHub checks as trusted evidence for change %s\n", args[0])
		return nil
	},
}

var govWorkRecordEvidenceCmd = &cobra.Command{
	Use:   "record-evidence <change-id>",
	Short: "Attach a manually-asserted evidence record (test/review/manual; NOT github/webhook)",
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

	// work create (manual)
	governanceWorkCmd.AddCommand(govWorkCreateCmd)
	govWorkCreateCmd.Flags().String("project", "", "Project ID")
	govWorkCreateCmd.Flags().String("title", "", "Change title (required)")
	govWorkCreateCmd.Flags().String("body", "", "Change description / intent")

	// work attach
	governanceWorkCmd.AddCommand(govWorkAttachCmd)
	govWorkAttachCmd.Flags().String("release", "", "Release ID to attach to")
	govWorkAttachCmd.Flags().Int("priority", 0, "Item priority")
	govWorkAttachCmd.Flags().Bool("required", false, "Mark the item as required")

	// work triage
	governanceWorkCmd.AddCommand(govWorkTriageCmd)
	govWorkTriageCmd.Flags().String("context", "", "Repo context to give the planner")
	govWorkTriageCmd.Flags().String("repo-context", "", "Path to a code repo to scan for context (default: project dir)")
	govWorkTriageCmd.Flags().String("actor", "planner_ai", "Actor recorded as the proposer")
	govWorkTriageCmd.Flags().Bool("deep", false, "Decompose intent into real stories + tasks via the planner (owned by this change)")
	govWorkTriageCmd.Flags().Bool("json", false, "Output as JSON")

	// work quickplan (lightweight lane)
	governanceWorkCmd.AddCommand(govWorkQuickplanCmd)
	govWorkQuickplanCmd.Flags().String("actor", "operator", "Actor recorded as the proposer")
	govWorkQuickplanCmd.Flags().Bool("json", false, "Output as JSON")

	// work stories
	governanceWorkCmd.AddCommand(govWorkStoriesCmd)
	govWorkStoriesCmd.Flags().Bool("json", false, "Output as JSON")

	// work impact
	governanceWorkCmd.AddCommand(govWorkImpactCmd)
	govWorkImpactCmd.Flags().Bool("json", false, "Output as JSON")

	// work operability
	governanceWorkCmd.AddCommand(govWorkOperabilityCmd)
	govWorkOperabilityCmd.Flags().Bool("json", false, "Output as JSON")

	// work merge (safety gate)
	governanceWorkCmd.AddCommand(govWorkMergeCmd)
	govWorkMergeCmd.Flags().String("by", "", "Approving authority ID (e.g. pm); required in an operator session")
	govWorkMergeCmd.Flags().String("method", "squash", "Merge method: squash | merge | rebase")

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

	// work pr-assess
	governanceWorkCmd.AddCommand(govWorkPRAssessCmd)
	govWorkPRAssessCmd.Flags().Bool("print", false, "Print the assessment markdown instead of posting to the PR")

	// work record-evidence
	governanceWorkCmd.AddCommand(govWorkSyncChecksCmd)

	governanceWorkCmd.AddCommand(govWorkRecordEvidenceCmd)
	govWorkRecordEvidenceCmd.Flags().String("kind", "", "Evidence kind (test|review|deploy|monitoring|manual)")
	govWorkRecordEvidenceCmd.Flags().String("source", "", "Evidence source (cli|agent|human|manual; github/webhook are connector-only, use sync-checks)")
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
