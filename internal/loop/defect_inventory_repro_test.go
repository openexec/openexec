//go:build defectrepro

package loop

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// Reproducer for docs/DEFECT_INVENTORY.md D1, not a regression test: it passes
// while the defect exists. Run with `go test -tags defectrepro ./internal/loop/`.
// The fix replaces it with a test that the escape is refused.
func TestDefectD1WriteFileEscapesWorkDir(t *testing.T) {
	root := t.TempDir()
	work := filepath.Join(root, "work")
	if err := os.Mkdir(work, 0o755); err != nil {
		t.Fatal(err)
	}
	h := &MCPToolHandler{workDir: work}
	for _, path := range []string{"../escaped-relative.txt", filepath.Join(root, "escaped-absolute.txt")} {
		in, _ := json.Marshal(map[string]string{"path": path, "content": "x"})
		if _, err := h.writeFile(in); err != nil {
			t.Fatalf("D1 not reproduced: writeFile(%q) refused: %v", path, err)
		}
	}
	for _, name := range []string{"escaped-relative.txt", "escaped-absolute.txt"} {
		if _, err := os.Stat(filepath.Join(root, name)); err != nil {
			t.Fatalf("D1 not reproduced: %s was not written outside the workspace", name)
		}
	}
}
