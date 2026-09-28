# Autonomous execution and human boundaries — implementation status

Status: incomplete implementation, 2026-09-27. This record does not establish
end-to-end acceptance, effect authority, or deployment readiness.

The owner assigns durable accepted Goal/Ready, execution policy, Goal review,
recovery and human decision scheduling to OpenExec. Console presents engine
state and forwards authenticated commands. The old Console ownership in the
Simple Loop contract is superseded; its implementation still needs migration.

## Implemented increments

- Full and compact planning, review and refinement share one human-boundary
  rule. Routine planning, automated QA and repair do not inherently need humans.
- New reviewed HITL tasks require a concrete reason. Legacy retained HITL tasks
  do not become automatic when reason/reference metadata is absent.
- Surgical compaction leaves boundary/dependency-bearing tasks separate.
  Preparation remains runnable. Refinement preserves the connected graph around
  retained human boundaries while allowing unrelated work to change.
- Both plan import paths and native verification repair retain decision reason
  and reference metadata. A reference is not proof of approval or authority.
- The native queue returns a typed per-task boundary after exhausting eligible
  work. Human wait, attempt limit, failed work, review and dependencies remain
  distinct. Waiting uses existing pending/HITL ledger state and consumes no
  attempts. Ordinary error logs contain counts, not decision descriptions.

## Verification evidence

The first increment passed OpenExec `make test`, `make compat-test` and
`make type-check`, plus Console `make check` for the paired documentation update.
The persisted runner was `autonomous-human-boundaries-check`, three successful
stages. These gates precede the later typed queue-boundary change. After that change,
`go test ./internal/planner ./internal/release ./pkg/manager` passed again in
the provider sandbox with a writable temporary Go cache.

Focused controlled-provider integration tests exercise both persisted import
paths, reviewed-plan replay, scheduling of independent work, real native pipeline
completion before waiting, and two subsequent queue entries after reopening the
database. Held work retains zero attempts and the task count does not grow.
Mixed legacy-HITL, failed, exhausted and review-held tasks remain distinguishable.
These are deterministic tests with simulated providers, not live model evidence.

Physical guard-removal controls fail for compaction, required reason, refinement,
repair metadata, each import path, and typed queue waiting. Source is restored
after each control.

## Still required

1. Bind durable requests to accepted Goal revisions and exact proposed actions;
   reuse approval storage without conflating preference, access and effect
   authorization. Metadata/reference preservation alone does not implement this.
2. Add an authenticated operator answer path that validates stale/duplicate
   submissions and atomically resumes the same work without completing it.
   Existing unauthenticated HTTP approval handlers cannot serve this boundary
   as-is. Existing operator-only MCP approval checks are a reuse candidate.
3. Transfer the outer Goal loop, accepted execution contract, resource/Stop
   policy and restart recovery from Console to one engine owner. Preserve
   in-flight work and exact candidate identity during transfer.
4. Wire Console request presentation and authenticated submission to that engine
   state, including reconnect behavior. No Console runtime/UI migration is
   included yet.
5. Exercise the complete requested journey, including failed verification and
   repair, a genuine decision across restart, answer/resume, stale/unauthorized
   refusals, resource exhaustion, Stop and Console disconnection.

No schema migration is required for these increments: optional decision strings
use native task metadata. No existing HITL task is reconciled automatically.

Complexity delta so far: no new persistent entity, queue, controller, lifecycle
transition or owner decision. Two optional metadata fields and a typed queue
result replace lost decision context and an undifferentiated blocked-queue error.
The existing native planner, task ledger, scheduler and repair loop remain owners.

## Delivery limitation

Changes remain in isolated `feat/autonomous-human-boundaries` worktrees in both
repositories. The owner accepted Goal and Ready on 2026-09-27. The persisted
accepted Goal is `df2737bb913bd7de2d97242044f33117`, also used as `feature_key`
by the current candidate API. The conversation's active outcome points to it;
the Goal remains planned, with no candidate or execution run. No duplicate
outcome or second acceptance is needed.

The acceptance handler only dispatched read-only proposals. This proposal came
from an independent workspace-write run, so neither a continuation operation nor
an execution run was created. A local Console fix queues both supported proposal
modes atomically and invokes existing admission checks. Focused tests cover
restart, repeated answers, the same outcome identity and visible admission
failure. Physical removal of each fix reproduces failure. The full Console gate
must be repeated for this later runtime change. This code is not deployed.
An idempotent replay of the retained answer can enter the fixed handler without
creating another outcome; no currently exposed recovery tool performs that replay.

The live supported candidate route refuses this task-kind session with
“unattended task sessions cannot operate on the owner's registered feature
checkout.” Its handoff action refuses with “handoff requires the invoking
writable conversation.” These are capability gaps despite workspace-write mode
and an accepted active outcome. Do not edit live state, change session kind,
choose another Goal or use generic Git commands to bypass these checks.
No commit, PR, dependency publication or deployment has been performed.
