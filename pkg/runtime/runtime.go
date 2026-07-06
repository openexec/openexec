// Package runtime is the PUBLIC seam between the open-source OpenExec runtime and
// a higher-layer product built on top of it. It re-exports exactly the runtime
// capabilities a higher layer needs — intent planning and the story/task
// backlog types — through a stable public API, so a higher layer can
// depend on this package instead of reaching into internal/planner and
// internal/release.
//
// This is what makes the two-layer split real: the runtime (this module's
// internals) stays MIT and self-contained, while a higher layer — which may
// live in a separate module or be licensed differently — consumes only this
// facade. Keep this surface small and stable; it is the contract.
package runtime

import (
	"context"

	"github.com/openexec/openexec/internal/planner"
	"github.com/openexec/openexec/internal/release"
)

// Backlog types. These aliases expose the runtime's story/task/goal types under
// stable public names so external code can construct and read them without
// importing internal packages.
type (
	Story = release.Story
	Task  = release.Task
	Goal  = release.Goal
)

// Backlog constants used when persisting planner output.
const (
	StoryTypeFeature   = release.StoryTypeFeature
	StoryStatusPending = release.StoryStatusPending
	TaskStatusPending  = release.TaskStatusPending
	// TaskModeHITL marks a release task as human-in-the-loop; the runtime
	// scheduler never auto-dispatches it (and holds its dependents). The
	// a higher layer can use it as a "held until approved" gate.
	TaskModeHITL = release.TaskModeHITL
	// TaskModeAFK marks a task the runtime may auto-run.
	TaskModeAFK = release.TaskModeAFK
)

// Planning types.
type (
	// ProjectPlan is a generated intent decomposition (goals -> stories -> tasks).
	ProjectPlan = planner.ProjectPlan
	// PlanGoal, PlanStory, and PlanTask are the elements of a ProjectPlan (the
	// planner's own types, distinct from the release backlog's Goal/Story/Task).
	// Exposed so callers can hand-build a small plan without importing the
	// internal planner package.
	PlanGoal  = planner.Goal
	PlanStory = planner.Story
	PlanTask  = planner.Task
	// ExistingLookup lets RemapPlanIDs detect id collisions with an existing backlog.
	ExistingLookup = planner.ExistingLookup
	// LLMProvider is the single-shot completion interface the planner needs; any
	// value with Complete(ctx, prompt) (string, error) satisfies it.
	LLMProvider = planner.LLMProvider
)

// PlanTaskModeHITL is the planner's human-in-the-loop task mode (distinct from
// the release backlog's TaskModeHITL).
const PlanTaskModeHITL = planner.TaskModeHITL

// Planner wraps the runtime planner behind the public seam.
type Planner struct{ inner *planner.Planner }

// NewPlanner builds a Planner over an LLM provider.
func NewPlanner(p LLMProvider) *Planner { return &Planner{inner: planner.New(p)} }

// GeneratePlan turns intent text into a ProjectPlan.
func (p *Planner) GeneratePlan(ctx context.Context, intent string) (*ProjectPlan, error) {
	return p.inner.GeneratePlan(ctx, intent, nil)
}

// GenerateCompactPlan turns intent text into a single-story plan (1-3 tasks,
// no Study story, no terminus) for changes the caller has already sized as
// small. GeneratePlan remains the full-shape path.
func (p *Planner) GenerateCompactPlan(ctx context.Context, intent string) (*ProjectPlan, error) {
	return p.inner.GenerateCompactPlan(ctx, intent)
}

// LintPlanVerification flags false-green verification scripts in a plan
// (patterns that report success even when the real check failed), keyed by the
// owning story/task id — surfaced for human review before approval.
func LintPlanVerification(plan *ProjectPlan) map[string][]string {
	return planner.LintPlanVerification(plan)
}

// RemapPlanIDs re-numbers colliding ids so a generated plan appends to an
// existing backlog instead of clashing with it. Returns the count remapped.
func RemapPlanIDs(plan *ProjectPlan, look ExistingLookup) int {
	return planner.RemapPlanIDs(plan, look)
}

// BacklogManager is the runtime's story/task backlog manager (persists to
// .openexec/openexec.db). It satisfies a higher layer's PlanStore interface.
type BacklogManager = release.Manager

// NewBacklogManager opens the backlog manager for a project directory with git
// integration disabled — appropriate for higher-layer persistence, where
// branch/PR side effects belong to execution, not planning.
func NewBacklogManager(baseDir string) (*BacklogManager, error) {
	return release.NewManager(baseDir, nil)
}
