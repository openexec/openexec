package planner

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func shapeBaseline() *Baseline {
	return &Baseline{Approach: "One chat with a frontier model on G3022", Result: "A usable equipment list", Bar: "A takeoff with quantities from the uploaded drawing", Verdict: VerdictBuild, Source: "INTENT.md Context"}
}

// shapedPlan is a plan that passes: study, a skeleton on the real example,
// core depth, infrastructure after the skeleton, then validation.
func shapedPlan() *ProjectPlan {
	return &ProjectPlan{
		SchemaVersion: PlanSchemaVersion,
		Baseline:      shapeBaseline(),
		Stories: []Story{
			{ID: "US-001", Title: "Codebase study", Role: RoleStudy},
			{ID: "US-002", Title: "Upload a drawing and see a result", Role: RoleSkeleton, RealExample: "fixtures/private/G3022.pdf (owner-held, not committed)", DependsOn: []string{"US-001"}},
			{ID: "US-003", Title: "Detect equipment symbols", Role: RoleCore, DependsOn: []string{"US-002"}},
			{ID: "US-004", Title: "Billing", Role: RoleInfrastructure, RoleReason: "Charge per takeoff once results are useful", DependsOn: []string{"US-003"}},
			{ID: "US-005", Title: "Goal validation", Role: RoleValidation, DependsOn: []string{"US-002", "US-003", "US-004"}},
		},
	}
}

func TestLintOutcomeShape(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*ProjectPlan)
		want   []string // substrings; empty means the plan passes
	}{
		{"shaped plan with study first passes", func(*ProjectPlan) {}, nil},
		{"infrastructure reaching the skeleton through core passes", func(p *ProjectPlan) { p.Stories[3].DependsOn = []string{"US-003"} }, nil},
		{"build_on_baseline passes", func(p *ProjectPlan) { p.Baseline.Verdict = VerdictBuildOnBaseline }, nil},
		{"stated prerequisite ahead of the skeleton passes", func(p *ProjectPlan) {
			pre := Story{ID: "US-010", Title: "Tenant isolation for the private drawing", Role: RolePrerequisite, RoleReason: "The real example is private; the skeleton must not leak it"}
			p.Stories = append([]Story{p.Stories[0], pre}, p.Stories[1:]...)
			p.Stories[2].DependsOn = []string{"US-001", "US-010"}
		}, nil},
		{"missing baseline", func(p *ProjectPlan) { p.Baseline = nil }, []string{"plan records no baseline"}},
		{"empty baseline field", func(p *ProjectPlan) { p.Baseline.Bar = "  " }, []string{"baseline bar is empty"}},
		{"unknown verdict", func(p *ProjectPlan) { p.Baseline.Verdict = "maybe" }, []string{"baseline verdict maybe is not"}},
		{"do_not_build with a build plan", func(p *ProjectPlan) { p.Baseline.Verdict = VerdictDoNotBuild }, []string{"contradicts a multi-story build plan"}},
		{"layer first", func(p *ProjectPlan) {
			p.Stories[1], p.Stories[2] = p.Stories[2], p.Stories[1]
		}, []string{"US-003: first implementation story is core, not the walking skeleton"}},
		{"no skeleton at all", func(p *ProjectPlan) { p.Stories[1].Role = RoleCore }, []string{"not the walking skeleton", "plan has no walking skeleton story"}},
		{"roles omitted", func(p *ProjectPlan) {
			for i := range p.Stories {
				p.Stories[i].Role = ""
			}
		}, []string{"US-001: story declares no role", "plan has no walking skeleton story"}},
		{"unknown role", func(p *ProjectPlan) { p.Stories[2].Role = "feature" }, []string{"US-003: unknown role feature"}},
		{"skeleton without real example", func(p *ProjectPlan) { p.Stories[1].RealExample = "" }, []string{"US-002: skeleton names no real_example input"}},
		{"real example holding the input", func(p *ProjectPlan) { p.Stories[1].RealExample = strings.Repeat("segment ", 50) }, []string{"must reference the input, not contain it"}},
		{"multi-line real example", func(p *ProjectPlan) { p.Stories[1].RealExample = "line one\nline two" }, []string{"must reference the input, not contain it"}},
		{"infrastructure ahead of the skeleton", func(p *ProjectPlan) { p.Stories[3].DependsOn = []string{"US-001"} }, []string{"US-004: infrastructure story does not depend on skeleton US-002"}},
		{"infrastructure without reason", func(p *ProjectPlan) { p.Stories[3].RoleReason = "" }, []string{"US-004: infrastructure story requires a role_reason"}},
		{"prerequisite without reason", func(p *ProjectPlan) {
			p.Stories = append([]Story{p.Stories[0], {ID: "US-010", Title: "Accounts", Role: RolePrerequisite}}, p.Stories[1:]...)
		}, []string{"US-010: prerequisite story requires a role_reason"}},
		{"skeleton waiting for core", func(p *ProjectPlan) { p.Stories[1].DependsOn = []string{"US-001", "US-003"} }, []string{"US-002: skeleton depends on US-003, which is neither a study nor a prerequisite story"}},
		// Roles come from the plan, never from words in titles.
		{"title says skeleton, role says core", func(p *ProjectPlan) {
			p.Stories[1].Role, p.Stories[1].Title = RoleCore, "Walking skeleton: upload to result"
		}, []string{"plan has no walking skeleton story"}},
		{"title says billing, role says core", func(p *ProjectPlan) {
			p.Stories[2].Title = "Billing and subscriptions"
		}, nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			plan := shapedPlan()
			c.mutate(plan)
			issues := LintOutcomeShape(plan)
			if len(c.want) == 0 {
				if len(issues) != 0 {
					t.Fatalf("sound plan refused: %v", issues)
				}
				return
			}
			joined := strings.Join(issues, "\n")
			for _, want := range c.want {
				if !strings.Contains(joined, want) {
					t.Fatalf("want %q in %v", want, issues)
				}
			}
		})
	}
}

func TestLintOutcomeShapeLeavesLegacySmallAndContinuedWorkAlone(t *testing.T) {
	unshaped := func() *ProjectPlan {
		p := shapedPlan()
		p.Baseline = nil
		for i := range p.Stories {
			p.Stories[i].Role = ""
		}
		return p
	}
	for _, version := range []string{"", "legacy", "1.0", "1.0.0", "1.1"} {
		p := unshaped()
		p.SchemaVersion = version
		if issues := LintOutcomeShape(p); len(issues) != 0 {
			t.Fatalf("plan persisted under %q judged by the new shape: %v", version, issues)
		}
	}
	// A future schema is still checked: only the known earlier ones are exempt.
	p := unshaped()
	p.SchemaVersion = "1.3"
	if len(LintOutcomeShape(p)) == 0 {
		t.Fatal("a later schema version skipped the check")
	}
	single := &ProjectPlan{SchemaVersion: PlanSchemaVersion, Stories: []Story{{ID: "US-001", Title: "Fix routing"}}}
	if issues := LintOutcomeShape(single); len(issues) != 0 {
		t.Fatalf("single-story plan refused: %v", issues)
	}
	// Study, one implementation story and validation is still one
	// implementation story.
	small := &ProjectPlan{SchemaVersion: PlanSchemaVersion, Stories: []Story{{ID: "US-001", Role: RoleStudy}, {ID: "US-002"}, {ID: "US-003", Role: RoleValidation}}}
	if issues := LintOutcomeShape(small); len(issues) != 0 {
		t.Fatalf("one-implementation-story plan refused: %v", issues)
	}
	// Persisted stories a re-plan continues carry no role; the new work does.
	replan := shapedPlan()
	replan.Stories[0] = Story{ID: "US-001", Title: "Persisted, done"}
	replan.Continues = []string{"US-001"}
	if issues := LintOutcomeShape(replan); len(issues) != 0 {
		t.Fatalf("continued story judged for a role: %v", issues)
	}
	replan.Stories[1].DependsOn = []string{"US-001"}
	if issues := LintOutcomeShape(replan); len(issues) != 0 {
		t.Fatalf("skeleton building on continued work refused: %v", issues)
	}
}

const unshapedResponse = `{%s"goals":[{"id":"G-001","title":"Takeoff"}],"stories":[` +
	`{"id":"US-001","title":"Data models","goal_id":"G-001","verification_script":"go test ./...","tasks":[{"id":"T-US-001-001","title":"Models","mode":"afk"}]},` +
	`{"id":"US-002","title":"Billing","goal_id":"G-001","verification_script":"go test ./...","tasks":[{"id":"T-US-002-001","title":"Billing","mode":"afk"}]}]}`

// The model's schema_version never decides whether the check applies: a
// multi-story plan without a baseline is refused when the model omits the
// version or writes an old one.
func TestGeneratedPlanCannotSkipOutcomeShapeByVersion(t *testing.T) {
	for name, version := range map[string]string{"omitted": "", "1.0.0": `"schema_version":"1.0.0",`, "1.1": `"schema_version":"1.1",`} {
		t.Run(name, func(t *testing.T) {
			provider := &mockProvider{response: strings.Replace(unshapedResponse, "%s", version, 1)}
			plan, err := New(provider).GeneratePlan(context.Background(), "intent", nil)
			if err != nil {
				t.Fatal(err)
			}
			if plan.SchemaVersion != PlanSchemaVersion {
				t.Fatalf("schema = %q, want stamped %q", plan.SchemaVersion, PlanSchemaVersion)
			}
			provider.response = `{"approved":true,"assessment":"Looks complete"}`
			review, err := New(provider).ReviewPlan(context.Background(), "intent", plan)
			if err != nil {
				t.Fatal(err)
			}
			if review.Approved || !strings.Contains(review.Assessment, "outcome shape lint refused") || !strings.Contains(review.Assessment, "plan records no baseline") {
				t.Fatalf("reviewer approval bypassed the outcome shape: %+v", review)
			}
			issues := strings.Join(DeterministicPlanIssues(plan), "\n")
			if !strings.Contains(issues, "outcome shape lint refused") || !strings.Contains(issues, "plan records no baseline") {
				t.Fatalf("post-fix checks miss the outcome shape: %s", issues)
			}
		})
	}
}

// Single-story full plans and compact plans are stamped too, and pass.
func TestSingleStoryAndCompactPlansAreNotRefused(t *testing.T) {
	const one = `{"schema_version":"1.0.0","goals":[{"id":"G-001","title":"Routing"}],"stories":[{"id":"US-001","title":"Routing","goal_id":"G-001","verification_script":"go test ./...","tasks":[{"id":"T-US-001-001","title":"Repair","mode":"afk"}]}]}`
	for _, compact := range []bool{false, true} {
		provider := &mockProvider{response: one}
		p := New(provider)
		var plan *ProjectPlan
		var err error
		if compact {
			plan, err = p.GenerateCompactPlan(context.Background(), "small fix")
		} else {
			plan, err = p.GeneratePlan(context.Background(), "small fix", nil)
		}
		if err != nil {
			t.Fatal(err)
		}
		provider.response = `{"approved":true,"assessment":"Small and verified"}`
		review, err := p.ReviewPlan(context.Background(), "small fix", plan)
		if err != nil || !review.Approved {
			t.Fatalf("compact=%v refused: %+v %v", compact, review, err)
		}
		if issues := DeterministicPlanIssues(plan); len(issues) != 0 {
			t.Fatalf("compact=%v post-fix checks refused: %v", compact, issues)
		}
	}
}

// A fix that returns only stories keeps the plan's baseline; a fix that
// returns the object form replaces it.
func TestRefinementKeepsOrReplacesTheBaseline(t *testing.T) {
	original := shapedPlan()
	stories, err := json.Marshal(original.Stories)
	if err != nil {
		t.Fatal(err)
	}
	provider := &mockProvider{response: string(stories)}
	rejected := &PlanReview{Assessment: "Tighten the core story"}
	refined, err := New(provider).RefinePlan(context.Background(), "intent", original, rejected)
	if err != nil {
		t.Fatal(err)
	}
	if refined.Baseline == nil || *refined.Baseline != *original.Baseline || refined.SchemaVersion != PlanSchemaVersion {
		t.Fatalf("story-only fix lost the baseline: %+v", refined.Baseline)
	}
	if !strings.Contains(provider.lastPrompt, OutcomeShapeRule) || !strings.Contains(provider.lastPrompt, `"baseline"`) {
		t.Fatal("fix prompt lacks the outcome shape or the current baseline")
	}
	provider.response = `{"baseline":{"approach":"Existing CAD tool","result":"Manual counts","bar":"Automatic counts","verdict":"build_on_baseline","source":"intent"},"stories":` + string(stories) + `}`
	refined, err = New(provider).RefinePlan(context.Background(), "intent", original, rejected)
	if err != nil {
		t.Fatal(err)
	}
	if refined.Baseline == nil || refined.Baseline.Verdict != VerdictBuildOnBaseline {
		t.Fatalf("object-form fix did not replace the baseline: %+v", refined.Baseline)
	}
}

func TestPlanningPromptsCarryOutcomeShapeRules(t *testing.T) {
	for _, rule := range []string{"BASELINE FIRST", "WALKING SKELETON FIRST", "CORE BEFORE INFRASTRUCTURE", `"real_example"`, `"role_reason"`, "never call an external or billed model"} {
		if !strings.Contains(OutcomeShapeRule, rule) {
			t.Fatalf("outcome shape rule lacks %q", rule)
		}
	}
	for name, prompt := range map[string]string{"generation": StoryGenerationPrompt, "review": StoryReviewPrompt, "refinement": StoryFixPrompt} {
		if !strings.Contains(prompt, OutcomeShapeRule) {
			t.Fatalf("%s prompt lacks the outcome shape rule", name)
		}
	}
	if !strings.Contains(StoryReviewPrompt, "**Outcome Shape**") {
		t.Fatal("review criteria do not apply the outcome shape")
	}
	// The generation example must itself pass the check it teaches.
	_, shape, _ := strings.Cut(StoryGenerationPrompt, "OUTPUT FORMAT (JSON object):")
	var example ProjectPlan
	if err := json.NewDecoder(strings.NewReader(shape)).Decode(&example); err != nil {
		t.Fatal(err)
	}
	if example.Baseline == nil || example.Stories[0].Role != RoleSkeleton || example.Stories[0].RealExample == "" {
		t.Fatalf("generation example omits the outcome shape: %+v", example)
	}
}
