package blueprint

import (
	"context"
	"strings"
	"testing"

	"github.com/openexec/openexec/internal/execution/evidence"
	"github.com/openexec/openexec/internal/execution/gates"
)

func TestRetentionBoundariesNativeCommand(t *testing.T) {
	executor := NewDefaultExecutor(t.TempDir())
	command := `TOKEN=NATIVE_COMMAND_SECRET; printf '%s\n' "$TOKEN"; printf 'token=NATIVE_DIAGNOSTIC_SECRET\n' >&2; head -c 20000 /dev/zero; exit 2`
	output, err := executor.runCommandWithCheck(context.Background(), command, "test")
	if err == nil || strings.Contains(output+err.Error(), "SECRET") {
		t.Fatal("native error missing or secret leaked")
	}
	refs := gates.VerificationFailureArtifacts(err)
	if !gates.ValidateVerificationFailureArtifacts(refs) || len(refs) != 3 {
		t.Fatal("native evidence missing")
	}
	for hash := range refs {
		if hash == gates.VerificationFailureReceiptKey || hash == gates.VerificationFailureDigestKey {
			continue
		}
		private, err := evidence.Read(executor.WorkDir, hash)
		if err != nil {
			t.Fatal(err)
		}
		if private.Argv[2] != command || len(private.Stdout) != evidence.StreamLimit || private.ExitCode != 2 {
			t.Fatal("native private identity/bounds lost")
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = executor.runCommandWithCheck(ctx, "exit 2", "test")
	if err == nil || gates.VerificationFailureArtifacts(err) != nil {
		t.Fatal("cancelled native command authorized repair")
	}
}
