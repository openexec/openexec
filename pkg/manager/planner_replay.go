package manager

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/openexec/openexec/internal/config"
	"github.com/openexec/openexec/internal/intent"
	"github.com/openexec/openexec/internal/mcp"
	"github.com/openexec/openexec/internal/planner"
	"github.com/openexec/openexec/internal/prompt"
	"github.com/openexec/openexec/internal/release"
)

type retainedPlanRequest struct {
	RequestID   string      `json:"request_id"`
	InputDigest string      `json:"input_digest"`
	Result      *PlanResult `json:"result,omitempty"`
	ReviewRound int         `json:"review_round,omitempty"`
	ReviewLimit int         `json:"review_limit,omitempty"`
}

func planDigest(v any) string {
	raw, _ := json.Marshal(v)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func (m *Manager) replayReviewedPlan(ctx context.Context, req PlanRequest) (*PlanResult, error) {
	if strings.TrimSpace(req.RequestID) == "" || len(req.RequestID) > 512 {
		return nil, &PlanInputError{Message: "invalid planning request ID"}
	}
	if !req.Review || m.cfg.PlanGenerator == nil || m.cfg.PlanReviewer == nil {
		return nil, fmt.Errorf("durable planning requires admitted generator, reviewer and explicit review; native fallback refused")
	}
	content, err := m.replayIntent(req)
	if err != nil {
		return nil, err
	}
	lock, err := m.lockTaskExecution(true)
	if err != nil {
		return nil, err
	}
	defer lock.Close()
	rel, err := m.GetInternalReleaseManager()
	if err != nil {
		return nil, err
	}
	if err := rel.Load(); err != nil {
		return nil, err
	}
	digest := planDigest(struct {
		Intent                         string
		AutoImport, Review, NoValidate bool
	}{content, req.AutoImport, req.Review, req.NoValidate})
	runID := "plan-request-" + planDigest(req.RequestID)
	stepID := runID + "-review"
	db := m.state.GetDB()
	retained := retainedPlanRequest{RequestID: req.RequestID, InputDigest: digest}
	var raw, storedDigest, storedRun, phase, agent string
	err = db.QueryRowContext(ctx, `SELECT metadata,inputs_hash,run_id,phase,agent FROM run_steps WHERE id=?`, stepID).Scan(&raw, &storedDigest, &storedRun, &phase, &agent)
	if err == nil {
		if storedDigest != digest || storedRun != runID || phase != "plan" || agent != "reviewed-planner" || json.Unmarshal([]byte(raw), &retained) != nil || retained.RequestID != req.RequestID || retained.InputDigest != digest {
			return nil, fmt.Errorf("planning request identity conflicts with retained input")
		}
	} else if err == sql.ErrNoRows {
		retained.ReviewRound = 1
		retained.ReviewLimit = m.cfg.MaxReviewCycles
		if retained.ReviewLimit == 0 {
			retained.ReviewLimit = config.DefaultMaxReviewCycles
		}
		if retained.ReviewLimit < 1 {
			return nil, fmt.Errorf("review cycle limit does not permit planning")
		}
		if err := m.state.CreateRun(ctx, runID, "", "", m.cfg.WorkDir, "reviewed-planning"); err != nil {
			return nil, err
		}
		encoded, _ := json.Marshal(retained)
		raw = string(encoded)
		if _, err := db.ExecContext(ctx, `INSERT INTO run_steps(id,run_id,phase,agent,iteration,status,inputs_hash,metadata) VALUES(?,?,'plan','reviewed-planner',0,'pending',?,?)`, stepID, runID, digest, raw); err != nil {
			return nil, err
		}
	} else {
		return nil, err
	}
	var retainedProject, retainedMode string
	if err := db.QueryRowContext(ctx, `SELECT project_path,mode FROM runs WHERE id=?`, runID).Scan(&retainedProject, &retainedMode); err != nil {
		return nil, err
	}
	if retainedProject != m.cfg.WorkDir || retainedMode != "reviewed-planning" {
		return nil, fmt.Errorf("planning run belongs to a different scope")
	}
	finished := false
	defer func() {
		// The planning request is an audit parent, not an admitted worker.
		// Interrupted replay must not leave a permanently starting provider run.
		auditCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		status := "paused"
		if finished {
			status = "completed"
		}
		_, _ = db.ExecContext(auditCtx, `UPDATE runs SET status=? WHERE id=? AND status!='completed'`, status, runID)
	}()
	save := func() error {
		encoded, err := json.Marshal(retained)
		if err != nil {
			return err
		}
		result, err := db.ExecContext(ctx, `UPDATE run_steps SET metadata=?,status=? WHERE id=? AND metadata=? AND inputs_hash=?`, string(encoded), map[bool]string{true: "completed", false: "pending"}[retained.Result != nil && retained.Result.Review != nil], stepID, raw, digest)
		if err != nil {
			return err
		}
		n, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if n != 1 {
			return fmt.Errorf("planning receipt changed concurrently")
		}
		raw = string(encoded)
		return nil
	}
	if retained.Result == nil {
		plan, err := planner.New(m.cfg.PlanGenerator).GeneratePlan(ctx, content, nil)
		if err != nil {
			return nil, err
		}
		if err := plan.Validate(); err != nil {
			return nil, err
		}
		if req.AutoImport {
			if err := m.preparePlanIDs(plan); err != nil {
				return nil, err
			}
		}
		id, hash, path := m.writePlanArtifact(plan)
		if path == "" {
			return nil, fmt.Errorf("generated plan artifact could not be persisted")
		}
		retained.Result = &PlanResult{Plan: plan, PlanID: id, ArtifactHash: hash, ArtifactPath: path, PromptVersion: prompt.PromptVersion}
		if err := save(); err != nil {
			return nil, err
		}
	}
	for {
		result := retained.Result
		// Validate content-addressed evidence before using it, including on replay.
		expected, _ := json.MarshalIndent(result.Plan, "", "  ")
		sum := sha256.Sum256(expected)
		if result.Plan == nil || result.Plan.Validate() != nil || result.ArtifactHash != hex.EncodeToString(sum[:]) {
			return nil, fmt.Errorf("retained generated plan integrity mismatch")
		}
		expectedPath := filepath.Join(m.cfg.WorkDir, ".openexec", "artifacts", "plans", result.ArtifactHash+".json")
		if result.ArtifactPath != expectedPath {
			return nil, fmt.Errorf("retained plan artifact path mismatch")
		}
		actual, err := m.readPlanEvidence(expectedPath)
		if err != nil || string(actual) != string(expected) {
			return nil, fmt.Errorf("retained plan artifact conflicts or is unavailable")
		}
		if result.Review == nil {
			review, err := planner.New(m.cfg.PlanReviewer).ReviewPlan(ctx, content, result.Plan)
			if err != nil {
				return nil, err
			}
			result.Review = review
			result.Valid = review.Approved
			if !review.Approved {
				result.Issues = []string{review.Assessment}
			}
			reviewRaw, _ := json.Marshal(struct {
				PlanDigest string
				Review     *planner.PlanReview
			}{result.ArtifactHash, review})
			sum := sha256.Sum256(reviewRaw)
			result.ReviewArtifactPath = filepath.Join(filepath.Dir(expectedPath), hex.EncodeToString(sum[:])+".review.json")
			if err := m.writePlanEvidence(result.ReviewArtifactPath, reviewRaw, 0600); err != nil {
				return nil, err
			}
			if err := save(); err != nil {
				return nil, err
			}
		}
		reviewRaw, _ := json.Marshal(struct {
			PlanDigest string
			Review     *planner.PlanReview
		}{result.ArtifactHash, result.Review})
		sum = sha256.Sum256(reviewRaw)
		reviewPath := filepath.Join(filepath.Dir(expectedPath), hex.EncodeToString(sum[:])+".review.json")
		actual, err = m.readPlanEvidence(reviewPath)
		if err != nil || result.ReviewArtifactPath != reviewPath || string(actual) != string(reviewRaw) || result.Valid != result.Review.Approved {
			return nil, fmt.Errorf("retained review evidence mismatch")
		}
		// Archive every exact reviewed version before changing the current pointer.
		// A rejected review is repair input, not an owner decision or permission.
		if retained.ReviewRound == 0 {
			retained.ReviewRound = 1
		}
		if retained.ReviewLimit == 0 && !result.Valid {
			retained.ReviewLimit = m.cfg.MaxReviewCycles
			if retained.ReviewLimit == 0 {
				retained.ReviewLimit = config.DefaultMaxReviewCycles
			}
			if retained.ReviewLimit < 1 {
				return nil, fmt.Errorf("review cycle limit does not permit refinement")
			}
			if err := save(); err != nil {
				return nil, err
			}
		}
		archiveID := fmt.Sprintf("%s-round-%d", stepID, retained.ReviewRound)
		if err := m.archivePlanReview(ctx, archiveID, runID, digest, raw); err != nil {
			return nil, err
		}
		limit := retained.ReviewLimit
		if m.cfg.MaxReviewCycles > 0 && m.cfg.MaxReviewCycles < limit {
			limit = m.cfg.MaxReviewCycles
		}
		if !result.Valid && retained.ReviewRound < limit {
			refined, err := planner.New(m.cfg.PlanGenerator).RefinePlan(ctx, content, result.Plan, result.Review)
			if err != nil {
				return nil, err
			}
			if err := refined.Validate(); err != nil {
				return nil, err
			}
			if req.AutoImport {
				if err := m.preparePlanIDs(refined); err != nil {
					return nil, err
				}
				goals, stories, tasks := reviewedPlanRows(refined)
				if err := rel.ValidatePlanIdentities(ctx, goals, stories, tasks); err != nil {
					return nil, err
				}
			}
			id, hash, path := m.writePlanArtifact(refined)
			if path == "" {
				return nil, fmt.Errorf("refined plan artifact could not be persisted")
			}
			if hash == result.ArtifactHash {
				return nil, fmt.Errorf("plan refinement produced no changed plan; unchanged review retry refused")
			}
			retained.Result = &PlanResult{Plan: refined, PlanID: id, ArtifactHash: hash, ArtifactPath: path, PromptVersion: prompt.PromptVersion}
			retained.ReviewRound++
			if err := save(); err != nil {
				return nil, err
			}
			continue
		}
		if req.AutoImport && result.Valid {
			goals, stories, tasks := reviewedPlanRows(result.Plan)
			if err := rel.ImportReviewedPlan(ctx, goals, stories, tasks, runID+"-import", runID, digest, raw); err != nil {
				return nil, err
			}
		}
		finished = true
		return result, nil
	}
}

func (m *Manager) archivePlanReview(ctx context.Context, stepID, runID, digest, raw string) error {
	db := m.state.GetDB()
	if _, err := db.ExecContext(ctx, `INSERT OR IGNORE INTO run_steps(id,run_id,phase,agent,iteration,status,inputs_hash,metadata) VALUES(?,?,'plan','plan-review-history',0,'completed',?,?)`, stepID, runID, digest, raw); err != nil {
		return err
	}
	var oldRun, oldDigest, oldRaw, agent, phase, status string
	if err := db.QueryRowContext(ctx, `SELECT run_id,inputs_hash,metadata,agent,phase,status FROM run_steps WHERE id=?`, stepID).Scan(&oldRun, &oldDigest, &oldRaw, &agent, &phase, &status); err != nil {
		return err
	}
	if oldRun != runID || oldDigest != digest || oldRaw != raw || agent != "plan-review-history" || phase != "plan" || status != "completed" {
		return fmt.Errorf("retained review history conflicts")
	}
	return nil
}

func (m *Manager) replayIntent(req PlanRequest) (string, error) {
	if req.Intent != "" {
		if req.IntentFile != "" || !req.NoValidate {
			return "", &PlanInputError{Message: "in-memory accepted intent requires NoValidate and no IntentFile"}
		}
		if strings.TrimSpace(req.Intent) == "" {
			return "", &PlanInputError{Message: "intent is empty"}
		}
		return req.Intent, nil
	}
	file := req.IntentFile
	if file == "" {
		file = "INTENT.md"
	}
	validator := mcp.NewPathValidator(mcp.PathValidatorConfig{AllowedRoots: []string{m.cfg.WorkDir}, AllowSymlinks: false, RequireExists: true, RequireFile: true})
	path, err := validator.Validate(filepath.Join(m.cfg.WorkDir, file))
	if err != nil {
		return "", &PlanInputError{Message: err.Error()}
	}
	if !req.NoValidate {
		res, err := intent.NewValidator(path).Validate()
		if err != nil {
			return "", err
		}
		if !res.Valid {
			return "", &PlanInputError{Message: "intent validation failed"}
		}
	}
	raw, err := os.ReadFile(path)
	return string(raw), err
}

func reviewedPlanRows(plan *planner.ProjectPlan) ([]*release.Goal, []*release.Story, []*release.Task) {
	var goals []*release.Goal
	var stories []*release.Story
	var tasks []*release.Task
	for _, g := range plan.Goals {
		goals = append(goals, &release.Goal{ID: g.ID, Title: g.Title, Description: g.Description, SuccessCriteria: g.SuccessCriteria, VerificationMethod: g.VerificationMethod})
	}
	for i, s := range plan.Stories {
		story := &release.Story{ID: s.ID, GoalID: s.GoalID, Title: s.Title, Description: s.Description, AcceptanceCriteria: s.AcceptanceCriteria, VerificationScript: s.VerificationScript, Contract: s.Contract, DependsOn: s.DependsOn, StoryType: release.StoryTypeFeature, Priority: i}
		for j, t := range s.Tasks {
			description := t.Description
			if strings.TrimSpace(t.TechnicalStrategy) != "" {
				description += "\n\nTechnical strategy:\n" + t.TechnicalStrategy
			}
			task := &release.Task{ID: t.ID, StoryID: s.ID, Title: t.Title, Description: description, VerificationScript: t.VerificationScript, DependsOn: t.DependsOn, Priority: j, MaxAttempts: 3}
			if t.Mode == planner.TaskModeHITL {
				task.Metadata = map[string]any{"mode": release.TaskModeHITL}
			}
			tasks = append(tasks, task)
			story.Tasks = append(story.Tasks, t.ID)
		}
		stories = append(stories, story)
	}
	return goals, stories, tasks
}
