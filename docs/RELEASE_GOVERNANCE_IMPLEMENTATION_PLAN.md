# Release Governance Implementation Plan

## Objective

Build release governance as a concrete OpenExec capability, not only a planning concept.

The first useful version should let a PM/developer create a release, attach GitHub or Jira work, ask AI to draft plans, run AI or human review, approve the release scope, expose only approved work to Codex/Claude, record PR/test evidence, and generate tester/customer communication.

Core invariant:

```text
Execution engines write code.
OpenExec decides what work is allowed, tracks evidence, and records communication.
```

## Current Codebase Anchors

Use existing OpenExec seams before adding new subsystems:

- `internal/release`: existing Release, Story, Task, SQLite store, changelog, approval, auto-merge, and tests.
- `internal/cli/release.go`: current `release`, `story`, and `task` commands.
- `internal/approval`: reusable approval request, policy, decision, and SQLite repository concepts.
- `internal/mcp`: MCP surface for external agents.
- `internal/git`: branch, commit, and PR-adjacent git primitives.
- `ui`: React/Vite dashboard surface.
- `.openexec/openexec.db`: canonical SQLite state store.

Important gap: `internal/release` currently models mostly one active release and local stories/tasks. Release governance needs multiple releases, external work links, versioned plans, review authorities, policy validation, execution claims, PR evidence, and communication state.

## Non-Goals For First Slice

- Do not build a custom coding agent.
- Do not require direct OpenAI/Anthropic API usage; CLI and MCP handoff is enough.
- Do not implement full Jira first. GitHub issues are the first integration.
- Do not enable product auto-merge by default.
- Do not build a large PM dashboard before the CLI workflow is correct.

## Target Vertical Slice

```text
GitHub issue
-> Change Record
-> attached to Release
-> AI triage plan
-> reviewer AI comments
-> PM/developer approves release item
-> Codex/Claude picks approved work through OpenExec
-> PR opened
-> CI/test evidence recorded
-> release ready for test
-> tester handoff and customer-safe summary generated
```

## Phase 0: Inventory And Compatibility

### Tasks

- Audit current `internal/release` structs, schema, store methods, and CLI behavior.
- Decide whether new release-governance code lives inside `internal/release` or a new package such as `internal/governance`.
- Keep existing `release`, `story`, and `task` commands working.
- Add a short migration note for existing `.openexec/openexec.db` users.

### Acceptance Criteria

- Existing release tests still pass.
- Existing release JSON export remains export-only.
- No command starts reading runtime state from JSON.
- New tables can be added without deleting existing data.

## Phase 1: Data Model And SQLite Schema

### 1. Multi-Release Model

Add a durable release model that supports multiple concurrent or historical releases.

Suggested fields:

```go
type GovernanceRelease struct {
    ID              string
    Name            string
    Description     string
    Owner           string
    Status          string // draft | planned | approved | implementing | ready_for_test | testing | ready_to_deploy | deployed | closed | blocked | cancelled
    Goal            string
    MustHave        []string
    OutOfScope      []string
    Risk            string // low | medium | high | critical
    ApprovedForAI   bool
    ApprovedVersion int
    CreatedAt       time.Time
    UpdatedAt       time.Time
}
```

Storage:

- Add `governance_releases`.
- Keep existing `releases` table for backward compatibility until migration is explicit.
- Add indexes on `status`, `owner`, and `updated_at`.

### 2. Change Record Model

Change Records are the bridge between source systems, release items, AI plans, PRs, evidence, and communication.

Suggested fields:

```go
type ChangeRecord struct {
    ID                 string
    ReleaseID          string
    ProjectID          string
    SourceType         string // github_issue | jira_issue | manual | opensre_finding
    SourceID           string
    SourceURL          string
    Title              string
    RawText            string
    Summary            string
    Kind               string // bug | feature | docs | ops | support_question | security | reliability
    Risk               string // low | medium | high | critical
    Status             string // candidate | planned | plan_ready | changes_requested | approved_for_ai | implementing | pr_open | ready_for_test | done | rejected | deferred | blocked
    ProposalVersion    int
    ApprovedVersion    int
    Plan               string
    AcceptanceCriteria string
    VerificationPlan   string
    Branch             string
    PRURL              string
    CreatedAt          time.Time
    UpdatedAt          time.Time
}
```

Storage:

- Add `change_records`.
- Add unique index on `(source_type, source_id, project_id)`.
- Add indexes on `release_id`, `status`, `risk`, and `updated_at`.

### 3. Release Item Link

One release can include many Change Records. A Change Record should normally belong to one release at a time, but the link table keeps future flexibility.

```go
type ReleaseItem struct {
    ReleaseID string
    ChangeID  string
    ItemType  string // epic | story | task | bug | remediation
    Priority  int
    Required  bool
    CreatedAt time.Time
}
```

Storage:

- Add `release_items`.
- Unique index on `(release_id, change_id)`.

### 4. Decision Events

Every proposal, review, approval, rejection, revision request, and done decision must be recorded.

```go
type DecisionEvent struct {
    ID              string
    ReleaseID       string
    ChangeID        string
    ProposalVersion int
    Actor           string
    ActorType       string // human | ai | verifier | policy | system
    Decision        string // proposed | reviewed | recommended_approval | approved | changes_requested | rejected | deferred | risk_accepted | marked_done
    Comment         string
    CreatedAt       time.Time
}
```

Storage:

- Add `decision_events`.
- Index by `release_id`, `change_id`, and `created_at`.

### 5. Review Authorities

Review authorities define what a human, AI, verifier, or policy check may do.

```go
type ReviewAuthority struct {
    ID          string
    Name        string
    Type        string // human | ai | verifier | policy
    Permissions []string // comment | request_changes | recommend_approval | approve_low_risk | approve | risk_accept | mark_done
    RiskLimit   string   // low | medium | high | critical
}
```

Storage:

- Add `review_authorities`.
- Seed defaults: `pm`, `developer`, `bugbot`, `tester_ai`, `security_ai`, `ci_verifier`.

### 6. Evidence And Communication

Evidence should be structured, not only free text.

```go
type Evidence struct {
    ID        string
    ChangeID  string
    Kind      string // test | ci | review | deploy | monitoring | manual
    Source    string // cli | github | jira | webhook | human
    Summary   string
    URL       string
    Raw       string
    CreatedAt time.Time
}
```

```go
type CommunicationArtifact struct {
    ID        string
    ReleaseID string
    ChangeID  string
    Audience  string // pm | tester | customer | support | developer
    Body      string
    PostedTo  string
    PostedAt  *time.Time
    CreatedAt time.Time
}
```

Storage:

- Add `evidence`.
- Add `communication_artifacts`.

## Phase 2: Status Machine And Validators

Create a central validator used by CLI, MCP, and UI.

Suggested package:

```text
internal/governance/validation
```

Initial release transitions:

```text
draft -> planned
planned -> approved
approved -> implementing
implementing -> ready_for_test
ready_for_test -> testing
testing -> ready_to_deploy
ready_to_deploy -> deployed
deployed -> closed
any active state -> blocked
blocked -> previous active state
draft/planned -> cancelled
```

Initial Change Record transitions:

```text
candidate -> planned
planned -> plan_ready
plan_ready -> changes_requested
changes_requested -> plan_ready
plan_ready -> approved_for_ai
approved_for_ai -> implementing
implementing -> pr_open
pr_open -> ready_for_test
ready_for_test -> done
candidate/planned/plan_ready -> rejected
candidate/planned/plan_ready -> deferred
```

Validation rules:

```text
Cannot approve a release with zero items.
Cannot approve a release item without acceptance criteria.
Cannot approve a release item without a verification plan.
Cannot approve when actor lacks authority for the risk tier.
Cannot implement work unless release status is approved or implementing.
Cannot implement work unless ChangeRecord.ApprovedVersion == ProposalVersion.
Cannot claim work already claimed by another active executor.
Cannot mark ready_for_test without PR or configured manual evidence.
Cannot mark done without verification evidence.
Cannot close customer-facing work without communication artifact.
Cannot add scope to approved release without invalidating release approval.
Cannot let the same actor plan, approve, implement, and mark done for medium/high/critical work.
```

Acceptance criteria:

- Validators have table-driven unit tests.
- Invalid transitions return actionable errors.
- CLI, MCP, and future UI use the same validator.

## Phase 3: Policy Engine

Add a small policy evaluator before building UI.

Suggested policy config:

```yaml
release_governance:
  enabled: true
  default_approval: human_required
  risk_tiers:
    low:
      ai_review_required: false
      ai_approval_allowed: true
      human_approval_required: false
    medium:
      ai_review_required: true
      ai_approval_allowed: false
      human_approval_required: true
    high:
      ai_review_required: true
      security_review_required: true
      ai_approval_allowed: false
      human_approval_required: true
    critical:
      ai_review_required: true
      security_review_required: true
      risk_acceptance_requires_human: true
```

Implementation tasks:

- Add config loading from `.openexec/openexec.yaml` or `.openexec/config.json`.
- Add `PolicyEvaluator.CanApprove(actor, change)`.
- Add `PolicyEvaluator.RequiredReviews(change)`.
- Add `PolicyEvaluator.CanAutoApprove(change)`.
- Add tests for low, medium, high, and critical work.

Acceptance criteria:

- Low-risk docs work can be AI-approved only when policy allows it.
- Medium/high work requires human approval.
- Security/SRE risk acceptance requires `risk_accept`.
- Policy failures explain the missing authority or evidence.

## Phase 4: CLI Workflow

Add commands that exercise the full flow without UI.

### Release Commands

```bash
openexec release create R-2026-07 --name "July customer fixes" --owner perttu
openexec release add R-2026-07 CHANGE-123 --priority 1 --required
openexec release plan R-2026-07
openexec release review R-2026-07 --reviewer bugbot
openexec release approve R-2026-07 --by perttu
openexec release start R-2026-07
openexec release status R-2026-07
openexec release handoff R-2026-07 --audience tester
openexec release deploy R-2026-07 --env staging --version 2026.07.1
openexec release close R-2026-07
```

### Work Item Commands

```bash
openexec work import-github --project unsorry --issue 123
openexec work attach CHANGE-123 --release R-2026-07
openexec work triage CHANGE-123
openexec work review-plan CHANGE-123 --reviewer bugbot
openexec work revise CHANGE-123 --by perttu --comment "Narrow scope to admin users"
openexec work revise-plan CHANGE-123
openexec work approve CHANGE-123 --by perttu
openexec work next --project unsorry
openexec work claim CHANGE-123 --agent codex
openexec work record-pr CHANGE-123 --url https://github.com/org/repo/pull/123
openexec work record-evidence CHANGE-123 --kind test --summary "go test ./..."
openexec work ready-for-test CHANGE-123
openexec work done CHANGE-123 --by tester
openexec work history CHANGE-123
```

Acceptance criteria:

- A developer can run the full vertical slice from CLI.
- `work next` returns only approved work from approved releases.
- Stale approvals are refused after plan revision.
- History shows every proposal, review, approval, and evidence event.

## Phase 5: GitHub Connector

Start with GitHub issues and PRs before Jira.

Implementation tasks:

- Add `internal/governance/connectors/github` or extend existing GitHub helpers if present.
- Use `gh` CLI first for speed and auth reuse.
- Import issue title, body, labels, comments, URL, and number.
- Create/update Change Records idempotently.
- Post OpenExec comments back to GitHub.
- Apply labels for governance state.

Initial labels:

```text
ai:triage
ai:plan-ready
ai:changes-requested
ai:approved
ai:implementing
ai:pr-open
ai:ready-for-test
ai:done
ai:blocked
ai:rejected
ai:deferred
```

GitHub comment commands:

```text
/openexec review
/openexec approve
/openexec revise <comment>
/openexec reject <reason>
/openexec defer <reason>
/openexec ready-for-test
```

Acceptance criteria:

- Import is idempotent.
- GitHub labels mirror OpenExec state.
- OpenExec never treats a label alone as sufficient approval unless policy allows it.
- PR body or comment links back to Change Record and Release.

## Phase 6: AI Planning And Review

Planner AI and reviewer AI must be separate actors in state, even if they use the same underlying provider.

Implementation tasks:

- Add `openexec work triage` prompt template.
- Add `openexec work review-plan` prompt template.
- Store planner output as proposal version `N`.
- Store reviewer output as `DecisionEvent`.
- Increment `ProposalVersion` on every material plan revision.
- Clear `ApprovedVersion` when proposal changes.

Planner output schema:

```yaml
summary: ""
kind: bug | feature | docs | ops | support_question | security | reliability
risk: low | medium | high | critical
affected_projects: []
affected_areas: []
acceptance_criteria: []
verification_plan: []
implementation_notes: ""
open_questions: []
recommended_release: ""
```

Reviewer output schema:

```yaml
decision: approve | request_changes | human_required
concerns: []
missing_acceptance_criteria: []
missing_tests: []
risk_comments: []
recommended_policy: ""
```

Acceptance criteria:

- Reviewer AI can request changes without having implementation authority.
- Reviewer AI can recommend approval without approving unless policy allows it.
- Plan revisions preserve old versions in history.
- The same actor cannot satisfy incompatible roles for non-trivial work.

## Phase 7: Executor Handoff

Expose approved work to Codex/Claude without copy-paste.

Implementation tasks:

- Add queue query: approved work, approved release, unclaimed, project matches.
- Add claim with lease expiry.
- Add claim release and blocked state.
- Generate executor brief with source issue, approved scope, acceptance criteria, verification plan, branch naming, and reporting requirements.

Suggested brief format:

```text
Change: CHANGE-123
Release: R-2026-07
Repo: /Users/perttu/projects/unsorry
Allowed scope: ...
Acceptance criteria:
- ...
Verification:
- ...
Required reporting:
- PR URL
- tests run
- known risks
```

MCP tools:

```text
openexec_list_releases
openexec_list_approved_work
openexec_get_work_brief
openexec_claim_work
openexec_record_plan
openexec_request_revision
openexec_record_pr
openexec_record_test_evidence
openexec_generate_handoff
openexec_request_done
```

Acceptance criteria:

- Codex/Claude can pick work through MCP or CLI.
- OpenExec refuses unapproved work.
- Claims expire or can be released.
- Dirty worktree and active-claim conflicts are surfaced before implementation.

## Phase 8: PR, CI, Test Evidence, And Handoff

Implementation tasks:

- Record PR URL, branch, source repo, and PR state.
- Record CI result manually first, webhook later.
- Record test evidence with command, result, summary, and raw output pointer.
- Generate tester handoff from Change Record plus evidence.
- Generate PM summary from release state.
- Generate customer-safe summary after deploy.

Tester handoff must include:

- release/version/environment,
- included changes,
- acceptance criteria,
- exact verification steps,
- known risks,
- links to PRs/issues,
- what is out of scope.

Acceptance criteria:

- `ready_for_test` requires PR/evidence unless policy has a manual exception.
- Handoff is generated from stored records, not only from AI claims.
- Release cannot close while customer/support communication is pending for customer-facing work.

## Phase 9: Minimal UI

Build UI only after CLI path works.

Screens:

- Release list: status, owner, risk, number of items, blockers.
- Release detail: included Change Records, approvals, PRs, evidence, communication.
- Approval queue: plan, reviewer comments, approve/revise/reject/defer buttons.
- Work queue: approved items available to agents.
- Tester view: ready-for-test releases and handoff notes.

Acceptance criteria:

- PM can see what is planned, approved, implementing, ready for test, and deployed.
- Developer can approve or request revision without CLI.
- Tester can see only relevant handoff information.
- UI actions call the same validation layer as CLI.

## Phase 10: Jira Connector

Add Jira after GitHub governance is stable.

Implementation tasks:

- Store Jira site, project key, issue key, status, assignee, priority, labels, fix version, and release link.
- Map Jira hierarchy: Epic -> Story -> Task -> Change Record.
- Support comments and status transitions.
- Treat Jira Release/FixVersion as source of truth when configured.
- Sync OpenExec Release status back to Jira only through validated transitions.

Acceptance criteria:

- Jira tasks cannot enter the implementation queue unless attached to an approved OpenExec release or approved Jira fix version.
- Jira comments show plan, approval, PR, evidence, and handoff.
- PM can approve/revise from Jira comment commands or UI.

## Phase 11: OpenSRE / Remediation Integration

SRE findings should enter the same governance model as remediation releases.

Implementation tasks:

- Import OpenSRE finding as `SourceType=opensre_finding`.
- Create remediation batch as release.
- Classify finding risk as low/medium/high/critical.
- Require human risk acceptance for deferred or accepted high/critical findings.
- Record monitoring, scan, or deploy evidence before closure.

Acceptance criteria:

- AI may triage findings without approval.
- AI may implement only approved remediation tasks.
- Production infra changes require explicit approval.
- A finding cannot be closed without evidence or risk-acceptance authority.

## Testing Strategy

Unit tests:

- SQLite migrations create all tables.
- Store create/update/list methods are idempotent.
- Status transitions accept valid moves and reject invalid moves.
- Policy evaluator enforces risk tiers.
- Approval invalidates on proposal revision.
- Review authority permissions are enforced.

CLI tests:

- Create release, import issue, attach work, triage, review, approve, next, claim, record PR, record evidence, ready-for-test.
- Verify stale approval is rejected.
- Verify unapproved release work is hidden from `work next`.

Integration tests:

- GitHub connector with mocked `gh` output.
- MCP tools use same validation path as CLI.
- UI actions call API endpoints that reuse validation.

Manual pilot:

```bash
openexec project link --repo agenticsnz/unsorry --path /Users/perttu/projects/unsorry --tracker github
openexec release create R-PILOT --name "Unsorry AI-governed pilot" --owner perttu
openexec github sync --project unsorry --label ai:triage
openexec work attach CHANGE-123 --release R-PILOT
openexec work triage CHANGE-123
openexec work review-plan CHANGE-123 --reviewer bugbot
openexec work approve CHANGE-123 --by perttu
openexec work next --project unsorry
```

## Suggested Developer Ticket Breakdown

1. Add governance schema migrations and store interfaces.
2. Add Change Record and Decision Event models.
3. Add Review Authority and policy evaluator.
4. Add transition validator.
5. Add release governance CLI commands.
6. Add GitHub issue import and label sync.
7. Add AI triage and AI review prompt commands.
8. Add approved work queue, claim, and executor brief.
9. Add PR and evidence recording.
10. Add tester handoff and PM summary generation.
11. Add MCP tools for approved work pickup and reporting.
12. Add minimal UI approval queue and release detail view.
13. Add Jira connector.
14. Add OpenSRE remediation source.

## Definition Of Done For First Release

- A release can be created and approved.
- GitHub issues can become Change Records.
- Change Records can be attached to a release.
- AI can draft and revise plans.
- Reviewer AI can comment or request changes.
- Human approval gates medium/high-risk work.
- Codex/Claude can retrieve only approved work.
- PR and test evidence are recorded.
- Tester handoff is generated.
- GitHub issue comments show status and next action.
- All critical transitions are validated in one shared validator.
