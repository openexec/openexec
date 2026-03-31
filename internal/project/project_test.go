package project

import (
	"os"
	"path/filepath"
	"testing"
)

func canonicalPath(t *testing.T, path string) string {
	t.Helper()

	resolved, err := filepath.EvalSymlinks(path)
	if err == nil {
		return resolved
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		t.Fatalf("failed to canonicalize path %q: %v", path, err)
	}
	return abs
}

func TestLoadProjectConfigCanonicalizesProjectDir(t *testing.T) {
	tmpDir := t.TempDir()

	if _, err := Initialize("canonical-project", tmpDir); err != nil {
		t.Fatalf("failed to initialize project: %v", err)
	}

	oldCwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get cwd: %v", err)
	}
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("failed to chdir: %v", err)
	}
	defer os.Chdir(oldCwd)

	cfg, err := LoadProjectConfig(".")
	if err != nil {
		t.Fatalf("failed to load project config: %v", err)
	}

	if !filepath.IsAbs(cfg.ProjectDir) {
		t.Fatalf("project dir should be absolute, got %q", cfg.ProjectDir)
	}
	if canonicalPath(t, cfg.ProjectDir) != canonicalPath(t, tmpDir) {
		t.Fatalf("project dir = %q, want canonical path for %q", cfg.ProjectDir, tmpDir)
	}
}

func TestLoadProjectConfigLegacyCanonicalizesProjectDir(t *testing.T) {
	tmpDir := t.TempDir()

	if err := os.MkdirAll(filepath.Join(tmpDir, ".uaos"), 0o755); err != nil {
		t.Fatalf("failed to create .uaos dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, ".uaos", "project.json"), []byte(`{"name":"legacy-project"}`), 0o600); err != nil {
		t.Fatalf("failed to write legacy config: %v", err)
	}

	oldCwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get cwd: %v", err)
	}
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("failed to chdir: %v", err)
	}
	defer os.Chdir(oldCwd)

	cfg, err := LoadProjectConfig(".")
	if err != nil {
		t.Fatalf("failed to load legacy project config: %v", err)
	}

	if canonicalPath(t, cfg.ProjectDir) != canonicalPath(t, tmpDir) {
		t.Fatalf("legacy project dir = %q, want canonical path for %q", cfg.ProjectDir, tmpDir)
	}
}
