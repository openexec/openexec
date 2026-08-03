package knowledge

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestFindCompatibleNodeSkipsObsoleteRuntime(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fixture uses POSIX executables")
	}
	root := t.TempDir()
	oldDirectory := filepath.Join(root, "old")
	newDirectory := filepath.Join(root, "new")
	for _, directory := range []string{oldDirectory, newDirectory} {
		if err := os.MkdirAll(directory, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(oldDirectory, "node"), []byte("#!/bin/sh\nprintf 6\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	newNode := filepath.Join(newDirectory, "node")
	if err := os.WriteFile(newNode, []byte("#!/bin/sh\nprintf 20\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", oldDirectory+string(os.PathListSeparator)+newDirectory)
	t.Setenv("NVM_BIN", "")
	t.Setenv("NVM_DIR", "")
	t.Setenv("VOLTA_HOME", "")

	found, err := findCompatibleNode(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if found != newNode {
		t.Fatalf("compatible Node = %q, want %q", found, newNode)
	}
}
