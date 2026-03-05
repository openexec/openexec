package cli

import (
	"bytes"
	"strings"
	"testing"
)

func TestVersionCmd(t *testing.T) {
	b := bytes.NewBufferString("")
	rootCmd.SetOut(b)
	rootCmd.SetArgs([]string{"version"})

	err := rootCmd.Execute()
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	out := b.String()
	if !strings.Contains(out, "OpenExec CLI v") {
		t.Errorf("unexpected output: %q", out)
	}
	if !strings.Contains(out, Version) {
		t.Errorf("output missing version %q: %q", Version, out)
	}
}
