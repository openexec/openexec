package knowledge

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestRepositoryCheckoutWorktreeAndForkIdentities(t *testing.T) {
	ctx := context.Background()
	base := t.TempDir()
	storeRoot := filepath.Join(base, "state")
	if err := os.MkdirAll(storeRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	store, err := NewStore(storeRoot)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	cloneA := filepath.Join(base, "clone-a")
	cloneB := filepath.Join(base, "clone-b")
	initGitFixture(t, cloneA, "https://token@example.test/acme/app.git")
	initGitFixture(t, cloneB, "git@example.test:acme/app.git")
	a, err := store.EnsureRepositoryIdentity(ctx, cloneA, "")
	if err != nil {
		t.Fatal(err)
	}
	b, err := store.EnsureRepositoryIdentity(ctx, cloneB, "")
	if err != nil {
		t.Fatal(err)
	}
	if a.RepositoryID != b.RepositoryID || a.CheckoutID == b.CheckoutID || a.WorktreeID == b.WorktreeID {
		t.Fatalf("clone hierarchy is wrong: a=%#v b=%#v", a, b)
	}

	linked := filepath.Join(base, "linked-worktree")
	runGitFixture(t, cloneA, "worktree", "add", "-b", "linked", linked)
	w, err := store.EnsureRepositoryIdentity(ctx, linked, "")
	if err != nil {
		t.Fatal(err)
	}
	if w.RepositoryID != a.RepositoryID || w.CheckoutID != a.CheckoutID || w.WorktreeID == a.WorktreeID {
		t.Fatalf("linked worktree hierarchy is wrong: checkout=%#v worktree=%#v", a, w)
	}

	standalone := filepath.Join(base, "standalone")
	initGitFixture(t, standalone, "")
	local, err := store.EnsureRepositoryIdentity(ctx, standalone, "")
	if err != nil {
		t.Fatal(err)
	}
	if local.RepositoryID == a.RepositoryID {
		t.Fatal("unrelated no-remote root inherited the first repository identity")
	}

	forkRoot := filepath.Join(base, "fork")
	initGitFixture(t, forkRoot, "https://example.test/perttu/app-fork.git")
	fork, err := store.EnsureRepositoryIdentityWithOptions(ctx, forkRoot, RepositoryIdentityOptions{ForkedFromRepository: a.RepositoryID})
	if err != nil {
		t.Fatal(err)
	}
	if fork.RepositoryID == a.RepositoryID {
		t.Fatal("fork reused parent repository identity")
	}
	var parent string
	if err := store.db.QueryRowContext(ctx, `SELECT forked_from_repository_id FROM repositories WHERE id = ?`, fork.RepositoryID).Scan(&parent); err != nil || parent != a.RepositoryID {
		t.Fatalf("fork lineage = %q, %v", parent, err)
	}

	hintedRoot := filepath.Join(base, "hinted")
	initGitFixture(t, hintedRoot, "")
	hinted, err := store.EnsureRepositoryIdentity(ctx, hintedRoot, a.RepositoryID)
	if err != nil || hinted.RepositoryID != a.RepositoryID || hinted.CheckoutID == a.CheckoutID {
		t.Fatalf("explicit clone hint was not honored: %#v, %v", hinted, err)
	}
}

func initGitFixture(t *testing.T, root, remote string) {
	t.Helper()
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	runGitFixture(t, root, "init")
	if remote != "" {
		runGitFixture(t, root, "remote", "add", "origin", remote)
	}
	if err := os.WriteFile(filepath.Join(root, "README.md"), []byte("fixture\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	runGitFixture(t, root, "add", "README.md")
	runGitFixture(t, root, "-c", "user.name=OpenExec Test", "-c", "user.email=test@example.test", "commit", "-m", "fixture")
}

func runGitFixture(t *testing.T, root string, arguments ...string) {
	t.Helper()
	command := exec.Command("git", append([]string{"-C", root}, arguments...)...)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", arguments, err, output)
	}
}
