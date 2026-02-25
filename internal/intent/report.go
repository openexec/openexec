package intent

import (
	"encoding/json"
	"fmt"
	"strings"
)

// ReportFormat specifies the output format for validation reports.
type ReportFormat int

const (
	ReportFormatText ReportFormat = iota
	ReportFormatJSON
	ReportFormatCompact
)

// Reporter generates human-readable validation reports.
type Reporter struct {
	result *ValidationResult
	format ReportFormat
}

// NewReporter creates a reporter for the given validation result.
func NewReporter(result *ValidationResult) *Reporter {
	return &Reporter{
		result: result,
		format: ReportFormatText,
	}
}

// SetFormat sets the output format.
func (r *Reporter) SetFormat(format ReportFormat) *Reporter {
	r.format = format
	return r
}

// Generate produces the formatted report.
func (r *Reporter) Generate() string {
	switch r.format {
	case ReportFormatJSON:
		return r.generateJSON()
	case ReportFormatCompact:
		return r.generateCompact()
	default:
		return r.generateText()
	}
}

func (r *Reporter) generateText() string {
	var sb strings.Builder

	// Header
	sb.WriteString(fmt.Sprintf("Intent Validation: %s\n", r.result.FilePath))
	sb.WriteString(strings.Repeat("=", 60) + "\n\n")

	// Summary
	if r.result.Summary != nil {
		sb.WriteString("Document Summary:\n")
		if r.result.Summary.Title != "" {
			sb.WriteString(fmt.Sprintf("  Title: %s\n", r.result.Summary.Title))
		}
		sb.WriteString(fmt.Sprintf("  Goals: %d\n", r.result.Summary.GoalsCount))
		sb.WriteString(fmt.Sprintf("  Requirements/Stories: %d\n", r.result.Summary.RequirementsCount))
		sb.WriteString(fmt.Sprintf("  Constraints: %d\n", r.result.Summary.ConstraintsCount))
		if len(r.result.Summary.SectionsFound) > 0 {
			sb.WriteString(fmt.Sprintf("  Sections found: %s\n", strings.Join(r.result.Summary.SectionsFound, ", ")))
		}
		sb.WriteString("\n")
	}

	// Critical issues
	if len(r.result.Critical) > 0 {
		sb.WriteString("Critical Issues:\n")
		for _, issue := range r.result.Critical {
			sb.WriteString(r.formatIssue(issue))
		}
		sb.WriteString("\n")
	}

	// Warnings
	if len(r.result.Warnings) > 0 {
		sb.WriteString("Warnings:\n")
		for _, issue := range r.result.Warnings {
			sb.WriteString(r.formatIssue(issue))
		}
		sb.WriteString("\n")
	}

	// Info (only in verbose mode, but include count)
	if len(r.result.Info) > 0 {
		sb.WriteString(fmt.Sprintf("Info: %d optional improvements available\n\n", len(r.result.Info)))
	}

	// Overall result
	if r.result.Valid {
		sb.WriteString("Result: PASS (all critical checks passed)\n")
	} else {
		sb.WriteString(fmt.Sprintf("Result: FAIL (%d critical issue(s))\n", len(r.result.Critical)))
		sb.WriteString("\nHint: Run 'openexec doctor intent --fix' to scaffold missing sections\n")
	}

	return sb.String()
}

func (r *Reporter) generateCompact() string {
	var lines []string

	// Issues as checklist
	for _, issue := range r.result.Critical {
		lines = append(lines, fmt.Sprintf("[FAIL] %s: %s", issue.Section, issue.Message))
	}
	for _, issue := range r.result.Warnings {
		lines = append(lines, fmt.Sprintf("[WARN] %s: %s", issue.Section, issue.Message))
	}

	// Summary line
	if r.result.Valid {
		lines = append(lines, "")
		lines = append(lines, "PASS: All critical checks passed")
	} else {
		lines = append(lines, "")
		lines = append(lines, fmt.Sprintf("FAIL: %d critical issue(s)", len(r.result.Critical)))
		lines = append(lines, "Hint: openexec doctor intent --fix")
	}

	return strings.Join(lines, "\n")
}

func (r *Reporter) generateJSON() string {
	output := struct {
		FilePath string            `json:"file_path"`
		Valid    bool              `json:"valid"`
		Summary  *DocumentSummary  `json:"summary"`
		Critical []ValidationIssue `json:"critical"`
		Warnings []ValidationIssue `json:"warnings"`
		Info     []ValidationIssue `json:"info,omitempty"`
	}{
		FilePath: r.result.FilePath,
		Valid:    r.result.Valid,
		Summary:  r.result.Summary,
		Critical: r.result.Critical,
		Warnings: r.result.Warnings,
		Info:     r.result.Info,
	}

	data, _ := json.MarshalIndent(output, "", "  ")
	return string(data)
}

func (r *Reporter) formatIssue(issue ValidationIssue) string {
	var sb strings.Builder

	// Severity badge
	sb.WriteString(fmt.Sprintf("  [%s] ", issue.Severity))

	// Section context
	if issue.Section != "" {
		sb.WriteString(fmt.Sprintf("%s: ", issue.Section))
	}

	// Message
	sb.WriteString(issue.Message)

	// Line number
	if issue.Line > 0 {
		sb.WriteString(fmt.Sprintf(" (line %d)", issue.Line))
	}

	sb.WriteString("\n")

	// Hint
	if issue.Hint != "" {
		sb.WriteString(fmt.Sprintf("         Hint: %s\n", issue.Hint))
	}

	return sb.String()
}

// Fixer can scaffold missing sections in an INTENT.md file.
type Fixer struct {
	result   *ValidationResult
	filePath string
}

// NewFixer creates a fixer for the given validation result.
func NewFixer(result *ValidationResult) *Fixer {
	return &Fixer{
		result:   result,
		filePath: result.FilePath,
	}
}

// GenerateStubs returns content to add for missing sections.
func (f *Fixer) GenerateStubs() []SectionStub {
	var stubs []SectionStub

	for _, issue := range f.result.Critical {
		if issue.Rule == "required_section" || issue.Rule == "title" {
			stub := f.generateStubForSection(issue.Section)
			if stub.Content != "" {
				stubs = append(stubs, stub)
			}
		}
	}

	return stubs
}

// SectionStub represents content to add to the document.
type SectionStub struct {
	Section string
	Content string
	After   string // Insert after this section (empty = end of file)
}

func (f *Fixer) generateStubForSection(section string) SectionStub {
	switch section {
	case "Goals":
		return SectionStub{
			Section: "Goals",
			Content: `## Goals

<!-- TODO: Define the primary goals of this project -->
- [ ] Goal 1: Describe what you want to achieve
- [ ] Goal 2: Add more goals as needed

`,
		}
	case "Requirements/User Stories":
		return SectionStub{
			Section: "Requirements",
			Content: `## Requirements

<!-- TODO: Add user stories or requirements -->
### US-001: Example User Story

**As a** [user type],
**I want** [feature/capability],
**So that** [benefit/value].

#### Acceptance Criteria
- [ ] Given [context], when [action], then [expected result]

`,
			After: "Goals",
		}
	case "Constraints/Non-functional":
		return SectionStub{
			Section: "Constraints",
			Content: `## Constraints

<!-- TODO: Define non-functional requirements and constraints -->
- **Performance**: [e.g., Response time < 200ms]
- **Security**: [e.g., Must support OAuth 2.0]
- **Compatibility**: [e.g., Must work on Node.js 18+]

`,
			After: "Requirements",
		}
	default:
		return SectionStub{}
	}
}

// Preview returns what the fix would add without modifying the file.
func (f *Fixer) Preview() string {
	stubs := f.GenerateStubs()
	if len(stubs) == 0 {
		return "No sections to add."
	}

	var sb strings.Builder
	sb.WriteString("The following sections would be added:\n\n")

	for _, stub := range stubs {
		sb.WriteString(fmt.Sprintf("--- Add: %s ---\n", stub.Section))
		sb.WriteString(stub.Content)
		sb.WriteString("\n")
	}

	return sb.String()
}
