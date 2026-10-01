package cli

import (
	"strings"
	"testing"
)

func TestTaskCorrectAgentCannotSelfPromote(t *testing.T) {
	cmd := newTaskCorrectCommand(false)
	t.Setenv("OPENEXEC_OPERATOR_SESSION", "1")
	cmd.SetArgs([]string{"A", "--authorize-correction", "--decision-ref", "owner:decision"})
	if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), "operator session") {
		t.Fatalf("agent self-promoted: %v", err)
	}
}
