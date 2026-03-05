package cli

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestHelpCmd(t *testing.T) {
	t.Run("Standard Help", func(t *testing.T) {
		b := bytes.NewBufferString("")
		rootCmd.SetOut(b)
		rootCmd.SetArgs([]string{"help"})

		err := rootCmd.Execute()
		if err != nil {
			t.Fatalf("Execute failed: %v", err)
		}

		if !strings.Contains(b.String(), "Usage:") {
			t.Error("missing Usage in help output")
		}
	})

	t.Run("Help All", func(t *testing.T) {
		b := bytes.NewBufferString("")
		rootCmd.SetOut(b)
		rootCmd.SetArgs([]string{"help", "--all"})

		err := rootCmd.Execute()
		if err != nil {
			t.Fatalf("Execute failed: %v", err)
		}

		// Should contain multiple command sections
		if !strings.Contains(b.String(), "openexec doctor") {
			t.Error("missing doctor command in help --all")
		}
		if !strings.Contains(b.String(), "openexec config") {
			t.Error("missing config command in help --all")
		}
	})

	t.Run("Help JSON", func(t *testing.T) {
		b := bytes.NewBufferString("")
		rootCmd.SetOut(b)
		rootCmd.SetArgs([]string{"help", "--json"})

		err := rootCmd.Execute()
		if err != nil {
			t.Fatalf("Execute failed: %v", err)
		}

		var schema CLISchema
		err = json.Unmarshal(b.Bytes(), &schema)
		if err != nil {
			t.Fatalf("failed to parse JSON help: %v", err)
		}

		if schema.Name != "openexec" {
			t.Errorf("got name %q, want %q", schema.Name, "openexec")
		}
		if len(schema.Children) == 0 {
			t.Error("expected child commands in JSON schema")
		}
		if len(schema.EnvVars) == 0 {
			t.Error("expected env vars in root JSON schema")
		}
	})
}
