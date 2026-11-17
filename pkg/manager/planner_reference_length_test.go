package manager

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/openexec/openexec/internal/planner"
)

// seedArtDescription is what Cardstore US-008 put in real_example: a
// description of the seed-art captures, not a reference to them.
var seedArtDescription = "Captured seed-art screenshots of the card editor at three zoom levels, " +
	strings.Repeat("each capture is a PNG of the rendered card with its bleed area and safe margin overlaid; ", 4) +
	"the captures are described here so the skeleton can run on them."

const seedArtReference = "fixtures/seed-art/captures/"

// reviewedCardstoreStories is the plan fixed after the review: a skeleton
// whose real_example is `example`, with perturbation and negative-control
// stories (reviewer-requested coverage) that no repair may touch.
func reviewedCardstoreStories(example string) []map[string]any {
	skeleton := shapeStory("US-002", "Render the seed-art captures", planner.RoleSkeleton, "", example, "US-001")
	skeleton["description"] = "Run the editor on the seed-art captures end to end."
	perturbation := shapeStory("US-003", "Perturbation: rotated and cropped captures", planner.RoleCore, "", "", "US-002")
	perturbation["acceptance_criteria"] = []string{"A capture rotated by 90 degrees renders the same card"}
	negative := shapeStory("US-004", "Negative control: blank capture is refused", planner.RoleCore, "", "", "US-002")
	negative["acceptance_criteria"] = []string{"A blank capture is rejected with a message"}
	return []map[string]any{
		shapeStory("US-001", "Tenant isolation for private captures", planner.RolePrerequisite, "The captures are private", ""),
		skeleton, perturbation, negative,
	}
}

func cardstorePlan(t *testing.T, example string) string {
	return shapeJSON(t, map[string]any{
		"baseline": map[string]any{"approach": "Open the captures by hand", "result": "A visual check", "bar": "An automatic check", "verdict": "build", "source": "intent"},
		"goals":    []map[string]any{{"id": "G-001", "title": "Cards", "description": "Card editor"}},
		"stories":  reviewedCardstoreStories(example),
	})
}

func runReferenceLengthRepair(t *testing.T, repairedExample string) (generated int, prompts []string, result *PlanResult, err error) {
	e := newSchedulerTestEnv(t)
	e.mgr.cfg.MaxReviewCycles = 2
	e.mgr.cfg.PlanGenerator = planCompletionFunc(func(_ context.Context, prompt string) (string, error) {
		generated++
		prompts = append(prompts, prompt)
		switch generated {
		case 1:
			return layeredPlan(t, ""), nil
		case 2:
			return cardstorePlan(t, seedArtDescription), nil // the fix after review
		default:
			// A repair answers with the stories only.
			example := repairedExample
			if example == seedArtDescription {
				example += strings.Repeat("!", generated) // still overlong, but a changed plan each time
			}
			stories := reviewedCardstoreStories(example)
			return shapeJSON(t, stories), nil
		}
	})
	e.mgr.cfg.PlanReviewer = planCompletionFunc(func(context.Context, string) (string, error) {
		return rejectedReplayReview, nil
	})
	req := PlanRequest{RequestID: "cardstore-us-008", Intent: layeredBrief, NoValidate: true, Review: true, AutoImport: true}
	result, err = e.mgr.Plan(context.Background(), req)
	if err == nil {
		if n := storyCount(t, e.mgr); n != 4 {
			t.Fatalf("imported %d stories, want 4", n)
		}
	} else if n := storyCount(t, e.mgr); n != 0 {
		t.Fatalf("refused plan imported %d stories", n)
	}
	return
}

// An over-long real_example in the plan fixed after review is repaired once,
// with the actual bound in the instruction, and the repaired plan keeps the
// input reference and every other story byte for byte.
func TestOverlongRealExampleIsRepairedOnceWithTheBound(t *testing.T) {
	if planner.IsReferenceLengthIssue("US-001: skeleton names no real_example input") {
		t.Fatal("a missing reference is not a reference-length issue")
	}
	generated, prompts, result, err := runReferenceLengthRepair(t, seedArtReference)
	if err != nil {
		t.Fatalf("overlong reference was not repaired: %v", err)
	}
	if generated != 3 || !result.Valid {
		t.Fatalf("want plan, fix and exactly one check repair: generated=%d valid=%v", generated, result.Valid)
	}
	repair := prompts[2]
	for _, want := range []string{"300 runes", "single line", "US-002", "change only that story's real_example", "perturbation and negative-control", "runes, but a reference is at most 300"} {
		if !strings.Contains(repair, want) {
			t.Fatalf("repair instruction lacks %q: %s", want, repair)
		}
	}
	if issues := planner.DeterministicPlanIssues(result.Plan); len(issues) != 0 {
		t.Fatalf("repaired plan still fails: %v", issues)
	}
	byID := map[string]planner.Story{}
	for _, s := range result.Plan.Stories {
		byID[s.ID] = s
	}
	if byID["US-002"].RealExample != seedArtReference {
		t.Fatalf("real input reference lost: %q", byID["US-002"].RealExample)
	}
	want := reviewedCardstoreStories(seedArtReference)
	for _, s := range want {
		wantRaw, _ := json.Marshal(s)
		var expected planner.Story
		_ = json.Unmarshal(wantRaw, &expected)
		got, _ := json.Marshal(byID[s["id"].(string)])
		exp, _ := json.Marshal(expected)
		if string(got) != string(exp) {
			t.Fatalf("story %v changed by the repair\n got %s\nwant %s", s["id"], got, exp)
		}
	}
	if len(byID["US-003"].AcceptanceCriteria) != 1 || len(byID["US-004"].AcceptanceCriteria) != 1 {
		t.Fatal("perturbation or negative-control coverage lost")
	}
}

// A repair that leaves the reference overlong stops after maxCheckRepairs,
// with the full issue text, not looping.
func TestUnrepairedOverlongRealExampleStopsAfterBoundedRepairs(t *testing.T) {
	generated, _, _, err := runReferenceLengthRepair(t, seedArtDescription)
	if err == nil || !strings.Contains(err.Error(), "still fails required checks after") || !strings.Contains(err.Error(), "a reference is at most 300 runes") {
		t.Fatalf("unrepaired reference not stopped with its issue: %v", err)
	}
	if generated != 2+maxCheckRepairs {
		t.Fatalf("generated %d, want %d", generated, 2+maxCheckRepairs)
	}
}

// A structural shape failure next to an overlong reference is still refused
// at once: only the reference length is repairable.
func TestOverlongRealExampleWithStructuralFailureIsRefused(t *testing.T) {
	e := newSchedulerTestEnv(t)
	e.mgr.cfg.MaxReviewCycles = 2
	generated := 0
	e.mgr.cfg.PlanGenerator = planCompletionFunc(func(context.Context, string) (string, error) {
		generated++
		if generated == 1 {
			return layeredPlan(t, ""), nil
		}
		var p map[string]any
		_ = json.Unmarshal([]byte(cardstorePlan(t, seedArtDescription)), &p)
		delete(p, "baseline")
		return shapeJSON(t, p), nil
	})
	e.mgr.cfg.PlanReviewer = planCompletionFunc(func(context.Context, string) (string, error) {
		return rejectedReplayReview, nil
	})
	_, err := e.mgr.Plan(context.Background(), PlanRequest{RequestID: "cardstore-structural", Intent: layeredBrief, NoValidate: true, Review: true, AutoImport: true})
	if err == nil || !strings.Contains(err.Error(), "plan records no baseline") {
		t.Fatalf("structural failure not refused: %v", err)
	}
	if generated != 2 {
		t.Fatalf("structural failure was sent to repair: %d generations", generated)
	}
}
