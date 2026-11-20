package execution

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// The restore script is the only authenticity guard for the private D26
// capture, so it must fail closed. These cases drive the real script with
// scratch inputs; the authentic capture is private and cannot be a fixture.
func TestD26RestoreScriptFailsClosed(t *testing.T) {
	script, err := filepath.Abs("../../scripts/restore-d26-capture.sh")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := exec.LookPath("sha256sum"); err != nil {
		t.Skip("sha256sum not available")
	}
	const want = "0cbc9da8b5a6ed7d03db2b4a0a8ac16818f1c648f3db776090c83651bba20708"
	dir := t.TempDir()
	secret := "SECRET-PRIVATE-MARKER"
	wrong := filepath.Join(dir, "wrong.json")
	if err := os.WriteFile(wrong, []byte(`{"x":"`+secret+`"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	empty := filepath.Join(dir, "empty.json")
	if err := os.WriteFile(empty, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	cases := map[string]string{
		"wrong content": wrong,
		"empty file":    empty,
		"missing file":  filepath.Join(dir, "missing.json"),
		"directory":     dir,
	}
	for name, src := range cases {
		t.Run(name, func(t *testing.T) {
			dest := filepath.Join(t.TempDir(), "capture.json")
			out, err := exec.Command("sh", script, src, dest).CombinedOutput()
			if err == nil {
				t.Fatalf("script accepted %s; output: %s", name, out)
			}
			if _, statErr := os.Stat(dest); statErr == nil {
				t.Fatalf("destination written for %s", name)
			}
			if !strings.Contains(string(out), "refused") {
				t.Fatalf("expected refusal, got: %s", out)
			}
			if strings.Contains(string(out), secret) {
				t.Fatal("script output exposed file content")
			}
			if name == "wrong content" && !strings.Contains(string(out), want) {
				t.Fatalf("refusal should name the required hash: %s", out)
			}
		})
	}
}
