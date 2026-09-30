package pipeline

import (
	"context"
	"testing"

	"github.com/openexec/openexec/internal/blueprint"
	"github.com/openexec/openexec/internal/types"
)

func TestRecaptureUnitInvalidStage(t *testing.T) {
	for _, stage := range []*blueprint.Stage{
		{Name: "verify", Type: types.StageTypeAgentic, Commands: []string{"exit 0"}},
		{Name: "verify", Type: types.StageTypeDeterministic},
		{Name: "verify", Type: types.StageTypeDeterministic, Commands: []string{"exit 0", "exit 0"}},
		{Name: "verify", Type: types.StageTypeDeterministic, Commands: []string{"exit 0"}, Action: "gate"},
	} {
		p, _ := New(Config{FWUID: "invalid-recapture", WorkDir: t.TempDir(), RecaptureStage: stage})
		if err := p.runBlueprintMode(context.Background()); err == nil {
			t.Fatal("invalid recapture stage accepted")
		}
	}
}
