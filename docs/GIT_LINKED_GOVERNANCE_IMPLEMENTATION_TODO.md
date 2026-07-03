# Git-Linked Governance Implementation TODO

## Objective

Implement the first OpenExec governance vertical slice:

```text
GitHub issue
-> Change Record
-> AI triage plan
-> human approval
-> approved work handed to Claude Code/Codex
-> PR and test evidence recorded
-> issue updated with tester handoff and PM summary
```

Do not build a new coding agent. Use Claude Code or Codex as the executor. OpenExec owns governance, validation, state, and communication.

## Non-Goals

- Do not implement full Jira support in the first slice.
- Do not build a full PM dashboard yet.
- Do not auto-merge product work by default.
- Do not implement release planning before basic GitHub issue governance works.
- Do not require Anthropic/OpenAI API integration; CLI/MCP usage is enough.

## Phase 1: Data Model

### 1. Add Project Link Model

Create a persistent project record.

Fields:

```go
type ProjectLink struct {
    ID            string
    Name          string
    Repo          string
    LocalPath     string
    Tracker       string // github | jira
    DefaultBranch string
    CreatedAt     time.Time
    UpdatedAt     time.Time
}
```

Acceptance criteria:

- A project can be linked from CLI.
- A linked project can be listed.
- The record persists across process restart.

Suggested CLI:

```bash
openexec project link --repo agenticsnz/unsorry --path /Users/perttu/projects/unsorry --tracker github
openexec projects list
```

### 2. Add Change Record Model

Create a persistent Change Record.

Required fields:

```go
type ChangeRecord struct {
    ID                 string
    ProjectID          string
    SourceType         string // github_issue | jira | manual | opensre_finding
    SourceURL          string
    SourceID           string
    Title              string
    RawText            string
    Summary            string
    Kind               string // bug | feature | docs | ops | support_question
    Risk               string // low | medium | high | critical
    VerificationTier   string // verified | scored | consensus | approval
    SuggestedMode      string // chat | inspect | fix | task | run | release | sre
    Status             string
    Approved           bool
    ApprovedBy         string
    ProposalVersion    int
    ApprovedVersion    int
    Plan               string
    AcceptanceCriteria string
    VerificationPlan   string
    Branch             string
    PRURL              string
    TestEvidence       string
    TesterNotes        string
    PMSummary          string
    CreatedAt          time.Time
    UpdatedAt          time.Time
}
```

Acceptance criteria:

- Change Records can be created from a GitHub issue.
- Change Records can be updated idempotently.
- Status transitions are validated.
- Approval is tied to a specific proposal version.

### 3. Add Decision History Model

Record every human and AI decision affecting approval.

Fields:

```go
type DecisionEvent struct {
    ID              string
    ChangeID        string
    ProposalVersion int
    Actor           string
    ActorType       string // human | ai | system
    Decision        string // proposed | approved | changes_requested | rejected | deferred | cancelled
    Comment         string
    CreatedAt       time.Time
}
```

Acceptance criteria:

- Every AI proposal creates a `proposed` event.
- Every approval, requested change, rejection, and deferral is stored.
- The decision history can be printed from CLI.
- Approval is invalidated when a newer proposal version is created.

### 4. Add Review Authority Model

Reviewers may be humans, AI reviewers, deterministic verifiers, or policy checks. Each authority has explicit permissions.

Fields:

```go
type ReviewAuthority struct {
    ID          string
    Name        string
    Type        string // human | ai | verifier | policy
    Permissions []string // comment | request_changes | recommend_approval | approve_low_risk | approve | risk_accept | mark_done
}
```

Acceptance criteria:

- A DecisionEvent records which authority made the decision.
- AI reviewers can comment and request changes.
- AI reviewers cannot approve high-risk work unless policy explicitly allows it.
- Risk acceptance always requires an authority with `risk_accept`.
- `mark_done` requires a distinct permission and validation evidence.

## Phase 2: GitHub Issue Sync

### 5. Add GitHub Issue Import

Read issues from a linked GitHub repo.

Suggested command:

```bash
openexec github sync --project unsorry --label ai:triage
```

Acceptance criteria:

- Imports matching open issues.
- Creates Change Records for new issues.
- Updates existing Change Records when issue title/body changes.
- Stores issue URL and number.
- Does not duplicate records.

Implementation note:

- Use `gh issue list` / `gh issue view` initially.
- Avoid GitHub API implementation unless needed.

### 6. Add Governance Labels

Initial labels:

```text
ai:triage
ai:needs-clarification
ai:plan-ready
ai:changes-requested
ai:approved
ai:implementing
ai:pr-open
ai:ready-for-test
ai:blocked
ai:done
ai:rejected
ai:deferred
```

Acceptance criteria:

- OpenExec can detect approval via `ai:approved`.
- OpenExec refuses implementation if approval is missing.
- OpenExec can update labels after state transitions.

## Phase 3: Triage Plan

### 7. Add AI Triage Prompt Command

Generate a triage plan for a Change Record.

Suggested command:

```bash
openexec work triage CHANGE-123
```

Output:

- summary,
- kind,
- risk,
- suggested mode,
- affected areas,
- acceptance criteria,
- verification plan,
- approval requirement.

Acceptance criteria:

- Stores the triage result in the Change Record.
- Increments `ProposalVersion`.
- Sets `Approved=false` and clears `ApprovedVersion` when a new proposal is created.
- Posts the plan as a GitHub issue comment.
- Does not implement code.

Implementation note:

- The first version may call Claude Code/Codex externally through a prompt template, or simply prepare a prompt for the user to run.
- Later, expose this through MCP.

## Phase 4: Human Iteration Loop

### 8. Add Revision Commands

The human must be able to approve, request changes, reject, defer, or cancel.

Suggested commands:

```bash
openexec work approve CHANGE-123 --by perttu
openexec work revise CHANGE-123 --by perttu --comment "Limit this to admins only"
openexec work reject CHANGE-123 --by perttu --reason "Out of scope"
openexec work defer CHANGE-123 --by perttu --reason "Move to next release"
openexec work history CHANGE-123
```

GitHub/Jira comment commands:

```text
/openexec approve
/openexec revise Limit this to admins only
/openexec reject Out of scope
/openexec defer Move to next release
/openexec request-tests Add regression coverage for expired sessions
```

Acceptance criteria:

- `revise` records a `changes_requested` DecisionEvent.
- `reject` moves the Change Record to terminal `rejected`.
- `defer` moves the Change Record to `deferred`.
- `approve` sets `Approved=true` and `ApprovedVersion=ProposalVersion`.
- `approve` is refused if no proposal exists.
- `approve` is refused if the proposal is missing acceptance criteria or verification plan.

### 9. Add Proposal Revision

AI can revise a proposal after human feedback.

Suggested command:

```bash
openexec work revise-plan CHANGE-123
```

Acceptance criteria:

- Reads the latest human revision comment.
- Produces a new plan, acceptance criteria, risk, and verification plan.
- Increments `ProposalVersion`.
- Clears approval.
- Posts the revised plan back to GitHub/Jira.
- Keeps all prior versions visible in history.

### 10. Add Approval Detection

Approval can be one of:

- GitHub label `ai:approved`,
- issue comment containing `/openexec approve`,
- local CLI command.

Suggested command:

```bash
openexec work approve CHANGE-123 --by perttu
```

Acceptance criteria:

- Approval updates Change Record.
- Approval updates GitHub labels/comments.
- Approval requires a triage plan to exist.
- Approval applies to the current `ProposalVersion`.
- If a newer proposal is created, prior approval no longer permits implementation.
- Approval is refused when the actor lacks permission for the change risk level.

## Phase 5: AI Review Authorities

### 11. Add Plan Review Step

Allow one or more review authorities to review the AI-generated plan before approval.

Suggested command:

```bash
openexec work review-plan CHANGE-123 --reviewer bugbot
```

Output:

- concerns,
- missing acceptance criteria,
- missing tests,
- affected-risk comments,
- recommendation: approve | request_changes | human_required.

Acceptance criteria:

- Review output is stored as a DecisionEvent.
- Reviewer AI may request changes.
- Reviewer AI may recommend approval.
- Reviewer AI may approve only if policy permits AI approval for the risk tier.
- Plan approval can require one or more reviewer recommendations.

### 12. Add Separation-Of-Duties Validation

Implement policy checks so a single actor cannot perform incompatible roles for non-trivial work.

Initial rule:

```text
For medium/high/critical risk work, the same actor may not be planner, approver, implementor, and done marker.
```

Acceptance criteria:

- Low-risk docs work may allow AI planner + AI reviewer approval if policy allows.
- Medium-risk work requires separate planner and approver.
- High/critical work requires human approval.
- SRE risk acceptance requires human or explicitly configured risk authority.

Suggested policy shape:

```yaml
policies:
  docs_low_risk:
    plan_reviewers: [bugbot]
    approval: ai_allowed
    implementation: any_agent
    done_requires: [test_evidence]

  product_feature:
    plan_reviewers: [bugbot, tester_ai]
    approval: human_required
    implementation: any_agent
    done_requires: [pr_merged, tester_handoff]

  security_change:
    plan_reviewers: [bugbot, security_ai]
    approval: human_required
    implementation: restricted_agent
    done_requires: [ci_passed, security_review, human_signoff]
```

## Phase 6: Executor Handoff

### 13. Add Approved Work Query

Provide the next approved item to Claude Code/Codex.

Suggested command:

```bash
openexec work next --project unsorry
openexec work brief CHANGE-123
```

Acceptance criteria:

- Returns only approved work.
- Returns only work where `ApprovedVersion == ProposalVersion`.
- Skips records already claimed or implementing.
- Includes source link, scope, acceptance criteria, and verification plan.

### 14. Add Claim / Write Lock

Suggested command:

```bash
openexec work claim CHANGE-123 --agent claude-code
```

Acceptance criteria:

- A Change Record can have only one active claim.
- A project can have only one active write claim in the first version.
- Claim has TTL or can be released manually.
- Claim is refused when approval is stale.

### 15. Add MCP Tool Surface

Expose the same operations as MCP tools for Claude Code/Codex.

Initial tools:

```text
openexec_list_projects
openexec_list_approved_work
openexec_get_work_brief
openexec_claim_work
openexec_record_plan
openexec_review_plan
openexec_request_revision
openexec_record_revision
openexec_record_pr
openexec_record_test_evidence
openexec_generate_handoff
openexec_request_done
```

Acceptance criteria:

- Claude Code/Codex can retrieve approved work without copy-paste.
- Claude Code/Codex can report PR and evidence back through OpenExec.
- Claude Code/Codex can request clarification or record blocked state without bypassing approval.
- Reviewer agents can comment or request changes without gaining implementation authority.

## Phase 7: PR And Evidence Recording

### 16. Add PR Recording

Suggested command:

```bash
openexec work record-pr CHANGE-123 --pr https://github.com/agenticsnz/unsorry/pull/123
```

Acceptance criteria:

- Stores PR URL and branch.
- Updates status to `pr_open`.
- Posts issue comment linking PR.
- Adds `ai:pr-open` label.

### 17. Add Test Evidence Recording

Suggested command:

```bash
openexec work evidence CHANGE-123 --tests "go test ./..." --result passed
```

Acceptance criteria:

- Stores command, result, and summary.
- Supports failed evidence.
- Refuses `ready_for_test` without evidence.

### 18. Generate Tester Handoff And PM Summary

Suggested command:

```bash
openexec work handoff CHANGE-123
```

Output:

- what changed,
- why it changed,
- linked issue/PR,
- verification performed,
- what tester should verify,
- known risks,
- PM summary.

Acceptance criteria:

- Writes tester notes and PM summary to the Change Record.
- Posts a GitHub issue comment.
- Updates status to `ready_for_test` only when PR and evidence exist.

### 19. Add PR Review Iteration

Human review comments can send implementation back for revision.

Suggested commands:

```bash
openexec work request-implementation-changes CHANGE-123 --by reviewer --comment "Avoid touching auth middleware"
openexec work record-implementation-revision CHANGE-123 --summary "Moved logic into billing route"
```

Acceptance criteria:

- Review changes requested moves status from `pr_open` to `review_changes_requested`.
- AI can revise implementation without creating a new approved plan only if the requested change stays inside the approved scope.
- If review changes alter scope, OpenExec requires a new proposal version and re-approval.
- The PR comment/update links to the DecisionEvent.

## Phase 8: Validation Rules

Implement central transition validation.

Rules:

```text
Cannot triage without source issue or manual source.
Cannot approve without plan.
Cannot approve without acceptance criteria and verification plan.
Cannot approve when actor lacks authority for risk level.
Cannot implement unless ApprovedVersion == ProposalVersion.
Cannot claim unless approved.
Cannot claim if project write lock is active.
Cannot mark pr_open without PR URL.
Cannot mark ready_for_test without PR and test evidence.
Cannot mark done without ready_for_test or configured exception.
Cannot close source issue without handoff summary.
Cannot revise a rejected/cancelled item without reopening it.
Cannot change approved scope without invalidating approval.
Cannot allow the same actor to plan, approve, implement, and mark done for medium/high/critical work.
```

Acceptance criteria:

- Validation rules are unit-tested.
- Invalid transitions return actionable errors.
- MCP and CLI both use the same validation layer.

## Phase 9: Missing Gaps To Address Before Pilot

These are easy to miss and should be deliberately handled before any autonomous run.

1. Identity and permissions.
   - Decide which GitHub/Jira identity posts comments and labels.
   - Record the human approver identity.
   - Avoid letting the AI approve its own work.
   - Distinguish planner, reviewer, implementor, approver, verifier, and done-marker identities.

2. Idempotency.
   - Re-running sync must not duplicate Change Records or comments.
   - Commands should be safe to retry after partial failure.

3. Source-of-truth conflict.
   - Define whether GitHub labels, local DB, or Jira status wins on conflict.
   - For the first slice, local Change Record plus source issue URL should be authoritative for governance state; GitHub labels are projections/input signals.

4. Audit trail.
   - Store all decisions, proposals, approvals, rejected transitions, and evidence.
   - Include timestamps and actor identity.

5. Stale claims.
   - Claims need TTL, release command, and stale-claim recovery.

6. Dirty worktrees.
   - Before implementation, detect uncommitted changes in the linked project.
   - Refuse or require explicit approval before editing a dirty checkout.

7. Branch and PR policy.
   - Define branch naming.
   - Require Change Record ID in PR title or body.
   - Prevent one PR from claiming multiple unrelated Change Records in the first version.

8. Security boundaries.
   - Deny writes outside the linked project path.
   - Treat workflow, token, branch-protection, and verifier changes as high risk.

9. Evidence quality.
   - Handoff text must cite actual PR/test evidence.
   - Do not allow "tests passed" without recorded command or CI status.

10. Rate limits and failure modes.
    - Handle `gh` failures, auth failures, and API rate limits.
    - Leave the Change Record in a recoverable blocked state.

11. Notification noise.
    - Avoid posting a new comment for every retry.
    - Prefer updating a sticky OpenExec comment when possible.

12. Release/Jira gap.
    - The first GitHub slice can work without releases.
    - Jira/release enforcement must be added before company-wide use.

## Phase 10: Unsorry Pilot

Use `unsorry` as the first test repository.

Suggested safe issue types:

- docs updates,
- generated-board/report fixes,
- ADR proposal drafts,
- issue triage,
- PR evidence summaries,
- small non-trust-bearing workflow comments.

Avoid in first pilot:

- Gate A soundness changes,
- branch protection changes,
- token/secret workflows,
- auto-merge policy changes,
- verifier trust paths,
- mass refactors.

Pilot steps:

1. Link project:

   ```bash
   openexec project link --repo agenticsnz/unsorry --path /Users/perttu/projects/unsorry --tracker github
   ```

2. Create or select GitHub issue with `ai:triage`.
3. Sync issues:

   ```bash
   openexec github sync --project unsorry --label ai:triage
   ```

4. Generate triage plan:

   ```bash
   openexec work triage CHANGE-123
   ```

5. Run an AI review once:

   ```bash
   openexec work review-plan CHANGE-123 --reviewer bugbot
   ```

6. Request a revision once:

   ```bash
   openexec work revise CHANGE-123 --by perttu --comment "Narrow this to docs only"
   openexec work revise-plan CHANGE-123
   ```

7. Approve the revised proposal:

   ```bash
   openexec work approve CHANGE-123 --by perttu
   ```

8. In Claude Code or Codex:

   ```text
   Use OpenExec. Pick the next approved work item for unsorry,
   implement it, open a PR, and record evidence.
   ```

9. Record PR/evidence/handoff.
10. Review whether the issue, PR, and Change Record contain enough information for PM/tester review.

## Review Checklist

Before merging the first implementation, verify:

- GitHub issue import is idempotent.
- Approval is required before implementation.
- Approval is versioned and stale approvals are rejected.
- Human revision comments can produce a new proposal version.
- AI review authorities can request changes without approval power unless policy allows it.
- Separation-of-duties rules prevent one actor from controlling the whole lifecycle for non-trivial work.
- Claude/Codex can get work without copy-paste.
- PRs link back to Change Records.
- Status transitions are validated.
- Tester handoff is generated from stored evidence, not only AI claims.
- No high-risk workflow paths are enabled by default.
- All state can be inspected from CLI.
