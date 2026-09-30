package planner

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

// Exercise the actual generation/refinement/serialization/review route which
// turned null into "" and made the reviewer request an impossible correction.
// Serialization stays unchanged because persisted plan receipts hash its bytes.
func TestEnablingRequirementSurvivesRefinementAndReview(t *testing.T) {
	const response = `{"schema_version":"1.0.0","goals":[{"id":"G","title":"Routing"}],"stories":[{"id":"S","title":"Prepare","requirement_id":null,"goal_id":"G","tasks":[{"id":"T","title":"Prepare","mode":"afk"}]}]}`
	provider := &mockProvider{response: response}
	p := New(provider)
	original, err := p.GeneratePlan(context.Background(), "Console owns candidate commits", nil)
	if err != nil {
		t.Fatal(err)
	}
	wire, err := json.Marshal(original)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(wire), `"requirement_id":""`) {
		t.Fatalf("canonical persisted representation changed: %s", wire)
	}
	var reopened ProjectPlan
	if err := json.Unmarshal(wire, &reopened); err != nil {
		t.Fatal(err)
	}
	refined, err := p.RefinePlan(context.Background(), "Console owns candidate commits", &reopened, &PlanReview{Assessment: "Use null for enabling story"})
	if err != nil {
		t.Fatal(err)
	}
	if refined.Stories[0].RequirementID != "" || len(refined.Goals) != 1 {
		t.Fatalf("refinement lost canonical absence or retained Goal: %+v", refined)
	}
	assertContractPrompt(t, provider.lastPrompt)
	provider.response = `{"approved":true,"assessment":"Fixture accepts the canonical representation"}`
	review, err := p.ReviewPlan(context.Background(), "Console owns candidate commits", refined)
	if err != nil || !review.Approved {
		t.Fatalf("review: %+v %v", review, err)
	}
	assertContractPrompt(t, provider.lastPrompt)
	if !strings.Contains(provider.lastPrompt, `"requirement_id":""`) {
		t.Fatal("review did not receive the canonical serialized plan")
	}
	after, _ := json.Marshal(refined)
	if string(after) != string(wire) {
		t.Fatal("refinement/review changed canonical plan bytes")
	}
	// Explicit independent rejection remains a rejection; the guidance does not
	// auto-approve a plan or override the reviewer.
	provider.response = `{"approved":false,"assessment":"A real implementation gap remains"}`
	review, err = p.ReviewPlan(context.Background(), "Console owns candidate commits", refined)
	if err != nil || review.Approved {
		t.Fatalf("independent rejection lost: %+v %v", review, err)
	}
}

func assertContractPrompt(t *testing.T, prompt string) {
	t.Helper()
	if !strings.Contains(prompt, `The schema normalizes null and an omitted requirement_id to ""`) {
		t.Fatal("provider prompt omits canonical empty requirement semantics")
	}
	if !strings.Contains(prompt, CandidateCommitRule) {
		t.Fatal("provider prompt omits conditional candidate commit ownership")
	}
	if strings.Contains(prompt, "It must conclude with a mandate to use 'safe_commit'") {
		t.Fatal("unconditional task commit mandate remains")
	}
}

func TestAllPlanningPathsRespectSchemaAndCommitOwnership(t *testing.T) {
	for name, prompt := range map[string]string{"generation": StoryGenerationPrompt, "compact": CompactStoryGenerationPrompt, "review": StoryReviewPrompt, "refinement": StoryFixPrompt} {
		t.Run(name, func(t *testing.T) { assertContractPrompt(t, prompt) })
	}
	// Keep standalone task commits available under their own delegated policy.
	if !strings.Contains(CandidateCommitRule, "workflow delegates task commits and safe_commit") {
		t.Fatal("standalone delegated commit policy disappeared")
	}
}
