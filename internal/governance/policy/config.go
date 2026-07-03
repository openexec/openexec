// Package policy implements the risk-tier authority and approval policy engine
// for OpenExec release governance (Phase 3 of
// docs/RELEASE_GOVERNANCE_IMPLEMENTATION_PLAN.md).
//
// Scope: this package owns ONLY risk-tier approval authority — which authority
// type may approve / auto-approve / risk-accept a change at a given risk tier.
// Status-transition tables and structural state rules (empty-release,
// stale-approval, etc.) belong to internal/governance/validation, not here.
//
// Evaluation is pure: given a loaded *Policy plus the already-loaded
// governance structs, the evaluator answers yes/no with a reason. The only
// I/O in the package is config-file reading inside LoadPolicy.
package policy

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/openexec/openexec/internal/governance"
	"gopkg.in/yaml.v3"
)

// Config file locations, relative to the workspace root. LoadPolicy prefers the
// YAML file (when it carries a release_governance block) and falls back to the
// JSON project config.
const (
	yamlConfigPath = ".openexec/openexec.yaml"
	jsonConfigPath = ".openexec/config.json"
	// rootKey is the top-level key both files nest the policy under.
	rootKey = "release_governance"
)

// default_approval values.
const (
	ApprovalHumanRequired = "human_required"
	ApprovalAIAllowed     = "ai_allowed"
)

// TierPolicy is the per-risk-tier approval policy. Every field defaults to the
// zero value (false), so an absent tier in a config file is the most permissive
// possible — which is why LoadPolicy fills missing tiers from DefaultPolicy
// rather than leaving them zero.
type TierPolicy struct {
	// AIReviewRequired requires an AI review before approval is meaningful.
	AIReviewRequired bool `yaml:"ai_review_required" json:"ai_review_required"`
	// AIApprovalAllowed lets an AI-type authority grant approval at this tier.
	AIApprovalAllowed bool `yaml:"ai_approval_allowed" json:"ai_approval_allowed"`
	// HumanApprovalRequired restricts approval at this tier to human authorities.
	HumanApprovalRequired bool `yaml:"human_approval_required" json:"human_approval_required"`
	// SecurityReviewRequired adds a security review to RequiredReviews.
	SecurityReviewRequired bool `yaml:"security_review_required" json:"security_review_required"`
	// RiskAcceptanceRequiresHuman restricts risk acceptance to humans.
	RiskAcceptanceRequiresHuman bool `yaml:"risk_acceptance_requires_human" json:"risk_acceptance_requires_human"`
}

// Policy is the loaded release-governance policy. RiskTiers is keyed by the
// governance.Risk* constants (low/medium/high/critical).
type Policy struct {
	Enabled         bool                  `yaml:"enabled" json:"enabled"`
	DefaultApproval string                `yaml:"default_approval" json:"default_approval"`
	RiskTiers       map[string]TierPolicy `yaml:"risk_tiers" json:"risk_tiers"`
}

// configWrapper matches the nesting in both file formats: the policy lives under
// a top-level release_governance key.
type configWrapper struct {
	ReleaseGovernance *Policy `yaml:"release_governance" json:"release_governance"`
}

// DefaultPolicy returns the conservative defaults from the plan: low risk is
// AI-approvable with no human required; medium/high/critical require a human;
// high/critical require a security review; critical requires a human to accept
// risk.
func DefaultPolicy() *Policy {
	return &Policy{
		Enabled:         true,
		DefaultApproval: ApprovalHumanRequired,
		RiskTiers: map[string]TierPolicy{
			governance.RiskLow: {
				AIReviewRequired:      false,
				AIApprovalAllowed:     true,
				HumanApprovalRequired: false,
			},
			governance.RiskMedium: {
				AIReviewRequired:      true,
				AIApprovalAllowed:     false,
				HumanApprovalRequired: true,
			},
			governance.RiskHigh: {
				AIReviewRequired:       true,
				SecurityReviewRequired: true,
				AIApprovalAllowed:      false,
				HumanApprovalRequired:  true,
			},
			governance.RiskCritical: {
				AIReviewRequired:            true,
				SecurityReviewRequired:      true,
				AIApprovalAllowed:           false,
				HumanApprovalRequired:       true,
				RiskAcceptanceRequiresHuman: true,
			},
		},
	}
}

// LoadPolicy loads the policy for a workspace. Resolution order:
//  1. .openexec/openexec.yaml, if it parses and carries a release_governance block.
//  2. .openexec/config.json, if it parses and carries a release_governance block.
//  3. DefaultPolicy().
//
// Missing files are not errors (they fall through to defaults); a present file
// that fails to parse IS an error so a typo'd policy never silently degrades to
// permissive defaults. Unknown keys are tolerated (no KnownFields enforcement):
// these files are shared with other subsystems, so foreign keys are expected.
// Any tier omitted by the file is filled from DefaultPolicy so an absent tier
// can never read as "no requirements".
func LoadPolicy(baseDir string) (*Policy, error) {
	// 1. Operator-owned home policy is TRUSTED as authored: it lives outside any
	//    agent-writable workspace (~/.openexec/governance-policy.yaml), so an
	//    operator can grant e.g. AI approval for low-risk work here.
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		homePath := filepath.Join(home, ".openexec", "governance-policy.yaml")
		if p, found, err := loadFromYAML(homePath); err != nil {
			return nil, err
		} else if found {
			return p, nil
		}
	}

	// 2. Workspace policy is UNTRUSTED for relaxation: it sits inside the
	//    agent-writable project, so it may only TIGHTEN the conservative
	//    defaults (add reviews / require humans), never grant more permission.
	//    Otherwise an agent that edits .openexec/openexec.yaml could weaken its
	//    own approval gates (trust-boundary inversion).
	yamlPath := filepath.Join(baseDir, filepath.FromSlash(yamlConfigPath))
	if p, found, err := loadFromYAML(yamlPath); err != nil {
		return nil, err
	} else if found {
		return clampNoRelax(p), nil
	}

	jsonPath := filepath.Join(baseDir, filepath.FromSlash(jsonConfigPath))
	if p, found, err := loadFromJSON(jsonPath); err != nil {
		return nil, err
	} else if found {
		return clampNoRelax(p), nil
	}

	return DefaultPolicy(), nil
}

// clampNoRelax returns a copy of p made at least as strict as DefaultPolicy:
// permission flags are ANDed with the default (a workspace file can only remove
// permission) and requirement flags are ORed (it can only add requirements). A
// workspace-authored policy can therefore never weaken a built-in approval gate.
func clampNoRelax(p *Policy) *Policy {
	def := DefaultPolicy()
	out := &Policy{Enabled: p.Enabled, DefaultApproval: p.DefaultApproval, RiskTiers: map[string]TierPolicy{}}
	for risk, dt := range def.RiskTiers {
		t := dt
		if pt, ok := p.RiskTiers[risk]; ok {
			t.AIApprovalAllowed = pt.AIApprovalAllowed && dt.AIApprovalAllowed
			t.AIReviewRequired = pt.AIReviewRequired || dt.AIReviewRequired
			t.HumanApprovalRequired = pt.HumanApprovalRequired || dt.HumanApprovalRequired
			t.SecurityReviewRequired = pt.SecurityReviewRequired || dt.SecurityReviewRequired
			t.RiskAcceptanceRequiresHuman = pt.RiskAcceptanceRequiresHuman || dt.RiskAcceptanceRequiresHuman
		}
		out.RiskTiers[risk] = t
	}
	// Any extra tier a workspace file defines is clamped against the strictest
	// (critical) default so it cannot become a permissive back door.
	crit := def.RiskTiers[governance.RiskCritical]
	for risk, pt := range p.RiskTiers {
		if _, ok := out.RiskTiers[risk]; ok {
			continue
		}
		t := pt
		t.AIApprovalAllowed = pt.AIApprovalAllowed && crit.AIApprovalAllowed
		t.HumanApprovalRequired = pt.HumanApprovalRequired || crit.HumanApprovalRequired
		out.RiskTiers[risk] = t
	}
	return out
}

// loadFromYAML reads path and returns (policy, true, nil) only when the file
// exists, parses, and contains a release_governance block. A missing file
// returns (nil, false, nil); a malformed file returns an error.
func loadFromYAML(path string) (*Policy, bool, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("read %s: %w", yamlConfigPath, err)
	}

	var wrap configWrapper
	if err := yaml.NewDecoder(strings.NewReader(string(data))).Decode(&wrap); err != nil {
		return nil, false, fmt.Errorf("parse %s: %w", yamlConfigPath, err)
	}
	if wrap.ReleaseGovernance == nil {
		return nil, false, nil
	}
	return normalize(wrap.ReleaseGovernance), true, nil
}

// loadFromJSON mirrors loadFromYAML for the JSON project config.
func loadFromJSON(path string) (*Policy, bool, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("read %s: %w", jsonConfigPath, err)
	}

	var wrap configWrapper
	if err := json.Unmarshal(data, &wrap); err != nil {
		return nil, false, fmt.Errorf("parse %s: %w", jsonConfigPath, err)
	}
	if wrap.ReleaseGovernance == nil {
		return nil, false, nil
	}
	return normalize(wrap.ReleaseGovernance), true, nil
}

// normalize fills any tier the loaded policy omitted from DefaultPolicy, so the
// evaluator always sees all four tiers. An absent tier must never read as a tier
// with no requirements.
func normalize(p *Policy) *Policy {
	if p.RiskTiers == nil {
		p.RiskTiers = map[string]TierPolicy{}
	}
	defaults := DefaultPolicy().RiskTiers
	for _, tier := range []string{governance.RiskLow, governance.RiskMedium, governance.RiskHigh, governance.RiskCritical} {
		if _, ok := p.RiskTiers[tier]; !ok {
			p.RiskTiers[tier] = defaults[tier]
		}
	}
	return p
}
