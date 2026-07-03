package ai

import (
	"context"
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/openexec/openexec/internal/governance"
)

// Classification is the kind + risk tier the classifier assigns to a work item.
type Classification struct {
	Kind string `yaml:"kind" json:"kind"`
	Risk string `yaml:"risk" json:"risk"`
}

// classifyKeys locate the structured object inside a prose-wrapped response.
var classifyKeys = []string{"kind", "risk"}

const classifyPrompt = `You are a triage classifier for a software delivery pipeline.
Classify the following work item. Return ONLY YAML with exactly two fields:

kind: one of bug | feature | docs | ops | support_question | security | reliability
risk: one of low | medium | high | critical

Risk guidance:
- critical/high: security, authentication, authorization, payments, data loss,
  production infrastructure, or anything that could break many users.
- medium: user-facing behavior changes, new features touching core flows.
- low: docs, copy, isolated low-impact changes with easy rollback.
When uncertain, choose the HIGHER risk.

Title: %s

Body:
%s
`

// knownKinds / knownRisks constrain the classifier output to the governance
// vocabulary; anything else is treated as unset (kind) or conservative (risk).
var knownKinds = map[string]bool{
	governance.KindBug: true, governance.KindFeature: true, governance.KindDocs: true,
	governance.KindOps: true, governance.KindSupportQuestion: true,
	governance.KindSecurity: true, governance.KindReliability: true,
}
var knownRisks = map[string]bool{
	governance.RiskLow: true, governance.RiskMedium: true,
	governance.RiskHigh: true, governance.RiskCritical: true,
}

// ClassifyIntent runs a single-shot classification over a work item's intent and
// returns its kind and risk tier. The returned Risk is always a valid tier:
// an unrecognized or missing value is clamped to medium (conservative — a
// medium+ change requires human approval), so a garbled model response can never
// downgrade a change to an auto-approvable low. Kind may be "" if unrecognized.
func ClassifyIntent(ctx context.Context, completer Completer, title, body string) (*Classification, error) {
	if completer == nil {
		return nil, fmt.Errorf("governance/ai: classify requires a completer")
	}
	raw, err := completer.Complete(ctx, fmt.Sprintf(classifyPrompt, title, body))
	if err != nil {
		return nil, fmt.Errorf("governance/ai: classification completion failed: %w", err)
	}

	c := &Classification{}
	_ = yaml.Unmarshal([]byte(extractStructured(raw, classifyKeys)), c)

	c.Kind = strings.ToLower(strings.TrimSpace(c.Kind))
	if !knownKinds[c.Kind] {
		c.Kind = ""
	}
	c.Risk = strings.ToLower(strings.TrimSpace(c.Risk))
	if !knownRisks[c.Risk] {
		c.Risk = governance.RiskMedium // conservative default
	}
	return c, nil
}
