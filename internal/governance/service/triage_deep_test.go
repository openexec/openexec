package service

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/openexec/openexec/internal/governance"
	"github.com/openexec/openexec/pkg/runtime"
)

// fakePlanStore is an in-memory PlanStore for exercising the deep-triage bridge
// without booting a real release.Manager.
type fakePlanStore struct {
	goals   map[string]*runtime.Goal
	stories map[string]*runtime.Story
	tasks   map[string]*runtime.Task
}

func newFakePlanStore() *fakePlanStore {
	return &fakePlanStore{
		goals:   map[string]*runtime.Goal{},
		stories: map[string]*runtime.Story{},
		tasks:   map[string]*runtime.Task{},
	}
}

func (f *fakePlanStore) GetGoal(id string) *runtime.Goal   { return f.goals[id] }
func (f *fakePlanStore) GetStory(id string) *runtime.Story { return f.stories[id] }
func (f *fakePlanStore) GetTask(id string) *runtime.Task   { return f.tasks[id] }
func (f *fakePlanStore) CreateGoal(g *runtime.Goal) error  { f.goals[g.ID] = g; return nil }
func (f *fakePlanStore) CreateStory(s *runtime.Story) error {
	f.stories[s.ID] = s
	return nil
}
func (f *fakePlanStore) CreateTask(t *runtime.Task) error {
	f.tasks[t.ID] = t
	// Mirror release.Manager.CreateTask: append the task to its parent story.
	if st := f.stories[t.StoryID]; st != nil {
		st.Tasks = append(st.Tasks, t.ID)
	}
	return nil
}
func (f *fakePlanStore) UpdateTask(t *runtime.Task) error { f.tasks[t.ID] = t; return nil }
func (f *fakePlanStore) DeleteStory(id string) error {
	if st := f.stories[id]; st != nil {
		for _, tid := range st.Tasks {
			delete(f.tasks, tid)
		}
		delete(f.stories, id)
	}
	return nil
}

const deepPlanJSON = `{
  "schema_version": "1.0.0",
  "goals": [{"id":"G-001","title":"Fix login","description":"Users can log in reliably"}],
  "stories": [{
    "id":"US-001","title":"Validate login form","goal_id":"G-001",
    "acceptance_criteria":["invalid credentials show an error"],
    "verification_script":"npm test -- login",
    "tasks":[{"id":"T-US-001-001","title":"Add form validation","mode":"afk","verification_script":"npm test -- login"}]
  }]
}`

// routingCompleter returns a classification response for the classifier prompt
// (which contains "classifier") and the plan JSON for the planner prompt, so a
// single completer serves both of TriageDeep's LLM calls.
type routingCompleter struct {
	plan   string
	classi string
	impact string
}

func (r routingCompleter) Complete(_ context.Context, prompt string) (string, error) {
	switch {
	case strings.Contains(prompt, "classifier"):
		return r.classi, nil
	case strings.Contains(prompt, "which files a change will touch"):
		return r.impact, nil
	default:
		return r.plan, nil
	}
}

func TestTriageDeep_DecomposesAndLinks(t *testing.T) {
	store := newTestStore(t)
	ps := newFakePlanStore()
	svc := NewService(store, Options{
		Completer: routingCompleter{
			plan:   deepPlanJSON,
			classi: "kind: security\nrisk: high\n",
			impact: "files:\n  - path: src/login.ts\n    action: modify\n    reason: add validation\n",
		},
		PlanStore: ps,
	})
	ctx := context.Background()

	seedChange(t, store, &governance.ChangeRecord{
		ID: "C-1", Title: "Login broken", RawText: "users can't log in",
		SourceType: governance.SourceManual, SourceID: "C-1",
		Status: governance.ChangeStatusCandidate,
	})

	// Non-empty repo context triggers the impact analysis.
	res, err := svc.TriageDeep(ctx, "C-1", "== src/login.ts ==\ncode", "planner_ai")
	if err != nil {
		t.Fatalf("TriageDeep: %v", err)
	}
	if len(res.StoryIDs) != 1 || res.StoryIDs[0] != "US-001" {
		t.Fatalf("expected [US-001], got %+v", res.StoryIDs)
	}
	// Stories/tasks were persisted into the plan store.
	if ps.GetStory("US-001") == nil {
		t.Fatalf("story US-001 not persisted")
	}
	if ps.GetTask("T-US-001-001") == nil {
		t.Fatalf("task T-US-001-001 not persisted")
	}
	// GOVERNANCE HOLD: triaged tasks are created hitl (held) so the ungoverned
	// runtime scheduler cannot auto-build unapproved work.
	if got := ps.GetTask("T-US-001-001").ExecutionMode(); got != runtime.TaskModeHITL {
		t.Fatalf("triaged task must be held (hitl) until approved+executed, got mode %q", got)
	}
	// Regression: the story's task list must contain each task exactly once
	// (CreateTask appends; the bridge must not also pre-populate).
	if got := ps.GetStory("US-001").Tasks; len(got) != 1 || got[0] != "T-US-001-001" {
		t.Fatalf("expected story tasks [T-US-001-001] exactly once, got %+v", got)
	}
	// The change now OWNS the story via the link.
	linked, _ := store.ListChangeStories(ctx, "C-1")
	if len(linked) != 1 || linked[0] != "US-001" {
		t.Fatalf("expected change linked to US-001, got %+v", linked)
	}
	// The change was updated so the approval gates operate on the real plan.
	ch, _ := store.GetChangeRecord(ctx, "C-1")
	if ch.Status != governance.ChangeStatusPlanReady {
		t.Fatalf("expected plan_ready, got %q", ch.Status)
	}
	if !strings.Contains(ch.AcceptanceCriteria, "invalid credentials") {
		t.Fatalf("acceptance not aggregated onto change: %q", ch.AcceptanceCriteria)
	}
	if ch.ProposalVersion != 1 || ch.ApprovedVersion != 0 {
		t.Fatalf("expected version 1 / approved 0, got %d / %d", ch.ProposalVersion, ch.ApprovedVersion)
	}
	if !strings.Contains(ch.Plan, "US-001") {
		t.Fatalf("full plan JSON not stored on change")
	}
	// Risk + kind come from the classifier, not a flat default.
	if ch.Risk != governance.RiskHigh {
		t.Fatalf("expected classified risk high, got %q", ch.Risk)
	}
	if ch.Kind != governance.KindSecurity {
		t.Fatalf("expected classified kind security, got %q", ch.Kind)
	}
	// File-level impact was analyzed and stored (repo context was provided).
	imp, err := svc.ChangeImpact(ctx, "C-1")
	if err != nil {
		t.Fatalf("ChangeImpact: %v", err)
	}
	if imp == nil || len(imp.Files) != 1 || imp.Files[0].Path != "src/login.ts" {
		t.Fatalf("expected impact with src/login.ts, got %+v", imp)
	}
}

func TestTriageDeep_RiskFallsBackConservatively(t *testing.T) {
	store := newTestStore(t)
	ps := newFakePlanStore()
	// Classifier returns garbage; risk must clamp to medium (never a low that
	// would be auto-approvable).
	svc := NewService(store, Options{
		Completer: routingCompleter{plan: deepPlanJSON, classi: "not valid yaml at all"},
		PlanStore: ps,
	})
	ctx := context.Background()
	seedChange(t, store, &governance.ChangeRecord{
		ID: "C-9", Title: "x", RawText: "y",
		SourceType: governance.SourceManual, SourceID: "C-9",
		Status: governance.ChangeStatusCandidate,
	})
	if _, err := svc.TriageDeep(ctx, "C-9", "", "planner_ai"); err != nil {
		t.Fatalf("TriageDeep: %v", err)
	}
	ch, _ := store.GetChangeRecord(ctx, "C-9")
	if ch.Risk != governance.RiskMedium {
		t.Fatalf("expected conservative medium on garbled classification, got %q", ch.Risk)
	}
}

func TestTriageDeep_RequiresPlanStore(t *testing.T) {
	store := newTestStore(t)
	svc := NewService(store, Options{Completer: fakeCompleter{reply: deepPlanJSON}}) // no PlanStore
	ctx := context.Background()
	seedChange(t, store, &governance.ChangeRecord{ID: "C-1", SourceType: governance.SourceManual, SourceID: "C-1", Status: governance.ChangeStatusCandidate})
	if _, err := svc.TriageDeep(ctx, "C-1", "", "x"); !errors.Is(err, errMissingPlanStore) {
		t.Fatalf("expected errMissingPlanStore, got %v", err)
	}
}
