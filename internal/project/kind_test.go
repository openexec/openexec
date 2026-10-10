package project

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseKind(t *testing.T) {
	for in, want := range map[string]string{"": KindRepository, "repository": KindRepository, "chat": KindChat} {
		got, err := ParseKind(in)
		if err != nil || got != want {
			t.Errorf("ParseKind(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	if _, err := ParseKind("wiki"); err == nil {
		t.Error("ParseKind(\"wiki\") succeeded, want error")
	}
}

func TestInitializeKindRepositoryCreatesGitRepo(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	dir := filepath.Join(t.TempDir(), "repo-app")

	cfg, err := InitializeKind("repo-app", dir, KindRepository)
	if err != nil {
		t.Fatalf("InitializeKind: %v", err)
	}
	if cfg.ProjectKind() != KindRepository || !cfg.GitEnabled {
		t.Errorf("kind=%q git=%v, want repository with git enabled", cfg.ProjectKind(), cfg.GitEnabled)
	}
	out, err := exec.Command("git", "-C", dir, "symbolic-ref", "--short", "HEAD").Output()
	if err != nil {
		t.Fatalf("directory is not a git repository: %v", err)
	}
	if branch := strings.TrimSpace(string(out)); branch != "main" {
		t.Errorf("initial branch = %q, want main", branch)
	}

	loaded, err := LoadProjectConfig(dir)
	if err != nil {
		t.Fatalf("LoadProjectConfig: %v", err)
	}
	if loaded.Kind != KindRepository {
		t.Errorf("persisted kind = %q, want repository", loaded.Kind)
	}
}

func TestInitializeKindChatIsGitFree(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "notes")

	if _, err := InitializeKind("notes", dir, KindChat); err != nil {
		t.Fatalf("InitializeKind: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".git")); !os.IsNotExist(err) {
		t.Errorf("chat project has a .git directory (stat err=%v)", err)
	}

	loaded, err := LoadProjectConfig(dir)
	if err != nil {
		t.Fatalf("LoadProjectConfig: %v", err)
	}
	if loaded.ProjectKind() != KindChat || loaded.GitEnabled || loaded.IsGitCommitEnabled() {
		t.Errorf("kind=%q git=%v commit=%v, want chat with git off", loaded.ProjectKind(), loaded.GitEnabled, loaded.IsGitCommitEnabled())
	}

	yaml, err := os.ReadFile(filepath.Join(dir, "openexec.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(yaml), "type: chat") || strings.Contains(string(yaml), "lint") {
		t.Errorf("chat openexec.yaml should declare type chat without code gates:\n%s", yaml)
	}
}

func TestInitializeKindRejectsUnknownKind(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "x")
	if _, err := InitializeKind("x", dir, "wiki"); err == nil {
		t.Fatal("InitializeKind accepted unknown kind")
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Error("unknown kind must not create the project directory")
	}
}

// Compatibility: configs written before kinds existed, and legacy .uaos
// projects, carry no kind and must load as repository projects.
func TestProjectKindDefaultsToRepositoryForExistingProjects(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".openexec"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".openexec", "config.json"), []byte(`{"name":"old","git_enabled":true}`), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadProjectConfig(dir)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ProjectKind() != KindRepository {
		t.Errorf("existing .openexec project kind = %q, want repository", cfg.ProjectKind())
	}

	legacy := t.TempDir()
	if err := os.MkdirAll(filepath.Join(legacy, ".uaos"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(legacy, ".uaos", "project.json"), []byte(`{"name":"legacy"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err = LoadProjectConfig(legacy)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ProjectKind() != KindRepository {
		t.Errorf("legacy .uaos project kind = %q, want repository", cfg.ProjectKind())
	}
}
