package checklist

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeFile(t *testing.T, root, rel, content string) {
	t.Helper()
	path := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", rel, err)
	}
}

func TestEnvVarsChecker_FlagsUndocumented(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, ".env.example", "DATABASE_URL=postgres://x\n")
	writeFile(t, root, "src/server.ts", `
const a = process.env.DATABASE_URL;
const b = process.env.STRIPE_SECRET;
const c = import.meta.env.PUBLIC_API;
`)

	c := EnvVarsChecker{}
	got, err := c.Run(root)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	names := map[string]bool{}
	for _, f := range got {
		if f.Severity != SeverityHigh {
			t.Errorf("expected HIGH, got %s", f.Severity)
		}
		for _, n := range []string{"STRIPE_SECRET", "PUBLIC_API", "DATABASE_URL"} {
			if strings.Contains(f.Message, `"`+n+`"`) {
				names[n] = true
			}
		}
	}
	if !names["STRIPE_SECRET"] {
		t.Errorf("expected STRIPE_SECRET finding, got %+v", got)
	}
	if !names["PUBLIC_API"] {
		t.Errorf("expected PUBLIC_API finding, got %+v", got)
	}
	if names["DATABASE_URL"] {
		t.Errorf("did not expect DATABASE_URL finding, got %+v", got)
	}
}

func TestEnvVarsChecker_MissingEnvExampleIsFinding(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "src/server.ts", "const x = process.env.ANYTHING;\n")
	c := EnvVarsChecker{}
	got, err := c.Run(root)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if len(got) != 1 || got[0].Severity != SeverityHigh {
		t.Errorf("expected single HIGH finding for missing .env.example, got %+v", got)
	}
}

func TestSecretsChecker_DetectsPatterns(t *testing.T) {
	root := t.TempDir()
	// Intentional bait strings — not real secrets, but match the regexes.
	writeFile(t, root, "src/leak.ts", "const k = 'AKIA"+"ABCDEFGHIJKLMNOP';\n")
	writeFile(t, root, "src/leak2.ts", "const g = 'ghp_"+strings.Repeat("a", 36)+"';\n")

	c := SecretsChecker{}
	got, err := c.Run(root)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if len(got) < 2 {
		t.Errorf("expected at least 2 findings, got %d: %+v", len(got), got)
	}
	for _, f := range got {
		if f.Severity != SeverityHigh {
			t.Errorf("expected HIGH, got %s", f.Severity)
		}
	}
}

func TestMigrationsChecker_FlagsMissingDown(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "drizzle/0001_init.sql", "-- create users table\nCREATE TABLE users (id int);\n")
	writeFile(t, root, "drizzle/0002_posts.sql", "CREATE TABLE posts (id int);\n")
	// Paired down file for 0001 makes it pass the down check.
	writeFile(t, root, "drizzle/0001_init_down.sql", "DROP TABLE users;\n")

	c := MigrationsChecker{}
	got, err := c.Run(root)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	var highOn0002, warnOn0002 bool
	for _, f := range got {
		if strings.Contains(f.File, "0002_posts.sql") {
			if f.Severity == SeverityHigh {
				highOn0002 = true
			}
			if f.Severity == SeverityWarn {
				warnOn0002 = true
			}
		}
		if strings.Contains(f.File, "0001_init.sql") && f.Severity == SeverityHigh {
			t.Errorf("0001 should not have HIGH finding (has paired _down.sql): %+v", f)
		}
	}
	if !highOn0002 {
		t.Errorf("expected HIGH finding on 0002 for missing down path, got %+v", got)
	}
	if !warnOn0002 {
		t.Errorf("expected WARN finding on 0002 for missing description, got %+v", got)
	}
}

func TestAuthAnnotationsChecker_FlagsMissingSessionCheck(t *testing.T) {
	root := t.TempDir()
	// Public/excluded paths — should not flag.
	writeFile(t, root, "src/routes/api/auth/login/+server.ts",
		"export async function POST() { return new Response('ok'); }\n")
	writeFile(t, root, "src/routes/api/health/+server.ts",
		"export async function GET() { return new Response('ok'); }\n")
	// Non-public handler without session check — should flag.
	writeFile(t, root, "src/routes/api/posts/+server.ts",
		"export async function POST({ request }) { return new Response('ok'); }\n")
	// Non-public handler with session check — should pass.
	writeFile(t, root, "src/routes/api/posts/[id]/+server.ts",
		"export const GET = async ({ locals }) => { if (!locals.user) return new Response('no', { status: 401 }); return new Response('ok'); };\n")

	c := AuthAnnotationsChecker{}
	got, err := c.Run(root)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 finding, got %d: %+v", len(got), got)
	}
	if !strings.Contains(got[0].File, "api/posts/+server.ts") {
		t.Errorf("unexpected finding file: %+v", got[0])
	}
	if got[0].Severity != SeverityHigh {
		t.Errorf("expected HIGH, got %s", got[0].Severity)
	}
}

func TestHealthCheckChecker_FindsHealthRoute(t *testing.T) {
	root := t.TempDir()
	// Missing.
	c := HealthCheckChecker{}
	got, err := c.Run(root)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if len(got) != 1 || got[0].Severity != SeverityHigh {
		t.Errorf("expected HIGH for missing health endpoint, got %+v", got)
	}

	// Present but no GET.
	writeFile(t, root, "src/routes/api/health/+server.ts",
		"export async function POST() { return new Response('x'); }\n")
	got, err = c.Run(root)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if len(got) != 1 || got[0].Severity != SeverityHigh {
		t.Errorf("expected HIGH for missing GET, got %+v", got)
	}

	// Present, GET, returns json().
	writeFile(t, root, "src/routes/api/health/+server.ts",
		"import { json } from '@sveltejs/kit';\nexport const GET = async () => json({ ok: true });\n")
	got, err = c.Run(root)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected no findings, got %+v", got)
	}
}

func TestGate_RunAllFailsOnHigh(t *testing.T) {
	root := t.TempDir()
	// Trigger HIGH from the health-check checker (no health endpoint).
	// Ensure .env.example exists so env_vars checker does not fire.
	writeFile(t, root, ".env.example", "\n")

	g := NewGate(root, Config{Skip: []string{
		"env_vars_documented",
		"no_committed_secrets",
		"migrations_reversible",
		"handlers_check_session",
	}})
	err := g.RunAll(context.Background())
	if err == nil {
		t.Fatal("expected gate to fail (missing health endpoint)")
	}
	if !strings.Contains(err.Error(), "production-ready") {
		t.Errorf("expected error to mention production-ready, got %v", err)
	}
}

func TestRun_SkipBypassesChecker(t *testing.T) {
	root := t.TempDir()
	findings, err := Run(root, Config{Skip: []string{
		"env_vars_documented",
		"no_committed_secrets",
		"migrations_reversible",
		"handlers_check_session",
		"health_endpoint_exists",
	}})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if len(findings) != 0 {
		t.Errorf("expected no findings when all skipped, got %+v", findings)
	}
}

func TestWriteMarkdown_SmokeTest(t *testing.T) {
	var buf bytes.Buffer
	findings := []Finding{
		{Check: "env_vars_documented", Severity: SeverityHigh, File: "src/a.ts", Line: 3, Message: "x"},
		{Check: "migrations_reversible", Severity: SeverityWarn, File: "drizzle/0001.sql", Message: "y"},
	}
	if err := WriteMarkdown(findings, &buf); err != nil {
		t.Fatalf("writemarkdown: %v", err)
	}
	s := buf.String()
	for _, needle := range []string{"HIGH", "WARN", "env_vars_documented", "migrations_reversible"} {
		if !strings.Contains(s, needle) {
			t.Errorf("markdown missing %q:\n%s", needle, s)
		}
	}
}
