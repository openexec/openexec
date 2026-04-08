package cli

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/openexec/openexec/internal/quality/nostubs"
	"github.com/spf13/cobra"
)

var (
	auditPath   string
	auditRules  string
	auditLang   string
	auditReport string
	auditOut    string
)

// ruleGroup maps a friendly keyword to a set of rule IDs.
var ruleGroups = map[string][]string{
	"all": nil, // nil means "all rules".
	"stubs": {
		nostubs.RuleMockConstants,
		nostubs.RuleEmptyHandler,
		nostubs.RuleNotImplementedThrow,
	},
	"mocks": {
		nostubs.RuleMockConstants,
		nostubs.RuleMockImports,
	},
	"tests": {
		nostubs.RuleTodoComments,
		nostubs.RuleServerNoAwait,
	},
}

var auditCmd = &cobra.Command{
	Use:   "audit",
	Short: "Scan a path for stub patterns and emit a markdown/JSON report",
	Long: `audit runs the no-stubs verifier against a directory tree and prints a
report of any stub/mock/placeholder patterns it finds.

The audit command is read-only — it always exits 0. The quality gate wired
into the execution pipeline is what fails the build on HIGH-severity findings.`,
	RunE: runAudit,
}

func init() {
	auditCmd.Flags().StringVar(&auditPath, "path", ".", "path to scan")
	auditCmd.Flags().StringVar(&auditRules, "rules", "all", "rule set: all | stubs | mocks | tests")
	auditCmd.Flags().StringVar(&auditLang, "lang", "", "comma-separated languages (ts,go,py). empty = all")
	auditCmd.Flags().StringVar(&auditReport, "report", "markdown", "report format: markdown | json")
	auditCmd.Flags().StringVar(&auditOut, "out", "", "write report to file instead of stdout")
	rootCmd.AddCommand(auditCmd)
}

func runAudit(cmd *cobra.Command, args []string) error {
	_ = cmd
	_ = args

	ruleIDs, ok := ruleGroups[strings.ToLower(strings.TrimSpace(auditRules))]
	if !ok {
		return fmt.Errorf("unknown --rules value %q (want: all, stubs, mocks, tests)", auditRules)
	}

	var languages []string
	if strings.TrimSpace(auditLang) != "" {
		for _, l := range strings.Split(auditLang, ",") {
			l = strings.TrimSpace(l)
			if l != "" {
				languages = append(languages, l)
			}
		}
	}

	cfg := nostubs.Config{
		Languages: languages,
		RuleIDs:   ruleIDs,
	}

	findings, err := nostubs.Run(auditPath, cfg)
	if err != nil {
		return fmt.Errorf("audit: %w", err)
	}

	var out io.Writer = os.Stdout
	if auditOut != "" {
		f, err := os.Create(auditOut) // #nosec G304
		if err != nil {
			return fmt.Errorf("create %s: %w", auditOut, err)
		}
		defer func() { _ = f.Close() }()
		out = f
	}

	switch strings.ToLower(auditReport) {
	case "json":
		if err := nostubs.WriteJSON(findings, out); err != nil {
			return err
		}
	case "markdown", "md", "":
		if err := nostubs.WriteMarkdown(findings, out); err != nil {
			return err
		}
	default:
		return fmt.Errorf("unknown --report value %q (want: markdown, json)", auditReport)
	}

	return nil
}
