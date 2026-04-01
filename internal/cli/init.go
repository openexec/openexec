package cli

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/openexec/openexec/internal/git"
	"github.com/openexec/openexec/internal/project"
	"github.com/spf13/cobra"
)

var (
	initPlannerModel   string
	initExecutorModel  string
	initReviewEnabled  bool
	initNoReview       bool
	initReviewerModel  string
	initNonInteractive bool
	initParallel       bool
	initWorkerCount    int
	initForce          bool
)

// Available models by provider
var availableModels = []struct {
	Provider string
	Model    string
	Name     string
}{
	// Claude (Anthropic)
	{"claude", "sonnet", "Claude 4.6 Sonnet"},
	{"claude", "opus", "Claude 4.6 Opus"},
	{"claude", "haiku", "Claude 4.6 Haiku"},
	// Codex (OpenAI)
	{"codex", "gpt-5.3-codex", "GPT-5.3 Codex"},
	{"codex", "gpt-5.3-codex-spark", "GPT-5.3 Codex Spark"},
	{"codex", "gpt-5.3", "GPT-5.3"},
	// Gemini (Google)
	{"gemini", "gemini-3.1-pro-preview", "Gemini 3.1 Pro"},
	{"gemini", "gemini-3.1-flash-preview", "Gemini 3.1 Flash"},
}

var initCmd = &cobra.Command{
	Use:   "init [project-name]",
	Short: "Initialize a new OpenExec project",
	Long: `Initialize a new OpenExec project in the current directory.

This command sets up the project infrastructure including:
- Local config in .openexec/config.json
- Tract state engine for tracking project state
- Engram memory context for storing execution context
- Execution settings (executor model, reviewer model, etc.)

The project name defaults to the current directory name if not provided.`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		var projectName string
		cwd, _ := os.Getwd()
		defaultProjectName := filepath.Base(cwd)

		if len(args) > 0 {
			projectName = args[0]
		}

		// Check if project is already initialized
		projectDir := "."
		absProjectDir, _ := filepath.Abs(projectDir)

		// Enforce git repository presence
		gitClient := git.NewClient(git.Config{Enabled: true, RepoPath: absProjectDir})
		if !gitClient.IsRepo() {
			return fmt.Errorf("this directory is not a git repository; initialize git first (git init) or run in a repo")
		}

		// Check for existing config
		config, err := project.LoadProjectConfig(absProjectDir)
		if err == nil && config != nil && !initForce {
			return fmt.Errorf("project already initialized: %s (use --force to re-initialize)", config.Name)
		}

		// Interactive mode if not explicitly set via flags
		var plannerModel, executorModel, reviewerModel string
		var reviewEnabled, parallelEnabled, gitCommitEnabled bool
		var qualityGates, cacheEnabled, memoryEnabled, checkpointEnabled, bitnetRouting bool
		var workerCount int
		var apiProvider, apiBaseURL, apiKey, apiModel string

		if !initNonInteractive {
			// 1. Project Name prompt
			if projectName == "" {
				reader := bufio.NewReader(cmd.InOrStdin())
				fmt.Printf("Enter project name [%s]: ", defaultProjectName)
				input, _ := reader.ReadString('\n')
				projectName = strings.TrimSpace(input)
				if projectName == "" {
					projectName = defaultProjectName
				}
			}

			// 2. Execution mode (CLI vs API)
			apiProvider, apiBaseURL, apiKey, apiModel = promptAPIConfig(cmd)

			// 3. Execution Config prompt
			plannerModel, executorModel, reviewEnabled, reviewerModel, parallelEnabled, workerCount, gitCommitEnabled, _, qualityGates, cacheEnabled, memoryEnabled, checkpointEnabled, bitnetRouting = promptExecutionConfig(cmd)
		} else {
			if projectName == "" {
				projectName = defaultProjectName
			}
			plannerModel = initPlannerModel
			executorModel = initExecutorModel
			reviewEnabled = true
			if cmd.Flags().Changed("review") {
				reviewEnabled = initReviewEnabled
			}
			if initNoReview {
				reviewEnabled = false
			}
			reviewerModel = initReviewerModel

			// Respect flag if provided in non-interactive mode
			parallelEnabled = initParallel
			workerCount = initWorkerCount
			if !parallelEnabled {
				workerCount = 1
			}

			gitCommitEnabled = false

			// Non-interactive defaults for advanced features
			qualityGates = true
			cacheEnabled = true
			memoryEnabled = false
			checkpointEnabled = false
			bitnetRouting = false
		}

		// Initialize the project structure
		cfg, err := project.Initialize(projectName, absProjectDir)
		if err != nil {
			return err
		}

		// Set execution config
		cfg.GitCommitEnabled = gitCommitEnabled
		cfg.Execution = project.ExecutionConfig{
			PlannerModel:      plannerModel,
			ExecutorModel:     executorModel,
			ReviewEnabled:     reviewEnabled,
			ReviewerModel:     reviewerModel,
			Port:              8080,
			WorkerCount:       workerCount,
			TimeoutSeconds:    600,
			ExecMode:          "danger-full-access",
			QualityGatesV2:    qualityGates,
			CacheEnabled:      cacheEnabled,
			MemoryEnabled:     memoryEnabled,
			CheckpointEnabled: checkpointEnabled,
			BitNetRouting:     bitnetRouting,
			APIProvider:       apiProvider,
			APIBaseURL:        apiBaseURL,
			APIKey:            apiKey,
			APIModel:          apiModel,
		}

		// Save updated config
		if err := project.SaveProjectConfig(cfg); err != nil {
			return fmt.Errorf("failed to save config: %w", err)
		}

		// Ensure .gitignore exists with .openexec entries
		ensureGitignore(absProjectDir)

		// Display success message
		cmd.Printf("\n✓ Project initialized successfully\n")
		cmd.Printf("  Project name: %s\n", cfg.Name)
		cmd.Printf("  Local configuration: %s/.openexec/config.json\n", cfg.ProjectDir)
		cmd.Printf("\nNext steps:\n")
		cmd.Printf("  1. Run 'openexec wizard' to define your project requirements\n")
		cmd.Printf("  2. Run 'openexec plan INTENT.md' to generate stories\n")
		cmd.Printf("  3. Run 'openexec run' to execute tasks\n")

		return nil
	},
}

func init() {
	initCmd.Flags().StringVar(&initPlannerModel, "planner", "sonnet", "Model to use for planning phase")
	initCmd.Flags().StringVar(&initExecutorModel, "executor", "sonnet", "Model to use for task execution")
	initCmd.Flags().BoolVar(&initReviewEnabled, "review", false, "Enable code review after task execution")
	initCmd.Flags().BoolVar(&initNoReview, "no-review", false, "Disable code review after task execution")
	initCmd.Flags().StringVar(&initReviewerModel, "reviewer", "opus", "Model to use for code review")
	initCmd.Flags().BoolVarP(&initNonInteractive, "yes", "y", false, "Non-interactive mode (use defaults)")
	initCmd.Flags().BoolVar(&initParallel, "parallel", true, "Enable parallel task execution (non-interactive)")
	initCmd.Flags().IntVar(&initWorkerCount, "worker-count", 4, "Number of concurrent workers (non-interactive)")
	initCmd.Flags().BoolVar(&initForce, "force", false, "Force re-initialization of an existing project")

	rootCmd.AddCommand(initCmd)
}

// promptExecutionConfig interactively prompts for execution configuration
func promptExecutionConfig(cmd *cobra.Command) (plannerModel string, executorModel string, reviewEnabled bool, reviewerModel string, parallelEnabled bool, workerCount int, gitCommitEnabled bool, gitPushEnabled bool, qualityGates bool, cacheEnabled bool, memoryEnabled bool, checkpointEnabled bool, bitnetRouting bool) {
	reader := bufio.NewReader(cmd.InOrStdin())

	fmt.Println("\n=== Execution Settings ===")

	// Planner model selection
	plannerModel = selectModelInteractively(reader, "planner", initPlannerModel)

	// Ask if same model should be used for execution
	fmt.Printf("\nUse same model '%s' for task execution? [Y/n]: ", plannerModel)
	answer, _ := reader.ReadString('\n')
	answer = strings.TrimSpace(strings.ToLower(answer))

	if answer == "n" || answer == "no" {
		executorModel = selectModelInteractively(reader, "executor", plannerModel)
	} else {
		executorModel = plannerModel
	}

	// Review configuration
	fmt.Printf("\nEnable code review after task execution? [Y/n]: ")
	answer, _ = reader.ReadString('\n')
	answer = strings.TrimSpace(strings.ToLower(answer))

	reviewEnabled = true
	if answer == "n" || answer == "no" {
		reviewEnabled = false
	}

	if reviewEnabled {
		reviewerModel = selectModelInteractively(reader, "reviewer", initReviewerModel)
	}

	// Git configuration
	fmt.Printf("\nEnable autonomous local commits? [y/N]: ")
	answer, _ = reader.ReadString('\n')
	answer = strings.TrimSpace(strings.ToLower(answer))
	gitCommitEnabled = false
	if answer == "y" || answer == "yes" {
		gitCommitEnabled = true
	}

	gitPushEnabled = false
	if gitCommitEnabled {
		fmt.Printf("Enable autonomous remote push on release completion? [y/N]: ")
		answer, _ = reader.ReadString('\n')
		answer = strings.TrimSpace(strings.ToLower(answer))
		if answer == "y" || answer == "yes" {
			gitPushEnabled = true
		}
	}

	// Parallel configuration
	fmt.Printf("\nEnable parallel task execution? [Y/n]: ")
	answer, _ = reader.ReadString('\n')
	answer = strings.TrimSpace(strings.ToLower(answer))

	parallelEnabled = true
	if answer == "n" || answer == "no" {
		parallelEnabled = false
		workerCount = 1
	}

	if parallelEnabled {
		workerCount = 4
		fmt.Printf("Number of concurrent workers [%d]: ", workerCount)
		line, _ := reader.ReadString('\n')
		line = strings.TrimSpace(line)
		if line != "" {
			fmt.Sscanf(line, "%d", &workerCount)
		}
	}

	// Advanced features
	fmt.Printf("\n=== Advanced Features (all optional) ===\n")

	fmt.Printf("Enable quality gates (auto lint/test after stages)? [Y/n]: ")
	answer, _ = reader.ReadString('\n')
	answer = strings.TrimSpace(strings.ToLower(answer))
	qualityGates = answer != "n" && answer != "no"

	fmt.Printf("Enable caching (avoid redundant work on re-runs)? [Y/n]: ")
	answer, _ = reader.ReadString('\n')
	answer = strings.TrimSpace(strings.ToLower(answer))
	cacheEnabled = answer != "n" && answer != "no"

	fmt.Printf("Enable memory (learn patterns across sessions)? [y/N]: ")
	answer, _ = reader.ReadString('\n')
	answer = strings.TrimSpace(strings.ToLower(answer))
	memoryEnabled = answer == "y" || answer == "yes"

	fmt.Printf("Enable checkpointing (crash recovery)? [y/N]: ")
	answer, _ = reader.ReadString('\n')
	answer = strings.TrimSpace(strings.ToLower(answer))
	checkpointEnabled = answer == "y" || answer == "yes"

	fmt.Println("\n=== Local LLM for Intelligent Routing ===")
	fmt.Println("OpenExec can use a local 1-bit LLM (BitNet) for intelligent routing")
	fmt.Println("and tool selection. This provides smarter skill matching but requires")
	fmt.Println("downloading a ~400MB model file.")
	fmt.Println()
	fmt.Println("Without this, OpenExec uses fast rule-based routing (deterministic).")
	fmt.Println()
	fmt.Printf("Install and use local LLM for intelligent routing? [y/N]: ")
	answer, _ = reader.ReadString('\n')
	answer = strings.TrimSpace(strings.ToLower(answer))
	bitnetRouting = answer == "y" || answer == "yes"

	if bitnetRouting {
		fmt.Println("\n✓ Local LLM enabled - model will download on first use (~400MB)")
	} else {
		fmt.Println("\n✓ Rule-based routing enabled (fast, no download required)")
	}

	return plannerModel, executorModel, reviewEnabled, reviewerModel, parallelEnabled, workerCount, gitCommitEnabled, gitPushEnabled, qualityGates, cacheEnabled, memoryEnabled, checkpointEnabled, bitnetRouting
}

// promptAPIConfig interactively prompts for API provider configuration.
// Returns empty strings if user selects CLI mode.
func promptAPIConfig(cmd *cobra.Command) (apiProvider, apiBaseURL, apiKey, apiModel string) {
	reader := bufio.NewReader(cmd.InOrStdin())

	fmt.Println("\n=== Execution Mode ===")
	fmt.Println("  [1] CLI tool (claude/codex/gemini) - default")
	fmt.Println("  [2] API provider (OpenAI-compatible)")
	fmt.Printf("\nSelect execution mode [1]: ")
	answer, _ := reader.ReadString('\n')
	answer = strings.TrimSpace(answer)

	if answer != "2" {
		return "", "", "", ""
	}

	apiProvider = "openai_compat"

	fmt.Printf("API Base URL (e.g. https://api.openai.com/v1): ")
	apiBaseURL, _ = reader.ReadString('\n')
	apiBaseURL = strings.TrimSpace(apiBaseURL)

	fmt.Printf("API Key (or $ENV_VAR): ")
	apiKey, _ = reader.ReadString('\n')
	apiKey = strings.TrimSpace(apiKey)

	fmt.Printf("Model name (e.g. gpt-4o, moonshot-v1-128k): ")
	apiModel, _ = reader.ReadString('\n')
	apiModel = strings.TrimSpace(apiModel)

	return apiProvider, apiBaseURL, apiKey, apiModel
}

// ensureGitignore ensures .gitignore exists and contains .openexec entries.
// If .gitignore doesn't exist, creates one with common defaults.
func ensureGitignore(projectDir string) {
	gitignorePath := filepath.Join(projectDir, ".gitignore")

	const openexecMarker = "# OpenExec Managed Block"
	const openexecBlock = "\n" + openexecMarker + "\n.openexec/logs/\n.openexec/data/\n.openexec/engram/cache/\n"

	existing, err := os.ReadFile(gitignorePath)
	if err == nil {
		// File exists — append openexec entries if missing marker
		if !strings.Contains(string(existing), openexecMarker) {
			f, err := os.OpenFile(gitignorePath, os.O_APPEND|os.O_WRONLY, 0644)
			if err == nil {
				_, _ = f.WriteString(openexecBlock)
				_ = f.Close()
			}
		}
		return
	}

	// No .gitignore — create with OpenExec entries only.
	// The agent will add stack-specific patterns during the first task.
	_ = os.WriteFile(gitignorePath, []byte("# OpenExec\n.openexec/logs/\n.openexec/data/\n"), 0644)
}

// selectModelInteractively prompts user to select a model
func selectModelInteractively(reader *bufio.Reader, modelType string, defaultModel string) string {
	fmt.Printf("\nSelect %s model:\n", modelType)

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

	fmt.Printf("\nEnter number or model name [%d]: ", defaultIdx)

	choice, _ := reader.ReadString('\n')
	choice = strings.TrimSpace(choice)

	if choice == "" {
		return defaultModel
	}

	// 1. Try numeric selection
	var choiceIdx int
	if _, err := fmt.Sscanf(choice, "%d", &choiceIdx); err == nil && choiceIdx >= 1 && choiceIdx <= len(availableModels) {
		return availableModels[choiceIdx-1].Model
	}

	// 2. Try short-name selection (case-insensitive)
	lowerChoice := strings.ToLower(choice)
	for _, m := range availableModels {
		if strings.ToLower(m.Model) == lowerChoice {
			return m.Model
		}
	}

	return defaultModel
}
