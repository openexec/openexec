package repository

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/openexec/openexec/internal/knowledge"
)

func TestRootedReaderEnforcesIdentityHashesAndBounds(t *testing.T) {
	root := t.TempDir()
	content := []byte("before\nfunc Run() {}\nafter\n")
	path := filepath.Join(root, "run.go")
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatal(err)
	}
	start, end := 7, 20
	reader, err := NewRootedReader(root, "repo", "worktree", 1024)
	if err != nil {
		t.Fatal(err)
	}
	request := knowledge.SourceReadRequest{RepositoryID: "repo", WorktreeID: "worktree", FilePath: "run.go", StartByte: start, EndByte: end, FileHash: testDigest(content), RangeHash: testDigest(content[start:end])}
	result, err := reader.ReadRange(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if result.Content != "func Run() {}" {
		t.Fatalf("unexpected source range %q", result.Content)
	}

	wrongIdentity := request
	wrongIdentity.RepositoryID = "another"
	if _, err := reader.ReadRange(context.Background(), wrongIdentity); !errors.Is(err, ErrOutsideRoot) {
		t.Fatalf("identity mismatch was allowed: %v", err)
	}
	traversal := request
	traversal.FilePath = "../run.go"
	if _, err := reader.ReadRange(context.Background(), traversal); !errors.Is(err, ErrOutsideRoot) {
		t.Fatalf("traversal was allowed: %v", err)
	}
	if err := os.WriteFile(path, []byte("changed\nfunc Run() {}\nafter\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := reader.ReadRange(context.Background(), request); !errors.Is(err, ErrStalePointer) {
		t.Fatalf("stale pointer was accepted: %v", err)
	}
}

func TestRootedReaderRejectsSymlinkEscapeAndOversizedRange(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "outside.go")
	data := []byte("package outside\n")
	if err := os.WriteFile(outside, data, 0o600); err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" {
		if err := os.Symlink(outside, filepath.Join(root, "escape.go")); err != nil {
			t.Fatal(err)
		}
		reader, _ := NewRootedReader(root, "repo", "worktree", 1024)
		request := knowledge.SourceReadRequest{RepositoryID: "repo", WorktreeID: "worktree", FilePath: "escape.go", StartByte: 0, EndByte: len(data), FileHash: testDigest(data), RangeHash: testDigest(data)}
		if _, err := reader.ReadRange(context.Background(), request); !errors.Is(err, ErrOutsideRoot) {
			t.Fatalf("symlink escape was allowed: %v", err)
		}
	}
	local := []byte("0123456789")
	if err := os.WriteFile(filepath.Join(root, "large.ts"), local, 0o600); err != nil {
		t.Fatal(err)
	}
	reader, _ := NewRootedReader(root, "repo", "worktree", 4)
	request := knowledge.SourceReadRequest{RepositoryID: "repo", WorktreeID: "worktree", FilePath: "large.ts", StartByte: 0, EndByte: len(local), FileHash: testDigest(local), RangeHash: testDigest(local)}
	if _, err := reader.ReadRange(context.Background(), request); !errors.Is(err, ErrRangeTooLarge) {
		t.Fatalf("oversized range was allowed: %v", err)
	}
}

func TestGraphResolutionReadsCurrentSourceThroughRepositoryAuthority(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".openexec"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "run.go"), []byte("package sample\n\nfunc Run() string { return \"ok\" }\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	store, err := knowledge.NewStore(root)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if _, err := store.ScanRepository(ctx, root); err != nil {
		t.Fatal(err)
	}
	identity, err := store.EnsureRepositoryIdentity(ctx, root, "")
	if err != nil {
		t.Fatal(err)
	}
	resolved, err := store.ResolveGraphSymbol(ctx, identity, "Run", "run.go", "function", 20)
	if err != nil || resolved.Result.Candidate == nil {
		t.Fatalf("resolve symbol: %#v %v", resolved, err)
	}
	reader, err := NewRootedReader(root, identity.RepositoryID, identity.WorktreeID, 1024)
	if err != nil {
		t.Fatal(err)
	}
	source, err := store.ReadGraphSymbol(ctx, identity, resolved.Result.Candidate.Symbol.ID, reader)
	if err != nil {
		t.Fatal(err)
	}
	if source.Result == nil || source.Result.Source.Content != "func Run() string { return \"ok\" }" {
		t.Fatalf("unexpected graph source: %#v", source)
	}
	if err := os.WriteFile(filepath.Join(root, "run.go"), []byte("package sample\n\nfunc Run() string { return \"changed\" }\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ReadGraphSymbol(ctx, identity, resolved.Result.Candidate.Symbol.ID, reader); !errors.Is(err, ErrStalePointer) {
		t.Fatalf("changed source was returned through stale pointer: %v", err)
	}
}

func testDigest(data []byte) string {
	sum := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(sum[:])
}
