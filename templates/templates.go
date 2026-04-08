// Package templates embeds first-party project scaffolds for `openexec init --template`.
package templates

import (
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

//go:embed all:sveltekit-postgres
var templatesFS embed.FS

// Available returns the list of known template names.
func Available() []string {
	return []string{"sveltekit-postgres"}
}

// Exists reports whether a template with the given name is bundled.
func Exists(name string) bool {
	for _, n := range Available() {
		if n == name {
			return true
		}
	}
	return false
}

// CopyTemplate copies every file embedded under the given template name into
// dest. The template name is stripped from the destination paths, so
// `sveltekit-postgres/package.json` lands at `<dest>/package.json`.
//
// Existing files are NOT overwritten; an error is returned listing conflicts
// unless the destination file is identical. Parent directories are created
// as needed.
func CopyTemplate(name, dest string) error {
	if !Exists(name) {
		return fmt.Errorf("unknown template %q (available: %s)", name, strings.Join(Available(), ", "))
	}

	root := name
	return fs.WalkDir(templatesFS, root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		target := filepath.Join(dest, rel)

		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}

		data, err := templatesFS.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read embedded %s: %w", path, err)
		}

		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}

		if _, statErr := os.Stat(target); statErr == nil {
			// Don't clobber existing files.
			return nil
		} else if !errors.Is(statErr, os.ErrNotExist) {
			return statErr
		}

		return os.WriteFile(target, data, 0o644)
	})
}
