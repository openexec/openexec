# OpenExec and OpenExec Pro: technical and product review

**Status:** historical record — point-in-time review, 2026-07-10. True on its date; do not read it as a current work list.

**Review date:** 2026-07-10  
**Repositories reviewed:** `openexec` and sibling `openexec-pro`  
**Review type:** source inspection, flow tracing, local build/test verification, documentation comparison, repository-history inspection, and public-project signal check

## Executive conclusion

OpenExec has a worthwhile core idea: make agentic software work repeatable, inspectable, provider-neutral, and governed by deterministic stages instead of treating a chat transcript as the delivery process. There is a real implementation behind the idea, not merely a wrapper or mock-up. Its strongest potential position is an **execution-policy and evidence layer around existing coding agents**.

It is not yet a dependable factory-grade runtime. The current release has data races in normal orchestration, quality gates that can report success without running lint or tests, security boundaries that are narrower than the documentation implies, dormant features exposed as configuration, and considerable repository/release debris. These are trust problems, not cosmetic polish.

OpenExec Pro's governance thesis is also valid. Proposal-version-pinned approval, risk-tiered review, PR provenance, externally stored audit state, a keyed decision chain, operator allowlists, and deny-by-default issue intake are useful controls. The execution-first contract is a much better user experience than the deprecated multi-step planning lane.

However, Pro's main `work run` flow bypasses OpenExec's execution engine and launches a coding CLI directly with broad permissions. It asks the agent to verify its own work, reviews only up to 80 KB of diff, opens a non-draft PR without requiring CI, and lets a human-authorized merge bypass the live-green-CI requirement. It also has concrete dirty-worktree and resume hazards. As built, it is a promising local governance prototype, not yet a safe autonomous delivery connector.

The best direction is not more features. It is one narrow, trustworthy product loop:

> labeled issue -> explicit contract -> isolated execution -> deterministic verification -> complete diff review -> draft PR -> live CI evidence -> human merge -> signed run receipt

Build that loop on one shared execution primitive, prove it on ordinary software repositories, and then pursue industrial customers through non-production engineering workflows. Do not position it as a system that directly controls production machinery.

## How the review was performed

This review did not rely on README claims alone. I traced the CLI entry points, manager/pipeline wiring, blueprint mutation, runners, governance service, SQLite stores, approval and merge gates, GitHub/Jira connectors, UI routes, tests, Makefiles, CI configuration, release scripts, Git history, tracked artifacts, and the current working-tree state.

Checks run against OpenExec:

- `go test -count=1 ./...` — passed.
- `go vet ./...` — passed.
- `GOPROXY=off go build ./...` with an isolated cache — passed.
- UI unit tests, single-worker — 40 files and 635 tests passed; React `act` and style warnings remain.
- UI lint and TypeScript checks — passed.
- `make compat-test` with an isolated Go cache — passed for current `.openexec`, legacy `.uaos`, and `.openexec/tasks.json` fallback behavior.
- Targeted `go test -race` over pipeline/manager/API — **failed with data races**.
- Playwright discovered 70 end-to-end cases; browser execution was not run in this review.

Checks run against OpenExec Pro:

- `go build ./...` — passed.
- `go vet ./...` — passed.
- `go test -count=1 ./...` — passed when run with localhost binding available. The first sandboxed attempt failed only because a Jira `httptest` server could not bind a local port.
- `go test -race -count=1 ./...` — passed.
- The primary execution-first functions have no direct tests: searches found no tests for `executeContractDirect`, `executorCommand`, `resolveExecutorCLI`, the full `work run` path, branch parking/resume, the 80 KB diff boundary, or dirty-worktree behavior.

The Pro checkout already contained unrelated modifications and untracked files before this review. They were preserved. No Pro source was changed.

---

# Part I — OpenExec open-source core

## What it actually is

OpenExec is a Go CLI/daemon with a React monitoring UI. Its normal execution model is:

1. `openexec init` creates project state under `.openexec`.
2. A story/task backlog is persisted in SQLite.
3. The manager selects work and sends it through the pipeline.
4. A blueprint advances through context, implementation, lint, test, and review stages.
5. A provider runner calls Claude, Codex, Gemini, or an OpenAI-compatible API.
6. State, events, evidence, and audit information are persisted.
7. The daemon API and UI expose status and control; MCP provides a lighter integration surface.

This is a coherent architecture for policy-controlled agent execution. The reusable plugin, MCP, runner, project, runtime, database, and agent packages also give the commercial connector legitimate extension seams.

## What is strong

### A real execution engine

The project contains substantial orchestration, persistence, migrations, compatibility handling, agent/provider integration, a web UI, and tests. It is not a thin prompt launcher.

### Deterministic workflow structure

Blueprint stages are the right conceptual primitive. They can make “done” mean something stronger than an agent saying it is done, provided the stages are immutable per run and real checks are mandatory.

### Provider neutrality

Supporting multiple coding CLIs and API providers is strategically sound. The durable product should govern execution regardless of which agent is fashionable this quarter.

### Compatibility discipline

The current project, legacy `.uaos`, and task JSON fallback paths have explicit compatibility coverage. The repository's anti-regression policy is appropriate and should remain.

### Useful extension seams

Public packages and registration hooks are sufficient for a sibling commercial module to compose a combined binary without importing core `internal` packages. Pro proves that this split can compile.

## Critical correctness findings

### 1. Normal orchestration is not race-safe

The targeted race suite found two categories of concurrent mutation:

- Run/event state written in `pkg/manager/events.go` while `Manager.Status()` reads it.
- Shared global blueprint state selected in `internal/pipeline/pipeline.go` and then mutated per run: timeout, commands, and review routing are changed on a shared blueprint pointer.

This can leak configuration between simultaneous runs and makes outcomes timing-dependent. An unmerged upstream fix proposal existed as [PR #11](https://github.com/openexec/openexec/pull/11), but the reviewed checkout still races.

**Required fix:** deep-clone a blueprint before any per-run mutation; protect manager run state with a lock or immutable snapshot; add a race-enabled concurrent-run regression test to CI.

### 2. The standard quality gates can be false-green

The default blueprint has lint and test stages, but their command lists can be empty. Empty commands auto-pass unless `.openexec/config.json` supplies concrete commands. The repository's root `openexec.yaml` enables lint but does not provide the custom command this path expects. A separate composite `run_gates` action exists in the manager, but the standard blueprint does not invoke it.

Therefore the common-looking “implement -> lint -> test -> review -> complete” flow can complete without actually running lint or tests.

**Required fix:** make required gates fail closed when no executable command resolves; define one canonical gate resolver; print the exact resolved commands in the contract and receipt; test the default generated project end to end.

### 3. Several exposed capabilities are not wired into the normal run

Configuration and status surfaces advertise or save quality-gates-v2, cache, predictive load, memory, checkpoints, and coordinator/worker concepts. The manager's ordinary pipeline construction does not consistently instantiate or use those components.

This increases cognitive load and makes operators believe protections are active when they are not.

**Required fix:** either wire and test each capability or mark it experimental and hide it from generated/default configuration. Delete unused breadth before adding more.

## Security findings

### 4. Security controls are real but not global

Some specialized infrastructure and MCP paths validate commands and paths. That does not make the whole runtime sandboxed.

- The general API runner accepts absolute paths, can write files, and executes a model-produced command using `sh -c`.
- Claude workspace execution adds `--dangerously-skip-permissions`, while broad built-in file/edit/shell abilities remain available.
- The daemon listens on `:%d`, which normally binds all interfaces, and the inspected API path has no general authentication middleware.

The documentation's broad security language overstates the effective boundary. A process with shell capability is not safely confined just because one connector validates its own inputs.

**Required fix:** establish a single threat model and one enforceable boundary. Default the daemon to loopback with an explicit remote-listen option and authentication. Run agents in an OS/container sandbox with scoped mounts, network policy, resource limits, and scrubbed credentials. Never describe prompt instructions or post-run diff checks as isolation.

## UI and operational findings

### 5. The UI is broad but contains unfinished and stale paths

The unit suite is large and passes, but notable gaps remain:

- Replay is a placeholder in `ui/src/App.tsx`.
- Blueprint approval can form `/api/api/v1/...` from the default `/api` base.
- Knowledge Hub links to a policies route that does not exist.
- Symbols and environment navigation target a DCP route rather than dedicated views.
- Frontend stage types omit backend stages.
- Many browser tests only prove that the root rendered non-empty content rather than exercising a meaningful workflow.

CI builds the UI but does not run its unit or browser tests. `golangci-lint` is advisory through `continue-on-error`.

**Direction:** reduce the UI to run status, live logs, evidence, approval, and failure diagnosis until those paths are excellent. Put UI unit tests in required CI and add one true issue-to-receipt browser smoke test.

## Documentation and repository cleanup

### 6. Configuration documentation is not a reliable contract

`docs/CONFIGURATION.md` is much broader than the configuration the current loader actually consumes. Legacy names remain, two local documentation links are broken, and security claims need qualification.

Generate reference documentation from the schema or config structs and validate all example configs in CI. Separate “available now,” “experimental,” and “planned.”

### 7. The repository contains a large amount of generated and stale material

At review time the checkout was about 3.3 GB and its Git pack about 1.88 GiB. Tracked content included 11 binaries, a compiled mock, 469 `.gocache` files, Go cache locks, artifacts, and suspicious generated data such as `data/fwus.jso`. Source reports v0.11 while tracked binaries report v0.10. Docker Compose and release scripts refer to old sibling repositories. A large portion of Git history uses the placeholder identity `Your Name`.

The public repository API reported roughly 2.46 GB, which makes this an onboarding, clone-time, trust, and maintenance problem.

**Required cleanup:** remove tracked binaries/caches/artifacts with a history rewrite if acceptable; publish binaries only as release assets; add artifact verification; repair release automation to build from a clean tag; delete or archive obsolete multi-repo scripts; add repository-size and generated-file checks.

### 8. The installer/release path needs supply-chain work

The installer fetched on the review date still declared version 0.10 and downloaded a raw binary without checksum or signature verification, while the source and latest release described v0.11.

**Required fix:** signed, checksummed release manifests; installer version derived from the release tag; clean reproducible release job; no checked-in binaries.

## Open-source traction and positioning

The project is very young: the public repository was created in February 2026 and, at review time, had single-digit-to-low-double-digit community signals, few outside contributions, and almost no release-asset downloads. That is evidence of an early project, not evidence that the idea is invalid. It does mean product-market fit and external operability have not been demonstrated.

Avoid presenting repository breadth as maturity. A smaller system that produces trustworthy evidence on five real repositories would be a stronger open-source story.

## Does OpenExec have a place in the factory world?

Yes, with a precise definition of “factory.”

### Strong fit

- Software factories and internal developer platforms.
- Controlled changes to industrial software repositories.
- PLC/robot configuration generation that is validated in simulation or a digital twin.
- Test generation, static analysis, documentation, traceability, and evidence packaging.
- Air-gapped or on-prem engineering assistants with strict approval and audit requirements.
- Changes that flow into an existing, certified deployment process.

### Unsafe or unconvincing fit today

- Direct autonomous writes to PLCs, robots, SCADA, safety systems, or production controllers.
- A generic claim that the runtime secures arbitrary shell-capable agents.
- Treating an AI review or hash chain as proof that a change is safe.
- Replacing a plant's existing functional-safety, change-control, validation, or deployment systems.

Industrial environments prioritize safety, availability, segmentation, recovery, and deterministic operational control. That is consistent with [NIST SP 800-82 Rev. 3](https://csrc.nist.gov/pubs/sp/800/82/r3/final) and the [ISA/IEC 62443 series](https://www.isa.org/standards-and-publications/isa-standards/isa-iec-62443-series-of-standards). AI governance should also map to the [NIST AI Risk Management Framework](https://www.nist.gov/itl/ai-risk-management-framework).

The credible industrial architecture is:

> offline engineering change -> isolated execution -> simulation/test bench -> signed evidence -> named human approval -> existing certified deployment path

Major industrial vendors are moving toward industrial copilots, so the category is plausible; for example, [Siemens publicly expanded its industrial AI/copilot portfolio in 2026](https://press.siemens.com/global/en/pressrelease/siemens-unveils-technologies-accelerate-industrial-ai-revolution-ces-2026). OpenExec's defensible role would be the vendor-neutral execution and evidence layer, not another copilot UI.

---

# Part II — OpenExec Pro

## What Pro actually adds

Pro is a separate Go module that builds a combined OpenExec binary. It registers governance CLI commands and an MCP provider through public core seams. Its governance layer adds:

- release and change-record lifecycle;
- GitHub and Jira ingestion;
- AI-assisted triage and execution contracts;
- risk policies and review authorities;
- approval, risk-acceptance, and merge gates;
- proposal-version-pinned decision events;
- PR provenance checks;
- a SQLite audit store outside the project workspace by default;
- a keyed HMAC decision chain with export/verification;
- an operator allowlist tied to the current `gh` identity;
- relay/autopilot commands and backpressure.

The module boundary is technically sensible. Core remains MIT and self-contained; Pro composes it without importing core internals.

## The primary user flow, as implemented

The default `governance work run <change>` path is:

1. Load an imported change.
2. Require an operator session unless the contract is already approved/implementing.
3. Optionally ask clarification questions and persist answers.
4. Give repository context to an AI and build a one-screen contract containing scope, acceptance criteria, allowed paths, and file/line budget.
5. Ask `Approve this contract and execute? [Y/n]`; Enter means yes. Relay uses `--yes`.
6. Record approval at the current proposal version and verify the keyed approval chain.
7. Delete any existing `gov/<change-id>` branch, create a fresh branch from base, and launch the configured coding CLI directly.
8. Stage everything with `git add -A` and commit it.
9. Measure files, changed lines, and out-of-contract paths after execution.
10. Retry once if only out-of-scope noise was produced. If real work exceeded budget, expand the contract to measured reality, raise risk to high, invalidate approval, and seek fresh approval or park.
11. Send up to 80,000 bytes of diff to `bugbot`; high/critical work also goes to `security_ai`. Run a bounded repair round if requested.
12. Push the branch, open a normal PR, record it, and best-effort post an impact/operability assessment.
13. A later `work merge` verifies provenance, authority, critical-risk acceptance, and required clean AI diff-review events, then calls GitHub merge.

This is easy to understand and much better than asking a user to drive a dozen governance subcommands. The contract, measured budget, bounded repair, and PR handoff are good interaction ideas.

## Does the new bucketing solve the old “everything runs the full flow” problem?

**Partly. It reduces the planning overhead, but it does not yet reliably route different tasks into materially different execution flows.**

Previously, even trivial work could enter the full planning/decomposition/review lifecycle. The new execution-first lane is a real improvement because nearly all non-epic work skips that decomposition and uses a concise contract instead. That should make small, well-described changes faster and less prone to planning-loop divergence.

However, the current default router is effectively binary:

| Classified size/risk | What `work run` actually does |
|---|---|
| `surgical` | contract -> approval -> direct agent run -> budget check -> diff review -> PR |
| `small` | the same flow |
| `standard` | the same flow |
| `epic` | stop before execution and propose sub-issues |
| high/critical risk | the same size flow, with an additional review authority |

`BuildContract` explicitly treats epic as the only sizing decision that changes the pre-execution route. Everything below epic becomes one contract and one direct executor run. The surgical/small/standard label is not passed into `ComputeContract`, so it does not deterministically select the contract budget, executor effort, verification depth, or repair policy. The contract model independently estimates file and line limits.

The new system therefore changes the default from **“always do full planning”** to approximately **“always do the contract flow unless it looks epic.”** That is simpler and probably faster, but it is not yet the task-sensitive routing capability the product needs.

### Why the current classifier will still misroute work

1. **It sizes from ticket text without repository context.** `ClassifyIntent` receives the title and body only, even though its prompt asks it to estimate files, modules, layers, and dependencies. Repository excerpts reach contract generation later, after the routing decision.
2. **It is intentionally biased toward under-sizing.** The prompt says to choose the smaller bucket when uncertain. The measured-diff gate can detect this after execution, but that is recovery from a bad route—not correct routing before spending agent time.
3. **Size is the wrong primary question.** A five-line bug with an unknown cause may need diagnosis, while a 100-line additive feature with a clear shape may be safe to execute directly. Magnitude alone cannot distinguish them.
4. **There is no confidence or ambiguity route.** Valid YAML is treated as a routing decision; there is no calibrated confidence, deterministic evidence requirement, or “inspect first” fallback.
5. **Tests prove plumbing, not classification quality.** Existing tests feed canned classifier responses and verify that the selected code branch runs. They do not evaluate real tickets against human-labeled expected routes or measure false-routing cost.

### Current scenario assessment

| Task | Expected behavior today | Assessment |
|---|---|---|
| Typo, copy edit, isolated config change | full contract flow | Faster than old planning, but still heavier than necessary |
| Small bug with a known cause and location | contract flow | Good fit; likely substantially better |
| Symptom-only regression (“used to work; now returns 500”) | probably contract flow | Weak: it may authorize an assumed fix before establishing root cause |
| Cross-module but coherent feature | contract flow, then possible budget escalation | Can work, but routing errors are discovered late |
| Greenfield subsystem or architectural redesign | contract unless called epic | Weak: unknown design shape is not the same as known separable pieces |
| Several clearly independent deliverables | epic split if the classifier notices them | Meaningful improvement; stops before expensive execution |

### The planned `approach` axis is the correct direction—but is not implemented

`openexec-pro/docs/plans/task-classification.md` accurately identifies the missing distinction and proposes routing by **what is unknown**:

| Unknown | Correct route |
|---|---|
| Nothing important; change is known and contained | `direct` -> surgical/small contract execution |
| Root cause of a reported defect | `diagnose` -> bounded evidence-gathering -> reclassify the minimal fix |
| Shape/architecture of the requested system | `design` -> deep planner or explicit design gate |
| Boundaries are known and contain multiple shippable pieces | `epic` -> split into tracked sub-issues |

The current `Classification` type contains only kind, risk, size, and size rationale. The proposed `approach: direct | diagnose | design`, its grounding stage, and its routing branches have not been implemented. This should be treated as a primary product gap, not merely future optimization.

### Recommended router

```text
ticket intent + repository evidence
               |
               v
        What is still unknown?
          |
          +-- nothing important ----> DIRECT
          |                            scoped contract + execution
          |
          +-- root cause ------------> DIAGNOSE
          |                            bounded investigation, then route fix
          |
          +-- system shape ----------> DESIGN
          |                            deep planning / decisions before execution
          |
          +-- many known pieces -----> SPLIT
                                       tracked sub-issues; no execution yet
```

Risk must remain orthogonal: it controls authority, review, evidence, and merge posture, while approach controls the shape of work. A surgical authentication fix can still be high risk; a larger documentation migration can be low risk but unsuitable for one contract.

### Concrete implementation recommendations

1. **Ground routing in the repository.** Pass a small, deterministic repository summary and relevant excerpts into classification. Record exactly what evidence supported the route.
2. **Implement `approach`.** Add `direct | diagnose | design`, an operator override, audit fields, and fail-safe parsing. Wire it to real routes instead of initially leaving it as a label indefinitely.
3. **Build one bounded diagnosis primitive.** It should produce root cause, evidence locations/reproduction, the minimal fix, affected surfaces, and a new size estimate. Reclassify that fix; never let “bug investigation” silently expand into redesign.
4. **Route design work before execution.** Greenfield and architecture-changing requests should reach the deep planner or a design-decision gate, not be flattened into a small-change contract.
5. **Keep epic for known separable work.** Epic means the pieces are already understandable and independently shippable. File/link the proposed child issues rather than only printing them.
6. **Make direct sub-buckets operationally meaningful.** Surgical/small/standard should select explicit limits such as executor effort/time, file/line caps, required deterministic checks, review coverage, and repair allowance. Do not let the size label be audit decoration.
7. **Add confidence and an inspect-first fallback.** Ambiguous routing should spend a cheap, read-only grounding pass instead of guessing smaller or immediately invoking the full agent.
8. **Preserve empirical budget feedback.** Measured diff size remains valuable, but use it to calibrate future routing per repository—not only to escalate the current run.
9. **Create a routing evaluation set.** Label 30–50 completed tickets with the route an experienced maintainer would choose. Include trivial edits, known bugs, symptom-only bugs, coherent features, greenfield design, security-sensitive one-liners, and true epics.
10. **Measure routing outcomes, not parser success.** Track route agreement, human overrides, false-direct rate, false-epic rate, time spent before reroute, budget escalations, successful recovery, and total time-to-verified-PR by bucket.

The key acceptance criterion for this feature should be:

> On representative repository-grounded work, OpenExec selects the same flow an experienced maintainer would choose often enough to reduce unnecessary planning/execution time without increasing unsafe direct execution.

Until that is measured, the honest product statement is: **the new lane is lighter than the old full flow, but reliable task-dependent routing remains under construction.**

**Implementation update (2026-07-11):** the task-sensitive router described below is now implemented in the OpenExec Pro working tree and awaiting independent review. `work run` classifies against bounded repository evidence, routes `direct`, performs one evidence-backed `diagnose` pass before reclassification, sends `design` to deep planning without starting the executor, and maps direct+epic to `split`. Malformed/un-grounded decisions fail closed, overrides are audited, design planning preserves the routing risk floor, and a committed 20-case fixture scores the former size-only baseline at 12/20 approach agreement. Real-ticket calibration remains outstanding; implementation tests do not yet prove production model accuracy.

## Implementation handoff: task-sensitive routing

This section is the implementation contract for the next contributor. It is intentionally narrower than the full product direction above so the resulting changes can be reviewed and tested independently.

### Objective

Change `governance work run` from an epic-versus-contract router into a repository-grounded router that distinguishes:

- **direct:** the requested change and its boundaries are sufficiently known to authorize implementation;
- **diagnose:** a symptom is known but the root cause/minimal fix is not;
- **design:** the system shape or material design decisions are not known;
- **split:** multiple known, independently shippable pieces should become separate work items.

The implementation is complete only when those decisions produce observably different command behavior and are covered by tests. Merely adding an `approach` field to model output is not completion.

### Non-goals for this change

- Do not redesign approval, audit, merge, or risk policy.
- Do not add another general planning framework.
- Do not solve executor isolation, deterministic verification, or CI gating in this change; those remain separate critical work.
- Do not remove the explicit `--size` escape hatch.
- Do not use task kind as the route by itself. A known one-line bug can be direct; a vague bug report can require diagnosis.
- Do not let model classification weaken risk floors or review requirements.
- Do not add hosted services, queues, or multi-tenant state.

### Required behavior

| Route | Required `work run` behavior |
|---|---|
| `direct` + surgical/small/standard | Build and show an execution contract, then continue through the existing approval/execution lane |
| `direct` + epic | Return `split`, propose tracked sub-issues, and perform no execution |
| `diagnose` | Run a bounded, read-only diagnosis; require repository evidence; classify the resulting minimal fix; then route that fix rather than the original symptom |
| `design` | Do not execute a flat contract; route to the existing deep-planning/design path and stop for human review before implementation |
| invalid/ambiguous classifier result | Do not silently authorize direct execution; run the bounded grounding/diagnosis fallback or stop with an actionable inspection result |

Risk remains independent. High/critical direct work still uses the direct flow but keeps its stronger reviews and approvals.

### Suggested code shape

Names may change if the implementation has a cleaner equivalent, but the responsibilities and observable behavior must remain.

1. Extend `internal/governance/ai/classify.go`:
   - Add `Approach` and `ApproachRationale` to `Classification`.
   - Define `direct`, `diagnose`, and `design` constants and strict parsing.
   - Make classification accept a bounded repository-context string.
   - Prompt explicitly distinguishes “known many pieces” from “shape must be invented.”
   - Return parse validity/confidence or an error; malformed output must not silently become authorization.
2. Add a bounded diagnosis primitive under `internal/governance/ai/`:
   - Input: ticket intent and bounded repository excerpts.
   - Output: `RootCause`, `Evidence`, `MinimalFix`, `AffectedSurfaces`, and `FixSizeEstimate`.
   - Evidence must identify a repository file/location, deterministic reproduction, or observed test result. Unsupported speculation is not a successful diagnosis.
   - The diagnosis is read-only. It does not invoke the executor or edit the checkout.
3. Add an explicit routing result in the governance service:
   - Record original classification, route, rationale, and grounding evidence.
   - For diagnosis, record both the original symptom classification and the reclassified minimal fix.
   - Keep routing separate from risk-policy evaluation.
4. Wire `BuildContract`/`work run`:
   - `direct` continues to contract construction.
   - `split` returns proposals and no persisted executable contract.
   - `design` invokes or clearly hands off to the existing deep-plan path and must not reach `executeContractDirect` in the same run.
   - `diagnose` must re-enter routing with the minimal fix and have a recursion/attempt bound of one diagnosis pass.
5. Add CLI overrides for recovery and controlled experiments:
   - `--approach direct|diagnose|design` records an operator override in the audit event.
   - Existing `--size` continues to override size within the selected approach.
   - An approach override never overrides risk floors or review policy.
6. Make output inspectable:
   - Before any approval, print approach, size, risk, rationale, and whether repository evidence was used.
   - Diagnosis output prints the supported root cause and minimal fix before contract approval.
   - Design/split output explains why execution did not start and gives the exact next command/state.

### Safety invariants

The implementor must preserve all of these:

- No executor process starts for `design`, `split`, failed diagnosis, invalid classification, or an unapproved contract.
- A diagnosis can narrow or clarify the requested fix; it cannot silently add unrelated work.
- If the diagnosed minimal fix is itself design-shaped or epic, execution stops and routes accordingly.
- Repository context is bounded and requester text remains clearly delimited as untrusted input.
- Proposal-version changes invalidate prior approval exactly as before.
- Risk can only be preserved or raised by existing floors; route selection cannot lower it.
- Existing `.openexec`/legacy compatibility is unchanged.
- Explicit overrides are visible in history/audit output.

### Required automated coverage

Add table-driven classifier tests and service/CLI routing tests for at least:

1. Typo in a named file -> `direct` + surgical; no planner or diagnosis.
2. Known one-line bug with file/function named -> `direct`, despite `kind=bug`.
3. “Used to work; now returns 500” with no cause -> `diagnose`; executor cannot start before grounded diagnosis.
4. Diagnosis finds a minimal one-file fix -> reclassifies to direct and builds the contract from the diagnosed fix.
5. Diagnosis has no evidence -> stops/parks; no executable contract or approval is produced.
6. Diagnosis discovers architectural redesign -> `design`; no direct execution.
7. New subsystem with unresolved interfaces/storage decisions -> `design`; deep-plan path only.
8. Three known independent deliverables -> `split`; no stories/tasks for one executable change and no executor call.
9. Large but coherent, already-designed change -> direct + standard, not automatically epic.
10. High-risk surgical authentication fix -> direct + surgical while retaining high-risk reviews.
11. Malformed/partial classifier response -> inspection fallback or actionable stop, never silent direct execution.
12. `--approach` and `--size` overrides -> selected route/shape and an audit record; risk remains unchanged or floored upward.
13. Diagnosis recursion bound -> a second diagnose classification stops rather than looping.
14. Cancellation/timeout in classification or diagnosis -> command aborts cleanly and persists no approval.

Use fakes for the classifier, diagnosis model, executor, and planning service. At least one command-level test must prove that the executor fake was not called for every non-direct route.

### Routing evaluation fixture

In addition to unit tests, add a versioned fixture (JSON/YAML is sufficient) containing representative historical or anonymized tickets with:

- title/body;
- bounded repository evidence or a fixture-repository reference;
- expected approach and acceptable size range;
- short human rationale;
- whether execution is allowed immediately.

Provide a deterministic evaluation command that reports a confusion matrix and disagreements. Model-backed evaluation may be optional in ordinary unit CI, but the fixture parser, scoring, and recorded baseline must be deterministic. Start with at least 20 varied cases and grow toward 50 real cases.

### Evidence the implementor must return for review

The change handoff must include:

- files changed and a short architectural explanation;
- the route state machine or decision table actually implemented;
- test command output, including race tests for touched packages;
- before/after output for one trivial, one diagnosis, one design, and one split fixture;
- the routing-evaluation baseline and every disagreement;
- explicit note of any behavior in this contract that was deferred or changed;
- confirmation that unrelated dirty-worktree files were not included.

### Reviewer acceptance checklist

The reviewer should reject the change if any answer below is no:

- Does repository evidence affect classification before route selection?
- Do direct, diagnose, design, and split reach observably different code paths?
- Can a malformed or ambiguous classification start the executor? It must not.
- Is diagnosis bounded, read-only, evidence-backed, and minimal-fix scoped?
- Are design and epic distinguished by unknown shape versus known separable pieces?
- Are risk and route independent?
- Are overrides explicit and audited?
- Does every non-direct route prove the executor was not invoked?
- Are routing quality and disagreements measurable on a committed fixture set?
- Do existing governance, compatibility, unit, and race tests still pass?

### Recommended reviewable delivery slices

If this is too large for one change, deliver it in these independently reviewable slices:

1. **Grounded classification and observability:** repository context, strict `approach` parsing, printed/audited decision, fixture evaluator; no production routing change yet.
2. **Design and split routing:** prevent executor entry and route to existing plan/split behavior.
3. **Bounded diagnosis:** evidence-backed minimal-fix report, one-time reclassification, no-executor failure modes.
4. **Meaningful direct size profiles:** make surgical/small/standard select documented budget/effort policies using collected calibration data.

Slice 1 is intentionally observational, but the overall task is not complete until slices 2 and 3 are wired. This sequencing allows classification quality to be reviewed before it is trusted with execution authority.

## Is the idea valid?

Yes. Enterprises do need a control plane between a ticket and a coding agent, especially when multiple providers are used. The high-value artifact is not the generated code alone; it is the evidence of:

- what was authorized;
- which version was authorized;
- what actually changed;
- which deterministic checks ran;
- who or what reviewed the complete result;
- which external CI result belongs to the exact PR head;
- who permitted merge.

Pro contains several of those primitives already. The market risk is not that governance is unnecessary. It is that GitHub/GitLab rules, CI, policy engines, agent hooks, and coding platforms already cover parts of this. GitHub Copilot CLI, for example, now exposes hooks, skills, plugins, and MCP integration. Pro needs to be the neutral evidence and policy layer across agents and execution environments, not a second issue tracker or a proprietary wrapper around one CLI.

## What Pro gets right

### Approval integrity is thoughtfully designed

Approvals are pinned to proposal versions, invalidated on material change, and checked against an intact chain before execution. Unknown risk fails toward critical, and risk escalation only moves upward.

### The audit store has a better default than workspace-local state

Governance state defaults to `~/.openexec/governance/<project-fingerprint>/governance.db`, and new chains use an HMAC key stored separately at `~/.openexec/audit_key`. This is a meaningful improvement over leaving approvals in an agent-writable project database.

### Operator identity can fail closed

When `~/.openexec/operators.yaml` exists, the environment-variable shortcut is ignored and the current authenticated GitHub identity must be allowlisted. Invalid or empty allowlists deny access.

### Merge provenance and policy checks are substantive

The service checks that a recorded PR belongs to the governed source/repository and expected branch. Critical work requires explicit risk acceptance. Auto-merge requires policy opt-in, a completed state, operability clearance, and live green checks on the current PR head.

### Relay starts with good intake controls

Only explicitly labeled issues are considered; dry-run, issue caps, and an open-PR cap are present. This is the correct shape for a pilot.

## Critical Pro findings

### 1. The primary lane bypasses the product it is supposed to govern

The normal Pro flow does not use the core daemon, blueprint, pipeline, checkpoint, gate runner, or run evidence. `executeContractDirect` invokes Claude/Codex/Gemini itself. Core is used for CLI composition, model/project helpers, SQLite helpers, and the deprecated/manual plan lane—not for the main execution-first delivery loop.

This produces two execution architectures with different safety and reliability behavior. Fixes to core gating do not automatically protect Pro, and Pro cannot truthfully present a core run receipt for its main path.

**Direction:** define one public `Executor`/`RunEngine` interface in core that supports an execution contract, isolated workspace, event stream, cancellation, deterministic commands, and evidence receipt. Use it from both the open-source CLI and Pro. Direct CLI execution may remain one adapter, but it must be behind the same contract and controls.

### 2. The “outside-workspace audit boundary” is defeated by the direct executor model

The audit design assumes an agent confined to workspace writes cannot access the governance DB or HMAC key in the operator's home directory. But the primary Claude executor is launched with `--dangerously-skip-permissions`, no container/OS sandbox, the user's environment, and the repository as its working directory. A shell-capable or prompt-injected agent can potentially read or alter `~/.openexec/governance`, `~/.openexec/audit_key`, GitHub credentials, and other home-directory state.

The HMAC chain still detects ordinary accidental DB edits and offline tampering without the key. It is not a security boundary against the executor as currently launched.

**Required fix:** run the executor under a separate OS identity or container/VM with only a temporary worktree mounted; do not mount operator home, audit state, SSH keys, or ambient `gh` credentials; use a narrow broker for approved Git operations. Sign the final receipt outside the executor boundary.

### 3. Allowed paths and budgets are enforced only after broad execution

The prompt tells the agent which paths to touch, but the process is not prevented from touching other files. The budget check occurs after execution and commit. This is useful detection, not containment.

Use a disposable worktree plus filesystem policy where practical, and always treat post-run scope measurement as an audit/gate rather than a sandbox.

### 4. A dirty user workspace can be committed into the governed PR

`executeContractDirect` does not check for a clean working tree. It switches/creates branches in the user's existing checkout, then uses `git add -A`. Pre-existing tracked changes and untracked files can be committed as if the agent produced them. Switching branches can also fail or carry changes across branches.

**Required fix:** never execute in the user's checkout. Create a detached temporary Git worktree from the exact base SHA, execute there, collect its diff, and delete it after the PR/receipt is safely recorded. Pin base SHA in the contract.

### 5. Parked scope escalation is not actually resumable

When autonomous execution exceeds budget, the command says the larger contract and branch are parked for later approval. But escalation changes the record back to `plan_ready`. A rerun enters contract-building again rather than the approved/implementing resume branch. Then `executeContractDirect` deletes any existing `gov/<change>` branch and starts fresh.

The same issue affects an interactive user who declines the larger measured scope after being told “the branch too” is saved. The commit may remain recoverable through reflog, but the supported flow discards it.

**Required fix:** introduce an explicit `scope_escalated`/`awaiting_reapproval` state that preserves the measured contract, base/head SHAs, worktree/branch, and diff digest. Reapproval must continue with that exact diff, not rebuild the contract or rerun execution. Add an end-to-end regression test.

### 6. Verification is delegated to the same agent and is not independently enforced

The executor prompt says to run verification, but after the agent exits Pro does not run a deterministic list of required commands. It measures the diff and sends it to AI reviewers. There is no command output, exit status, or test artifact tied into the run gate.

This is the largest usability/trust gap. “The agent said tests pass” is not verification.

**Required fix:** resolve required commands before approval, show them in the contract, execute them independently after the agent, capture bounded logs and hashes, and fail closed. For industrial use, allow externally attested simulation/test-bench evidence.

### 7. Diff review can approve an incomplete diff

`work run` calls `branchDiffText(..., 80000)`. AI reviewers therefore see at most the prefix of a large diff. The merge gate checks for clean diff-review events, not proof that reviewers saw the complete diff digest. A large or generated change could be approved based only on its first 80 KB.

**Required fix:** fail closed if a diff cannot fit, or review a manifest plus every chunk and record coverage. Bind each review to the full diff SHA, file manifest, base SHA, and head SHA. Any later commit must invalidate reviews.

### 8. PR readiness and merge rules are inconsistent

The flow opens a normal PR, not a draft, even though independent verification and live CI have not been required. Assessment posting is best-effort. The human-operator merge path checks governance reviews but does not require current green CI; only auto-merge does. Thus a human operator can merge a red, pending, or absent-CI PR through the Pro command.

This may be a deliberate escape hatch, but it conflicts with the product's stronger “verified delivery” direction and its own plans.

**Required fix:** open draft PRs until deterministic local verification and required live checks pass on the exact head. Make green required checks the default merge rule for both human and automated paths, with an explicit, separately audited emergency override—not an implicit human bypass.

### 9. Default approval and autonomous approval blur authority

Interactive contract approval defaults to yes on Enter. Relay reuses an operator session as standing authority and passes `--yes`, so labeled issues can have AI-drafted contracts approved without a human reading them. The actual human gate becomes merge, as the README states.

That is a valid low-risk operating mode, but it should not be described as human contract approval. Separate identities and event types for “operator explicitly approved this contract” and “policy pre-authorized this labeled class of work.” Default interactive approval should be `[y/N]` for a command that immediately launches a broadly privileged agent.

### 10. Model configuration and standalone versioning are inconsistent

The direct executor reads `PlannerModel`, even though project setup asks for separate planner and executor models; the public project view apparently did not expose the executor field. The module requires core v0.9.0 but relies on a local `replace ../openexec`, while the README says standalone use needs v0.11.0 seams.

**Required fix:** expose a stable execution-model accessor in core, pin Pro to the minimum real released API version, and test both sibling-development and clean standalone builds in CI.

### 11. The main happy path lacks direct tests and repository CI

Service/store/policy tests are substantial and the race suite passes, but the riskiest command code has no targeted coverage. No `.github` workflow was present in the Pro checkout. A green service suite cannot catch branch deletion, dirty-worktree commits, truncation, CLI argument mismatch, resume loss, or PR draft/readiness behavior.

Add hermetic end-to-end tests with fake agent and GitHub executables covering:

- clean success;
- dirty source checkout;
- no-op agent;
- out-of-scope-only retry;
- real scope escalation and exact resume;
- executor failure/cancellation;
- deterministic verification failure;
- diff larger than the review limit;
- repair invalidating previous reviews;
- PR creation failure and retry;
- pending/red/current-head CI;
- base branch moving during a run.

### 12. Relay backpressure counts local state, not GitHub reality

The open-PR cap counts change records in local `pr_open` status. If GitHub PRs close without sync or state transitions lag, the cap can pause unnecessarily or fail to reflect actual review load.

Use live GitHub state as the source, reconcile local state each tick, and make each relay item an idempotent state machine with a durable lease.

## Usability assessment

### What is already usable

- A developer willing to run a local CLI can import an issue, inspect a concise contract, execute one agent, and get a governed PR.
- Error messages around GitHub permissions and operator setup are generally actionable.
- Persistent clarification answers and bounded repair are good human-facing behavior.
- `relay tick --dry-run --max-issues 1` is a reasonable pilot interface.
- Governance status/history/audit commands offer useful inspection.

### What prevents dependable team use

- Setup spans a sibling core checkout, a local `replace`, model/CLI installation, `gh` auth, operator config, policy, and environment variables.
- There is no clean “doctor/preflight” command proving all dependencies and boundaries before a run.
- Work happens in the user's checkout rather than an isolated worktree.
- The command can take destructive branch actions and commit unrelated files.
- A parked escalation does not safely resume.
- No independent verification receipt tells a reviewer exactly what passed.
- PR state does not clearly distinguish unverified, locally verified, and CI-verified.
- The CLI has both deprecated and current lanes, making the surface much larger than the supported golden path.

### Recommended usability flow

One command should establish readiness:

```text
openexec-pro doctor
  ✓ core/pro versions compatible
  ✓ repository clean; base SHA pinned
  ✓ isolated executor available
  ✓ model/provider reachable
  ✓ operator identity and policy loaded
  ✓ GitHub permission: push + draft PR
  ✓ required checks discovered
  ✓ audit signer outside executor boundary
```

Then make the golden path explicit:

```text
openexec-pro work run ISSUE --dry-run
openexec-pro work run ISSUE
openexec-pro work status ISSUE
openexec-pro work approve-merge ISSUE
```

Hide legacy plan/release/autopilot commands from primary help or move them under `experimental`/`legacy` until they have a clear audience.

## Product direction

### Recommended product definition

**OpenExec Core:** provider-neutral, local-first execution runtime that turns an immutable execution contract into a reproducible evidence receipt.

**OpenExec Pro:** policy and connector plane that decides which contracts may run, binds human/policy authority, connects ticket/SCM/CI systems, verifies provenance, and authorizes promotion/merge.

The boundary should be:

| Core owns | Pro owns |
|---|---|
| isolated worktree/executor | ticket and repository connectors |
| immutable run contract | organization policy and risk tiers |
| deterministic commands and artifacts | authority, approval, and separation of duties |
| complete diff manifest | required reviewers/check policies |
| event stream/checkpoint/cancellation | cross-system audit export and retention |
| signed or signable run receipt | merge/promotion authorization |

Pro should not implement another agent runner. Core should not own enterprise approval semantics.

### The artifact to optimize: a run receipt

Every run should produce one machine-readable, content-addressed receipt containing at least:

- project/repository identity;
- ticket/change identity;
- contract and proposal version;
- base SHA and final head SHA;
- agent/provider/model and tool version;
- isolated execution policy identifier;
- complete changed-file manifest and diff digest;
- every required command, exit code, log/artifact digest, and duration;
- review authorities, complete-diff coverage, and decisions;
- external CI check IDs and their head SHA;
- approval/override identities and timestamps;
- final outcome and PR URL.

This receipt is the common surface for the UI, audit export, PR comment, industrial evidence pack, and API integration. It is much more defensible than a broad dashboard.

## A practical 90-day plan

### Days 0–30: trust reset

- Fix core data races and add race CI.
- Make core gates fail closed and prove generated-project defaults.
- Move Pro execution to temporary worktrees immediately, even before full sandboxing.
- Add dirty-worktree, branch-resume, truncation, and deterministic-verification tests.
- Change PRs to draft and require live checks for normal merge.
- Correct docs to distinguish detection, governance, and isolation.
- Align core/Pro module versions and add CI for standalone composition.

### Days 31–60: one shared execution contract

- Add the public core execution interface and immutable receipt schema.
- Make Pro call that interface instead of launching the coding CLI directly.
- Run agents with a separate identity/container and minimal mounted state.
- Bind reviews and CI evidence to base/head/diff digests.
- Add `doctor`, a minimal golden-path help surface, and recovery commands.
- Remove or quarantine dormant core features and deprecated Pro lanes.

### Days 61–90: prove the product

- Run 30–50 real, small changes across 3–5 repositories and multiple agents.
- Publish anonymized reliability metrics: completion, false-green rate, human rejection, median intervention, verification failures, recovery success, and time-to-PR.
- Integrate with existing agent hooks/CI rather than requiring teams to replace their agent.
- Recruit one industrial pilot limited to offline/non-production engineering and simulation.
- Deliver an evidence export mapped to the customer's change-control requirements; do not promise compliance certification.

## What to stop doing

- Stop adding named subsystems before the core loop is trustworthy.
- Stop equating an AI verdict with verification.
- Stop describing `--dangerously-skip-permissions` execution as workspace-confined.
- Stop maintaining two primary execution architectures.
- Stop opening ready-for-review PRs before deterministic checks and required CI are known.
- Stop treating a hash chain as proof of correctness; it proves sequence/integrity only within its key boundary.
- Stop targeting direct factory control. Target governed engineering changes and evidence first.

## Final verdict

### OpenExec

**Idea:** strong.  
**Implementation depth:** substantial.  
**Current reliability:** alpha; not safe to trust unattended.  
**Best niche:** provider-neutral execution policy and evidence runtime.  
**Immediate priority:** correctness, real gates, isolation, release hygiene, and a single receipt—not more features.

### OpenExec Pro

**Idea:** valid and commercially plausible.  
**Flow design:** good high-level UX, especially contract -> measured diff -> bounded review -> PR.  
**Current usability:** useful for supervised local pilots; not ready for dependable autonomous team delivery.  
**Main architectural issue:** it governs around a direct agent invocation instead of governing the core execution engine.  
**Immediate priority:** isolated worktrees/execution, independent verification, complete-diff binding, safe resume, draft/CI state, and end-to-end tests.

### Factory-world answer

There is a place for this if the product becomes the controlled bridge between engineering intent and validated evidence. The winning statement is not “AI operates your factory.” It is:

> “Any approved agent can propose an industrial software change; OpenExec runs it in isolation, proves exactly what was tested, and hands a signed evidence package to your existing human-controlled deployment process.”

That is narrower, safer, and more valuable.
