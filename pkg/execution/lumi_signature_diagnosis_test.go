package execution

import (
	"context"
	"os/exec"
	"testing"
	"time"
)

// Keep the pinned Python comparison executable through the declared Go suite.
// It imports no endpoint implementation and uses only invented public errors.
func TestLUMIReplaySignatureControl(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "python3", "-I", "-B", "testdata/lumi-signature/control.py")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("offline pinned Console signature control failed: %v\n%s", err, output)
	}
	t.Logf("%s", output)
}
