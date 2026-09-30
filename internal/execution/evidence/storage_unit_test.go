package evidence

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestStorageUnitIdentityAndPermissions(t *testing.T) {
	dir := t.TempDir()
	for _, parent := range []string{".openexec", ".openexec/data"} {
		path := filepath.Join(dir, parent)
		if err := os.Mkdir(path, 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(path, 0755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("OPENEXEC_STORAGE_SECRET", "environment-only-sentinel")
	command := Command{Argv: []string{"sh", "-c", "token=private-value"}, Cwd: dir, ExitCode: 2, Stderr: "private-value", Toolchain: map[string]string{"go_version": "v1", "TOKEN": "excluded-sentinel"}}
	hash, path, err := Write(dir, command)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(raw)
	if hash != hex.EncodeToString(sum[:]) || path != filepath.Join(dir, directory, hash+".json") {
		t.Fatal("incorrect content identity or path")
	}
	if strings.Contains(string(raw), "environment-only-sentinel") || strings.Contains(string(raw), "excluded-sentinel") {
		t.Fatal("unapproved metadata retained")
	}
	delete(command.Toolchain, "TOKEN")
	got, err := Read(dir, hash)
	if err != nil || !reflect.DeepEqual(got, &command) {
		t.Fatal("private roundtrip changed command")
	}
	if strings.Contains(Public(got.Stderr, CommandSecrets(got.Argv[2])), "private-value") {
		t.Fatal("private credential published")
	}
	again, same, err := Write(dir, command)
	if err != nil || again != hash || same != path {
		t.Fatal("identity is not deterministic")
	}
	command.ExitCode++
	other, _, err := Write(dir, command)
	if err != nil || other == hash {
		t.Fatal("changed content reused identity")
	}
	for p, mode := range map[string]os.FileMode{".openexec": 0755, ".openexec/data": 0755, directory: 0700, filepath.Join(directory, hash+".json"): 0600} {
		info, err := os.Stat(filepath.Join(dir, p))
		if err != nil || info.Mode().Perm() != mode {
			t.Fatal("permissions changed", p, err)
		}
	}
	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil || len(entries) != 2 {
		t.Fatal("temporary capture leaked")
	}
}

func TestStorageUnitWriteFailureCleanup(t *testing.T) {
	dir := t.TempDir()
	hash, path, err := Write(dir, Command{})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(path, 0700); err != nil {
		t.Fatal(err)
	}
	if _, err := Read(dir, hash); err == nil {
		t.Fatal("directory accepted as artifact")
	}
	if hash, path, err := Write(dir, Command{}); err == nil || hash != "" || path != "" {
		t.Fatal("failed rename published reference")
	}
	entries, err := os.ReadDir(filepath.Join(dir, directory))
	if err != nil || len(entries) != 1 {
		t.Fatal("failed write leaked temporary file")
	}
}

func TestStorageUnitMalformedIdentities(t *testing.T) {
	for _, hash := range []string{"", ".", "..", "../" + strings.Repeat("a", 61), strings.Repeat("a", 63), strings.Repeat("a", 65), strings.Repeat("g", 64), strings.Repeat("A", 64), "/" + strings.Repeat("a", 64), strings.Repeat("a", 64) + ".json"} {
		if _, err := Read(t.TempDir(), hash); err == nil || !strings.Contains(err.Error(), "invalid evidence identity") {
			t.Fatal("malformed identity not refused before filesystem access")
		}
	}
}
