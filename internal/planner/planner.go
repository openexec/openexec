package planner

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/openexec/openexec/internal/knowledge"
)

// Goal represents a project-level objective
type Goal struct {
	ID                 string `json:"id"`
	Title              string `json:"title"`
	Description        string `json:"description"`
	SuccessCriteria    string `json:"success_criteria"`
	VerificationMethod string `json:"verification_method"`
}

// Task execution modes. Mirrored by release.TaskModeAFK/TaskModeHITL.
const (
	TaskModeAFK  = "afk"  // agent can complete and verify autonomously (default)
	TaskModeHITL = "hitl" // requires a human in the loop; never auto-dispatched
)

// Task represents a technical unit of work within a story
type Task struct {
	ID                 string   `json:"id"`
	Title              string   `json:"title"`
	Description        string   `json:"description"`
	TechnicalStrategy  string   `json:"technical_strategy"`
	DependsOn          []string `json:"depends_on"`
	Mode               string   `json:"mode,omitempty"` // TaskModeAFK (default) or TaskModeHITL
	DecisionReason     string   `json:"decision_reason,omitempty"`
	DecisionRef        string   `json:"decision_ref,omitempty"`
	VerificationScript string   `json:"verification_script"`
}

// Story represents a functional requirement mapped to a goal
type Story struct {
	ID                 string   `json:"id"`
	Title              string   `json:"title"`
	Description        string   `json:"description"`
	RequirementID      string   `json:"requirement_id"`
	GoalID             string   `json:"goal_id"`
	DependsOn          []string `json:"depends_on"`
	AcceptanceCriteria []string `json:"acceptance_criteria"`
	VerificationScript string   `json:"verification_script"`
	Contract           string   `json:"contract"`
	Tasks              []Task   `json:"tasks"`
}

// UnmarshalJSON accepts requirement_id as a string, null, or a list of
// strings. Models asked to refine a plan sometimes answer the list form, and a
// single mistyped optional field must not discard every story in the plan.
func (s *Story) UnmarshalJSON(data []byte) error {
	type story Story
	var raw struct {
		story
		RequirementID json.RawMessage `json:"requirement_id"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	*s = Story(raw.story)
	s.RequirementID = ""
	if len(raw.RequirementID) == 0 || string(raw.RequirementID) == "null" {
		return nil
	}
	if err := json.Unmarshal(raw.RequirementID, &s.RequirementID); err == nil {
		return nil
	}
	var ids []string
	if err := json.Unmarshal(raw.RequirementID, &ids); err != nil {
		return fmt.Errorf("requirement_id: want a string or a list of strings: %w", err)
	}
	s.RequirementID = strings.Join(ids, ", ")
	return nil
}

// PlanSchemaVersion is the current version of the plan artifact schema.
const PlanSchemaVersion = "1.0.0"

// ProjectPlan is the complete output schema for story generation
type ProjectPlan struct {
	SchemaVersion string  `json:"schema_version"`
	Goals         []Goal  `json:"goals"`
	Stories       []Story `json:"stories"`
}

// Validate checks that the plan has required fields and is well-formed.
// Returns an error if the plan is invalid.
func (p *ProjectPlan) Validate() error {
	if p == nil {
		return fmt.Errorf("plan is nil")
	}
	if p.SchemaVersion == "" {
		p.SchemaVersion = PlanSchemaVersion
	}
	// Ensure at least one story or goal exists
	if len(p.Goals) == 0 && len(p.Stories) == 0 {
		return fmt.Errorf("plan must have at least one goal or story")
	}
	// Validate stories have required fields
	for i, story := range p.Stories {
		if story.ID == "" {
			return fmt.Errorf("story %d: missing ID", i)
		}
		if story.Title == "" {
			return fmt.Errorf("story %s: missing title", story.ID)
		}
	}
	return nil
}

// LLMProvider defines the interface for calling an AI model
type LLMProvider interface {
	Complete(ctx context.Context, prompt string) (string, error)
}

// Planner handles the conversion of intents into stories and tasks
type Planner struct {
	provider LLMProvider
}

func New(p LLMProvider) *Planner {
	return &Planner{provider: p}
}

// GeneratePlan takes intent content and optional PRD context to generate a project plan
func (p *Planner) GeneratePlan(ctx context.Context, intent string, prdContext map[string][]*knowledge.PRDRecord) (*ProjectPlan, error) {
	// 1. Prepare PRD block if available
	prdBlock := ""
	if len(prdContext) > 0 {
		data, _ := json.MarshalIndent(prdContext, "", "  ")
		prdBlock = fmt.Sprintf("STRUCTURED PRD CONTEXT (DCP):\n%s\n\nINSTRUCTION: Cross-reference the Personas and User Journeys above. Ensure generated stories specifically address the personas and follow the journey steps described.", string(data))
	}

	// 2. Format the generation prompt
	prompt := fmt.Sprintf(StoryGenerationPrompt, prdBlock, intent)

	// 3. Call LLM
	response, err := p.provider.Complete(ctx, prompt)
	if err != nil {
		return nil, fmt.Errorf("LLM completion failed: %w", err)
	}

	// 4. Parse JSON response
	return p.parseResponse(response)
}

// GenerateCompactPlan turns intent text into a single-story plan (1-3 tasks)
// using CompactStoryGenerationPrompt — no Study story, no terminus. For
// callers that have already sized the change as small; GeneratePlan remains
// the full-shape path.
func (p *Planner) GenerateCompactPlan(ctx context.Context, intent string) (*ProjectPlan, error) {
	response, err := p.provider.Complete(ctx, fmt.Sprintf(CompactStoryGenerationPrompt, intent))
	if err != nil {
		return nil, fmt.Errorf("LLM completion failed: %w", err)
	}
	return p.parseResponse(response)
}

func (p *Planner) parseResponse(response string) (*ProjectPlan, error) {
	// Extract JSON if it's wrapped in markdown blocks
	jsonText := response

	// Find the first occurrence of { or [
	start := strings.IndexAny(response, "{[")
	if start != -1 {
		char := response[start]
		var endChar string
		if char == '{' {
			endChar = "}"
		} else {
			endChar = "]"
		}

		end := strings.LastIndex(response, endChar)
		if end != -1 && end > start {
			jsonText = response[start : end+1]
		}
	}

	plan := &ProjectPlan{}
	// Try parsing as ProjectPlan object
	objectErr := json.Unmarshal([]byte(jsonText), plan)
	if objectErr == nil && len(plan.Stories) > 0 {
		return plan, nil
	}

	// Fallback: try parsing as an array of stories directly
	var stories []Story
	arrayErr := json.Unmarshal([]byte(jsonText), &stories)
	if arrayErr == nil && len(stories) > 0 {
		return &ProjectPlan{
			SchemaVersion: "1.1",
			Stories:       stories,
		}, nil
	}

	// Name the decode error of the shape the response actually has, so a
	// mistyped field is not reported as a response without stories.
	reason := "no stories found"
	if strings.HasPrefix(jsonText, "[") && arrayErr != nil {
		reason = arrayErr.Error()
	} else if strings.HasPrefix(jsonText, "{") && objectErr != nil {
		reason = objectErr.Error()
	}
	return nil, fmt.Errorf("failed to parse LLM response as JSON: %s\nResponse was: %s", reason, response)
}

// ProcessWizardMessage handles one turn of the interactive interview
func (p *Planner) ProcessWizardMessage(ctx context.Context, message string, currentState string) (*WizardResponse, error) {
	prompt := fmt.Sprintf("%s\n\nCurrent Intent State:\n%s\n\nUser Message: %s", WizardSystemPrompt, currentState, message)

	response, err := p.provider.Complete(ctx, prompt)
	if err != nil {
		return nil, err
	}

	// Extract JSON
	jsonText := response
	if start := strings.Index(response, "{"); start != -1 {
		if end := strings.LastIndex(response, "}"); end != -1 && end > start {
			jsonText = response[start : end+1]
		}
	}

	resp := &WizardResponse{}
	if err := json.Unmarshal([]byte(jsonText), resp); err != nil {
		return nil, fmt.Errorf("failed to parse wizard response: %w", err)
	}

	// Auto-complete if state is ready
	if resp.UpdatedState.IsReady() {
		resp.IsComplete = true
	}

	return resp, nil
}

// RenderIntent generates the markdown document from the state
func (p *Planner) RenderIntent(ctx context.Context, state string) (string, error) {
	var intentState IntentState
	if err := json.Unmarshal([]byte(state), &intentState); err != nil {
		return "", err
	}
	return intentState.RenderIntentMD(), nil
}
