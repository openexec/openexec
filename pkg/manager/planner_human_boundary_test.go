package manager

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/openexec/openexec/internal/planner"
	"github.com/openexec/openexec/internal/release"
)

func TestHumanBoundaryImportPathsPreserveSchedulingAndMetadata(t *testing.T) {
	for _, path := range []string{"legacy", "reviewed", "replayed"} {
		t.Run(path, func(t *testing.T) {
			e := newSchedulerTestEnv(t)
			p := &planner.ProjectPlan{Goals: []planner.Goal{{ID: "G", Title: "Deliver"}}, Stories: []planner.Story{{ID: "S", GoalID: "G", Title: "Deliver", VerificationScript: "go test ./...", Tasks: []planner.Task{
				{ID: "prepare", Title: "Prepare", Mode: planner.TaskModeAFK},
				{ID: "decision", Title: "Grant access", Mode: planner.TaskModeHITL, DecisionReason: "Owner must grant access", DecisionRef: "request:access", DependsOn: []string{"prepare"}},
				{ID: "apply", Title: "Use access", Mode: planner.TaskModeAFK, DependsOn: []string{"decision"}},
				{ID: "independent", Title: "Run independent checks", Mode: planner.TaskModeAFK},
			}}}}
			raw, _ := json.Marshal(p)
			e.mgr.cfg.PlanGenerator = fixedPlanCompletion(string(raw))
			e.mgr.cfg.PlanReviewer = fixedPlanCompletion(replayReviewFixture)
			if err := os.WriteFile(filepath.Join(e.dir, "INTENT.md"), []byte("Deliver"), 0600); err != nil {
				t.Fatal(err)
			}
			req := PlanRequest{IntentFile: "INTENT.md", NoValidate: true, AutoImport: true, Review: path != "legacy"}
			if path == "replayed" {
				req.RequestID = "boundary-import"
			}
			ctx := context.Background()
			result, err := e.mgr.Plan(ctx, req)
			if err != nil || !result.Valid {
				t.Fatalf("plan import = %+v, %v", result, err)
			}
			if path == "replayed" {
				fresh := &Manager{cfg: e.mgr.cfg, state: e.mgr.state}
				if _, err := fresh.Plan(ctx, req); err != nil {
					t.Fatal(err)
				}
			}
			// Read the persisted ledger, not the generated planner response.
			backlog, err := e.mgr.GetInternalReleaseManager()
			if err != nil {
				t.Fatal(err)
			}
			task, err := backlog.TaskSnapshot(ctx, "decision")
			if err != nil {
				t.Fatal(err)
			}
			if task.ExecutionMode() != release.TaskModeHITL || task.Metadata["decision_reason"] != "Owner must grant access" || task.Metadata["decision_ref"] != "request:access" {
				t.Fatalf("boundary metadata lost: %+v", task)
			}
			ready, err := backlog.RunnableTasks(ctx, []string{"S"})
			if err != nil || len(ready) != 2 {
				t.Fatalf("eligible work: %+v, %v", ready, err)
			}
			for _, task := range ready {
				if task.ID != "prepare" && task.ID != "independent" {
					t.Fatalf("human boundary bypassed: %s", task.ID)
				}
			}
		})
	}
}
