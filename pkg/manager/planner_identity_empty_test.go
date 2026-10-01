package manager

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/openexec/openexec/internal/planner"
)

func TestNativeIdenticalReimportPreservesIdentities(t *testing.T) {
	e := newSchedulerTestEnv(t)
	e.mgr.cfg.PlanGenerator = fixedPlanCompletion(identityFixture(t, "retained"))
	req := nativeIdentityRequest(t, e.mgr)
	req.RequestID = ""
	req.Review = false
	req.AutoImport = true
	first, err := e.mgr.Plan(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	before := map[string]string{}
	for _, table := range []string{"goals", "stories", "tasks"} {
		before[table] = identitySnapshot(t, e.mgr, "SELECT * FROM "+table+" ORDER BY id")
	}
	second, err := e.mgr.Plan(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if first.Plan.Stories[0].ID != "US-001" || second.Plan.Stories[0].ID != "US-001" {
		t.Errorf("identical native replay moved US-001 to %s", second.Plan.Stories[0].ID)
	}
	for table, want := range before {
		if got := identitySnapshot(t, e.mgr, "SELECT * FROM "+table+" ORDER BY id"); got != want {
			t.Errorf("identical native replay changed %s identities/counts/bytes", table)
		}
	}
	e.mgr.Close()
	e.closeState()
	fresh := freshQueueManager(t, e)
	fresh.cfg.PlanGenerator = fixedPlanCompletion(identityFixture(t, "retained"))
	if _, err := fresh.Plan(context.Background(), nativeIdentityRequest(t, fresh)); err != nil {
		t.Fatal(err)
	}
	for table, want := range before {
		if identitySnapshot(t, fresh, "SELECT * FROM "+table+" ORDER BY id") != want {
			t.Fatalf("reopened native replay changed %s", table)
		}
	}
}

func nativeIdentityRequest(t *testing.T, m *Manager) PlanRequest {
	t.Helper()
	if err := os.WriteFile(filepath.Join(m.cfg.WorkDir, "INTENT.md"), []byte("Preserve retained plan identities."), 0600); err != nil {
		t.Fatal(err)
	}
	return PlanRequest{IntentFile: "INTENT.md", NoValidate: true, AutoImport: true}
}

// Exercise each nullable list independently: a task-only mismatch used to move
// its parent and siblings as well. Nonempty values elsewhere must survive.
func TestReviewedIdentityLegacyEmptyLists(t *testing.T) {
	for _, column := range []struct{ table, name, id string }{
		{"stories", "acceptance_criteria", "US-001"},
		{"stories", "depends_on", "US-001"},
		{"tasks", "depends_on", "T-US-001-001"},
	} {
		for _, value := range []string{"'null'", "NULL", "CAST('null' AS BLOB)"} {
			for _, reviewed := range []bool{false, true} {
				name := column.table + "/" + column.name + "/" + value
				if reviewed {
					name += "/reviewed"
				} else {
					name += "/native"
				}
				t.Run(name, func(t *testing.T) {
					e := newSchedulerTestEnv(t)
					ctx := context.Background()
					var p planner.ProjectPlan
					if err := json.Unmarshal([]byte(identityFixture(t, "retained")), &p); err != nil {
						t.Fatal(err)
					}
					p.Stories[0].AcceptanceCriteria = nil
					p.Stories[0].Tasks = append(p.Stories[0].Tasks, planner.Task{ID: "T-US-001-002", Title: "Sibling", Description: "Retain sibling"})
					p.Stories[1].DependsOn = []string{"US-001"}
					p.Stories[1].Tasks[0].DependsOn = []string{"T-US-001-001"}
					raw, err := json.Marshal(p)
					if err != nil {
						t.Fatal(err)
					}
					e.mgr.cfg.PlanGenerator = fixedPlanCompletion(string(raw))
					e.mgr.cfg.PlanReviewer = fixedPlanCompletion(replayReviewFixture)
					seed := replayRequest()
					seeded, err := e.mgr.Plan(ctx, seed)
					if err != nil {
						t.Fatal(err)
					}
					if _, err := e.mgr.state.GetDB().Exec("UPDATE "+column.table+" SET "+column.name+"="+value+" WHERE id=?", column.id); err != nil {
						t.Fatal(err)
					}
					if _, err := e.mgr.state.GetDB().Exec(`UPDATE tasks SET status='done',attempt_count=2,completed_at='2026-01-01T00:00:00Z',git_branch='retained',git_commits='["retained-commit"]',metadata='{"mode":"afk","evidence":"retained"}'`); err != nil {
						t.Fatal(err)
					}
					if _, err := e.mgr.state.GetDB().Exec("UPDATE stories SET status='done'"); err != nil {
						t.Fatal(err)
					}
					before := map[string]string{}
					for _, table := range []string{"goals", "stories", "tasks"} {
						before[table] = identitySnapshot(t, e.mgr, "SELECT * FROM "+table+" ORDER BY id")
					}
					receipt := identitySnapshot(t, e.mgr, "SELECT * FROM run_steps WHERE agent='reviewed-plan-import' ORDER BY id")
					var seedStep string
					if err := e.mgr.state.GetDB().QueryRow("SELECT id FROM run_steps WHERE agent='reviewed-plan-import'").Scan(&seedStep); err != nil {
						t.Fatal(err)
					}
					e.mgr.Close()
					e.closeState()
					fresh := freshQueueManager(t, e)
					fresh.cfg.PlanGenerator = fixedPlanCompletion(string(raw))
					fresh.cfg.PlanReviewer = fixedPlanCompletion(replayReviewFixture)
					rel, err := fresh.GetInternalReleaseManager()
					if err != nil {
						t.Fatal(err)
					}
					goals, stories, tasks := reviewedPlanRows(&p)
					// No receipt can short-circuit this rollback-only atomic predicate check.
					if err := rel.ValidatePlanIdentities(ctx, goals, stories, tasks); err != nil {
						t.Fatal(err)
					}
					req := nativeIdentityRequest(t, fresh)
					if reviewed {
						req = replayRequest()
						req.RequestID += "-legacy"
					}
					var replayReceipt string
					for i := 0; i < 2; i++ {
						got, err := fresh.Plan(ctx, req)
						if err != nil {
							t.Fatal(err)
						}
						currentReceipt := identitySnapshot(t, fresh, "SELECT * FROM run_steps WHERE agent='reviewed-plan-import' ORDER BY id")
						if i == 0 {
							replayReceipt = currentReceipt
						} else if currentReceipt != replayReceipt {
							t.Fatal("exact replay changed receipt bytes")
						}
						if !reflect.DeepEqual(got.Plan, seeded.Plan) {
							t.Fatal("exact legacy replay changed plan identities/content")
						}
						for table, want := range before {
							if identitySnapshot(t, fresh, "SELECT * FROM "+table+" ORDER BY id") != want {
								t.Fatalf("legacy replay changed %s identities/counts/bytes", table)
							}
						}
					}
					if identitySnapshot(t, fresh, "SELECT * FROM run_steps WHERE id=?", seedStep) != receipt {
						t.Fatal("retained receipt changed")
					}
					if reviewed {
						var count int
						if err := fresh.state.GetDB().QueryRow("SELECT count(*) FROM run_steps WHERE agent='reviewed-plan-import'").Scan(&count); err != nil || count != 2 {
							t.Fatalf("receipt count=%d: %v", count, err)
						}
					}
				})
			}
		}
	}
}

func TestNativeIdenticalReimportCanonicalStorage(t *testing.T) {
	for name, lists := range map[string][]string{"nil": nil, "empty": {}} {
		t.Run(name, func(t *testing.T) {
			e := newSchedulerTestEnv(t)
			var p planner.ProjectPlan
			if err := json.Unmarshal([]byte(identityFixture(t, "retained")), &p); err != nil {
				t.Fatal(err)
			}
			p.Stories[0].AcceptanceCriteria = lists
			p.Stories[0].DependsOn = lists
			p.Stories[0].Tasks[0].DependsOn = lists
			p.Stories[1].DependsOn = []string{"US-001"}
			p.Stories[1].Tasks[0].DependsOn = []string{"T-US-001-001"}
			// Call the native writer directly so omitempty in provider JSON cannot
			// collapse the explicit empty case into the nil case. Public Plan
			// replay is independently exercised above.
			if err := e.mgr.importBoundPlan(&p, false); err != nil {
				t.Fatal(err)
			}

			for _, c := range []struct{ query, want string }{
				{"SELECT acceptance_criteria FROM stories WHERE id='US-001'", "[]"},
				{"SELECT depends_on FROM stories WHERE id='US-001'", "[]"},
				{"SELECT depends_on FROM tasks WHERE id='T-US-001-001'", "[]"},
				{"SELECT acceptance_criteria FROM stories WHERE id='US-005'", `["Verified"]`},
				{"SELECT depends_on FROM stories WHERE id='US-005'", `["US-001"]`},
				{"SELECT depends_on FROM tasks WHERE id='T-US-005-001'", `["T-US-001-001"]`},
			} {
				var got string
				if err := e.mgr.state.GetDB().QueryRow(c.query).Scan(&got); err != nil {
					t.Fatal(err)
				}
				if got != c.want {
					t.Errorf("%s: got %s want %s", c.query, got, c.want)
				}
			}
		})
	}
}

// Persistence errors must remain errors after canonicalizing native rows.
func TestNativeIdenticalReimportWriteRefusals(t *testing.T) {
	for _, table := range []string{"goals", "stories", "tasks"} {
		t.Run(table, func(t *testing.T) {
			e := newSchedulerTestEnv(t)
			var p planner.ProjectPlan
			if err := json.Unmarshal([]byte(identityFixture(t, "retained")), &p); err != nil {
				t.Fatal(err)
			}
			if _, err := e.mgr.state.GetDB().Exec("CREATE TRIGGER refuse_identity BEFORE INSERT ON " + table + " BEGIN SELECT RAISE(ABORT,'refused native write'); END"); err != nil {
				t.Fatal(err)
			}
			if err := e.mgr.importBoundPlan(&p, false); err == nil || !strings.Contains(err.Error(), "refused native write") {
				t.Fatalf("write refusal lost: %v", err)
			}
		})
	}
	t.Run("unknown goal and strategy", func(t *testing.T) {
		e := newSchedulerTestEnv(t)
		p := &planner.ProjectPlan{Stories: []planner.Story{{ID: "US-001", GoalID: "missing", Tasks: []planner.Task{{ID: "T-US-001-001", Description: "base", TechnicalStrategy: "strategy"}}}}}
		if err := e.mgr.importBoundPlan(p, false); err != nil {
			t.Fatal(err)
		}
		var description string
		var goal any
		if err := e.mgr.state.GetDB().QueryRow("SELECT goal_id FROM stories WHERE id='US-001'").Scan(&goal); err != nil || goal != nil {
			t.Fatalf("dangling goal: %v %v", goal, err)
		}
		if err := e.mgr.state.GetDB().QueryRow("SELECT description FROM tasks WHERE id='T-US-001-001'").Scan(&description); err != nil || description != "base\n\nTechnical strategy:\nstrategy" {
			t.Fatalf("strategy: %q %v", description, err)
		}
	})
	t.Run("reviewed stale identities", func(t *testing.T) {
		e := newSchedulerTestEnv(t)
		var p planner.ProjectPlan
		if err := json.Unmarshal([]byte(identityFixture(t, "retained")), &p); err != nil {
			t.Fatal(err)
		}
		if err := e.mgr.importBoundPlan(&p, false); err != nil {
			t.Fatal(err)
		}
		p.Stories[0].Description = "changed after review"
		if err := e.mgr.importBoundPlan(&p, true); err == nil || !strings.Contains(err.Error(), "backlog changed after plan review") {
			t.Fatalf("stale approval: %v", err)
		}
		e.closeState()
		if err := e.mgr.importBoundPlan(&p, false); err == nil {
			t.Fatal("closed store accepted")
		}
	})
}
