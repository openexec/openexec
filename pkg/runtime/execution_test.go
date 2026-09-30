package runtime

import (
	"context"
	"errors"
	"os/exec"
	"testing"

	"github.com/openexec/openexec/internal/execution/gates"
)

func TestWithOutputRefusesUnclassifiedErrors(t *testing.T) {
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	exit2 := exec.Command("sh", "-c", "exit 2").Run()
	for _, tc := range []struct {
		name string
		ctx  context.Context
		err  error
	}{
		{"success", context.Background(), nil},
		{"launch", context.Background(), exec.Command("missing_adapter_command_98312").Run()},
		{"transport", context.Background(), errors.New("transport failed")},
		{"reserved-exit", context.Background(), exec.Command("sh", "-c", "exit 127").Run()},
		{"cancelled", cancelled, exit2},
		{"mixed", context.Background(), errors.Join(exit2, errors.New("refused"))},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := VerificationCommandFailureWithOutput(tc.ctx, "test", tc.err, "make test", "tail")
			if got != tc.err || gates.VerificationFailureArtifacts(got) != nil {
				t.Fatal("unclassified error granted repair authority", got)
			}
		})
	}
}
