package intent

import (
	"strings"
	"testing"
)

func TestReporter(t *testing.T) {
	result := &ValidationResult{
		FilePath: "INTENT.md",
		Valid:    false,
		Summary: &DocumentSummary{
			Title: "Test",
		},
		Critical: []ValidationIssue{
			{Rule: "required_section", Severity: SeverityCritical, Message: "Goals missing", Section: "Goals"},
		},
		Warnings: []ValidationIssue{
			{Rule: "story_ids", Severity: SeverityWarning, Message: "No IDs", Section: "User Stories"},
		},
	}

	t.Run("Text Format", func(t *testing.T) {
		r := NewReporter(result).SetFormat(ReportFormatText)
		output := r.Generate()
		if !strings.Contains(output, "Intent Validation") {
			t.Error("missing header")
		}
		if !strings.Contains(output, "Goals missing") {
			t.Error("missing critical issue")
		}
	})

	t.Run("Compact Format", func(t *testing.T) {
		r := NewReporter(result).SetFormat(ReportFormatCompact)
		output := r.Generate()
		if !strings.Contains(output, "[FAIL] Goals: Goals missing") {
			t.Error("missing fail line")
		}
		if !strings.Contains(output, "[WARN] User Stories: No IDs") {
			t.Error("missing warn line")
		}
	})

	t.Run("JSON Format", func(t *testing.T) {
		r := NewReporter(result).SetFormat(ReportFormatJSON)
		output := r.Generate()
		if !strings.Contains(output, `"valid": false`) {
			t.Error("missing valid field in JSON")
		}
		if !strings.Contains(output, `"critical"`) {
			t.Error("missing critical field in JSON")
		}
	})
}

func TestFixer(t *testing.T) {
	result := &ValidationResult{
		FilePath: "INTENT.md",
		Critical: []ValidationIssue{
			{Rule: "required_section", Section: "Goals"},
			{Rule: "required_section", Section: "Requirements/User Stories"},
		},
	}

	f := NewFixer(result)
	stubs := f.GenerateStubs()

	if len(stubs) != 2 {
		t.Errorf("got %d stubs, want 2", len(stubs))
	}

	var foundGoals bool
	for _, stub := range stubs {
		if stub.Section == "Goals" {
			foundGoals = true
			if !strings.Contains(stub.Content, "## Goals") {
				t.Error("stub content missing heading")
			}
		}
	}
	if !foundGoals {
		t.Error("missing Goals stub")
	}

	preview := f.Preview()
	if !strings.Contains(preview, "--- Add: Goals ---") {
		t.Error("preview missing Goals section")
	}
}
