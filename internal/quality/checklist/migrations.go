package checklist

import (
	"bufio"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// MigrationsChecker verifies that every DB migration has a human
// description and a reversible down path.
type MigrationsChecker struct{}

// ID implements Checker.
func (MigrationsChecker) ID() string { return "migrations_reversible" }

var migrationDirs = []string{"drizzle", "migrations", "db/migrations"}

// Run implements Checker.
func (c MigrationsChecker) Run(root string) ([]Finding, error) {
	var findings []Finding

	for _, rel := range migrationDirs {
		dir := filepath.Join(root, rel)
		stat, err := os.Stat(dir)
		if err != nil || !stat.IsDir() {
			continue
		}

		// Collect sql files and detect down markers.
		var upFiles []string
		downNames := map[string]bool{}
		hasDrizzleMeta := false

		walkErr := filepath.WalkDir(dir, func(path string, d fs.DirEntry, werr error) error {
			if werr != nil {
				return nil
			}
			if d.IsDir() {
				if d.Name() == "meta" {
					hasDrizzleMeta = true
				}
				return nil
			}
			base := d.Name()
			if !strings.HasSuffix(base, ".sql") {
				return nil
			}
			name := strings.TrimSuffix(base, ".sql")
			if strings.HasSuffix(name, "_down") || strings.HasSuffix(name, ".down") {
				downNames[strings.TrimSuffix(strings.TrimSuffix(name, "_down"), ".down")] = true
				return nil
			}
			upFiles = append(upFiles, path)
			return nil
		})
		if walkErr != nil {
			return nil, walkErr
		}

		sort.Strings(upFiles)

		for _, path := range upFiles {
			relPath, _ := filepath.Rel(root, path)
			relPath = filepath.ToSlash(relPath)

			hasDesc, hasDown, err := inspectMigration(path)
			if err != nil {
				continue
			}

			if !hasDesc {
				findings = append(findings, Finding{
					Check:    "migrations_reversible",
					Severity: SeverityWarn,
					File:     relPath,
					Message:  "migration lacks a description comment (first-line comment explaining intent)",
				})
			}

			// If the migration doesn't have an inline down section,
			// look for a paired _down.sql, or a drizzle meta folder.
			if !hasDown {
				base := strings.TrimSuffix(filepath.Base(path), ".sql")
				if downNames[base] {
					continue
				}
				if hasDrizzleMeta {
					continue
				}
				findings = append(findings, Finding{
					Check:    "migrations_reversible",
					Severity: SeverityHigh,
					File:     relPath,
					Message:  fmt.Sprintf("migration %q has no down path (expected %s_down.sql, drizzle meta, or `-- down:` section)", base, base),
				})
			}
		}
	}

	return findings, nil
}

// inspectMigration returns (hasDescription, hasDown).
//   - A description is a non-empty `--` or `/* */` comment that is the
//     first non-blank line of the file.
//   - A down section is any line containing `-- down:` or `// down:`.
func inspectMigration(path string) (bool, bool, error) {
	f, err := os.Open(path)
	if err != nil {
		return false, false, err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)

	var hasDesc, hasDown bool
	firstNonBlankChecked := false

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !firstNonBlankChecked {
			if line == "" {
				continue
			}
			firstNonBlankChecked = true
			if strings.HasPrefix(line, "--") {
				text := strings.TrimSpace(strings.TrimPrefix(line, "--"))
				if text != "" {
					hasDesc = true
				}
			} else if strings.HasPrefix(line, "/*") {
				text := strings.TrimSpace(strings.TrimPrefix(strings.TrimSuffix(line, "*/"), "/*"))
				if text != "" {
					hasDesc = true
				}
			}
		}
		lower := strings.ToLower(line)
		if strings.Contains(lower, "-- down:") || strings.Contains(lower, "// down:") {
			hasDown = true
		}
	}
	return hasDesc, hasDown, scanner.Err()
}
