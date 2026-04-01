package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/openexec/openexec/internal/skills"
	"github.com/spf13/cobra"
)

var skillsCmd = &cobra.Command{
	Use:   "skills",
	Short: "Manage reusable skills (knowledge modules)",
	Long:  `Skills are reusable instruction modules that can be injected into pipeline context. They are loaded from SKILL.md files with YAML frontmatter.`,
}

var skillsListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all available skills",
	RunE: func(cmd *cobra.Command, args []string) error {
		r := skills.NewRegistry()
		if err := r.LoadAll("."); err != nil {
			return err
		}

		category, _ := cmd.Flags().GetString("category")

		var list []*skills.Skill
		if category != "" {
			list = r.ListByCategory(category)
		} else {
			list = r.List()
		}

		if len(list) == 0 {
			cmd.Println("No skills found.")
			return nil
		}

		cmd.Printf("%-25s %-10s %-8s %s\n", "NAME", "SOURCE", "ENABLED", "DESCRIPTION")
		cmd.Printf("%-25s %-10s %-8s %s\n",
			strings.Repeat("-", 25),
			strings.Repeat("-", 10),
			strings.Repeat("-", 8),
			strings.Repeat("-", 30))

		for _, s := range list {
			enabled := "yes"
			if !s.Enabled {
				enabled = "no"
			}
			desc := s.Description
			if len(desc) > 50 {
				desc = desc[:47] + "..."
			}
			cmd.Printf("%-25s %-10s %-8s %s\n", s.Name, s.Source, enabled, desc)
		}

		cmd.Printf("\nTotal: %d skill(s)\n", len(list))
		return nil
	},
}

var skillsInfoCmd = &cobra.Command{
	Use:   "info <name>",
	Short: "Show detailed information about a skill",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		r := skills.NewRegistry()
		if err := r.LoadAll("."); err != nil {
			return err
		}

		s, ok := r.Get(args[0])
		if !ok {
			return fmt.Errorf("skill %q not found", args[0])
		}

		cmd.Printf("Name:        %s\n", s.Name)
		cmd.Printf("Description: %s\n", s.Description)
		cmd.Printf("Source:      %s\n", s.Source)
		cmd.Printf("Path:        %s\n", s.SourcePath)
		cmd.Printf("Enabled:     %v\n", s.Enabled)
		cmd.Printf("Priority:    %s\n", s.Priority)
		if len(s.Categories) > 0 {
			cmd.Printf("Categories:  %s\n", strings.Join(s.Categories, ", "))
		}
		if len(s.Tags) > 0 {
			cmd.Printf("Tags:        %s\n", strings.Join(s.Tags, ", "))
		}
		if s.WhenToUse != "" {
			cmd.Printf("When to use: %s\n", s.WhenToUse)
		}
		if s.Content != "" {
			cmd.Printf("\n--- Content ---\n%s\n", s.Content)
		}
		return nil
	},
}

var skillsSearchCmd = &cobra.Command{
	Use:   "search <query>",
	Short: "Search skills by keyword",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		r := skills.NewRegistry()
		if err := r.LoadAll("."); err != nil {
			return err
		}

		results := r.Search(args[0])
		if len(results) == 0 {
			cmd.Printf("No skills matching %q.\n", args[0])
			return nil
		}

		for _, s := range results {
			cmd.Printf("  %s (%s) - %s\n", s.Name, s.Source, s.Description)
		}
		cmd.Printf("\n%d result(s)\n", len(results))
		return nil
	},
}

var skillsImportCmd = &cobra.Command{
	Use:   "import",
	Short: "Import skills from external sources",
	RunE: func(cmd *cobra.Command, args []string) error {
		fromClaude, _ := cmd.Flags().GetBool("from-claude")
		importPath, _ := cmd.Flags().GetString("path")

		if !fromClaude && importPath == "" {
			return fmt.Errorf("specify --from-claude or --path <dir>")
		}

		home, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("resolve home directory: %w", err)
		}
		targetDir := filepath.Join(home, ".openexec", "skills", "imported")

		var imported []string
		if fromClaude {
			imported, err = skills.ImportFromClaude(targetDir)
		} else {
			imported, err = skills.ImportFromPath(importPath, targetDir)
		}

		if err != nil {
			return err
		}

		if len(imported) == 0 {
			cmd.Println("No skills found to import.")
			return nil
		}

		for _, name := range imported {
			cmd.Printf("  Imported: %s\n", name)
		}
		cmd.Printf("\n%d skill(s) imported to %s\n", len(imported), targetDir)
		return nil
	},
}

var skillsCreateCmd = &cobra.Command{
	Use:   "create <name>",
	Short: "Scaffold a new user skill",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]
		home, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("resolve home directory: %w", err)
		}

		skillDir := filepath.Join(home, ".openexec", "skills", "user", name)
		if err := os.MkdirAll(skillDir, 0o755); err != nil {
			return fmt.Errorf("create skill directory: %w", err)
		}

		skillFile := filepath.Join(skillDir, "SKILL.md")
		if _, err := os.Stat(skillFile); err == nil {
			return fmt.Errorf("skill %q already exists at %s", name, skillFile)
		}

		template := fmt.Sprintf(`---
name: %s
description: TODO - describe this skill
categories: []
tags: []
when_to_use: TODO - describe when this skill should be activated
priority: medium
---
# %s

TODO - Add skill instructions here.
`, name, name)

		if err := os.WriteFile(skillFile, []byte(template), 0o644); err != nil {
			return fmt.Errorf("write skill file: %w", err)
		}

		cmd.Printf("Created skill scaffold at %s\n", skillFile)
		return nil
	},
}

var skillsEnableCmd = &cobra.Command{
	Use:   "enable <name>",
	Short: "Enable a skill",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		r := skills.NewRegistry()
		if err := r.LoadAll("."); err != nil {
			return err
		}
		if err := r.Enable(args[0]); err != nil {
			return err
		}
		cmd.Printf("Enabled skill %q\n", args[0])
		return nil
	},
}

var skillsDisableCmd = &cobra.Command{
	Use:   "disable <name>",
	Short: "Disable a skill",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		r := skills.NewRegistry()
		if err := r.LoadAll("."); err != nil {
			return err
		}
		if err := r.Disable(args[0]); err != nil {
			return err
		}
		cmd.Printf("Disabled skill %q\n", args[0])
		return nil
	},
}

func init() {
	skillsListCmd.Flags().String("category", "", "Filter by category")
	skillsImportCmd.Flags().Bool("from-claude", false, "Import from ~/.claude/skills/")
	skillsImportCmd.Flags().String("path", "", "Import from a specific directory")

	skillsCmd.AddCommand(skillsListCmd)
	skillsCmd.AddCommand(skillsInfoCmd)
	skillsCmd.AddCommand(skillsSearchCmd)
	skillsCmd.AddCommand(skillsImportCmd)
	skillsCmd.AddCommand(skillsCreateCmd)
	skillsCmd.AddCommand(skillsEnableCmd)
	skillsCmd.AddCommand(skillsDisableCmd)
	rootCmd.AddCommand(skillsCmd)
}
