package skills

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ImportFromClaude copies skill directories from ~/.claude/skills/ into
// targetDir. Returns a list of imported skill names.
func ImportFromClaude(targetDir string) ([]string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("resolve home directory: %w", err)
	}
	claudeSkills := filepath.Join(home, ".claude", "skills")
	return ImportFromPath(claudeSkills, targetDir)
}

// ImportFromPath copies skill directories from sourcePath into targetDir.
// If a SKILL.md lacks OpenExec extensions (categories, tags), empty defaults
// are added. Returns a list of imported skill names.
func ImportFromPath(sourcePath, targetDir string) ([]string, error) {
	entries, err := os.ReadDir(sourcePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("source path %s does not exist", sourcePath)
		}
		return nil, fmt.Errorf("read source path %s: %w", sourcePath, err)
	}

	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		return nil, fmt.Errorf("create target dir %s: %w", targetDir, err)
	}

	var imported []string
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		skillFile := filepath.Join(sourcePath, entry.Name(), "SKILL.md")
		data, err := os.ReadFile(skillFile)
		if err != nil {
			continue // No SKILL.md, skip.
		}

		content := string(data)
		content = ensureOpenExecExtensions(content)

		destDir := filepath.Join(targetDir, entry.Name())
		if err := os.MkdirAll(destDir, 0o755); err != nil {
			continue
		}

		destFile := filepath.Join(destDir, "SKILL.md")
		if err := os.WriteFile(destFile, []byte(content), 0o644); err != nil {
			continue
		}

		imported = append(imported, entry.Name())
	}

	return imported, nil
}

// ensureOpenExecExtensions checks if a SKILL.md has categories and tags in
// its frontmatter. If the frontmatter is missing these fields, they are added
// with empty defaults.
func ensureOpenExecExtensions(content string) string {
	frontmatter, body, err := splitFrontmatter(content)
	if err != nil {
		// No frontmatter — wrap with a minimal one.
		return fmt.Sprintf("---\ncategories: []\ntags: []\n---\n%s", content)
	}

	modified := false
	if !strings.Contains(frontmatter, "categories:") {
		frontmatter += "\ncategories: []"
		modified = true
	}
	if !strings.Contains(frontmatter, "tags:") {
		frontmatter += "\ntags: []"
		modified = true
	}

	if !modified {
		return content
	}

	return fmt.Sprintf("---\n%s\n---\n%s", frontmatter, body)
}
