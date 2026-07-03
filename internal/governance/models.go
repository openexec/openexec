// Package governance provides the shared data layer for OpenExec release governance:
// multi-release tracking, change records bridging source systems to release items,
// versioned plans, review authorities, decision history, evidence, communication
// artifacts, and execution claim primitives.
//
// This package is additive. It creates its own SQLite tables and never modifies the
// existing release/story/task schema in internal/release.
package governance

import "time"

// GovernanceRelease is a durable, multi-instance release record. Unlike the
// single-active release model in internal/release, many of these can coexist.
type GovernanceRelease struct {
	ID              string
	Name            string
	Description     string
	Owner           string
	Status          string // see Release* status constants
	Goal            string
	MustHave        []string
	OutOfScope      []string
	Risk            string // see Risk* constants
	ApprovedForAI   bool
	ApprovedVersion int
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// ChangeRecord bridges a source system item (GitHub issue, Jira issue, manual,
// OpenSRE finding) to release items, AI plans, PRs, evidence, and communication.
type ChangeRecord struct {
	ID                 string
	ReleaseID          string
	ProjectID          string
	SourceType         string // see Source* constants
	SourceID           string
	SourceURL          string
	Title              string
	RawText            string
	Summary            string
	Kind               string // see Kind* constants
	Risk               string // see Risk* constants
	Status             string // see Change* status constants
	ProposalVersion    int
	ApprovedVersion    int
	Plan               string
	AcceptanceCriteria string
	VerificationPlan   string
	Branch             string
	PRURL              string
	CreatedAt          time.Time
	UpdatedAt          time.Time

	// Claim fields for executor handoff (lease-based concurrency control).
	ClaimedBy      string
	ClaimExpiresAt *time.Time
}

// ReleaseItem links a ChangeRecord to a GovernanceRelease. The link table keeps
// future flexibility even though a change normally belongs to one release at a time.
type ReleaseItem struct {
	ReleaseID string
	ChangeID  string
	ItemType  string // see Item* constants
	Priority  int
	Required  bool
	CreatedAt time.Time
}

// DecisionEvent records every proposal, review, approval, rejection, revision
// request, and done decision against a change.
type DecisionEvent struct {
	ID              string
	ReleaseID       string
	ChangeID        string
	ProposalVersion int
	Actor           string
	ActorType       string // see Actor* constants
	Decision        string // see Decision* constants
	Comment         string
	CreatedAt       time.Time
	// PrevHash and Hash form the tamper-evident chain. Hash =
	// SHA256(PrevHash || canonical(event)); PrevHash is the Hash of the
	// immediately preceding event (empty for the first). Set by the store on
	// insert; read back for verification and export. Not user-writable.
	PrevHash string
	Hash     string
}

// ReviewAuthority defines what a human, AI, verifier, or policy check may do.
type ReviewAuthority struct {
	ID          string
	Name        string
	Type        string   // see Authority* constants
	Permissions []string // see Perm* constants
	RiskLimit   string   // see Risk* constants
}

// Evidence is a structured record of test/CI/review/deploy/monitoring proof
// attached to a change.
type Evidence struct {
	ID        string
	ChangeID  string
	Kind      string // see EvidenceKind* constants
	Source    string // see EvidenceSource* constants
	Summary   string
	URL       string
	Raw       string
	CreatedAt time.Time
}

// CommunicationArtifact is audience-targeted output (tester handoff, customer
// summary, PM update) generated from stored records.
type CommunicationArtifact struct {
	ID        string
	ReleaseID string
	ChangeID  string
	Audience  string // see Audience* constants
	Body      string
	PostedTo  string
	PostedAt  *time.Time
	CreatedAt time.Time
}

// Release statuses.
const (
	ReleaseStatusDraft         = "draft"
	ReleaseStatusPlanned       = "planned"
	ReleaseStatusApproved      = "approved"
	ReleaseStatusImplementing  = "implementing"
	ReleaseStatusReadyForTest  = "ready_for_test"
	ReleaseStatusTesting       = "testing"
	ReleaseStatusReadyToDeploy = "ready_to_deploy"
	ReleaseStatusDeployed      = "deployed"
	ReleaseStatusClosed        = "closed"
	ReleaseStatusBlocked       = "blocked"
	ReleaseStatusCancelled     = "cancelled"
)

// ChangeRecord statuses.
const (
	ChangeStatusCandidate        = "candidate"
	ChangeStatusPlanned          = "planned"
	ChangeStatusPlanReady        = "plan_ready"
	ChangeStatusChangesRequested = "changes_requested"
	ChangeStatusApprovedForAI    = "approved_for_ai"
	ChangeStatusImplementing     = "implementing"
	ChangeStatusPROpen           = "pr_open"
	ChangeStatusReadyForTest     = "ready_for_test"
	ChangeStatusDone             = "done"
	ChangeStatusRejected         = "rejected"
	ChangeStatusDeferred         = "deferred"
	ChangeStatusBlocked          = "blocked"
)

// Risk tiers (shared by releases, change records, and review authority limits).
const (
	RiskLow      = "low"
	RiskMedium   = "medium"
	RiskHigh     = "high"
	RiskCritical = "critical"
)

// Change source types.
const (
	SourceGitHubIssue    = "github_issue"
	SourceJiraIssue      = "jira_issue"
	SourceManual         = "manual"
	SourceOpenSREFinding = "opensre_finding"
)

// Change kinds.
const (
	KindBug             = "bug"
	KindFeature         = "feature"
	KindDocs            = "docs"
	KindOps             = "ops"
	KindSupportQuestion = "support_question"
	KindSecurity        = "security"
	KindReliability     = "reliability"
)

// Release item types.
const (
	ItemTypeEpic        = "epic"
	ItemTypeStory       = "story"
	ItemTypeTask        = "task"
	ItemTypeBug         = "bug"
	ItemTypeRemediation = "remediation"
)

// Actor types for decision events.
const (
	ActorTypeHuman    = "human"
	ActorTypeAI       = "ai"
	ActorTypeVerifier = "verifier"
	ActorTypePolicy   = "policy"
	ActorTypeSystem   = "system"
)

// Decision types for decision events.
const (
	DecisionProposed            = "proposed"
	DecisionReviewed            = "reviewed"
	DecisionRecommendedApproval = "recommended_approval"
	DecisionApproved            = "approved"
	DecisionChangesRequested    = "changes_requested"
	DecisionRejected            = "rejected"
	DecisionDeferred            = "deferred"
	DecisionRiskAccepted        = "risk_accepted"
	DecisionMarkedDone          = "marked_done"
)

// Review authority types.
const (
	AuthorityHuman    = "human"
	AuthorityAI       = "ai"
	AuthorityVerifier = "verifier"
	AuthorityPolicy   = "policy"
)

// Review authority permissions.
const (
	PermComment           = "comment"
	PermRequestChanges    = "request_changes"
	PermRecommendApproval = "recommend_approval"
	PermApproveLowRisk    = "approve_low_risk"
	PermApprove           = "approve"
	PermRiskAccept        = "risk_accept"
	PermMarkDone          = "mark_done"
)

// Evidence kinds.
const (
	EvidenceKindTest       = "test"
	EvidenceKindCI         = "ci"
	EvidenceKindReview     = "review"
	EvidenceKindDeploy     = "deploy"
	EvidenceKindMonitoring = "monitoring"
	EvidenceKindManual     = "manual"
)

// Evidence sources.
const (
	EvidenceSourceCLI     = "cli"
	EvidenceSourceGitHub  = "github"
	EvidenceSourceJira    = "jira"
	EvidenceSourceWebhook = "webhook"
	EvidenceSourceHuman   = "human"
	// EvidenceSourceAgent marks evidence self-reported by an AI agent over the
	// MCP plane. It is NOT independently verified and must not be treated as a
	// trusted attestation (e.g. human or CI provenance).
	EvidenceSourceAgent = "agent"
)

// Communication audiences.
const (
	AudiencePM        = "pm"
	AudienceTester    = "tester"
	AudienceCustomer  = "customer"
	AudienceSupport   = "support"
	AudienceDeveloper = "developer"
)
