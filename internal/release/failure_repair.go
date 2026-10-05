package release

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
)

// CreateFailureRepair closes a failed task and creates the fix task that runs
// next. Evidence is an explicit caller-supplied record identity, not a
// classification inferred from an error message. It grants no effects.
//
// The failed task is closed, not reopened: it stays failed with fixed_by
// naming its fix, and is never run again. The fix task delivers only the fix:
// it carries the original task's verification check, and its description
// (the caller's diagnosis) holds the original task, its code and the failure.
// A fix that fails is closed the same way and the next fix follows, until the
// original task's attempt budget is spent. A closed task is never changed
// again: work that waited on it runs once its fix chain ends in a passed fix
// (Delivered), and the chain stays in the ledger as it happened.
//
// Reopening the failed task and re-running it after a repair made every
// failure a long loop: the whole task again, with a reviewer free to widen it.
// A fix task is small, and its review judges the original task, its code and
// the fix together, so it cannot move out of scope (owner model, 2026-10-05).
func (s *SQLiteStore) CreateFailureRepair(ctx context.Context, taskID, evidenceID, diagnosis string) (*Task, error) {
	taskID, evidenceID, diagnosis = strings.TrimSpace(taskID), strings.TrimSpace(evidenceID), strings.TrimSpace(diagnosis)
	if taskID == "" || evidenceID == "" || diagnosis == "" {
		return nil, ErrInvalidData
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	digest := sha256.Sum256([]byte(taskID + "\x00" + evidenceID))
	id := "repair-" + hex.EncodeToString(digest[:16])
	var existing string
	err = tx.QueryRowContext(ctx, `SELECT metadata FROM tasks WHERE id=?`, id).Scan(&existing)
	if err == nil {
		var m map[string]interface{}
		if json.Unmarshal([]byte(existing), &m) != nil || m["fix_of"] != taskID || m["failure_evidence"] != evidenceID || m["diagnosis"] != diagnosis {
			return nil, fmt.Errorf("failure evidence already has a different repair disposition")
		}
		if err := tx.Commit(); err != nil {
			return nil, err
		}
		return s.getTaskInternal(ctx, id)
	}
	if err != sql.ErrNoRows {
		return nil, err
	}
	var storyID, status, metadata, branch, prURL string
	var prNumber sql.NullInt64
	var needsReview int
	err = tx.QueryRowContext(ctx, `SELECT story_id,status,metadata,git_branch,git_pr_number,git_pr_url,needs_review FROM tasks WHERE id=?`, taskID).Scan(&storyID, &status, &metadata, &branch, &prNumber, &prURL, &needsReview)
	if err == sql.ErrNoRows {
		return nil, ErrTaskNotFound
	}
	if err != nil {
		return nil, err
	}
	// A failed task is closed by its fix. A done task stays exactly as it
	// was: a finding on delivered work (review, gate, hosted check) is a new
	// fix task that names it, never a status change of finished work.
	if status != TaskStatusFailed && status != TaskStatusDone {
		return nil, fmt.Errorf("repair requires a persisted failed or done task, not %s", status)
	}
	var failedMetadata map[string]interface{}
	if err := json.Unmarshal([]byte(metadata), &failedMetadata); err != nil {
		return nil, err
	}
	if fixedBy, _ := failedMetadata["fixed_by"].(string); fixedBy != "" {
		return nil, fmt.Errorf("task %s is already closed with fix %s", taskID, fixedBy)
	}
	// A failed fix is fixed for the same original task: the chain shares the
	// original's check, scope and attempt budget.
	rootID := taskID
	if of, _ := failedMetadata["repair_of"].(string); of != "" {
		rootID = of
	}
	var rootStory, rootStatus, rootTitle, rootDeps, rootMetadata, verification string
	var rootAttempts, rootMax, priority int
	if err := tx.QueryRowContext(ctx, `SELECT story_id,status,title,depends_on,metadata,COALESCE(verification_script,''),attempt_count,max_attempts,priority FROM tasks WHERE id=?`, rootID).Scan(&rootStory, &rootStatus, &rootTitle, &rootDeps, &rootMetadata, &verification, &rootAttempts, &rootMax, &priority); err != nil {
		return nil, fmt.Errorf("original task %s of the failed fix: %w", rootID, err)
	}
	if rootStory != storyID {
		return nil, fmt.Errorf("fix %s and its original task %s are in different stories", taskID, rootID)
	}
	var priorFixes int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM tasks WHERE json_extract(metadata,'$.repair_of')=?`, rootID).Scan(&priorFixes); err != nil {
		return nil, err
	}
	// The budget bounds a failure chain: fixes that keep failing. Findings
	// on delivered work each carry their own evidence and are not retries.
	if status == TaskStatusFailed && (rootMax <= 0 || priorFixes >= rootMax) {
		return nil, fmt.Errorf("task repair attempt limit reached")
	}
	var rootMeta map[string]interface{}
	if err := json.Unmarshal([]byte(rootMetadata), &rootMeta); err != nil {
		return nil, err
	}
	var storyTasks, storyBranch, storyStatus string
	if err := tx.QueryRowContext(ctx, `SELECT tasks,git_branch,status FROM stories WHERE id=?`, storyID).Scan(&storyTasks, &storyBranch, &storyStatus); err != nil {
		return nil, err
	}
	if storyStatus == StoryStatusDone {
		return nil, fmt.Errorf("completed story cannot be reopened by repair")
	}
	if branch == "" {
		branch = storyBranch
	}
	if branch != "" && storyBranch != "" && branch != storyBranch {
		return nil, fmt.Errorf("task and story branch disagree; reconcile retained candidate")
	}
	var tasks []string
	if err := json.Unmarshal([]byte(storyTasks), &tasks); err != nil {
		return nil, err
	}
	mode := (&Task{Metadata: rootMeta}).ExecutionMode()
	repairFields := map[string]interface{}{"repair_of": rootID, "fix_of": taskID, "failure_evidence": evidenceID, "failure_kind": FailureKind(evidenceID), "diagnosis": diagnosis, "mode": mode}
	for _, key := range []string{"decision_reason", "decision_ref"} {
		if value, ok := rootMeta[key]; ok {
			repairFields[key] = value
		}
	}
	repairMetadata, _ := json.Marshal(repairFields)
	now := time.Now().UTC().Format(time.RFC3339)
	// The fix needs what its original needed, and it must pass the original's
	// own check. Two attempts: one more when a provider stops it, never a
	// second fix of the same failure (that is the next fix task).
	_, err = tx.ExecContext(ctx, `INSERT INTO tasks(id,story_id,title,description,verification_script,depends_on,task_type,priority,max_attempts,git_branch,git_pr_number,git_pr_url,needs_review,status,created_at,metadata) VALUES(?,?,?,?,?,?,'fix',?,2,?,?,?,?,'pending',?,?)`,
		id, storyID, "Fix: "+rootTitle, diagnosis, verification, rootDeps, priority, branch, prNumber, prURL, needsReview, now, string(repairMetadata))
	if err != nil {
		return nil, err
	}
	if status == TaskStatusFailed {
		// Closing the failed task: the one record written as it closes.
		failedMetadata["fixed_by"] = id
		closed, _ := json.Marshal(failedMetadata)
		if _, err := tx.ExecContext(ctx, `UPDATE tasks SET metadata=? WHERE id=? AND status='failed'`, string(closed), taskID); err != nil {
			return nil, err
		}
	}
	tasks = append(tasks, id)
	tasksJSON, _ := json.Marshal(tasks)
	if _, err := tx.ExecContext(ctx, `UPDATE stories SET tasks=? WHERE id=?`, string(tasksJSON), storyID); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return s.getTaskInternal(ctx, id)
}

// Evidence id prefixes name what kind of failure a fix task answers.
const (
	ProviderStopEvidence = "provider-stop-"
	FindingEvidence      = "console-repair:"
)

// FailureKind is how a fix task's failure is counted: "provider" when the
// provider or a stage stopped the attempt, "finding" when a review, gate or
// verdict found a defect in work that ran, "check" when the task failed its
// own verification check.
func FailureKind(evidenceID string) string {
	switch {
	case strings.HasPrefix(evidenceID, ProviderStopEvidence):
		return "provider"
	case strings.HasPrefix(evidenceID, FindingEvidence):
		return "finding"
	default:
		return "check"
	}
}

// Delivered reports whether a task's work is delivered: done (or approved),
// or closed after a failure with a fix chain that ends in a passed fix. A
// closed task is never rewritten; what it delivered is read through its chain,
// so the ledger keeps every failure and every fix as it happened.
func Delivered(task *Task, byID map[string]*Task) bool {
	for steps := 0; task != nil && steps <= 64; steps++ {
		switch task.Status {
		case TaskStatusDone, TaskStatusApproved:
			return true
		case TaskStatusFailed:
			next, _ := task.Metadata["fixed_by"].(string)
			if next == "" {
				return false
			}
			task = byID[next]
		default:
			return false
		}
	}
	return false
}

// ClosedWithFix reports a failed task closed with a fix: its open work, if
// any, is that fix, not the task itself.
func ClosedWithFix(task *Task) bool {
	next, _ := task.Metadata["fixed_by"].(string)
	return task.Status == TaskStatusFailed && next != ""
}

// RunnableTasks re-reads the authoritative ledger each time, so newly created
// repairs participate without rebuilding a manager's in-memory task list.
// This is selection only; execution still needs its ordinary claim/effect gates.
func RunnableTasks(ctx context.Context, store Store, storyIDs []string) ([]*Task, error) {
	return runnableTasks(ctx, store, storyIDs, "")
}

func runnableTasks(ctx context.Context, store Store, storyIDs []string, recaptureID string) ([]*Task, error) {
	scope := map[string]bool{}
	for _, id := range storyIDs {
		scope[id] = true
	}
	if len(scope) == 0 {
		return nil, nil
	}
	tasks, err := store.ListTasks(ctx)
	if err != nil {
		return nil, err
	}
	stories, err := store.ListStories(ctx)
	if err != nil {
		return nil, err
	}
	taskByID := map[string]*Task{}
	storyByID := map[string]*Story{}
	for _, task := range tasks {
		if task.ID == recaptureID && task.Status == TaskStatusFailed {
			task.Status = TaskStatusPending
		}
		taskByID[task.ID] = task
	}
	for _, story := range stories {
		storyByID[story.ID] = story
	}
	var ready []*Task
	for _, task := range tasks {
		if task.Status != TaskStatusPending || !scope[task.StoryID] || task.ExecutionMode() == TaskModeHITL || task.MaxAttempts <= 0 || task.AttemptCount >= task.MaxAttempts {
			continue
		}
		story := storyByID[task.StoryID]
		if story == nil || story.Status == StoryStatusDone {
			continue
		}
		ok := true
		for _, id := range story.DependsOn {
			dep := storyByID[id]
			complete := dep != nil
			count := 0
			for _, prerequisite := range tasks {
				if prerequisite.StoryID == id {
					count++
					complete = complete && Delivered(prerequisite, taskByID)
				}
			}
			ok = ok && complete && count > 0
		}
		for _, id := range task.DependsOn {
			dep := taskByID[id]
			ok = ok && dep != nil && Delivered(dep, taskByID)
		}
		if ok {
			ready = append(ready, task)
		}
	}
	sort.Slice(ready, func(i, j int) bool {
		// A fix runs next: it is the smallest open work, and what waits on
		// its original task waits on it.
		isRepair := func(task *Task) bool {
			id, _ := task.Metadata["repair_of"].(string)
			evidence, _ := task.Metadata["failure_evidence"].(string)
			original := taskByID[id]
			return original != nil && evidence != "" && original.StoryID == task.StoryID
		}
		if isRepair(ready[i]) != isRepair(ready[j]) {
			return isRepair(ready[i])
		}
		if ready[i].Priority != ready[j].Priority {
			return ready[i].Priority < ready[j].Priority
		}
		if !ready[i].CreatedAt.Equal(ready[j].CreatedAt) {
			return ready[i].CreatedAt.Before(ready[j].CreatedAt)
		}
		return ready[i].ID < ready[j].ID
	})
	return ready, nil
}

func (m *Manager) RunnableTasks(ctx context.Context, storyIDs []string) ([]*Task, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return RunnableTasks(ctx, m.store, storyIDs)
}

// TaskSnapshot reads one current task without reloading or replacing the rest
// of the backlog while an executor owns work.
func (m *Manager) TaskSnapshot(ctx context.Context, id string) (*Task, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.store.GetTask(ctx, id)
}

func (m *Manager) TasksInStories(ctx context.Context, storyIDs []string) ([]*Task, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var out []*Task
	for _, id := range storyIDs {
		if _, err := m.store.GetStory(ctx, id); err != nil {
			return nil, err
		}
		tasks, err := m.store.ListTasksByStory(ctx, id)
		if err != nil {
			return nil, err
		}
		out = append(out, tasks...)
	}
	return out, nil
}

func (m *Manager) CreateFailureRepair(ctx context.Context, taskID, evidenceID, diagnosis string) (*Task, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	store, ok := m.store.(interface {
		CreateFailureRepair(context.Context, string, string, string) (*Task, error)
	})
	if !ok {
		return nil, fmt.Errorf("backlog store lacks atomic failure repair support")
	}
	repair, err := store.CreateFailureRepair(ctx, taskID, evidenceID, diagnosis)
	if err != nil {
		return nil, err
	}
	original, err := m.store.GetTask(ctx, taskID)
	if err != nil {
		return nil, err
	}
	story, err := m.store.GetStory(ctx, repair.StoryID)
	if err != nil {
		return nil, err
	}
	m.tasks[original.ID], m.tasks[repair.ID], m.stories[story.ID] = original, repair, story
	return repair, nil
}

// RecaptureEligible applies ordinary selection without changing durable status.
func (m *Manager) RecaptureEligible(ctx context.Context, task *Task) (bool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	tasks, err := runnableTasks(ctx, m.store, []string{task.StoryID}, task.ID)
	if err != nil {
		return false, err
	}
	for _, next := range tasks {
		if next.ID == task.ID {
			return true, nil
		}
	}
	return false, nil
}
