package nostubs

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeFile(t *testing.T, dir, name, body string) string {
	t.Helper()
	full := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	return full
}

func hasRule(findings []Finding, rule string) bool {
	for _, f := range findings {
		if f.Rule == rule {
			return true
		}
	}
	return false
}

func countRule(findings []Finding, rule string) int {
	n := 0
	for _, f := range findings {
		if f.Rule == rule {
			n++
		}
	}
	return n
}

func TestTSScannerDetectsStubs(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "src/routes/api/users/+server.ts", `
export async function GET() {
  const mockUsers = [{ id: 1 }, { id: 2 }, { id: 3 }];
  // TODO: wire up database
  return Response.json({ status: "ok", count: 3 });
}
`)
	writeFile(t, dir, "src/lib/promise.ts", `
export function get() {
  return Promise.resolve({ id: 1, name: "foo" });
}
`)
	writeFile(t, dir, "src/lib/thrown.ts", `
export function todo() {
  throw new Error("not implemented");
}
`)

	cfg := Config{Languages: []string{"ts"}}
	findings, err := Run(dir, cfg)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(findings) == 0 {
		t.Fatalf("expected findings, got none")
	}
	for _, rule := range []string{
		RuleMockConstants,
		RuleTodoComments,
		RuleHandlerNoDBImport,
		RuleHandlerHardcoded,
		RuleServerNoAwait,
		RulePromiseResolveLit,
		RuleHardcodedArray,
		RuleNotImplementedThrow,
	} {
		if !hasRule(findings, rule) {
			t.Errorf("expected rule %s to fire, findings: %v", rule, findings)
		}
	}
}

func TestGoScannerDetectsStubs(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "pkg/api/handler.go", `package api

import "net/http"

const mockUser = "alice"

// TODO: implement properly
func GetUser(w http.ResponseWriter, r *http.Request) {
	return
}

func NotDone() {
	panic("TODO: not implemented")
}
`)

	cfg := Config{Languages: []string{"go"}}
	findings, err := Run(dir, cfg)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	for _, rule := range []string{
		RuleMockConstants,
		RuleTodoComments,
		RuleHandlerNoDBImport,
		RuleEmptyHandler,
		RuleNotImplementedThrow,
	} {
		if !hasRule(findings, rule) {
			t.Errorf("go: expected rule %s to fire, findings: %+v", rule, findings)
		}
	}
}

func TestPythonScannerDetectsStubs(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "api/routes.py", `
from flask import Flask
app = Flask(__name__)

MOCK_USERS = [{"id": 1}, {"id": 2}, {"id": 3}]

@app.get("/users")
def get_users():
    # TODO: connect to db
    return {"status": "ok", "count": 3}

def not_done():
    raise NotImplementedError("later")
`)

	cfg := Config{Languages: []string{"py"}}
	findings, err := Run(dir, cfg)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	for _, rule := range []string{
		RuleMockConstants,
		RuleTodoComments,
		RuleHandlerNoDBImport,
		RuleHandlerHardcoded,
		RuleHardcodedArray,
		RuleNotImplementedThrow,
	} {
		if !hasRule(findings, rule) {
			t.Errorf("py: expected rule %s to fire, findings: %+v", rule, findings)
		}
	}
}

func TestTestFilesSuppressed(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "pkg/api/handler_test.go", `package api

const mockUser = "alice"
// TODO: later
`)
	findings, err := Run(dir, Config{Languages: []string{"go"}})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(findings) != 0 {
		t.Errorf("expected no findings in test files, got: %+v", findings)
	}
}

func TestRuleOverridesOff(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "src/lib/x.ts", `// TODO: fix me later`)
	cfg := Config{
		Languages:     []string{"ts"},
		RuleOverrides: map[string]Severity{RuleTodoComments: SeverityOff},
	}
	findings, err := Run(dir, cfg)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if countRule(findings, RuleTodoComments) != 0 {
		t.Errorf("expected todo_comments suppressed, got %+v", findings)
	}
}

func TestGateHighFailsRun(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "src/routes/api/+server.ts", `
export async function GET() {
  return Response.json({ status: "ok" });
}
`)
	g := NewGate(dir, Config{Languages: []string{"ts"}})
	report, err := g.Run(nil)
	if err != nil {
		t.Fatalf("gate run: %v", err)
	}
	if report.Passed {
		t.Errorf("expected gate to fail due to HIGH findings, got report: %+v", report)
	}
	if report.HighCount == 0 {
		t.Errorf("expected HighCount>0, got %d", report.HighCount)
	}
}

func TestReportWriters(t *testing.T) {
	findings := []Finding{
		{File: "a.ts", Line: 1, Rule: RuleMockConstants, Severity: SeverityHigh, Message: "m1"},
		{File: "a.ts", Line: 5, Rule: RuleTodoComments, Severity: SeverityWarn, Message: "m2"},
	}
	var mdBuf, jsonBuf bytes.Buffer
	if err := WriteMarkdown(findings, &mdBuf); err != nil {
		t.Fatalf("markdown: %v", err)
	}
	if !strings.Contains(mdBuf.String(), "HIGH (1)") {
		t.Errorf("markdown missing HIGH header: %s", mdBuf.String())
	}
	if err := WriteJSON(findings, &jsonBuf); err != nil {
		t.Fatalf("json: %v", err)
	}
	if !strings.Contains(jsonBuf.String(), `"total": 2`) {
		t.Errorf("json missing total: %s", jsonBuf.String())
	}
}
