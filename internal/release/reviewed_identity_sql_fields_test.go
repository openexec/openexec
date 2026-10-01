package release

import (
	"context"
	"reflect"
	"strings"
	"testing"
)

// Every scalar canonical field must participate in the SQL predicate too.
// Parent references point to real rows so a foreign-key error cannot pass.
func TestReviewedIdentitySQLFields(t *testing.T) {
	for _, entity := range []struct {
		table  string
		fields []string
	}{
		{"goals", []string{"Title", "Description", "SuccessCriteria", "VerificationMethod"}},
		{"stories", []string{"GoalID", "Title", "Description", "VerificationScript", "Contract", "StoryType", "Priority"}},
		{"tasks", []string{"StoryID", "Title", "Description", "VerificationScript", "Priority", "MaxAttempts", "Metadata"}},
	} {
		for _, field := range entity.fields {
			t.Run(entity.table+"/"+field, func(t *testing.T) {
				db, cleanup := testDB(t)
				defer cleanup()
				store, err := NewSQLiteStore(db)
				if err != nil {
					t.Fatal(err)
				}
				ctx := context.Background()
				goal := &Goal{ID: "G-1", Title: "title", Description: "description", SuccessCriteria: "success", VerificationMethod: "method"}
				story := &Story{ID: "US-1", GoalID: "G-1", Title: "title", Description: "description", StoryType: "feature", Priority: 1}
				task := &Task{ID: "T-1", StoryID: "US-1", Title: "title", Description: "description", Priority: 1, MaxAttempts: 3, Metadata: map[string]interface{}{"mode": "afk"}}
				for _, g := range []*Goal{goal, {ID: "G-2", Title: "other"}} {
					if err := store.CreateGoal(ctx, g); err != nil {
						t.Fatal(err)
					}
				}
				for _, s := range []*Story{story, {ID: "US-2", Title: "other"}} {
					if err := store.CreateStory(ctx, s); err != nil {
						t.Fatal(err)
					}
				}
				if err := store.CreateTask(ctx, task); err != nil {
					t.Fatal(err)
				}
				m := &Manager{store: store}
				validate := func() error { return m.ValidatePlanIdentities(ctx, []*Goal{goal}, []*Story{story}, []*Task{task}) }
				if err := validate(); err != nil {
					t.Fatalf("identical canonical fields refused: %v", err)
				}
				var target any
				switch entity.table {
				case "goals":
					target = goal
				case "stories":
					target = story
				case "tasks":
					target = task
				}
				value := reflect.ValueOf(target).Elem().FieldByName(field)
				original := reflect.New(value.Type()).Elem()
				original.Set(value)
				switch field {
				case "GoalID":
					value.SetString("G-2")
				case "StoryID":
					value.SetString("US-2")
				case "Metadata":
					value.Set(reflect.ValueOf(map[string]interface{}{"mode": "hitl"}))
				default:
					if value.Kind() == reflect.String {
						value.SetString(value.String() + " changed")
					} else {
						value.SetInt(value.Int() + 1)
					}
				}
				want := "conflicts with retained content"
				if field == "Metadata" {
					want = "mode conflicts"
				}
				if err := validate(); err == nil || !strings.Contains(err.Error(), want) {
					t.Fatalf("SQL ignored canonical field %s: %v", field, err)
				}
				value.Set(original)
				if err := validate(); err != nil {
					t.Fatalf("refusal changed retained content: %v", err)
				}
			})
		}
	}
}
