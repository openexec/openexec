package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"github.com/spf13/cobra"
)

// ProjectConfig holds project-level configuration.
type ProjectConfig struct {
	// Git integration
	GitEnabled  bool   `json:"git_enabled"`
	BaseBranch  string `json:"base_branch"`

	// Approval workflow
	ApprovalEnabled bool `json:"approval_enabled"`

	// Auto-merge settings
	AutoMergeStories bool `json:"auto_merge_stories"` // Auto-merge story when all tasks done
	AutoMergeToMain  bool `json:"auto_merge_to_main"` // Auto-merge release to main when complete
	AutoTagRelease   bool `json:"auto_tag_release"`   // Auto-create tag when release complete

	// Auto-link commits to tasks
	AutoLinkCommits bool `json:"auto_link_commits"`

	// Branch naming
	ReleaseBranchPrefix string `json:"release_branch_prefix"`
	FeatureBranchPrefix string `json:"feature_branch_prefix"`
}

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Manage project configuration",
	Long: `View and modify project configuration for git integration and approval workflows.

Configuration is stored in .openexec/config.json`,
}

var configShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Show current configuration",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := loadProjectConfig()
		if err != nil {
			if os.IsNotExist(err) {
				cfg = defaultProjectConfig()
				fmt.Println("No configuration file found. Using defaults:")
			} else {
				return err
			}
		}

		jsonOutput, _ := cmd.Flags().GetBool("json")
		if jsonOutput {
			data, err := json.MarshalIndent(cfg, "", "  ")
			if err != nil {
				return err
			}
			fmt.Println(string(data))
			return nil
		}

		fmt.Println("OpenExec Configuration")
		fmt.Println("======================")
		fmt.Println()
		fmt.Println("Git Integration:")
		fmt.Printf("  git_enabled:           %v\n", cfg.GitEnabled)
		fmt.Printf("  base_branch:           %s\n", cfg.BaseBranch)
		fmt.Printf("  auto_link_commits:     %v\n", cfg.AutoLinkCommits)
		fmt.Printf("  release_branch_prefix: %s\n", cfg.ReleaseBranchPrefix)
		fmt.Printf("  feature_branch_prefix: %s\n", cfg.FeatureBranchPrefix)
		fmt.Println()
		fmt.Println("Auto-Merge (when git_enabled=true):")
		fmt.Printf("  auto_merge_stories:    %v\n", cfg.AutoMergeStories)
		fmt.Printf("  auto_merge_to_main:    %v\n", cfg.AutoMergeToMain)
		fmt.Printf("  auto_tag_release:      %v\n", cfg.AutoTagRelease)
		fmt.Println()
		fmt.Println("Approval Workflow:")
		fmt.Printf("  approval_enabled:      %v\n", cfg.ApprovalEnabled)

		return nil
	},
}

var configSetCmd = &cobra.Command{
	Use:   "set <key> <value>",
	Short: "Set a configuration value",
	Long: `Set a configuration value.

Available keys:
  git_enabled           Enable git branch/commit tracking (true/false)
  base_branch           Default base branch for releases (e.g., main)
  approval_enabled      Require approval for merges/releases (true/false)
  auto_link_commits     Auto-link commits to tasks from messages (true/false)
  release_branch_prefix Prefix for release branches (e.g., release/)
  feature_branch_prefix Prefix for feature branches (e.g., feature/)

Examples:
  openexec config set git_enabled true
  openexec config set approval_enabled true
  openexec config set base_branch main`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		key := args[0]
		value := args[1]

		cfg, err := loadProjectConfig()
		if err != nil {
			if os.IsNotExist(err) {
				cfg = defaultProjectConfig()
			} else {
				return err
			}
		}

		switch key {
		case "git_enabled":
			b, err := strconv.ParseBool(value)
			if err != nil {
				return fmt.Errorf("invalid boolean value: %s", value)
			}
			cfg.GitEnabled = b
		case "base_branch":
			cfg.BaseBranch = value
		case "approval_enabled":
			b, err := strconv.ParseBool(value)
			if err != nil {
				return fmt.Errorf("invalid boolean value: %s", value)
			}
			cfg.ApprovalEnabled = b
		case "auto_link_commits":
			b, err := strconv.ParseBool(value)
			if err != nil {
				return fmt.Errorf("invalid boolean value: %s", value)
			}
			cfg.AutoLinkCommits = b
		case "auto_merge_stories":
			b, err := strconv.ParseBool(value)
			if err != nil {
				return fmt.Errorf("invalid boolean value: %s", value)
			}
			cfg.AutoMergeStories = b
		case "auto_merge_to_main":
			b, err := strconv.ParseBool(value)
			if err != nil {
				return fmt.Errorf("invalid boolean value: %s", value)
			}
			cfg.AutoMergeToMain = b
		case "auto_tag_release":
			b, err := strconv.ParseBool(value)
			if err != nil {
				return fmt.Errorf("invalid boolean value: %s", value)
			}
			cfg.AutoTagRelease = b
		case "release_branch_prefix":
			cfg.ReleaseBranchPrefix = value
		case "feature_branch_prefix":
			cfg.FeatureBranchPrefix = value
		default:
			return fmt.Errorf("unknown configuration key: %s", key)
		}

		if err := saveProjectConfig(cfg); err != nil {
			return err
		}

		fmt.Printf("Set %s = %s\n", key, value)
		return nil
	},
}

var configInitCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize configuration with defaults",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg := defaultProjectConfig()

		// Check flags for initial values
		if gitEnabled, _ := cmd.Flags().GetBool("git"); gitEnabled {
			cfg.GitEnabled = true
		}
		if approvalEnabled, _ := cmd.Flags().GetBool("approval"); approvalEnabled {
			cfg.ApprovalEnabled = true
		}

		if err := saveProjectConfig(cfg); err != nil {
			return err
		}

		fmt.Println("Configuration initialized.")
		fmt.Printf("  Git integration: %v\n", cfg.GitEnabled)
		fmt.Printf("  Approval workflow: %v\n", cfg.ApprovalEnabled)
		fmt.Println()
		fmt.Println("Modify with: openexec config set <key> <value>")
		return nil
	},
}

func init() {
	rootCmd.AddCommand(configCmd)

	configCmd.AddCommand(configShowCmd)
	configShowCmd.Flags().Bool("json", false, "Output as JSON")

	configCmd.AddCommand(configSetCmd)

	configCmd.AddCommand(configInitCmd)
	configInitCmd.Flags().Bool("git", false, "Enable git integration")
	configInitCmd.Flags().Bool("approval", false, "Enable approval workflow")
}

func defaultProjectConfig() *ProjectConfig {
	return &ProjectConfig{
		GitEnabled:          false,
		BaseBranch:          "main",
		ApprovalEnabled:     false,
		AutoLinkCommits:     true,
		ReleaseBranchPrefix: "release/",
		FeatureBranchPrefix: "feature/",
	}
}

func loadProjectConfig() (*ProjectConfig, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return nil, err
	}

	configPath := filepath.Join(cwd, ".openexec", "config.json")
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, err
	}

	cfg := defaultProjectConfig()
	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, err
	}

	return cfg, nil
}

func saveProjectConfig(cfg *ProjectConfig) error {
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}

	openexecDir := filepath.Join(cwd, ".openexec")
	if err := os.MkdirAll(openexecDir, 0o750); err != nil {
		return err
	}

	configPath := filepath.Join(openexecDir, "config.json")
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(configPath, data, 0o600)
}
