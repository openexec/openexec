package checklist

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// HealthCheckChecker verifies that a health endpoint exists and
// returns a 200.
type HealthCheckChecker struct{}

// ID implements Checker.
func (HealthCheckChecker) ID() string { return "health_endpoint_exists" }

var healthGetRe = regexp.MustCompile(
	`export\s+(?:async\s+function|const)\s+GET\b`,
)

// Run implements Checker.
func (c HealthCheckChecker) Run(root string) ([]Finding, error) {
	candidates := []string{
		filepath.Join(root, "src", "routes", "api", "health", "+server.ts"),
		filepath.Join(root, "src", "routes", "api", "health", "+server.js"),
	}

	var found string
	for _, p := range candidates {
		if _, err := os.Stat(p); err == nil {
			found = p
			break
		}
	}
	if found == "" {
		return []Finding{{
			Check:    "health_endpoint_exists",
			Severity: SeverityHigh,
			File:     "src/routes/api/health/+server.ts",
			Message:  "health endpoint not found (expected src/routes/api/health/+server.ts)",
		}}, nil
	}

	rel, _ := filepath.Rel(root, found)
	rel = filepath.ToSlash(rel)

	data, err := os.ReadFile(found)
	if err != nil {
		return nil, err
	}
	body := string(data)

	if !healthGetRe.MatchString(body) {
		return []Finding{{
			Check:    "health_endpoint_exists",
			Severity: SeverityHigh,
			File:     rel,
			Message:  "health endpoint does not export a GET handler",
		}}, nil
	}

	// Heuristic for "returns 200":
	//   - Uses a helper like ok(...) from $lib/api/response, OR
	//   - Constructs a Response with explicit status 200 (or default), OR
	//   - Calls json(...) (SvelteKit's json() defaults to 200).
	returns200 := false
	patterns := []string{
		"ok(",
		"json(",
		"return new Response",
		"status: 200",
		"status: 204",
	}
	for _, p := range patterns {
		if strings.Contains(body, p) {
			returns200 = true
			break
		}
	}
	if !returns200 {
		return []Finding{{
			Check:    "health_endpoint_exists",
			Severity: SeverityWarn,
			File:     rel,
			Message:  "health endpoint GET handler: could not confirm a 200 response path",
		}}, nil
	}

	return nil, nil
}
