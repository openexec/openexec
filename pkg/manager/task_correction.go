package manager

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"encoding/json"
	"github.com/google/uuid"
	"github.com/openexec/openexec/internal/blueprint"
	"github.com/openexec/openexec/internal/execution/gates"
	"github.com/openexec/openexec/internal/knowledge"
	"github.com/openexec/openexec/internal/pipeline"
	"github.com/openexec/openexec/internal/release"
	"github.com/openexec/openexec/pkg/db/state"
	"github.com/openexec/openexec/pkg/runtime"
)

// CorrectionCandidate binds the actual Git candidate, including tracked and
// unignored untracked bytes, modes, branch, HEAD and canonical worktree path.
// Ignored runtime output is outside Git's candidate; callers must not use it
// as source. Graph state is separately bound by the accepted validation plan.
func (m *Manager) CorrectionCandidate(ctx context.Context) (path, branch, digest string, err error) {
	path, err = filepath.EvalSymlinks(m.cfg.WorkDir)
	if err != nil {
		return
	}
	path, err = filepath.Abs(path)
	if err != nil {
		return
	}
	git := func(args ...string) (string, error) {
		cmd := exec.CommandContext(ctx, "git", args...)
		cmd.Dir = path
		b, e := cmd.Output()
		return string(b), e
	}
	branch, err = git("symbolic-ref", "--short", "HEAD")
	if err != nil {
		return
	}
	branch = strings.TrimSpace(branch)
	root, e := git("rev-parse", "--show-toplevel")
	if e != nil {
		err = e
		return
	}
	if strings.TrimSpace(root) != path {
		err = fmt.Errorf("correction requires the candidate root")
		return
	}
	head, e := git("rev-parse", "HEAD")
	if e != nil {
		err = e
		return
	}
	files, e := git("ls-files", "-z", "--cached", "--others", "--exclude-standard")
	if e != nil {
		err = e
		return
	}
	names := strings.Split(files, "\x00")
	sort.Strings(names)
	h := sha256.New()
	fmt.Fprintf(h, "%s\x00%s\x00%s\x00", path, branch, head)
	previous := ""
	for _, name := range names {
		if name == "" || name == previous {
			continue
		}
		previous = name
		file := filepath.Join(path, name)
		info, e := os.Lstat(file)
		if os.IsNotExist(e) {
			fmt.Fprintf(h, "%s\x00deleted\x00", name)
			continue
		}
		if e != nil {
			err = e
			return
		}
		var data []byte
		if info.Mode()&os.ModeSymlink != 0 {
			var target string
			target, e = os.Readlink(file)
			data = []byte(target)
			if e == nil {
				resolved, resolveErr := filepath.EvalSymlinks(file)
				if resolveErr != nil {
					err = resolveErr
					return
				}
				relative, relErr := filepath.Rel(path, resolved)
				if relErr != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
					err = fmt.Errorf("candidate symlink escapes worktree")
					return
				}
				contents, readErr := os.ReadFile(resolved)
				if readErr != nil {
					err = readErr
					return
				}
				data = append(append(data, 0), contents...)
			}
		} else if info.Mode().IsRegular() {
			data, e = os.ReadFile(file)
		} else {
			e = fmt.Errorf("unsupported candidate input %s", name)
		}
		if e != nil {
			err = e
			return
		}
		sum := sha256.Sum256(data)
		fmt.Fprintf(h, "%s\x00%s\x00%x\x00", name, info.Mode(), sum)
	}
	digest = hex.EncodeToString(h.Sum(nil))
	return
}

// AuthorizeTaskCorrection is a trusted control-plane API, not worker output.
// The decision must explicitly authorize this exact candidate and accepted
// plan. It does not broaden the configured executor's resource/effect policy.
func (m *Manager) AuthorizeTaskCorrection(ctx context.Context, c release.TaskCorrection) error {
	lock, err := m.lockTaskExecution(true)
	if err != nil {
		return err
	}
	defer lock.Close()
	if err = m.checkCorrectionCandidate(ctx, c); err != nil {
		return err
	}
	if _, err = m.correctionPlan(ctx, c); err != nil {
		return err
	}
	store, err := release.NewSQLiteStore(m.state.GetDB())
	if err != nil {
		return err
	}
	return store.AuthorizeTaskCorrection(ctx, c)
}

func (m *Manager) checkCorrectionCandidate(ctx context.Context, c release.TaskCorrection) error {
	path, branch, digest, err := m.CorrectionCandidate(ctx)
	if err != nil {
		return err
	}
	if path != c.CandidatePath || branch != c.Branch || digest != c.CandidateDigest {
		return fmt.Errorf("%w: correction candidate mismatch", release.ErrInvalidData)
	}
	manifest, err := knowledge.BuildScanManifest(path)
	if err != nil {
		return err
	}
	if manifest.WorktreeStateHash != c.StateHash {
		return fmt.Errorf("%w: correction validation state mismatch", release.ErrInvalidData)
	}
	return nil
}

func (m *Manager) correctionPlan(ctx context.Context, c release.TaskCorrection) (state.ValidationPlanRevision, error) {
	plan, err := m.state.GetValidationPlanRevision(ctx, c.PlanID)
	if err != nil {
		return plan, err
	}
	var latest string
	err = m.state.GetDB().QueryRowContext(ctx, `SELECT p.id FROM validation_plan_revisions p JOIN graph_generations g ON g.id=p.generation_id WHERE p.task_id=? AND p.status='accepted' ORDER BY p.revision DESC LIMIT 1`, c.TaskID).Scan(&latest)
	if err != nil {
		return plan, err
	}
	if latest != plan.ID || plan.TaskID != c.TaskID || plan.Status != "accepted" || plan.WorktreeStateHash != c.StateHash {
		return plan, fmt.Errorf("%w: correction requires current accepted validation plan", release.ErrInvalidData)
	}
	var graphState, graphStatus string
	if err := m.state.GetDB().QueryRowContext(ctx, `SELECT worktree_state_hash,status FROM graph_generations WHERE id=?`, plan.GenerationID).Scan(&graphState, &graphStatus); err != nil {
		return plan, err
	}
	if graphStatus != "current" || graphState != c.StateHash {
		return plan, fmt.Errorf("%w: correction validation graph is stale", release.ErrInvalidData)
	}
	count := 0
	for _, item := range plan.Items {
		if item.Disposition == "accepted" && (item.Requirement == "required" || item.Requirement == "blocking") {
			count++
			if _, _, err := correctionCheck(item); err != nil {
				return plan, err
			}
		}
	}
	if count == 0 {
		return plan, fmt.Errorf("%w: correction requires explicit required checks", release.ErrInvalidData)
	}
	return plan, nil
}

func correctionCheck(item state.ValidationItem) (string, string, error) {
	if len(item.CommandArgv) == 0 && (item.Scope == "lint" || item.Scope == "test") {
		return item.Scope, "", nil
	}
	if len(item.CommandArgv) == 3 && (item.CommandArgv[0] == "sh" || item.CommandArgv[0] == "/bin/sh") && item.CommandArgv[1] == "-c" && strings.TrimSpace(item.CommandArgv[2]) != "" {
		return "verify", item.CommandArgv[2], nil
	}
	return "", "", fmt.Errorf("%w: required validation item %s has no supported deterministic check", release.ErrInvalidData, item.ID)
}

// Called only by the native queue under exclusive workspace ownership.
func (m *Manager) reconcileTaskCorrection(ctx context.Context, task *release.Task) (err error) {
	if release.CorrectionRefused(task) {
		return errRecaptureWaiting
	}
	store, err := release.NewSQLiteStore(m.state.GetDB())
	if err != nil {
		return err
	}
	admitted := false
	defer func() {
		if admitted || err == nil || !invalidCorrectionState(err) {
			return
		}
		if saveErr := store.RefuseTaskCorrection(ctx, task, err.Error()); saveErr != nil {
			err = errors.Join(err, fmt.Errorf("persist correction refusal: %w", saveErr))
		} else {
			err = nil // Durable refusal; native queue continues independent work.
		}
	}()
	c, err := release.CorrectionForTask(task)
	if err != nil {
		return fmt.Errorf("%w: %v", release.ErrInvalidData, err)
	}
	if task.Git == nil || task.Git.Branch != c.Branch || task.Metadata["verification_failure_evidence"] != c.EvidenceID {
		return fmt.Errorf("%w: correction task binding changed", release.ErrInvalidData)
	}

	step, loadErr := m.state.GetRunStep(ctx, c.EvidenceID)
	if loadErr != nil {
		return loadErr
	}
	var artifacts map[string]string
	if step == nil || step.RunID != task.ID || step.Status != "failed" || step.Agent.String != "deterministic-verification" || json.Unmarshal([]byte(step.Metadata), &artifacts) != nil || !gates.ValidateVerificationFailureArtifacts(artifacts) {
		return fmt.Errorf("%w: correction requires the retained task failure receipt", release.ErrInvalidData)
	}
	if c.Consumed {
		return fmt.Errorf("correction verification allowance already consumed")
	}
	if c.Outcome != "" || c.FreshEvidenceID != "" || c.Reason != "" {
		return fmt.Errorf("%w: malformed correction disposition", release.ErrInvalidData)
	}
	if err = m.checkCorrectionCandidate(ctx, c); err != nil {
		return err
	}
	plan, err := m.correctionPlan(ctx, c)
	if err != nil {
		return err
	}
	rel, err := m.GetInternalReleaseManager()
	if err != nil {
		return err
	}
	eligible, err := rel.CorrectionEligible(ctx, task)
	if err != nil {
		return err
	}
	if !eligible {
		return errRecaptureWaiting
	}
	if err = store.AdmitTaskCorrection(ctx, c); err != nil {
		return err
	}
	admitted = true
	defer func() {
		if err != nil {
			outcome := "refused"
			if ctx.Err() != nil {
				outcome = "cancelled"
			} else if info, e := m.Status(task.ID); e == nil {
				if info.Status == StatusStopped {
					outcome = "stopped"
				} else if info.FailureEvidenceID != "" {
					outcome = "continuing_failure"
				}
			}
			if saveErr := store.FailTaskCorrection(context.Background(), c, outcome, err.Error()); saveErr != nil {
				err = errors.Join(err, fmt.Errorf("persist correction disposition: %w", saveErr))
			} else if outcome == "continuing_failure" {
				err = nil // Durable unfinished work; drain independently runnable tasks.
			}
		}
	}()
	for _, item := range plan.Items {
		if item.Disposition != "accepted" || (item.Requirement != "required" && item.Requirement != "blocking") {
			continue
		}
		if err = ctx.Err(); err != nil {
			return err
		}
		name, command, _ := correctionCheck(item)
		timeout := 5 * time.Minute
		if m.cfg.TaskTimeout > 0 && m.cfg.TaskTimeout < timeout {
			timeout = m.cfg.TaskTimeout
		}
		stage := &blueprint.Stage{Name: name, Type: runtime.StageTypeDeterministic, Timeout: timeout}
		if command != "" {
			stage.Commands = []string{command}
		}
		if err = m.start(ctx, task.ID, true, WithBlueprint("standard_task"), func(cfg *pipeline.Config) { cfg.RecaptureStage = stage }); err != nil {
			return err
		}
		if err = m.waitTaskQueueRun(ctx, task.ID); err != nil {
			return err
		}
		if err = m.checkCorrectionCandidate(ctx, c); err != nil {
			return err
		}
		stepID := "correction-" + uuid.NewString()
		proof, _ := json.Marshal(map[string]string{"correction_decision": c.DecisionRef, "validation_item_id": item.ID, "plan_id": c.PlanID})
		if err = m.state.AddRunStepFull(ctx, stepID, task.ID, "", name, "deterministic-verification", 0, "completed", c.CandidateDigest, string(proof)); err != nil {
			return err
		}
		if err = m.state.LinkValidationEvidence(ctx, state.ValidationEvidenceLink{ValidationItemID: item.ID, RunID: task.ID, RunStepID: stepID, WorktreeStateHash: c.StateHash, PatchHash: plan.PatchHash, Status: "passed"}); err != nil {
			return err
		}
	}
	if _, err = m.state.EvidenceCoverage(ctx, c.PlanID); err != nil {
		return err
	}
	// Serialize Stop with the final disposition; an observed Stop wins.
	m.mu.Lock()
	defer m.mu.Unlock()
	if err = ctx.Err(); err != nil {
		return err
	}
	if entry := m.pipelines[task.ID]; entry == nil || entry.info.Status != StatusComplete {
		return fmt.Errorf("correction stopped before completion")
	}
	if err = m.checkCorrectionCandidate(ctx, c); err != nil {
		return err
	}
	return store.FinishTaskCorrection(ctx, c, true)
}

// Missing or malformed retained records are terminal input refusals. Cancellation,
// unavailable stores and other I/O failures must remain retryable operational errors.
func invalidCorrectionState(err error) bool {
	var syntax *json.SyntaxError
	var shape *json.UnmarshalTypeError
	return errors.Is(err, release.ErrInvalidData) || errors.Is(err, sql.ErrNoRows) ||
		errors.As(err, &syntax) || errors.As(err, &shape)
}
