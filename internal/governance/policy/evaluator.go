package policy

import (
	"fmt"

	"github.com/openexec/openexec/internal/governance"
)

// Review kinds returned by RequiredReviews.
const (
	ReviewAIReview       = "ai_review"
	ReviewSecurityReview = "security_review"
)

// riskRank orders the risk tiers so an authority's RiskLimit can be compared
// against a change's risk. Higher means more severe.
var riskRank = map[string]int{
	governance.RiskLow:      0,
	governance.RiskMedium:   1,
	governance.RiskHigh:     2,
	governance.RiskCritical: 3,
}

// PolicyEvaluator answers approval questions against a loaded Policy. It holds
// no state beyond the policy and performs no I/O.
type PolicyEvaluator struct {
	policy *Policy
}

// NewEvaluator returns an evaluator over p. A nil p is replaced with
// DefaultPolicy so the evaluator is always usable.
func NewEvaluator(p *Policy) *PolicyEvaluator {
	if p == nil {
		p = DefaultPolicy()
	}
	return &PolicyEvaluator{policy: p}
}

// tierFor returns the TierPolicy for a risk value. Unknown or empty risk maps to
// the critical tier — the most conservative choice, so an unrecognized risk can
// never be treated as permissive.
func (e *PolicyEvaluator) tierFor(risk string) TierPolicy {
	if t, ok := e.policy.RiskTiers[risk]; ok {
		return t
	}
	if t, ok := e.policy.RiskTiers[governance.RiskCritical]; ok {
		return t
	}
	return DefaultPolicy().RiskTiers[governance.RiskCritical]
}

// rankOf returns the severity rank of a risk value. Unknown/empty risk ranks as
// critical so it exceeds any finite RiskLimit.
func rankOf(risk string) int {
	if r, ok := riskRank[risk]; ok {
		return r
	}
	return riskRank[governance.RiskCritical]
}

// hasPerm reports whether the authority holds the given permission.
func hasPerm(authority *governance.ReviewAuthority, perm string) bool {
	for _, p := range authority.Permissions {
		if p == perm {
			return true
		}
	}
	return false
}

// CanApprove reports whether authority may approve change ch under the policy.
// It returns false with a human-readable reason naming the missing authority,
// permission, or policy constraint. Approval requires, in order: a non-nil
// authority; an approve permission (Perm_approve, or Perm_approve_low_risk on a
// low-risk change); a RiskLimit at least as high as the change's risk; and that
// the tier policy permits this authority type (human-required tiers admit only
// humans; AI authorities additionally require ai_approval_allowed; verifier and
// policy authorities can recommend but never approve).
func (e *PolicyEvaluator) CanApprove(authority *governance.ReviewAuthority, ch *governance.ChangeRecord) (bool, string) {
	if authority == nil {
		return false, "no review authority provided"
	}
	if ch == nil {
		return false, "no change record provided"
	}

	// Permission: full approve, or low-risk-only approve on a low-risk change.
	canApproveLowOnly := hasPerm(authority, governance.PermApproveLowRisk) && ch.Risk == governance.RiskLow
	if !hasPerm(authority, governance.PermApprove) && !canApproveLowOnly {
		return false, fmt.Sprintf("authority %q lacks an approve permission for %s-risk change", authority.Name, riskLabel(ch.Risk))
	}

	// RiskLimit: the authority cannot approve above its configured ceiling.
	if authority.RiskLimit != "" && rankOf(ch.Risk) > rankOf(authority.RiskLimit) {
		return false, fmt.Sprintf("%s-risk change exceeds authority %q risk limit of %s", riskLabel(ch.Risk), authority.Name, riskLabel(authority.RiskLimit))
	}

	tier := e.tierFor(ch.Risk)

	// Human-required tiers admit only human authorities.
	if tier.HumanApprovalRequired && authority.Type != governance.AuthorityHuman {
		return false, fmt.Sprintf("%s-risk tier requires human approval; authority %q is type %s", riskLabel(ch.Risk), authority.Name, authority.Type)
	}

	switch authority.Type {
	case governance.AuthorityHuman:
		return true, ""
	case governance.AuthorityAI:
		if !tier.AIApprovalAllowed {
			return false, fmt.Sprintf("AI approval is not allowed for the %s-risk tier", riskLabel(ch.Risk))
		}
		return true, ""
	default:
		// Verifier and policy authorities recommend; they never grant approval.
		return false, fmt.Sprintf("authority type %s may recommend but cannot approve", authority.Type)
	}
}

// RequiredReviews lists the review kinds the policy requires before a change at
// its risk tier can be approved. The slice is always non-nil (empty for tiers
// with no required reviews) and ordered ai_review before security_review.
func (e *PolicyEvaluator) RequiredReviews(ch *governance.ChangeRecord) []string {
	reviews := []string{}
	if ch == nil {
		return reviews
	}
	tier := e.tierFor(ch.Risk)
	if tier.AIReviewRequired {
		reviews = append(reviews, ReviewAIReview)
	}
	if tier.SecurityReviewRequired {
		reviews = append(reviews, ReviewSecurityReview)
	}
	return reviews
}

// CanAutoApprove reports whether a change may be approved by AI/automation with
// no human in the loop. True only when the tier allows AI approval, does not
// require a human, and does not require a security review.
func (e *PolicyEvaluator) CanAutoApprove(ch *governance.ChangeRecord) bool {
	if ch == nil {
		return false
	}
	tier := e.tierFor(ch.Risk)
	return tier.AIApprovalAllowed && !tier.HumanApprovalRequired && !tier.SecurityReviewRequired
}

// CanAutoMerge reports whether a change of this risk tier may be merged WITHOUT
// a human operator (the caller must separately confirm verification evidence is
// present). False for every tier under DefaultPolicy — the merge gate fails
// closed, so a change never auto-merges unless an operator explicitly opts its
// tier in. This is the last line before a CI/CD production deploy.
func (e *PolicyEvaluator) CanAutoMerge(ch *governance.ChangeRecord) bool {
	if ch == nil {
		return false
	}
	return e.tierFor(ch.Risk).AutoMergeAllowed
}

// CanRiskAccept reports whether authority may accept the risk of change ch. It
// requires the Perm_risk_accept permission and, on tiers that set
// risk_acceptance_requires_human (critical by default), a human authority. The
// authority's RiskLimit must also cover the change's risk.
func (e *PolicyEvaluator) CanRiskAccept(authority *governance.ReviewAuthority, ch *governance.ChangeRecord) (bool, string) {
	if authority == nil {
		return false, "no review authority provided"
	}
	if ch == nil {
		return false, "no change record provided"
	}

	if !hasPerm(authority, governance.PermRiskAccept) {
		return false, fmt.Sprintf("authority %q lacks the risk_accept permission", authority.Name)
	}

	tier := e.tierFor(ch.Risk)
	if tier.RiskAcceptanceRequiresHuman && authority.Type != governance.AuthorityHuman {
		return false, fmt.Sprintf("%s-risk risk acceptance requires a human; authority %q is type %s", riskLabel(ch.Risk), authority.Name, authority.Type)
	}

	if authority.RiskLimit != "" && rankOf(ch.Risk) > rankOf(authority.RiskLimit) {
		return false, fmt.Sprintf("%s-risk change exceeds authority %q risk limit of %s", riskLabel(ch.Risk), authority.Name, riskLabel(authority.RiskLimit))
	}

	return true, ""
}

// RequiresRiskAcceptance reports whether a change's risk tier demands an explicit
// human risk-acceptance before it may be approved or merged (critical by
// default). The service enforces that a current acceptance is on record.
func (e *PolicyEvaluator) RequiresRiskAcceptance(ch *governance.ChangeRecord) bool {
	if ch == nil {
		return false
	}
	return e.tierFor(ch.Risk).RiskAcceptanceRequiresHuman
}

// CanApproveRelease reports whether authority may approve a release's scope.
// Release scope approval is the PM/delivery-lead boundary, so it is stricter
// than per-change approval: it requires the full approve permission (NOT
// approve_low_risk), a human authority (scope sign-off is a human accountability
// point — AI/verifier/policy authorities may advise but never approve scope),
// and a RiskLimit covering the release's risk. Returns false with a reason
// naming the missing requirement.
func (e *PolicyEvaluator) CanApproveRelease(authority *governance.ReviewAuthority, rel *governance.GovernanceRelease) (bool, string) {
	if authority == nil {
		return false, "no review authority provided"
	}
	if rel == nil {
		return false, "no release provided"
	}
	if !hasPerm(authority, governance.PermApprove) {
		return false, fmt.Sprintf("authority %q lacks the approve permission required for release scope", authority.Name)
	}
	if authority.Type != governance.AuthorityHuman {
		return false, fmt.Sprintf("release scope approval requires a human authority; %q is type %s", authority.Name, authority.Type)
	}
	risk := rel.Risk
	if risk == "" {
		risk = governance.RiskLow
	}
	if authority.RiskLimit != "" && rankOf(risk) > rankOf(authority.RiskLimit) {
		return false, fmt.Sprintf("%s-risk release exceeds authority %q risk limit of %s", riskLabel(risk), authority.Name, riskLabel(authority.RiskLimit))
	}
	return true, ""
}

// CanMarkDone reports whether authority may mark change ch done. Marking done is
// a verification action: it requires the mark_done permission and a RiskLimit
// covering the change's risk. Separation of duties (one actor cannot propose,
// approve, AND mark done on medium+ work) is enforced separately in validation.
func (e *PolicyEvaluator) CanMarkDone(authority *governance.ReviewAuthority, ch *governance.ChangeRecord) (bool, string) {
	if authority == nil {
		return false, "no review authority provided"
	}
	if ch == nil {
		return false, "no change record provided"
	}
	if !hasPerm(authority, governance.PermMarkDone) {
		return false, fmt.Sprintf("authority %q lacks the mark_done permission", authority.Name)
	}
	if authority.RiskLimit != "" && rankOf(ch.Risk) > rankOf(authority.RiskLimit) {
		return false, fmt.Sprintf("%s-risk change exceeds authority %q risk limit of %s", riskLabel(ch.Risk), authority.Name, riskLabel(authority.RiskLimit))
	}
	return true, ""
}

// riskLabel renders a risk value for messages, falling back to "unknown" for an
// empty value so reasons never read "-risk".
func riskLabel(risk string) string {
	if risk == "" {
		return "unknown"
	}
	return risk
}
