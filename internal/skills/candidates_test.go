package skills

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCandidateLifecycle(t *testing.T) {
	projDir := t.TempDir()

	// Propose
	path, err := ProposeCandidate(projDir, "vitest-jsdom-quirks",
		"Workarounds for JSDOM layout gaps", "When debugging Vitest mouse-event tests",
		[]string{"vitest", "jsdom"}, "## Rule\nUse userEvent, not fireEvent.")
	if err != nil {
		t.Fatalf("ProposeCandidate: %v", err)
	}
	if !strings.Contains(path, "_candidates") {
		t.Fatalf("candidate written outside _candidates: %s", path)
	}

	// Candidate is parseable and listed
	candidates, err := ListCandidates(projDir)
	if err != nil || len(candidates) != 1 {
		t.Fatalf("expected 1 candidate, got %d (err=%v)", len(candidates), err)
	}
	if candidates[0].Name != "vitest-jsdom-quirks" || candidates[0].Description == "" {
		t.Fatalf("candidate parsed wrong: %+v", candidates[0])
	}

	// Registry must NOT see it
	reg := NewRegistry()
	if err := reg.LoadFromDir(filepath.Join(projDir, ".openexec", "skills"), "project"); err != nil {
		t.Fatalf("LoadFromDir: %v", err)
	}
	if _, found := reg.Get("vitest-jsdom-quirks"); found {
		t.Fatal("registry loaded an unapproved candidate — propose-then-approve is broken")
	}

	// Duplicate proposal refused
	if _, err := ProposeCandidate(projDir, "vitest-jsdom-quirks", "dup", "", nil, "body"); err == nil {
		t.Fatal("expected duplicate candidate to be refused")
	}

	// Approve → moves into active skills, registry now sees it
	activePath, err := ApproveCandidate(projDir, "vitest-jsdom-quirks")
	if err != nil {
		t.Fatalf("ApproveCandidate: %v", err)
	}
	if strings.Contains(activePath, "_candidates") {
		t.Fatalf("approved skill still in _candidates: %s", activePath)
	}
	reg2 := NewRegistry()
	_ = reg2.LoadFromDir(filepath.Join(projDir, ".openexec", "skills"), "project")
	if _, found := reg2.Get("vitest-jsdom-quirks"); !found {
		t.Fatal("approved skill not loaded by registry")
	}

	// Candidate list now empty; re-approve fails
	candidates, _ = ListCandidates(projDir)
	if len(candidates) != 0 {
		t.Fatalf("expected 0 candidates after approval, got %d", len(candidates))
	}
	if _, err := ApproveCandidate(projDir, "vitest-jsdom-quirks"); err == nil {
		t.Fatal("expected approving a non-existent candidate to fail")
	}

	// Proposing a name that shadows an active skill is refused
	if _, err := ProposeCandidate(projDir, "vitest-jsdom-quirks", "shadow", "", nil, "body"); err == nil {
		t.Fatal("expected proposal shadowing an active skill to be refused")
	}
}

func TestRejectCandidate(t *testing.T) {
	projDir := t.TempDir()
	if _, err := ProposeCandidate(projDir, "bad-idea", "A wrong lesson", "", nil, "Never write tests."); err != nil {
		t.Fatalf("ProposeCandidate: %v", err)
	}
	if err := RejectCandidate(projDir, "bad-idea"); err != nil {
		t.Fatalf("RejectCandidate: %v", err)
	}
	candidates, _ := ListCandidates(projDir)
	if len(candidates) != 0 {
		t.Fatal("rejected candidate still listed")
	}
	if err := RejectCandidate(projDir, "bad-idea"); err == nil {
		t.Fatal("expected rejecting a non-existent candidate to fail")
	}
}

func TestCandidateNameValidation(t *testing.T) {
	projDir := t.TempDir()
	// Names are directory components: path traversal and hidden/reserved
	// prefixes must be rejected.
	bad := []string{
		"../escape", "a/b", "a\\b", "..", ".hidden", "_candidates",
		"UPPER", "has space", "x", "", "-leading-hyphen",
	}
	for _, name := range bad {
		if _, err := ProposeCandidate(projDir, name, "d", "", nil, "c"); err == nil {
			t.Errorf("expected name %q to be rejected", name)
		}
	}
	// Nothing escaped the candidates dir
	if _, err := os.Stat(filepath.Join(projDir, "escape")); err == nil {
		t.Fatal("path traversal escaped the candidates directory")
	}
}

func TestPromoteSkill(t *testing.T) {
	projDir := t.TempDir()
	userSkillsDir := filepath.Join(t.TempDir(), "user")

	// No active project skill → refused (candidates don't count).
	if _, err := ProposeCandidate(projDir, "shared-lesson", "A lesson", "", nil, "## Rule"); err != nil {
		t.Fatalf("ProposeCandidate: %v", err)
	}
	if _, err := PromoteSkill(projDir, "shared-lesson", userSkillsDir); err == nil {
		t.Fatal("promoting an unapproved candidate must be refused")
	}

	// Approve, then promote.
	if _, err := ApproveCandidate(projDir, "shared-lesson"); err != nil {
		t.Fatalf("ApproveCandidate: %v", err)
	}
	dst, err := PromoteSkill(projDir, "shared-lesson", userSkillsDir)
	if err != nil {
		t.Fatalf("PromoteSkill: %v", err)
	}

	// Both copies exist: promotion shares, it does not move.
	if _, err := os.Stat(filepath.Join(dst, "SKILL.md")); err != nil {
		t.Fatalf("user copy missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(projDir, ".openexec", "skills", "shared-lesson", "SKILL.md")); err != nil {
		t.Fatalf("project copy must remain: %v", err)
	}

	// User-level registry loads it.
	reg := NewRegistry()
	if err := reg.LoadFromDir(userSkillsDir, "user"); err != nil {
		t.Fatalf("LoadFromDir: %v", err)
	}
	if _, found := reg.Get("shared-lesson"); !found {
		t.Fatal("promoted skill not loadable from user dir")
	}

	// Re-promotion refused.
	if _, err := PromoteSkill(projDir, "shared-lesson", userSkillsDir); err == nil {
		t.Fatal("promoting over an existing user skill must be refused")
	}
}
