package blueprint

import (
	"os"
	"regexp"
	"testing"
)

// Compare the actual backend definition with the UI list, including repair
// branches. A new backend stage must not silently disappear from the timeline.
func TestDefaultBlueprintStagesAppearInUI(t *testing.T) {
	source, err := os.ReadFile("../../ui/src/types/blueprint.ts")
	if err != nil {
		t.Fatal(err)
	}
	list := regexp.MustCompile(`(?s)export const BLUEPRINT_STAGES[^=]*=\s*\[(.*?)\]`).FindSubmatch(source)
	if len(list) != 2 {
		t.Fatal("BLUEPRINT_STAGES array not found")
	}
	names := make(map[string]bool)
	for _, match := range regexp.MustCompile(`'([^']+)'`).FindAllSubmatch(list[1], -1) {
		name := string(match[1])
		if names[name] {
			t.Errorf("duplicate UI stage %q", name)
		}
		names[name] = true
		if _, ok := DefaultBlueprint.Stages[name]; !ok {
			t.Errorf("UI stage %q is not in DefaultBlueprint", name)
		}
	}
	for name := range DefaultBlueprint.Stages {
		if !names[name] {
			t.Errorf("default blueprint stage %q is missing from BLUEPRINT_STAGES", name)
		}
	}
}
