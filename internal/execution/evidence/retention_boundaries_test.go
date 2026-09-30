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

func TestDiagnosticTailCapture(t *testing.T) {
	input := "first\n" + strings.Repeat("0123456789\n", 2000) + "token=PRIVATE_SECRET\nDIAGNOSTIC_TAIL\n"
	want := input[:StreamLimit/2] + input[len(input)-StreamLimit/2:]
	for _, chunk := range []int{1, 17, 2048, 4096, len(input)} {
		var b Buffer
		for i := 0; i < len(input); i += chunk {
			part := input[i:min(i+chunk, len(input))]
			if n, err := b.Write([]byte(part)); err != nil || n != len(part) {
				t.Fatal("stream not drained")
			}
		}
		if b.String() != want || b.Len() != StreamLimit || !b.Truncated {
			t.Fatalf("chunk %d lost prefix/tail", chunk)
		}
		public := PublicStream(&b, []string{"PRIVATE_SECRET"})
		if len(public) > StreamLimit || strings.Contains(public, "PRIVATE_SECRET") || !strings.HasPrefix(public, "first\n") || !strings.HasSuffix(public, "DIAGNOSTIC_TAIL\n") {
			t.Fatalf("unsafe or missing public tail: %q", public)
		}
	}
	dir := t.TempDir()
	hash, _, err := Write(dir, Command{Stdout: input, Stderr: input})
	if err != nil {
		t.Fatal(err)
	}
	got, err := Read(dir, hash)
	if err != nil || got.Stdout != want || got.Stderr != want || !got.StdoutTruncated || !got.StderrTruncated {
		t.Fatal("direct retention lost tail", err)
	}
	// A tail starting inside a secret must never publish the unmatched suffix.
	var b Buffer
	b.Write([]byte(strings.Repeat("x", StreamLimit) + "token=" + strings.Repeat("S", StreamLimit) + "\nTAIL\n"))
	if public := PublicStream(&b, nil); strings.Contains(public, "S") || !strings.HasSuffix(public, "TAIL\n") {
		t.Fatal("split secret exposed", public)
	}
}
