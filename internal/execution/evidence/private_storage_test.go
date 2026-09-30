package evidence

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPrivateStorageLegacyRefusal(t *testing.T) {
	dir := t.TempDir()
	hash, path, err := Write(dir, Command{Argv: []string{"sh", "-c", "exit 2"}, ExitCode: 2})
	if err != nil {
		t.Fatal(err)
	}
	legacy := filepath.Join(dir, ".openexec-verification")
	if err = os.Mkdir(legacy, 0700); err != nil {
		t.Fatal(err)
	}
	old := filepath.Join(legacy, hash+".json")
	if err = os.Rename(path, old); err != nil {
		t.Fatal(err)
	}
	if _, err = Read(dir, hash); err == nil {
		t.Fatal("legacy fallback accepted")
	}
	if _, err = os.Stat(old); err != nil {
		t.Fatal("legacy evidence altered")
	}
}

func TestPrivateStorageNestedPathRefusals(t *testing.T) {
	for _, component := range []string{".openexec", ".openexec/data", directory} {
		for _, kind := range []string{"symlink", "file"} {
			t.Run(component+"/"+kind, func(t *testing.T) {
				dir := t.TempDir()
				path := filepath.Join(dir, component)
				if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
					t.Fatal(err)
				}
				var err error
				if kind == "symlink" {
					err = os.Symlink(t.TempDir(), path)
				} else {
					err = os.WriteFile(path, nil, 0600)
				}
				if err != nil {
					t.Fatal(err)
				}
				if _, _, err = Write(dir, Command{}); err == nil {
					t.Fatal("unsafe parent written")
				}
				if _, err = Read(dir, "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"); err == nil {
					t.Fatal("unsafe parent read")
				}
			})
		}
	}
	dir := t.TempDir()
	hash, path, err := Write(dir, Command{})
	if err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(filepath.Dir(path), "other")
	if err = os.Rename(path, target); err != nil {
		t.Fatal(err)
	}
	if err = os.Symlink(target, path); err != nil {
		t.Fatal(err)
	}
	if _, err = Read(dir, hash); err == nil {
		t.Fatal("symlink file read")
	}
}
