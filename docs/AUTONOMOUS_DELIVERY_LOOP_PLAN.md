# Autonomous Delivery Loop — Plan

## Objective

A scheduled, label-gated, single-slot loop that connects to GitHub on its own,
picks the next actionable job, drives it through governance to a PR, and — when a
job needs human input — parks it and moves to the next, returning once answered.
The human controls *what* gets worked (a label) and *what ships* (merge); the
loop does the rest unattended.

## Requirements (from the user)

1. **Control label** — only issues labeled e.g. `AI Fix` are eligible. Deny-by-
   default: no label, no autonomous work.
2. **Single slot** — one job in progress at a time.
3. **Clarification parking** — if the current job needs clarification, block it,
   post the question, and move to the next actionable job; return to the parked
   job when the human answers.
4. **No manual connect** — a cron job connects (syncs), pulls the next job, and
   runs it. No hand-driven invocation per step.

## What already exists (building blocks)

- **Label sync + gating**: `github sync --label <L>` imports only labeled issues;
  `SyncLabels`/`GovernanceLabels` manage `ai:*` labels.
- **Single-slot precedent**: the light-mode one-story-in-progress rule + change
  claim/lease (`work claim`).
- **Clarification write-back** (DONE): review → `changes_requested` on a GitHub
  change posts the concerns + label on the issue (`postGitHubClarification`).
- **Answer ingestion**: `github poll` → `IngestGitHubComments` parses `/openexec`
  commands (review/approve/reject/defer/revise/ready-for-test) and replies.
- **PR creation** (DONE): `work open-pr` pushes + opens a real PR.
- **Phases/scheduler**: `release.ComputePhase`, the batch scheduler that already
  holds back hitl tasks and their dependents.

The loop is mostly an orchestrator over these, plus a job-selection policy.

## Design

### Eligibility + the `AI Fix` gate
A change is **eligible** only if its source issue carries the control label
(default `AI Fix`, configurable). The gate is checked at sync/pickup, deny-by-
default. This is the human's scope control — exactly like the SRE allowlist is
the operator's control plane.

### Job states for the loop
- **Actionable**: eligible, not terminal (done/rejected/deferred), and not
  currently blocked-for-clarification or blocked-for-approval.
- **Parked**: blocked awaiting a human (clarification or approval).
- **In-slot**: the single change currently being worked (claim/lease held).

### Selection = "next actionable"
Pick the highest-priority actionable change that is **not** parked. Work it until
it either reaches a PR (done for the loop) or parks. A parked job becomes
actionable again when its blocking comment is answered (`poll` clears the
block); the loop then returns to it by priority/age. This gives the
"do first, park on clarification, move to next, come back" behavior for free —
it falls out of "always pick next actionable."

### The auto-approval decision (critical)
The AI reviewer cannot self-approve (`errNotOperator`). So full autonomy needs a
policy for who approves:
- **risk_profile low** → auto-approve in-loop (mirrors the SRE low-risk autonomy).
- **otherwise** → park for human approval; the human comments `/openexec approve`
  (poll ingests it) and the job resumes next tick.
Never auto-merge — the loop produces PRs; a human merges. Label-gate + single-slot
+ PR-not-merge is the safety envelope.

### The command
`openexec governance autopilot --label "AI Fix" --repo <owner/repo> --project <p>`:
1. **Connect**: sync labeled issues → change records.
2. **Select**: next actionable change (single slot; claim a lease).
3. **Drive**: triage → review → (park-on-clarification | proceed) → (auto-approve
   low-risk | park-for-approval) → execute → `open-pr`.
4. **Park or finish**, release the slot, and either loop to the next actionable
   job or exit (cron re-invokes).

Schedulable via cron (or the platform's CronCreate). One tick = one job advanced;
a job that parks doesn't stall the queue.

## Phases

### P1 — clarification write-back (DONE)
Review posts concerns + label to the issue when it blocks
(`postGitHubClarification`).

### P2 — answer ingestion resumes the job
Extend `poll`/`IngestGitHubComments`: a human reply on a `changes_requested`
issue records a `clarification_answered` decision_event and moves the change back
to a re-reviewable state (or `/openexec revise`). Cap rounds; stay parked if
unanswered.

### P3 — the `AI Fix` gate + single-slot selector
Config for the control label (default `AI Fix`); a selector that returns the next
actionable change honoring the label and the single in-flight lease.

### P4 — `autopilot` command
Wire connect → select → drive → park/finish into one command; parking moves to
the next actionable job. Add the low-risk auto-approve policy; everything else
parks for `/openexec approve`.

### P5 — schedule it
Run `autopilot` on a cron. Idempotent per tick (lease prevents double-pick);
observable (each tick logs what it advanced/parked).

## Safety envelope (must hold)

- **Deny-by-default**: no `AI Fix` label → never touched.
- **Single slot**: one lease; no concurrent self-collision.
- **Never auto-merge**: the loop opens PRs; humans merge.
- **No self-approve**: agents never approve; only low-risk auto-approves by
  policy, everything else parks for a human `/openexec approve`.
- **Parked ≠ proceed**: an unanswered clarification/approval never silently
  advances.

## Open decisions
1. Control-label name/config (`AI Fix` vs `ai:autopilot`) and whether it's per-
   project.
2. Auto-approve scope: low-risk only, or a per-project allowlist of change kinds.
3. PR granularity in-loop: task-level (see `GOVERNED_PR_CREATION_PLAN.md`) vs
   change-level to start.
4. Cron cadence + max jobs per tick.
5. Escalation for long-parked jobs (ping after N days?).
