package ai

import (
	"context"
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// OperabilityReport is the production-readiness / SRE review of a change: the
// operability impact a senior engineer evaluates before deploying — not "is the
// code correct" but "is it safe to ship, and can we undo it."
type OperabilityReport struct {
	RollbackSafe string   `yaml:"rollback_safe" json:"rollback_safe"` // yes | conditional | no
	DBMigration  string   `yaml:"db_migration" json:"db_migration"`   // none | additive | destructive
	DeployRisk   string   `yaml:"deploy_risk" json:"deploy_risk"`     // low | medium | high | critical
	Mitigations  []string `yaml:"mitigations" json:"mitigations"`
	Monitoring   []string `yaml:"monitoring" json:"monitoring"`
	Notes        string   `yaml:"notes" json:"notes"`
}

// AutoMergeSafe reports whether a change's operability profile is safe enough to
// auto-merge WITHOUT a human. It is intentionally conservative and fails closed:
// a change is auto-merge-safe only if it can be cleanly rolled back, needs no
// destructive DB migration, and is low deploy risk. Any unknown/blank value
// counts as unsafe. The merge gate uses this so an operationally-risky change
// can never auto-deploy even when policy would otherwise allow the risk tier.
func (r *OperabilityReport) AutoMergeSafe() bool {
	if r == nil {
		return false
	}
	if r.RollbackSafe != "yes" {
		return false
	}
	if r.DBMigration != "none" && r.DBMigration != "additive" {
		return false
	}
	switch r.DeployRisk {
	case "low", "medium":
		return true
	default:
		return false
	}
}

var operabilityKeys = []string{"rollback_safe", "db_migration", "deploy_risk", "mitigations", "monitoring"}

const operabilityPrompt = `You are a senior SRE reviewing a change for PRODUCTION deployment safety — not
whether the code is correct, but whether it is safe to ship and can be undone.

Given the change intent and repository excerpts, assess operability impact.
Return ONLY YAML:

rollback_safe: yes | conditional | no        # can a deploy of this be cleanly reverted?
db_migration: none | additive | destructive  # does it change schema/data? destructive = drops/renames/backfills that a revert cannot restore
deploy_risk: low | medium | high | critical  # blast radius if it goes wrong
mitigations:                                 # if not cleanly rollback-safe, how to de-risk (feature flag, staged rollout, backfill-then-switch, ...)
  - ...
monitoring:                                  # what to watch after deploy to confirm health
  - ...
notes: anything uncertain

Be conservative: when unsure, choose the SAFER-to-assume-worse value (no / destructive / higher risk).

CHANGE INTENT:
%s

REPOSITORY EXCERPTS:
%s
`

// AnalyzeOperability runs a single-shot SRE production-readiness review over the
// change intent + real repo excerpts. Values are clamped to the known
// vocabulary; unrecognized/blank values clamp to the conservative-worst option
// so the report can only make the merge gate stricter, never laxer.
func AnalyzeOperability(ctx context.Context, completer Completer, intent, repoExcerpts string) (*OperabilityReport, error) {
	if completer == nil {
		return nil, fmt.Errorf("governance/ai: operability analysis requires a completer")
	}
	raw, err := completer.Complete(ctx, fmt.Sprintf(operabilityPrompt, intent, repoExcerpts))
	if err != nil {
		return nil, fmt.Errorf("governance/ai: operability completion failed: %w", err)
	}
	r := &OperabilityReport{}
	_ = yaml.Unmarshal([]byte(extractStructured(raw, operabilityKeys)), r)

	r.RollbackSafe = clamp(r.RollbackSafe, map[string]bool{"yes": true, "conditional": true, "no": true}, "no")
	r.DBMigration = clamp(r.DBMigration, map[string]bool{"none": true, "additive": true, "destructive": true}, "destructive")
	r.DeployRisk = clamp(r.DeployRisk, map[string]bool{"low": true, "medium": true, "high": true, "critical": true}, "high")
	return r, nil
}

// clamp lowercases v and returns it if in allowed, else the conservative fallback.
func clamp(v string, allowed map[string]bool, fallback string) string {
	v = strings.ToLower(strings.TrimSpace(v))
	if allowed[v] {
		return v
	}
	return fallback
}
