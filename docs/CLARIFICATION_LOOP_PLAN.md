# Clarification Loop — Plan (`ai:needs-clarification`)

## Objective

When the AI hits a decision it should not make on its own, it asks the human on
the work item, blocks, and resumes when answered — instead of guessing (unsafe)
or blocking on everything (useless). This is the async, GitHub-issue-native
version of the interactive `--clarify` interview that blueprint mode already has.

## The rule that makes it useful: calibrate on risk, not on uncertainty

Uncertainty alone must NOT block. Block only when the decision is risky or
irreversible. Otherwise proceed with a documented assumption the human can
correct at review.

| Situation | Action |
|-----------|--------|
| Low-risk, reversible, a sensible default exists | **Proceed with a documented assumption** — pick the default, state it explicitly in the plan/PR/commit; the human corrects it in review |
| High-risk / irreversible / legal / compliance / security / data-loss / no safe default | **Block** — post a specific question, label `needs-clarification`, do not proceed |

A documented assumption is visible and cheap to reverse (it's in the PR body). A
blocking question is reserved for decisions a machine should not make. Blocking
on everything makes the agent useless; guessing on the risky ones is an incident.

Worked example (the P0-13 footer, three review concerns):
- Placeholder privacy policy + newsletter email collection → **block** (GDPR).
- Missing TikTok/Instagram URLs → **proceed** with a marked placeholder, noted in
  the PR ("replace before launch").
- Facebook link removal → **proceed** on the literal reading, flag for confirm.

Only 1 of 3 blocks. That calibration is the product.

## Non-goals

- Replacing human judgment: the AI proposes *questions*; the human's answer is
  authoritative (same asymmetry as approvals — the agent may never self-answer).
- Blocking on ordinary uncertainty (see the rule above).
- A new lifecycle: reuse the existing `blocked` status with a clarification flavor.

## Existing primitives (this is mostly wiring)

- **States**: `ChangeStatusBlocked`, `ChangeStatusChangesRequested` already exist
  (`internal/governance/models.go`).
- **Write-back**: `github.PostComment` (issue comment), `SyncLabels` (create/apply
  labels), `ListIssueComments`, `ParseCommentCommand` (reads `/openexec …`
  replies) — all in `internal/governance/connectors/github/github.go`.
- **Audit**: `decision_events` records every transition (question posted, answer
  received, resumed).

## Where clarification arises (two points)

1. **Plan-time** (cleanest): the *plan* has an unknown; the reviewer or triage
   raises it. Block at review, comment on the issue. (The footer case.)
2. **Execution-time**: the agent mid-implementation hits an ambiguity. Prefer a
   documented assumption flagged in the PR; block only if the ambiguity is
   high-risk. Never let it guess-and-commit silently on a risky point.

## Question templates (source-aware)

- **Bug** (`github_issue`, defect): "What is the expected behavior?", "Can you
  give a reproduction / example input?", "Which environment/version?"
- **Enhancement / feature**: product decisions ("Is a placeholder X acceptable
  for the demo?", "Confirm removal of Y?", "Provide the real Z URL/content").
- Questions must be **specific and answerable** (yes/no + optional value), never
  "please clarify."

## Flow

```text
review/execute detects a BLOCKING unknown
  -> emit structured question(s)
  -> github.PostComment(issue, questions) + SyncLabels(+ai:needs-clarification)
  -> change status -> blocked; decision_event "clarification_requested"
  -> (human answers in an issue comment)
poll:
  -> ListIssueComments / ParseCommentCommand reads the answer
  -> decision_event "clarification_answered" (human = authoritative)
  -> AI ingests answer, re-plans/re-reviews
  -> status leaves blocked; label removed
```

For proceed-with-assumption cases there is no block: the assumption is written
into the plan/PR and recorded as a `decision_event` so review can catch it.

## Phases

### P1 — labels + blocked-for-clarification state
Add the `ai:needs-clarification` label to the governance label set; add a
clarification reason to `blocked` (or a `needs_clarification` sub-status). Emit
the `clarification_requested` decision_event. No AI yet — a manual
`work request-clarification <change> --questions ...` proves the write-back.

### P2 — structured questions from review/triage
Review/triage classifies each concern as **block** vs **assumption** per the risk
rule, emits blocking ones as structured questions, and posts them. Assumptions
are annotated into the plan, not posted.

### P3 — read the answer + resume
`poll` parses the human's reply, records `clarification_answered`, and re-runs the
blocked stage with the answer injected. Cap clarification rounds (e.g. 3) to avoid
ping-pong; stay blocked if unanswered (never silently proceed).

### P4 — execution-time clarification
The executor can raise a blocking question mid-task (high-risk ambiguity),
pausing the task rather than guessing; low-risk ambiguities become documented
assumptions surfaced in the PR.

## Guardrails

- Agent proposes questions; human answers are authoritative and audited.
- Cap rounds; escalate/stay-blocked on timeout — never auto-proceed.
- A blocking question must name the risk (why a human is needed), so reviewers can
  audit the calibration.

## Sequencing

Depends on the finalized GitHub connector (comment write-back + poll). Fits after
GitHub is done, alongside/after `GOVERNED_PR_CREATION_PLAN.md`; the same poll loop
that reads `/openexec` commands reads clarification answers.
