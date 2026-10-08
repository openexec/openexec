package manager

import (
	"context"
	"strings"
	"testing"

	"github.com/openexec/openexec/internal/release"
)

// A re-plan lists only the goals it touches. The reviewer and fixer must see
// the persisted ones, and a fix citing a goal nobody holds must not end the
// run: the SRE Goal 0ca8ccef's reviewer called the persisted G-000 undefined,
// asked for a new G-SR6 where G-009 served SR6, and import refused the fixed
// plan (run d1687257, 10-08).
func TestReplanReviewSeesPersistedGoalsAndFixCannotCiteAnInventedOne(t *testing.T) {
	e := newSchedulerTestEnv(t)
	e.mgr.cfg.MaxReviewCycles = 2
	if err := e.rel.CreateGoal(&release.Goal{ID: "G-9", Title: "The console watches itself and OpenExec (SR6)"}); err != nil {
		t.Fatal(err)
	}
	fixed := `[{"id":"US-1","title":"Edit","goal_id":"G-1","verification_script":"go test ./...","tasks":[{"id":"T-1","title":"Implement edit","description":"Implement and verify editing","mode":"afk"}]},{"id":"US-2","title":"Watch OpenExec","goal_id":"G-SR6","verification_script":"go test ./...","tasks":[{"id":"T-3","title":"Observe OpenExec","description":"Observe the peer project","mode":"afk"}]}]`
	var prompts []string
	e.mgr.cfg.PlanGenerator = planCompletionFunc(func(_ context.Context, prompt string) (string, error) {
		prompts = append(prompts, prompt)
		if len(prompts) == 1 {
			return replayPlanFixture, nil
		}
		return fixed, nil
	})
	var reviewed string
	e.mgr.cfg.PlanReviewer = planCompletionFunc(func(_ context.Context, prompt string) (string, error) {
		reviewed = prompt
		return rejectedReplayReview, nil
	})
	if _, err := e.mgr.Plan(context.Background(), replayRequest()); err != nil {
		t.Fatal("a fix citing an invented goal ended the plan:", err)
	}
	if !strings.Contains(reviewed, "G-9") || len(prompts) != 2 || !strings.Contains(prompts[1], "G-9") {
		t.Fatal("reviewer or fixer was not shown the persisted goal")
	}
	story := e.rel.GetStory("US-2")
	if story == nil || story.GoalID != "" {
		t.Fatalf("fixed story not imported unbound: %+v", story)
	}
}
