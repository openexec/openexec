package manager

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/openexec/openexec/internal/planner"
	"github.com/openexec/openexec/internal/release"
	"github.com/openexec/openexec/pkg/db/state"
)

// Use the real planner-to-ledger conversion, native pipeline and durable store.
// Queue success is task scope completion; it neither waits for nor supplies D2.
func TestTaskQueuePreparationDoesNotWaitForOrClaimMerge(t *testing.T) {
	e := newSchedulerTestEnv(t)
	// A real deterministic check runs without any post-queue merge artifact.
	config := `{"execution":{"lint_commands":["true"],"test_commands":["test ! -e coordinator-merge.json && printf 'verified\\n' >> preparation.txt"]}}`
	if err := os.WriteFile(filepath.Join(e.dir, ".openexec", "config.json"), []byte(config), 0600); err != nil {
		t.Fatal(err)
	}
	const contract = `D2 outstanding: Agent Console merges after queue completion under existing owner controls; no merge evidence yet`
	var plan planner.ProjectPlan
	if err := json.Unmarshal([]byte(`{"goals":[{"id":"G","title":"Correction and merge"}],"stories":[{"id":"S","goal_id":"G","requirement_id":"D2","title":"Prepare","contract":"`+contract+`","acceptance_criteria":["D1 preparation","D2 actual coordinator merge"],"tasks":[{"id":"A","title":"Prepare","mode":"afk"},{"id":"B","title":"Check preparation","mode":"afk","depends_on":["A"]}]}]}`), &plan); err != nil {
		t.Fatal(err)
	}
	goals, stories, tasks := reviewedPlanRows(&plan)
	for _, g := range goals {
		if err := e.rel.CreateGoal(g); err != nil {
			t.Fatal(err)
		}
	}
	for _, s := range stories {
		if err := e.rel.CreateStory(s); err != nil {
			t.Fatal(err)
		}
	}
	for _, task := range tasks {
		task.Status = release.TaskStatusPending
		if err := e.rel.CreateTask(task); err != nil {
			t.Fatal(err)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	opts := RunOptions{TaskOriented: true, StoryIDs: []string{"S"}}
	if err := e.mgr.ExecuteTasks(ctx, opts); err != nil {
		t.Fatal("preparation waits for post-queue merge", err)
	}
	for _, id := range []string{"A", "B"} {
		info, err := e.mgr.Status(id)
		if err != nil || info.Status != StatusComplete {
			t.Fatal("native pipeline not exercised", id, err)
		}
	}
	// The absent artifact cannot be a prerequisite or be invented by completion.
	if _, err := os.Stat(filepath.Join(e.dir, "coordinator-merge.json")); !os.IsNotExist(err) {
		t.Fatal("queue fabricated merge evidence", err)
	}
	if err := e.mgr.ExecuteTasks(ctx, opts); err != nil {
		t.Fatal("completed queue waits on delivery on restart", err)
	}
	prepared, err := os.ReadFile(filepath.Join(e.dir, "preparation.txt"))
	if err != nil || string(prepared) != "verified\nverified\n" {
		t.Fatal("native checks did not run exactly once per preparation task", string(prepared), err)
	}
	e.closeState()
	store, err := state.NewStore(filepath.Join(e.dir, "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	rel, err := release.NewManagerWithDB(e.dir, release.DefaultConfig(), store.GetDB())
	if err != nil {
		t.Fatal(err)
	}
	persisted := rel.GetStory("S")
	if persisted == nil || persisted.Contract != contract || len(persisted.AcceptanceCriteria) != 2 {
		t.Fatal("queue completion lost or satisfied outstanding D2", persisted)
	}
	rows, err := rel.TasksInStories(ctx, []string{"S"})
	if err != nil || len(rows) != 2 {
		t.Fatal("queue created post-queue delivery work", err)
	}
	for _, task := range rows {
		if task.Status != release.TaskStatusDone || task.AttemptCount != 1 {
			t.Fatal("preparation not durably completed once", task)
		}
		if task.Metadata["merge_evidence"] != nil || task.Metadata["goal_complete"] != nil {
			t.Fatal("queue invented delivery verdict", task.Metadata)
		}
	}
}
