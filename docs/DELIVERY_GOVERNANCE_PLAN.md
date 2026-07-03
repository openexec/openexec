# Delivery Governance Plan

## Goal

OpenExec should not compete primarily as another coding harness. The stronger product is a delivery-governance layer that connects customer feedback, Jira/GitHub work items, AI implementation, PRs, testing, deployment, and stakeholder communication.

The core problem:

```text
AI can make code faster than teams can triage, review, test, deploy, and communicate it.
```

The product goal:

```text
Move work from idea or customer feedback to approved plan, PR, tester handoff,
deployment status, and customer-safe communication without losing context.
```

## Layering Decision

Separate OpenExec into two layers.

### Layer 1: Open Runtime

This can stay open source.

- Local execution runtime.
- Provider adapters for Codex, Claude Code, Gemini, and OpenAI-compatible APIs.
- Workspace discovery.
- MCP/server primitives.
- Basic policies, gates, logs, and evidence schema.
- Repo-as-OS primitives inspired by unsorry: work units, state, gates, claims, and reconcilers.

Layer 1 answers:

```text
How does an AI agent safely inspect, change, verify, and report work?
```

### Layer 2: Delivery Governance

This should be treated as the higher-value product layer and does not need to be open sourced early.

- Jira/GitHub/Slack/customer-feedback intake.
- Change records.
- Risk classification and approval routing.
- AI task picking from approved queues.
- PM/tester/customer summaries.
- Deployment communication.
- Multi-project portfolio dashboard.
- Governance policy: what can auto-merge, what needs review, what needs approval.

Layer 2 answers:

```text
What work should happen, who approved it, what changed, who needs to know, and is it done?
```

## Core Object: Change Record

Every meaningful unit of work should produce or attach to a Change Record.

Example shape:

```yaml
id: change-2026-06-29-001
source:
  type: jira | github_issue | slack | customer_feedback | chat | manual
  url: ""
request:
  raw_text: ""
  summary: ""
classification:
  kind: bug | feature | support_question | refactor | docs | ops
  risk: low | medium | high
  verification_tier: verified | scored | consensus | approval
  suggested_mode: chat | inspect | fix | task | run | release | sre
proposal:
  affected_projects: []
  affected_areas: []
  acceptance_criteria: []
  verification_plan: []
approval:
  required: true
  status: pending | approved | rejected | needs_clarification
  approved_by: ""
execution:
  branch: ""
  pr_url: ""
  commits: []
  tests_run: []
  ci_status: ""
communication:
  pm_summary: ""
  tester_notes: ""
  customer_note: ""
  deployment_note: ""
status:
  state: intake | planned | approved | implementing | review | ready_for_test | deployed | closed
```

This object is the bridge between Jira, GitHub, AI runs, PRs, CI, deployment, and human communication.

## Product Workflow

1. Intake.
   - Read Jira issues, GitHub issues, Slack/customer messages, or manual chat requests.
   - Classify each item as bug, feature, support question, unclear request, or duplicate.
   - Create or update a Change Record.

2. Triage.
   - Inspect code and related history.
   - Propose affected projects/files.
   - Draft acceptance criteria.
   - Decide suggested mode: `chat`, `fix`, `task`, `run`, `release`, or `sre`.
   - Decide whether approval is required.

3. Human approval.
   - Developer/PM reviews the proposed interpretation, scope, risk, and verification plan.
   - Human can approve, request changes, reject, defer, or ask for clarification.
   - AI revises the proposal until the human approves, rejects, defers, or cancels it.
   - Approval applies only to the approved proposal version; any scope-changing revision invalidates approval.
   - Approved items enter the agent work queue.

4. Implementation.
   - OpenExec assigns approved items to an execution engine.
   - The engine may be Codex, Claude Code, Gemini, or another provider.
   - The work produces a branch, PR, test evidence, and implementation summary.

5. Review and merge.
   - `VERIFIED` deterministic work can auto-merge when policy allows.
   - Low-risk pre-approved fixes can auto-merge when tests pass.
   - Normal product features require human review or explicit merge approval.
   - High-risk items always require approval.

6. Testing handoff.
   - Generate tester notes from the Change Record and diff.
   - Include environment/version, changed behavior, test steps, edge cases, and known risks.
   - Update Jira/GitHub status to ready for test.

7. Deployment and communication.
   - Listen to deployment events.
   - Update the Change Record with deployed version/environment.
   - Generate PM summary, customer-safe release note, and support response.
   - Track whether stakeholders were informed.

8. Feedback loop.
   - Tester/customer feedback attaches to the original Change Record.
   - AI classifies whether it is a regression, clarification, duplicate, or new task.
   - Follow-up work is created without losing context.

## Review Authorities And Separation Of Duties

Approval should be modeled as a policy decision by a review authority, not only as a human button click. A review authority can be a PM, developer, tester, reviewer AI, bugbot, security bot, deterministic verifier, or policy rule.

Each authority should have explicit permissions:

```text
comment
request_changes
recommend_approval
approve_low_risk
approve
risk_accept
mark_done
```

This allows OpenExec to replace human review with AI review where it is safe, while still keeping high-risk decisions governed. For example, a bugbot may review a plan and request changes, but it should not approve security-sensitive work unless policy explicitly grants that authority.

Initial policy tiers:

- Low-risk docs or copy changes: AI reviewer may approve if tests/evidence are sufficient.
- Normal product features: AI reviewer may recommend approval, but a PM or developer approves.
- Security, auth, payments, infra, and SRE risk acceptance: AI/security review can comment, but a human or designated risk authority approves.

Separation-of-duties rule:

```text
For medium/high/critical work, the same actor should not plan, approve,
implement, and mark done.
```

## OpenExec Implementation Plan

### P0: Product Boundary

1. Decide which parts are open.
   - Keep low-level runtime open.
   - Keep governance UX, routing policy, and integration workflows private until validated.

2. Rename the product concepts.
   - Avoid positioning as only a coding harness.
   - Use terms like Change Record, Delivery Item, Governance Queue, Tester Handoff, and Stakeholder Update.

3. Update docs to state the two-layer architecture.
   - Runtime executes.
   - Governance coordinates.

### P1: Change Record Foundation

4. Add a Change Record schema.
   - Store locally first in SQLite.
   - Support links to GitHub issues, PRs, Jira issues, Slack threads, and deployment events.

5. Add event ingestion model.
   - `issue.created`
   - `issue.updated`
   - `pr.opened`
   - `pr.reviewed`
   - `ci.completed`
   - `deploy.completed`
   - `comment.added`

6. Add status state machine.
   - `intake -> triaged -> approved -> implementing -> pr_open -> ready_for_test -> deployed -> closed`
   - Include blocked and needs-clarification states.

### P2: GitHub-First Vertical Slice

7. Start with GitHub before Jira.
   - GitHub issues are simpler and fit the repo-as-OS model.
   - Map issue labels to classification and approval state.

8. Build issue-to-change-record sync.
   - Read open issues.
   - Create/update Change Records.
   - Preserve source links and comments.

9. Build approval labels.
   - `ai:triage`
   - `ai:needs-approval`
   - `ai:approved`
   - `ai:implementing`
   - `ai:ready-for-test`
   - `ai:blocked`

10. Build PR linking.
    - PR body includes Change Record ID.
    - Change Record stores PR URL, branch, commits, CI status, and test evidence.

### P3: AI Triage

11. Add AI triage command.
    - Input: issue/customer text plus repo context.
    - Output: classification, risk, affected areas, acceptance criteria, and suggested mode.

12. Add duplicate/related search.
    - Search open/closed issues.
    - Search recent PRs and commits.
    - Search docs and code references.

13. Add human-editable plan.
    - Store AI proposal separately from approved plan.
    - Do not implement until approved.

### P4: Approved Task Pickup

14. Add queue picker.
    - Select approved work only.
    - Respect priority, risk, project, and write locks.
    - Avoid manually copying tasks into a terminal.

15. Add implementation trigger.
    - `openexec work next`
    - `openexec work run <change-id>`
    - UI button: Approve and Implement.

16. Add provider-neutral execution.
    - Use Codex/Claude as engines.
    - Do not rebuild agent capability unless governance requires it.

### P5: Communication Outputs

17. Generate role-specific summaries.
    - Developer summary.
    - PM status.
    - Tester handoff.
    - Customer-safe note.
    - Release note.

18. Post updates back to source systems.
    - GitHub issue comments.
    - PR comments.
    - Jira comments/status later.
    - Slack summaries later.

19. Add deployment awareness.
    - Accept webhook or manual deploy event.
    - Link deployed version/environment to Change Records.
    - Generate "what is in this deployment" summaries.

### P6: Jira And Slack

20. Add Jira connector.
    - Read issues, comments, status, assignee, priority, and labels.
    - Write comments and transition status.
    - Preserve Jira as source of truth when used.

21. Add Slack/customer-message intake.
    - Start with manual paste/import.
    - Later add Slack app/webhook.
    - Convert conversations into candidate Change Records.

22. Add support-question mode.
    - Not every customer message becomes a code task.
    - AI can answer "is this expected behavior?" by inspecting code/docs/history.

### P7: Governance Dashboard

23. Build dashboard around Change Records, not agent runs.
    - Intake queue.
    - Awaiting approval.
    - Implementing.
    - PR open.
    - Ready for test.
    - Deployed.
    - Customer update pending.

24. Add multi-project portfolio view.
    - Project health.
    - Active changes.
    - Blockers.
    - Unreviewed PRs.
    - Ready-for-test items.
    - Deployments.

25. Add manager/tester views.
    - PM sees status and blockers.
    - Tester sees what to verify.
    - Developer sees approved work and code evidence.

## Relationship To Unsorry

Unsorry proves the Git-as-OS pattern:

- repository as canonical state,
- work units as files/issues/branches,
- reconcilers as workflows/tools,
- gates as trust boundaries,
- auto-merge only where verifier strength allows it,
- generated visibility artifacts.

OpenExec should generalize this, but not copy the Lean trust model into product software. Product software usually needs `APPROVAL` tier governance because tests are incomplete and customer intent matters.

The useful rule:

```text
Auto-merge only when the verifier is strong enough for the risk.
Otherwise automate everything up to the approval boundary.
```

## Why This May Be Separate From OpenExec

This governance layer may deserve a separate product/package because it has a different buyer and value proposition.

OpenExec runtime:

- buyer/user: developer, agent operator,
- value: safe execution,
- interface: CLI, local server, MCP, workspace tools.

Delivery governance:

- buyer/user: CTO, PM, delivery lead, support lead, tester,
- value: visibility, approvals, communication, reduced coordination latency,
- interface: dashboard, Jira/GitHub/Slack integrations, status reports.

Recommended path:

1. Keep OpenExec as the open runtime.
2. Build governance as a separate layer that uses OpenExec.
3. Avoid open-sourcing the full governance workflow until validated.
4. Use `unsorry` as the public proof that repo-as-OS works.
5. Keep product-specific delivery coordination private while exploring market fit.

## First Vertical Slice

The smallest useful version:

```text
GitHub issue
-> AI triage
-> Change Record
-> human approves plan
-> AI implements with Codex or Claude
-> PR opens
-> tests/CI attach evidence
-> tester notes and PM summary are posted
```

This does not require building a better coding agent. It requires connecting existing coding agents to company workflow and communication.

Success criteria:

- Developer does not copy issue text into a console.
- PM can see what changed and what is blocked.
- Tester gets clear verification steps.
- Customer/support gets accurate update text.
- PR links back to the original request and evidence.
- Human approval boundary is explicit.
