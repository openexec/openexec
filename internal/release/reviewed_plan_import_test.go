package release

import (
	"context"
	"path/filepath"
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
