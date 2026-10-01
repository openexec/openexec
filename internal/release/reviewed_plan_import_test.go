package release

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/openexec/openexec/pkg/db/state"
)

// A reviewed story may serve no goal. The ordinary store writes that as NULL;
// the reviewed import wrote ”, a goal id no row has, and the whole refined
// plan failed with "FOREIGN KEY constraint failed (787)".
func TestReviewedStoryWithoutAGoalImports(t *testing.T) {
	// The ledger's schema, where stories.goal_id references goals(id).
	dir := testDir(t)
	ledger, err := state.NewStore(filepath.Join(dir, ".openexec", "openexec.db"))
	if err != nil {
		t.Fatal(err)
	}
	ledger.Close()
	m, err := NewManager(dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { m.Close() })
	ctx := context.Background()
	stories := []*Story{{ID: "US-014", Title: "Reconcile", StoryType: StoryTypeFeature, Tasks: []string{"T-US-014-001"}}}
	tasks := []*Task{{ID: "T-US-014-001", StoryID: "US-014", Title: "Reconcile receipts", MaxAttempts: 3}}
	if err := m.ValidatePlanIdentities(ctx, nil, stories, tasks); err != nil {
		t.Fatal(err)
	}
}

func TestReviewedIdentityAtomicRefusals(t *testing.T) {
	ctx := context.Background()
	if err := (&Manager{}).ValidatePlanIdentities(ctx, nil, nil, nil); err == nil {
		t.Fatal("non-SQLite accepted")
	}
	for _, kind := range []string{"cancelled", "metadata", "mode", "receipt", "json"} {
		t.Run(kind, func(t *testing.T) {
			dir := testDir(t)
			ledger, err := state.NewStore(filepath.Join(dir, ".openexec", "openexec.db"))
			if err != nil {
				t.Fatal(err)
			}
			defer ledger.Close()
			m, err := NewManagerWithDB(dir, nil, ledger.GetDB())
			if err != nil {
				t.Fatal(err)
			}
			db := ledger.GetDB()
			if _, err := db.Exec(`INSERT INTO stories(id,title,story_type,tasks,depends_on,acceptance_criteria) VALUES('US-001','Retained','feature','["T-1"]','[]','[]'); INSERT INTO tasks(id,story_id,title,max_attempts,depends_on,metadata) VALUES('T-1','US-001','Retained',3,'[]','{"mode":"afk"}')`); err != nil {
				t.Fatal(err)
			}
			task := &Task{ID: "T-1", StoryID: "US-001", Title: "Retained", MaxAttempts: 3}
			goals := []*Goal{{ID: "G-new", Title: "Must roll back"}}
			callCtx := ctx
			switch kind {
			case "cancelled":
				c, cancel := context.WithCancel(ctx)
				cancel()
				callCtx = c
			case "metadata":
				if _, err := db.Exec(`UPDATE tasks SET metadata='broken'`); err != nil {
					t.Fatal(err)
				}
			case "mode":
				task.Metadata = map[string]interface{}{"mode": "hitl"}
			case "json":
				if _, err := db.Exec(`UPDATE tasks SET depends_on='broken'`); err != nil {
					t.Fatal(err)
				}
			}
			err = m.ImportReviewedPlan(callCtx, goals, nil, []*Task{task}, "receipt", "missing-run", "digest", "receipt")
			want := map[string]string{"cancelled": "context canceled", "metadata": "invalid retained task metadata", "mode": "mode conflicts", "receipt": "FOREIGN KEY constraint failed", "json": "malformed JSON"}[kind]
			if err == nil || !strings.Contains(err.Error(), want) {
				t.Fatalf("expected %q refusal, got %v", want, err)
			}
			var n int
			if err := db.QueryRow(`SELECT COUNT(*) FROM goals WHERE id='G-new'`).Scan(&n); err != nil || n != 0 {
				t.Fatalf("partial insertion: %d %v", n, err)
			}
			if err := db.QueryRow(`SELECT COUNT(*) FROM run_steps WHERE id='receipt'`).Scan(&n); err != nil || n != 0 {
				t.Fatalf("partial receipt: %d %v", n, err)
			}
		})
	}
}
