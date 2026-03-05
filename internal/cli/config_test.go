package cli

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

func TestConfigCmd(t *testing.T) {
	tmpDir := t.TempDir()
	oldCwd, _ := os.Getwd()
	os.Chdir(tmpDir)
	defer os.Chdir(oldCwd)

	t.Run("Config Init", func(t *testing.T) {
		b := bytes.NewBufferString("")
		rootCmd.SetOut(b)
		rootCmd.SetArgs([]string{"config", "init", "--git"})

		err := rootCmd.Execute()
		if err != nil {
			t.Fatalf("Execute failed: %v", err)
		}

		if !strings.Contains(b.String(), "Configuration initialized") {
			t.Errorf("unexpected output: %s", b.String())
		}
		if !strings.Contains(b.String(), "Git integration: true") {
			t.Errorf("expected git enabled, output: %s", b.String())
		}
	})

	t.Run("Config Show", func(t *testing.T) {
		b := bytes.NewBufferString("")
		rootCmd.SetOut(b)
		rootCmd.SetArgs([]string{"config", "show"})

		err := rootCmd.Execute()
		if err != nil {
			t.Fatalf("Execute failed: %v", err)
		}

		if !strings.Contains(b.String(), "OpenExec Configuration") {
			t.Errorf("unexpected output: %s", b.String())
		}
	})

	t.Run("Config Set", func(t *testing.T) {
		b := bytes.NewBufferString("")
		rootCmd.SetOut(b)
		rootCmd.SetArgs([]string{"config", "set", "base_branch", "develop"})

		err := rootCmd.Execute()
		if err != nil {
			t.Fatalf("Execute failed: %v", err)
		}

		if !strings.Contains(b.String(), "Set base_branch = develop") {
			t.Errorf("unexpected output: %s", b.String())
		}

		// Verify change
		b.Reset()
		rootCmd.SetArgs([]string{"config", "show"})
		rootCmd.Execute()
		if !strings.Contains(b.String(), "base_branch:           develop") {
			t.Errorf("change not persisted, output: %s", b.String())
		}
	})
}
