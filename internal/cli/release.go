package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"regexp"

	"github.com/spf13/cobra"
	"github.com/openexec/openexec/internal/release"
)

var releaseCmd = &cobra.Command{
	Use:   "release",
	Short: "Manage releases, stories, and generate changelogs",
	Long: `Release management commands for tracking releases, stories, tasks,
and generating ISO 27001 compliant documentation.

Git integration and approval workflows can be enabled via configuration.`,
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

		fmt.Printf("Created release: %s (v%s)\n", rel.Name, rel.Version)
		if rel.Git != nil && rel.Git.Branch != "" {
			fmt.Printf("  Branch: %s\n", rel.Git.Branch)
		}
		if mgr.GetConfig().ApprovalEnabled {
			fmt.Printf("  Approval required: yes\n")
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
			fmt.Println("No release defined. Create one with: openexec release create <version>")
			return nil
		}

		jsonOutput, _ := cmd.Flags().GetBool("json")
		if jsonOutput {
			data, err := json.MarshalIndent(rel, "", "  ")
			if err != nil {
				return err
			}
			fmt.Println(string(data))
			return nil
		}

		fmt.Printf("Release: %s\n", rel.Name)
		fmt.Printf("  Version: %s\n", rel.Version)
		fmt.Printf("  Status: %s\n", rel.Status)
		if rel.Description != "" {
			fmt.Printf("  Description: %s\n", rel.Description)
		}
		fmt.Printf("  Stories: %d\n", len(rel.Stories))
		if rel.Git != nil {
			fmt.Printf("  Branch: %s\n", rel.Git.Branch)
			if rel.Git.Tag != "" {
				fmt.Printf("  Tag: %s\n", rel.Git.Tag)
			}
		}
		if rel.Approval != nil {
			fmt.Printf("  Approval: %s\n", rel.Approval.Status)
			if rel.Approval.ApprovedBy != "" {
				fmt.Printf("  Approved by: %s\n", rel.Approval.ApprovedBy)
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
			fmt.Println(string(output))
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
			fmt.Printf("Changelog written to %s\n", outputFile)
		} else {
			fmt.Println(changelog)
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
			fmt.Printf("Created tag: %s\n", rel.Git.Tag)

			if push {
				if err := mgr.PushTag(rel.Git.Tag); err != nil {
					return fmt.Errorf("tag created but push failed: %w", err)
				}
				fmt.Printf("Pushed tag to origin\n")
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
				fmt.Println("[dry-run] No stories ready to merge.")
				fmt.Println("Stories must be complete (all tasks done) and approved (if approval_enabled).")
				return nil
			}

			fmt.Printf("[dry-run] Would process %d stories:\n", len(result.WouldMerge))
			for _, id := range result.WouldMerge {
				story := mgr.GetStory(id)
				branchInfo := ""
				if story != nil && story.Git != nil {
					branchInfo = fmt.Sprintf(" (%s)", story.Git.Branch)
				}
				fmt.Printf("  - Would merge %s%s to release\n", id, branchInfo)
			}

			if result.ReleaseComplete {
				fmt.Println("\n[dry-run] Release would be complete!")
				if result.WouldTag {
					rel := mgr.GetRelease()
					fmt.Printf("  Would create tag: v%s\n", rel.Version)
					if push {
						fmt.Printf("  Would push tag to origin\n")
					}
				}
				if result.WouldMergeToMain {
					fmt.Printf("  Would merge to: %s\n", mgr.GetConfig().BaseBranch)
					if push {
						fmt.Printf("  Would push %s to origin\n", mgr.GetConfig().BaseBranch)
					}
				}
				if !result.WouldTag && !result.WouldMergeToMain && mgr.GetConfig().ApprovalEnabled {
					rel := mgr.GetRelease()
					if rel.Approval == nil || rel.Approval.Status != release.ApprovalApproved {
						fmt.Println("  Would await release approval before tag/merge")
					}
				}
			}

			fmt.Println("\nRun without --dry-run to execute these actions.")
			return nil
		}

		// Handle actual execution
		if len(result.StoriesMerged) == 0 {
			fmt.Println("No stories ready to merge.")
			fmt.Println("Stories must be complete (all tasks done) and approved (if approval_enabled).")
			fmt.Println("\nTip: Use --dry-run to preview what would happen.")
			return nil
		}

		fmt.Printf("Processed %d stories:\n", len(result.StoriesMerged))
		for _, id := range result.StoriesMerged {
			fmt.Printf("  - Merged %s to release\n", id)
		}

		if len(result.Errors) > 0 {
			fmt.Println("\nErrors:")
			for _, e := range result.Errors {
				fmt.Printf("  - %s\n", e)
			}
		}

		if result.ReleaseComplete {
			fmt.Println("\nRelease complete!")

			rel := mgr.GetRelease()
			if result.ReleaseTagged {
				fmt.Printf("  Tag: %s\n", rel.Git.Tag)
				if push {
					if err := mgr.PushTag(rel.Git.Tag); err != nil {
						fmt.Printf("  Warning: failed to push tag: %v\n", err)
					} else {
						fmt.Printf("  Pushed tag to origin\n")
					}
				}
			}
			if result.ReleaseMergedToMain {
				fmt.Printf("  Merged to: %s\n", mgr.GetConfig().BaseBranch)
				if push {
					if err := mgr.PushBranch(mgr.GetConfig().BaseBranch); err != nil {
						fmt.Printf("  Warning: failed to push %s: %v\n", mgr.GetConfig().BaseBranch, err)
					} else {
						fmt.Printf("  Pushed %s to origin\n", mgr.GetConfig().BaseBranch)
					}
				}
			}

			if !result.ReleaseTagged && !result.ReleaseMergedToMain {
				if mgr.GetConfig().ApprovalEnabled {
					if rel.Approval == nil || rel.Approval.Status != release.ApprovalApproved {
						fmt.Println("  Awaiting release approval before tag/merge")
						fmt.Println("  Run: openexec release approve --approver <name>")
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

		fmt.Printf("Pushed branch %s to origin\n", branch)
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
			fmt.Println("Approval workflow is not enabled.")
			fmt.Println("Enable it with: openexec config set approval_enabled true")
			return nil
		}

		approver, _ := cmd.Flags().GetString("approver")
		email, _ := cmd.Flags().GetString("email")
		comments, _ := cmd.Flags().GetString("comments")

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
			return fmt.Errorf("--approver is required (or configure git user.name)")
		}

		approverID := approver
		if email != "" {
			approverID = fmt.Sprintf("%s <%s>", approver, email)
		}

		if err := mgr.ApproveRelease(approverID, comments); err != nil {
			return err
		}

		fmt.Printf("Release approved by %s\n", approverID)
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
			fmt.Println("Approval workflow is not enabled.")
			fmt.Println("Enable it with: openexec config set approval_enabled true")
			return nil
		}

		pending := mgr.GetPendingApprovals()

		jsonOutput, _ := cmd.Flags().GetBool("json")
		if jsonOutput {
			data, err := json.MarshalIndent(pending, "", "  ")
			if err != nil {
				return err
			}
			fmt.Println(string(data))
			return nil
		}

		hasItems := false

		if len(pending.Tasks) > 0 {
			hasItems = true
			fmt.Printf("Tasks awaiting approval (%d):\n", len(pending.Tasks))
			for _, t := range pending.Tasks {
				fmt.Printf("  - %s: %s [%s]\n", t.ID, t.Title, t.Status)
			}
			fmt.Println()
		}

		if len(pending.Stories) > 0 {
			hasItems = true
			fmt.Printf("Stories awaiting approval (%d):\n", len(pending.Stories))
			for _, s := range pending.Stories {
				fmt.Printf("  - %s: %s\n", s.ID, s.Title)
			}
			fmt.Println()
		}

		if pending.Release != nil {
			hasItems = true
			fmt.Printf("Release awaiting approval:\n")
			fmt.Printf("  - %s (v%s)\n", pending.Release.Name, pending.Release.Version)
			fmt.Println()
		}

		if !hasItems {
			fmt.Println("No items awaiting approval.")
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

		fmt.Printf("Created story: %s\n", storyID)
		fmt.Printf("  Title: %s\n", title)
		fmt.Printf("  Type: %s\n", storyType)
		if story.Git != nil && story.Git.Branch != "" {
			fmt.Printf("  Branch: %s\n", story.Git.Branch)
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
			fmt.Println(string(data))
			return nil
		}

		if len(stories) == 0 {
			fmt.Println("No stories found.")
			return nil
		}

		fmt.Printf("Stories (%d):\n\n", len(stories))
		for _, story := range stories {
			status := statusIcon(story.Status)
			fmt.Printf("%s %s: %s [%s]\n", status, story.ID, story.Title, story.StoryType)
			if story.Git != nil && story.Git.Branch != "" {
				fmt.Printf("     Branch: %s\n", story.Git.Branch)
			}
			tasks := mgr.GetTasksForStory(story.ID)
			if len(tasks) > 0 {
				completed := 0
				for _, t := range tasks {
					if t.Status == release.TaskStatusDone {
						completed++
					}
				}
				fmt.Printf("     Tasks: %d/%d completed\n", completed, len(tasks))
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
			fmt.Println(string(data))
			return nil
		}

		fmt.Printf("Story: %s\n", story.ID)
		fmt.Printf("  Title: %s\n", story.Title)
		fmt.Printf("  Type: %s\n", story.StoryType)
		fmt.Printf("  Status: %s\n", story.Status)
		fmt.Printf("  Priority: %d\n", story.Priority)

		if story.Git != nil {
			fmt.Printf("\nGit:\n")
			fmt.Printf("  Branch: %s\n", story.Git.Branch)
			if story.Git.MergedTo != "" {
				fmt.Printf("  Merged to: %s\n", story.Git.MergedTo)
				fmt.Printf("  Merge commit: %s\n", story.Git.MergeCommit)
			}
		}

		if story.Approval != nil {
			fmt.Printf("\nApproval:\n")
			fmt.Printf("  Status: %s\n", story.Approval.Status)
			if story.Approval.ApprovedBy != "" {
				fmt.Printf("  Approved by: %s\n", story.Approval.ApprovedBy)
			}
		}

		tasks := mgr.GetTasksForStory(storyID)
		if len(tasks) > 0 {
			fmt.Printf("\nTasks (%d):\n", len(tasks))
			for _, task := range tasks {
				status := statusIcon(task.Status)
				fmt.Printf("  %s %s: %s\n", status, task.ID, task.Title)
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

		fmt.Printf("Story %s merged to release\n", storyID)
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
			fmt.Println("Approval workflow is not enabled.")
			return nil
		}

		approver, _ := cmd.Flags().GetString("approver")
		email, _ := cmd.Flags().GetString("email")
		comments, _ := cmd.Flags().GetString("comments")

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
			return fmt.Errorf("--approver is required (or configure git user.name)")
		}

		approverID := approver
		if email != "" {
			approverID = fmt.Sprintf("%s <%s>", approver, email)
		}

		if err := mgr.ApproveStory(storyID, approverID, comments); err != nil {
			return err
		}

		fmt.Printf("Story %s approved by %s\n", storyID, approverID)

		// Process approved stories to trigger auto-merge if configured
		if mgr.GetConfig().AutoMergeStories {
			result, err := mgr.ProcessApprovedStories()
			if err != nil {
				fmt.Printf("Warning: failed to process approved stories: %v\n", err)
			} else if len(result.StoriesMerged) > 0 {
				for _, id := range result.StoriesMerged {
					fmt.Printf("  Auto-merged story %s to release\n", id)
				}
				if result.ReleaseComplete {
					if mgr.GetConfig().ApprovalEnabled {
						rel := mgr.GetRelease()
						if rel != nil && (rel.Approval == nil || rel.Approval.Status != release.ApprovalApproved) {
							fmt.Printf("  Release complete but awaiting approval\n")
							fmt.Printf("  Run: openexec release approve --approver <name>\n")
						} else {
							fmt.Printf("  Release complete and approved!\n")
						}
					} else {
						fmt.Printf("  Release complete!\n")
					}
					if result.ReleaseTagged {
						fmt.Printf("  Created release tag\n")
					}
					if result.ReleaseMergedToMain {
						fmt.Printf("  Merged release to main\n")
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
	ID                 string          `json:"id"`
	Title              string          `json:"title"`
	Description        string          `json:"description"`
	AcceptanceCriteria []string        `json:"acceptance_criteria"`
	GoalID             string          `json:"goal_id,omitempty"`
	DependsOn          []string        `json:"depends_on,omitempty"`
	VerificationScript string          `json:"verification_script,omitempty"`
	Contract           string          `json:"contract,omitempty"`
	Tasks              []GeneratedTask `json:"tasks"`
}

var storyImportCmd = &cobra.Command{
	Use:   "import [file]",
	Short: "Import stories and tasks from stories.json",
	Long: `Import stories and tasks from a generated stories.json file.

This command reads the stories.json file (generated by 'openexec plan') and
creates all stories and their tasks in the tracking system.

If no file is specified, defaults to .openexec/stories.json

Examples:
  openexec story import                          # Import from .openexec/stories.json
  openexec story import custom-stories.json      # Import from custom file
  openexec story import --dry-run                # Preview what would be imported`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		// Determine input file
		inputFile := ".openexec/stories.json"
		if len(args) > 0 {
			inputFile = args[0]
		}

		dryRun, _ := cmd.Flags().GetBool("dry-run")

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
			fmt.Println("Warning: missing schema_version in stories file")
		} else if sf.SchemaVersion != "1.0" && sf.SchemaVersion != "1.1" && sf.SchemaVersion != "legacy" {
			return fmt.Errorf("unsupported stories schema version: %s (expected 1.0 or 1.1)", sf.SchemaVersion)
		}

		stories := sf.Stories

		if len(stories) == 0 {
			fmt.Println("No stories found in file.")
			return nil
		}

		// PLANNING GATE: Enforce goal coverage and verifiability
		if len(sf.Goals) > 0 {
			fmt.Println("Running Planning Gate checks...")
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
			fmt.Println("✓ Planning Gate passed.")
		}

		if dryRun {
			fmt.Printf("Would import %d goals and %d stories:\n\n", len(sf.Goals), len(stories))
			for _, g := range sf.Goals {
				fmt.Printf("  Goal %s: %s\n", g.ID, g.Description)
			}
			for _, s := range stories {
				fmt.Printf("  %s [%s]: %s\n", s.ID, s.GoalID, s.Title)
				for _, t := range s.Tasks {
					id := t.ID
					if id == "" {
						// Fallback ID if not provided
						id = fmt.Sprintf("T-%s-%03d", s.ID, 0) // Just placeholder for dry-run
					}
					fmt.Printf("    %s: %s\n", id, t.Title)
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
					fmt.Printf("  [error] %s: %v\n", g.ID, err)
					continue
				}
				goalsCreated++
			}
		}

		// Track counts
		storiesCreated := 0
		tasksCreated := 0
		skipped := 0

		fmt.Printf("Importing %d stories from %s...\n\n", len(stories), inputFile)

		storyIDPattern := regexp.MustCompile(`^(US|REQ)-\d{3}$`)

		for _, genStory := range stories {
			// Validate ID format
			if !storyIDPattern.MatchString(genStory.ID) {
				fmt.Printf("  [error] %s: invalid ID format (expected US-### or REQ-###)\n", genStory.ID)
				continue
			}

			// Check if story already exists
			existing := mgr.GetStory(genStory.ID)
			if existing != nil {
				fmt.Printf("  [skip] %s: already exists\n", genStory.ID)
				skipped++
				continue
			}

			// Create story
			story := &release.Story{
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
			}

			if err := mgr.CreateStory(story); err != nil {
				fmt.Printf("  [error] %s: %v\n", genStory.ID, err)
				continue
			}

			storiesCreated++
			fmt.Printf("  [created] %s: %s\n", genStory.ID, genStory.Title)

			// Create tasks for this story
			for i, genTask := range genStory.Tasks {
				taskID := genTask.ID
				if taskID == "" {
					taskID = fmt.Sprintf("T-%s-%03d", genStory.ID, i+1)
				}

				task := &release.Task{
					ID:                 taskID,
					Title:              genTask.Title,
					Description:        genTask.Description,
					StoryID:            genStory.ID,
					DependsOn:          genTask.DependsOn,
					VerificationScript: genTask.VerificationScript,
				}

				if err := mgr.CreateTask(task); err != nil {
					fmt.Printf("    [error] %s: %v\n", taskID, err)
					continue
				}

				tasksCreated++
				fmt.Printf("    [created] %s: %s\n", taskID, genTask.Title)
			}
		}

		fmt.Printf("\nImport complete:\n")
		fmt.Printf("  Stories created: %d\n", storiesCreated)
		fmt.Printf("  Tasks created: %d\n", tasksCreated)
		if skipped > 0 {
			fmt.Printf("  Skipped (existing): %d\n", skipped)
		}

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

		fmt.Printf("Created task: %s\n", taskID)
		fmt.Printf("  Title: %s\n", title)
		fmt.Printf("  Story: %s\n", storyID)
		if task.Git != nil && task.Git.Branch != "" {
			fmt.Printf("  Branch: %s\n", task.Git.Branch)
		}
		if needsReview {
			fmt.Printf("  Needs review: yes\n")
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

		fmt.Printf("Task %s marked as done\n", taskID)

		if result.StoryComplete {
			if result.AwaitingApproval {
				fmt.Printf("  Story %s is complete but awaiting approval\n", result.StoryID)
				fmt.Printf("  Run: openexec story approve %s\n", result.StoryID)
			} else if result.StoryMerged {
				fmt.Printf("  Story %s auto-merged to release\n", result.StoryID)
			}
		}

		if result.ReleaseComplete {
			fmt.Println("  Release is complete!")
			if result.ReleaseTagged {
				fmt.Println("  Release tagged")
			}
			if result.ReleaseMergedToMain {
				fmt.Println("  Release merged to main")
			}
			if !result.ReleaseTagged && !result.ReleaseMergedToMain {
				rel := mgr.GetRelease()
				if rel != nil && mgr.GetConfig().ApprovalEnabled {
					if rel.Approval == nil || rel.Approval.Status != release.ApprovalApproved {
						fmt.Println("  Awaiting release approval")
						fmt.Println("  Run: openexec release approve")
					}
				}
			}
		}

		if result.Error != "" {
			fmt.Printf("  Warning: %s\n", result.Error)
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
			fmt.Println(string(data))
			return nil
		}

		if len(tasks) == 0 {
			fmt.Println("No tasks found.")
			return nil
		}

		fmt.Printf("Tasks (%d):\n\n", len(tasks))
		for _, task := range tasks {
			status := statusIcon(task.Status)
			fmt.Printf("%s %s: %s [%s]\n", status, task.ID, task.Title, task.StoryID)
			if task.Git != nil && len(task.Git.Commits) > 0 {
				fmt.Printf("     Commits: %d\n", len(task.Git.Commits))
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
		fmt.Printf("Linked commit %s to task %s\n", shortHash, taskID)
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
			fmt.Println("No new commits linked to tasks.")
		} else {
			fmt.Printf("Linked %d commit(s) to tasks.\n", linked)
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

		fmt.Printf("Updated PR metadata for task %s\n", taskID)
		if prNumber > 0 {
			fmt.Printf("  PR #%d\n", prNumber)
		}
		if prURL != "" {
			fmt.Printf("  URL: %s\n", prURL)
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
			fmt.Println("Approval workflow is not enabled.")
			return nil
		}

		approver, _ := cmd.Flags().GetString("approver")
		email, _ := cmd.Flags().GetString("email")
		comments, _ := cmd.Flags().GetString("comments")

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
			return fmt.Errorf("--approver is required (or configure git user.name)")
		}

		// Include email in approver identity
		approverID := approver
		if email != "" {
			approverID = fmt.Sprintf("%s <%s>", approver, email)
		}

		if err := mgr.ApproveTask(taskID, approverID, comments); err != nil {
			return err
		}

		fmt.Printf("Task %s approved by %s\n", taskID, approverID)
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

		fmt.Println("📋 Goal Verification Report")
		fmt.Println("==========================")

		if len(args) > 0 {
			goalID := args[0]
			if targetStories, ok := goalMap[goalID]; ok {
				fmt.Printf("Goal %s:\n", goalID)
				verifyStories(targetStories, execute)
			} else {
				fmt.Printf("Goal %s not found or has no supporting stories.\n", goalID)
			}
		} else {
			if len(goalMap) == 0 {
				fmt.Println("No goals tracked in current stories.")
				return nil
			}
			for goalID, targetStories := range goalMap {
				fmt.Printf("\nGoal %s [%d stories]:\n", goalID, len(targetStories))
				allDone := verifyStories(targetStories, execute)
				if allDone && !execute {
					fmt.Printf("  ✨ Goal %s is implementations-complete.\n", goalID)
				} else if allDone && execute {
					fmt.Printf("  ✨ Goal %s PASSES all verification scripts.\n", goalID)
				}
			}
		}

		return nil
	},
}

func verifyStories(stories []*release.Story, execute bool) bool {
	allDone := true
	for _, s := range stories {
		fmt.Printf("  %s %s: %s\n", statusIcon(s.Status), s.ID, s.Title)
		
		if s.Status != "done" && s.Status != "completed" && s.Status != "approved" {
			allDone = false
		}
		
		if s.VerificationScript != "" {
			fmt.Printf("    Verification: %s\n", s.VerificationScript)
			if execute {
				fmt.Printf("    Running verification...\n")
				verifyCmd := exec.Command("bash", "-c", s.VerificationScript)
				output, err := verifyCmd.CombinedOutput()
				if err != nil {
					fmt.Printf("    ✗ FAILED:\n%s\n", string(output))
					allDone = false
				} else {
					fmt.Printf("    ✓ PASSED\n")
				}
			}
		} else if execute {
			fmt.Printf("    ⚠ No verification script provided. Skipping execution.\n")
		}
	}
	return allDone
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

	releaseCmd.AddCommand(releaseChangelogCmd)
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

	// Story subcommands
	rootCmd.AddCommand(storyCmd)

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

	storyCmd.AddCommand(storyImportCmd)
	storyImportCmd.Flags().Bool("dry-run", false, "Preview import without creating stories/tasks")

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
	cfg := release.DefaultConfig()

	// Try to load from config file
	configPath := projectDir + "/.openexec/config.json"
	data, err := os.ReadFile(configPath)
	if err != nil {
		return cfg
	}

	var fileConfig struct {
		GitEnabled          *bool   `json:"git_enabled"`
		ApprovalEnabled     *bool   `json:"approval_enabled"`
		BaseBranch          *string `json:"base_branch"`
		AutoMergeStories    *bool   `json:"auto_merge_stories"`
		AutoMergeToMain     *bool   `json:"auto_merge_to_main"`
		AutoTagRelease      *bool   `json:"auto_tag_release"`
		AutoLinkCommits     *bool   `json:"auto_link_commits"`
		ReleaseBranchPrefix *string `json:"release_branch_prefix"`
		FeatureBranchPrefix *string `json:"feature_branch_prefix"`
	}

	if err := json.Unmarshal(data, &fileConfig); err != nil {
		return cfg
	}

	if fileConfig.GitEnabled != nil {
		cfg.GitEnabled = *fileConfig.GitEnabled
	}
	if fileConfig.ApprovalEnabled != nil {
		cfg.ApprovalEnabled = *fileConfig.ApprovalEnabled
	}
	if fileConfig.BaseBranch != nil {
		cfg.BaseBranch = *fileConfig.BaseBranch
	}
	if fileConfig.AutoMergeStories != nil {
		cfg.AutoMergeStories = *fileConfig.AutoMergeStories
	}
	if fileConfig.AutoMergeToMain != nil {
		cfg.AutoMergeToMain = *fileConfig.AutoMergeToMain
	}
	if fileConfig.AutoTagRelease != nil {
		cfg.AutoTagRelease = *fileConfig.AutoTagRelease
	}
	if fileConfig.AutoLinkCommits != nil {
		cfg.AutoLinkCommits = *fileConfig.AutoLinkCommits
	}
	if fileConfig.ReleaseBranchPrefix != nil {
		cfg.ReleaseBranchPrefix = *fileConfig.ReleaseBranchPrefix
	}
	if fileConfig.FeatureBranchPrefix != nil {
		cfg.FeatureBranchPrefix = *fileConfig.FeatureBranchPrefix
	}

	return cfg
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
