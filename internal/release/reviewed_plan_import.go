package release

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
)

// ImportReviewedPlan atomically inserts an exact reviewed decomposition and its
// import receipt into existing tables. Existing lifecycle/candidate data is
// never rewritten. The caller has already persisted the reviewed plan.
func (m *Manager) ImportReviewedPlan(ctx context.Context, goals []*Goal, stories []*Story, tasks []*Task, stepID, runID, inputDigest, receipt string) error {
	return m.importReviewedPlan(ctx, goals, stories, tasks, stepID, runID, inputDigest, receipt, false)
}

// ValidatePlanIdentities runs the same exact-content and constraint checks in
// a rollback-only transaction. It creates no durable work or review receipt.
func (m *Manager) ValidatePlanIdentities(ctx context.Context, goals []*Goal, stories []*Story, tasks []*Task) error {
	return m.importReviewedPlan(ctx, goals, stories, tasks, "", "", "", "", true)
}

func (m *Manager) importReviewedPlan(ctx context.Context, goals []*Goal, stories []*Story, tasks []*Task, stepID, runID, inputDigest, receipt string, validateOnly bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.store.(*SQLiteStore)
	if !ok {
		return fmt.Errorf("reviewed import requires SQLite")
	}
	s.mu.Lock()
	locked := true
	defer func() {
		if locked {
			s.mu.Unlock()
		}
	}()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var existingDigest, existingReceipt, existingRun, existingPhase, existingAgent, existingStatus string
	err = sql.ErrNoRows
	if !validateOnly {
		err = tx.QueryRowContext(ctx, `SELECT inputs_hash,metadata,run_id,phase,agent,status FROM run_steps WHERE id=?`, stepID).Scan(&existingDigest, &existingReceipt, &existingRun, &existingPhase, &existingAgent, &existingStatus)
	}
	if err == nil {
		if existingDigest != inputDigest || existingReceipt != receipt || existingRun != runID || existingPhase != "plan" || existingAgent != "reviewed-plan-import" || existingStatus != "completed" {
			return fmt.Errorf("reviewed import receipt conflicts")
		}
		return nil
	}
	if err != sql.ErrNoRows {
		return err
	}
	insert := func(table, id string, columns []string, values []any, jsonFields map[string]bool) error {
		var n int
		if err := tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+table+" WHERE id=?", id).Scan(&n); err != nil {
			return err
		}
		if n != 0 {
			predicates := []string{"id=?"}
			args := []any{id}
			for i, column := range columns {
				if jsonFields[column] {
					predicates = append(predicates, "json("+column+")=json(?)")
				} else {
					predicates = append(predicates, "COALESCE("+column+",'')=?")
				}
				args = append(args, values[i])
			}
			if err := tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+table+" WHERE "+strings.Join(predicates, " AND "), args...).Scan(&n); err != nil {
				return err
			}
			if n != 1 {
				return fmt.Errorf("reviewed %s %s conflicts with retained content", table, id)
			}
			return nil
		}
		args := append([]any{id}, values...)
		marks := strings.TrimSuffix(strings.Repeat("?,", len(args)), ",")
		_, err := tx.ExecContext(ctx, "INSERT INTO "+table+" (id,"+strings.Join(columns, ",")+") VALUES ("+marks+")", args...)
		return err
	}
	array := func(v []string) string {
		if v == nil {
			v = []string{}
		}
		raw, _ := json.Marshal(v)
		return string(raw)
	}
	for _, g := range goals {
		if err := insert("goals", g.ID, []string{"title", "description", "success_criteria", "verification_method"}, []any{g.Title, g.Description, g.SuccessCriteria, g.VerificationMethod}, nil); err != nil {
			return err
		}
	}
	for _, s := range stories {
		if err := insert("stories", s.ID, []string{"goal_id", "title", "description", "acceptance_criteria", "verification_script", "contract", "depends_on", "story_type", "priority", "tasks"}, []any{s.GoalID, s.Title, s.Description, array(s.AcceptanceCriteria), s.VerificationScript, s.Contract, array(s.DependsOn), s.StoryType, s.Priority, array(s.Tasks)}, map[string]bool{"acceptance_criteria": true, "depends_on": true, "tasks": true}); err != nil {
			return err
		}
	}
	for _, t := range tasks {
		// Mode is a routing constraint. Extra existing metadata remains untouched.
		mode := TaskModeAFK
		if v, ok := t.Metadata["mode"].(string); ok {
			mode = v
		}
		var metadata string
		err := tx.QueryRowContext(ctx, `SELECT metadata FROM tasks WHERE id=?`, t.ID).Scan(&metadata)
		if err == nil {
			var retained map[string]any
			if json.Unmarshal([]byte(metadata), &retained) != nil {
				return fmt.Errorf("invalid retained task metadata")
			}
			oldMode := TaskModeAFK
			if v, ok := retained["mode"].(string); ok {
				oldMode = v
			}
			if oldMode != mode {
				return fmt.Errorf("reviewed task %s mode conflicts", t.ID)
			}
		} else if err != sql.ErrNoRows {
			return err
		}
		if err := insert("tasks", t.ID, []string{"story_id", "title", "description", "verification_script", "depends_on", "priority", "max_attempts"}, []any{t.StoryID, t.Title, t.Description, t.VerificationScript, array(t.DependsOn), t.Priority, t.MaxAttempts}, map[string]bool{"depends_on": true}); err != nil {
			return err
		}
		if metadata == "" {
			raw, _ := json.Marshal(t.Metadata)
			if string(raw) == "null" {
				raw = []byte("{}")
			}
			if _, err := tx.ExecContext(ctx, `UPDATE tasks SET metadata=? WHERE id=?`, string(raw), t.ID); err != nil {
				return err
			}
		}
	}
	if validateOnly {
		return nil
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO run_steps(id,run_id,phase,agent,iteration,status,inputs_hash,metadata) VALUES(?,?,'plan','reviewed-plan-import',0,'completed',?,?)`, stepID, runID, inputDigest, receipt); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	s.mu.Unlock()
	locked = false
	return m.refreshCacheUnlocked(ctx)
}
