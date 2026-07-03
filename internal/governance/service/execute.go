package service

import (
	"context"
	"fmt"
	"time"

	"github.com/openexec/openexec/internal/governance"
)

// executeLease is the claim lease taken while a change's tasks are executed.
const executeLease = 2 * time.Hour

// ExecuteReport summarizes an ExecuteChange run.
type ExecuteReport struct {
	ChangeID        string
	DispatchedTasks []string
	Failures        map[string]string // taskID -> error message
}

// ExecuteChange runs the approved work for a change through the execution engine.
// This is the "build it for me" hop: it claims the change (which enforces the
// governance gate — only approved work in an approved release can be claimed),
// advances it to implementing, gathers the task ids from the change's linked
// stories, and dispatches each through the injected Executor.
//
// It never runs unapproved work: ClaimWork applies ValidateImplementable, so an
// unapproved change (or one whose release is not approved) is refused before any
// task runs. Requires a PlanStore (to resolve tasks) and an Executor.
//
// V1 scope: it dispatches the tasks and reports outcomes; the change stays
// `implementing`. Linking the produced PR back to the change (implementing ->
// pr_open -> ready_for_test) is done via `work record-pr` / `ready-for-test`,
// because the executor runs in a separate process and the PR lands on the
// release task, not here.
func (s *Service) ExecuteChange(ctx context.Context, changeID, agent, mode string) (*ExecuteReport, error) {
	if s.executor == nil {
		return nil, errMissingExecutor
	}
	if s.planStore == nil {
		return nil, errMissingPlanStore
	}
	if agent == "" {
		agent = "openexec-executor"
	}

	// Claim enforces the governance gate (approved change + approved/implementing
	// release, not already claimed) and advances the change to implementing.
	if err := s.ClaimWork(ctx, changeID, agent, executeLease); err != nil {
		return nil, err
	}

	taskIDs, err := s.changeTaskIDs(ctx, changeID)
	if err != nil {
		return nil, err
	}
	if len(taskIDs) == 0 {
		return nil, fmt.Errorf("change %s has no tasks to execute (run deep triage first: work triage %s --deep)", changeID, changeID)
	}

	report := &ExecuteReport{ChangeID: changeID, Failures: map[string]string{}}
	for _, tid := range taskIDs {
		if err := s.executor.RunTask(ctx, tid, mode); err != nil {
			report.Failures[tid] = err.Error()
			continue
		}
		report.DispatchedTasks = append(report.DispatchedTasks, tid)
	}
	return report, nil
}

// ReleaseExecuteReport summarizes a batch ExecuteRelease run.
type ReleaseExecuteReport struct {
	ReleaseID string
	Executed  []*ExecuteReport  // per-change results
	Skipped   []string          // "changeID (status)" for non-approved items
	Errors    map[string]string // changeID -> error message
}

// ExecuteRelease runs a whole release's approved work in one go — the "implement
// all (or a selected subset) at once" batch. If only is non-empty it restricts
// execution to those change ids; otherwise every approved_for_ai change in the
// release runs. Non-approved changes are reported as skipped, never built.
//
// The release must be approved (it is advanced to implementing on first run).
// Each change goes through ExecuteChange, so the same governance gate applies
// per item. Errors on one change are collected and do not stop the batch.
func (s *Service) ExecuteRelease(ctx context.Context, releaseID string, only []string, agent, mode string) (*ReleaseExecuteReport, error) {
	if s.executor == nil {
		return nil, errMissingExecutor
	}
	rel, err := s.getRelease(ctx, releaseID)
	if err != nil {
		return nil, err
	}
	if !releaseExecutable(rel.Status) {
		return nil, fmt.Errorf("release %s is %q; approve it before executing", releaseID, rel.Status)
	}
	// Advance approved -> implementing once so the release reflects that work
	// has started (best-effort; a StartRelease error should not block the batch).
	_ = s.StartRelease(ctx, releaseID)

	changes, err := s.store.ListChangeRecordsByRelease(ctx, releaseID)
	if err != nil {
		return nil, fmt.Errorf("list changes for release %q: %w", releaseID, err)
	}
	onlySet := map[string]bool{}
	for _, id := range only {
		onlySet[id] = true
	}

	report := &ReleaseExecuteReport{ReleaseID: releaseID, Errors: map[string]string{}}
	for _, ch := range changes {
		if len(onlySet) > 0 && !onlySet[ch.ID] {
			continue
		}
		if !changeExecutable(ch.Status) {
			report.Skipped = append(report.Skipped, fmt.Sprintf("%s (%s)", ch.ID, ch.Status))
			continue
		}
		r, err := s.ExecuteChange(ctx, ch.ID, agent, mode)
		if err != nil {
			report.Errors[ch.ID] = err.Error()
			continue
		}
		report.Executed = append(report.Executed, r)
	}
	return report, nil
}

func releaseExecutable(status string) bool {
	return status == governance.ReleaseStatusApproved || status == governance.ReleaseStatusImplementing
}

func changeExecutable(status string) bool {
	return status == governance.ChangeStatusApprovedForAI || status == governance.ChangeStatusImplementing
}

// changeTaskIDs resolves the task ids belonging to a change, via its linked
// stories (each story carries its task ids). Order: stories oldest-first, tasks
// in story order.
func (s *Service) changeTaskIDs(ctx context.Context, changeID string) ([]string, error) {
	storyIDs, err := s.store.ListChangeStories(ctx, changeID)
	if err != nil {
		return nil, err
	}
	var taskIDs []string
	for _, sid := range storyIDs {
		if st := s.planStore.GetStory(sid); st != nil {
			taskIDs = append(taskIDs, st.Tasks...)
		}
	}
	return taskIDs, nil
}
