package nostubs

import (
	"path/filepath"
	"regexp"
	"strings"
)

// Rule describes a single detection rule.
type Rule struct {
	ID          string
	Severity    Severity
	Description string
	// Languages limits the rule to specific scanners ("ts", "go", "py").
	// Empty means all languages.
	Languages []string
}

// Rule IDs as exported constants for programmatic filtering.
const (
	RuleMockConstants       = "mock_constants"
	RuleMockImports         = "mock_imports"
	RuleTodoComments        = "todo_comments"
	RuleHandlerNoDBImport   = "handler_no_db_import"
	RuleHandlerHardcoded    = "handler_hardcoded_return"
	RuleServerNoAwait       = "server_no_await"
	RulePromiseResolveLit   = "promise_resolve_literal"
	RuleHardcodedArray      = "hardcoded_array"
	RuleEmptyHandler        = "empty_handler"
	RuleNotImplementedThrow = "not_implemented_throw"
)

// AllRules lists every rule with its default severity and description.
var AllRules = []Rule{
	{RuleMockConstants, SeverityHigh, "Detects constants whose names announce they are mocks or placeholders.", nil},
	{RuleMockImports, SeverityHigh, "Detects imports pulled from __mocks__/fixtures/mock* modules in production code.", []string{"ts"}},
	{RuleTodoComments, SeverityWarn, "Detects TODO/FIXME/HACK/XXX/stub/placeholder comments in production files.", nil},
	{RuleHandlerNoDBImport, SeverityHigh, "Detects HTTP handlers that never import a database driver.", nil},
	{RuleHandlerHardcoded, SeverityHigh, "Detects handlers that return hardcoded literal objects.", nil},
	{RuleServerNoAwait, SeverityWarn, "Detects SvelteKit +server.ts files that contain no await.", []string{"ts"}},
	{RulePromiseResolveLit, SeverityWarn, "Detects Promise.resolve() calls that wrap literal objects.", []string{"ts"}},
	{RuleHardcodedArray, SeverityWarn, "Detects hardcoded arrays of >2 object literals.", nil},
	{RuleEmptyHandler, SeverityHigh, "Detects handlers whose body is just 'return null' or 'return {}'.", nil},
	{RuleNotImplementedThrow, SeverityHigh, "Detects 'not implemented' panics/throws/raises.", nil},
}

// severityFor returns the effective severity for a rule, applying overrides.
func severityFor(cfg Config, rule string, defaultSev Severity) Severity {
	if sev, ok := cfg.RuleOverrides[rule]; ok {
		return sev
	}
	return defaultSev
}

// ruleActive reports whether a rule should fire given the config.
func ruleActive(cfg Config, rule string) bool {
	if sev, ok := cfg.RuleOverrides[rule]; ok && sev == SeverityOff {
		return false
	}
	if len(cfg.RuleIDs) == 0 {
		return true
	}
	for _, id := range cfg.RuleIDs {
		if id == rule {
			return true
		}
	}
	return false
}

// --- shared regular expressions ---

var (
	// Rule 1: mock/sample/fake/dummy/stub constant names.
	reMockConstJS = regexp.MustCompile(`\b(?:const|let|var)\s+(mock|MOCK_|SAMPLE_|fake|dummy|STUB_|FAKE_|DUMMY_)\w*`)
	reMockConstGo = regexp.MustCompile(`\b(mock|MOCK_|SAMPLE_|fake|dummy|STUB_|FAKE_|DUMMY_)\w*`)
	reMockConstPy = regexp.MustCompile(`^\s*(mock|MOCK_|SAMPLE_|fake|dummy|STUB_|FAKE_|DUMMY_)\w*\s*=`)

	// Rule 2: mock imports. Matches TS/JS imports only.
	reMockImport = regexp.MustCompile(`(?:import|from)\s+.*?['"]([^'"]*(?:__mocks__|/mocks/|/fixtures/|[/.]mock[A-Z][\w]*))['"]`)

	// Rule 3: TODO/FIXME/HACK/XXX/stub/placeholder comments.
	reTodoSlash = regexp.MustCompile(`(?://|#)\s*(TODO|FIXME|HACK|XXX|stub|placeholder|STUB|PLACEHOLDER)\b`)

	// Rule 5: hardcoded literal returns from handlers.
	reHardcodedReturnJS = regexp.MustCompile(`return\s+(?:new\s+)?(?:Response\.)?(?:json|NextResponse\.json)?\s*\(?\s*\{[^{}]*(?::\s*['"][^'"]*['"]|:\s*\d)[^{}]*\}`)
	reHardcodedReturnPy = regexp.MustCompile(`return\s+\{[^{}]*(?::\s*['"][^'"]*['"]|:\s*\d)`)

	// Rule 7: Promise.resolve with literal object.
	rePromiseResolveLit = regexp.MustCompile(`Promise\.resolve\(\s*\{[^}]*:\s*['"\d]`)

	// Rule 8: hardcoded array of >2 object literals. Conservative: looks for
	// `[ { ... }, { ... }, { ... }` with at least three top-level `{`.
	reHardcodedArray = regexp.MustCompile(`\[\s*\{[^\[\]]*\},\s*\{[^\[\]]*\},\s*\{`)

	// Rule 9: empty handler body.
	reEmptyReturn = regexp.MustCompile(`^\s*return\s+(null|\{\s*\}|new\s+Response\(\s*null\s*\)|None)\s*;?\s*$`)

	// Rule 10: not-implemented panics/throws/raises.
	reNotImplemented = regexp.MustCompile(`(?i)(throw\s+new\s+Error\(['"]not[ _-]?implemented|panic\(['"]TODO|raise\s+NotImplementedError)`)

	// DB-ish import markers (rule 4).
	reDBImport = regexp.MustCompile(`(?i)\b(db|prisma|drizzle|sql|mongoose|gorm|sqlalchemy|pg|mysql2|sqlite3|pymongo|redis)\b`)

	// Handler detection.
	reHandlerJS = regexp.MustCompile(`(?:export\s+async\s+function\s+(GET|POST|PUT|DELETE|PATCH)\b|app\.(get|post|put|delete|patch)\()`)
	reHandlerGo = regexp.MustCompile(`func\s+\w+\s*\(\s*\w+\s+http\.ResponseWriter\s*,\s*\w+\s+\*http\.Request`)
	reHandlerPy = regexp.MustCompile(`@(app|router|bp|blueprint)\.(get|post|put|delete|patch|route)\b`)
)

// isStatelessHandlerPath reports whether the handler path is a stateless
// endpoint that legitimately has no database interaction — auth flows
// (login/logout/signup), health checks, and similar infrastructure routes.
// Used to suppress rules 4, 5, 6, 9 on these paths.
func isStatelessHandlerPath(path string) bool {
	lower := strings.ToLower(filepath.ToSlash(path))
	// SvelteKit / Next.js style
	if strings.Contains(lower, "/routes/api/auth/") ||
		strings.Contains(lower, "/routes/api/health") ||
		strings.Contains(lower, "/api/auth/") ||
		strings.Contains(lower, "/api/health") {
		return true
	}
	// Generic "health" or "healthz" endpoints
	base := filepath.Base(lower)
	dir := filepath.Base(filepath.Dir(lower))
	if dir == "health" || dir == "healthz" || dir == "readyz" || dir == "livez" {
		return true
	}
	if base == "health.ts" || base == "health.js" || base == "health.go" || base == "health.py" {
		return true
	}
	return false
}

// isHandlerFile reports whether the path looks like an HTTP handler file.
// Used by rules 4, 5, 9 to reduce noise.
func isHandlerFile(path, content string) bool {
	lower := strings.ToLower(filepath.ToSlash(path))

	// Explicitly NOT handlers: SvelteKit UI files, library helpers.
	if strings.HasSuffix(lower, "+page.svelte") ||
		strings.HasSuffix(lower, "+layout.svelte") ||
		strings.HasSuffix(lower, "+page.ts") ||
		strings.HasSuffix(lower, "+page.js") ||
		strings.HasSuffix(lower, "+layout.ts") ||
		strings.HasSuffix(lower, "+layout.js") {
		return false
	}
	if strings.Contains(lower, "/lib/") {
		return false
	}

	// SvelteKit / Remix / Next-style explicit handler files.
	if strings.HasSuffix(lower, "+server.ts") || strings.HasSuffix(lower, "+server.js") {
		return true
	}

	// Path-based detection — require /routes/api/ (API routes only, not UI
	// routes), or an explicit handlers/controllers directory.
	if strings.Contains(lower, "/routes/api/") ||
		strings.Contains(lower, "/handlers/") ||
		strings.Contains(lower, "/controllers/") {
		return true
	}
	// /api/ alone is ambiguous (could be a client SDK); require an adjacent
	// handler-signature regex match to confirm.
	if strings.Contains(lower, "/api/") &&
		(reHandlerJS.MatchString(content) || reHandlerGo.MatchString(content) || reHandlerPy.MatchString(content)) {
		return true
	}

	// Content-only detection: handler signature regex in any file.
	if reHandlerJS.MatchString(content) || reHandlerGo.MatchString(content) || reHandlerPy.MatchString(content) {
		return true
	}
	return false
}

// hasDBImport reports whether the file content imports anything that looks
// like a database driver.
func hasDBImport(content string) bool {
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "import") ||
			strings.HasPrefix(trimmed, "from ") ||
			strings.HasPrefix(trimmed, "require") ||
			strings.HasPrefix(trimmed, `"`) {
			if reDBImport.MatchString(trimmed) {
				return true
			}
		}
	}
	return false
}
