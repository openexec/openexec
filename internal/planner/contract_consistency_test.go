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
		t.Run(name, func(t *testing.T) {
			assertContractPrompt(t, prompt)
			if name != "review" {
				assertRequirementOutput(t, name, prompt)
			}
		})
	}
	// Keep standalone task commits available under their own delegated policy.
	if !strings.Contains(CandidateCommitRule, "workflow delegates task commits and safe_commit") {
		t.Fatal("standalone delegated commit policy disappeared")
	}
}

// Decode only the output example, excluding rules, intent and current stories.
func assertRequirementOutput(t *testing.T, name, prompt string) {
	t.Helper()
	markers := map[string]string{"generation": "OUTPUT FORMAT (JSON object):", "compact": "in this exact shape:", "refinement": "OUTPUT FORMAT - JSON array:"}
	_, shape, ok := strings.Cut(prompt, markers[name])
	if !ok {
		t.Fatal("missing output format section")
	}
	var stories []struct {
		RequirementID string `json:"requirement_id"`
	}
	decoder := json.NewDecoder(strings.NewReader(shape))
	if name == "refinement" {
		if err := decoder.Decode(&stories); err != nil {
			t.Fatal(err)
		}
	} else {
		var plan struct {
			Stories []struct {
				RequirementID string `json:"requirement_id"`
			} `json:"stories"`
		}
		if err := decoder.Decode(&plan); err != nil {
			t.Fatal(err)
		}
		stories = plan.Stories
	}
	if len(stories) != 1 || stories[0].RequirementID != "REQ-001" {
		t.Fatal("output format omits scalar requirement_id")
	}
}

func TestCompactRequirementIdentityThroughReviewAndRefinement(t *testing.T) {
	ctx := context.Background()
	const intent = "REQ-001: repair the small routing change"
	const response = `{"schema_version":"1.0.0","goals":[{"id":"G-001","title":"Routing"}],"stories":[{"id":"US-001","title":"Routing","requirement_id":"REQ-001","goal_id":"G-001","tasks":[{"id":"T-US-001-001","title":"Repair","mode":"afk"}]}]}`
	generator := &mockProvider{response: response}
	reviewer := &mockProvider{response: `{"approved":false,"assessment":"Add the routing assertion"}`}
	p, r := New(generator), New(reviewer)
	check := func(stage string, plan *ProjectPlan) {
		t.Helper()
		// Round-trip the actual decoded result after every transition.
		wire, err := json.Marshal(plan)
		if err != nil {
			t.Fatal(err)
		}
		var reopened ProjectPlan
		if err := json.Unmarshal(wire, &reopened); err != nil {
			t.Fatal(err)
		}
		if len(reopened.Stories) != 1 || reopened.Stories[0].RequirementID != "REQ-001" || reopened.Stories[0].GoalID != "G-001" {
			t.Fatalf("%s lost accepted identity: %s", stage, wire)
		}
	}
	plan, err := p.GenerateCompactPlan(ctx, intent)
	if err != nil {
		t.Fatal(err)
	}
	assertContractPrompt(t, generator.lastPrompt)
	assertRequirementOutput(t, "compact", generator.lastPrompt)
	if !strings.Contains(generator.lastPrompt, "EXACTLY ONE story") || !strings.Contains(generator.lastPrompt, intent) {
		t.Fatal("actual compact route not selected")
	}
	check("generation", plan)
	rejected, err := r.ReviewPlan(ctx, intent, plan)
	if err != nil || rejected.Approved {
		t.Fatalf("independent rejection: %+v %v", rejected, err)
	}
	assertContractPrompt(t, reviewer.lastPrompt)
	if !strings.Contains(reviewer.lastPrompt, `"requirement_id":"REQ-001"`) {
		t.Fatal("rejection prompt lost identity")
	}
	check("rejection", plan)
	generator.response = `[{"id":"US-001","title":"Routing verified","requirement_id":"REQ-001","tasks":[{"id":"T-US-001-001","title":"Repair and assert","mode":"afk"}]}]`
	refined, err := p.RefinePlan(ctx, intent, plan, rejected)
	if err != nil {
		t.Fatal(err)
	}
	assertContractPrompt(t, generator.lastPrompt)
	assertRequirementOutput(t, "refinement", generator.lastPrompt)
	if !strings.Contains(generator.lastPrompt, rejected.Assessment) || !strings.Contains(generator.lastPrompt, `"requirement_id":"REQ-001"`) {
		t.Fatal("refinement prompt lost findings or identity")
	}
	check("refinement", refined)
	reviewer.response = `{"approved":true,"assessment":"Routing assertion now present"}`
	approved, err := r.ReviewPlan(ctx, intent, refined)
	if err != nil || !approved.Approved {
		t.Fatalf("fresh approval: %+v %v", approved, err)
	}
	assertContractPrompt(t, reviewer.lastPrompt)
	if !strings.Contains(reviewer.lastPrompt, "Routing verified") || !strings.Contains(reviewer.lastPrompt, `"requirement_id":"REQ-001"`) {
		t.Fatal("approval did not receive refined identity")
	}
	check("approval", refined)
}
