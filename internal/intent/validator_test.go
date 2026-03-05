package intent

import (
	"os"
	"path/filepath"
	"testing"
)

func TestValidator(t *testing.T) {
	tmpDir := t.TempDir()
	intentPath := filepath.Join(tmpDir, "INTENT.md")

	t.Run("Valid INTENT.md", func(t *testing.T) {
		content := `# Test Project

## Goals
- [ ] Goal 1: Build a CLI
- [ ] Goal 2: Support Docker

## Requirements
- [ ] US-001: As a user, I want a CLI so that I can run tasks.
  - Acceptance Criteria: Given a command, when I run it, then it succeeds.

## Constraints
- Platform: macOS, Docker
- Shape: CLI
- Data Source: Local SQLite
- Performance: < 1s
`
		err := os.WriteFile(intentPath, []byte(content), 0644)
		if err != nil {
			t.Fatal(err)
		}

		v := NewValidator(intentPath)
		result, err := v.Validate()
		if err != nil {
			t.Fatalf("Validate failed: %v", err)
		}

		if !result.Valid {
			t.Errorf("expected valid result, got critical issues: %v", result.Critical)
		}

		if result.Summary.Title != "Test Project" {
			t.Errorf("got title %q, want %q", result.Summary.Title, "Test Project")
		}
		if result.Summary.GoalsCount != 2 {
			t.Errorf("got %d goals, want 2", result.Summary.GoalsCount)
		}
	})

	t.Run("Missing Title", func(t *testing.T) {
		content := `## Goals
- Goal 1`
		err := os.WriteFile(intentPath, []byte(content), 0644)
		if err != nil {
			t.Fatal(err)
		}

		v := NewValidator(intentPath)
		result, _ := v.Validate()
		if result.Valid {
			t.Error("expected invalid result for missing title")
		}
		if !hasRule(result.Critical, "title") {
			t.Error("expected critical issue for 'title'")
		}
	})

	t.Run("Missing Sections", func(t *testing.T) {
		content := `# Title
## Only Goals
- Goal 1`
		err := os.WriteFile(intentPath, []byte(content), 0644)
		if err != nil {
			t.Fatal(err)
		}

		v := NewValidator(intentPath)
		result, _ := v.Validate()
		if result.Valid {
			t.Error("expected invalid result for missing sections")
		}
		if !hasRule(result.Critical, "required_section") {
			t.Error("expected critical issue for 'required_section'")
		}
	})

	t.Run("Missing Constraints Content", func(t *testing.T) {
		content := `# Title
## Goals
- Goal 1
## Requirements
- Req 1
## Constraints
- Platform: macOS
`
		// Missing shape and data source
		err := os.WriteFile(intentPath, []byte(content), 0644)
		if err != nil {
			t.Fatal(err)
		}

		v := NewValidator(intentPath)
		result, _ := v.Validate()
		if !hasRule(result.Critical, "shape_missing") {
			t.Error("expected critical issue for 'shape_missing'")
		}
		if !hasRule(result.Critical, "data_source_missing") {
			t.Error("expected critical issue for 'data_source_missing'")
		}
	})
}

func hasRule(issues []ValidationIssue, rule string) bool {
	for _, issue := range issues {
		if issue.Rule == rule {
			return true
		}
	}
	return false
}

func TestNormalizeSectionName(t *testing.T) {
	v := NewValidator("")
	tests := []struct {
		input    string
		expected string
	}{
		{"Goals", "goals"},
		{"User Stories", "user stories"},
		{"Non-functional", "non functional"},
		{"  Trim  ", "trim"},
	}

	for _, tt := range tests {
		if got := v.normalizeSectionName(tt.input); got != tt.expected {
			t.Errorf("normalizeSectionName(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

func TestStoryQuality(t *testing.T) {
	tmpDir := t.TempDir()
	intentPath := filepath.Join(tmpDir, "INTENT.md")

	content := `# Title
## Goals
- Goal 1
## Requirements
### A Vague Story
It should work properly.
## Constraints
Platform: macOS, Shape: CLI, Data Source: API
`
	err := os.WriteFile(intentPath, []byte(content), 0644)
	if err != nil {
		t.Fatal(err)
	}

	v := NewValidator(intentPath)
	result, _ := v.Validate()

	if !hasRule(result.Warnings, "story_ids") {
		t.Error("expected warning for missing story IDs")
	}
	if !hasRule(result.Warnings, "vague_criteria") {
		t.Error("expected warning for vague criteria")
	}
}

func TestFormatting(t *testing.T) {
	tmpDir := t.TempDir()
	intentPath := filepath.Join(tmpDir, "INTENT.md")

	content := `# Title
# Duplicate Title
## Goals
## Goals
`
	err := os.WriteFile(intentPath, []byte(content), 0644)
	if err != nil {
		t.Fatal(err)
	}

	v := NewValidator(intentPath)
	result, _ := v.Validate()

	if !hasRule(result.Warnings, "header_levels") {
		t.Error("expected warning for duplicate level-1 header")
	}
	if !hasRule(result.Warnings, "duplicate_section") {
		t.Error("expected warning for duplicate section")
	}
}
