package workflow

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadWorkflowWithParams(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "mutation-testing.md", `---
params:
  intent: "Why this agent runs mutation testing"
  stopping_criteria: "When to stop"
---

<instructions>
{{intent}}
</instructions>

<process>
1. Run mutation testing tool
2. Continue until: {{stopping_criteria}}
</process>
`)

	store := NewStore(dir)
	tmpl, err := store.Get("mutation-testing")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}

	if len(tmpl.Params) != 2 {
		t.Errorf("Params count = %d, want 2", len(tmpl.Params))
	}
	if _, ok := tmpl.Params["intent"]; !ok {
		t.Error("missing param: intent")
	}
	if _, ok := tmpl.Params["stopping_criteria"]; !ok {
		t.Error("missing param: stopping_criteria")
	}
	if tmpl.Instructions != "{{intent}}" {
		t.Errorf("Instructions = %q, want %q", tmpl.Instructions, "{{intent}}")
	}
}

func TestLoadWorkflowWithoutParams(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "implement.md", `---
---

<instructions>
Execute full TDD implementation workflow.
</instructions>

<process>
1. Read story file
2. Design test suite
3. Execute red-green-refactor
</process>
`)

	store := NewStore(dir)
	tmpl, err := store.Get("implement")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}

	if tmpl.Params != nil {
		t.Errorf("Params = %v, want nil", tmpl.Params)
	}
	if tmpl.Instructions != "Execute full TDD implementation workflow." {
		t.Errorf("Instructions = %q", tmpl.Instructions)
	}
}

func TestUnknownWorkflow(t *testing.T) {
	store := NewStore(t.TempDir())
	_, err := store.Get("nonexistent")
	if err == nil {
		t.Error("expected error for unknown workflow, got nil")
	}
}

func TestCacheBehavior(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "cached.md", `---
---

<instructions>Cached</instructions>
<process>Steps</process>
`)

	store := NewStore(dir)
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
