package loop

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Exercise the actual API tool dispatcher and re-read disk state after each call.
func TestAPIFileToolsWorkspaceBoundary(t *testing.T) {
	parent := t.TempDir()
	workspace := filepath.Join(parent, "workspace")
	if err := os.Mkdir(workspace, 0755); err != nil {
		t.Fatal(err)
	}
	h := &MCPToolHandler{workDir: workspace}
	outside := filepath.Join(parent, "secret.txt")
	if err := os.WriteFile(outside, []byte("secret"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"../secret.txt", outside, "nested/../../secret.txt", "../workspace-sibling/new/file.txt", parent} {
		for _, tool := range []string{"read_file", "write_file"} {
			t.Run(tool+"/"+path, func(t *testing.T) {
				input, err := json.Marshal(map[string]any{"path": path, "content": "overwrite", "create_directories": true})
				if err != nil {
					t.Fatal(err)
				}
				output, err := h.ExecuteTool(context.Background(), tool, input)
				if err == nil || !strings.Contains(err.Error(), "outside workspace") || output != "" {
					t.Fatalf("escape must be refused: output=%q err=%v", output, err)
				}
				data, err := os.ReadFile(outside)
				if err != nil || string(data) != "secret" {
					t.Fatalf("outside file changed: %q, %v", data, err)
				}
				if _, err := os.Stat(filepath.Join(parent, "workspace-sibling")); !os.IsNotExist(err) {
					t.Fatalf("outside directory created: %v", err)
				}
			})
		}
	}
	for _, path := range []string{"normal.txt", "nested/new/file.txt", filepath.Join(workspace, "absolute.txt"), "nested/../clean.txt", "..allowed.txt"} {
		t.Run("roundtrip/"+path, func(t *testing.T) {
			input, _ := json.Marshal(map[string]any{"path": path, "content": "persisted", "create_directories": true})
			if _, err := h.ExecuteTool(context.Background(), "write_file", input); err != nil {
				t.Fatal(err)
			}
			// A fresh handler reads the persisted file, rather than relying on write output.
			fresh := &MCPToolHandler{workDir: workspace}
			output, err := fresh.ExecuteTool(context.Background(), "read_file", input)
			if err != nil || output != "persisted" {
				t.Fatalf("roundtrip: %q, %v", output, err)
			}
			diskPath := path
			if !filepath.IsAbs(path) {
				diskPath = filepath.Join(workspace, path)
			}
			data, err := os.ReadFile(diskPath)
			if err != nil || string(data) != "persisted" {
				t.Fatalf("disk: %q, %v", data, err)
			}
		})
	}
}
