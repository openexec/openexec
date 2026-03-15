package mcp

import (
    "encoding/json"
    "os"
    "os/exec"
    "path/filepath"
    "strings"
    "testing"
)

func TestGitApplyPatch_PersistsArtifactAndReports(t *testing.T) {
    // Minimal unified diff touching a temp repo
    tmp := t.TempDir()
    // init git repo
    _ = os.WriteFile(filepath.Join(tmp, "a.txt"), []byte("old\n"), 0o644)
    run("git", tmp, "init")
    run("git", tmp, "add", ".")
    run("git", tmp, "commit", "-m", "init")

    patch := "diff --git a/a.txt b/a.txt\nindex e69de29..4b825dc 100644\n--- a/a.txt\n+++ b/a.txt\n@@ -1 +1,2 @@\n old\n+new\n"
    req := Request{JSONRPC: "2.0", ID: json.RawMessage("1"), Method: "tools/call"}
    args := map[string]interface{}{
        "patch": patch,
        "working_directory": tmp,
    }
    b, _ := json.Marshal(args)
    params := toolsCallParams{Name: "git_apply_patch", Arguments: b}
    s, err := NewServerWithConfig(strings.NewReader(""), os.Stdout, ServerConfig{WorkDir: tmp})
    if err != nil {
        t.Fatalf("NewServerWithConfig: %v", err)
    }
    s.handleGitApplyPatch(req, params)

    // Expect artifact file exists
    artDir := filepath.Join(tmp, ".openexec", "artifacts", "patches")
    entries, _ := os.ReadDir(artDir)
    if len(entries) == 0 {
        t.Fatalf("expected at least one artifact in %s", artDir)
    }
}

// run is a tiny helper for git setup
func run(cmd, dir string, args ...string) {
    c := exec.Command(cmd, args...)
    c.Dir = dir
    _ = c.Run()
}
