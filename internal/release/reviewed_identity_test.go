package release

import (
	"reflect"
	"testing"
)

func TestReviewedCanonicalFields(t *testing.T) {
	goal := Goal{Title: "title", Description: "description", SuccessCriteria: "success", VerificationMethod: "method"}
	story := Story{GoalID: "G-001", Title: "title", Description: "description", AcceptanceCriteria: []string{"a", "b"}, VerificationScript: "verify", Contract: "contract", DependsOn: []string{"US-001", "US-002"}, StoryType: "feature", Priority: 2, Tasks: []string{"T-1", "T-2"}}
	task := Task{StoryID: "US-001", Title: "title", Description: "description", VerificationScript: "verify", DependsOn: []string{"T-1", "T-2"}, Priority: 2, MaxAttempts: 3, Metadata: map[string]interface{}{"mode": "afk"}}
	cases := []struct {
		name     string
		original interface{}
		fields   []string
		equal    func(interface{}) bool
	}{
		{"goal", goal, []string{"Title", "Description", "SuccessCriteria", "VerificationMethod"}, func(v interface{}) bool { n := v.(Goal); return ReviewedGoalEqual(&goal, &n) }},
		{"story", story, []string{"GoalID", "Title", "Description", "AcceptanceCriteria", "VerificationScript", "Contract", "DependsOn", "StoryType", "Priority", "Tasks"}, func(v interface{}) bool { n := v.(Story); return ReviewedStoryEqual(&story, &n) }},
		{"task", task, []string{"StoryID", "Title", "Description", "VerificationScript", "DependsOn", "Priority", "MaxAttempts", "Metadata"}, func(v interface{}) bool { n := v.(Task); return ReviewedTaskEqual(&task, &n) }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if !c.equal(c.original) {
				t.Fatal("identical content differs")
			}
			for _, field := range c.fields {
				t.Run(field, func(t *testing.T) {
					v := reflect.New(reflect.TypeOf(c.original)).Elem()
					v.Set(reflect.ValueOf(c.original))
					f := v.FieldByName(field)
					switch f.Kind() {
					case reflect.String:
						f.SetString(f.String() + " changed")
					case reflect.Int:
						f.SetInt(f.Int() + 1)
					case reflect.Slice:
						f.Set(reflect.ValueOf([]string{"b", "a"}))
					case reflect.Map:
						f.Set(reflect.ValueOf(map[string]interface{}{"mode": "hitl"}))
					}
					if c.equal(v.Interface()) {
						t.Fatalf("ignored canonical field %s", field)
					}
				})
			}
		})
	}
}

func TestReviewedCanonicalNormalizationAndRetention(t *testing.T) {
	for _, c := range []struct {
		old, next []string
		want      bool
	}{
		{[]string{}, nil, true}, {[]string{}, []string{}, true}, {nil, nil, false}, {nil, []string{}, false},
		{[]string{"a", "b"}, []string{"b", "a"}, false}, {[]string{"a"}, []string{"a", "a"}, false},
	} {
		if got := reviewedArrayEqual(c.old, c.next); got != c.want {
			t.Fatalf("arrays %#v %#v: %v", c.old, c.next, got)
		}
	}
	for _, c := range []struct {
		metadata map[string]interface{}
		want     string
	}{
		{nil, "afk"}, {map[string]interface{}{"mode": 1}, "afk"}, {map[string]interface{}{"mode": ""}, ""}, {map[string]interface{}{"mode": "hitl"}, "hitl"},
	} {
		if got := reviewedMode(&Task{Metadata: c.metadata}); got != c.want {
			t.Fatalf("mode %q", got)
		}
	}
	old := Task{DependsOn: []string{}, Metadata: map[string]interface{}{"mode": "afk", "evidence": "retained"}, Status: "done", AttemptCount: 2, Git: &TaskGitInfo{Commits: []string{"commit"}}}
	next := Task{Metadata: map[string]interface{}{"decision_reason": "new routing evidence"}}
	if !ReviewedTaskEqual(&old, &next) {
		t.Fatal("lifecycle/extra metadata participates in identity")
	}
	s := Story{AcceptanceCriteria: []string{}, DependsOn: []string{}, Tasks: []string{}, Status: "done", Git: &StoryGitInfo{Branch: "retained"}}
	if !ReviewedStoryEqual(&s, &Story{}) {
		t.Fatal("story lifecycle participates in identity")
	}
	g := Goal{Title: "title"}
	n := g
	n.Title = " title "
	if ReviewedGoalEqual(&g, &n) {
		t.Fatal("trimmed significant string")
	}
}
