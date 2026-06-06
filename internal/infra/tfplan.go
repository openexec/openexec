package infra

import (
	"encoding/json"
	"fmt"
)

// Deterministic terraform plan gating (Phase 2 of the SRE roadmap):
// instead of asking an LLM to "review" plan text, `terraform show -json`
// output is parsed structurally and destructive actions are detected in Go.

// tfPlan is the subset of `terraform show -json <planfile>` we gate on.
type tfPlan struct {
	ResourceChanges []struct {
		Address string `json:"address"`
		Change  struct {
			Actions []string `json:"actions"`
		} `json:"change"`
	} `json:"resource_changes"`
}

// DestructiveChanges parses plan JSON and returns one line per resource
// that would be deleted or replaced.
//
// NOTE: Terraform never emits a literal "replace" action. Replacement is
// encoded as the action PAIR ["delete","create"] (or ["create","delete"]
// with create_before_destroy), so detecting "delete" catches both pure
// deletions and all replacements; the pair is labeled "replace" for the
// operator.
//
// A parse error is returned as an error, never as "no destructive
// changes" — an unverifiable plan must not pass the gate.
func DestructiveChanges(planJSON []byte) ([]string, error) {
	var plan tfPlan
	if err := json.Unmarshal(planJSON, &plan); err != nil {
		return nil, fmt.Errorf("malformed plan json: %w", err)
	}
	var destructive []string
	for _, rc := range plan.ResourceChanges {
		for _, action := range rc.Change.Actions {
			if action == "delete" {
				kind := "delete"
				if len(rc.Change.Actions) > 1 { // delete paired with create = replace
					kind = "replace"
				}
				destructive = append(destructive, fmt.Sprintf("[%s] %s", kind, rc.Address))
			}
		}
	}
	return destructive, nil
}
