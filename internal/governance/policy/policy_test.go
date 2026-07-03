package policy

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/openexec/openexec/internal/governance"
)

// humanApprover is a full human authority that may approve up to critical.
func humanApprover() *governance.ReviewAuthority {
	return &governance.ReviewAuthority{
		Name:        "perttu",
		Type:        governance.AuthorityHuman,
		Permissions: []string{governance.PermApprove, governance.PermRiskAccept},
		RiskLimit:   governance.RiskCritical,
	}
}

// aiApprover is an AI authority limited to low risk.
func aiApprover() *governance.ReviewAuthority {
	return &governance.ReviewAuthority{
		Name:        "bugbot",
		Type:        governance.AuthorityAI,
		Permissions: []string{governance.PermApprove, governance.PermApproveLowRisk, governance.PermRiskAccept},
		RiskLimit:   governance.RiskLow,
	}
}

// aiApproverNoLimit is an AI authority whose risk limit is high enough that it
// is rejected by tier policy (human-required), not by its risk ceiling.
func aiApproverNoLimit() *governance.ReviewAuthority {
	return &governance.ReviewAuthority{
		Name:        "bugbot",
		Type:        governance.AuthorityAI,
		Permissions: []string{governance.PermApprove},
		RiskLimit:   governance.RiskCritical,
	}
}

func change(risk, kind string) *governance.ChangeRecord {
	return &governance.ChangeRecord{ID: "CHANGE-1", Risk: risk, Kind: kind}
}

func TestCanApprove(t *testing.T) {
	e := NewEvaluator(DefaultPolicy())

	tests := []struct {
		name      string
		authority *governance.ReviewAuthority
		change    *governance.ChangeRecord
		want      bool
		reasonHas string // substring expected in the reason when want==false
	}{
		{
			name:      "low risk docs AI-approvable when policy allows",
			authority: aiApprover(),
			change:    change(governance.RiskLow, governance.KindDocs),
			want:      true,
		},
		{
			name:      "low risk human-approvable",
			authority: humanApprover(),
			change:    change(governance.RiskLow, governance.KindDocs),
			want:      true,
		},
		{
			name:      "medium risk rejects AI",
			authority: aiApproverNoLimit(),
			change:    change(governance.RiskMedium, governance.KindFeature),
			want:      false,
			reasonHas: "requires human approval",
		},
		{
			name:      "medium risk allows human",
			authority: humanApprover(),
			change:    change(governance.RiskMedium, governance.KindFeature),
			want:      true,
		},
		{
			name:      "high risk rejects AI",
			authority: aiApproverNoLimit(),
			change:    change(governance.RiskHigh, governance.KindFeature),
			want:      false,
			reasonHas: "requires human approval",
		},
		{
			name:      "high risk allows human",
			authority: humanApprover(),
			change:    change(governance.RiskHigh, governance.KindSecurity),
			want:      true,
		},
		{
			name:      "critical risk allows human",
			authority: humanApprover(),
			change:    change(governance.RiskCritical, governance.KindSecurity),
			want:      true,
		},
		{
			name:      "critical risk rejects AI",
			authority: aiApproverNoLimit(),
			change:    change(governance.RiskCritical, governance.KindSecurity),
			want:      false,
			reasonHas: "requires human approval",
		},
		{
			name:      "nil authority refused",
			authority: nil,
			change:    change(governance.RiskLow, governance.KindDocs),
			want:      false,
			reasonHas: "no review authority",
		},
		{
			name: "missing approve permission refused",
			authority: &governance.ReviewAuthority{
				Name:        "viewer",
				Type:        governance.AuthorityHuman,
				Permissions: []string{governance.PermComment},
				RiskLimit:   governance.RiskCritical,
			},
			change:    change(governance.RiskLow, governance.KindDocs),
			want:      false,
			reasonHas: "lacks an approve permission",
		},
		{
			name: "risk limit exceeded refused",
			authority: &governance.ReviewAuthority{
				Name:        "junior",
				Type:        governance.AuthorityHuman,
				Permissions: []string{governance.PermApprove},
				RiskLimit:   governance.RiskLow,
			},
			change:    change(governance.RiskHigh, governance.KindFeature),
			want:      false,
			reasonHas: "exceeds authority",
		},
		{
			name: "verifier cannot approve even low risk",
			authority: &governance.ReviewAuthority{
				Name:        "ci",
				Type:        governance.AuthorityVerifier,
				Permissions: []string{governance.PermApprove},
				RiskLimit:   governance.RiskCritical,
			},
			change:    change(governance.RiskLow, governance.KindDocs),
			want:      false,
			reasonHas: "cannot approve",
		},
		{
			name: "approve_low_risk only works on low risk",
			authority: &governance.ReviewAuthority{
				Name:        "lowbot",
				Type:        governance.AuthorityHuman,
				Permissions: []string{governance.PermApproveLowRisk},
				RiskLimit:   governance.RiskCritical,
			},
			change:    change(governance.RiskMedium, governance.KindFeature),
			want:      false,
			reasonHas: "lacks an approve permission",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, reason := e.CanApprove(tt.authority, tt.change)
			if got != tt.want {
				t.Fatalf("CanApprove = %v (reason %q), want %v", got, reason, tt.want)
			}
			if !tt.want {
				if reason == "" {
					t.Fatalf("expected a non-empty reason on refusal")
				}
				if tt.reasonHas != "" && !strings.Contains(reason, tt.reasonHas) {
					t.Fatalf("reason %q does not contain %q", reason, tt.reasonHas)
				}
			}
		})
	}
}

func TestRequiredReviews(t *testing.T) {
	e := NewEvaluator(DefaultPolicy())

	tests := []struct {
		risk string
		want []string
	}{
		{governance.RiskLow, []string{}},
		{governance.RiskMedium, []string{ReviewAIReview}},
		{governance.RiskHigh, []string{ReviewAIReview, ReviewSecurityReview}},
		{governance.RiskCritical, []string{ReviewAIReview, ReviewSecurityReview}},
	}

	for _, tt := range tests {
		t.Run(tt.risk, func(t *testing.T) {
			got := e.RequiredReviews(change(tt.risk, governance.KindFeature))
			if got == nil {
				t.Fatalf("RequiredReviews returned nil, want non-nil slice")
			}
			if len(got) != len(tt.want) {
				t.Fatalf("RequiredReviews = %v, want %v", got, tt.want)
			}
			for i := range tt.want {
				if got[i] != tt.want[i] {
					t.Fatalf("RequiredReviews = %v, want %v", got, tt.want)
				}
			}
		})
	}
}

func TestCanAutoApprove(t *testing.T) {
	e := NewEvaluator(DefaultPolicy())

	tests := []struct {
		risk string
		want bool
	}{
		{governance.RiskLow, true},
		{governance.RiskMedium, false},
		{governance.RiskHigh, false},
		{governance.RiskCritical, false},
	}

	for _, tt := range tests {
		t.Run(tt.risk, func(t *testing.T) {
			if got := e.CanAutoApprove(change(tt.risk, governance.KindDocs)); got != tt.want {
				t.Fatalf("CanAutoApprove(%s) = %v, want %v", tt.risk, got, tt.want)
			}
		})
	}
}

func TestCanRiskAccept(t *testing.T) {
	e := NewEvaluator(DefaultPolicy())

	aiCritical := &governance.ReviewAuthority{
		Name:        "bugbot",
		Type:        governance.AuthorityAI,
		Permissions: []string{governance.PermRiskAccept},
		RiskLimit:   governance.RiskCritical,
	}
	noPerm := &governance.ReviewAuthority{
		Name:        "viewer",
		Type:        governance.AuthorityHuman,
		Permissions: []string{governance.PermComment},
		RiskLimit:   governance.RiskCritical,
	}

	tests := []struct {
		name      string
		authority *governance.ReviewAuthority
		change    *governance.ChangeRecord
		want      bool
		reasonHas string
	}{
		{
			name:      "human accepts critical security risk",
			authority: humanApprover(),
			change:    change(governance.RiskCritical, governance.KindSecurity),
			want:      true,
		},
		{
			name:      "human accepts high reliability risk",
			authority: humanApprover(),
			change:    change(governance.RiskHigh, governance.KindReliability),
			want:      true,
		},
		{
			name:      "AI cannot accept critical risk",
			authority: aiCritical,
			change:    change(governance.RiskCritical, governance.KindSecurity),
			want:      false,
			reasonHas: "requires a human",
		},
		{
			name:      "missing risk_accept permission refused",
			authority: noPerm,
			change:    change(governance.RiskLow, governance.KindDocs),
			want:      false,
			reasonHas: "lacks the risk_accept permission",
		},
		{
			name:      "nil authority refused",
			authority: nil,
			change:    change(governance.RiskCritical, governance.KindSecurity),
			want:      false,
			reasonHas: "no review authority",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, reason := e.CanRiskAccept(tt.authority, tt.change)
			if got != tt.want {
				t.Fatalf("CanRiskAccept = %v (reason %q), want %v", got, reason, tt.want)
			}
			if !tt.want {
				if reason == "" {
					t.Fatalf("expected a non-empty reason on refusal")
				}
				if tt.reasonHas != "" && !strings.Contains(reason, tt.reasonHas) {
					t.Fatalf("reason %q does not contain %q", reason, tt.reasonHas)
				}
			}
		})
	}
}

func release(risk string) *governance.GovernanceRelease {
	return &governance.GovernanceRelease{ID: "R-1", Risk: risk}
}

func TestCanApproveRelease(t *testing.T) {
	e := NewEvaluator(DefaultPolicy())

	// AI authority with the approve permission — release scope still requires a
	// human, so this is refused.
	aiWithApprove := &governance.ReviewAuthority{
		Name: "bot", Type: governance.AuthorityAI,
		Permissions: []string{governance.PermApprove}, RiskLimit: governance.RiskCritical,
	}
	// Human with only approve_low_risk lacks the full approve permission.
	humanLowOnly := &governance.ReviewAuthority{
		Name: "dev", Type: governance.AuthorityHuman,
		Permissions: []string{governance.PermApproveLowRisk}, RiskLimit: governance.RiskCritical,
	}
	// Human approver capped at medium risk.
	humanMedium := &governance.ReviewAuthority{
		Name: "lead", Type: governance.AuthorityHuman,
		Permissions: []string{governance.PermApprove}, RiskLimit: governance.RiskMedium,
	}

	tests := []struct {
		name      string
		authority *governance.ReviewAuthority
		rel       *governance.GovernanceRelease
		want      bool
		reasonHas string
	}{
		{"human full approver, low release", humanApprover(), release(governance.RiskLow), true, ""},
		{"human full approver, critical release", humanApprover(), release(governance.RiskCritical), true, ""},
		{"AI with approve permission refused", aiWithApprove, release(governance.RiskLow), false, "requires a human"},
		{"human without approve permission refused", humanLowOnly, release(governance.RiskLow), false, "lacks the approve permission"},
		{"human over risk limit refused", humanMedium, release(governance.RiskCritical), false, "risk limit"},
		{"nil authority refused", nil, release(governance.RiskLow), false, "no review authority"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, reason := e.CanApproveRelease(tt.authority, tt.rel)
			if got != tt.want {
				t.Fatalf("CanApproveRelease = %v (reason %q), want %v", got, reason, tt.want)
			}
			if !tt.want && tt.reasonHas != "" && !strings.Contains(reason, tt.reasonHas) {
				t.Fatalf("reason %q does not contain %q", reason, tt.reasonHas)
			}
		})
	}
}

func TestCanMarkDone(t *testing.T) {
	e := NewEvaluator(DefaultPolicy())

	verifierLow := &governance.ReviewAuthority{
		Name: "ci", Type: governance.AuthorityVerifier,
		Permissions: []string{governance.PermMarkDone}, RiskLimit: governance.RiskLow,
	}
	noPerm := &governance.ReviewAuthority{
		Name: "bugbot", Type: governance.AuthorityAI,
		Permissions: []string{governance.PermComment, governance.PermRequestChanges}, RiskLimit: governance.RiskHigh,
	}

	tests := []struct {
		name      string
		authority *governance.ReviewAuthority
		change    *governance.ChangeRecord
		want      bool
		reasonHas string
	}{
		{"verifier marks low-risk done", verifierLow, change(governance.RiskLow, governance.KindBug), true, ""},
		{"verifier over risk limit refused", verifierLow, change(governance.RiskHigh, governance.KindBug), false, "risk limit"},
		{"authority without mark_done refused", noPerm, change(governance.RiskLow, governance.KindBug), false, "lacks the mark_done permission"},
		{"nil authority refused", nil, change(governance.RiskLow, governance.KindBug), false, "no review authority"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, reason := e.CanMarkDone(tt.authority, tt.change)
			if got != tt.want {
				t.Fatalf("CanMarkDone = %v (reason %q), want %v", got, reason, tt.want)
			}
			if !tt.want && tt.reasonHas != "" && !strings.Contains(reason, tt.reasonHas) {
				t.Fatalf("reason %q does not contain %q", reason, tt.reasonHas)
			}
		})
	}
}

func TestLoadPolicyDefaultsWhenNoConfig(t *testing.T) {
	dir := t.TempDir()
	p, err := LoadPolicy(dir)
	if err != nil {
		t.Fatalf("LoadPolicy: %v", err)
	}
	if !p.Enabled || p.DefaultApproval != ApprovalHumanRequired {
		t.Fatalf("expected default policy, got %+v", p)
	}
	if got := len(p.RiskTiers); got != 4 {
		t.Fatalf("expected 4 tiers, got %d", got)
	}
}

func TestLoadPolicyFromYAML(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".openexec"), 0o755); err != nil {
		t.Fatal(err)
	}
	yamlBody := `
some_other_subsystem:
  ignored: true
release_governance:
  enabled: true
  default_approval: human_required
  risk_tiers:
    low:
      ai_approval_allowed: true
      human_approval_required: false
    high:
      ai_review_required: true
      security_review_required: true
      human_approval_required: true
`
	if err := os.WriteFile(filepath.Join(dir, ".openexec", "openexec.yaml"), []byte(yamlBody), 0o644); err != nil {
		t.Fatal(err)
	}

	p, err := LoadPolicy(dir)
	if err != nil {
		t.Fatalf("LoadPolicy: %v", err)
	}
	// Tiers present in the file are honored.
	if !p.RiskTiers[governance.RiskLow].AIApprovalAllowed {
		t.Fatalf("expected low tier ai_approval_allowed from file")
	}
	// Omitted tiers (medium, critical) are filled from defaults.
	if _, ok := p.RiskTiers[governance.RiskMedium]; !ok {
		t.Fatalf("expected medium tier to be filled from defaults")
	}
	if !p.RiskTiers[governance.RiskCritical].RiskAcceptanceRequiresHuman {
		t.Fatalf("expected critical tier default risk_acceptance_requires_human")
	}
}

func TestLoadPolicyFromJSONFallback(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".openexec"), 0o755); err != nil {
		t.Fatal(err)
	}
	jsonBody := `{
  "name": "demo",
  "release_governance": {
    "enabled": true,
    "default_approval": "human_required",
    "risk_tiers": {
      "low": {"ai_approval_allowed": true, "human_approval_required": false}
    }
  }
}`
	if err := os.WriteFile(filepath.Join(dir, ".openexec", "config.json"), []byte(jsonBody), 0o644); err != nil {
		t.Fatal(err)
	}

	p, err := LoadPolicy(dir)
	if err != nil {
		t.Fatalf("LoadPolicy: %v", err)
	}
	if !p.Enabled {
		t.Fatalf("expected enabled policy from json")
	}
	e := NewEvaluator(p)
	if !e.CanAutoApprove(change(governance.RiskLow, governance.KindDocs)) {
		t.Fatalf("expected low-risk auto-approve from json policy")
	}
}

func TestLoadPolicyMalformedYAMLErrors(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".openexec"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".openexec", "openexec.yaml"), []byte("release_governance: [unterminated"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadPolicy(dir); err == nil {
		t.Fatalf("expected error on malformed yaml")
	}
}

func TestLoadPolicyYAMLWithoutGovernanceFallsThrough(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".openexec"), 0o755); err != nil {
		t.Fatal(err)
	}
	// YAML exists but has no release_governance key -> fall through to defaults.
	if err := os.WriteFile(filepath.Join(dir, ".openexec", "openexec.yaml"), []byte("other: 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	p, err := LoadPolicy(dir)
	if err != nil {
		t.Fatalf("LoadPolicy: %v", err)
	}
	if p.DefaultApproval != ApprovalHumanRequired {
		t.Fatalf("expected default policy when yaml lacks release_governance")
	}
}

func TestClampNoRelaxFromWorkspace(t *testing.T) {
	// A workspace policy that tries to WEAKEN the high tier (drop human approval,
	// allow AI approval) must be clamped back to at least the defaults.
	weak := &Policy{
		Enabled:         true,
		DefaultApproval: ApprovalAIAllowed,
		RiskTiers: map[string]TierPolicy{
			governance.RiskHigh: {
				AIApprovalAllowed:     true,
				HumanApprovalRequired: false,
			},
		},
	}
	clamped := clampNoRelax(weak)
	high := clamped.RiskTiers[governance.RiskHigh]
	def := DefaultPolicy().RiskTiers[governance.RiskHigh]
	if high.AIApprovalAllowed {
		t.Fatalf("workspace policy must not enable AI approval for high risk")
	}
	if !high.HumanApprovalRequired {
		t.Fatalf("workspace policy must not drop human approval for high risk")
	}
	if !high.SecurityReviewRequired || high.SecurityReviewRequired != def.SecurityReviewRequired {
		t.Fatalf("security review requirement must be preserved from defaults")
	}
	// A workspace policy may still TIGHTEN low risk (require a human).
	tighten := &Policy{
		RiskTiers: map[string]TierPolicy{
			governance.RiskLow: {HumanApprovalRequired: true},
		},
	}
	if !clampNoRelax(tighten).RiskTiers[governance.RiskLow].HumanApprovalRequired {
		t.Fatalf("workspace policy must be able to add a human-approval requirement")
	}
}

func TestClampNoRelaxDropsUnknownTiers(t *testing.T) {
	// A workspace policy inventing a tier and opting it into auto-merge must NOT
	// survive the clamp — unknown tiers are dropped (evaluator falls back to the
	// critical tier for unknown risk).
	p := &Policy{
		RiskTiers: map[string]TierPolicy{
			"funky": {AutoMergeAllowed: true, HumanApprovalRequired: false},
		},
	}
	clamped := clampNoRelax(p)
	if _, ok := clamped.RiskTiers["funky"]; ok {
		t.Fatalf("unknown tier 'funky' must be dropped, not preserved")
	}
	// A change with an unknown risk therefore evaluates against the critical tier.
	e := NewEvaluator(clamped)
	if e.CanAutoMerge(&governance.ChangeRecord{Risk: "funky"}) {
		t.Fatalf("unknown-risk change must not be auto-mergeable")
	}
}
