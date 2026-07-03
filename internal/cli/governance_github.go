package cli

// GitHub inbound integration: poll issue comments for /openexec commands and
// drive the governance workflow, so humans steer work from GitHub instead of
// copy-pasting into consoles. This is the primitive a webhook/GitHub Action
// would wrap (like Claude Code's issue-tracker integration), but governance-gated.

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

var governanceGitHubCmd = &cobra.Command{
	Use:   "github",
	Short: "GitHub inbound integration (poll issue comments for /openexec commands)",
}

var govGitHubPollCmd = &cobra.Command{
	Use:   "poll",
	Short: "Process new /openexec commands in a project's GitHub issue comments",
	Long: `Poll the GitHub issues backing this project's change records and execute any
new /openexec commands (review, approve, reject, defer, revise, ready-for-test)
found in their comments, replying with the result.

Authorization: only GitHub logins listed in the operator-owned approver map
(~/.openexec/github-approvers.yaml, keyed login -> authority id) may drive
commands. Approve additionally requires OPENEXEC_OPERATOR_SESSION=1. An
unmapped commenter is ignored and answered with a "not authorized" reply.

Pair with the /loop skill or a cron/GitHub Action to poll continuously.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		svc, _, db, err := newGovService(cmd)
		if err != nil {
			return err
		}
		defer db.Close()

		projectID, _ := cmd.Flags().GetString("project")
		repo, _ := cmd.Flags().GetString("repo")
		if projectID == "" || repo == "" {
			return fmt.Errorf("--project <id> and --repo <owner/repo> are required")
		}

		approvers, err := loadGitHubApprovers()
		if err != nil {
			return err
		}

		rep, err := svc.IngestGitHubComments(cmd.Context(), projectID, repo, approvers)
		if err != nil {
			return err
		}
		if jsonOut, _ := cmd.Flags().GetBool("json"); jsonOut {
			return printJSON(cmd, rep)
		}
		cmd.Printf("Scanned %d github-sourced change(s); processed %d command(s).\n", rep.ScannedChanges, len(rep.Actions))
		for _, a := range rep.Actions {
			mark := "refused"
			if a.Applied {
				mark = "applied"
			}
			cmd.Printf("  [%s] issue #%d /openexec %s by @%s -> %s\n", mark, a.IssueNum, a.Command, a.Author, a.Message)
		}
		return nil
	},
}

// gitHubApproverConfig is the operator-owned mapping of GitHub login to review
// authority id. It lives in ~/.openexec (outside any agent-writable workspace)
// so an agent cannot add itself as an approver.
type gitHubApproverConfig struct {
	Approvers map[string]string `yaml:"approvers"`
}

// loadGitHubApprovers reads ~/.openexec/github-approvers.yaml. A missing file
// yields an empty (deny-all) map — safe default: no author is authorized until
// the operator maps them.
func loadGitHubApprovers() (map[string]string, error) {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return map[string]string{}, nil
	}
	path := filepath.Join(home, ".openexec", "github-approvers.yaml")
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return map[string]string{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	var cfg gitHubApproverConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	if cfg.Approvers == nil {
		return map[string]string{}, nil
	}
	return cfg.Approvers, nil
}

var govGitHubSyncCmd = &cobra.Command{
	Use:   "sync",
	Short: "Import (and auto-triage) a repo's open issues as change records",
	Long: `Discover open issues in a repo (optionally filtered to --label, e.g.
"ai:triage"), import each as a change record, and — unless --no-triage — deep-
triage freshly-imported ones into stories/tasks so they arrive as reviewable
interpretations awaiting your approval.

This is the passive automation input. Idempotent: re-running never duplicates,
and only fresh (untriaged) issues are triaged, so it is safe to schedule.
Individual repos and org repos both work — access follows your gh/GH_TOKEN auth.

Schedule it, e.g. once a day:
  openexec schedule "0 9 * * *" -- governance github sync --project p --repo owner/repo --label ai:triage
or watch continuously with /loop.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		svc, _, db, err := newGovService(cmd)
		if err != nil {
			return err
		}
		defer db.Close()

		projectID, _ := cmd.Flags().GetString("project")
		repo, _ := cmd.Flags().GetString("repo")
		if projectID == "" || repo == "" {
			return fmt.Errorf("--project <id> and --repo <owner/repo> are required")
		}
		label, _ := cmd.Flags().GetString("label")
		noTriage, _ := cmd.Flags().GetBool("no-triage")

		rep, err := svc.SyncGitHubIssues(cmd.Context(), projectID, repo, label, !noTriage)
		if err != nil {
			return err
		}
		if jsonOut, _ := cmd.Flags().GetBool("json"); jsonOut {
			return printJSON(cmd, rep)
		}
		cmd.Printf("Synced %s (%d open issue(s)%s)\n", rep.Repo, rep.Scanned, labelSuffix(label))
		for _, a := range rep.Actions {
			switch {
			case a.Error != "":
				cmd.Printf("  error issue #%d: %s\n", a.IssueNum, a.Error)
			case a.Triaged:
				cmd.Printf("  triaged issue #%d -> %s (%d stor[y|ies]) — awaiting approval\n", a.IssueNum, a.ChangeID, len(a.StoryIDs))
			case a.Imported:
				cmd.Printf("  imported issue #%d -> %s (candidate)\n", a.IssueNum, a.ChangeID)
			default:
				cmd.Printf("  up-to-date issue #%d -> %s\n", a.IssueNum, a.ChangeID)
			}
		}
		return nil
	},
}

func labelSuffix(label string) string {
	if label == "" {
		return ""
	}
	return ", label=" + label
}

func init() {
	governanceCmd.AddCommand(governanceGitHubCmd)

	governanceGitHubCmd.AddCommand(govGitHubPollCmd)
	govGitHubPollCmd.Flags().String("project", "", "Project ID")
	govGitHubPollCmd.Flags().String("repo", "", "GitHub repo (owner/repo)")
	govGitHubPollCmd.Flags().Bool("json", false, "Output as JSON")

	governanceGitHubCmd.AddCommand(govGitHubSyncCmd)
	govGitHubSyncCmd.Flags().String("project", "", "Project ID")
	govGitHubSyncCmd.Flags().String("repo", "", "GitHub repo (owner/repo)")
	govGitHubSyncCmd.Flags().String("label", "", "Only import issues with this label (e.g. ai:triage)")
	govGitHubSyncCmd.Flags().Bool("no-triage", false, "Import only; do not auto-deep-triage")
	govGitHubSyncCmd.Flags().Bool("json", false, "Output as JSON")
}
