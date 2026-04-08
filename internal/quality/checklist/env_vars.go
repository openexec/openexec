package checklist

import (
	"bufio"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// EnvVarsChecker verifies that every environment variable referenced in
// the codebase is documented in .env.example.
type EnvVarsChecker struct{}

// ID implements Checker.
func (EnvVarsChecker) ID() string { return "env_vars_documented" }

// envRefPatterns matches common ways of reading an env var from code.
// Each pattern has the env var name in the first capture group.
var envRefPatterns = []*regexp.Regexp{
	regexp.MustCompile(`process\.env\.([A-Z_][A-Z0-9_]*)`),
	regexp.MustCompile(`process\.env\[["']([A-Z_][A-Z0-9_]*)["']\]`),
	regexp.MustCompile(`import\.meta\.env\.([A-Z_][A-Z0-9_]*)`),
	regexp.MustCompile(`Deno\.env\.get\(["']([A-Z_][A-Z0-9_]*)["']\)`),
	regexp.MustCompile(`os\.environ\[["']([A-Z_][A-Z0-9_]*)["']\]`),
	regexp.MustCompile(`os\.environ\.get\(["']([A-Z_][A-Z0-9_]*)["']`),
	regexp.MustCompile(`os\.Getenv\(["']([A-Z_][A-Z0-9_]*)["']\)`),
}

// envBuiltins are env var names we never flag. These are universally
// available on any Unix-like system or enforced by the runtime itself.
var envBuiltins = map[string]bool{
	"PATH":     true,
	"HOME":     true,
	"USER":     true,
	"SHELL":    true,
	"PWD":      true,
	"NODE_ENV": true,
	"CI":       true,
	"TZ":       true,
}

// Run implements Checker.
func (c EnvVarsChecker) Run(root string) ([]Finding, error) {
	envPath := filepath.Join(root, ".env.example")
	documented, err := parseEnvExample(envPath)
	if err != nil {
		// Missing .env.example is itself a finding.
		if os.IsNotExist(err) {
			return []Finding{{
				Check:    "env_vars_documented",
				Severity: SeverityHigh,
				File:     ".env.example",
				Message:  ".env.example not found: env vars cannot be validated",
			}}, nil
		}
		return nil, err
	}

	type ref struct {
		file string
		line int
		name string
	}
	var refs []ref

	walkErr := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if defaultExcludeDirs[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		if !isCodeFile(d.Name()) {
			return nil
		}
		rel, _ := filepath.Rel(root, path)
		rel = filepath.ToSlash(rel)
		if strings.Contains(rel, "/tests/") || strings.Contains(rel, "/__tests__/") {
			return nil
		}

		f, err := os.Open(path)
		if err != nil {
			return nil
		}
		defer f.Close()

		scanner := bufio.NewScanner(f)
		scanner.Buffer(make([]byte, 64*1024), 1024*1024)
		lineNo := 0
		for scanner.Scan() {
			lineNo++
			line := scanner.Text()
			for _, re := range envRefPatterns {
				for _, m := range re.FindAllStringSubmatch(line, -1) {
					if len(m) < 2 {
						continue
					}
					name := m[1]
					if envBuiltins[name] {
						continue
					}
					refs = append(refs, ref{file: rel, line: lineNo, name: name})
				}
			}
		}
		return nil
	})
	if walkErr != nil {
		return nil, walkErr
	}

	// Deduplicate by (file, name) so each missing var is reported once
	// per file.
	seen := map[string]bool{}
	var findings []Finding
	for _, r := range refs {
		if documented[r.name] {
			continue
		}
		key := r.file + "|" + r.name
		if seen[key] {
			continue
		}
		seen[key] = true
		findings = append(findings, Finding{
			Check:    "env_vars_documented",
			Severity: SeverityHigh,
			File:     r.file,
			Line:     r.line,
			Message:  fmt.Sprintf("env var %q used but not documented in .env.example", r.name),
		})
	}

	sort.SliceStable(findings, func(i, j int) bool {
		if findings[i].File != findings[j].File {
			return findings[i].File < findings[j].File
		}
		return findings[i].Line < findings[j].Line
	})
	return findings, nil
}

// parseEnvExample returns the set of KEY names declared in a .env-style
// file. Comments (lines starting with `#`) and blank lines are ignored.
func parseEnvExample(path string) (map[string]bool, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	out := map[string]bool{}
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		// Strip optional `export ` prefix.
		line = strings.TrimPrefix(line, "export ")
		idx := strings.Index(line, "=")
		if idx <= 0 {
			continue
		}
		key := strings.TrimSpace(line[:idx])
		if key != "" {
			out[key] = true
		}
	}
	return out, scanner.Err()
}

// isCodeFile reports whether the file is one of the languages whose env
// patterns we know how to match.
func isCodeFile(name string) bool {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".ts", ".tsx", ".js", ".jsx", ".mjs", ".cjs", ".svelte", ".py", ".go":
		return true
	}
	return false
}
