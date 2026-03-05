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

// Task represents a technical unit of work within a story
type Task struct {
	ID                 string   `json:"id"`
	Title              string   `json:"title"`
	Description        string   `json:"description"`
	TechnicalStrategy  string   `json:"technical_strategy"`
	DependsOn          []string `json:"depends_on"`
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

// ProjectPlan is the complete output schema for story generation
type ProjectPlan struct {
	SchemaVersion string  `json:"schema_version"`
	Goals         []Goal  `json:"goals"`
	Stories       []Story `json:"stories"`
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
	if err := json.Unmarshal([]byte(jsonText), plan); err == nil && len(plan.Stories) > 0 {
		return plan, nil
	}

	// Fallback: try parsing as an array of stories directly
	var stories []Story
	if err := json.Unmarshal([]byte(jsonText), &stories); err == nil && len(stories) > 0 {
		return &ProjectPlan{
			SchemaVersion: "1.1",
			Stories:       stories,
		}, nil
	}

	return nil, fmt.Errorf("failed to parse LLM response as JSON: no stories found\nResponse was: %s", response)
}
