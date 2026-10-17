# US-008 / T-US-008-005 story evidence

## Candidate and commands

The final tested source revision is `fcbc3e61b137c76d76f2b02dd91426fde923d1c1`.
All completed US-008 cases were rerun on this same clean candidate on
2026-09-29. The subsequent evidence-only commit changes documentation and
recorded results, not implementation, tests or verifiers. `results.json` records
the three aggregate commands, exit statuses and candidate identity;
`verification.txt` retains the story command transcript. The refreshed unit
report replaces `../implementation-unit-coverage/final.json`.

Run `bash -euo pipefail scripts/autonomy-contract/verify-runtime-evidence.sh
--case CASE`, with CASE `native-recovery`, `engine-recovery` or `story-evidence`.
Aggregates require a clean committed checkout and refuse a changed revision or
working tree at completion. They execute existing cases, preserve child failure
as a nonzero result, and print aggregate PASS only after all children succeed.
The story inventory covers every completed US-008 leaf case plus harness tests;
the separate US-007 architecture cases are not part of this story's inventory.
A failure in any required child is blocking, never converted into partial PASS.

## Acceptance criteria

| # | Accepted criterion | Verification and observed result |
|---|---|---|
| 1 | Behavioral pre-fix recovery reproduction | `behavioral-reproduction` and `exit-1-slice` extracted baseline `245746baaa66372c4641d137053e330fbc75f679`, ran the retained fixture against supported APIs, and required its behavioral failure. Actual exit 1 was persisted and reloaded; reopened A remained failed at attempt 1 with two tasks and no repair. Compilation failure cannot satisfy the check. Immutable fixture and original log remain in `../exit-1/`. |
| 2 | Exported trusted terminal boundary and persisted bindings | `runtime-boundary` passed 69 required tests/subtests across runtime, gates, pipeline and blueprint. `TestTerminalBoundary` and admitted binding checks validate terminal ID, task, stage, both attempts and deterministic source, trusted loading, actual exit, and mixed-error refusal. Recovery tests persist and reload completion before classification. |
| 3 | Real admitted exit 1 enters existing validated repair path | `native-recovery-tracer` passed all 49 required manager tests/subtests. `TestPersistedExitOneRecovery` executes an actual failing file check, reloads terminal bytes, passes the exported classifier and existing gates, and creates native repair from validated failure artifacts. |
| 4 | Repair executes, A resumes, B runs; reopen durable state | The same test and `TestPersistedRecoveryMatrix` require A -> repair -> A -> B, actual repaired file contents, three completed tasks, A at attempt 2, and a durable failed run-step with matching terminal ID/exit/bindings after closing and reopening SQLite and manager. |
| 5 | Exit 0, qualifying range, replay and restart | Matrix exits 0, 1 and 125 pass both continuous and receipt-boundary restart branches. Exit 0 leaves two completed tasks, A at attempt 1 and zero receipts; nonzero cases retain exactly one repair and one receipt. Twice replaying retained evidence and rerunning the completed queue dispatch nothing and create no duplicates. Boundary inventory checks 1, 2, 125 and excluded endpoints; it does not claim subprocess enumeration of every integer. |
| 6 | Refuse unauthorized failures without persisted repair | All 34 named `TestPersistedTerminalRefusalsDoNotRepair` branches pass, including missing/tampered/untrusted completion, stale bindings, missing/unsupported exits, forged worker artifacts, loader/context failures, cancellation, timeout, launch, transport and mixed errors. Freshly reopened storage has one unfinished task at attempt 1, no repair, no attached failure evidence and zero deterministic-verification receipt rows. |
| 7 | Local process and protected-project compatibility | `compatibility-refusal` passed 125 required tests/subtests without skips. Real local processes cover exits 0, 1, 125, 126, 127, 128 and 255 with `exec.ExitError` and mixed-error refusal. Current `.openexec` and legacy `.uaos` status CLI fixtures are loaded in two fresh processes; legacy config and `.openexec/tasks.json` fallback are reread, preserving project/task state. Evaluation and fixture details remain in `../compatibility-refusal/README.md`; this run refreshes that evidence on the final source. |
| 8 | Reuse native machinery and preserve authority | No production files changed in tasks 003, 004 or 005. The implementation from the behavioral slices still uses the trusted caller loader, existing failure-artifact conversion, run-step receipt and native queue. No process-error fabrication, public trust boolean, alternate classifier or orchestration loop was added by aggregation. Refusal tests exercise these boundaries. |
| 9 | Strictly greater than 90% changed-function unit coverage | Fresh `implementation-unit-coverage` passes: **325/344 statements, 94.48%**, across all eleven complete changed production functions, each individually above 90%. The AST comparison, commands, per-function counts and uncovered blocks are in the refreshed `final.json`. Scope excludes unchanged functions, tests/tooling and nonexecutable declarations; integration/recovery and compatibility coverage do not enter the numerator. Detailed method remains in the coverage README. |
| 10 | Runnable slices and fail-closed test inventories | Every US-008 leaf case passed. All 68 Python harness tests passed, including absent required tests/branches, skipped tests, zero-test runs, incomplete/wrong package verdicts, unsupported CLI cases, failed/killed commands and timeouts. Aggregate controls fail each child in turn and prove no later dispatch or aggregate PASS; actual shell dispatch preserves failing child exit 17. Dirty or changed candidates are also rejected. |
| 11 | Revision-bound final story evidence | All three aggregate entry points passed on the source revision above. This table, command transcript, refreshed coverage report and results manifest bind baseline, running recovery, refusals, compatibility and reopened-state checks to that candidate. Limitations below remain explicit. |

## Compatibility and complexity evaluation

This task changes only the existing verification entry point, a small aggregate
composer, its tests and evidence. Product loaders, migrations, schemas, APIs,
native execution and repair behavior are unchanged. Both prerequisite branches
added tests/tooling only; their combined coverage and compatibility checks were
nevertheless refreshed on the final source. No runtime concepts, persistent
product state, transitions, owner decisions or failure modes were added or
replaced. Existing native task convergence remains the implementation owner;
Console retains Goal review and deterministic delivery.

## Unresolved failures and unverified checks

The historical US-007 architecture check rejected the restored runtime because
it still asserted resource absence. US-010 replaces that stale observation and
requires the restored declarations; all 46 architecture command tests pass.
This corrects the documentation verifier, not a runtime or deployment result.

The canonical repository gate, full `make test`, `make compat-test`,
`make type-check`, Console launch/policy and real-HTTP integration, external
consumer fixture, independent review, publication, owner acceptance, merge and
deployment are **unverified by this stage**. The running native journeys use real
local processes and temporary persisted fixtures; they do not prove Console
integration or deployment. The observed Console serving revision is not evidence
of OpenExec delivery. No unresolved failure occurred in the required US-008
aggregate inventories. Any future required-case failure blocks that aggregate.
