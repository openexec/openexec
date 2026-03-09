package persona

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadPersonaWithoutExtends(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "solo.md", `---
---

<role>The solo role</role>
<identity>Solo identity</identity>
<communication_style>Direct</communication_style>
<principles>Be excellent</principles>
`)

	store := NewStore(os.DirFS(dir))
	p, err := store.Get("solo")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}

	if p.Role != "The solo role" {
		t.Errorf("Role = %q, want %q", p.Role, "The solo role")
	}
	if p.Identity != "Solo identity" {
		t.Errorf("Identity = %q, want %q", p.Identity, "Solo identity")
	}
	if p.CommunicationStyle != "Direct" {
		t.Errorf("CommunicationStyle = %q, want %q", p.CommunicationStyle, "Direct")
	}
	if p.Principles != "Be excellent" {
		t.Errorf("Principles = %q, want %q", p.Principles, "Be excellent")
	}
}

func TestLoadPersonaWithExtends(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "_base.md", `---
---

<principles>Team first
Document everything</principles>
`)

	writeFile(t, dir, "spark.md", `---
extends: _base
---

<role>Developer</role>
<identity>Focused craftsman</identity>
<communication_style>Calm and methodical</communication_style>
<principles>Test behavior not implementation
Welcome ruthless review</principles>
`)

	store := NewStore(os.DirFS(dir))
	p, err := store.Get("spark")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}

	if p.Role != "Developer" {
		t.Errorf("Role = %q, want %q", p.Role, "Developer")
	}

	// Principles should have base prepended.
	if !strings.HasPrefix(p.Principles, "Team first") {
		t.Errorf("Principles should start with base principles, got: %q", p.Principles)
	}
	if !strings.Contains(p.Principles, "Test behavior not implementation") {
		t.Errorf("Principles should contain agent principles, got: %q", p.Principles)
	}
}

func TestUnknownPersona(t *testing.T) {
	store := NewStore(os.DirFS(t.TempDir()))
	_, err := store.Get("nonexistent")
	if err == nil {
		t.Error("expected error for unknown persona, got nil")
	}
}

func TestCacheBehavior(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "cached.md", `---
---

<role>Cached role</role>
<principles>Cached principles</principles>
`)

	store := NewStore(os.DirFS(dir))
	first, err := store.Get("cached")
	if err != nil {
		t.Fatalf("first Get: %v", err)
	}
	second, err := store.Get("cached")
	if err != nil {
		t.Fatalf("second Get: %v", err)
	}
	if first != second {
		t.Error("second Get returned different pointer; expected cached value")
	}
}

func TestBasePersonaWithoutExtends(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "_base.md", `---
---

<principles>Shared principles</principles>
`)

	store := NewStore(os.DirFS(dir))
	p, err := store.Get("_base")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}

	if p.Principles != "Shared principles" {
		t.Errorf("Principles = %q, want %q", p.Principles, "Shared principles")
	}
}

func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}
