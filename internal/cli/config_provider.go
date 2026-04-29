package cli

import (
	"bufio"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/openexec/openexec/internal/project"
	"github.com/spf13/cobra"
)

var configProviderCmd = &cobra.Command{
	Use:   "provider",
	Short: "Manage named API provider endpoints",
	Long: `Manage named OpenAI-compatible API provider endpoints.

Each entry is stored in .openexec/config.json under execution.providers
and selected via execution.active_provider.

Examples:
  openexec config provider list
  openexec config provider add
  openexec config provider use kimi-prod
  openexec config provider remove vllm-local`,
}

var configProviderListCmd = &cobra.Command{
	Use:   "list",
	Short: "List configured API provider endpoints",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := loadExecutionProjectConfig()
		if err != nil {
			return err
		}

		// Show named entries.
		if len(cfg.Execution.Providers) == 0 {
			if cfg.Execution.APIProvider != "" {
				cmd.Printf("(legacy single-provider config)\n")
				cmd.Printf("  %s\n", cfg.Execution.APIProvider)
				cmd.Printf("    base_url: %s\n", cfg.Execution.APIBaseURL)
				cmd.Printf("    model:    %s\n", cfg.Execution.APIModel)
				cmd.Printf("\nRun 'openexec config provider add' to migrate to named providers.\n")
				return nil
			}
			cmd.Println("No API providers configured.")
			cmd.Println("Run 'openexec config provider add' to add one, or 'openexec init' to start over.")
			return nil
		}

		names := make([]string, 0, len(cfg.Execution.Providers))
		for n := range cfg.Execution.Providers {
			names = append(names, n)
		}
		sort.Strings(names)

		active := cfg.Execution.ActiveProvider
		if active == "" && len(names) == 1 {
			active = names[0]
		}

		for _, n := range names {
			p := cfg.Execution.Providers[n]
			marker := "  "
			if n == active {
				marker = "* "
			}
			cmd.Printf("%s%s\n", marker, n)
			cmd.Printf("    base_url: %s\n", p.BaseURL)
			cmd.Printf("    model:    %s\n", p.Model)
			cmd.Printf("    api_key:  %s\n", maskKey(p.APIKey))
		}
		cmd.Printf("\n* = active. Switch with 'openexec config provider use <name>'.\n")
		return nil
	},
}

var configProviderUseCmd = &cobra.Command{
	Use:   "use <name>",
	Short: "Switch the active API provider",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]
		cfg, err := loadExecutionProjectConfig()
		if err != nil {
			return err
		}
		if _, ok := cfg.Execution.Providers[name]; !ok {
			return fmt.Errorf("no provider named %q (run 'openexec config provider list')", name)
		}
		cfg.Execution.ActiveProvider = name
		if err := project.SaveProjectConfig(cfg); err != nil {
			return err
		}
		cmd.Printf("Active provider set to %q.\n", name)
		return nil
	},
}

var configProviderAddCmd = &cobra.Command{
	Use:   "add",
	Short: "Add a new API provider endpoint (interactive)",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := loadExecutionProjectConfig()
		if err != nil {
			return err
		}

		name, entry := promptAPIConfig(cmd, false)
		if name == "" || entry == nil {
			cmd.Println("Cancelled — no provider added.")
			return nil
		}

		if cfg.Execution.Providers == nil {
			cfg.Execution.Providers = map[string]project.ProviderConfig{}
		}
		if _, exists := cfg.Execution.Providers[name]; exists {
			fmt.Printf("Provider %q already exists. Overwrite? [y/N]: ", name)
			reader := bufio.NewReader(cmd.InOrStdin())
			ans, _ := reader.ReadString('\n')
			if strings.ToLower(strings.TrimSpace(ans)) != "y" {
				cmd.Println("Cancelled.")
				return nil
			}
		}
		cfg.Execution.Providers[name] = *entry

		// Offer to make it active. Default yes when there's no current active.
		makeActive := cfg.Execution.ActiveProvider == ""
		if !makeActive {
			fmt.Printf("Make %q the active provider? [y/N]: ", name)
			reader := bufio.NewReader(cmd.InOrStdin())
			ans, _ := reader.ReadString('\n')
			makeActive = strings.ToLower(strings.TrimSpace(ans)) == "y"
		}
		if makeActive {
			cfg.Execution.ActiveProvider = name
		}

		if err := project.SaveProjectConfig(cfg); err != nil {
			return err
		}
		cmd.Printf("\n✓ Added provider %q.\n", name)
		if makeActive {
			cmd.Printf("  (now active)\n")
		}
		return nil
	},
}

var configProviderRemoveCmd = &cobra.Command{
	Use:   "remove <name>",
	Short: "Remove a named API provider endpoint",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]
		cfg, err := loadExecutionProjectConfig()
		if err != nil {
			return err
		}
		if _, ok := cfg.Execution.Providers[name]; !ok {
			return fmt.Errorf("no provider named %q", name)
		}
		delete(cfg.Execution.Providers, name)
		if cfg.Execution.ActiveProvider == name {
			cfg.Execution.ActiveProvider = ""
		}
		if err := project.SaveProjectConfig(cfg); err != nil {
			return err
		}
		cmd.Printf("Removed provider %q.\n", name)
		if cfg.Execution.ActiveProvider == "" && len(cfg.Execution.Providers) > 0 {
			cmd.Println("No active provider — set one with 'openexec config provider use <name>'.")
		}
		return nil
	},
}

// loadExecutionProjectConfig loads the canonical project config from the cwd.
// Distinct from cli.loadProjectConfig (which uses a different, partial shape).
func loadExecutionProjectConfig() (*project.ProjectConfig, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	cfg, err := project.LoadProjectConfig(cwd)
	if err != nil {
		return nil, err
	}
	return cfg, nil
}

// maskKey redacts everything past the first 4 chars of an API key, but leaves
// "$ENV_VAR" references untouched so users can see their env-var indirection.
func maskKey(k string) string {
	if k == "" {
		return "(unset)"
	}
	if strings.HasPrefix(k, "$") {
		return k
	}
	if len(k) <= 8 {
		return "****"
	}
	return k[:4] + "…" + k[len(k)-2:]
}

func init() {
	configCmd.AddCommand(configProviderCmd)
	configProviderCmd.AddCommand(configProviderListCmd)
	configProviderCmd.AddCommand(configProviderUseCmd)
	configProviderCmd.AddCommand(configProviderAddCmd)
	configProviderCmd.AddCommand(configProviderRemoveCmd)
}
