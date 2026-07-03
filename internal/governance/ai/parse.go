package ai

import (
	"fmt"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// fenceRe matches a fenced code block, optionally tagged (```yaml / ```json).
// It captures the block body. The (?s) flag lets . match newlines.
var fenceRe = regexp.MustCompile("(?s)```[a-zA-Z0-9]*\\s*\\n(.*?)```")

// plannerKeys / reviewerKeys are the top-level keys used to locate the
// structured object inside a prose-wrapped (unfenced) response.
var (
	plannerKeys  = []string{"summary", "kind", "risk", "affected_projects", "acceptance_criteria", "verification_plan"}
	reviewerKeys = []string{"decision", "concerns", "missing_acceptance_criteria", "missing_tests", "risk_comments"}
)

// ParsePlannerOutput tolerantly extracts a PlannerOutput from an LLM response,
// stripping markdown fences and surrounding prose before unmarshalling.
func ParsePlannerOutput(raw string) (*PlannerOutput, error) {
	body := extractStructured(raw, plannerKeys)
	if body == "" {
		return nil, fmt.Errorf("governance/ai: empty planner response")
	}
	var out PlannerOutput
	if err := yaml.Unmarshal([]byte(body), &out); err != nil {
		return nil, fmt.Errorf("governance/ai: parse planner output: %w", err)
	}
	return &out, nil
}

// ParseReviewerOutput tolerantly extracts a ReviewerOutput from an LLM response,
// stripping markdown fences and surrounding prose before unmarshalling.
func ParseReviewerOutput(raw string) (*ReviewerOutput, error) {
	body := extractStructured(raw, reviewerKeys)
	if body == "" {
		return nil, fmt.Errorf("governance/ai: empty reviewer response")
	}
	var out ReviewerOutput
	if err := yaml.Unmarshal([]byte(body), &out); err != nil {
		return nil, fmt.Errorf("governance/ai: parse reviewer output: %w", err)
	}
	return &out, nil
}

// extractStructured isolates the YAML/JSON object from an LLM response. It first
// honors a fenced code block if present; otherwise it drops leading prose lines
// up to the first recognized top-level key (or a JSON opening brace). yaml.v3
// parses JSON as a subset of YAML, so both formats round-trip through one path.
func extractStructured(raw string, keys []string) string {
	s := strings.TrimSpace(raw)
	if s == "" {
		return ""
	}
	if m := fenceRe.FindStringSubmatch(s); m != nil {
		return strings.TrimSpace(m[1])
	}
	if strings.HasPrefix(s, "{") {
		return s
	}
	lines := strings.Split(s, "\n")
	for i, ln := range lines {
		trimmed := strings.TrimSpace(ln)
		if strings.HasPrefix(trimmed, "{") {
			return strings.Join(lines[i:], "\n")
		}
		for _, k := range keys {
			if strings.HasPrefix(trimmed, k+":") {
				return strings.Join(lines[i:], "\n")
			}
		}
	}
	// No recognizable structure found; return as-is and let the unmarshal fail
	// with a useful error.
	return s
}
