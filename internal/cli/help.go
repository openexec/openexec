package cli

import (
	"bytes"
	"encoding/json"
	"sort"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// helpCmd extends Cobra's default help with agent-friendly options.
//
// Flags:
//
//	--all   Aggregate help for all commands/subcommands
//	--json  Output a JSON description of the entire CLI (commands, flags)
var helpCmd = &cobra.Command{
	Use:   "help",
	Short: "Show help for commands (agent-friendly options available)",
	Long: `Extended help:

  openexec help --all    # Aggregate help for all commands
  openexec help --json   # Output full CLI schema as JSON

Without flags, behaves like standard help.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		showAll, _ := cmd.Flags().GetBool("all")
		asJSON, _ := cmd.Flags().GetBool("json")

		if asJSON {
			schema := buildCLISchema(rootCmd)
			data, err := json.MarshalIndent(schema, "", "  ")
			if err != nil {
				return err
			}
			cmd.Println(string(data))
			return nil
		}

		if showAll {
			// Print aggregated help for all commands
			var buf bytes.Buffer
			writeAllHelp(&buf, rootCmd)
			cmd.Print(buf.String())
			return nil
		}

		// Default: show standard help
		return rootCmd.Help()
	},
}

func init() {
	// Add our extended help command (overrides Cobra's default help)
	helpCmd.Flags().Bool("all", false, "Show help for all commands")
	helpCmd.Flags().Bool("json", false, "Output full CLI schema as JSON")
	rootCmd.AddCommand(helpCmd)
}

// writeAllHelp writes usage for every command in the tree to buf.
func writeAllHelp(buf *bytes.Buffer, c *cobra.Command) {
	// Skip hidden commands
	if c.Hidden {
		return
	}

	// Header per command
	buf.WriteString("========================================\n")
	buf.WriteString(c.CommandPath())
	buf.WriteString("\n")
	buf.WriteString("----------------------------------------\n")
	buf.WriteString(c.UsageString())
	buf.WriteString("\n")

	// Recurse into subcommands (sorted by name for stable output)
	subs := c.Commands()
	sort.Slice(subs, func(i, j int) bool { return subs[i].Name() < subs[j].Name() })
	for _, sc := range subs {
		writeAllHelp(buf, sc)
	}
}

// CLISchema is a machine-readable description of the CLI for agents.
type CLISchema struct {
	Name     string       `json:"name"`
	Use      string       `json:"use"`
	Short    string       `json:"short"`
	Long     string       `json:"long,omitempty"`
	Path     string       `json:"path"`
	Aliases  []string     `json:"aliases,omitempty"`
	Flags    []FlagSchema `json:"flags,omitempty"`
	Children []CLISchema  `json:"children,omitempty"`
	// EnvVars lists well-known environment variables the toolchain respects.
	// Provided on the root command for agent discovery.
	EnvVars []EnvVarSchema `json:"env_vars,omitempty"`
}

// FlagSchema describes a flag on a command.
type FlagSchema struct {
	Name       string `json:"name"`
	Shorthand  string `json:"shorthand,omitempty"`
	Usage      string `json:"usage"`
	DefValue   string `json:"default,omitempty"`
	Persistent bool   `json:"persistent"`
}

// EnvVarSchema describes an environment variable consulted by the CLI or related services.
type EnvVarSchema struct {
	Name        string `json:"name"`
	Required    bool   `json:"required"`
	Default     string `json:"default,omitempty"`
	Description string `json:"description"`
}

func buildCLISchema(cmd *cobra.Command) CLISchema {
	schema := CLISchema{
		Name:    cmd.Name(),
		Use:     cmd.Use,
		Short:   cmd.Short,
		Long:    cmd.Long,
		Path:    cmd.CommandPath(),
		Aliases: cmd.Aliases,
		Flags:   collectFlags(cmd, false),
	}

	// Collect persistent flags from this command (if any)
	pflags := collectFlags(cmd, true)
	if len(pflags) > 0 {
		schema.Flags = append(schema.Flags, pflags...)
	}

	// Children
	subs := cmd.Commands()
	sort.Slice(subs, func(i, j int) bool { return subs[i].Name() < subs[j].Name() })
	for _, sc := range subs {
		if sc.Hidden {
			continue
		}
		child := buildCLISchema(sc)
		schema.Children = append(schema.Children, child)
	}
	// Only include env vars at the root level for brevity
	if cmd == rootCmd {
		schema.EnvVars = collectEnvVars()
	}
	return schema
}

func collectFlags(cmd *cobra.Command, persistent bool) []FlagSchema {
	var flags []FlagSchema
	var flg *pflag.FlagSet
	if persistent {
		flg = cmd.PersistentFlags()
	} else {
		flg = cmd.Flags()
	}
	flg.VisitAll(func(f *pflag.Flag) {
		flags = append(flags, FlagSchema{
			Name:       f.Name,
			Shorthand:  f.Shorthand,
			Usage:      f.Usage,
			DefValue:   f.DefValue,
			Persistent: persistent,
		})
	})
	// Stable order
	sort.Slice(flags, func(i, j int) bool { return flags[i].Name < flags[j].Name })
	return flags
}

// collectEnvVars returns a curated list of environment variables used across OpenExec.
// This list focuses on agent-discoverable config; all are optional unless noted.
func collectEnvVars() []EnvVarSchema {
	return []EnvVarSchema{
		// Interface Gateway (Telegram/Twilio)
		{Name: "TELEGRAM_BOT_TOKEN", Required: false, Description: "Telegram bot token; enables Telegram channel when set"},
		{Name: "TELEGRAM_WEBHOOK_SECRET", Required: false, Description: "Secret for Telegram webhook validation"},
		{Name: "TWILIO_ACCOUNT_SID", Required: false, Description: "Twilio Account SID; enables WhatsApp channel when set with TWILIO_AUTH_TOKEN"},
		{Name: "TWILIO_AUTH_TOKEN", Required: false, Description: "Twilio auth token"},
		{Name: "TWILIO_FROM_NUMBER", Required: false, Description: "Twilio sender (e.g., whatsapp:+123...)"},
		{Name: "TWILIO_EXTERNAL_URL", Required: false, Description: "Public URL used for Twilio signature validation behind proxies"},
		{Name: "EXECUTION_ENDPOINT", Required: false, Description: "HTTP base URL of the execution engine for interface status/commands"},

		// Execution Engine
		{Name: "OPENEXEC_AUDIT_DB_PATH", Required: false, Default: "audit.db", Description: "Path to the audit SQLite file"},
		{Name: "OPENEXEC_DATA_DIR", Required: false, Default: "/data", Description: "Data directory for audit DB and state"},

		// Common
		{Name: "DEBUG", Required: false, Description: "Enable verbose debug logging when 'true'"},
	}
}
