package manager

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/openexec/openexec/internal/planner"
)

func TestPlanArtifactsRejectWorkspaceSymlinkEscape(t *testing.T) {
	workspace, outside := t.TempDir(), t.TempDir()
	if err := os.MkdirAll(filepath.Join(workspace, ".openexec", "artifacts"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outside, "sentinel"), []byte("preserved"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(workspace, ".openexec", "artifacts", "plans")); err != nil {
		t.Fatal(err)
	}
	m := &Manager{cfg: Config{WorkDir: workspace}}
	var plan planner.ProjectPlan
	if err := json.Unmarshal([]byte(replayPlanFixture), &plan); err != nil {
		t.Fatal(err)
	}
	if _, _, path := m.writePlanArtifact(&plan); path != "" {
		t.Fatal("artifact escaped workspace", path)
	}
	alias := filepath.Join(workspace, ".openexec", "artifacts", "plans", "sentinel")
	if err := m.writePlanEvidence(alias, []byte("overwritten"), 0600); err == nil {
		t.Fatal("review write followed external symlink")
	}
	if _, err := m.readPlanEvidence(alias); err == nil {
		t.Fatal("evidence read followed external symlink")
	}
	data, _ := os.ReadFile(filepath.Join(outside, "sentinel"))
	entries, _ := os.ReadDir(outside)
	if string(data) != "preserved" || len(entries) != 1 {
		t.Fatal("outside evidence modified")
	}
}

func TestPlanArtifactRootReadbackAndReviewSwap(t *testing.T) {
	e := newSchedulerTestEnv(t)
	var plan planner.ProjectPlan
	_ = json.Unmarshal([]byte(replayPlanFixture), &plan)
	_, _, path := e.mgr.writePlanArtifact(&plan)
	if path == "" {
		t.Fatal("legitimate artifact not saved")
	}
	raw, err := e.mgr.readPlanEvidence(path)
	if err != nil {
		t.Fatal(err)
	}
	var actual planner.ProjectPlan
	if err = json.Unmarshal(raw, &actual); err != nil || actual.Stories[0].ID != plan.Stories[0].ID {
		t.Fatal("root artifact not round-tripped")
	}
	outside := t.TempDir()
	sentinel := filepath.Join(outside, "sentinel")
	_ = os.WriteFile(sentinel, []byte("preserved"), 0600)
	e.mgr.cfg.PlanGenerator = planCompletionFunc(func(context.Context, string) (string, error) { return replayPlanFixture, nil })
	e.mgr.cfg.PlanReviewer = planCompletionFunc(func(context.Context, string) (string, error) {
		dir := filepath.Join(e.dir, ".openexec", "artifacts", "plans")
		if err := os.Rename(dir, dir+"-retained"); err != nil {
			return "", err
		}
		if err := os.Symlink(outside, dir); err != nil {
			return "", err
		}
		return replayReviewFixture, nil
	})
	if _, err := e.mgr.Plan(context.Background(), replayRequest()); err == nil {
		t.Fatal("review directory swap escaped write boundary")
	}
	entries, _ := os.ReadDir(outside)
	data, _ := os.ReadFile(sentinel)
	if len(entries) != 1 || string(data) != "preserved" {
		t.Fatal("review wrote outside candidate")
	}
	task, err := e.rel.TaskSnapshot(context.Background(), "T-1")
	if err == nil || task != nil {
		t.Fatal("unpersisted review imported task")
	}
}
