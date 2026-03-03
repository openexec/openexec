package cli

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/openexec/openexec/internal/git"
	"github.com/openexec/openexec/internal/project"
	"github.com/spf13/cobra"
)

var (
	initPlannerModel   string
	initExecutorModel  string
	initReviewEnabled  bool
	initReviewerModel  string
	initNonInteractive bool
)

// Available models by provider
var availableModels = []struct {
	Provider string
	Model    string
	Name     string
}{
	// Claude (Anthropic)
	{"claude", "opus", "Opus"},
	{"claude", "sonnet", "Sonnet"},
	{"claude", "haiku", "Haiku"},
	// Codex (OpenAI)
	{"codex", "gpt-5", "GPT-5"},
	{"codex", "gpt-5-codex", "GPT-5 Codex"},
	// Gemini (Google)
	{"gemini", "gemini-3.1-pro-preview", "Gemini 3.1 Pro"},
	{"gemini", "gemini-3.1-flash-preview", "Gemini 3.1 Flash"},
}

var initCmd = &cobra.Command{
	Use:   "init [project-name]",
	Short: "Initialize a new OpenExec project",
	Long: `Initialize a new OpenExec project in the current directory.

This command sets up the project infrastructure including:
- Tract state engine for tracking project state
- Engram memory context for storing execution context
- Execution settings (executor model, reviewer model, etc.)

The project name defaults to the current directory name if not provided.

Recommended for new projects:
  openexec wizard (interactive, guided setup)

Examples:
  # Interactive initialization
  openexec init

  # Non-interactive with specific models
  openexec init --executor sonnet --review --reviewer opus

  # Use Codex for execution, Claude for review
  openexec init --executor gpt-5-codex --review --reviewer opus

  # Non-interactive without reviewer
  openexec init --executor sonnet --no-review

  # Quick non-interactive with defaults
  openexec init -y`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		var projectName string
		if len(args) > 0 {
			projectName = args[0]
		}

		// Check if project is already initialized
		projectDir, err := cmd.Flags().GetString("project-dir")
		if err != nil {
			projectDir = "."
		}

		// Enforce git repository presence (gitflow precondition)
		if projectDir == "." {
			if cwd, err := os.Getwd(); err == nil {
				projectDir = cwd
			}
		}
		gitClient := git.NewClient(git.Config{Enabled: true, RepoPath: projectDir})
		if !gitClient.IsRepo() {
			return fmt.Errorf("this directory is not a git repository; initialize git first (git init && git remote add origin ...) or run in a repo")
		}

		// Helpful gitflow hints (non-fatal)
		if remoteURL, err := gitClient.GetRemoteURL(); err != nil || remoteURL == "" {
			fmt.Println("Hint: no 'origin' remote configured. Configure one with: git remote add origin <url>")
		}
		hasBase := gitClient.BranchExists("main") || gitClient.BranchExists("origin/main") ||
			gitClient.BranchExists("develop") || gitClient.BranchExists("origin/develop")
		if !hasBase {
			fmt.Println("Hint: no base branch ('main' or 'develop') found. Create one, e.g.: git checkout -b main && git push -u origin main")
		}

		config, err := project.LoadProjectConfig(projectDir)
		if err == nil && config != nil {
			return fmt.Errorf("project already initialized: %s", config.Name)
		}

		// Configure execution settings
		var plannerModel, executorModel, reviewerModel string
		var reviewEnabled, parallelEnabled bool
		var workerCount int

		// Interactive mode if not explicitly set via flags
		if !initNonInteractive {
			plannerModel, executorModel, reviewEnabled, reviewerModel, parallelEnabled, workerCount = promptExecutionConfig(cmd)
		} else {
			plannerModel = initPlannerModel
			executorModel = initExecutorModel
			reviewEnabled = initReviewEnabled
			reviewerModel = initReviewerModel
			parallelEnabled = true
			workerCount = 4
		}

		// Initialize the project
		cfg, err := project.Initialize(projectName, "")
		if err != nil {
			return err
		}

		// Set execution config
		cfg.Execution = project.ExecutionConfig{
			PlannerModel:    plannerModel,
			ExecutorModel:   executorModel,
			ReviewEnabled:   reviewEnabled,
			ReviewerModel:   reviewerModel,
			MaxIterations:   10,
			Port:            8080,
			ParallelEnabled: parallelEnabled,
			WorkerCount:     workerCount,
		}

		// Save updated config
		if err := project.SaveProjectConfig(cfg); err != nil {
			return fmt.Errorf("failed to save config: %w", err)
		}

		// Display success message
		fmt.Printf("\n✓ Project initialized successfully\n")
		fmt.Printf("  Project name: %s\n", cfg.Name)
		fmt.Printf("  Project directory: %s\n", cfg.ProjectDir)
		fmt.Printf("  Tract store: %s\n", cfg.TractStore)
		fmt.Printf("  Engram memory context: %s\n", cfg.EngramStore)
		fmt.Printf("\n")
		fmt.Printf("  Execution settings:\n")
		fmt.Printf("    Planner model:  %s\n", cfg.Execution.PlannerModel)
		fmt.Printf("    Executor model: %s\n", cfg.Execution.ExecutorModel)
		if cfg.Execution.ReviewEnabled {
			fmt.Printf("    Code review: enabled\n")
			fmt.Printf("    Reviewer model: %s\n", cfg.Execution.ReviewerModel)
		} else {
			fmt.Printf("    Code review: disabled\n")
		}
		if cfg.Execution.ParallelEnabled {
			fmt.Printf("    Parallel processing: enabled (%d workers)\n", cfg.Execution.WorkerCount)
		} else {
			fmt.Printf("    Parallel processing: disabled\n")
		}
		fmt.Printf("\nNext steps:\n")
		fmt.Printf("  1. Run 'openexec wizard' to define your project requirements (Recommended)\n")
		fmt.Printf("  2. Run 'openexec plan INTENT.md' to generate stories\n")
		fmt.Printf("  3. Run 'openexec story import' to import tasks\n")
		fmt.Printf("  4. Run 'openexec start --daemon' to start execution engine\n")
		fmt.Printf("  5. Run 'openexec run' to execute tasks\n")

		return nil
	},
}

func init() {
	initCmd.Flags().StringVar(&initPlannerModel, "planner", "sonnet", "Model to use for planning phase")
	initCmd.Flags().StringVar(&initExecutorModel, "executor", "sonnet", "Model to use for task execution")
	initCmd.Flags().BoolVar(&initReviewEnabled, "review", false, "Enable code review after task execution")
	initCmd.Flags().BoolVar(&initNonInteractive, "no-review", false, "Disable code review (non-interactive)")
	initCmd.Flags().StringVar(&initReviewerModel, "reviewer", "opus", "Model to use for code review")
	initCmd.Flags().BoolVarP(&initNonInteractive, "yes", "y", false, "Non-interactive mode (use defaults)")

	rootCmd.AddCommand(initCmd)
}

// promptExecutionConfig interactively prompts for execution configuration
func promptExecutionConfig(cmd *cobra.Command) (plannerModel string, executorModel string, reviewEnabled bool, reviewerModel string, parallelEnabled bool, workerCount int) {
	reader := bufio.NewReader(os.Stdin)

	fmt.Println("\n=== Execution Settings ===")

	// Planner model selection
	plannerModel = selectModel(reader, "planner", cmd.Flags().Changed("planner"), initPlannerModel)

	// Ask if same model should be used for execution
	fmt.Println()
	fmt.Printf("Use same model '%s' for task execution? [Y/n]: ", plannerModel)
	answer, _ := reader.ReadString('\n')
	answer = strings.TrimSpace(strings.ToLower(answer))

	if answer == "n" || answer == "no" {
		executorModel = selectModel(reader, "executor", cmd.Flags().Changed("executor"), plannerModel)
	} else {
		executorModel = plannerModel
	}

	// Review configuration
	fmt.Println()
	fmt.Print("Enable code review after task execution? [Y/n]: ")
	answer, _ = reader.ReadString('\n')
	answer = strings.TrimSpace(strings.ToLower(answer))

	reviewEnabled = true
	if answer == "n" || answer == "no" {
		reviewEnabled = false
	}

	if reviewEnabled {
		reviewerModel = selectModel(reader, "reviewer", cmd.Flags().Changed("reviewer"), initReviewerModel)
	}

	// Parallel configuration
	fmt.Println()
	fmt.Print("Enable parallel task execution? [Y/n]: ")
	answer, _ = reader.ReadString('\n')
	answer = strings.TrimSpace(strings.ToLower(answer))

	parallelEnabled = true
	if answer == "n" || answer == "no" {
		parallelEnabled = false
	}

	workerCount = 4
	if parallelEnabled {
		fmt.Printf("Number of concurrent workers [%d]: ", workerCount)
		answer, _ = reader.ReadString('\n')
		answer = strings.TrimSpace(answer)
		if answer != "" {
			var count int
			if _, err := fmt.Sscanf(answer, "%d", &count); err == nil && count > 0 {
				workerCount = count
			}
		}
	}

	return plannerModel, executorModel, reviewEnabled, reviewerModel, parallelEnabled, workerCount
}

// selectModel prompts user to select a model
func selectModel(reader *bufio.Reader, modelType string, flagChanged bool, defaultModel string) string {
	// If flag was explicitly set, use that value
	if flagChanged {
		return defaultModel
	}

	fmt.Println()
	fmt.Printf("Select %s model:\n", modelType)

	// Group by provider
	currentProvider := ""
	idx := 0
	for _, m := range availableModels {
		if m.Provider != currentProvider {
			currentProvider = m.Provider
			fmt.Printf("\n  %s:\n", strings.ToUpper(currentProvider))
		}
		idx++
		defaultMark := ""
		if m.Model == defaultModel {
			defaultMark = " (default)"
		}
		fmt.Printf("    [%d] %s - %s%s\n", idx, m.Model, m.Name, defaultMark)
	}

	// Find default index
	defaultIdx := 1
	for i, m := range availableModels {
		if m.Model == defaultModel {
			defaultIdx = i + 1
			break
		}
	}

	fmt.Printf("\nEnter number [%d]: ", defaultIdx)

	choice, _ := reader.ReadString('\n')
	choice = strings.TrimSpace(choice)

	if choice == "" {
		return defaultModel
	}

	var choiceIdx int
	if _, err := fmt.Sscanf(choice, "%d", &choiceIdx); err == nil && choiceIdx >= 1 && choiceIdx <= len(availableModels) {
		return availableModels[choiceIdx-1].Model
	}

	return defaultModel
}
