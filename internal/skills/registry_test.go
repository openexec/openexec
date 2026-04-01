package skills

import (
	"os"
	"path/filepath"
	"testing"
)

const testSkillMD = `---
name: test-skill
description: A test skill for unit testing
categories:
  - testing
  - backend
tags:
  - go
  - unit-test
when_to_use: Use when writing Go tests
priority: high
---
# Test Skill

This skill helps you write Go unit tests effectively.

- Always use table-driven tests.
- Use testify for assertions when available.
`

const testSkillNoFrontmatter = `# Plain Skill

This is a skill with no frontmatter. It should still be loadable.
`

const testSkillFrontend = `---
name: frontend-skill
description: A frontend development skill
categories:
  - frontend
  - ui
tags:
  - react
  - typescript
when_to_use: Use when building React components
priority: medium
---
# Frontend Skill

Follow component-driven development patterns.
`

func TestParseSkillFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "SKILL.md")
	if err := os.WriteFile(path, []byte(testSkillMD), 0o644); err != nil {
		t.Fatal(err)
	}

	skill, err := ParseSkillFile(path)
	if err != nil {
		t.Fatalf("ParseSkillFile: %v", err)
	}

	if skill.Name != "test-skill" {
		t.Errorf("Name = %q, want %q", skill.Name, "test-skill")
	}
	if skill.Description != "A test skill for unit testing" {
		t.Errorf("Description = %q, want %q", skill.Description, "A test skill for unit testing")
	}
	if len(skill.Categories) != 2 || skill.Categories[0] != "testing" {
		t.Errorf("Categories = %v, want [testing backend]", skill.Categories)
	}
	if len(skill.Tags) != 2 || skill.Tags[0] != "go" {
		t.Errorf("Tags = %v, want [go unit-test]", skill.Tags)
	}
	if skill.WhenToUse != "Use when writing Go tests" {
		t.Errorf("WhenToUse = %q", skill.WhenToUse)
	}
	if skill.Priority != "high" {
		t.Errorf("Priority = %q, want %q", skill.Priority, "high")
	}
	if !skill.Enabled {
		t.Error("Enabled should default to true")
	}
	if skill.Content == "" {
		t.Error("Content should not be empty")
	}
	if skill.SourcePath != path {
		t.Errorf("SourcePath = %q, want %q", skill.SourcePath, path)
	}
}

func TestParseSkillContent_NoFrontmatter(t *testing.T) {
	skill, err := ParseSkillContent(testSkillNoFrontmatter, "/fake/path")
	if err != nil {
		t.Fatalf("ParseSkillContent: %v", err)
	}
	if skill.Content == "" {
		t.Error("Content should not be empty for frontmatter-less files")
	}
	if !skill.Enabled {
		t.Error("Enabled should default to true")
	}
}

func TestLoadFromDir(t *testing.T) {
	dir := t.TempDir()

	// Create two skill directories
	for _, name := range []string{"skill-a", "skill-b"} {
		skillDir := filepath.Join(dir, name)
		if err := os.MkdirAll(skillDir, 0o755); err != nil {
			t.Fatal(err)
		}
		content := testSkillMD
		if name == "skill-b" {
			content = testSkillFrontend
		}
		if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	// Create a non-skill directory (no SKILL.md)
	if err := os.MkdirAll(filepath.Join(dir, "not-a-skill"), 0o755); err != nil {
		t.Fatal(err)
	}

	r := NewRegistry()
	if err := r.LoadFromDir(dir, "test"); err != nil {
		t.Fatalf("LoadFromDir: %v", err)
	}

	skills := r.List()
	if len(skills) != 2 {
		t.Fatalf("List() returned %d skills, want 2", len(skills))
	}

	// Verify sources
	for _, s := range skills {
		if s.Source != "test" {
			t.Errorf("skill %q Source = %q, want %q", s.Name, s.Source, "test")
		}
	}
}

func TestLoadFromDir_NonExistent(t *testing.T) {
	r := NewRegistry()
	err := r.LoadFromDir("/nonexistent/path", "test")
	if err != nil {
		t.Errorf("LoadFromDir on nonexistent path should return nil, got %v", err)
	}
}

func TestSelectForTask(t *testing.T) {
	r := NewRegistry()

	// Register skills manually
	r.mu.Lock()
	r.skills["test-skill"] = &Skill{
		Name:        "test-skill",
		Description: "A test skill for unit testing",
		Categories:  []string{"testing", "backend"},
		Tags:        []string{"go", "unit-test"},
		WhenToUse:   "Use when writing Go tests",
		Priority:    "high",
		Content:     "Test content",
		Enabled:     true,
	}
	r.skills["frontend-skill"] = &Skill{
		Name:        "frontend-skill",
		Description: "A frontend development skill",
		Categories:  []string{"frontend", "ui"},
		Tags:        []string{"react", "typescript"},
		WhenToUse:   "Use when building React components",
		Priority:    "medium",
		Content:     "Frontend content",
		Enabled:     true,
	}
	r.skills["docs-skill"] = &Skill{
		Name:        "docs-skill",
		Description: "Documentation writing skill",
		Categories:  []string{"documentation"},
		Tags:        []string{"markdown"},
		WhenToUse:   "Use when writing docs",
		Priority:    "low",
		Content:     "Docs content",
		Enabled:     true,
	}
	r.mu.Unlock()

	// Query related to testing
	selected := r.SelectForTask("write Go unit tests")
	if len(selected) == 0 {
		t.Fatal("SelectForTask returned no results for 'write Go unit tests'")
	}
	if selected[0].Name != "test-skill" {
		t.Errorf("Top skill = %q, want %q", selected[0].Name, "test-skill")
	}

	// Query related to frontend
	selected = r.SelectForTask("build React components")
	if len(selected) == 0 {
		t.Fatal("SelectForTask returned no results for 'build React components'")
	}
	found := false
	for _, s := range selected {
		if s.Name == "frontend-skill" {
			found = true
			break
		}
	}
	if !found {
		t.Error("frontend-skill should be in results for 'build React components'")
	}
}

func TestSelectForTask_Empty(t *testing.T) {
	r := NewRegistry()
	selected := r.SelectForTask("")
	if selected != nil {
		t.Errorf("SelectForTask('') should return nil, got %v", selected)
	}
}

func TestSelectForTask_MaxThree(t *testing.T) {
	r := NewRegistry()
	r.mu.Lock()
	for i := 0; i < 10; i++ {
		name := "skill-" + string(rune('a'+i))
		r.skills[name] = &Skill{
			Name:       name,
			Tags:       []string{"common"},
			WhenToUse:  "always useful for common tasks",
			Enabled:    true,
		}
	}
	r.mu.Unlock()

	selected := r.SelectForTask("common tasks")
	if len(selected) > 3 {
		t.Errorf("SelectForTask should return at most 3, got %d", len(selected))
	}
}

func TestSearch(t *testing.T) {
	r := NewRegistry()
	r.mu.Lock()
	r.skills["go-testing"] = &Skill{
		Name:        "go-testing",
		Description: "Go testing patterns",
		Tags:        []string{"go", "test"},
		Content:     "Use table-driven tests",
		Enabled:     true,
	}
	r.skills["python-lint"] = &Skill{
		Name:        "python-lint",
		Description: "Python linting rules",
		Tags:        []string{"python", "lint"},
		Content:     "Use ruff for linting",
		Enabled:     true,
	}
	r.mu.Unlock()

	results := r.Search("go")
	if len(results) != 1 {
		t.Fatalf("Search('go') returned %d results, want 1", len(results))
	}
	if results[0].Name != "go-testing" {
		t.Errorf("Search('go') result = %q, want %q", results[0].Name, "go-testing")
	}

	results = r.Search("lint")
	if len(results) != 1 {
		t.Fatalf("Search('lint') returned %d results, want 1", len(results))
	}
}

func TestEnableDisable(t *testing.T) {
	r := NewRegistry()
	r.mu.Lock()
	r.skills["my-skill"] = &Skill{
		Name:    "my-skill",
		Enabled: true,
	}
	r.mu.Unlock()

	// Disable
	if err := r.Disable("my-skill"); err != nil {
		t.Fatalf("Disable: %v", err)
	}
	s, _ := r.Get("my-skill")
	if s.Enabled {
		t.Error("skill should be disabled")
	}

	// Enable
	if err := r.Enable("my-skill"); err != nil {
		t.Fatalf("Enable: %v", err)
	}
	s, _ = r.Get("my-skill")
	if !s.Enabled {
		t.Error("skill should be enabled")
	}

	// Not found
	if err := r.Enable("nonexistent"); err == nil {
		t.Error("Enable nonexistent should return error")
	}
	if err := r.Disable("nonexistent"); err == nil {
		t.Error("Disable nonexistent should return error")
	}
}

func TestListByCategory(t *testing.T) {
	r := NewRegistry()
	r.mu.Lock()
	r.skills["a"] = &Skill{Name: "a", Categories: []string{"testing"}, Enabled: true}
	r.skills["b"] = &Skill{Name: "b", Categories: []string{"frontend"}, Enabled: true}
	r.skills["c"] = &Skill{Name: "c", Categories: []string{"testing", "backend"}, Enabled: true}
	r.mu.Unlock()

	result := r.ListByCategory("testing")
	if len(result) != 2 {
		t.Fatalf("ListByCategory('testing') returned %d, want 2", len(result))
	}

	result = r.ListByCategory("frontend")
	if len(result) != 1 {
		t.Fatalf("ListByCategory('frontend') returned %d, want 1", len(result))
	}

	result = r.ListByCategory("nonexistent")
	if len(result) != 0 {
		t.Fatalf("ListByCategory('nonexistent') returned %d, want 0", len(result))
	}
}

func TestImportFromPath(t *testing.T) {
	// Create source directory with skills
	sourceDir := t.TempDir()
	skillDir := filepath.Join(sourceDir, "claude-skill")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// Skill without OpenExec extensions
	claudeSkill := `---
name: claude-skill
description: A Claude skill
---
# Claude Skill Content
`
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(claudeSkill), 0o644); err != nil {
		t.Fatal(err)
	}

	targetDir := filepath.Join(t.TempDir(), "imported")
	imported, err := ImportFromPath(sourceDir, targetDir)
	if err != nil {
		t.Fatalf("ImportFromPath: %v", err)
	}
	if len(imported) != 1 || imported[0] != "claude-skill" {
		t.Errorf("imported = %v, want [claude-skill]", imported)
	}

	// Verify the file was written and has extensions
	destFile := filepath.Join(targetDir, "claude-skill", "SKILL.md")
	data, err := os.ReadFile(destFile)
	if err != nil {
		t.Fatalf("read imported file: %v", err)
	}
	content := string(data)
	if !contains(content, "categories:") {
		t.Error("imported file should have categories field")
	}
	if !contains(content, "tags:") {
		t.Error("imported file should have tags field")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsSubstr(s, substr))
}

func containsSubstr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
