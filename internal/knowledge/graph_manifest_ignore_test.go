package knowledge

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestManifestHonorsGitignore(t *testing.T) {
	root := t.TempDir()
	if out, err := exec.Command("git", "-C", root, "init", "-q").CombinedOutput(); err != nil {
		t.Skipf("git unavailable: %v %s", err, out)
	}
	must := func(path, content string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	must(filepath.Join(root, ".gitignore"), "GOPATH_DIR/\n")
	must(filepath.Join(root, "main.go"), "package main\nfunc main() {}\n")
	must(filepath.Join(root, "GOPATH_DIR", "cached", "dep.go"), "package cached\nfunc Dep() {}\n")
	manifest, err := BuildScanManifest(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, input := range manifest.Inputs {
		if filepath.ToSlash(input.FilePath) == "GOPATH_DIR/cached/dep.go" {
			t.Fatalf("gitignored dependency cache was scanned: %#v", manifest.Inputs)
		}
	}
	if len(manifest.Inputs) != 1 || manifest.Inputs[0].FilePath != "main.go" {
		t.Fatalf("manifest inputs = %#v", manifest.Inputs)
	}
}
