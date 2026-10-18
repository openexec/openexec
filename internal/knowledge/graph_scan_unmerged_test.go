package knowledge

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestScanRepositoryResolvedUnmergedIndex(t *testing.T) {
	root := t.TempDir()
	initGitFixture(t, root, "")
	commit := func(message string) {
		runGitFixture(t, root, "add", "sample.go")
		runGitFixture(t, root, "-c", "user.name=OpenExec Test", "-c", "user.email=test@example.test", "commit", "-m", message)
	}
	writeTestFile(t, root, "sample.go", "package sample\nfunc Value() int { return 0 }\n")
	commit("base")
	runGitFixture(t, root, "checkout", "-b", "left")
	writeTestFile(t, root, "sample.go", "package sample\nfunc Value() int { return 1 }\n")
	commit("left")
	runGitFixture(t, root, "checkout", "-b", "right", "HEAD~1")
	writeTestFile(t, root, "sample.go", "package sample\nfunc Value() int { return 2 }\n")
	commit("right")
	output, err := exec.Command("git", "-C", root, "-c", "user.name=OpenExec Test", "-c", "user.email=test@example.test", "merge", "left").CombinedOutput()
	if err == nil || !strings.Contains(string(output), "CONFLICT") {
		t.Fatalf("expected merge conflict: %v\n%s", err, output)
	}
	// Resolve only the worktree content: the lane handoff retains all index stages.
	resolved := "package sample\nfunc Value() int { return 3 }\n"
	writeTestFile(t, root, "sample.go", resolved)
	indexBefore, err := exec.Command("git", "-C", root, "ls-files", "--unmerged").Output()
	if err != nil || strings.Count(string(indexBefore), "\tsample.go\n") != 3 {
		t.Fatalf("expected three unmerged stages: %v\n%s", err, indexBefore)
	}
	store, err := NewStore(root)
	if err != nil {
		t.Fatal(err)
	}
	result, err := store.ScanRepository(context.Background(), root)
	if err != nil {
		store.Close()
		t.Fatal(err)
	}
	if result.Generation.Status != GraphCurrent || result.Files != 1 {
		t.Fatalf("unexpected scan: %#v", result)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	// Reopen the database to prove the input was persisted, not merely returned.
	store, err = NewStore(root)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	var count int
	var contentHash string
	if err := store.db.QueryRow(`SELECT COUNT(*), MIN(content_hash) FROM graph_scan_inputs WHERE generation_id = ? AND file_path = 'sample.go'`, result.Generation.ID).Scan(&count, &contentHash); err != nil {
		t.Fatal(err)
	}
	if count != 1 || contentHash != hashBytes([]byte(resolved)) {
		t.Fatalf("persisted input count=%d hash=%s", count, contentHash)
	}
	second, err := store.ScanRepository(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	if second.Generation.ManifestHash != result.Generation.ManifestHash {
		t.Fatal("repeat scan changed manifest")
	}
	indexAfter, err := exec.Command("git", "-C", root, "ls-files", "--unmerged").Output()
	if err != nil || string(indexAfter) != string(indexBefore) {
		t.Fatalf("scan changed unmerged index: %v\n%s", err, indexAfter)
	}
}

func TestGitManifestPreservesFilteringAndOrder(t *testing.T) {
	root := t.TempDir()
	initGitFixture(t, root, "")
	writeTestFile(t, root, "z-tracked.go", "package sample\n")
	writeTestFile(t, root, "deleted.go", "package sample\n")
	writeTestFile(t, root, "tracked-ignored.go", "package sample\n")
	runGitFixture(t, root, "add", "z-tracked.go", "deleted.go", "tracked-ignored.go")
	if err := os.Remove(filepath.Join(root, "deleted.go")); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, root, ".gitignore", "*ignored.go\n")
	writeTestFile(t, root, "untracked-ignored.go", "package ignored\n")
	writeTestFile(t, root, "a-untracked.go", "package sample\n")
	writeTestFile(t, root, "node_modules/hidden.go", "package hidden\n")
	if err := os.Symlink("z-tracked.go", filepath.Join(root, "link.go")); err != nil {
		t.Fatal(err)
	}
	runGitFixture(t, root, "add", "link.go")
	first, err := BuildScanManifest(root)
	if err != nil {
		t.Fatal(err)
	}
	var paths []string
	for _, input := range first.Inputs {
		paths = append(paths, input.FilePath)
		if input.FilePath == "link.go" && input.SymlinkTarget != "z-tracked.go" {
			t.Fatalf("lost symlink target: %#v", input)
		}
	}
	if want := []string{"a-untracked.go", "link.go", "tracked-ignored.go", "z-tracked.go"}; !reflect.DeepEqual(paths, want) {
		t.Fatalf("paths=%v want=%v", paths, want)
	}
	second, err := BuildScanManifest(root)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatal("manifest is not deterministic")
	}
	outside := filepath.Join(t.TempDir(), "outside.go")
	writeTestFile(t, filepath.Dir(outside), "outside.go", "package outside\n")
	if err := os.Symlink(outside, filepath.Join(root, "escape.go")); err != nil {
		t.Fatal(err)
	}
	runGitFixture(t, root, "add", "escape.go")
	if _, err := BuildScanManifest(root); err == nil || !strings.Contains(err.Error(), "escapes repository") {
		t.Fatalf("expected symlink escape rejection: %v", err)
	}
}
