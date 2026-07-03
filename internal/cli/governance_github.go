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

func init() {
	governanceCmd.AddCommand(governanceGitHubCmd)
	governanceGitHubCmd.AddCommand(govGitHubPollCmd)
	govGitHubPollCmd.Flags().String("project", "", "Project ID")
	govGitHubPollCmd.Flags().String("repo", "", "GitHub repo (owner/repo)")
	govGitHubPollCmd.Flags().Bool("json", false, "Output as JSON")
}
