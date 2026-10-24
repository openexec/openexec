package actions

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/openexec/openexec/internal/execution/gates"
	"github.com/openexec/openexec/internal/types"
)

// The registry's run_gates action is the route when no pipeline gate runner
// replaces it; a refusal without a failed gate must still say why.
func TestRunGatesActionRefusals(t *testing.T) {
	cases := []struct {
		name, yaml, wantErr string
		status              types.StageStatus
	}{
		{"none-declared", "project:\n  name: x\nquality:\n  gates: []\n", gates.NoGatesDeclared, types.StageStatusFailed},
		{"undefined", "project:\n  name: x\nquality:\n  gates: [typecheck]\n", "typecheck", types.StageStatusFailed},
		{"custom-passes", "project:\n  name: x\nquality:\n  gates: [ok]\n  custom:\n    - name: ok\n      command: exit 0\n", "", types.StageStatusCompleted},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, "openexec.yaml"), []byte(tc.yaml), 0o600); err != nil {
				t.Fatal(err)
			}
			action, ok := DefaultRegistry(dir).Get("run_gates")
			if !ok {
				t.Fatal("run_gates not registered")
			}
			resp, err := action.Execute(context.Background(), ActionRequest{WorkspaceDir: dir})
			if err != nil {
				t.Fatal(err)
			}
			if resp.Status != tc.status || (tc.wantErr == "" && resp.Error != "") || !strings.Contains(resp.Error, tc.wantErr) {
				t.Fatalf("status=%s error=%q output=%q", resp.Status, resp.Error, resp.Output)
			}
			if tc.name == "undefined" && !strings.Contains(resp.Output, "not defined under quality.custom") {
				t.Fatalf("undefined gate reads as a failed check: %q", resp.Output)
			}
		})
	}
}
