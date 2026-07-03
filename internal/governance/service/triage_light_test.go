package service

import (
	"context"
	"strings"
	"testing"

	"github.com/openexec/openexec/internal/governance"
	"github.com/openexec/openexec/pkg/runtime"
)

// planStoreStub is a minimal in-memory PlanStore for light-lane tests. CreateTask
// appends the task id to its parent story (mirroring the real backlog manager),
// so changeTaskIDs resolves the linked story's tasks.
type planStoreStub struct {
	goals   map[string]*runtime.Goal
	stories map[string]*runtime.Story
	tasks   map[string]*runtime.Task
}

func newPlanStoreStub() *planStoreStub {
	return &planStoreStub{
		goals:   map[string]*runtime.Goal{},
		stories: map[string]*runtime.Story{},
		tasks:   map[string]*runtime.Task{},
	}
}

func (p *planStoreStub) GetGoal(id string) *runtime.Goal   { return p.goals[id] }
func (p *planStoreStub) GetStory(id string) *runtime.Story { return p.stories[id] }
func (p *planStoreStub) GetTask(id string) *runtime.Task   { return p.tasks[id] }
func (p *planStoreStub) CreateGoal(g *runtime.Goal) error  { p.goals[g.ID] = g; return nil }
func (p *planStoreStub) CreateStory(s *runtime.Story) error {
	p.stories[s.ID] = s
	return nil
}
func (p *planStoreStub) CreateTask(t *runtime.Task) error {
	p.tasks[t.ID] = t
	if st := p.stories[t.StoryID]; st != nil {
		st.Tasks = append(st.Tasks, t.ID)
	}
	return nil
}
func (p *planStoreStub) UpdateTask(t *runtime.Task) error { p.tasks[t.ID] = t; return nil }

func TestTriageLight_SingleTaskAndOperatorApprovalWaivesReview(t *testing.T) {
	store := newTestStore(t)
	ps := newPlanStoreStub()
	// No completer => risk defaults to low; operator session so approval is allowed.
	svc := NewService(store, Options{PlanStore: ps, OperatorSession: true})
	ctx := context.Background()

	seedChange(t, store, &governance.ChangeRecord{
		ID: "C-1", Status: governance.ChangeStatusCandidate,
		Title: "Bump a constant", RawText: "Change MAX to 40.",
	})

	res, err := svc.TriageLight(ctx, "C-1", "operator")
	if err != nil {
		t.Fatalf("TriageLight: %v", err)
	}
	if len(res.StoryIDs) != 1 {
		t.Fatalf("expected 1 story, got %d", len(res.StoryIDs))
	}
	ch, _ := store.GetChangeRecord(ctx, "C-1")
	if !ch.Light {
		t.Fatalf("change should be marked light")
	}
	if ch.Status != governance.ChangeStatusPlanReady {
		t.Fatalf("expected plan_ready, got %s", ch.Status)
	}

	// Operator approval must succeed WITHOUT any AI review on record (waived).
	if err := svc.ApproveChange(ctx, "C-1", "pm"); err != nil {
		t.Fatalf("light-lane operator approval should succeed without AI review, got %v", err)
	}
	ch, _ = store.GetChangeRecord(ctx, "C-1")
	if ch.Status != governance.ChangeStatusApprovedForAI {
		t.Fatalf("expected approved_for_ai, got %s", ch.Status)
	}
	// The waiver must be visible in the audit trail.
	events, _, _ := svc.History(ctx, "C-1")
	var sawWaiver bool
	for _, e := range events {
		if e.Decision == governance.DecisionApproved && strings.Contains(e.Comment, "AI review waived") {
			sawWaiver = true
		}
	}
	if !sawWaiver {
		t.Fatalf("approval event should record the AI-review waiver")
	}
}

func TestTriageLight_RefusesHighRisk(t *testing.T) {
	store := newTestStore(t)
	ps := newPlanStoreStub()
	// Completer classifies the change as critical → light lane must refuse.
	svc := NewService(store, Options{
		PlanStore: ps,
		Completer: fakeCompleter{reply: "kind: security\nrisk: critical\n"},
	})
	ctx := context.Background()
	seedChange(t, store, &governance.ChangeRecord{
		ID: "C-1", Status: governance.ChangeStatusCandidate,
		Title: "Rework auth", RawText: "Replace the auth system.",
	})

	if _, err := svc.TriageLight(ctx, "C-1", "operator"); err == nil || !strings.Contains(err.Error(), "lightweight lane is only for trivial") {
		t.Fatalf("expected high-risk refusal, got %v", err)
	}
}
