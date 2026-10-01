package planner

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestRemapContentReservationsAndReplay(t *testing.T) {
	if RemapPlanIDs(nil, ExistingLookup{}) != 0 {
		t.Fatal("nil plan changed")
	}
	p := &ProjectPlan{Stories: []Story{
		{ID: "US-001", Tasks: []Task{{ID: "T-US-001-001", Description: "new"}, {ID: "custom"}, {ID: "fresh"}, {ID: "T-US-0010-001"}}},
		{ID: "US-005", Tasks: []Task{{ID: "T-US-002-001"}}},
	}}
	look := ExistingLookup{
		GoalTitle:  func(string) (string, bool) { return "", false },
		StoryTitle: func(id string) (string, bool) { return "", id == "US-001" },
		TaskExists: func(id string) bool { return id == "custom" || id == "T-US-002-002" },
		Conflicts:  func(p *ProjectPlan) map[string]bool { return map[string]bool{"US-001": true} },
	}
	if n := RemapPlanIDs(p, look); n != 3 {
		t.Fatalf("changed %d identities", n)
	}
	if p.Stories[0].ID != "US-002" || p.Stories[0].Tasks[0].ID != "T-US-002-003" || p.Stories[0].Tasks[1].ID != "custom-001" {
		t.Fatalf("reservations lost: %+v", p)
	}
	if p.Stories[0].Tasks[2].ID != "fresh" || p.Stories[0].Tasks[3].ID != "T-US-0010-001" {
		t.Fatal("unoccupied custom or longer-prefix task moved")
	}
	raw, _ := json.Marshal(p)
	if n := RemapPlanIDs(p, look); n != 0 {
		t.Fatal("fresh mapping moved")
	}
	after, _ := json.Marshal(p)
	if !reflect.DeepEqual(raw, after) {
		t.Fatal("replay changed bytes")
	}
}

func TestRewritePlanRefsAllFields(t *testing.T) {
	text := "G-001 US-001 T-US-001-001 US-0010"
	p := &ProjectPlan{Goals: []Goal{{ID: "G-001", Title: text, Description: text, SuccessCriteria: text, VerificationMethod: text}}, Stories: []Story{{ID: "US-001", Title: text, GoalID: "G-001", Description: text, Contract: text, VerificationScript: text, AcceptanceCriteria: []string{text}, DependsOn: []string{"US-001", "US-009"}, Tasks: []Task{{ID: "T-US-001-001", Title: text, Description: text, TechnicalStrategy: text, VerificationScript: text, DependsOn: []string{"T-US-001-001"}, DecisionRef: text}}}}}
	rewritePlanRefs(p, map[string]string{"G-001": "G-002", "US-001": "US-002", "T-US-001-001": "T-US-002-001"})
	want := "G-002 US-002 T-US-002-001 US-0010"
	g, s, task := p.Goals[0], p.Stories[0], p.Stories[0].Tasks[0]
	for _, got := range []string{g.Description, g.SuccessCriteria, g.VerificationMethod, s.Description, s.Contract, s.VerificationScript, s.AcceptanceCriteria[0], task.Description, task.TechnicalStrategy, task.VerificationScript} {
		if got != want {
			t.Fatalf("prose %q", got)
		}
	}
	if g.Title != text || s.Title != text || task.Title != text || task.DecisionRef != text {
		t.Fatal("non-reference fields changed")
	}
	if s.DependsOn[0] != "US-002" || s.DependsOn[1] != "US-009" || task.DependsOn[0] != "T-US-002-001" {
		t.Fatal("structured references wrong")
	}
}
