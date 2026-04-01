package skills

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// ParseSkillFile reads a SKILL.md file and returns a Skill.
// The file format is YAML frontmatter between --- delimiters followed by
// markdown content.
func ParseSkillFile(path string) (*Skill, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read skill file %s: %w", path, err)
	}
	return ParseSkillContent(string(data), path)
}

// ParseSkillContent parses a SKILL.md string (frontmatter + body).
func ParseSkillContent(content, sourcePath string) (*Skill, error) {
	frontmatter, body, err := splitFrontmatter(content)
	if err != nil {
		// No frontmatter — treat entire content as body with minimal defaults.
		return &Skill{
			Content:    strings.TrimSpace(content),
			SourcePath: sourcePath,
			Enabled:    true,
		}, nil
	}

	var skill Skill
	if err := yaml.Unmarshal([]byte(frontmatter), &skill); err != nil {
		return nil, fmt.Errorf("parse skill frontmatter in %s: %w", sourcePath, err)
	}

	skill.Content = strings.TrimSpace(body)
	skill.SourcePath = sourcePath
	skill.Enabled = true

	return &skill, nil
}

// splitFrontmatter splits YAML frontmatter (between --- delimiters) from the
// remaining markdown body. Returns frontmatter, body, error.
func splitFrontmatter(content string) (string, string, error) {
	const delimiter = "---"

	trimmed := strings.TrimSpace(content)
	if !strings.HasPrefix(trimmed, delimiter) {
		return "", "", fmt.Errorf("no frontmatter delimiter found")
	}

	// Find the closing delimiter after the opening one.
	rest := trimmed[len(delimiter):]
	idx := strings.Index(rest, "\n"+delimiter)
	if idx < 0 {
		// Try with \r\n
		idx = strings.Index(rest, "\r\n"+delimiter)
	}
	if idx < 0 {
		return "", "", fmt.Errorf("no closing frontmatter delimiter found")
	}

	frontmatter := rest[:idx]
	// Skip past the closing delimiter line.
	afterDelimiter := rest[idx+1+len(delimiter):]
	// Trim the leading newline from body if present.
	if strings.HasPrefix(afterDelimiter, "\n") {
		afterDelimiter = afterDelimiter[1:]
	} else if strings.HasPrefix(afterDelimiter, "\r\n") {
		afterDelimiter = afterDelimiter[2:]
	}

	return strings.TrimSpace(frontmatter), afterDelimiter, nil
}
