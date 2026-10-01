package release

import (
	"context"
	"testing"
)

func TestReviewedIdentityNullableStoryMembership(t *testing.T) {
	db, cleanup := testDB(t)
	defer cleanup()
	store, err := NewSQLiteStore(db)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	st := &Story{ID: "US-empty", Title: "Empty", StoryType: "feature", AcceptanceCriteria: []string{}, DependsOn: []string{}, Tasks: []string{}}
	if err := store.CreateStory(ctx, st); err != nil {
		t.Fatal(err)
	}
	for _, value := range []string{"NULL", "'null'"} {
		if _, err := db.Exec("UPDATE stories SET tasks=" + value + ", acceptance_criteria=" + value + ", depends_on=" + value + ", git_branch='retained', git_merged_at='2026-01-01T00:00:00Z' WHERE id='US-empty'"); err != nil {
			t.Fatal(err)
		}
		m := &Manager{store: store}
		if err := m.ValidatePlanIdentities(ctx, nil, []*Story{st}, nil); err != nil {
			t.Fatal(err)
		}
		changed := *st
		changed.Tasks = []string{"T-new"}
		if err := m.ValidatePlanIdentities(ctx, nil, []*Story{&changed}, nil); err == nil {
			t.Fatal("different ordered membership accepted")
		}
		got, err := store.GetStory(ctx, st.ID)
		if err != nil {
			t.Fatal(err)
		}
		if !ReviewedStoryEqual(got, st) || got.Git == nil || got.Git.MergedAt == nil {
			t.Fatalf("nullable story round trip: %+v", got)
		}
	}
	if _, err := store.GetStory(ctx, "missing"); err != ErrStoryNotFound {
		t.Fatalf("missing story: %v", err)
	}
	db.Close()
	if _, err := store.GetStory(ctx, st.ID); err == nil {
		t.Fatal("closed database accepted")
	}
}
