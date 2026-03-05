package planner

import (
	"fmt"
	"strings"
)

// IntentState represents the structured state of the project intent
type IntentState struct {
	ProjectName      string       `json:"project_name"`
	Flow             string       `json:"flow"` // greenfield, refactor, unknown
	AppType          string       `json:"app_type"`
	Platforms        []string     `json:"platforms"`
	ProblemStatement string       `json:"problem_statement"`
	PrimaryGoals     []Goal       `json:"primary_goals"`
	SuccessMetric    string       `json:"success_metric"`
	Entities         []Entity     `json:"entities"`
	Constraints      []Constraint `json:"constraints"`
	LegacyRepoPath   string       `json:"legacy_repo_path"`
}

type Entity struct {
	Name       string `json:"name"`
	DataSource string `json:"data_source"`
}

type Constraint struct {
	ID          string `json:"id"`
	Description string `json:"description"`
}

// WizardResponse represents the AI's response during the interview
type WizardResponse struct {
	UpdatedState    IntentState `json:"updated_state"`
	NextQuestion    string      `json:"next_question"`
	Acknowledgement string      `json:"acknowledgement"`
	IsComplete      bool        `json:"is_complete"`
	NewFacts        []string    `json:"new_facts"`
	NewAssumptions  []string    `json:"new_assumptions"`
}

// IsReady checks if the minimum viable intent has been gathered
func (s *IntentState) IsReady() bool {
	if s.Flow == "" || s.Flow == "unknown" { return false }
	if s.AppType == "" || s.AppType == "unknown" { return false }
	if s.ProblemStatement == "" { return false }
	if len(s.PrimaryGoals) == 0 { return false }
	if len(s.Constraints) == 0 { return false }
	
	// Check entities
	if len(s.Entities) == 0 { return false }
	hasDataSource := false
	for _, e := range s.Entities {
		if e.DataSource != "" {
			hasDataSource = true
			break
		}
	}
	if !hasDataSource { return false }

	if (s.AppType == "desktop" || s.AppType == "mobile") && len(s.Platforms) == 0 {
		return false
	}

	if s.Flow == "refactor" && s.LegacyRepoPath == "" {
		return false
	}

	return true
}

// RenderIntentMD converts the state into a formatted INTENT.md
func (s *IntentState) RenderIntentMD() string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("# Intent: %s\n\n", s.ProjectName))
	
	sb.WriteString("## Goals\n")
	sb.WriteString(fmt.Sprintf("- %s\n", s.ProblemStatement))
	for _, g := range s.PrimaryGoals {
		sb.WriteString(fmt.Sprintf("### %s: %s\n", g.ID, g.Description))
		sb.WriteString(fmt.Sprintf("- Success Criteria: %s\n", g.SuccessCriteria))
		sb.WriteString(fmt.Sprintf("- Verification: %s\n", g.VerificationMethod))
	}
	sb.WriteString(fmt.Sprintf("- Global Success Metric: %s\n\n", s.SuccessMetric))

	sb.WriteString("## Requirements\n")
	sb.WriteString("### REQ-001: Core Architecture\n")
	sb.WriteString(fmt.Sprintf("- Shape: %s\n", s.AppType))
	sb.WriteString(fmt.Sprintf("- Platforms: %s\n\n", strings.Join(s.Platforms, ", ")))

	sb.WriteString("### REQ-002: Data Source Mapping\n")
	for _, e := range s.Entities {
		sb.WriteString(fmt.Sprintf("- %s: Source of Truth: %s\n", e.Name, e.DataSource))
	}
	sb.WriteString("\n")

	sb.WriteString("## Constraints\n")
	for _, c := range s.Constraints {
		sb.WriteString(fmt.Sprintf("- %s: %s\n", c.ID, c.Description))
	}

	return sb.String()
}
