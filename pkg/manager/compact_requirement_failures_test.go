package manager

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCompactRequirementFileRouteFailures(t *testing.T) {
	for _, name := range []string{"memory", "missing-file", "invalid-intent", "provider", "invalid-plan", "review-provider", "artifact", "native-provider", "native-review"} {
		t.Run(name, func(t *testing.T) {
			e := newSchedulerTestEnv(t)
			if err := os.WriteFile(filepath.Join(e.dir, "INTENT.md"), []byte("incomplete intent"), 0600); err != nil {
				t.Fatal(err)
			}
			e.mgr.cfg.PlanGenerator = fixedPlanCompletion(replayPlanFixture)
			e.mgr.cfg.PlanReviewer = fixedPlanCompletion(replayReviewFixture)
			req := PlanRequest{Compact: true, Review: true, NoValidate: true}
			want := ""
			switch name {
			case "memory":
				req.Intent = "intent"
				want = "stable reviewed request ID"
			case "missing-file":
				req.IntentFile = "absent.md"
				want = "invalid intent_file"
			case "invalid-intent":
				req.NoValidate = false
			case "provider":
				e.mgr.cfg.PlanGenerator = fixedPlanCompletion(`broken`)
				want = "planner failed"
			case "invalid-plan":
				e.mgr.cfg.PlanGenerator = fixedPlanCompletion(`{"stories":[{"id":"S"}]}`)
				want = "missing title"
			case "review-provider":
				e.mgr.cfg.PlanReviewer = fixedPlanCompletion(`{}`)
				want = "requires explicit"
			case "artifact":
				e.mgr.cfg.PlanReviewer = planCompletionFunc(func(context.Context, string) (string, error) {
					if err := os.WriteFile(filepath.Join(e.dir, ".openexec", "artifacts"), []byte("blocked"), 0600); err != nil {
						t.Fatal(err)
					}
					return replayReviewFixture, nil
				})
				want = "artifact was not persisted"
			case "native-provider", "native-review":
				// Exercise the native fallback with a local executable, never an ambient provider.
				script := filepath.Join(e.dir, "fixture-provider")
				body := "#!/bin/sh\ncat >/dev/null\nprintf '%s' '" + replayPlanFixture + "'\n"
				if name == "native-provider" {
					body = "#!/bin/sh\nexit 9\n"
					want = "planner failed"
				} else {
					want = "requires explicit"
				}
				if err := os.WriteFile(script, []byte(body), 0700); err != nil {
					t.Fatal(err)
				}
				t.Setenv("OPENEXEC_PLANNER_CLI", script)
				t.Setenv("OPENEXEC_PLANNER_ARGS", "")
				e.mgr.cfg.PlanGenerator = nil
				e.mgr.cfg.PlanReviewer = nil
			}
			result, err := e.mgr.Plan(context.Background(), req)
			if name == "invalid-intent" {
				if err != nil || result == nil || result.Valid || len(result.Issues) == 0 {
					t.Fatalf("invalid intent not refused: %+v %v", result, err)
				}
			} else if err == nil || !strings.Contains(err.Error(), want) {
				t.Fatalf("want %q: %+v %v", want, result, err)
			}
			var n int
			if err := e.mgr.state.GetDB().QueryRow(`SELECT COUNT(*) FROM tasks`).Scan(&n); err != nil || n != 0 {
				t.Fatalf("failed plan imported: %d %v", n, err)
			}
		})
	}
}

func TestCompactRequirementReplayFailureBoundaries(t *testing.T) {
	for _, name := range []string{"invalid-id", "invalid-intent", "negative-limit", "generator", "invalid-plan", "artifact", "save", "concurrent-save", "refined-invalid", "refined-artifact", "unchanged", "refined-save", "review-save", "create-run", "create-step"} {
		t.Run(name, func(t *testing.T) {
			e := newSchedulerTestEnv(t)
			req := replayRequest()
			req.Compact = true
			e.mgr.cfg.MaxReviewCycles = 2
			e.mgr.cfg.PlanReviewer = fixedPlanCompletion(replayReviewFixture)
			calls := 0
			execSQL := func(sql string) {
				t.Helper()
				if _, err := e.mgr.state.GetDB().Exec(sql); err != nil {
					t.Fatal(err)
				}
			}
			blockArtifacts := func() {
				t.Helper()
				p := filepath.Join(e.dir, ".openexec", "artifacts")
				if err := os.RemoveAll(p); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(p, []byte("blocked"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			e.mgr.cfg.PlanGenerator = planCompletionFunc(func(context.Context, string) (string, error) {
				calls++
				switch name {
				case "generator":
					return "broken", nil
				case "invalid-plan":
					return `{"stories":[{"id":"S"}]}`, nil
				case "artifact":
					blockArtifacts()
				case "save":
					execSQL(`CREATE TRIGGER deny_save BEFORE UPDATE ON run_steps BEGIN SELECT RAISE(FAIL,'fixture save'); END`)
				case "concurrent-save":
					execSQL(`UPDATE run_steps SET metadata='{}'`)
				}
				if calls > 1 {
					switch name {
					case "refined-invalid":
						return `[{"id":"S"}]`, nil
					case "refined-artifact":
						blockArtifacts()
					case "unchanged":
						return replayPlanFixture, nil
					case "refined-save":
						execSQL(`CREATE TRIGGER deny_save BEFORE UPDATE ON run_steps BEGIN SELECT RAISE(FAIL,'fixture save'); END`)
					}
					return strings.Replace(replayPlanFixture, "Verify the running editing journey", "Verify saved reload", 1), nil
				}
				return replayPlanFixture, nil
			})
			want := ""
			switch name {
			case "invalid-id":
				req.RequestID = " "
				want = "invalid planning request ID"
			case "invalid-intent":
				req.Intent = " "
				want = "intent is empty"
			case "negative-limit":
				e.mgr.cfg.MaxReviewCycles = -1
				want = "limit does not permit"
			case "generator":
				want = "failed to parse"
			case "invalid-plan", "refined-invalid":
				want = "missing title"
			case "artifact":
				want = "generated plan artifact"
			case "save", "refined-save", "review-save":
				want = "fixture save"
			case "concurrent-save":
				want = "changed concurrently"
			case "refined-artifact":
				want = "refined plan artifact"
			case "unchanged":
				want = "unchanged review retry refused"
			case "create-run":
				execSQL(`CREATE TRIGGER deny_run BEFORE INSERT ON runs BEGIN SELECT RAISE(FAIL,'fixture run'); END`)
				want = "fixture run"
			case "create-step":
				execSQL(`CREATE TRIGGER deny_step BEFORE INSERT ON run_steps BEGIN SELECT RAISE(FAIL,'fixture step'); END`)
				want = "fixture step"
			}
			if strings.HasPrefix(name, "refined-") || name == "unchanged" {
				e.mgr.cfg.PlanReviewer = fixedPlanCompletion(rejectedReplayReview)
			}
			if name == "review-save" {
				e.mgr.cfg.PlanReviewer = planCompletionFunc(func(context.Context, string) (string, error) {
					execSQL(`CREATE TRIGGER deny_save BEFORE UPDATE ON run_steps BEGIN SELECT RAISE(FAIL,'fixture save'); END`)
					return replayReviewFixture, nil
				})
			}
			if _, err := e.mgr.Plan(context.Background(), req); err == nil || !strings.Contains(err.Error(), want) {
				t.Fatalf("want %q: %v", want, err)
			}
			var n int
			if err := e.mgr.state.GetDB().QueryRow(`SELECT COUNT(*) FROM tasks`).Scan(&n); err != nil || n != 0 {
				t.Fatalf("failure imported work: %d %v", n, err)
			}
		})
	}
}

func TestCompactRequirementRetainedEvidenceAndLegacyLimits(t *testing.T) {
	for _, name := range []string{"hash", "path", "file", "review", "scope", "history", "legacy", "legacy-negative", "lower-limit"} {
		t.Run(name, func(t *testing.T) {
			e := newSchedulerTestEnv(t)
			req := replayRequest()
			req.Compact = true
			req.AutoImport = false
			e.mgr.cfg.MaxReviewCycles = 1
			e.mgr.cfg.PlanGenerator = fixedPlanCompletion(replayPlanFixture)
			e.mgr.cfg.PlanReviewer = fixedPlanCompletion(rejectedReplayReview)
			result, err := e.mgr.Plan(context.Background(), req)
			if err != nil {
				t.Fatal(err)
			}
			db := e.mgr.state.GetDB()
			var id, raw string
			if err := db.QueryRow(`SELECT id,metadata FROM run_steps WHERE agent='reviewed-planner'`).Scan(&id, &raw); err != nil {
				t.Fatal(err)
			}
			var retained retainedPlanRequest
			if err := json.Unmarshal([]byte(raw), &retained); err != nil {
				t.Fatal(err)
			}
			want := ""
			execSQL := func(sql string) {
				t.Helper()
				if _, err := db.Exec(sql); err != nil {
					t.Fatal(err)
				}
			}
			switch name {
			case "hash":
				retained.Result.ArtifactHash = "bad"
				want = "integrity mismatch"
			case "path":
				retained.Result.ArtifactPath = "bad"
				want = "path mismatch"
			case "file":
				if err := os.Remove(result.ArtifactPath); err != nil {
					t.Fatal(err)
				}
				want = "unavailable"
			case "review":
				retained.Result.Valid = true
				want = "review evidence mismatch"
			case "scope":
				execSQL(`UPDATE runs SET mode='other'`)
				want = "different scope"
			case "history":
				execSQL(`UPDATE run_steps SET metadata='{}' WHERE agent='plan-review-history'`)
				want = "history conflicts"
			case "legacy", "legacy-negative":
				retained.ReviewRound = 0
				retained.ReviewLimit = 0
				execSQL(`DELETE FROM run_steps WHERE agent='plan-review-history'`)
				e.mgr.cfg.MaxReviewCycles = 0
				if name == "legacy-negative" {
					e.mgr.cfg.MaxReviewCycles = -1
					want = "limit does not permit refinement"
				} else {
					want = "unchanged review retry refused"
				}
			case "lower-limit":
				retained.ReviewLimit = 3
			}
			encoded, err := json.Marshal(retained)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := db.Exec(`UPDATE run_steps SET metadata=? WHERE id=?`, string(encoded), id); err != nil {
				t.Fatal(err)
			}
			if name == "lower-limit" {
				if _, err := db.Exec(`UPDATE run_steps SET metadata=? WHERE agent='plan-review-history'`, string(encoded)); err != nil {
					t.Fatal(err)
				}
			}
			fresh := &Manager{cfg: e.mgr.cfg, state: e.mgr.state}
			again, err := fresh.Plan(context.Background(), req)
			if name == "lower-limit" {
				if err != nil || again.Valid {
					t.Fatalf("lower bound ignored: %+v %v", again, err)
				}
			} else if err == nil || !strings.Contains(err.Error(), want) {
				t.Fatalf("want %q: %v", want, err)
			}
		})
	}
}
