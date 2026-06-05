package skills

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// PromoteSkill copies an active project skill into the user skills directory
// so its lessons apply across all projects (the user-level layer loads before
// project skills in LoadAll). The project keeps its copy — promotion shares a
// lesson, it does not move ownership. Refuses to overwrite an existing user
// skill of the same name.
//
// Returns the destination skill directory.
func PromoteSkill(projectDir, name, userSkillsDir string) (string, error) {
	if err := ValidateCandidateName(name); err != nil {
		return "", err
	}
	src := activeSkillDir(projectDir, name)
	if _, err := os.Stat(filepath.Join(src, "SKILL.md")); err != nil {
		return "", fmt.Errorf("no active project skill named %q (candidates must be approved first)", name)
	}
	dst := filepath.Join(userSkillsDir, name)
	if _, err := os.Stat(dst); err == nil {
		return "", fmt.Errorf("a user skill named %q already exists at %s", name, dst)
	}
	if err := copyDir(src, dst); err != nil {
		return "", fmt.Errorf("promote skill: %w", err)
	}
	return dst, nil
}

// copyDir recursively copies a directory tree (regular files only — skills
// are markdown, data files, and scripts; symlinks are deliberately skipped).
func copyDir(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		if !info.Mode().IsRegular() {
			return nil // skip symlinks and special files
		}
		in, err := os.Open(path)
		if err != nil {
			return err
		}
		defer in.Close()
		out, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, info.Mode().Perm())
		if err != nil {
			return err
		}
		defer out.Close()
		_, err = io.Copy(out, in)
		return err
	})
}
