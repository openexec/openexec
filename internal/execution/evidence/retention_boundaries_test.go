package evidence

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRetentionBoundariesCapture(t *testing.T) {
	for _, size := range []int{0, StreamLimit - 1, StreamLimit, StreamLimit + 1, StreamLimit * 20} {
		var b Buffer
		input := strings.Repeat("x", size)
		n, err := b.Write([]byte(input))
		if err != nil || n != size || b.Len() != min(size, StreamLimit) || b.Truncated != (size > StreamLimit) {
			t.Fatalf("capture size %d: %d %v", size, n, err)
		}
		n, err = b.Write([]byte("tail"))
		if err != nil || n != 4 || b.Len() > StreamLimit {
			t.Fatal("stream not drained")
		}
	}
	var b Buffer
	secret := "SECRET_BOUNDARY_SENTINEL"
	b.Write([]byte("marker\n" + strings.Repeat("x", StreamLimit-12) + secret))
	if public := PublicStream(&b, []string{secret}); public != "marker\n[truncated]" {
		t.Fatalf("partial secret leaked: %q", public)
	}
	command := `TOKEN='COMMAND_SECRET' tool --password="ANOTHER_SECRET"`
	public := Public("COMMAND_SECRET ANOTHER_SECRET token=DIAGNOSTIC_SECRET", CommandSecrets(command))
	if strings.Contains(public, "SECRET") {
		t.Fatal("command or diagnostic secret exposed", public)
	}
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".openexec", "data"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(t.TempDir(), filepath.Join(dir, directory)); err != nil {
		t.Fatal(err)
	}
	if _, _, err := Write(dir, Command{}); err == nil {
		t.Fatal("symlink evidence directory accepted")
	}
}
