package planner

import (
	"fmt"
	"strings"
)

// EnforceFastTrack performs deterministic post-processing on a generated plan
// to ensure it adheres to the "Surgical" and "Existing Project" rules.
func EnforceFastTrack(plan *ProjectPlan, scope string, flow string) {
	if plan == nil || len(plan.Stories) == 0 {
		return
	}

	isSurgical := strings.ToLower(scope) == "surgical"
	isExisting := strings.ToLower(flow) == "existing" || strings.ToLower(flow) == "refactor"

	// 1. Ensure Study Phase for Existing Projects
	if isExisting {
		hasStudy := false
		for _, s := range plan.Stories {
			title := strings.ToLower(s.Title)
			if strings.Contains(title, "study") || strings.Contains(title, "mapping") || strings.Contains(title, "discovery") {
				hasStudy = true
				break
			}
		}

		if !hasStudy {
			// Inject mandatory study story
			studyStory := Story{
				ID:          "US-000",
				Title:       "Codebase Study & Mapping",
				Description: "Automatically map existing contracts and dependencies.",
				GoalID:      plan.Goals[0].ID,
				Tasks: []Task{
					{
						ID:                 "T-US-000-001",
						Title:              "Analyze codebase and map APIs",
						Description:        "Extract existing schemas and patterns into knowledge base.",
						TechnicalStrategy:  "Read existing core files. Map inputs/outputs. Verify local buildability.",
						VerificationScript: "openexec knowledge show codebase",
					},
				},
			}
			// Prepend to stories
			plan.Stories = append([]Story{studyStory}, plan.Stories...)

			// Make all implementation stories depend on it
			for i := 1; i < len(plan.Stories); i++ {
				found := false
				for _, d := range plan.Stories[i].DependsOn {
					if d == "US-000" { found = true; break }
				}
				if !found {
					plan.Stories[i].DependsOn = append(plan.Stories[i].DependsOn, "US-000")
				}
			}
		}
	}

	// 2. Compact Surgical Tasks
	if isSurgical {
		for i := range plan.Stories {
			s := &plan.Stories[i]
			// Skip study stories and validation stories
			title := strings.ToLower(s.Title)
			if strings.Contains(title, "study") || strings.Contains(title, "validation") || strings.Contains(title, "terminus") {
				continue
			}

			if len(s.Tasks) > 1 {
				// Collapse into a single Chassis task
				var combinedDesc strings.Builder
				var combinedStrategy strings.Builder
				
				for _, t := range s.Tasks {
					combinedDesc.WriteString(t.Title + ": " + t.Description + "\n")
					combinedStrategy.WriteString(t.TechnicalStrategy + " ")
				}

				chassisTask := Task{
					ID:                 fmt.Sprintf("T-%s-CHS", s.ID),
					Title:              "Chassis: " + s.Title,
					Description:        combinedDesc.String(),
					TechnicalStrategy:  "FAST-TRACK: " + combinedStrategy.String() + " Complete implementation and verification in a single atomic loop.",
					VerificationScript: s.VerificationScript,
				}
				s.Tasks = []Task{chassisTask}
			}
		}
	}
}
