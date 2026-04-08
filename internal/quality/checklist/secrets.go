package checklist

import (
	"bufio"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// SecretsChecker scans the tree for common committed-secret patterns.
type SecretsChecker struct{}

// ID implements Checker.
func (SecretsChecker) ID() string { return "no_committed_secrets" }

type secretPattern struct {
	name string
	re   *regexp.Regexp
}

var secretPatterns = []secretPattern{
	{name: "AWS access key", re: regexp.MustCompile(`AKIA[0-9A-Z]{16}`)},
	{name: "GitHub token", re: regexp.MustCompile(`ghp_[0-9A-Za-z]{36}`)},
	{name: "OpenAI-style key", re: regexp.MustCompile(`sk-[0-9A-Za-z]{48}`)},
	{name: "Slack token", re: regexp.MustCompile(`xox[baprs]-[0-9A-Za-z-]+`)},
	{name: "Private key block", re: regexp.MustCompile(`PRIVATE KEY-----`)},
	{name: "RSA private key", re: regexp.MustCompile(`-----BEGIN RSA PRIVATE KEY-----`)},
}

// Run implements Checker.
func (c SecretsChecker) Run(root string) ([]Finding, error) {
	var findings []Finding

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

		rel, _ := filepath.Rel(root, path)
		rel = filepath.ToSlash(rel)
		base := filepath.Base(rel)

		// Skip our own source (Go files referencing the regex literals).
		if strings.HasSuffix(base, "_test.go") {
			return nil
		}
		if base == ".env.example" {
			return nil
		}
		if strings.Contains(rel, "tests/") || strings.Contains(rel, "testdata/") {
			return nil
		}
		if strings.Contains(rel, "internal/quality/checklist/") {
			return nil
		}
		// Avoid scanning binary-ish files.
		if !shouldScanForSecrets(base) {
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
			for _, p := range secretPatterns {
				if p.re.MatchString(line) {
					findings = append(findings, Finding{
						Check:    "no_committed_secrets",
						Severity: SeverityHigh,
						File:     rel,
						Line:     lineNo,
						Message:  "possible " + p.name + " committed to repo",
					})
				}
			}
		}
		return nil
	})
	if walkErr != nil {
		return nil, walkErr
	}
	return findings, nil
}

// shouldScanForSecrets whitelists textual file types. We avoid blobs to
// keep the scan fast and deterministic.
func shouldScanForSecrets(name string) bool {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".ts", ".tsx", ".js", ".jsx", ".mjs", ".cjs", ".svelte", ".py",
		".go", ".json", ".yaml", ".yml", ".toml", ".env", ".md", ".sh",
		".rb", ".rs", ".java", ".kt", ".txt", ".ini", ".conf", ".cfg":
		return true
	}
	// Dotfiles like .env, .npmrc — scan if small.
	if strings.HasPrefix(name, ".env") {
		return true
	}
	return false
}
