# NOTES

Raw capture. One line per thought, any grammar.

## Now

- [US-012 / T-US-012-005] Consolidated story verification and refusal of
  fixture-only adoption are recorded in
  [admitted evidence](docs/verification/admitted-evidence.md). Actual Console
  source at the supplied serving revision still lacks private capture/attachment;
  fresh host checks and the aggregate reconfirmed the failure and local passes;
  completion is refused. The concrete remaining adapter change is recorded there.
  Local strict coverage lives in
  [admitted unit coverage](docs/verification/admitted-unit-coverage.md).

- [US-014 / T-US-014-003] Storage checklist, strict coverage, isolated old-directory
  source/staging negative control and host checks verified. Consolidated commands,
  permissions, round trips and legacy policy live in
  [private storage](docs/verification/private-storage.md); coverage scope lives in
  [storage unit coverage](docs/verification/storage-unit-coverage.md).

- [US-013 / T-US-013-001] Task verifier, resolver negative controls,
  public-command regression and targeted recapture/compatibility tests pass.
  Continuation from c9447aa2 reconfirms diagnostic-free receipts and their
  timing across the dispatcher fix; neither records the executed revision.
  Historical receipts cannot identify their executed revision. Console checks
  are now available; current passing storage/story results are recorded
  under US-014 above. Historical incident evidence remains in
  [named recapture](docs/verification/named-recapture.md).

- [US-010 / T-US-010-001] Current G-007 review study and exact provenance live in
  [repair study](docs/verification/repair-study.md). All three source defects are
  accepted provisionally; repair proofs remain with US-012/013/014, aggregate
  and external D2 with US-015/Console. Historical G-006 fixture passes do not
  cover the empty-phase incident or actual Console adapter adoption.

- [US-011 / T-US-011-001] Final fail-closed technical composition and native
  journeys are maintained in [delivery evidence](docs/verification-evidence-delivery.md).
  Current baseline results, per-slice coverage, mutation proof and pending external
  D2 are recorded there; Console retains canonical gate and delivery ownership.

- [US-009 / T-US-009-005] Shared recapture dispatcher and fail-closed aggregate
  implemented. Fresh scenario, persisted boundary, coverage and protected-format
  evidence is maintained in [recapture evidence](docs/verification-evidence-recapture.md).
  Canonical gates and delivery remain Console/runner-owned.

- [US-009 / T-US-009-004] Protected-format recapture journeys and standalone
  verifier implemented. Exact verification results and source-backed limits
  live in [recapture compatibility](docs/verification/recapture-compatibility.md).
  Production behavior is unchanged; full canonical gates remain runner-owned.

- [US-009 / T-US-009-003] Dedicated recovery unit tests and the full-body
  fail-closed coverage verifier are complete. Scope, verification results and
  aggregation contract live in
  [recapture unit coverage](docs/verification/recapture-unit-coverage.md).
  Production behavior is unchanged; canonical gates remain runner-owned.

- [US-009 / T-US-009-002] Dedicated native recapture boundary journeys and
  standalone verifier complete. Persisted retry/terminal/repair and Settings
  completion-obligation evidence lives once in
  [recapture boundaries](docs/verification/recapture-boundaries.md).
  Shared composition is recorded in the T-US-009-005 evidence above.

- [US-009 / T-US-009-001] Existing authoritative recapture implementation and
  injected-executor repair verified on continuation. Added the remaining-budget
  interrupted restart journey to the required verifier manifest: exactly one
  remaining dispatch, durable exhaustion, no repair or restart refund, Settings
  still waiting. Current verification and implementation evidence live once in
  [legacy recapture](docs/verification/legacy-recapture.md).

- [US-008 / T-US-008-005] Shared retention dispatcher and fail-closed aggregate
  complete. Fresh real-command/reopen/repair, coverage and both discard mutation
  checks passed with an explicit no-skip scenario manifest. Commands, results,
  reload assertions and runner-owned gates are recorded once in
  [retention evidence](docs/verification-evidence-retention.md).

- [US-008 / T-US-008-004] Standalone isolated discard mutation verification
  and dedicated admitted repair fixtures are complete. Results and verifier
  controls are maintained in [mutation evidence](docs/verification/retention-mutations.md).
  Shared composition is recorded in the T-US-008-005 evidence above.

- [US-008 / T-US-008-003] Added dedicated retained-evidence unit tests and the
  standalone full-body coverage gate. Scope, baseline, executed results and
  aggregation artifact contract are maintained in
  [retention unit coverage](docs/verification/retention-unit-coverage.md).
  Real command, persisted repair/reopen, negative cases and verifier controls
  passed. No production behavior changed; canonical gates remain with the
  repository runner. Shared dispatcher and mutation ownership are unchanged.

- [US-008 / T-US-008-002] Implemented shared production bounded capture, private
  artifact reads, explicit toolchain allowlisting, redacted public diagnostics,
  and artifact-reference propagation separate from classification digests.
  evidence-boundaries and retained-result verifiers passed real commands through
  database reopen and persisted repair state; targeted gate/capture, engine,
  native deterministic command, terminal receipt, atomic task failure,
  repair/restart and admitted cancellation regressions passed. Discovery, shell
  syntax and diff checks passed. Broader package execution attempted blocked
  provider access; full canonical gates remain with the repository runner.
  Adapter contract and exact bounds are maintained in docs/ARCHITECTURE.md.
  Compatibility: no project-loading, schema or migration changes; existing
  legacy receipt parsing and repair/restart checks passed. Complexity: reuse of
  artifact files, run_steps and typed failures; no new scheduler concepts,
  transitions or owner decisions. New internal capture helper/private artifact
  payload only; capture-storage failure refuses repair authority. Independent
  coverage and mutation verification retain their assigned task ownership.

- [US-008 / T-US-008-001] Both engine APIs retain non-nil failed results.
  Pipeline terminal failure carries existing artifact references, stage identity,
  output and diagnostics; typed receipts alone authorize repair. References and
  evidence are persisted before repair generation. The retained-result verifier
  passed an actual admitted exit-2 process, both engine branches, database reopen
  and repair creation with exact argv/cwd, bounded streams and diagnostic marker.
  Existing forged-artifact, cancellation, atomic failure binding and repair/restart
  journeys passed, as did targeted engine/pipeline/gate tests, discovery, shell
  syntax and diff checks. Broader package execution attempted provider network
  access and was blocked by the sandbox; canonical gates remain with the runner.
  Compatibility: no project loading/schema/migration changes; legacy receipt
  parsing and existing repair/restart journeys remain supported. Complexity:
  no new persistent concepts, transitions or owner decisions; existing machinery
  reused. Independent mutation/coverage work retains its
  assigned task ownership in docs/ARCHITECTURE.md.

- [US-007 / T-US-007-002] Source-backed architecture, normalized requirement
  mapping and independent test/helper ownership are in
  [ARCHITECTURE.md](docs/ARCHITECTURE.md). Discovery passed (22 source declarations); nine verifier tests, shell syntax
  and diff checks passed. Full gates and live product delivery were not run; pending retention, recapture and delivery evidence belong to the
  task owners in that map. Earlier source inspection remains in
  [inspection evidence](docs/VERIFICATION_FAILURE_INSPECTION.md).

- [Simple Loop contract, 2026-09-14] `fix/native-reviewed-planning` exposes
  existing compact generation through `Manager.Plan`; native reviewed-plan
  replay/refinement/import and task execution remain canonical. Compact
  generation/review/import/replay plus real persisted-file task execution pass;
  removing compact selection makes the test fail. No Console Goal loop is
  required for that small-task journey. Full gates/review/publication and native
  brownfield acceptance remain pending. Do not reuse the exhausted PR185 live
  window. Contract: `docs/OPENEXEC_SIMPLE_LOOP_ARCHITECTURE_CONTRACT.md`.

- [Console handover seam, 2026-09-13] Building on the validated PR #54 slice:
  `Manager.Config.StageExecutor` admits Console execution without native/host
  fallback; `PlanRequest.RequestID` + accepted `Intent` retain exact reviewed
  plan IDs and import atomically using existing SQLite records. Console owns
  only initial product assessment and fresh Goal review at stable boundaries.
  Focused queue/repair, cancelled-review restart and import rollback checks
  pass. Composed canonical review/publication and live brownfield delivery are
  still outstanding; preserve the c7c2265 first-slice evidence as historical.

- [OpenExec-first task route, 2026-09-13] Current feature:
  `feat/task-oriented-goal-loop`, design/acceptance in
  `docs/TASK_ORIENTED_GOAL_ROUTE.md`. Reuse the planner/reviewer prompts,
  SQLite task ledger and blueprint engine. Opt-in sequential queue derives
  one same-story repair from a trusted failed-check receipt and resumes the
  original task; controlled-provider integration and restart tests exercise
  actual processes and files. This is not native/deployed Goal convergence.
  Next: finish independent review/full compatibility validation, then bridge
  Console admission/effects to this engine; do not enable the old unrestricted
  host command path or inherit expired grants. The outer fresh Goal review,
  second planning pass and long brownfield proof remain unfinished.

- [Non-inference readiness, 2026-09-12] APIProvider.Probe must never generate
  tokens outside an execution reservation. OpenAI-compatible adapters now use
  bounded GET model metadata; absent support is explicitly unknown, never a
  completion fallback. Console's unknown-state consumer must remain compatible.
  Focused HTTP fixtures and a restored-inference negative control passed; no
  live inference was used. CLI probes use auth-status commands, with unknown
  for unsupported status shapes. Both advertise non_inference_readiness so
  Console can refuse older inference-producing probe implementations before
  invoking them. Full validation and paired deployment remain.

- [Bounded execution context ceiling, 2026-09-12] A larger cumulative grant
  must not enlarge a bounded worker's authorized model context. Optional
  ContextTokenLimit now caps native admission independently of TokenBudget;
  hard_context_limit advertises enforcement, zero preserves legacy behavior.
  Focused tests and a removed-ceiling falsifier passed. Live paired verification
  remains owned by the Console recovery route; no provider was dispatched here.

- [V3 retrieval-context repair, 2026-09-10] Paired with Agent Console's bounded
  record retrieval/remote-fresh repair. Assembled native requests now receive
  padded context estimation and typed refusal; exact duplicate results can be
  reduced once without losing unique facts or granting capacity. Agent/execution
  suites and negative controls passed. Exact review/deployment and live native
  task advancement remain; estimates are not proof of tokenizer fit.

- [V3 bounded repair, 2026-09-10] Shrinking-context HTTP 400 diagnosed and
  locally repaired on the existing token-admission line. Preserve the owner's
  non-renewing grant; remaining delivery is exact-head review, guarded promotion
  and a reserved live request. See docs/HARD_TOKEN_ADMISSION.md. No new grant.

- [owner-experience evidence, 2026-09-03] Autonomous runs stopping after roughly
  30 minutes on a navigation budget defeats the core benefit: the owner should
  be able to leave while the system continues its task loop toward completion.
  This has interrupted both OpenExec and Rahoitettava work; after leaving for a
  walk, the owner returned to work that had stopped and could not progress
  without intervention. For local or otherwise unmetered providers, elapsed,
  cycle, tool-call, token, and estimated-cost limits should be visible warnings
  and telemetry by default, not execution blockers. Hard stops should remain
  for genuine safety, authority, consequential-effect, or explicitly metered
  spending boundaries. Another agent already owns the budget repair; preserve
  this as acceptance evidence and do not start a duplicate implementation.

- [task] Dogfood the experience-first operating model, then implement an
  advisory, provenance-labelled triage that the owner must refine and accept
  before architecture or implementation begins.
- Run the preregistered 20–30 task pointer-graph baseline/treatment evaluation
  on at least two repositories before deciding whether to fund Version 2.
- [task] V2.1: enforce freshness on every graph resolve/read, with stale re-resolution (trust gap from 2026-08-03 audit)
- [task] V2.3: secure graph query API for Agent Console + calls --direction outgoing (feeds Explore graph UX)
- [task] Supply OpenExec's read-only evidence dependency for Agent Console's
  external advisory MCP plan: complete V2.1 freshness and V2.3 secured graph
  access, then expose typed checkout-bound reads with body provenance.

## Questions

- US-010 concrete integration/evidence gaps are maintained once in the
  [study resource boundary](docs/verification/repair-study.md#resources-authority-and-delivery):
  authorized Console writes/adapter execution, binary attestation and canonical
  gate/Settings reconciliation command mapping. No new owner decision requested.

- US-007 implementation interface questions are maintained in the
  [inspection evidence](docs/VERIFICATION_FAILURE_INSPECTION.md#unresolved-interface-questions-for-implementation).

## For me
- [me] Laki env for Juha
