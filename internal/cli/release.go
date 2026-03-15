package cli

import (
	"encoding/json"
	"fmt"
	"github.com/fatih/color"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/openexec/openexec/internal/release"
	"github.com/spf13/cobra"
)

var releaseCmd = &cobra.Command{
	Use:   "release",
	Short: "Manage releases, stories, and generate changelogs",
	Long: `Release management commands for tracking releases, stories, tasks,
and generating ISO 27001 compliant documentation.

Git integration and approval workflows can be enabled via configuration.`,
}

// story export: write the current DB stories/tasks to .openexec/stories.json (export-only)
var storyExportCmd = &cobra.Command{
    Use:   "export",
    Short: "Export stories and tasks from DB to .openexec/stories.json (read-only export)",
    RunE: func(cmd *cobra.Command, args []string) error {
        mgr, err := getReleaseManager(cmd)
        if err != nil { return err }

        stories := mgr.GetStories()
        tasks := mgr.GetTasks()
        // build map from story ID to task summaries
        byStory := make(map[string][]map[string]interface{})
        for _, t := range tasks {
            byStory[t.StoryID] = append(byStory[t.StoryID], map[string]interface{}{
                "id":                 t.ID,
                "title":              t.Title,
                "description":        t.Description,
                "depends_on":         t.DependsOn,
                "verification_script": t.VerificationScript,
            })
        }

        export := map[string]interface{}{
            "schema_version": "1.1",
            "goals":          []interface{}{}, // goals not persisted here; export stories/tasks only
            "stories":        []interface{}{},
        }
        for _, s := range stories {
            export["stories"] = append(export["stories"].([]interface{}), map[string]interface{}{
                "id":                 s.ID,
                "goal_id":            s.GoalID,
                "title":              s.Title,
                "description":        s.Description,
                "acceptance_criteria": s.AcceptanceCriteria,
                "verification_script": s.VerificationScript,
                "depends_on":         s.DependsOn,
                "tasks":              byStory[s.ID],
                "story_type":         s.StoryType,
                "priority":           s.Priority,
                "status":             s.Status,
            })
        }

        outPath := filepath.Join(".openexec", "stories.json")
        _ = os.MkdirAll(filepath.Dir(outPath), 0750)
        data, _ := json.MarshalIndent(export, "", "  ")
        if err := os.WriteFile(outPath, data, 0644); err != nil {
            return fmt.Errorf("failed to write %s: %w", outPath, err)
        }
        cmd.Printf("✓ Exported stories to %s\n", outPath)
        return nil
    },
}

var releaseCreateCmd = &cobra.Command{
	Use:   "create <version>",
	Short: "Create a new release",
	Long: `Create a new release with optional git branch creation.

If git integration is enabled, this will create a release branch (e.g., release/1.0.0).`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		version := args[0]
		name, _ := cmd.Flags().GetString("name")
		description, _ := cmd.Flags().GetString("description")

		if name == "" {
			name = "Release " + version
		}

		mgr, err := getReleaseManager(cmd)
		if err != nil {
			return err
		}

		rel, err := mgr.CreateRelease(name, version, description)
		if err != nil {
			return err
		}

		cmd.Printf("Created release: %s (v%s)\n", rel.Name, rel.Version)
		if rel.Git != nil && rel.Git.Branch != "" {
			cmd.Printf("  Branch: %s\n", rel.Git.Branch)
		}
		if mgr.GetConfig().ApprovalEnabled {
			cmd.Printf("  Approval required: yes\n")
		}

		return nil
	},
}

var releaseShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Show current release information",
	RunE: func(cmd *cobra.Command, args []string) error {
		mgr, err := getReleaseManager(cmd)
		if err != nil {
			return err
		}

		rel := mgr.GetRelease()
		if rel == nil {
			cmd.Println("No release defined. Create one with: openexec release create <version>")
			return nil
		}

		jsonOutput, _ := cmd.Flags().GetBool("json")
		if jsonOutput {
			data, err := json.MarshalIndent(rel, "", "  ")
			if err != nil {
				return err
			}
			cmd.Println(string(data))
			return nil
		}

		cmd.Printf("Release: %s\n", rel.Name)
		cmd.Printf("  Version: %s\n", rel.Version)
		cmd.Printf("  Status: %s\n", rel.Status)
		if rel.Description != "" {
			cmd.Printf("  Description: %s\n", rel.Description)
		}
		cmd.Printf("  Stories: %d\n", len(rel.Stories))
		if rel.Git != nil {
			cmd.Printf("  Branch: %s\n", rel.Git.Branch)
			if rel.Git.Tag != "" {
				cmd.Printf("  Tag: %s\n", rel.Git.Tag)
			}
		}
		if rel.Approval != nil {
			cmd.Printf("  Approval: %s\n", rel.Approval.Status)
			if rel.Approval.ApprovedBy != "" {
				cmd.Printf("  Approved by: %s\n", rel.Approval.ApprovedBy)
			}
		}

		return nil
	},
}

var releaseChangelogCmd = &cobra.Command{
	Use:   "changelog",
	Short: "Generate changelog for the current release",
	Long: `Generate a changelog document from stories and tasks.

The changelog groups changes by type (features, bug fixes, etc.) and includes
git commit links and approval information when enabled.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		mgr, err := getReleaseManager(cmd)
		if err != nil {
			return err
		}

		gen := release.NewChangelogGenerator(mgr)

		jsonOutput, _ := cmd.Flags().GetBool("json")
		if jsonOutput {
			data, err := gen.GenerateJSON()
			if err != nil {
				return err
			}
			output, err := json.MarshalIndent(data, "", "  ")
			if err != nil {
				return err
			}
			cmd.Println(string(output))
			return nil
		}

		repoURL, _ := cmd.Flags().GetString("repo-url")
		includeApprovals, _ := cmd.Flags().GetBool("include-approvals")
		releaseNotes, _ := cmd.Flags().GetBool("release-notes")

		opts := release.DefaultChangelogOptions()
		opts.RepoURL = repoURL
		opts.IncludeApprovals = includeApprovals

		var changelog string
		if releaseNotes {
			changelog, err = gen.GenerateReleaseNotes(opts)
		} else {
			changelog, err = gen.Generate(opts)
		}
		if err != nil {
			return err
		}

		outputFile, _ := cmd.Flags().GetString("output")
		if outputFile != "" {
			if err := os.WriteFile(outputFile, []byte(changelog), 0o644); err != nil {
				return err
			}
			cmd.Printf("Changelog written to %s\n", outputFile)
		} else {
			cmd.Println(changelog)
		}

		return nil
	},
}

var releaseTagCmd = &cobra.Command{
	Use:   "tag",
	Short: "Create a git tag for the release",
	Long: `Create an annotated git tag for the release.

Examples:
  openexec release tag                      # Create local tag
  openexec release tag --push               # Create and push tag to origin
  openexec release tag --message "v1.0.0"   # Custom tag message`,
	RunE: func(cmd *cobra.Command, args []string) error {
		mgr, err := getReleaseManager(cmd)
		if err != nil {
			return err
		}

		message, _ := cmd.Flags().GetString("message")
		if message == "" {
			rel := mgr.GetRelease()
			if rel != nil {
				message = fmt.Sprintf("Release %s", rel.Version)
			}
		}

		push, _ := cmd.Flags().GetBool("push")

		if err := mgr.TagRelease(message); err != nil {
			return err
		}

		rel := mgr.GetRelease()
		if rel != nil && rel.Git != nil {
			cmd.Printf("Created tag: %s\n", rel.Git.Tag)

			if push {
				if err := mgr.PushTag(rel.Git.Tag); err != nil {
					return fmt.Errorf("tag created but push failed: %w", err)
				}
				cmd.Printf("Pushed tag to origin\n")
			}
		}

		return nil
	},
}

var releaseProcessCmd = &cobra.Command{
	Use:   "process",
	Short: "Process approved stories and complete release pipeline",
	Long: `Run the release processing pipeline to merge approved stories,
tag the release, and optionally merge to main.

This command is useful for CI/CD pipelines or when you want to explicitly
trigger the merge/tag/release pipeline after approvals.

Examples:
  openexec release process                  # Process all approved stories
  openexec release process --push           # Also push tags/branches to origin
  openexec release process --dry-run        # Preview actions without executing`,
	RunE: func(cmd *cobra.Command, args []string) error {
		mgr, err := getReleaseManager(cmd)
		if err != nil {
			return err
		}

		dryRun, _ := cmd.Flags().GetBool("dry-run")
		push, _ := cmd.Flags().GetBool("push")

		result, err := mgr.ProcessApprovedStoriesWithOptions(dryRun)
		if err != nil {
			return err
		}

		// Handle dry-run output
		if dryRun {
			if len(result.WouldMerge) == 0 {
				cmd.Println("[dry-run] No stories ready to merge.")
				cmd.Println("Stories must be complete (all tasks done) and approved (if approval_enabled).")
				return nil
			}

			cmd.Printf("[dry-run] Would process %d stories:\n", len(result.WouldMerge))
			for _, id := range result.WouldMerge {
				story := mgr.GetStory(id)
				branchInfo := ""
				if story != nil && story.Git != nil {
					branchInfo = fmt.Sprintf(" (%s)", story.Git.Branch)
				}
				cmd.Printf("  - Would merge %s%s to release\n", id, branchInfo)
			}

			if result.ReleaseComplete {
				cmd.Println("\n[dry-run] Release would be complete!")
				if result.WouldTag {
					rel := mgr.GetRelease()
					cmd.Printf("  Would create tag: v%s\n", rel.Version)
					if push {
						cmd.Printf("  Would push tag to origin\n")
					}
				}
				if result.WouldMergeToMain {
					cmd.Printf("  Would merge to: %s\n", mgr.GetConfig().BaseBranch)
					if push {
						cmd.Printf("  Would push %s to origin\n", mgr.GetConfig().BaseBranch)
					}
				}
				if !result.WouldTag && !result.WouldMergeToMain && mgr.GetConfig().ApprovalEnabled {
					rel := mgr.GetRelease()
					if rel.Approval == nil || rel.Approval.Status != release.ApprovalApproved {
						cmd.Println("  Would await release approval before tag/merge")
					}
				}
			}

			cmd.Println("\nRun without --dry-run to execute these actions.")
			return nil
		}

		// Handle actual execution
		if len(result.StoriesMerged) == 0 {
			cmd.Println("No stories ready to merge.")
			cmd.Println("Stories must be complete (all tasks done) and approved (if approval_enabled).")
			cmd.Println("\nTip: Use --dry-run to preview what would happen.")
			return nil
		}

		cmd.Printf("Processed %d stories:\n", len(result.StoriesMerged))
		for _, id := range result.StoriesMerged {
			cmd.Printf("  - Merged %s to release\n", id)
		}

		if len(result.Errors) > 0 {
			cmd.Println("\nErrors:")
			for _, e := range result.Errors {
				cmd.Printf("  - %s\n", e)
			}
		}

		if result.ReleaseComplete {
			cmd.Println("\nRelease complete!")

			rel := mgr.GetRelease()
			if result.ReleaseTagged {
				cmd.Printf("  Tag: %s\n", rel.Git.Tag)
				if push {
					if err := mgr.PushTag(rel.Git.Tag); err != nil {
						cmd.Printf("  Warning: failed to push tag: %v\n", err)
					} else {
						cmd.Printf("  Pushed tag to origin\n")
					}
				}
			}
			if result.ReleaseMergedToMain {
				cmd.Printf("  Merged to: %s\n", mgr.GetConfig().BaseBranch)
				if push {
					if err := mgr.PushBranch(mgr.GetConfig().BaseBranch); err != nil {
						cmd.Printf("  Warning: failed to push %s: %v\n", mgr.GetConfig().BaseBranch, err)
					} else {
						cmd.Printf("  Pushed %s to origin\n", mgr.GetConfig().BaseBranch)
					}
				}
			}

			if !result.ReleaseTagged && !result.ReleaseMergedToMain {
				if mgr.GetConfig().ApprovalEnabled {
					if rel.Approval == nil || rel.Approval.Status != release.ApprovalApproved {
						cmd.Println("  Awaiting release approval before tag/merge")
						cmd.Println("  Run: openexec release approve --approver <name>")
					}
				}
			}
		}

		return nil
	},
}

var releasePushBranchCmd = &cobra.Command{
	Use:   "push-branch",
	Short: "Push the release branch to origin",
	Long: `Push the release branch to the remote repository for remote evidence.

Examples:
  openexec release push-branch              # Push current release branch`,
	RunE: func(cmd *cobra.Command, args []string) error {
		mgr, err := getReleaseManager(cmd)
		if err != nil {
			return err
		}

		rel := mgr.GetRelease()
		if rel == nil {
			return fmt.Errorf("no release defined")
		}

		if rel.Git == nil || rel.Git.Branch == "" {
			return fmt.Errorf("release has no git branch (git_enabled may be false)")
		}

		branch := rel.Git.Branch
		branchOverride, _ := cmd.Flags().GetString("branch")
		if branchOverride != "" {
			branch = branchOverride
		}

		if err := mgr.PushBranch(branch); err != nil {
			return fmt.Errorf("failed to push branch: %w", err)
		}

		cmd.Printf("Pushed branch %s to origin\n", branch)
		return nil
	},
}

var releaseApproveCmd = &cobra.Command{
	Use:   "approve",
	Short: "Approve the current release",
	RunE: func(cmd *cobra.Command, args []string) error {
		mgr, err := getReleaseManager(cmd)
		if err != nil {
			return err
		}

		if !mgr.GetConfig().ApprovalEnabled {
			cmd.Println("Approval workflow is not enabled.")
			cmd.Println("Enable it with: openexec config set approval_enabled true")
			return nil
		}

		approverID, err := resolveApproverIdentity(cmd, mgr)
		if err != nil {
			return err
		}
		comments, _ := cmd.Flags().GetString("comments")

		if err := mgr.ApproveRelease(approverID, comments); err != nil {
			return err
		}

		cmd.Printf("Release approved by %s\n", approverID)
		return nil
	},
}

var releaseApprovalsCmd = &cobra.Command{
	Use:   "approvals",
	Short: "Show pending approvals",
	Long: `List all tasks, stories, and the release that are awaiting approval.

This is useful for CI/CD pipelines to check if there are items blocking the release.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		mgr, err := getReleaseManager(cmd)
		if err != nil {
			return err
		}

		if !mgr.GetConfig().ApprovalEnabled {
			cmd.Println("Approval workflow is not enabled.")
			cmd.Println("Enable it with: openexec config set approval_enabled true")
			return nil
		}

		pending := mgr.GetPendingApprovals()

		jsonOutput, _ := cmd.Flags().GetBool("json")
		if jsonOutput {
			data, err := json.MarshalIndent(pending, "", "  ")
			if err != nil {
				return err
			}
			cmd.Println(string(data))
			return nil
		}

		hasItems := false

		if len(pending.Tasks) > 0 {
			hasItems = true
			cmd.Printf("Tasks awaiting approval (%d):\n", len(pending.Tasks))
			for _, t := range pending.Tasks {
				cmd.Printf("  - %s: %s [%s]\n", t.ID, t.Title, t.Status)
			}
			cmd.Println()
		}

		if len(pending.Stories) > 0 {
			hasItems = true
			cmd.Printf("Stories awaiting approval (%d):\n", len(pending.Stories))
			for _, s := range pending.Stories {
				cmd.Printf("  - %s: %s\n", s.ID, s.Title)
			}
			cmd.Println()
		}

		if pending.Release != nil {
			hasItems = true
			cmd.Printf("Release awaiting approval:\n")
			cmd.Printf("  - %s (v%s)\n", pending.Release.Name, pending.Release.Version)
			cmd.Println()
		}

		if !hasItems {
			cmd.Println("No items awaiting approval.")
		}

		return nil
	},
}

var storyCmd = &cobra.Command{
	Use:   "story",
	Short: "Manage stories",
}

var storyCreateCmd = &cobra.Command{
	Use:   "create <story-id> <title>",
	Short: "Create a new story",
	Long: `Create a new story and optionally link it to the current release.

If git integration is enabled, this will create a feature branch for the story.

Examples:
  openexec story create US-001 "User authentication"
  openexec story create US-002 "Password reset" --type bugfix --priority 1`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		storyID := args[0]
		title := args[1]

		mgr, err := getReleaseManager(cmd)
		if err != nil {
			return err
		}

		description, _ := cmd.Flags().GetString("description")
		storyType, _ := cmd.Flags().GetString("type")
		priority, _ := cmd.Flags().GetInt("priority")

		if storyType == "" {
			storyType = "feature"
		}

		story := &release.Story{
			ID:          storyID,
			Title:       title,
			Description: description,
			StoryType:   storyType,
			Priority:    priority,
			Tasks:       []string{},
		}

		if err := mgr.CreateStory(story); err != nil {
			return err
		}

		cmd.Printf("Created story: %s\n", storyID)
		cmd.Printf("  Title: %s\n", title)
		cmd.Printf("  Type: %s\n", storyType)
		if story.Git != nil && story.Git.Branch != "" {
			cmd.Printf("  Branch: %s\n", story.Git.Branch)
		}

		return nil
	},
}

var storyListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all stories",
	RunE: func(cmd *cobra.Command, args []string) error {
		mgr, err := getReleaseManager(cmd)
		if err != nil {
			return err
		}

		stories := mgr.GetStories()

		jsonOutput, _ := cmd.Flags().GetBool("json")
		if jsonOutput {
			data, err := json.MarshalIndent(stories, "", "  ")
			if err != nil {
				return err
			}
			cmd.Println(string(data))
			return nil
		}

		if len(stories) == 0 {
			cmd.Println("No stories found.")
			return nil
		}

		cmd.Printf("Stories (%d):\n\n", len(stories))
		for _, story := range stories {
			status := statusIcon(story.Status)
			cmd.Printf("%s %s: %s [%s]\n", status, story.ID, story.Title, story.StoryType)
			if story.Git != nil && story.Git.Branch != "" {
				cmd.Printf("     Branch: %s\n", story.Git.Branch)
			}
			tasks := mgr.GetTasksForStory(story.ID)
			if len(tasks) > 0 {
				completed := 0
				for _, t := range tasks {
					if t.Status == release.TaskStatusDone {
						completed++
					}
				}
				cmd.Printf("     Tasks: %d/%d completed\n", completed, len(tasks))
			}
		}

		return nil
	},
}

var storyShowCmd = &cobra.Command{
	Use:   "show <story-id>",
	Short: "Show story details",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		storyID := args[0]

		mgr, err := getReleaseManager(cmd)
		if err != nil {
			return err
		}

		story := mgr.GetStory(storyID)
		if story == nil {
			return fmt.Errorf("story %s not found", storyID)
		}

		jsonOutput, _ := cmd.Flags().GetBool("json")
		if jsonOutput {
			data, err := json.MarshalIndent(story, "", "  ")
			if err != nil {
				return err
			}
			cmd.Println(string(data))
			return nil
		}

		cmd.Printf("Story: %s\n", story.ID)
		cmd.Printf("  Title: %s\n", story.Title)
		cmd.Printf("  Type: %s\n", story.StoryType)
		cmd.Printf("  Status: %s\n", story.Status)
		cmd.Printf("  Priority: %d\n", story.Priority)

		if story.Git != nil {
			cmd.Printf("\nGit:\n")
			cmd.Printf("  Branch: %s\n", story.Git.Branch)
			if story.Git.MergedTo != "" {
				cmd.Printf("  Merged to: %s\n", story.Git.MergedTo)
				cmd.Printf("  Merge commit: %s\n", story.Git.MergeCommit)
			}
		}

		if story.Approval != nil {
			cmd.Printf("\nApproval:\n")
			cmd.Printf("  Status: %s\n", story.Approval.Status)
			if story.Approval.ApprovedBy != "" {
				cmd.Printf("  Approved by: %s\n", story.Approval.ApprovedBy)
			}
		}

		tasks := mgr.GetTasksForStory(storyID)
		if len(tasks) > 0 {
			cmd.Printf("\nTasks (%d):\n", len(tasks))
			for _, task := range tasks {
				status := statusIcon(task.Status)
				cmd.Printf("  %s %s: %s\n", status, task.ID, task.Title)
			}
		}

		return nil
	},
}

var storyMergeCmd = &cobra.Command{
	Use:   "merge <story-id>",
	Short: "Merge a story branch to the release branch",
	Long: `Merge the story's feature branch into the release branch.

If approval workflow is enabled, the story must be approved first.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		storyID := args[0]

		mgr, err := getReleaseManager(cmd)
		if err != nil {
			return err
		}

		if err := mgr.MergeStoryToRelease(storyID); err != nil {
			return err
		}

		cmd.Printf("Story %s merged to release\n", storyID)
		return nil
	},
}

var storyApproveCmd = &cobra.Command{
	Use:   "approve <story-id>",
	Short: "Approve a story",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		storyID := args[0]

		mgr, err := getReleaseManager(cmd)
		if err != nil {
			return err
		}

		if !mgr.GetConfig().ApprovalEnabled {
			cmd.Println("Approval workflow is not enabled.")
			return nil
		}

		approverID, err := resolveApproverIdentity(cmd, mgr)
		if err != nil {
			return err
		}
		comments, _ := cmd.Flags().GetString("comments")

		if err := mgr.ApproveStory(storyID, approverID, comments); err != nil {
			return err
		}

		cmd.Printf("Story %s approved by %s\n", storyID, approverID)

		// Process approved stories to trigger auto-merge if configured
		if mgr.GetConfig().AutoMergeStories {
			result, err := mgr.ProcessApprovedStories()
			if err != nil {
				cmd.Printf("Warning: failed to process approved stories: %v\n", err)
			} else if len(result.StoriesMerged) > 0 {
				for _, id := range result.StoriesMerged {
					cmd.Printf("  Auto-merged story %s to release\n", id)
				}
				if result.ReleaseComplete {
					if mgr.GetConfig().ApprovalEnabled {
						rel := mgr.GetRelease()
						if rel != nil && (rel.Approval == nil || rel.Approval.Status != release.ApprovalApproved) {
							cmd.Printf("  Release complete but awaiting approval\n")
							cmd.Printf("  Run: openexec release approve --approver <name>\n")
						} else {
							cmd.Printf("  Release complete and approved!\n")
						}
					} else {
						cmd.Printf("  Release complete!\n")
					}
					if result.ReleaseTagged {
						cmd.Printf("  Created release tag\n")
					}
					if result.ReleaseMergedToMain {
						cmd.Printf("  Merged release to main\n")
					}
				}
			}
		}

		return nil
	},
}

// GeneratedTask represents a task from stories.json
type GeneratedTask struct {
	ID                 string   `json:"id"`
	Title              string   `json:"title"`
	Description        string   `json:"description"`
	DependsOn          []string `json:"depends_on,omitempty"`
	VerificationScript string   `json:"verification_script,omitempty"`
}

// GeneratedStory represents a story from stories.json generated by orchestration
type GeneratedStory struct {
	ID                 string   `json:"id"`
	Title              string   `json:"title"`
	Description        string   `json:"description"`
	AcceptanceCriteria []string `json:"acceptance_criteria"`
	GoalID             string   `json:"goal_id,omitempty"`
	DependsOn          []string `json:"depends_on,omitempty"`
	VerificationScript string   `json:"verification_script,omitempty"`
	Contract           string   `json:"contract,omitempty"`
	Tasks              []any    `json:"tasks"`
}

var storyImportCmd = &cobra.Command{
	Use:   "import [file]",
	Short: "Advanced: Import stories manually (Deprecated: use 'openexec run' instead)",
	Long: `Import stories and tasks from a generated stories.json file.
Note: 'openexec run' now performs this step automatically via deep-healing.`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		cmd.Println(color.New(color.FgYellow).Sprint("💡 Note: 'openexec story import' is now an advanced command. 'openexec run' handles auto-import by default."))
		// Determine input file
		inputFile := ".openexec/stories.json"
		if len(args) > 0 {
			inputFile = args[0]
		}

		dryRun, _ := cmd.Flags().GetBool("dry-run")
		reassign, _ := cmd.Flags().GetBool("reassign")
		pruneOrphans, _ := cmd.Flags().GetBool("prune-orphans")

		// Read stories file
		data, err := os.ReadFile(inputFile)
		if err != nil {
			return fmt.Errorf("failed to read %s: %w", inputFile, err)
		}

		var sf struct {
			SchemaVersion string           `json:"schema_version"`
			Goals         []release.Goal   `json:"goals"`
			Stories       []GeneratedStory `json:"stories"`
		}

		if err := json.Unmarshal(data, &sf); err != nil {
			// Try old format (bare array) for one last time
			var bareStories []GeneratedStory
			if errArray := json.Unmarshal(data, &bareStories); errArray == nil {
				sf.Stories = bareStories
				sf.SchemaVersion = "legacy"
			} else {
				return fmt.Errorf("failed to parse stories (invalid schema): %w", err)
			}
		}

		// Version validation
		if sf.SchemaVersion == "" {
			cmd.Println("Warning: missing schema_version in stories file")
		} else if sf.SchemaVersion != "1.0" && sf.SchemaVersion != "1.1" && sf.SchemaVersion != "legacy" {
			return fmt.Errorf("unsupported stories schema version: %s (expected 1.0 or 1.1)", sf.SchemaVersion)
		}

		stories := sf.Stories

		if len(stories) == 0 {
			cmd.Println("No stories found in file.")
			return nil
		}

		// PLANNING GATE: Enforce goal coverage and verifiability
		if len(sf.Goals) > 0 {
			cmd.Println("Running Planning Gate checks...")
			goalStoryCount := make(map[string]int)
			goalVerifyCount := make(map[string]int)

			for _, s := range stories {
				if s.GoalID != "" {
					goalStoryCount[s.GoalID]++
					if s.VerificationScript != "" {
						goalVerifyCount[s.GoalID]++
					}
				}
			}

			for _, g := range sf.Goals {
				if goalStoryCount[g.ID] == 0 {
					return fmt.Errorf("PLANNING GATE FAILED: Primary goal %s (%s) has no supporting stories", g.ID, g.Title)
				}
				if goalVerifyCount[g.ID] == 0 {
					return fmt.Errorf("PLANNING GATE FAILED: Primary goal %s (%s) has no stories with a verification_script", g.ID, g.Title)
				}
			}
			cmd.Println("✓ Planning Gate passed.")
		}

		if dryRun {
			cmd.Printf("Would import %d goals and %d stories:\n\n", len(sf.Goals), len(stories))
			for _, g := range sf.Goals {
				cmd.Printf("  Goal %s: %s\n", g.ID, g.Description)
			}
			for _, s := range stories {
				cmd.Printf("  %s [%s]: %s\n", s.ID, s.GoalID, s.Title)
				for _, tRaw := range s.Tasks {
					id := ""
					title := ""

					switch v := tRaw.(type) {
					case string:
						id = v
					case map[string]any:
						id, _ = v["id"].(string)
						title, _ = v["title"].(string)
					}

					if id == "" {
						id = fmt.Sprintf("T-%s-%03d", s.ID, 0)
					}
					if title != "" {
						cmd.Printf("    %s: %s\n", id, title)
					} else {
						cmd.Printf("    %s\n", id)
					}
				}
			}
			return nil
		}

		mgr, err := getReleaseManager(cmd)
		if err != nil {
			return err
		}

		// Import goals
		goalsCreated := 0
		for _, g := range sf.Goals {
			existing := mgr.GetGoal(g.ID)
			if existing == nil {
				// We need a copy to pass by pointer
				newGoal := g
				if err := mgr.CreateGoal(&newGoal); err != nil {
					cmd.Printf("  [error] %s: %v\n", g.ID, err)
					continue
				}
				goalsCreated++
			}
		}

		// Track counts
		storiesCreated := 0
		tasksCreated := 0
		skipped := 0

		cmd.Printf("Importing %d stories from %s...\n\n", len(stories), inputFile)

		storyIDPattern := regexp.MustCompile(`^(US|REQ)-\d{3}$`)

		// Map to track which tasks are currently in the incoming stories.json
		incomingTaskIDs := make(map[string]bool)
		for _, s := range stories {
			for _, tRaw := range s.Tasks {
				id := ""
				switch v := tRaw.(type) {
				case string:
					id = v
				case map[string]any:
					id, _ = v["id"].(string)
				}
				if id != "" {
					incomingTaskIDs[id] = true
				}
			}
		}

		for _, genStory := range stories {
			// Validate ID format
			if !storyIDPattern.MatchString(genStory.ID) {
				cmd.Printf("  [error] %s: invalid ID format (expected US-### or REQ-###)\n", genStory.ID)
				continue
			}

			// Check if story already exists
			story := mgr.GetStory(genStory.ID)
			if story == nil {
				// Create story
				story = &release.Story{
					ID:                 genStory.ID,
					GoalID:             genStory.GoalID,
					Title:              genStory.Title,
					Description:        genStory.Description,
					AcceptanceCriteria: genStory.AcceptanceCriteria,
					VerificationScript: genStory.VerificationScript,
					Contract:           genStory.Contract,
					StoryType:          "feature",
					DependsOn:          genStory.DependsOn,
					Tasks:              []string{},
					Status:             release.StoryStatusPending,
				}

				if err := mgr.CreateStory(story); err != nil {
					cmd.Printf("  [error] %s: %v\n", genStory.ID, err)
					continue
				}

				// Materialize story markdown file for engine visibility
				storyDir := filepath.Join(mgr.BaseDir(), ".openexec", "stories")
				_ = os.MkdirAll(storyDir, 0o750)
				storyPath := filepath.Join(storyDir, genStory.ID+".md")
				if _, err := os.Stat(storyPath); os.IsNotExist(err) {
					content := fmt.Sprintf("# Story %s: %s\n\n%s\n\n## Acceptance Criteria\n", genStory.ID, genStory.Title, genStory.Description)
					for _, ac := range genStory.AcceptanceCriteria {
						content += fmt.Sprintf("- %s\n", ac)
					}
					_ = os.WriteFile(storyPath, []byte(content), 0o644)
				}

				storiesCreated++
				cmd.Printf("  [created] %s: %s\n", genStory.ID, genStory.Title)
			} else {
				cmd.Printf("  [exists] %s: %s (status: %s)\n", genStory.ID, genStory.Title, story.Status)
				skipped++
			}

			// ALWAYS ensure story markdown file exists for engine visibility (Self-Healing)
			storyDir := filepath.Join(mgr.BaseDir(), ".openexec", "stories")
			_ = os.MkdirAll(storyDir, 0o750)
			storyPath := filepath.Join(storyDir, genStory.ID+".md")

			fileExisted := false
			if _, err := os.Stat(storyPath); err == nil {
				fileExisted = true
				// Check if file content says it's done
				if data, err := os.ReadFile(storyPath); err == nil {
					content := string(data)
					if strings.Contains(strings.ToLower(content), "status: completed") ||
						strings.Contains(strings.ToLower(content), "status: done") {
						// Sync back to database if it was pending
						if story != nil && story.Status == release.StoryStatusPending {
							story.Status = release.StoryStatusDone
							_ = mgr.CreateStory(story)
							cmd.Printf("  [sync] %s: marked as done based on filesystem\n", genStory.ID)
						}
					}
				}
			}

			if !fileExisted {
				content := fmt.Sprintf("# Story %s: %s\n\n%s\n\n## Acceptance Criteria\n", genStory.ID, genStory.Title, genStory.Description)
				for _, ac := range genStory.AcceptanceCriteria {
					content += fmt.Sprintf("- %s\n", ac)
				}
				_ = os.WriteFile(storyPath, []byte(content), 0o644)
			}

			// Create tasks for this story
			for i, tRaw := range genStory.Tasks {
				var genTask GeneratedTask

				// Handle both formats: string ID or full object
				switch v := tRaw.(type) {
				case string:
					genTask = GeneratedTask{ID: v}
				case map[string]any:
					// Marshal back to JSON and unmarshal into struct to handle nesting
					data, _ := json.Marshal(v)
					_ = json.Unmarshal(data, &genTask)
				default:
					continue
				}

				taskID := genTask.ID
				if taskID == "" {
					taskID = fmt.Sprintf("T-%s-%03d", genStory.ID, i+1)
				}

				// Check if task already exists
				existingTask := mgr.GetTask(taskID)
				if existingTask != nil {
					// Optionally reassign existing tasks to this story and sync status
					if reassign {
						if existingTask.StoryID == "" || existingTask.StoryID != genStory.ID {
							if err := mgr.ReassignTask(existingTask.ID, genStory.ID); err == nil {
								cmd.Printf("    [reassign] %s: linked to story %s\n", taskID, genStory.ID)
							} else {
								cmd.Printf("    [error] %s: failed to reassign: %v\n", taskID, err)
							}
						}
						// If story is done, mark task done too
						if story != nil && story.Status == release.StoryStatusDone && existingTask.Status != release.TaskStatusDone {
							if err := mgr.SetTaskStatus(existingTask.ID, release.TaskStatusDone); err == nil {
								cmd.Printf("    [sync] %s: marked done to match story status\n", taskID)
							}
						}
					} else {
						// Without reassign, just warn about mismatched linkage
						if existingTask.StoryID != genStory.ID {
							cmd.Printf("    [warning] %s: exists but belongs to story %s\n", taskID, existingTask.StoryID)
						}
					}
					continue
				}

				task := &release.Task{
					ID:                 taskID,
					Title:              genTask.Title,
					Description:        genTask.Description,
					StoryID:            genStory.ID,
					DependsOn:          genTask.DependsOn,
					VerificationScript: genTask.VerificationScript,
					Status:             release.TaskStatusPending,
				}

				if task.Title == "" {
					task.Title = "Imported Task " + taskID
				}

				// If the parent story is already done, sync task status to done
				if story != nil && story.Status == release.StoryStatusDone {
					task.Status = release.TaskStatusDone
				}

				if err := mgr.CreateTask(task); err != nil {
					cmd.Printf("    [error] %s: %v\n", taskID, err)
					continue
				}

				tasksCreated++
				cmd.Printf("    [created] %s: %s\n", taskID, task.Title)
			}
		}

		// Prune legacy tasks not in incoming plan (Only if --prune-orphans is set)
		prunedCount := 0
		if pruneOrphans {
			allTasks := mgr.GetTasks()
			for _, t := range allTasks {
				if !incomingTaskIDs[t.ID] {
					if err := mgr.DeleteTask(t.ID); err == nil {
						prunedCount++
					}
				}
			}
		}

		// Integrity Audit: Verify the final state matches the incoming plan
		allFinalTasks := mgr.GetTasks()
		taskMap := make(map[string]*release.Task)
		for _, t := range allFinalTasks {
			taskMap[t.ID] = t
		}

		missingTasks := 0
		mislinkedTasks := 0
		for _, s := range stories {
			for _, tRaw := range s.Tasks {
				id := ""
				switch v := tRaw.(type) {
				case string:
					id = v
				case map[string]any:
					id, _ = v["id"].(string)
				}

				if id == "" {
					continue
				}

				t, exists := taskMap[id]
				if !exists {
					cmd.Printf("  [audit-error] %s: task missing from database after import\n", id)
					missingTasks++
				} else if t.StoryID != s.ID {
					if reassign {
						cmd.Printf("  [audit-fix] %s: task was mislinked but reassigned to story %s\n", id, s.ID)
					} else {
						cmd.Printf("  [audit-error] %s: task is linked to story %s, expected %s (run with --reassign to fix)\n", id, t.StoryID, s.ID)
						mislinkedTasks++
					}
				}
			}
		}

		cmd.Printf("\nImport complete:\n")
		cmd.Printf("  Stories created: %d\n", storiesCreated)
		cmd.Printf("  Tasks created:   %d\n", tasksCreated)
		cmd.Printf("  Skipped (existing): %d\n", skipped)
		if prunedCount > 0 {
			cmd.Printf("  Legacy tasks pruned: %d\n", prunedCount)
		}

		if missingTasks > 0 || mislinkedTasks > 0 {
			cmd.Printf("\n⚠️  INTEGRITY WARNING: %d task(s) missing, %d task(s) mislinked.\n", missingTasks, mislinkedTasks)
			return fmt.Errorf("integrity check failed")
		}

		cmd.Printf("✓ Integrity Audit passed: all tasks present and correctly linked.\n")
		return nil
	},
}

var taskCmd = &cobra.Command{
	Use:   "task",
	Short: "Manage tasks",
}

var taskCreateCmd = &cobra.Command{
	Use:   "create <task-id> <title>",
	Short: "Create a new task",
	Long: `Create a new task and link it to a story.

Examples:
  openexec task create T-001 "Implement login API" --story US-001
  openexec task create T-002 "Add unit tests" --story US-001 --needs-review`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		taskID := args[0]
		title := args[1]

		mgr, err := getReleaseManager(cmd)
		if err != nil {
			return err
		}

		storyID, _ := cmd.Flags().GetString("story")
		description, _ := cmd.Flags().GetString("description")
		needsReview, _ := cmd.Flags().GetBool("needs-review")

		if storyID == "" {
			return fmt.Errorf("--story is required")
		}

		// Verify story exists
		story := mgr.GetStory(storyID)
		if story == nil {
			return fmt.Errorf("story %s not found", storyID)
		}

		task := &release.Task{
			ID:          taskID,
			Title:       title,
			Description: description,
			StoryID:     storyID,
			NeedsReview: needsReview,
		}

		if err := mgr.CreateTask(task); err != nil {
			return err
		}

		cmd.Printf("Created task: %s\n", taskID)
		cmd.Printf("  Title: %s\n", title)
		cmd.Printf("  Story: %s\n", storyID)
		if task.Git != nil && task.Git.Branch != "" {
			cmd.Printf("  Branch: %s\n", task.Git.Branch)
		}
		if needsReview {
			cmd.Printf("  Needs review: yes\n")
		}

		return nil
	},
}

var taskCompleteCmd = &cobra.Command{
	Use:   "complete <task-id>",
	Short: "Mark a task as done",
	Long: `Mark a task as complete and trigger auto-merge if configured.

When auto_merge_stories is enabled, completing the last task of a story
will automatically merge that story into the release branch.

Examples:
  openexec task complete T-001`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		taskID := args[0]

		mgr, err := getReleaseManager(cmd)
		if err != nil {
			return err
		}

		result, err := mgr.CompleteTask(taskID)
		if err != nil {
			return err
		}

		cmd.Printf("Task %s marked as done\n", taskID)

		if result.StoryComplete {
			if result.AwaitingApproval {
				cmd.Printf("  Story %s is complete but awaiting approval\n", result.StoryID)
				cmd.Printf("  Run: openexec story approve %s\n", result.StoryID)
			} else if result.StoryMerged {
				cmd.Printf("  Story %s auto-merged to release\n", result.StoryID)
			}
		}

		if result.ReleaseComplete {
			cmd.Println("  Release is complete!")
			if result.ReleaseTagged {
				cmd.Println("  Release tagged")
			}
			if result.ReleaseMergedToMain {
				cmd.Println("  Release merged to main")
			}
			if !result.ReleaseTagged && !result.ReleaseMergedToMain {
				rel := mgr.GetRelease()
				if rel != nil && mgr.GetConfig().ApprovalEnabled {
					if rel.Approval == nil || rel.Approval.Status != release.ApprovalApproved {
						cmd.Println("  Awaiting release approval")
						cmd.Println("  Run: openexec release approve")
					}
				}
			}
		}

		if result.Error != "" {
			cmd.Printf("  Warning: %s\n", result.Error)
		}

		return nil
	},
}

var taskListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all tasks",
	RunE: func(cmd *cobra.Command, args []string) error {
		mgr, err := getReleaseManager(cmd)
		if err != nil {
			return err
		}

		storyID, _ := cmd.Flags().GetString("story")

		var tasks []*release.Task
		if storyID != "" {
			tasks = mgr.GetTasksForStory(storyID)
		} else {
			tasks = mgr.GetTasks()
		}

		jsonOutput, _ := cmd.Flags().GetBool("json")
		if jsonOutput {
			data, err := json.MarshalIndent(tasks, "", "  ")
			if err != nil {
				return err
			}
			cmd.Println(string(data))
			return nil
		}

		if len(tasks) == 0 {
			cmd.Println("No tasks found.")
			return nil
		}

		cmd.Printf("Tasks (%d):\n\n", len(tasks))
		for _, task := range tasks {
			status := statusIcon(task.Status)
			cmd.Printf("%s %s: %s [%s]\n", status, task.ID, task.Title, task.StoryID)
			if task.Git != nil && len(task.Git.Commits) > 0 {
				cmd.Printf("     Commits: %d\n", len(task.Git.Commits))
			}
		}

		return nil
	},
}

var taskLinkCmd = &cobra.Command{
	Use:   "link <task-id> <commit-hash>",
	Short: "Link a commit to a task",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		taskID := args[0]
		commitHash := args[1]

		mgr, err := getReleaseManager(cmd)
		if err != nil {
			return err
		}

		if err := mgr.LinkCommitToTask(taskID, commitHash); err != nil {
			return err
		}

		shortHash := commitHash
		if len(commitHash) > 7 {
			shortHash = commitHash[:7]
		}
		cmd.Printf("Linked commit %s to task %s\n", shortHash, taskID)
		return nil
	},
}

var taskAutolinkCmd = &cobra.Command{
	Use:   "autolink",
	Short: "Auto-link commits to tasks based on commit messages",
	Long: `Scan commit messages for task ID patterns (e.g., T-001, T-123) and
automatically link matching commits to their tasks.

Examples:
  openexec task autolink                    # Link commits since last run
  openexec task autolink --since abc123     # Link commits since specific hash
  openexec task autolink --all              # Link all commits in history`,
	RunE: func(cmd *cobra.Command, args []string) error {
		mgr, err := getReleaseManager(cmd)
		if err != nil {
			return err
		}

		if !mgr.GetConfig().GitEnabled {
			return fmt.Errorf("git integration is not enabled; run: openexec config set git_enabled true")
		}

		since, _ := cmd.Flags().GetString("since")
		all, _ := cmd.Flags().GetBool("all")

		if all {
			since = ""
		}

		linked, err := mgr.AutoLinkCommits(since)
		if err != nil {
			return err
		}

		if linked == 0 {
			cmd.Println("No new commits linked to tasks.")
		} else {
			cmd.Printf("Linked %d commit(s) to tasks.\n", linked)
		}

		return nil
	},
}

var taskSetPRCmd = &cobra.Command{
	Use:   "set-pr <task-id>",
	Short: "Set pull request metadata for a task",
	Long: `Associate a pull request with a task for traceability.

Examples:
  openexec task set-pr T-001 --number 123 --url https://github.com/org/repo/pull/123`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		taskID := args[0]

		mgr, err := getReleaseManager(cmd)
		if err != nil {
			return err
		}

		prNumber, _ := cmd.Flags().GetInt("number")
		prURL, _ := cmd.Flags().GetString("url")

		if prNumber == 0 && prURL == "" {
			return fmt.Errorf("at least --number or --url is required")
		}

		if err := mgr.SetTaskPR(taskID, prNumber, prURL); err != nil {
			return err
		}

		cmd.Printf("Updated PR metadata for task %s\n", taskID)
		if prNumber > 0 {
			cmd.Printf("  PR #%d\n", prNumber)
		}
		if prURL != "" {
			cmd.Printf("  URL: %s\n", prURL)
		}
		return nil
	},
}

var taskApproveCmd = &cobra.Command{
	Use:   "approve <task-id>",
	Short: "Approve a task",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		taskID := args[0]

		mgr, err := getReleaseManager(cmd)
		if err != nil {
			return err
		}

		if !mgr.GetConfig().ApprovalEnabled {
			cmd.Println("Approval workflow is not enabled.")
			return nil
		}

		approverID, err := resolveApproverIdentity(cmd, mgr)
		if err != nil {
			return err
		}
		comments, _ := cmd.Flags().GetString("comments")

		if err := mgr.ApproveTask(taskID, approverID, comments); err != nil {
			return err
		}

		cmd.Printf("Task %s approved by %s\n", taskID, approverID)
		return nil
	},
}

var goalCmd = &cobra.Command{
	Use:   "goal",
	Short: "Manage project goals",
}

var goalVerifyCmd = &cobra.Command{
	Use:   "verify [goal-id]",
	Short: "Verify implementation against goals",
	Long: `Run high-level verification scripts to confirm that project goals
have been met by the current implementation.

Examples:
  openexec goal verify G-001
  openexec goal verify`,
	RunE: func(cmd *cobra.Command, args []string) error {
		mgr, err := getReleaseManager(cmd)
		if err != nil {
			return err
		}

		execute, _ := cmd.Flags().GetBool("execute")

		stories := mgr.GetStories()
		goalMap := make(map[string][]*release.Story)
		for _, s := range stories {
			if s.GoalID != "" {
				goalMap[s.GoalID] = append(goalMap[s.GoalID], s)
			}
		}

		cmd.Println("📋 Goal Verification Report")
		cmd.Println("==========================")

		if len(args) > 0 {
			goalID := args[0]
			if targetStories, ok := goalMap[goalID]; ok {
				cmd.Printf("Goal %s:\n", goalID)
				verifyStories(cmd, targetStories, execute)
			} else {
				cmd.Printf("Goal %s not found or has no supporting stories.\n", goalID)
			}
		} else {
			if len(goalMap) == 0 {
				cmd.Println("No goals tracked in current stories.")
				return nil
			}
			for goalID, targetStories := range goalMap {
				cmd.Printf("\nGoal %s [%d stories]:\n", goalID, len(targetStories))
				allDone := verifyStories(cmd, targetStories, execute)
				if allDone && !execute {
					cmd.Printf("  ✨ Goal %s is implementations-complete.\n", goalID)
				} else if allDone && execute {
					cmd.Printf("  ✨ Goal %s PASSES all verification scripts.\n", goalID)
				}
			}
		}

		return nil
	},
}

func verifyStories(cmd *cobra.Command, stories []*release.Story, execute bool) bool {
	allDone := true
	for _, s := range stories {
		cmd.Printf("  %s %s: %s\n", statusIcon(s.Status), s.ID, s.Title)

		if s.Status != "done" && s.Status != "completed" && s.Status != "approved" {
			allDone = false
		}

		if s.VerificationScript != "" {
			cmd.Printf("    Verification: %s\n", s.VerificationScript)
			if execute {
				cmd.Printf("    Running verification...\n")
				verifyCmd := exec.Command("bash", "-c", s.VerificationScript)
				output, err := verifyCmd.CombinedOutput()
				if err != nil {
					cmd.Printf("    ✗ FAILED:\n%s\n", string(output))
					allDone = false
				} else {
					cmd.Printf("    ✓ PASSED\n")
				}
			}
		} else if execute {
			cmd.Printf("    ⚠ No verification script provided. Skipping execution.\n")
		}
	}
	return allDone
}

var releaseResetCmd = &cobra.Command{
	Use:   "reset",
	Short: "Reset all stories and tasks to pending status",
	RunE: func(cmd *cobra.Command, args []string) error {
		mgr, err := getReleaseManager(cmd)
		if err != nil {
			return err
		}

		if err := mgr.ResetStatuses(); err != nil {
			return err
		}

		cmd.Println("✓ All stories and tasks have been reset to pending status.")
		return nil
	},
}

func init() {
	rootCmd.AddCommand(releaseCmd)

	// Goal subcommands
	rootCmd.AddCommand(goalCmd)
	goalCmd.AddCommand(goalVerifyCmd)
	goalVerifyCmd.Flags().Bool("execute", false, "Execute verification scripts locally")

	// Release subcommands
	releaseCmd.AddCommand(releaseCreateCmd)
	releaseCreateCmd.Flags().String("name", "", "Release name")
	releaseCreateCmd.Flags().String("description", "", "Release description")

	releaseCmd.AddCommand(releaseShowCmd)
	releaseShowCmd.Flags().Bool("json", false, "Output as JSON")

	releaseCmd.AddCommand(releaseResetCmd)

    releaseCmd.AddCommand(releaseChangelogCmd)
    // story subcommands
    storyCmd.AddCommand(storyExportCmd)
    rootCmd.AddCommand(storyCmd)
	releaseChangelogCmd.Flags().Bool("json", false, "Output as JSON")
	releaseChangelogCmd.Flags().String("output", "", "Write to file instead of stdout")
	releaseChangelogCmd.Flags().String("repo-url", "", "Repository URL for commit links")
	releaseChangelogCmd.Flags().Bool("include-approvals", false, "Include approval information")
	releaseChangelogCmd.Flags().Bool("release-notes", false, "Generate brief release notes format")

	releaseCmd.AddCommand(releaseTagCmd)
	releaseTagCmd.Flags().String("message", "", "Tag message")
	releaseTagCmd.Flags().Bool("push", false, "Push tag to origin after creation")

	releaseCmd.AddCommand(releaseProcessCmd)
	releaseProcessCmd.Flags().Bool("push", false, "Push tags and branches to origin")
	releaseProcessCmd.Flags().Bool("dry-run", false, "Preview actions without executing")

	releaseCmd.AddCommand(releasePushBranchCmd)
	releasePushBranchCmd.Flags().String("branch", "", "Override branch to push (default: release branch)")

	releaseCmd.AddCommand(releaseApproveCmd)
	releaseApproveCmd.Flags().String("approver", "", "Approver ID/name (uses git user if not specified)")
	releaseApproveCmd.Flags().String("email", "", "Approver email (uses git email if not specified)")
	releaseApproveCmd.Flags().String("comments", "", "Approval comments")

	releaseCmd.AddCommand(releaseApprovalsCmd)
	releaseApprovalsCmd.Flags().Bool("json", false, "Output as JSON")

	storyCmd.AddCommand(storyCreateCmd)
	storyCreateCmd.Flags().String("description", "", "Story description")
	storyCreateCmd.Flags().String("type", "feature", "Story type (feature, bugfix, enhancement, chore)")
	storyCreateCmd.Flags().Int("priority", 0, "Priority (0=normal, 1=high, 2=critical)")

	storyCmd.AddCommand(storyListCmd)
	storyListCmd.Flags().Bool("json", false, "Output as JSON")

	storyCmd.AddCommand(storyShowCmd)
	storyShowCmd.Flags().Bool("json", false, "Output as JSON")

	storyCmd.AddCommand(storyMergeCmd)

	storyCmd.AddCommand(storyApproveCmd)
	storyApproveCmd.Flags().String("approver", "", "Approver ID/name (uses git user if not specified)")
	storyApproveCmd.Flags().String("email", "", "Approver email (uses git email if not specified)")
	storyApproveCmd.Flags().String("comments", "", "Approval comments")

	storyImportCmd.Flags().Bool("dry-run", false, "Preview import without creating stories/tasks")
	storyImportCmd.Flags().Bool("reassign", false, "Reassign existing tasks to stories if StoryID is missing")
	storyImportCmd.Flags().Bool("prune-orphans", false, "Prune legacy tasks not listed in the stories file")
	storyCmd.AddCommand(storyImportCmd)

	// Task subcommands
	rootCmd.AddCommand(taskCmd)

	taskCmd.AddCommand(taskCreateCmd)
	taskCreateCmd.Flags().String("story", "", "Story ID to link the task to (required)")
	taskCreateCmd.Flags().String("description", "", "Task description")
	taskCreateCmd.Flags().Bool("needs-review", false, "Mark task as requiring review before approval")

	taskCmd.AddCommand(taskCompleteCmd)

	taskCmd.AddCommand(taskListCmd)
	taskListCmd.Flags().Bool("json", false, "Output as JSON")
	taskListCmd.Flags().String("story", "", "Filter by story ID")

	taskCmd.AddCommand(taskLinkCmd)

	taskCmd.AddCommand(taskAutolinkCmd)
	taskAutolinkCmd.Flags().String("since", "", "Link commits since this hash")
	taskAutolinkCmd.Flags().Bool("all", false, "Link all commits in history")

	taskCmd.AddCommand(taskSetPRCmd)
	taskSetPRCmd.Flags().Int("number", 0, "Pull request number")
	taskSetPRCmd.Flags().String("url", "", "Pull request URL")

	taskCmd.AddCommand(taskApproveCmd)
	taskApproveCmd.Flags().String("approver", "", "Approver ID/name (uses git user if not specified)")
	taskApproveCmd.Flags().String("email", "", "Approver email (uses git email if not specified)")
	taskApproveCmd.Flags().String("comments", "", "Approval comments")
}

func getReleaseManager(cmd *cobra.Command) (*release.Manager, error) {
	projectDir, _ := cmd.Flags().GetString("project-dir")
	if projectDir == "" {
		var err error
		projectDir, err = os.Getwd()
		if err != nil {
			return nil, err
		}
	}

	// Load config from .openexec/config.json if exists
	cfg := loadReleaseConfig(projectDir)

	return release.NewManager(projectDir, cfg)
}

func loadReleaseConfig(projectDir string) *release.Config {
	return release.LoadConfig(projectDir)
}

// resolveApproverIdentity resolves the approver and email from flags or git config.
// Returns the full approverID string (e.g., "Name <email>"), approver name only if
// empty, or error if approver cannot be determined.
func resolveApproverIdentity(cmd *cobra.Command, mgr *release.Manager) (string, error) {
	approver, _ := cmd.Flags().GetString("approver")
	email, _ := cmd.Flags().GetString("email")

	// Get approver identity from git config if not specified
	if approver == "" || email == "" {
		gitName, gitEmail := mgr.GetGitIdentity()
		if approver == "" {
			approver = gitName
		}
		if email == "" {
			email = gitEmail
		}
	}

	if approver == "" {
		return "", fmt.Errorf("--approver is required (or configure git user.name)")
	}

	if email != "" {
		return fmt.Sprintf("%s <%s>", approver, email), nil
	}
	return approver, nil
}

func statusIcon(status string) string {
	switch status {
	case "done", "completed":
		return "[x]"
	case "in_progress":
		return "[-]"
	case "approved":
		return "[+]"
	case "failed":
		return "[!]"
	case "needs_review", "ready_for_review":
		return "[?]"
	default:
		return "[ ]"
	}
}
