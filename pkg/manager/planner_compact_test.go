package manager

import (
	"context"
	"encoding/json"
	"github.com/openexec/openexec/internal/planner"
	"github.com/openexec/openexec/pkg/db/state"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/openexec/openexec/pkg/runtime"
)

func TestCompactReviewedPlanUsesExistingPlannerAndReplay(t *testing.T) {
	e := newSchedulerTestEnv(t)
	generated, reviewed := 0, 0
	e.mgr.cfg.PlanGenerator = planCompletionFunc(func(_ context.Context, prompt string) (string, error) {
		generated++
		if !strings.Contains(prompt, "SMALL, well-scoped change") {
			t.Fatal("compact request used full planning")
		}
		return replayPlanFixture, nil
	})
	e.mgr.cfg.PlanReviewer = planCompletionFunc(func(_ context.Context, prompt string) (string, error) {
		reviewed++
		if !strings.Contains(prompt, "Implement edit") {
			t.Fatal("review lost native plan")
		}
		return replayReviewFixture, nil
	})
	req := replayRequest()
	req.Compact = true
	result, err := e.mgr.Plan(context.Background(), req)
	if err != nil || !result.Valid || generated != 1 || reviewed != 1 {
		t.Fatalf("compact review/import: %+v %v %d/%d", result, err, generated, reviewed)
	}
	fresh := &Manager{cfg: e.mgr.cfg, state: e.mgr.state}
	if _, err := fresh.Plan(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	if generated != 1 || reviewed != 1 {
		t.Fatal("fresh manager repeated completed planning")
	}
	req.Compact = false
	if _, err := fresh.Plan(context.Background(), req); err == nil || !strings.Contains(err.Error(), "conflict") {
		t.Fatalf("planning mode change reused another request: %v", err)
	}
	var tasks int
	if err := e.mgr.state.GetDB().QueryRow(`SELECT COUNT(*) FROM tasks`).Scan(&tasks); err != nil || tasks != 2 {
		t.Fatalf("native task import duplicated or absent: %d %v", tasks, err)
	}
}

func TestCompactPlanExecutesNativeTaskWithoutGoalController(t *testing.T) {
	e := newSchedulerTestEnv(t)
	path := filepath.Join(e.dir, "label.txt")
	if err := os.WriteFile(path, []byte("Old label"), 0600); err != nil {
		t.Fatal(err)
	}
	plans, reviews, implementations := 0, 0, 0
	e.mgr.cfg.PlanGenerator = planCompletionFunc(func(_ context.Context, prompt string) (string, error) {
		plans++
		if !strings.Contains(prompt, "SMALL, well-scoped change") || !strings.Contains(prompt, "Old label") {
			t.Fatal("missing compact brownfield context")
		}
		return `{"goals":[{"id":"G","title":"Rename label"}],"stories":[{"id":"S","goal_id":"G","title":"Rename label","tasks":[{"id":"T","title":"Rename and verify","description":"Change the existing label","verification_script":"test -f label.txt"}]}]}`, nil
	})
	e.mgr.cfg.PlanReviewer = planCompletionFunc(func(context.Context, string) (string, error) { reviews++; return replayReviewFixture, nil })
	e.mgr.cfg.StageExecutor = admittedFixture(func(_ context.Context, s *runtime.Stage, _ *runtime.StageInput) (*runtime.StageResult, error) {
		if s.Name == "implement" {
			implementations++
			if err := os.WriteFile(path, []byte("New label"), 0600); err != nil {
				return nil, err
			}
		}
		if s.Name == "test" {
			raw, err := os.ReadFile(path)
			if err != nil || string(raw) != "New label" {
				t.Fatal("persisted label not verified", err)
			}
		}
		return &runtime.StageResult{StageName: s.Name, Status: runtime.StageStatusCompleted, Attempt: 1}, nil
	})
	req := replayRequest()
	req.RequestID = "small-label-change"
	req.Intent = "Existing label.txt contains Old label; change it to New label and verify persistence."
	req.Compact = true
	plan, err := e.mgr.Plan(context.Background(), req)
	if err != nil || !plan.Valid {
		t.Fatal("compact plan refused", err)
	}
	if err := e.mgr.ExecuteTasks(context.Background(), RunOptions{TaskOriented: true, StoryIDs: []string{"S"}, MaxParallel: 1}); err != nil {
		t.Fatal(err)
	}
	task, err := e.rel.TaskSnapshot(context.Background(), "T")
	if err != nil || task.Status != "done" || implementations != 1 || plans != 1 || reviews != 1 {
		t.Fatalf("native compact completion failed: %+v %v %d/%d/%d", task, err, implementations, plans, reviews)
	}
	raw, err := os.ReadFile(path)
	if err != nil || string(raw) != "New label" {
		t.Fatal("product result absent", err)
	}
	if err := e.mgr.ExecuteTasks(context.Background(), RunOptions{TaskOriented: true, StoryIDs: []string{"S"}, MaxParallel: 1}); err != nil {
		t.Fatal(err)
	}
	if implementations != 1 {
		t.Fatal("completed small work repeated")
	}
}

func TestCompactRequirementIdentityPersistsAcrossManagerReopen(t *testing.T) {
	e := newSchedulerTestEnv(t)
	req := replayRequest()
	req.Compact = true
	req.Intent = "REQ-001: implement edit and verify reload"
	generated, reviewed := 0, 0
	fixture := strings.Replace(replayPlanFixture, `"goal_id":`, `"requirement_id":"REQ-001","goal_id":`, 1)
	e.mgr.cfg.PlanGenerator = planCompletionFunc(func(_ context.Context, prompt string) (string, error) {
		generated++
		if !strings.Contains(prompt, `"requirement_id"`) || !strings.Contains(prompt, "REQ-001") {
			t.Fatal("generation/refinement prompt lost identity")
		}
		if generated == 1 {
			if !strings.Contains(prompt, "SMALL, well-scoped change") {
				t.Fatal("not compact")
			}
			return fixture, nil
		}
		if !strings.Contains(prompt, "Missing reload evidence") {
			t.Fatal("missing independent findings")
		}
		return strings.Replace(fixture, "Verify the running editing journey", "Verify saved reload", 1), nil
	})
	e.mgr.cfg.PlanReviewer = planCompletionFunc(func(_ context.Context, prompt string) (string, error) {
		reviewed++
		if !strings.Contains(prompt, `"requirement_id":"REQ-001"`) {
			t.Fatal("review prompt lost identity")
		}
		if reviewed == 1 {
			return rejectedReplayReview, nil
		}
		return replayReviewFixture, nil
	})
	result, err := e.mgr.Plan(context.Background(), req)
	if err != nil || !result.Valid || generated != 2 || reviewed != 2 {
		t.Fatalf("compact refinement failed: %+v %v %d/%d", result, err, generated, reviewed)
	}
	raw, err := os.ReadFile(result.ArtifactPath)
	if err != nil {
		t.Fatal(err)
	}
	var saved planner.ProjectPlan
	if err := json.Unmarshal(raw, &saved); err != nil || saved.Stories[0].RequirementID != "REQ-001" {
		t.Fatalf("persisted identity lost: %s %v", raw, err)
	}
	e.mgr.Close()
	e.closeState()
	reopened, err := state.NewStore(filepath.Join(e.dir, "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	cfg := e.mgr.cfg
	cfg.StateStore = reopened
	fresh := &Manager{cfg: cfg, state: reopened}
	again, err := fresh.Plan(context.Background(), req)
	if err != nil || !again.Valid || again.Plan.Stories[0].RequirementID != "REQ-001" || generated != 2 || reviewed != 2 {
		t.Fatalf("reopened receipt lost identity or replayed provider: %+v %v", again, err)
	}
	var imports int
	if err := reopened.GetDB().QueryRow(`SELECT COUNT(*) FROM run_steps WHERE agent='reviewed-plan-import'`).Scan(&imports); err != nil || imports != 1 {
		t.Fatalf("import repeated: %d %v", imports, err)
	}
}
