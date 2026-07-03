package service

import (
	"context"
	"fmt"
	"time"
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
