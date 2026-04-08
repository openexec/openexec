package checklist

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// AuthAnnotationsChecker verifies that every non-public SvelteKit API
// route handler references the session / auth machinery.
type AuthAnnotationsChecker struct{}

// ID implements Checker.
func (AuthAnnotationsChecker) ID() string { return "handlers_check_session" }

// Matches both `export async function GET(...)` and
// `export const GET: RequestHandler = async (...) => {`.
var handlerFuncRe = regexp.MustCompile(
	`export\s+(?:async\s+function|const)\s+(GET|POST|PUT|PATCH|DELETE)\b`,
)

var authSignals = []string{
	"locals.user",
	"locals.session",
	"requireAuth",
	"requireSession",
	"getSession",
}

// Run implements Checker.
func (c AuthAnnotationsChecker) Run(root string) ([]Finding, error) {
	apiDir := filepath.Join(root, "src", "routes", "api")
	if stat, err := os.Stat(apiDir); err != nil || !stat.IsDir() {
		return nil, nil
	}

	var findings []Finding

	walkErr := filepath.WalkDir(apiDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			return nil
		}
		base := filepath.Base(path)
		if base != "+server.ts" && base != "+server.js" {
			return nil
		}

		rel, _ := filepath.Rel(root, path)
		rel = filepath.ToSlash(rel)

		// Exclude auth and health subtrees.
		if strings.HasPrefix(rel, "src/routes/api/auth/") ||
			strings.HasPrefix(rel, "src/routes/api/health/") {
			return nil
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		content := string(data)

		// Find per-handler line numbers for reporting.
		lines := strings.Split(content, "\n")
		hasAuthSignal := false
		for _, sig := range authSignals {
			if strings.Contains(content, sig) {
				hasAuthSignal = true
				break
			}
		}

		for i, line := range lines {
			m := handlerFuncRe.FindStringSubmatch(line)
			if m == nil {
				continue
			}
			if !hasAuthSignal {
				findings = append(findings, Finding{
					Check:    "handlers_check_session",
					Severity: SeverityHigh,
					File:     rel,
					Line:     i + 1,
					Message: fmt.Sprintf(
						"%s handler does not reference locals.user / locals.session / requireAuth — session check appears missing",
						m[1],
					),
				})
			}
		}
		return nil
	})
	if walkErr != nil {
		return nil, walkErr
	}

	return findings, nil
}
