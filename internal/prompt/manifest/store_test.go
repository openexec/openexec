package manifest

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadManifest(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "spark.yaml", `name: spark
description: "Developer Agent"
title: "Developer Agent - TDD Implementation Specialist"
persona: spark

workflows:
  - id: implement
  - id: mutation-testing
    params:
      intent: "Verify test suite catches intentional bugs"
      stopping_criteria: "mutation score meets standards"
  - id: consult
    params:
      target: "Clario"
      context: "implementation questions"
`)

	store := NewStore(os.DirFS(dir))
	m, err := store.Get("spark")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}

	if m.Name != "spark" {
		t.Errorf("Name = %q, want %q", m.Name, "spark")
	}
	if m.Persona != "spark" {
		t.Errorf("Persona = %q, want %q", m.Persona, "spark")
	}
	if len(m.Workflows) != 3 {
		t.Fatalf("Workflows count = %d, want 3", len(m.Workflows))
	}

	// First workflow: no params.
	if m.Workflows[0].ID != "implement" {
		t.Errorf("Workflows[0].ID = %q, want %q", m.Workflows[0].ID, "implement")
	}
	if m.Workflows[0].Params != nil {
		t.Errorf("Workflows[0].Params = %v, want nil", m.Workflows[0].Params)
	}

	// Second workflow: with params.
	if m.Workflows[1].ID != "mutation-testing" {
		t.Errorf("Workflows[1].ID = %q, want %q", m.Workflows[1].ID, "mutation-testing")
	}
	if len(m.Workflows[1].Params) != 2 {
		t.Errorf("Workflows[1].Params count = %d, want 2", len(m.Workflows[1].Params))
	}
}

func TestManifestWorkflowParams(t *testing.T) {
	m := &Manifest{
		Workflows: []WorkflowRef{
			{ID: "implement"},
			{ID: "mutation-testing", Params: map[string]string{"intent": "test quality"}},
		},
	}

	if p := m.WorkflowParams("mutation-testing"); p == nil || p["intent"] != "test quality" {
		t.Errorf("WorkflowParams(mutation-testing) = %v", p)
	}
	if p := m.WorkflowParams("implement"); p != nil {
		t.Errorf("WorkflowParams(implement) = %v, want nil", p)
	}
	if p := m.WorkflowParams("nonexistent"); p != nil {
		t.Errorf("WorkflowParams(nonexistent) = %v, want nil", p)
	}
}

func TestUnknownManifest(t *testing.T) {
	store := NewStore(os.DirFS(t.TempDir()))
	_, err := store.Get("nonexistent")
	if err == nil {
		t.Error("expected error for unknown manifest, got nil")
	}
}

func TestCacheBehavior(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "cached.yaml", `name: cached
persona: cached
workflows:
  - id: test
`)

	store := NewStore(os.DirFS(dir))
	first, err := store.Get("cached")
	if err != nil {
		t.Fatalf("first Get: %v", err)
	}
	second, err := store.Get("cached")
	if err != nil {
		t.Fatalf("second Get: %v", err)
	}
	if first != second {
		t.Error("second Get returned different pointer; expected cached value")
	}
}

func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}
