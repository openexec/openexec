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

// CreateFailureRepair converts an observed failed attempt into one prerequisite
// on the same story. Evidence is an explicit caller-supplied record identity,
// not a classification inferred from an error message. It grants no effects.
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
		if json.Unmarshal([]byte(existing), &m) != nil || m["repair_of"] != taskID || m["failure_evidence"] != evidenceID || m["diagnosis"] != diagnosis {
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
	var storyID, status, title, dependencies, metadata, branch, prURL string
	var attempts, maxAttempts, priority, needsReview int
	var prNumber sql.NullInt64
	err = tx.QueryRowContext(ctx, `SELECT story_id,status,title,depends_on,metadata,git_branch,git_pr_number,git_pr_url,attempt_count,max_attempts,priority,needs_review FROM tasks WHERE id=?`, taskID).Scan(&storyID, &status, &title, &dependencies, &metadata, &branch, &prNumber, &prURL, &attempts, &maxAttempts, &priority, &needsReview)
	if err == sql.ErrNoRows {
		return nil, ErrTaskNotFound
	}
	if err != nil {
		return nil, err
	}
	if status != TaskStatusFailed {
		return nil, fmt.Errorf("repair requires a persisted failed task, not %s", status)
	}
	var originalMetadata map[string]interface{}
	var deps []string
	if err := json.Unmarshal([]byte(metadata), &originalMetadata); err != nil {
		return nil, err
	}
	if err := json.Unmarshal([]byte(dependencies), &deps); err != nil {
		return nil, err
	}
	if originalMetadata["repair_of"] != nil {
		return nil, fmt.Errorf("repair task must use its own bounded attempts; recursive repair creation is not authorized")
	}
	var priorRepairs int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM tasks WHERE json_extract(metadata,'$.repair_of')=?`, taskID).Scan(&priorRepairs); err != nil {
		return nil, err
	}
	if maxAttempts <= 0 || attempts >= maxAttempts || priorRepairs >= maxAttempts {
		return nil, fmt.Errorf("task repair attempt limit reached")
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
	mode := (&Task{Metadata: originalMetadata}).ExecutionMode()
	repairMetadata, _ := json.Marshal(map[string]interface{}{"repair_of": taskID, "failure_evidence": evidenceID, "diagnosis": diagnosis, "mode": mode})
	now := time.Now().UTC().Format(time.RFC3339)
	_, err = tx.ExecContext(ctx, `INSERT INTO tasks(id,story_id,title,description,depends_on,task_type,priority,max_attempts,git_branch,git_pr_number,git_pr_url,needs_review,status,created_at,metadata) VALUES(?,?,?,?,?,'fix',?,?,?,?,?,?,'pending',?,?)`, id, storyID, "Repair: "+title, diagnosis, dependencies, priority, maxAttempts-attempts, branch, prNumber, prURL, needsReview, now, string(repairMetadata))
	if err != nil {
		return nil, err
	}
	deps = append(deps, id)
	depsJSON, _ := json.Marshal(deps)
	if _, err := tx.ExecContext(ctx, `UPDATE tasks SET status='pending', depends_on=? WHERE id=? AND status='failed'`, string(depsJSON), taskID); err != nil {
		return nil, err
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

// RunnableTasks re-reads the authoritative ledger each time, so newly created
// repairs participate without rebuilding a manager's in-memory task list.
// This is selection only; execution still needs its ordinary claim/effect gates.
func RunnableTasks(ctx context.Context, store Store, storyIDs []string) ([]*Task, error) {
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
					complete = complete && prerequisite.Status == TaskStatusDone
				}
			}
			ok = ok && complete && count > 0
		}
		for _, id := range task.DependsOn {
			dep := taskByID[id]
			ok = ok && dep != nil && dep.Status == TaskStatusDone
		}
		if ok {
			ready = append(ready, task)
		}
	}
	sort.Slice(ready, func(i, j int) bool {
		isRepair := func(task *Task) bool {
			id, _ := task.Metadata["repair_of"].(string)
			evidence, _ := task.Metadata["failure_evidence"].(string)
			original := taskByID[id]
			if original == nil || evidence == "" || original.StoryID != task.StoryID {
				return false
			}
			for _, dependency := range original.DependsOn {
				if dependency == task.ID {
					return true
				}
			}
			return false
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
