//go:build e2e

package cli

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestStoryImportBinaryStaleBaseGate drives the built bin/openexec (not the
// in-process cobra command) through `story import --dry-run` in a temp
// .openexec project. Build first: `make build` or
// `go build -o bin/openexec ./cmd/openexec`, then
// `go test -tags e2e ./internal/cli/ -run TestStoryImportBinaryStaleBaseGate`.
func TestStoryImportBinaryStaleBaseGate(t *testing.T) {
	bin, err := filepath.Abs(filepath.Join("..", "..", "bin", "openexec"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(bin); err != nil {
		t.Fatalf("bin/openexec not built: %v", err)
	}

	dir := t.TempDir()
	if out, err := exec.Command("git", "-C", dir, "init", "-q").CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}
	if err := os.MkdirAll(filepath.Join(dir, ".openexec"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".openexec", "config.json"), []byte(`{"name":"stale-base-e2e"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	stories := func(script string) []map[string]any {
		return []map[string]any{{
			"id": "US-001", "title": "s", "description": "d",
			"tasks": []map[string]any{{"id": "T-US-001-001", "title": "t", "description": "d", "verification_script": script}},
		}}
	}
	write := func(name string, v any) string {
		data, err := json.Marshal(v)
		if err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, data, 0o644); err != nil {
			t.Fatal(err)
		}
		return path
	}
	run := func(file string) (string, error) {
		cmd := exec.Command(bin, "story", "import", "--dry-run", file)
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		return string(out), err
	}

	bare := "git diff --name-only main...HEAD -- x | grep -vc y"
	remote := "git diff --name-only origin/main...HEAD -- x | grep -vc y"

	for name, file := range map[string]string{
		"goal-less object": write("bad.json", map[string]any{"schema_version": "1.1", "stories": stories(bare)}),
		"legacy array":     write("legacy.json", stories(bare)),
	} {
		out, err := run(file)
		t.Logf("%s:\n%s", name, out)
		if err == nil {
			t.Errorf("%s: bare main...HEAD import succeeded; want refusal", name)
		}
		if !strings.Contains(out, "PLANNING GATE FAILED: story US-001 task T-US-001-001") || !strings.Contains(out, "origin/main") {
			t.Errorf("%s: refusal does not name the task and the origin/ fix:\n%s", name, out)
		}
	}

	out, err := run(write("good.json", map[string]any{"schema_version": "1.1", "stories": stories(remote)}))
	t.Logf("origin/main:\n%s", out)
	if err != nil || strings.Contains(out, "PLANNING GATE FAILED") {
		t.Errorf("origin/main...HEAD import refused (err=%v):\n%s", err, out)
	}
	if !strings.Contains(out, "Would import") {
		t.Errorf("origin/main...HEAD dry run did not preview the import:\n%s", out)
	}
}
