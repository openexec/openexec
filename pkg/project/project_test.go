package project

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadProjectsExecutorModel(t *testing.T) {
	dir := t.TempDir()
	configDir := filepath.Join(dir, ".openexec")
	if err := os.Mkdir(configDir, 0o750); err != nil {
		t.Fatal(err)
	}
	data := []byte(`{"name":"test","execution":{"planner_model":"planner-only","executor_model":"executor-only"}}`)
	if err := os.WriteFile(filepath.Join(configDir, "config.json"), data, 0o600); err != nil {
		t.Fatal(err)
	}
	view, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if view.ExecutorModel != "executor-only" || view.PlannerModel != "planner-only" {
		t.Fatalf("model projection = planner %q executor %q", view.PlannerModel, view.ExecutorModel)
	}
}
