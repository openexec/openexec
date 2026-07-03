# Release Governance Plan

For the developer-facing build plan, see [Release Governance Implementation Plan](RELEASE_GOVERNANCE_IMPLEMENTATION_PLAN.md).

## Goal

Use releases as the control boundary for AI-assisted delivery.

Instead of letting any Jira task trigger implementation immediately, work should flow through a release plan owned by a PM or delivery lead. This makes it clear what is being built, what needs testing, what is included in a deployment, and what can be communicated to customers.

Core rule:

```text
AI may help triage and plan any work.
AI may implement only work assigned to an approved release.
```

## Why Releases Matter

Without a release boundary, AI delivery becomes too free-form:

- anyone can create a task,
- AI can implement isolated changes without product grouping,
- testers do not know what version to test,
- PMs cannot see what is actually shipping,
- customer communication becomes fragmented,
- deployment becomes a pile of unrelated PRs.

A release turns scattered tasks into a governed batch:

```text
customer feedback / ideas / bugs
-> epics and stories
-> candidate tasks
-> release selection by PM
-> approved implementation queue
-> PRs
-> testing
-> deployment
-> release notes and customer communication
```

## Jira Hierarchy

### Epic

An Epic describes the larger business outcome or customer problem.

It should include:

- owner,
- customer or business context,
- scope boundaries,
- success criteria,
- target release,
- linked stories.

AI may draft Epics and suggest decomposition, but a PM should approve the Epic before it becomes an active planning container.

### Story

A Story describes user-facing behavior or product value.

It should include:

- user-facing requirement,
- acceptance criteria,
- priority,
- risk level,
- testing expectations,
- linked tasks,
- target release.

AI may help refine Stories, detect ambiguity, and suggest acceptance criteria. A PM or product owner approves Stories for a release.

### Task

A Task is the technical implementation unit.

It should include:

- target repo/project,
- implementation plan,
- affected areas,
- verification plan,
- linked Story,
- linked Release,
- approval state,
- PR link when implementation begins.

AI may implement only approved Tasks that belong to an approved Release.

## Release Object

The release should be a first-class object in the governance layer.

Example shape:

```yaml
id: release-2026-07-01
name: "July customer fixes"
owner: "pm@example.com"
status: draft | planned | approved | implementing | ready_for_test | testing | ready_to_deploy | deployed | closed
scope:
  epics: []
  stories: []
  tasks: []
criteria:
  release_goal: ""
  must_have: []
  out_of_scope: []
  acceptance_summary: ""
risk:
  level: low | medium | high
  risky_areas: []
  approval_required: true
implementation:
  approved_for_ai: false
  active_tasks: []
  prs: []
testing:
  tester_owner: ""
  test_plan: ""
  environments: []
  known_risks: []
deployment:
  environment: ""
  deployed_at: ""
  version: ""
communication:
  pm_summary: ""
  tester_handoff: ""
  customer_release_note: ""
  support_note: ""
```

## Release Workflow

1. Intake.
   - Customer feedback, bug reports, ideas, and internal requests enter Jira.
   - AI classifies items and suggests Epic/Story/Task structure.

2. Planning.
   - PM reviews candidate Stories and Tasks.
   - PM assigns selected work to a draft Release.
   - AI identifies duplicates, dependencies, unclear items, and risky areas.

3. Release approval.
   - PM approves the Release scope.
   - Developer or technical lead approves implementation plans for Tasks.
   - The Release becomes `approved`.

4. AI implementation.
   - AI may pick only Tasks in approved Releases.
   - AI claims one Task at a time.
   - AI creates branch, implements, runs verification, and opens PR.
   - Each PR links back to Task, Story, and Release.

5. PR review and merge.
   - Low-risk pre-approved Tasks may auto-merge if policy allows.
   - Normal product Tasks wait for human review.
   - High-risk Tasks require explicit approval.

6. Release testing.
   - When all required PRs are merged or deployed to test, Release becomes `ready_for_test`.
   - AI generates tester handoff:
     - what changed,
     - which stories/tasks are included,
     - how to verify,
     - edge cases,
     - known risks.

7. Deployment.
   - Deployment event updates the Release.
   - AI generates "what is in this deployment" summary.
   - Release status moves to `deployed`.

8. Communication.
   - PM summary is generated.
   - Customer-safe release note is generated.
   - Support note is generated.
   - Jira stories/tasks are closed only after release criteria are met.

9. Feedback loop.
   - Tester or customer feedback attaches to the Release.
   - AI classifies feedback as regression, clarification, duplicate, or new work.
   - Follow-up items go into a future Release unless explicitly approved as hotfix.

## Review Authorities

Release governance should support multiple review authorities. The reviewer does not always need to be human, but the authority must be explicit.

Recommended authorities:

- Planner AI: drafts Epic, Story, Task, or remediation plans.
- Reviewer AI or bugbot: finds weak acceptance criteria, missing tests, risky scope, and inconsistent plans.
- Tester AI: checks whether verification steps are concrete enough for handoff.
- Security bot: reviews security, auth, infrastructure, and SRE remediation risk.
- PM or product owner: approves release scope and customer-facing priority.
- Developer or technical lead: approves implementation plan and technical risk.
- Verifier: records CI, test, deployment, or monitoring evidence.

Policy should separate recommendation from approval:

```text
AI reviewers may comment, request changes, and recommend approval.
AI reviewers may approve only risk tiers explicitly allowed by policy.
Human or designated risk authority approval is required for high-risk release scope,
security-sensitive work, production infrastructure changes, and risk acceptance.
```

For medium/high/critical work, one actor should not plan, approve, implement, and mark done. This keeps the process transparent even when some reviewers are AI agents.

## Policy Rules

Initial policy should be conservative:

```text
AI may not implement a Jira Task unless it belongs to an approved Release.
AI may not add scope to an approved Release without PM approval.
AI may not mark a Story done unless linked Tasks meet completion criteria.
AI may not mark a Release ready_for_test unless required PRs are merged or deployed to test.
AI may not close customer-facing work without release communication.
AI may not auto-merge high-risk work.
```

Hotfix exception:

```text
Critical bug fixes may enter a Hotfix Release with narrower approval,
but they still need a Release object, tester handoff, and communication record.
```

## Status Model

Recommended Release statuses:

```text
Draft
Planned
Approved
Implementing
Ready for Test
Testing
Ready to Deploy
Deployed
Closed
Blocked
Cancelled
```

Recommended Task statuses:

```text
Candidate
Needs Clarification
Ready for Release
Approved for AI
Implementing
PR Open
Merged
Ready for Test
Done
Blocked
```

## OpenExec Responsibilities

OpenExec governance should own:

- release object storage,
- Jira/GitHub release synchronization,
- task-to-release validation,
- approval state,
- implementation queue,
- PR linkage,
- CI/test evidence collection,
- tester handoff generation,
- release note generation,
- deployment event ingestion,
- stakeholder communication state.

Claude Code, Codex, or another execution engine should own:

- reading code,
- editing files,
- running commands,
- committing,
- opening PRs,
- responding to code review comments.

The boundary:

```text
Execution engines do code work.
OpenExec decides what work is allowed and records what happened.
```

## First Vertical Slice

Build the smallest release-governed workflow:

1. PM creates Release `R1`.
2. PM adds two Jira Tasks to `R1`.
3. AI triages each Task and writes implementation plans.
4. PM/developer approves the Release and both plans.
5. AI implements one approved Task.
6. AI opens a PR linked to Task, Story, and Release.
7. OpenExec records CI/test evidence.
8. OpenExec generates tester handoff for `R1`.
9. After merge/deploy, OpenExec generates release notes and customer-safe summary.

Success criteria:

- A task cannot be implemented until it is in an approved Release.
- PM can see what the Release contains.
- Tester can see what to verify.
- Developer does not copy Jira text into a console.
- PR and Jira updates are linked to the Release.
- Customer/support communication is generated from actual shipped scope.

## Relationship To Change Records

Change Records are still useful, but they should sit inside releases.

```text
Release
  -> Epic
      -> Story
          -> Task
              -> Change Record
                  -> PR / CI / deploy / communication evidence
```

For tiny projects, a Task and Change Record may be the same object. For company use, keeping Release as the planning boundary prevents ungoverned task execution.

## SRE And Security Remediation Flow

The same release-governance model should work for OpenSRE and similar systems that produce reliability, security, compliance, cost, or performance findings.

In this case the release may be called a remediation batch, maintenance window, or risk treatment plan.

Example flow:

```text
OpenSRE finding
-> remediation ticket
-> remediation batch / release
-> AI triage and remediation plan
-> human approval
-> implementation PR or infra/config change
-> verification evidence
-> deployment or mitigation record
-> finding closed, deferred, marked false-positive, or risk-accepted
```

Recommended hierarchy:

```text
Remediation Batch
  -> Finding
      -> Remediation Task
          -> Change Record
              -> PR / infra change / config change / verification evidence
```

SRE policy must be stricter than normal product work:

```text
AI may triage any finding.
AI may draft remediation plans.
AI may implement only approved remediation tasks.
AI may not apply production infrastructure changes without approval.
AI may not close a finding without verification evidence.
AI may not mark risk accepted without human sign-off.
```

Finding-backed Change Records should include:

```yaml
source:
  type: opensre_finding
  finding_id: ""
classification:
  kind: security | reliability | compliance | cost | performance
  risk: low | medium | high | critical
  verification_tier: approval
proposal:
  remediation_plan: ""
  affected_systems: []
  rollback_plan: ""
  verification_plan: []
approval:
  required: true
  approved_by: ""
execution:
  pr_url: ""
  deploy_event: ""
evidence:
  finding_before: ""
  finding_after: ""
  command_outputs: []
  ci_status: ""
communication:
  sre_summary: ""
  stakeholder_note: ""
status:
  state: open | planned | approved | remediating | remediated | risk_accepted | false_positive | deferred
```

The governance layer should be able to answer:

- which findings are waiting for approval,
- which remediations are included in the next batch,
- which findings have PRs open,
- what evidence proves a finding was fixed,
- which high-risk findings are deferred or risk-accepted,
- what can be reported to customers, auditors, or leadership.
