package manager

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/openexec/openexec/internal/intent"
	"github.com/openexec/openexec/internal/mcp"
	"github.com/openexec/openexec/internal/planner"
	"github.com/openexec/openexec/internal/project"
	"github.com/openexec/openexec/internal/prompt"
	"github.com/openexec/openexec/internal/release"
	"github.com/openexec/openexec/internal/runner"
)

// PlanRequest defines the input for a planning operation.
type PlanRequest struct {
	IntentFile string `json:"intent_file"`
	NoValidate bool   `json:"no_validate"`
	AutoImport bool   `json:"auto_import"` // Automatically load stories into DB
}

// PlanResult contains the generated plan and validation status.
type PlanResult struct {
	Plan          *planner.ProjectPlan `json:"plan"`
	Valid         bool                 `json:"valid"`
	Issues        []string             `json:"issues,omitempty"`
	PlanID        string               `json:"plan_id,omitempty"`
	ArtifactHash  string               `json:"artifact_hash,omitempty"`
	ArtifactPath  string               `json:"artifact_path,omitempty"`
	PromptVersion string               `json:"prompt_version,omitempty"`
}

// PlanInputError represents a validation error for plan input.
// It is returned for 400 Bad Request responses.
type PlanInputError struct {
	Message string
}

func (e *PlanInputError) Error() string {
	return e.Message
}

// Plan executes the planning workflow on the server side (V1.0 Service).
func (m *Manager) Plan(ctx context.Context, req PlanRequest) (*PlanResult, error) {
	intentFile := req.IntentFile
	if intentFile == "" {
		intentFile = "INTENT.md"
	}

	// Security: Validate intent_file is constrained to workspace and not in denylist
	absIntentPath := filepath.Join(m.cfg.WorkDir, intentFile)

	// Use mcp path validator to enforce workspace root constraint and denylist
	validator := mcp.NewPathValidator(mcp.PathValidatorConfig{
		AllowedRoots:    []string{m.cfg.WorkDir},
		AllowSymlinks:   false,
		RequireAbsolute: false,
		RequireExists:   true,
		RequireFile:     true,
	})

	validatedPath, err := validator.Validate(absIntentPath)
	if err != nil {
		// Return a typed error so the handler can return 400
		return nil, &PlanInputError{Message: fmt.Sprintf("invalid intent_file: %v", err)}
	}
	absIntentPath = validatedPath

	if _, err := os.Stat(absIntentPath); os.IsNotExist(err) {
		return nil, &PlanInputError{Message: fmt.Sprintf("intent file not found: %s", intentFile)}
	}

	// 1. Validation
	if !req.NoValidate {
		validator := intent.NewValidator(absIntentPath)
		res, err := validator.Validate()
		if err != nil {
			return nil, fmt.Errorf("validation error: %w", err)
		}
		if !res.Valid {
			issues := []string{}
			for _, i := range res.Critical {
				issues = append(issues, i.Message)
			}
			return &PlanResult{Valid: false, Issues: issues}, nil
		}
	}

	// 2. Load Config
	projCfg, err := project.LoadProjectConfig(m.cfg.WorkDir)
	if err != nil {
		projCfg = &project.ProjectConfig{
			Execution: project.ExecutionConfig{PlannerModel: "sonnet"},
		}
	}

	plannerModel := projCfg.Execution.PlannerModel
	if plannerModel == "" {
		plannerModel = m.cfg.ExecutorModel
	}

	// 3. Run Planner
	intentContent, err := os.ReadFile(absIntentPath)
	if err != nil {
		return nil, err
	}

	p := planner.New(&cliLLMProvider{model: plannerModel})
	plan, err := p.GeneratePlan(ctx, string(intentContent), nil)
	if err != nil {
		return nil, fmt.Errorf("planner failed: %w", err)
	}

	// Write plan artifact to .openexec/artifacts/plans/<hash>.json
	planID, artifactHash, artifactPath := m.writePlanArtifact(plan)

	// Auto-import: persist stories/tasks to DB so runs:execute can find them
	if req.AutoImport {
		if err := m.importPlan(plan); err != nil {
			return nil, fmt.Errorf("failed to import plan to database: %w", err)
		}
	}

	return &PlanResult{
		Plan:          plan,
		Valid:         true,
		PlanID:        planID,
		ArtifactHash:  artifactHash,
		ArtifactPath:  artifactPath,
		PromptVersion: prompt.PromptVersion,
	}, nil
}

// writePlanArtifact persists a plan to the artifacts directory and returns its ID and hash.
func (m *Manager) writePlanArtifact(plan *planner.ProjectPlan) (planID, artifactHash, artifactPath string) {
	// Generate plan ID from timestamp
	planID = fmt.Sprintf("PLAN-%s", time.Now().UTC().Format("20060102-150405"))

	// Validate plan before writing to avoid malformed artifacts
	if err := plan.Validate(); err != nil {
		return planID, "", ""
	}

	// Serialize plan to JSON
	data, err := json.MarshalIndent(plan, "", "  ")
	if err != nil {
		return planID, "", ""
	}

	// Compute content hash
	hash := sha256.Sum256(data)
	artifactHash = hex.EncodeToString(hash[:])

	// Write to artifacts directory
	dir := filepath.Join(m.cfg.WorkDir, ".openexec", "artifacts", "plans")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return planID, artifactHash, ""
	}

	artifactPath = filepath.Join(dir, artifactHash+".json")
	if err := os.WriteFile(artifactPath, data, 0644); err != nil {
		return planID, artifactHash, ""
	}

	return planID, artifactHash, artifactPath
}

func (m *Manager) importPlan(plan *planner.ProjectPlan) error {
	rel, err := m.getInternalReleaseManager()
	if err != nil {
		return err
	}

	now := time.Now()
	var importedGoals, importedStories, importedTasks int

	// 1. Create goals first (stories reference them via FK)
	importedGoalIDs := make(map[string]bool)
	for _, g := range plan.Goals {
		if rel.GetGoal(g.ID) == nil {
			goal := &release.Goal{
				ID:                 g.ID,
				Title:              g.Title,
				Description:        g.Description,
				SuccessCriteria:    g.SuccessCriteria,
				VerificationMethod: g.VerificationMethod,
			}
			if err := rel.CreateGoal(goal); err != nil {
				return fmt.Errorf("import goal %s: %w", g.ID, err)
			}
			importedGoals++
		}
		importedGoalIDs[g.ID] = true
	}

	// 2. Create stories and tasks
	for i, s := range plan.Stories {
		if rel.GetStory(s.ID) == nil {
			taskIDs := make([]string, len(s.Tasks))
			for j, t := range s.Tasks {
				taskIDs[j] = t.ID
			}

			// Validate GoalID exists
			goalID := s.GoalID
			if goalID != "" && !importedGoalIDs[goalID] && rel.GetGoal(goalID) == nil {
				log.Printf("[Planner] Warning: Story %s references unknown Goal %s. Clearing GoalID.", s.ID, goalID)
				goalID = ""
			}

			st := &release.Story{
				ID:                 s.ID,
				GoalID:             goalID,
				Title:              s.Title,
				Description:        s.Description,
				AcceptanceCriteria: s.AcceptanceCriteria,
				VerificationScript: s.VerificationScript,
				Tasks:              taskIDs,
				DependsOn:          s.DependsOn,
				StoryType:          release.StoryTypeFeature,
				Priority:           i,
				Status:             release.StoryStatusPending,
				CreatedAt:          now,
				}
				if err := rel.CreateStory(st); err != nil {
				return fmt.Errorf("import story %s: %w", s.ID, err)
				}
				importedStories++
				}

				for j, t := range s.Tasks {
				if rel.GetTask(t.ID) == nil {
				task := &release.Task{
					ID:                 t.ID,
					Title:              t.Title,
					Description:        t.Description,
					VerificationScript: t.VerificationScript,
					StoryID:            s.ID,
					DependsOn:          t.DependsOn,
					Priority:           j,
					MaxAttempts:        3,
					Status:             release.TaskStatusPending,
					CreatedAt:          now,
				}

				if err := rel.CreateTask(task); err != nil {
					return fmt.Errorf("import task %s: %w", t.ID, err)
				}
				importedTasks++
			}
		}
	}

	log.Printf("[Planner] Imported %d goals, %d stories, %d tasks into database", importedGoals, importedStories, importedTasks)
	return nil
}

// getLLMProvider returns a provider for the given model.
func (m *Manager) getLLMProvider(model string) planner.LLMProvider {
	return &cliLLMProvider{model: model}
}

type cliLLMProvider struct {
	model string
}

func (p *cliLLMProvider) Complete(ctx context.Context, prompt string) (string, error) {
	cliCmd, cmdArgs, err := runner.Resolve(
		p.model,
		os.Getenv("OPENEXEC_PLANNER_CLI"),
		strings.Fields(os.Getenv("OPENEXEC_PLANNER_ARGS")),
	)
	if err != nil {
		return "", err
	}

	if strings.Contains(strings.ToLower(cliCmd), "claude") {
		cmdArgs = []string{"--print"}
	}

	c := exec.CommandContext(ctx, cliCmd, cmdArgs...)
	c.Stdin = strings.NewReader(prompt)

	output, err := c.CombinedOutput()
	if err != nil {
		outStr := string(output)
		if strings.Contains(outStr, "authentication_error") || strings.Contains(outStr, "OAuth token has expired") {
			return "", fmt.Errorf("\n❌ AI Provider Authentication Failed. Please run: %s login", cliCmd)
		}
		return "", fmt.Errorf("native LLM provider failed: %w\nOutput: %s", err, outStr)
	}

	return string(output), nil
}
