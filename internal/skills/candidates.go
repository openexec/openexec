package skills

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Candidate skills are agent-proposed skills awaiting human review. They live
// under <projectDir>/.openexec/skills/_candidates/<name>/SKILL.md and are
// NEVER loaded by the registry (LoadFromDir skips "_"-prefixed entries) until
// a human approves them with `openexec skills approve <name>`, which moves
// the directory into the active project skills location.
//
// This is the propose-then-approve loop: agents compound their learnings into
// durable skills, but an unsupervised agent can never poison future runs with
// a wrong lesson.

// candidateDirName is the reserved, registry-invisible directory for proposals.
const candidateDirName = "_candidates"

// skillNameRe constrains proposed skill names: kebab-case, no path separators,
// no leading underscore or dot. This is a security boundary — the name becomes
// a directory component.
var skillNameRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{1,63}$`)

// CandidatesDir returns the candidate-skill directory for a project.
func CandidatesDir(projectDir string) string {
	return filepath.Join(projectDir, ".openexec", "skills", candidateDirName)
}

// activeSkillDir returns the active project-skill directory for a name.
func activeSkillDir(projectDir, name string) string {
	return filepath.Join(projectDir, ".openexec", "skills", name)
}

// ValidateCandidateName reports whether a proposed skill name is acceptable.
func ValidateCandidateName(name string) error {
	if !skillNameRe.MatchString(name) {
		return fmt.Errorf("invalid skill name %q: use kebab-case (lowercase letters, digits, hyphens; 2-64 chars)", name)
	}
	return nil
}

// ProposeCandidate writes a candidate SKILL.md for human review. It refuses
// to overwrite an existing candidate or shadow an active project skill.
// Returns the path of the written file.
func ProposeCandidate(projectDir, name, description, whenToUse string, tags []string, content string) (string, error) {
	if err := ValidateCandidateName(name); err != nil {
		return "", err
	}
	if strings.TrimSpace(description) == "" {
		return "", fmt.Errorf("description is required")
	}
	if strings.TrimSpace(content) == "" {
		return "", fmt.Errorf("content is required")
	}

	if _, err := os.Stat(filepath.Join(activeSkillDir(projectDir, name), "SKILL.md")); err == nil {
		return "", fmt.Errorf("an active project skill named %q already exists — propose an update under a different name or edit it directly", name)
	}

	dir := filepath.Join(CandidatesDir(projectDir), name)
	path := filepath.Join(dir, "SKILL.md")
	if _, err := os.Stat(path); err == nil {
		return "", fmt.Errorf("a candidate named %q is already awaiting review (approve or reject it first)", name)
	}

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("create candidate directory: %w", err)
	}

	var sb strings.Builder
	sb.WriteString("---\n")
	fmt.Fprintf(&sb, "name: %s\n", name)
	fmt.Fprintf(&sb, "description: %q\n", description)
	if len(tags) > 0 {
		fmt.Fprintf(&sb, "tags: [%s]\n", strings.Join(tags, ", "))
	}
	if strings.TrimSpace(whenToUse) != "" {
		fmt.Fprintf(&sb, "when_to_use: %q\n", whenToUse)
	}
	sb.WriteString("---\n\n")
	sb.WriteString(strings.TrimSpace(content))
	sb.WriteString("\n")

	if err := os.WriteFile(path, []byte(sb.String()), 0o644); err != nil {
		return "", fmt.Errorf("write candidate skill: %w", err)
	}
	return path, nil
}

// ListCandidates returns parsed candidate skills awaiting review.
func ListCandidates(projectDir string) ([]*Skill, error) {
	dir := CandidatesDir(projectDir)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read candidates dir: %w", err)
	}

	var out []*Skill
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		path := filepath.Join(dir, entry.Name(), "SKILL.md")
		skill, err := ParseSkillFile(path)
		if err != nil {
			continue
		}
		if skill.Name == "" {
			skill.Name = entry.Name()
		}
		skill.Source = "candidate"
		out = append(out, skill)
	}
	return out, nil
}

// ApproveCandidate promotes a candidate into the active project skills
// directory, after which the registry loads it like any project skill.
func ApproveCandidate(projectDir, name string) (string, error) {
	if err := ValidateCandidateName(name); err != nil {
		return "", err
	}
	src := filepath.Join(CandidatesDir(projectDir), name)
	if _, err := os.Stat(filepath.Join(src, "SKILL.md")); err != nil {
		return "", fmt.Errorf("no candidate named %q awaiting review", name)
	}
	dst := activeSkillDir(projectDir, name)
	if _, err := os.Stat(dst); err == nil {
		return "", fmt.Errorf("active skill directory %s already exists", dst)
	}
	if err := os.Rename(src, dst); err != nil {
		return "", fmt.Errorf("promote candidate: %w", err)
	}
	return filepath.Join(dst, "SKILL.md"), nil
}

// RejectCandidate removes a candidate without activating it.
func RejectCandidate(projectDir, name string) error {
	if err := ValidateCandidateName(name); err != nil {
		return err
	}
	dir := filepath.Join(CandidatesDir(projectDir), name)
	if _, err := os.Stat(filepath.Join(dir, "SKILL.md")); err != nil {
		return fmt.Errorf("no candidate named %q awaiting review", name)
	}
	return os.RemoveAll(dir)
}
