package release

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

// The same matrix reaches each list-bearing identity field, so omitting one
// comparator or SQL predicate cannot hide behind coverage of the shared helper.
func TestReviewedIdentityListMatrix(t *testing.T) {
	values := []struct {
		name string
		list []string
	}{
		{"nil", nil}, {"empty", []string{}},
		{"ab", []string{"a", "b"}}, {"ba", []string{"b", "a"}},
		{"ac", []string{"a", "c"}}, {"aa", []string{"a", "a"}},
	}
	for _, field := range []struct{ table, column string }{
		{"stories", "acceptance_criteria"}, {"stories", "depends_on"},
		{"stories", "tasks"}, {"tasks", "depends_on"},
	} {
		db, cleanup := testDB(t)
		defer cleanup()
		store, err := NewSQLiteStore(db)
		if err != nil {
			t.Fatal(err)
		}
		ctx := context.Background()
		if err := store.CreateStory(ctx, &Story{ID: "US-list"}); err != nil {
			t.Fatal(err)
		}
		if err := store.CreateTask(ctx, &Task{ID: "T-list", StoryID: "US-list"}); err != nil {
			t.Fatal(err)
		}
		m := &Manager{store: store}

		for i, old := range values {
			for j, next := range values {
				t.Run(field.table+"/"+field.column+"/"+old.name+"/"+next.name, func(t *testing.T) {
					want := i == j || i < 2 && j < 2
					st, ns := Story{}, Story{}
					task, nt := Task{}, Task{}
					switch field.column {
					case "acceptance_criteria":
						st.AcceptanceCriteria, ns.AcceptanceCriteria = old.list, next.list
					case "tasks":
						st.Tasks, ns.Tasks = old.list, next.list
					case "depends_on":
						st.DependsOn, ns.DependsOn = old.list, next.list
						task.DependsOn, nt.DependsOn = old.list, next.list
					}
					got := ReviewedStoryEqual(&st, &ns)
					if field.table == "tasks" {
						got = ReviewedTaskEqual(&task, &nt)
					}
					if got != want {
						t.Fatalf("list identity: got %v want %v", got, want)
					}

					st.ID, ns.ID = "US-list", "US-list"
					task.ID, nt.ID = "T-list", "T-list"
					task.StoryID, nt.StoryID = st.ID, st.ID

					id := st.ID
					if field.table == "tasks" {
						id = task.ID
					}
					raw, err := json.Marshal(old.list)
					if err != nil {
						t.Fatal(err)
					}
					// Include SQL NULL only for these nullable schema columns.
					retained := []any{string(raw), raw}
					if old.list == nil {
						retained = append(retained, nil)
					}
					for _, value := range retained {
						if _, err := db.Exec("UPDATE "+field.table+" SET "+field.column+"=? WHERE id=?", value, id); err != nil {
							t.Fatal(err)
						}
						var err error
						if field.table == "tasks" {
							err = m.ValidatePlanIdentities(ctx, nil, nil, []*Task{&nt})
						} else {
							err = m.ValidatePlanIdentities(ctx, nil, []*Story{&ns}, nil)
						}
						if want && err != nil {
							t.Fatalf("SQL empty/equal list refused: %v", err)
						}
						if !want && (err == nil || !strings.Contains(err.Error(), "conflicts with retained content")) {
							t.Fatalf("SQL differing list must conflict: %v", err)
						}
						var persisted any
						if err := db.QueryRow("SELECT "+field.column+" FROM "+field.table+" WHERE id=?", id).Scan(&persisted); err != nil {
							t.Fatal(err)
						}
						if !reflect.DeepEqual(persisted, value) {
							t.Fatalf("validation rewrote retained JSON: %v != %v", persisted, value)
						}
					}
				})
			}
		}
	}
}
