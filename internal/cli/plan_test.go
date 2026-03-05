package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPlanCmd_ValidateOnly(t *testing.T) {
	tmpDir := t.TempDir()
	oldCwd, _ := os.Getwd()
	os.Chdir(tmpDir)
	defer os.Chdir(oldCwd)

	t.Run("Valid Intent", func(t *testing.T) {
		content := `# Title
## Goals
- Goal 1
## Requirements
- US-001: Req
## Constraints
- Platform: macOS
- Shape: CLI
- Data Source: Local
`
		intentPath := filepath.Join(tmpDir, "INTENT.md")
		os.WriteFile(intentPath, []byte(content), 0644)

		b := bytes.NewBufferString("")
		rootCmd.SetOut(b)
		rootCmd.SetArgs([]string{"plan", intentPath, "--validate-only"})

		err := rootCmd.Execute()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if !strings.Contains(b.String(), "Validation passed") {
			t.Errorf("expected validation success, got: %s", b.String())
		}
	})

	t.Run("Invalid Intent", func(t *testing.T) {
		content := `# Title
Only Goals missing everything else.
`
		intentPath := filepath.Join(tmpDir, "INVALID.md")
		os.WriteFile(intentPath, []byte(content), 0644)

		b := bytes.NewBufferString("")
		rootCmd.SetOut(b)
		rootCmd.SetArgs([]string{"plan", intentPath, "--validate-only"})

		err := rootCmd.Execute()
		if err == nil {
			t.Error("expected error for invalid intent")
		}

		if !strings.Contains(b.String(), "Intent Validation") {
			t.Error("missing validation report in output")
		}
	})
}
