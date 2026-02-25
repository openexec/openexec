package main

import (
	"bytes"
	"testing"
)

func TestVersionCmd(t *testing.T) {
	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetArgs([]string{"version"})

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got := buf.String()
	if got == "" {
		t.Fatal("expected version output, got empty string")
	}

	want := "axon dev (commit: none, built: unknown)"
	if got[:len(want)] != want {
		t.Errorf("got %q, want prefix %q", got, want)
	}
}
