package pipeline

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/openexec/openexec/internal/blueprint"
	"github.com/openexec/openexec/internal/execution/gates"
	"github.com/openexec/openexec/internal/loop"
	"github.com/openexec/openexec/internal/types"
)

func TestNamedRecaptureNativeDefinition(t *testing.T) {
	for _, gate := range []string{"lint", "test"} {
		for _, configured := range []bool{false, true} {
			label := gate + "/missing"
			if configured {
				label = gate + "/configured"
			}
			t.Run(label, func(t *testing.T) {
				dir := t.TempDir()
				if configured {
					if err := os.Mkdir(filepath.Join(dir, ".openexec"), 0700); err != nil {
						t.Fatal(err)
					}
					data, _ := json.Marshal(map[string]any{"execution": map[string]any{gate + "_commands": []string{"printf NATIVE_DIAGNOSTIC >&2; exit 2"}}})
					if err := os.WriteFile(filepath.Join(dir, ".openexec", "config.json"), data, 0600); err != nil {
						t.Fatal(err)
					}
				}
				p, events := New(Config{FWUID: "native-recapture", WorkDir: dir, BlueprintID: "standard_task", RecaptureStage: &blueprint.Stage{Name: gate, Type: types.StageTypeDeterministic}})
				defer p.Close()
				err := p.runBlueprintMode(context.Background())
				if err == nil {
					t.Fatal("empty definition or failed command accepted")
				}
				if !configured {
					if !strings.Contains(err.Error(), "no authoritative command") {
						t.Fatal(err)
					}
					return
				}
				var terminal *loop.Event
				for len(events) > 0 {
					event := <-events
					if event.Type == loop.EventBlueprintFailed {
						terminal = &event
					}
				}
				if terminal == nil || terminal.StageName != gate || !strings.Contains(terminal.Text, "NATIVE_DIAGNOSTIC") || terminal.Artifacts["recapture_definition"] != "current-check-definition" || !gates.ValidateVerificationFailureArtifacts(terminal.Artifacts) {
					t.Fatalf("native definition evidence lost: %+v", terminal)
				}
			})
		}
	}
}
