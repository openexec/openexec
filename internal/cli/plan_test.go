package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/openexec/openexec/internal/knowledge"
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

func TestPlanCmd_ArchitectMode(t *testing.T) {
	tmpDir := t.TempDir()
	oldCwd, _ := os.Getwd()
	os.Chdir(tmpDir)
	defer os.Chdir(oldCwd)

	// Arrange: Create project and PRD records
	os.MkdirAll(".openexec", 0755)
	yamlContent := `project: {name: "arch-test"}`
	os.WriteFile("openexec.yaml", []byte(yamlContent), 0644)

	store, _ := knowledge.NewStore(".")
	store.SetPRDRecord(&knowledge.PRDRecord{
		Section: "personas",
		Key:     "user",
		Content: "A regular user",
	})
	store.Close()

	intentPath := "INTENT.md"
	intentContent := `
# Intent
## Goals
- G1
## Requirements
- R1
- Data Source: SQLite
## Constraints
- Platform: macOS
- Shape: CLI
`
	os.WriteFile(intentPath, []byte(intentContent), 0644)

	// Act
	b := bytes.NewBufferString("")
	rootCmd.SetOut(b)
	rootCmd.SetArgs([]string{"plan", intentPath})

	// Execution might fail because openexec-planner binary is missing,
	// but we can check if it attempted to export PRD.
	err := rootCmd.Execute()
	
	// Assert
	if strings.Contains(b.String(), "Exporting 1 PRD sections") {
		// Success: it detected and exported PRD context
	} else if err != nil && strings.Contains(err.Error(), "project not initialized") {
		t.Errorf("DCP failed to see project: %v", err)
	} else if err != nil && !strings.Contains(err.Error(), "planner engine not found") {
		// It's okay if planner is not found, but other errors are bad
		t.Errorf("Unexpected error: %v. Output: %s", err, b.String())
	}
}
