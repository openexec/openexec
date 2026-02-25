// Package intent provides INTENT.md validation for OpenExec projects.
package intent

import (
	"bufio"
	"fmt"
	"os"
	"regexp"
	"strings"
)

// Validator validates INTENT.md documents against structural and content rules.
type Validator struct {
	filePath string
	lines    []string
	sections map[string]*Section
	title    string
}

// Section represents a parsed section from the document.
type Section struct {
	Name       string
	Level      int
	StartLine  int
	EndLine    int
	Content    []string
	Items      []string // Bullet/numbered items
	SubSections []*Section
}

// ValidationResult holds the complete validation outcome.
type ValidationResult struct {
	FilePath string
	Valid    bool
	Critical []ValidationIssue
	Warnings []ValidationIssue
	Info     []ValidationIssue
	Summary  *DocumentSummary
}

// ValidationIssue represents a single validation problem.
type ValidationIssue struct {
	Rule       string
	Severity   Severity
	Message    string
	Line       int
	Hint       string
	Section    string
}

// DocumentSummary provides an overview of what was found.
type DocumentSummary struct {
	Title           string
	GoalsCount      int
	RequirementsCount int
	StoriesCount    int
	ConstraintsCount int
	SectionsFound   []string
	StoriesWithAC   int
	StoriesWithoutAC []string
}

// Severity levels for validation issues.
type Severity int

const (
	SeverityInfo Severity = iota
	SeverityWarning
	SeverityCritical
)

func (s Severity) String() string {
	switch s {
	case SeverityInfo:
		return "INFO"
	case SeverityWarning:
		return "WARN"
	case SeverityCritical:
		return "FAIL"
	default:
		return "UNKNOWN"
	}
}

// NewValidator creates a new validator for the given file.
func NewValidator(filePath string) *Validator {
	return &Validator{
		filePath: filePath,
		sections: make(map[string]*Section),
	}
}

// Validate runs all validation rules and returns the result.
func (v *Validator) Validate() (*ValidationResult, error) {
	if err := v.loadFile(); err != nil {
		return nil, err
	}

	v.parseDocument()

	result := &ValidationResult{
		FilePath: v.filePath,
		Valid:    true,
		Critical: []ValidationIssue{},
		Warnings: []ValidationIssue{},
		Info:     []ValidationIssue{},
		Summary:  v.buildSummary(),
	}

	// Run all validation rules
	v.validateTitle(result)
	v.validateRequiredSections(result)
	v.validateGoals(result)
	v.validateRequirements(result)
	v.validateConstraints(result)
	v.validateOptionalSections(result)
	v.validateStoryQuality(result)
	v.validateFormatting(result)

	// Set overall validity based on critical issues
	result.Valid = len(result.Critical) == 0

	return result, nil
}

func (v *Validator) loadFile() error {
	file, err := os.Open(v.filePath)
	if err != nil {
		return fmt.Errorf("failed to open %s: %w", v.filePath, err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		v.lines = append(v.lines, scanner.Text())
	}

	return scanner.Err()
}

func (v *Validator) parseDocument() {
	var currentSection *Section
	var currentLevel int

	headerRegex := regexp.MustCompile(`^(#{1,6})\s+(.+)$`)
	bulletRegex := regexp.MustCompile(`^\s*[-*+]\s+(.+)$`)
	numberedRegex := regexp.MustCompile(`^\s*\d+\.\s+(.+)$`)

	for i, line := range v.lines {
		lineNum := i + 1

		if matches := headerRegex.FindStringSubmatch(line); matches != nil {
			level := len(matches[1])
			name := strings.TrimSpace(matches[2])

			// Title is level 1
			if level == 1 && v.title == "" {
				v.title = name
				continue
			}

			// Close previous section
			if currentSection != nil {
				currentSection.EndLine = lineNum - 1
			}

			section := &Section{
				Name:      name,
				Level:     level,
				StartLine: lineNum,
				Content:   []string{},
				Items:     []string{},
			}

			// Normalize section name for lookup
			normalizedName := v.normalizeSectionName(name)

			if level == 2 {
				v.sections[normalizedName] = section
				currentSection = section
				currentLevel = level
			} else if currentSection != nil && level > currentLevel {
				currentSection.SubSections = append(currentSection.SubSections, section)
			}
		} else if currentSection != nil {
			currentSection.Content = append(currentSection.Content, line)

			// Extract bullet items
			if matches := bulletRegex.FindStringSubmatch(line); matches != nil {
				currentSection.Items = append(currentSection.Items, matches[1])
			} else if matches := numberedRegex.FindStringSubmatch(line); matches != nil {
				currentSection.Items = append(currentSection.Items, matches[1])
			}
		}
	}

	// Close last section
	if currentSection != nil {
		currentSection.EndLine = len(v.lines)
	}
}

func (v *Validator) normalizeSectionName(name string) string {
	name = strings.ToLower(name)
	name = strings.TrimSpace(name)
	// Remove common variations
	name = strings.ReplaceAll(name, "-", " ")
	name = strings.ReplaceAll(name, "_", " ")
	return name
}

func (v *Validator) findSection(names ...string) *Section {
	for _, name := range names {
		normalized := v.normalizeSectionName(name)
		if section, ok := v.sections[normalized]; ok {
			return section
		}
	}
	return nil
}

func (v *Validator) buildSummary() *DocumentSummary {
	summary := &DocumentSummary{
		Title:         v.title,
		SectionsFound: []string{},
	}

	for name, section := range v.sections {
		summary.SectionsFound = append(summary.SectionsFound, name)

		switch {
		case strings.Contains(name, "goal"):
			summary.GoalsCount = len(section.Items)
		case strings.Contains(name, "requirement"):
			summary.RequirementsCount = len(section.Items)
		case strings.Contains(name, "user stor") || strings.Contains(name, "stories"):
			summary.StoriesCount = len(section.SubSections)
			if summary.StoriesCount == 0 {
				summary.StoriesCount = len(section.Items)
			}
		case strings.Contains(name, "constraint") || strings.Contains(name, "non functional"):
			summary.ConstraintsCount = len(section.Items)
		}
	}

	return summary
}

// Validation rules

func (v *Validator) validateTitle(result *ValidationResult) {
	if v.title == "" {
		result.Critical = append(result.Critical, ValidationIssue{
			Rule:     "title",
			Severity: SeverityCritical,
			Message:  "Document title not found",
			Line:     1,
			Hint:     "Add a level-1 heading at the top: # Project Intent",
		})
	}
}

func (v *Validator) validateRequiredSections(result *ValidationResult) {
	requiredSections := []struct {
		names []string
		label string
	}{
		{[]string{"Goals", "Objectives"}, "Goals"},
		{[]string{"Requirements", "User Stories", "Stories", "Features"}, "Requirements/User Stories"},
		{[]string{"Constraints", "Non-functional", "Non-functional Requirements", "NFR"}, "Constraints/Non-functional"},
	}

	for _, req := range requiredSections {
		section := v.findSection(req.names...)
		if section == nil {
			result.Critical = append(result.Critical, ValidationIssue{
				Rule:     "required_section",
				Severity: SeverityCritical,
				Message:  fmt.Sprintf("%s section not found", req.label),
				Hint:     fmt.Sprintf("Add a \"## %s\" section with at least one item", req.names[0]),
				Section:  req.label,
			})
		}
	}
}

func (v *Validator) validateGoals(result *ValidationResult) {
	section := v.findSection("Goals", "Objectives")
	if section == nil {
		return // Already reported in required sections
	}

	if len(section.Items) == 0 {
		result.Critical = append(result.Critical, ValidationIssue{
			Rule:     "goals_content",
			Severity: SeverityCritical,
			Message:  "Goals section is empty",
			Line:     section.StartLine,
			Hint:     "Add at least one goal as a bullet point under ## Goals",
			Section:  "Goals",
		})
	}

	result.Summary.GoalsCount = len(section.Items)
}

func (v *Validator) validateRequirements(result *ValidationResult) {
	section := v.findSection("Requirements", "User Stories", "Stories", "Features")
	if section == nil {
		return
	}

	itemCount := len(section.Items) + len(section.SubSections)
	if itemCount == 0 {
		result.Critical = append(result.Critical, ValidationIssue{
			Rule:     "requirements_content",
			Severity: SeverityCritical,
			Message:  "Requirements section is empty",
			Line:     section.StartLine,
			Hint:     "Add at least one requirement or user story",
			Section:  "Requirements",
		})
	}

	result.Summary.RequirementsCount = itemCount
}

func (v *Validator) validateConstraints(result *ValidationResult) {
	section := v.findSection("Constraints", "Non-functional", "Non-functional Requirements", "NFR")
	if section == nil {
		return
	}

	contentLower := strings.ToLower(strings.Join(v.lines, "\n"))

	// Mandatory: Platform check
	platforms := []string{"macos", "windows", "linux", "ios", "android", "web", "cross-platform", "docker"}
	hasPlatform := false
	for _, p := range platforms {
		if strings.Contains(contentLower, p) {
			hasPlatform = true
			break
		}
	}

	if !hasPlatform {
		result.Critical = append(result.Critical, ValidationIssue{
			Rule:     "platform_missing",
			Severity: SeverityCritical,
			Message:  "Target platform not explicitly defined",
			Hint:     "Add target platform (e.g., macOS, Linux, Web, Docker) to Constraints or Requirements",
			Section:  "Constraints",
		})
	}

	// Mandatory: App Shape check
	shapes := []string{"cli", "web app", "mobile app", "desktop app", "api", "library", "plugin", "microservice"}
	hasShape := false
	for _, s := range shapes {
		if strings.Contains(contentLower, s) {
			hasShape = true
			break
		}
	}

	if !hasShape {
		result.Critical = append(result.Critical, ValidationIssue{
			Rule:     "shape_missing",
			Severity: SeverityCritical,
			Message:  "Application shape/type not defined",
			Hint:     "Specify if this is a CLI, Web App, Mobile App, API, etc.",
			Section:  "Constraints",
		})
	}

	// Mandatory: Data Source Mapping check
	dataKeywords := []string{"data source", "source of truth", "database", "supabase", "postgres", "storage", "api", "external service"}
	hasDataSource := false
	for _, k := range dataKeywords {
		if strings.Contains(contentLower, k) {
			hasDataSource = true
			break
		}
	}

	if !hasDataSource {
		result.Critical = append(result.Critical, ValidationIssue{
			Rule:     "data_source_missing",
			Severity: SeverityCritical,
			Message:  "Data source mapping / source of truth not defined",
			Hint:     "Define where your data lives (e.g., 'Users stored in Supabase Auth', 'Logs in local SQLite')",
			Section:  "Requirements",
		})
	}

	if len(section.Items) == 0 {
		result.Critical = append(result.Critical, ValidationIssue{
			Rule:     "constraints_content",
			Severity: SeverityCritical,
			Message:  "Constraints section is empty",
			Line:     section.StartLine,
			Hint:     "Add at least one constraint (performance, security, compatibility, etc.)",
			Section:  "Constraints",
		})
	}

	result.Summary.ConstraintsCount = len(section.Items)
}

func (v *Validator) validateOptionalSections(result *ValidationResult) {
	optionalSections := []string{
		"Risks",
		"Assumptions",
		"Out of Scope",
		"Milestones",
		"Open Questions",
		"Dependencies",
	}

	for _, name := range optionalSections {
		section := v.findSection(name)
		if section == nil {
			result.Info = append(result.Info, ValidationIssue{
				Rule:     "optional_section",
				Severity: SeverityInfo,
				Message:  fmt.Sprintf("Optional section \"%s\" not found", name),
				Hint:     fmt.Sprintf("Consider adding a \"## %s\" section for completeness", name),
				Section:  name,
			})
		} else if len(section.Items) == 0 && len(section.Content) <= 1 {
			result.Warnings = append(result.Warnings, ValidationIssue{
				Rule:     "empty_section",
				Severity: SeverityWarning,
				Message:  fmt.Sprintf("Section \"%s\" is empty", name),
				Line:     section.StartLine,
				Hint:     "Add content or remove the empty section",
				Section:  name,
			})
		}
	}
}

func (v *Validator) validateStoryQuality(result *ValidationResult) {
	section := v.findSection("User Stories", "Stories", "Requirements")
	if section == nil {
		return
	}

	storyIDRegex := regexp.MustCompile(`(?i)(US|USER|STORY|REQ|FR|NFR)[-_]?\d+`)
	acKeywords := []string{"acceptance criteria", "given", "when", "then", "criteria"}

	// Check for story IDs in content
	hasStoryIDs := false
	for _, line := range v.lines {
		if storyIDRegex.MatchString(line) {
			hasStoryIDs = true
			break
		}
	}

	if !hasStoryIDs && len(section.SubSections) > 0 {
		result.Warnings = append(result.Warnings, ValidationIssue{
			Rule:     "story_ids",
			Severity: SeverityWarning,
			Message:  "No story IDs found (e.g., US-001, REQ-001)",
			Line:     section.StartLine,
			Hint:     "Consider using consistent story IDs for traceability",
			Section:  "User Stories",
		})
	}

	// Check for acceptance criteria
	contentLower := strings.ToLower(strings.Join(v.lines, "\n"))
	hasAC := false
	for _, keyword := range acKeywords {
		if strings.Contains(contentLower, keyword) {
			hasAC = true
			break
		}
	}

	if len(section.SubSections) > 0 && !hasAC {
		result.Warnings = append(result.Warnings, ValidationIssue{
			Rule:     "acceptance_criteria",
			Severity: SeverityWarning,
			Message:  "No acceptance criteria found for stories",
			Line:     section.StartLine,
			Hint:     "Add acceptance criteria using Given/When/Then format or bullet points",
			Section:  "User Stories",
		})
	}

	// Check for vague criteria
	vaguePatterns := []string{"it works", "works correctly", "should work", "functions properly"}
	for _, pattern := range vaguePatterns {
		if strings.Contains(contentLower, pattern) {
			result.Warnings = append(result.Warnings, ValidationIssue{
				Rule:     "vague_criteria",
				Severity: SeverityWarning,
				Message:  fmt.Sprintf("Vague acceptance criteria detected: \"%s\"", pattern),
				Hint:     "Replace with specific, testable criteria",
				Section:  "Acceptance Criteria",
			})
			break
		}
	}
}

func (v *Validator) validateFormatting(result *ValidationResult) {
	// Check for duplicate sections
	sectionCounts := make(map[string][]int)
	headerRegex := regexp.MustCompile(`^##\s+(.+)$`)

	for i, line := range v.lines {
		if matches := headerRegex.FindStringSubmatch(line); matches != nil {
			name := v.normalizeSectionName(matches[1])
			sectionCounts[name] = append(sectionCounts[name], i+1)
		}
	}

	for name, lines := range sectionCounts {
		if len(lines) > 1 {
			result.Warnings = append(result.Warnings, ValidationIssue{
				Rule:     "duplicate_section",
				Severity: SeverityWarning,
				Message:  fmt.Sprintf("Duplicate section \"%s\" found at lines %v", name, lines),
				Line:     lines[0],
				Hint:     "Merge duplicate sections into one",
				Section:  name,
			})
		}
	}

	// Check for mixed header levels within same logical section
	// (This is a simplified check)
	inSection := false
	sectionLevel := 0
	for i, line := range v.lines {
		if strings.HasPrefix(line, "## ") {
			inSection = true
			sectionLevel = 2
		} else if strings.HasPrefix(line, "### ") && inSection {
			// Subsection is fine
		} else if strings.HasPrefix(line, "# ") && i > 0 {
			result.Warnings = append(result.Warnings, ValidationIssue{
				Rule:     "header_levels",
				Severity: SeverityWarning,
				Message:  "Multiple level-1 headers found",
				Line:     i + 1,
				Hint:     "Document should have only one # title; use ## for sections",
			})
		}
		_ = sectionLevel // Suppress unused warning
	}
}
