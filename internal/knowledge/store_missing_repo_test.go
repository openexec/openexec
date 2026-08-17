package knowledge

import (
	"os"
	"path/filepath"
	"testing"
)

// A knowledge store is opened with a repository directory that the caller was
// given — a -d flag, a registered checkout, a workspace root. When that
// directory is not there, the caller is pointed at nothing, and the store used
// to answer by creating it: MkdirAll makes every missing parent, so the
// repository root appeared along with .openexec beneath it.
//
// Agent Console refreshes knowledge once per registered checkout, so this ran
// for every project seconds after each boot and resurrected the directory of
// any project whose folder had been deleted — empty, with a fresh database, and
// indistinguishable from a live checkout to anything that asks the filesystem.
func TestNewStoreRefusesAMissingRepositoryInsteadOfCreatingIt(t *testing.T) {
	root := t.TempDir()
	absent := filepath.Join(root, "deleted-checkout")

	store, err := NewStore(absent)
	if err == nil {
		if store != nil {
			_ = store.Close()
		}
		t.Fatal("opening a store on a directory that does not exist was accepted")
	}
	if _, statErr := os.Stat(absent); !os.IsNotExist(statErr) {
		t.Fatalf("the missing repository was created anyway: %v", statErr)
	}
}

// The nested case is the one that actually bit: MkdirAll would have built the
// whole path, so a checkout several levels below a missing parent came back too.
func TestNewStoreCreatesNoParentsForAMissingRepository(t *testing.T) {
	root := t.TempDir()
	nested := filepath.Join(root, "projects", "gone", "checkout")

	if _, err := NewStore(nested); err == nil {
		t.Fatal("opening a store below a missing parent was accepted")
	}
	if _, err := os.Stat(filepath.Join(root, "projects")); !os.IsNotExist(err) {
		t.Fatalf("intermediate directories were created: %v", err)
	}
}

// The store still owns .openexec inside a repository that does exist — refusing
// a missing repository must not turn into refusing to initialize a real one.
func TestNewStoreStillCreatesItsOwnDirectoryInsideARealRepository(t *testing.T) {
	root := t.TempDir()

	store, err := NewStore(root)
	if err != nil {
		t.Fatalf("opening a store in an existing repository failed: %v", err)
	}
	defer store.Close()

	if info, err := os.Stat(filepath.Join(root, ".openexec")); err != nil || !info.IsDir() {
		t.Fatalf(".openexec was not created inside the repository: %v", err)
	}
}
