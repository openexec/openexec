package blueprint

import (
	"context"
	"testing"

	"github.com/openexec/openexec/internal/execution/gates"
	"github.com/openexec/openexec/internal/types"
)

func TestConfiguredVerificationCommandEvidence(t *testing.T) {
	for _, tc := range []struct {
		name, command    string
		registered, want bool
	}{
		{"configured failure", "printf private-stderr >&2; exit 1", true, true},
		{"arbitrary command", "exit 1", false, false},
		{"missing executable", "unknown_executable_12345", true, false},
		{"passed", "exit 0", true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stage := &Stage{Name: "check", Type: types.StageTypeDeterministic, Commands: []string{tc.command}}
			e := NewDefaultExecutor(t.TempDir())
			e.VerificationStages = map[*Stage]bool{stage: tc.registered}
			var receipt map[string]string
			e.OnVerificationFailure = func(_ *Stage, err error) { receipt = gates.VerificationFailureArtifacts(err) }
			_, err := e.Execute(context.Background(), stage, &StageInput{})
			if err != nil {
				t.Fatal(err)
			}
			if (receipt != nil) != tc.want {
				t.Fatalf("classified=%v want=%v", receipt, tc.want)
			}
			// Another stage with the same name is not the authorized check pointer.
			copyStage := *stage
			receipt = nil
			e.Execute(context.Background(), &copyStage, &StageInput{})
			if receipt != nil {
				t.Fatal("stage name substituted for configured verification identity")
			}
		})
	}
}
