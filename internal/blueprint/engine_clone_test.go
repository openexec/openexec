package blueprint

import "testing"

// The built-in blueprints are package-level pointers and the pipeline writes each
// run's timeout and commands into their stages. Without a copy, two concurrent
// runs decided each other's lint and test commands.
func TestCloneIsolatesStagesFromTheShared(t *testing.T) {
	original := DefaultBlueprint
	copied := original.Clone()

	lint, ok := copied.Stages["lint"]
	if !ok {
		t.Fatal("cloned blueprint has no lint stage")
	}
	lint.Commands = []string{"golangci-lint run"}
	lint.Timeout = 1

	if got := original.Stages["lint"]; len(got.Commands) != 0 || got.Timeout != 0 {
		t.Fatalf("mutating the clone changed the shared blueprint: commands=%v timeout=%v", got.Commands, got.Timeout)
	}
	if copied.Stages["lint"] == original.Stages["lint"] {
		t.Fatal("clone shares stage pointers with the original")
	}

	// Inputs is the other reference field on Stage.
	if src, ok := original.Stages["production_readiness_check"]; ok && len(src.Inputs) > 0 {
		clonedStage := copied.Stages["production_readiness_check"]
		for k := range clonedStage.Inputs {
			clonedStage.Inputs[k] = "mutated"
		}
		for k, v := range src.Inputs {
			if v == "mutated" {
				t.Fatalf("clone shares the Inputs map (key %q)", k)
			}
		}
	}
}
