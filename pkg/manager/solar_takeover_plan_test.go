package manager

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/openexec/openexec/internal/planner"
	"github.com/openexec/openexec/internal/release"
	"github.com/openexec/openexec/pkg/db/state"
)

// Exercise the checked-in task artifact through the real parser, review gate,
// importer and SQLite reopen. No Solar workspace or provider is used. The
// scripted verdicts test import mechanics, not actual independent plan review.
func TestSolarTakeoverTaskSplit(t *testing.T) {
	raw, err := os.ReadFile("../../docs/plans/solar-takeover.json")
	if err != nil {
		t.Fatal(err)
	}
	wantIDs := []string{"system-understanding", "environment-setup", "safe-operation", "development-release", "takeover-plan"}
	wantCriteria := []string{
		"system-understanding: System architecture and communication are documented",
		"environment-setup: Test and development setup is documented from zero",
		"safe-operation: Safe operation has a clear documented plan",
		"development-release: Development and releases have a clear documented process",
		"takeover-plan: Takeover preserves service with [source text ends here; recover the full accepted condition before claiming Ready]",
	}
	wantDeps := [][]string{nil, nil, wantIDs[:2], wantIDs[:2], wantIDs[:4]}
	for _, approved := range []bool{false, true} {
		t.Run(map[bool]string{false: "rejected", true: "import_and_reopen"}[approved], func(t *testing.T) {
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, "INTENT.md"), []byte("Split the five supplied Solar documentation conditions into separate tasks; no handover execution."), 0600); err != nil {
				t.Fatal(err)
			}
			dbPath := filepath.Join(dir, "state.db")
			ledger, err := state.NewStore(dbPath)
			if err != nil {
				t.Fatal(err)
			}
			closeLedger := sync.OnceFunc(func() { ledger.Close() })
			t.Cleanup(closeLedger)
			verdict, err := json.Marshal(planner.PlanReview{Approved: approved, Assessment: "Scripted import control; not a Solar assessment"})
			if err != nil {
				t.Fatal(err)
			}
			mgr, err := New(Config{WorkDir: dir, StateStore: ledger, PlanGenerator: fixedPlanCompletion(raw), PlanReviewer: fixedPlanCompletion(verdict)})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { mgr.Close() })
			rel, err := mgr.GetInternalReleaseManager()
			if err != nil {
				t.Fatal(err)
			}
			// A separate owner's task is outside the split and must survive it.
			createStory(t, rel, "US-OWNER", nil)
			createTask(t, rel, "T-OWNER", "US-OWNER", nil)
			ownerBefore, err := rel.TaskSnapshot(context.Background(), "T-OWNER")
			if err != nil {
				t.Fatal(err)
			}
			result, err := mgr.Plan(context.Background(), PlanRequest{IntentFile: "INTENT.md", NoValidate: true, Review: true, AutoImport: true})
			if err != nil || result.Valid != approved {
				t.Fatalf("plan/import: %+v, %v", result, err)
			}
			if len(result.Plan.Goals) != 0 || len(result.Plan.Stories) != 1 {
				t.Fatal("task split invented goals or extra stories")
			}
			story := result.Plan.Stories[0]
			if story.Title != "Document and prepare the Solar system takeover" || !reflect.DeepEqual(story.AcceptanceCriteria, wantCriteria) || len(story.Tasks) != 5 {
				t.Fatalf("supplied outcome/conditions not preserved: %+v", story)
			}
			for i, task := range story.Tasks {
				if task.ID != wantIDs[i] || strings.Join(task.DependsOn, ",") != strings.Join(wantDeps[i], ",") || task.Mode != planner.TaskModeAFK {
					t.Fatalf("task identity, dependency or documentation mode changed: %+v", task)
				}
				if task.Title == "" || task.Description == "" || task.TechnicalStrategy == "" || !strings.Contains(task.Description, "Completion evidence:") {
					t.Fatalf("task lacks its own scope and completion evidence: %+v", task)
				}
				path := "docs/solar/" + task.ID + ".md"
				if !strings.Contains(task.Description, path) || task.VerificationScript != "test -s "+path {
					t.Fatalf("task has no distinct deliverable check: %+v", task)
				}
			}
			// Confirm state on a new connection, not from the importer's cache.
			mgr.Close()
			closeLedger()
			reopened, err := state.NewStore(dbPath)
			if err != nil {
				t.Fatal(err)
			}
			defer reopened.Close()
			fresh, err := release.NewManagerWithDB(dir, nil, reopened.GetDB())
			if err != nil {
				t.Fatal(err)
			}
			defer fresh.Close()
			if err := fresh.Load(); err != nil {
				t.Fatal(err)
			}
			ownerAfter, err := fresh.TaskSnapshot(context.Background(), "T-OWNER")
			if err != nil || !reflect.DeepEqual(ownerBefore, ownerAfter) {
				t.Fatalf("unrelated owner task changed: %+v, %v", ownerAfter, err)
			}
			if len(fresh.GetGoals()) != 0 {
				t.Fatal("split created a new accepted destination")
			}
			if !approved {
				if len(fresh.GetTasks()) != 1 || len(fresh.GetStories()) != 1 {
					t.Fatal("rejected plan imported work")
				}
				return
			}
			if len(fresh.GetTasks()) != 6 || len(fresh.GetStories()) != 2 {
				t.Fatal("split did not persist exactly five new tasks and one story")
			}
			savedStory := fresh.GetStory(story.ID)
			if savedStory == nil || !reflect.DeepEqual(savedStory.Tasks, wantIDs) || !reflect.DeepEqual(savedStory.AcceptanceCriteria, wantCriteria) || savedStory.Contract != story.Contract {
				t.Fatalf("story contract/condition membership lost on reopen: %+v", savedStory)
			}
			for _, task := range story.Tasks {
				saved, err := fresh.TaskSnapshot(context.Background(), task.ID)
				if err != nil || saved.StoryID != story.ID || saved.Title != task.Title || saved.Description != task.Description+"\n\nTechnical strategy:\n"+task.TechnicalStrategy || saved.Status != release.TaskStatusPending || saved.ExecutionMode() != release.TaskModeAFK || strings.Join(saved.DependsOn, ",") != strings.Join(task.DependsOn, ",") || saved.VerificationScript != task.VerificationScript {
					t.Fatalf("task lost content or became completed during the split: %+v, %v", saved, err)
				}
				// The deliverable check must refuse missing and empty documents.
				// Passing it establishes file presence only, not Solar Ready.
				path := filepath.Join(dir, "docs", "solar", task.ID+".md")
				check := func(wantPass bool) {
					t.Helper()
					cmd := exec.Command("sh", "-c", saved.VerificationScript)
					cmd.Dir = dir
					if err := cmd.Run(); (err == nil) != wantPass {
						t.Fatalf("deliverable check: pass=%t, error=%v", wantPass, err)
					}
				}
				check(false)
				if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, nil, 0600); err != nil {
					t.Fatal(err)
				}
				check(false)
				if err := os.WriteFile(path, []byte("Fixture document; no Solar evidence"), 0600); err != nil {
					t.Fatal(err)
				}
				check(true)
			}
		})
	}
}
