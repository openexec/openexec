# OpenExec Simple Loop Architecture Contract

Status: owner-mandated architectural constraint, 2026-09-14. This operational
restatement preserves the owner's contract; it is not proof of implementation
or permission to expand effects, resources or repository scope.

## Architecture

There is one implementation loop, one outer Goal-convergence loop, and a
deterministic delivery boundary. Reuse OpenExec. Additional complexity carries
the burden of concrete empirical proof.

OpenExec's productive task-oriented loop is the baseline, not the accumulated
Console controller architecture. Its historical weakness was final product
convergence, not inability to execute. The outer loop must improve convergence
without replacing task execution.

### Task convergence — OpenExec

Intent/current state → native task generation → independent task-plan review →
repair incomplete plans → highest-priority runnable native task → implement →
verify → complete, or create higher-priority native repair tasks → execute
repairs → resume affected work → continue the original queue.

Ordinary implementation, test, build, integration, prerequisite and review
failures mean more work exists. They must not automatically become Goal failure,
owner decisions, generations, recovery routes or autonomous termination. Runs
are attempts; native tasks remain the durable truth of unfinished work.

### Goal convergence — Agent Console

At a stable native task boundary, independently inspect the actual resulting
product against accepted Goal/Ready. Do not infer Ready from task completion.
Material remaining gaps feed existing OpenExec task generation and plan review,
then the SAME implementation loop. No Console implementation backlog or
Goal-recovery executor. Goal/Ready describes the destination, not per-run task
selection, retry or repair policy.

Greenfield and brownfield share this architecture. Brownfield assessment must
identify existing behavior and plan only missing/change work; it must not rebuild
what exists. Planning and Goal review occur at meaningful boundaries, not after
every model run.

### Small-task escape hatch

Focused request → relevant current-state inspection → existing compact planning
→ preferably one vertical task (normally 1–3 tasks) → implement → verify → done.
No mandatory project-scale architecture study or full Goal-convergence ceremony.
Escalate only when execution reveals broader scope or material uncertainty.

### Deterministic delivery

Completed exact candidate → validate Goal/repository/candidate provenance →
required checks → independent review where judgment is required → applicable
approval/effect gate → publication/deployment → verify running revision.

Do not dispatch a general-purpose delivery worker to rediscover known commit,
review-request, publication, promotion or deployment transitions. Agents reason
where judgment is necessary; software performs known transitions. Exact-head
review, Stop, effect controls and accounting remain mandatory.

## Ownership and continuation

OpenExec owns planning, plan review, native tasks, dependencies, priorities,
implementation, verification, repairs, retries and task continuation/convergence.

Console owns Intent/Story, Goal/Ready, explicit outcome selection, independent
Goal review, outcome lifecycle, owner-facing progress, scoped Stop (and explicit
Stop-All only if required), external/effect authority, evidence and portfolio
awareness. Deterministic software owns known delivery mechanics.

Normal continuation: observe explicit Stop and genuine hard authority/resource
boundaries; otherwise execute the highest-priority runnable native task; at a
stable queue boundary perform Goal review. Do not reconstruct generations,
conversations, old runs/reservations or Navigator routes to establish work.

Historical budgets, generations, runs, conversations, Stops, commitments, routes
and reservations may remain audit/accounting/evidence/diagnostics. They must not
become authority over unrelated explicitly accepted Goals. Preserve restrictive
authority, historical accounting, security and candidate provenance.

Owner attention is for material destination changes, genuine product judgment,
external irreversible effects, hard resource boundaries, contradictory outcomes
and Stop/Resume. It is not for next tasks, ordinary retries/repairs, sequencing,
continuation, stale evidence refresh, routine replanning or soft budget renewal.
Target: zero avoidable execution-coordination decisions after Goal acceptance.

## Required engineering practice

Owner brevity is not architectural permission. Experts have implicit knowledge;
retrieve durable context instead of making the owner reconstruct it. Reuse must
be verified in code, not assumed. LLMs tend to add state and recovery machinery;
counter that bias by asking whether the task loop, Goal review, deterministic
software, or deletion already solves the failure.

Before changing architecture, explicitly restate:

1. The user outcome.
2. Current observed state.
3. Which loop or deterministic boundary owns the problem.
4. The existing OpenExec primitive to reuse.
5. Why no new abstraction is needed.

Before any new persistent architecture concept, answer: can this be a native
task; can Goal review handle it; can deterministic software handle it; can an
existing concept be simplified/deleted; what observed failure remains impossible
without the addition? Without a concrete answer to the last question, do not add
it. Proposals for another grant, generation, task type, backlog, continuation,
recovery route, controller state, delivery worker or authority layer require an
architecture challenge BEFORE implementation.

Each architecture change must report:

```
concepts added:
concepts removed:
new persistent state:
new transitions:
new owner decisions:
new failure modes:
existing machinery replaced:
```

Require a negative or empirically justified complexity delta. Tests and many
callers do not justify an unnecessary mechanism. Classify existing mechanisms
KEEP (genuine Console responsibility), DEMOTE (history/accounting/diagnostics),
MIGRATE (native OpenExec responsibility), or DELETE (unnecessary).

Reproduce the real product failure; trace existing primitives; make the smallest
reuse/deletion/boundary change; test useful work; generalize only after repeated
evidence. Reject reduced unattended productivity unless a demonstrated hard
safety requirement necessitates it. Never call extra governance coverage a
substitute for actual useful product advancement.

## Acceptance

Prove brownfield assessment → reviewed native plan → task A fails verification →
priority native repair → repair executes → A resumes → remaining tasks execute →
queue converges → independent product review finds a material gap → another
reviewed native plan → SAME loop → fresh review concludes Ready, without routine
owner coordination. Separately prove the compact path without project-scale
orchestration. Then run real brownfield work and measure useful implementation,
failures survived, repair/resume, plan/review quality, remaining gaps, owner
interventions and final quality against the earlier productive-loop baseline.

Success means useful software continues toward reasonably Ready without the
owner becoming scheduler, task manager or recovery operator. It does not mean
more controller states, modes, governance or tests.
