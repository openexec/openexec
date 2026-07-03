package cli

// Release Governance CLI (Phase 4 of docs/RELEASE_GOVERNANCE_IMPLEMENTATION_PLAN.md).
//
// These commands are a THIN ADAPTER over internal/governance/service. They parse
// input, call exactly one Service method, and render the result — they never
// re-implement validation, policy, or store logic.
//
// To avoid colliding with the existing git-branch `release` command group (see
// release.go), every governance command lives under a NEW top-level parent
// command `governance` (alias `gov`): `openexec governance release <sub>` and
// `openexec governance work <sub>`. The existing release/story/task commands are
// untouched.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	pagent "github.com/openexec/openexec/pkg/agent"

	"github.com/openexec/openexec/internal/governance"
	"github.com/openexec/openexec/internal/governance/ai"
	"github.com/openexec/openexec/internal/governance/connectors/github"
	"github.com/openexec/openexec/internal/governance/policy"
	"github.com/openexec/openexec/internal/governance/service"
	"github.com/openexec/openexec/internal/project"
	"github.com/openexec/openexec/internal/runner"
	oxruntime "github.com/openexec/openexec/pkg/runtime"
	"github.com/spf13/cobra"
)

// multiCloser closes several resources, returning the first error.
type multiCloser []io.Closer

func (m multiCloser) Close() error {
	var err error
	for _, c := range m {
		if e := c.Close(); e != nil && err == nil {
			err = e
		}
	}
	return err
}

var governanceCmd = &cobra.Command{
	Use:     "governance",
	Aliases: []string{"gov"},
	Short:   "Release governance: govern what work is allowed, track evidence, generate handoffs",
	Long: `Release governance commands.

OpenExec decides what work is allowed, tracks evidence, and records
communication. Execution engines (Codex/Claude) write the code.

This is a separate command group from the git-branch ` + "`release`" + ` group:
  governance release ...   manage governance releases and their scope
  governance work ...      manage change records through the work lifecycle`,
}

var governanceReleaseCmd = &cobra.Command{
	Use:   "release",
	Short: "Manage governance releases (create, attach work, approve, start, status, handoff)",
}

// --- wiring helpers -------------------------------------------------------

// govBaseDir resolves the project base dir the same way the existing release
// commands do: the --project-dir flag, else the current working directory. It
// ensures the .openexec directory exists so governance.Open can create the db.
func govBaseDir(cmd *cobra.Command) (string, error) {
	baseDir, _ := cmd.Flags().GetString("project-dir")
	if baseDir == "" {
		var err error
		baseDir, err = os.Getwd()
		if err != nil {
			return "", err
		}
	}
	if err := os.MkdirAll(filepath.Join(baseDir, ".openexec"), 0o750); err != nil {
		return "", fmt.Errorf("create .openexec dir: %w", err)
	}
	return baseDir, nil
}

// newGovService opens the governance store and wires a Service with policy,
// the real gh runner, and a best-effort AI completer. The caller MUST close the
// returned *sql.DB. The store is returned too for read-only display commands
// (status), which read records directly rather than re-implementing logic.
func newGovService(cmd *cobra.Command) (*service.Service, governance.Store, io.Closer, error) {
	baseDir, err := govBaseDir(cmd)
	if err != nil {
		return nil, nil, nil, err
	}
	db, store, err := governance.Open(baseDir)
	if err != nil {
		return nil, nil, nil, err
	}
	pol, err := policy.LoadPolicy(baseDir)
	if err != nil {
		db.Close()
		return nil, nil, nil, fmt.Errorf("load policy: %w", err)
	}
	// PlanStore for deep triage: a git-disabled release manager persists the
	// planner-generated stories/tasks into the same .openexec/openexec.db. A
	// failure here is non-fatal — deep triage returns a clear error, other
	// commands are unaffected.
	var planStore service.PlanStore
	closers := []io.Closer{db}
	if relMgr, rErr := oxruntime.NewBacklogManager(baseDir); rErr == nil {
		planStore = relMgr
		closers = append([]io.Closer{relMgr}, closers...) // close release mgr before governance db
	}
	svc := service.NewService(store, service.Options{
		Policy:          pol,
		Runner:          &github.ExecRunner{},
		Completer:       buildGovCompleter(baseDir),
		PlanStore:       planStore,
		Executor:        &shellExecutor{baseDir: baseDir},
		OperatorSession: os.Getenv("OPENEXEC_OPERATOR_SESSION") == "1",
	})
	return svc, store, multiCloser(closers), nil
}

// shellExecutor runs an approved task through the EXISTING engine by invoking
// `openexec run <taskID>` as a subprocess in the project dir. It reuses the full
// execution stack (daemon, blueprint loop, git/PR) rather than reimplementing
// it — governance decides what runs, `openexec run` does the code work.
type shellExecutor struct{ baseDir string }

func (e *shellExecutor) RunTask(ctx context.Context, taskID, mode string) error {
	args := []string{"run", taskID}
	if mode != "" {
		args = append(args, "--mode", mode)
	}
	cmd := exec.CommandContext(ctx, os.Args[0], args...)
	cmd.Dir = e.baseDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	return cmd.Run()
}

// buildGovCompleter constructs an ai.Completer from the project's active API
// provider, mirroring the OpenAI-compatible provider construction in
// internal/pipeline (createAPIAgenticLoop). It is best-effort: any
// missing/invalid configuration yields a nil completer, and the Service returns
// a clear "AI completer not configured" error for the operations (triage,
// review-plan) that actually need it. Non-AI commands are unaffected.
func buildGovCompleter(baseDir string) ai.Completer {
	cfg, err := project.LoadProjectConfig(baseDir)
	if err != nil || cfg == nil {
		return nil
	}
	// Prefer an explicitly-configured OpenAI-compatible API.
	name, baseURL, apiKey, model := cfg.Execution.ActiveAPI()
	if model != "" {
		if strings.HasPrefix(apiKey, "$") {
			apiKey = os.Getenv(strings.TrimPrefix(apiKey, "$"))
		}
		if apiKey != "" {
			if prov, err := pagent.NewOpenAIProvider(pagent.OpenAIProviderConfig{
				APIKey:  apiKey,
				BaseURL: baseURL,
				Name:    name,
				Models:  append(pagent.DefaultOpenAIModels(), model),
			}); err == nil {
				return &providerCompleter{provider: prov, model: model}
			}
		}
	}
	// Fall back to the CLI-runner path the planner uses (claude/codex/gemini),
	// so governance triage works on the same config as `openexec run` — a
	// project configured with planner_model/executor_model (no API section)
	// drives triage through the resolved runner CLI.
	plannerModel := cfg.Execution.PlannerModel
	if plannerModel == "" {
		plannerModel = "sonnet"
	}
	return &cliCompleter{model: plannerModel}
}

// cliCompleter drives governance triage through the same runner CLI the planner
// uses (mirrors pkg/manager.cliLLMProvider): it resolves the model to a CLI
// (claude/codex/gemini) and feeds the prompt on stdin.
type cliCompleter struct{ model string }

func (c *cliCompleter) Complete(ctx context.Context, prompt string) (string, error) {
	cliCmd, cmdArgs, err := runner.Resolve(
		c.model,
		os.Getenv("OPENEXEC_PLANNER_CLI"),
		strings.Fields(os.Getenv("OPENEXEC_PLANNER_ARGS")),
	)
	if err != nil {
		return "", err
	}
	if strings.Contains(strings.ToLower(cliCmd), "claude") {
		cmdArgs = []string{"--print"}
	}
	cmd := exec.CommandContext(ctx, cliCmd, cmdArgs...)
	cmd.Stdin = strings.NewReader(prompt)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("governance planner CLI (%s) failed: %w\n%s", cliCmd, err, string(out))
	}
	return string(out), nil
}

// providerCompleter adapts a pkg/agent ProviderAdapter to the ai.Completer
// interface the governance AI actors depend on: a single user-message completion.
type providerCompleter struct {
	provider pagent.ProviderAdapter
	model    string
}

func (c *providerCompleter) Complete(ctx context.Context, prompt string) (string, error) {
	resp, err := c.provider.Complete(ctx, pagent.Request{
		Model:     c.model,
		Messages:  []pagent.Message{pagent.NewTextMessage(pagent.RoleUser, prompt)},
		MaxTokens: 4096,
	})
	if err != nil {
		return "", err
	}
	return resp.GetText(), nil
}

// printJSON renders v as indented JSON to the command's output.
func printJSON(cmd *cobra.Command, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	cmd.Println(string(data))
	return nil
}

// --- release subcommands --------------------------------------------------

var govReleaseCreateCmd = &cobra.Command{
	Use:   "create <release-id>",
	Short: "Create a governance release (in draft status)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		svc, _, db, err := newGovService(cmd)
		if err != nil {
			return err
		}
		defer db.Close()

		name, _ := cmd.Flags().GetString("name")
		owner, _ := cmd.Flags().GetString("owner")

		rel, err := svc.CreateRelease(cmd.Context(), args[0], name, owner)
		if err != nil {
			return err
		}
		cmd.Printf("Created release %s [%s]\n", rel.ID, rel.Status)
		if rel.Name != "" {
			cmd.Printf("  Name: %s\n", rel.Name)
		}
		if rel.Owner != "" {
			cmd.Printf("  Owner: %s\n", rel.Owner)
		}
		return nil
	},
}

var govReleaseAddCmd = &cobra.Command{
	Use:   "add <release-id> <change-id>",
	Short: "Attach a change record to a release as a release item",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		svc, _, db, err := newGovService(cmd)
		if err != nil {
			return err
		}
		defer db.Close()

		priority, _ := cmd.Flags().GetInt("priority")
		required, _ := cmd.Flags().GetBool("required")

		if err := svc.AttachChange(cmd.Context(), args[0], args[1], priority, required); err != nil {
			return err
		}
		cmd.Printf("Attached change %s to release %s (priority %d, required %t)\n", args[1], args[0], priority, required)
		return nil
	},
}

var govReleaseApproveCmd = &cobra.Command{
	Use:   "approve <release-id>",
	Short: "Approve a release's scope, opening its approved work to AI executors",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		svc, _, db, err := newGovService(cmd)
		if err != nil {
			return err
		}
		defer db.Close()

		by, _ := cmd.Flags().GetString("by")
		if by == "" {
			return fmt.Errorf("--by <authorityID> is required (seeded authorities: pm, developer, bugbot, tester_ai, security_ai, ci_verifier)")
		}
		if err := svc.ApproveRelease(cmd.Context(), args[0], by); err != nil {
			return err
		}
		cmd.Printf("Release %s approved by %s\n", args[0], by)
		return nil
	},
}

var govReleasePlanCmd = &cobra.Command{
	Use:   "plan <release-id>",
	Short: "Move a draft release to planned (ready for scope approval)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		svc, _, db, err := newGovService(cmd)
		if err != nil {
			return err
		}
		defer db.Close()

		if err := svc.PlanRelease(cmd.Context(), args[0]); err != nil {
			return err
		}
		cmd.Printf("Release %s is now planned\n", args[0])
		return nil
	},
}

var govReleaseStartCmd = &cobra.Command{
	Use:   "start <release-id>",
	Short: "Move an approved release into implementing",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		svc, _, db, err := newGovService(cmd)
		if err != nil {
			return err
		}
		defer db.Close()

		if err := svc.StartRelease(cmd.Context(), args[0]); err != nil {
			return err
		}
		cmd.Printf("Release %s is now implementing\n", args[0])
		return nil
	},
}

var govReleaseExecuteCmd = &cobra.Command{
	Use:   "execute <release-id>",
	Short: "Build all approved work in a release in one go (or a --changes subset)",
	Long: `Run every approved change in the release through the execution engine in one
batch (or restrict to --changes a,b,c). Non-approved items are skipped. The
release is advanced to implementing.

Trunk-based: bundle a small set of fixes, execute, open one short-lived release
PR, merge to trunk, delete the branch — keep the batch small and frequent
rather than accumulating a long-lived branch.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		svc, _, db, err := newGovService(cmd)
		if err != nil {
			return err
		}
		defer db.Close()

		var only []string
		if csv, _ := cmd.Flags().GetString("changes"); csv != "" {
			for _, id := range strings.Split(csv, ",") {
				if id = strings.TrimSpace(id); id != "" {
					only = append(only, id)
				}
			}
		}
		agent, _ := cmd.Flags().GetString("agent")
		mode, _ := cmd.Flags().GetString("mode")

		rep, err := svc.ExecuteRelease(cmd.Context(), args[0], only, agent, mode)
		if err != nil {
			return err
		}
		if jsonOut, _ := cmd.Flags().GetBool("json"); jsonOut {
			return printJSON(cmd, rep)
		}
		cmd.Printf("Release %s: executed %d change(s), skipped %d, %d error(s)\n",
			rep.ReleaseID, len(rep.Executed), len(rep.Skipped), len(rep.Errors))
		for _, r := range rep.Executed {
			cmd.Printf("  built %s (%d task(s))\n", r.ChangeID, len(r.DispatchedTasks))
		}
		for _, s := range rep.Skipped {
			cmd.Printf("  skip  %s\n", s)
		}
		for id, msg := range rep.Errors {
			cmd.Printf("  error %s: %s\n", id, msg)
		}
		return nil
	},
}

var govReleaseStatusCmd = &cobra.Command{
	Use:   "status <release-id>",
	Short: "Show a release and its change records",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		_, store, db, err := newGovService(cmd)
		if err != nil {
			return err
		}
		defer db.Close()

		ctx := cmd.Context()
		rel, err := store.GetRelease(ctx, args[0])
		if err != nil {
			if errors.Is(err, governance.ErrNotFound) {
				return fmt.Errorf("release %q not found", args[0])
			}
			return err
		}
		changes, err := store.ListChangeRecordsByRelease(ctx, args[0])
		if err != nil {
			return err
		}

		if jsonOut, _ := cmd.Flags().GetBool("json"); jsonOut {
			return printJSON(cmd, map[string]any{
				"release": rel,
				"changes": changes,
			})
		}

		cmd.Printf("Release: %s [%s]\n", rel.ID, rel.Status)
		if rel.Name != "" {
			cmd.Printf("  Name: %s\n", rel.Name)
		}
		if rel.Owner != "" {
			cmd.Printf("  Owner: %s\n", rel.Owner)
		}
		cmd.Printf("  Risk: %s\n", rel.Risk)
		cmd.Printf("  Approved for AI: %t\n", rel.ApprovedForAI)
		cmd.Printf("\nChange records (%d):\n", len(changes))
		if len(changes) == 0 {
			cmd.Println("  (none attached)")
			return nil
		}
		for _, ch := range changes {
			cmd.Printf("  - %s [%s] %s (risk=%s, plan v%d, approved v%d)\n",
				ch.ID, ch.Status, ch.Title, ch.Risk, ch.ProposalVersion, ch.ApprovedVersion)
		}
		return nil
	},
}

var govReleaseHandoffCmd = &cobra.Command{
	Use:   "handoff <release-id>",
	Short: "Generate an audience-targeted communication artifact for a release",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		svc, _, db, err := newGovService(cmd)
		if err != nil {
			return err
		}
		defer db.Close()

		audience, _ := cmd.Flags().GetString("audience")
		env, _ := cmd.Flags().GetString("env")
		version, _ := cmd.Flags().GetString("version")
		if audience == "" {
			return fmt.Errorf("--audience is required (tester|pm|customer)")
		}

		body, err := svc.GenerateHandoff(cmd.Context(), args[0], audience, env, version)
		if err != nil {
			return err
		}
		cmd.Println(body)
		return nil
	},
}

func init() {
	// Register the single new top-level command group. This is the ONLY change
	// to the root command surface; existing release/story/task commands are
	// untouched.
	rootCmd.AddCommand(governanceCmd)

	// A --project-dir persistent flag scoped to the governance group, mirroring
	// the project-dir lookup the existing release commands perform.
	governanceCmd.PersistentFlags().String("project-dir", "", "Project directory (default: current working directory)")

	governanceCmd.AddCommand(governanceReleaseCmd)
	governanceCmd.AddCommand(governanceWorkCmd)

	// release create
	governanceReleaseCmd.AddCommand(govReleaseCreateCmd)
	govReleaseCreateCmd.Flags().String("name", "", "Release name")
	govReleaseCreateCmd.Flags().String("owner", "", "Release owner")

	// release add
	governanceReleaseCmd.AddCommand(govReleaseAddCmd)
	govReleaseAddCmd.Flags().Int("priority", 0, "Item priority")
	govReleaseAddCmd.Flags().Bool("required", false, "Mark the item as required")

	// release plan
	governanceReleaseCmd.AddCommand(govReleasePlanCmd)

	// release approve
	governanceReleaseCmd.AddCommand(govReleaseApproveCmd)
	govReleaseApproveCmd.Flags().String("by", "", "Approving authority ID (e.g. pm)")

	// release start
	governanceReleaseCmd.AddCommand(govReleaseStartCmd)

	// release execute (batch)
	governanceReleaseCmd.AddCommand(govReleaseExecuteCmd)
	govReleaseExecuteCmd.Flags().String("changes", "", "Comma-separated change ids to build (default: all approved in the release)")
	govReleaseExecuteCmd.Flags().String("agent", "openexec-executor", "Executor name recorded on claims")
	govReleaseExecuteCmd.Flags().String("mode", "workspace-write", "Execution mode")
	govReleaseExecuteCmd.Flags().Bool("json", false, "Output as JSON")

	// release status
	governanceReleaseCmd.AddCommand(govReleaseStatusCmd)
	govReleaseStatusCmd.Flags().Bool("json", false, "Output as JSON")

	// release handoff
	governanceReleaseCmd.AddCommand(govReleaseHandoffCmd)
	govReleaseHandoffCmd.Flags().String("audience", "", "Audience: tester|pm|customer")
	govReleaseHandoffCmd.Flags().String("env", "", "Target environment (tester handoff only)")
	govReleaseHandoffCmd.Flags().String("version", "", "Release version (tester handoff only)")
}
