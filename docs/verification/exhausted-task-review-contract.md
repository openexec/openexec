# Exhausted-task review contract — US-010 / T-US-010-002

This stage defines acceptance; it does not claim the four repairs are implemented.
The authoritative provisional dispositions and case matrix are
[scripts/verification/exhausted-task-review-cases.json](../../scripts/verification/exhausted-task-review-cases.json).
All four diagnoses are provisionally accepted with source evidence and explicit
qualifications. The discovery assessment remains the source analysis, not a
second repair result. No finding has been resolved at the review service.

The desired outcome is an authorized, bounded correction of the same exhausted
candidate, preserving attempts, receipts and independent work. Current source
has pre-admission errors, no shipped manager-authority caller, plan-only checks,
and incomplete boundary evidence. OpenExec's native queue and conditional store
writes own the repair; reuse its manager API, executor, completion rules and
operator split. No second implementation loop is needed. Complexity delta in
this stage: two machine contracts and a verifier with tests; no runtime concepts,
states, controllers, effects or production behavior changes. Live Project context
was read; its wider portfolio roadmap does not expand this selected task.

## How the checklist is consumed

Each case names a package and exact test/subtest selector, fixture mutation,
required executable assertions and a negative control. Existing tests are
explicitly marked as needing strengthened assertions; other selectors are
requirements for implementation, not claims that tests already exist. A test
that only checks `err != nil` cannot satisfy the refusal checklist.

Run each selector using the command template in the JSON, anchoring every
slash-separated component. Collect native `go test -json` events and feed them
to the verifier's `--events` option. It requires run and pass events for every
exact case plus package passes. Zero matching tests, parent-only passes, absent
cases, skips and failures refuse. Assertion descriptions must be implemented in
the named tests before this evidence can count as repair acceptance; synthetic
JSON is not native verification evidence.

Every refusal fixture must actually reach the intended boundary: grant valid
initial authority first, mutate exactly the named binding, and observe the
refusal before checks. For independent-work drift, arrange dependency ordering
so the independent task changes the candidate in the same queue before A is
eligible; do not substitute an external edit for this concurrency scenario.
Use channel barriers for races and Stop/cancellation, and `-race` for concurrent
cases. Record store reopen, exact original receipt bytes, completed repair,
3/3 attempts, blocked dependents, check counts, and second-run disposition.

Each finding is complete only when all its cases and named negative controls
pass. Apply mutants with source overlays, demand the intended named assertion
failure (never a compile error), then verify original source hashes and repeat
the positive journey. The original exhausted-task regression also retains its
existing exact-refusal removal control. Public access additionally requires a
real built-CLI scratch journey, including persisted decision identity and scoped
queue completion. OpenExec surface evidence does not establish Console transport
or deployment; that cross-repository absence claim remains unproven.

No-plan success requires an actual native obligation; the suggested legacy
fallback does not authorize vacuous success. Retain all accepted plan obligations
alongside the task script. Failed scripts must produce failed/continuing_failure
with a consumed pass and no silent restart. An arbitrary different but internally
consistent decision reference is not inherently invalid: the falsifier compares
the supplied decision to the persisted grant. Merely returning nil on refusal
is insufficient unless the next iteration skips the terminal record.

## Coverage denominator

[scripts/exhausted-task-coverage-scope.json](../../scripts/exhausted-task-coverage-scope.json)
is the scope policy. It consumes the existing whole-function inventory without
copying it, adds every function in the correction production files, and unions
all added/modified production functions since the retained implementation
baseline. AST discovery includes untracked files and all internal/pkg/cmd paths,
so new CLI handlers, constructors, helpers and shared registration changes cannot
escape merely by using another filename. Deleted functions require scope review.

The verifier derives statement blocks from Go source instrumentation. Absent
profile blocks keep their full statement count and receive zero hits; malformed
or missing profiles refuse. Coverage must be strictly greater than 90% across
the complete whole-function denominator. No exclusion to increase coverage,
partial ranges, package-average substitution or threshold reduction is allowed.
The older US-008 verifier remains historical baseline tooling; new review
acceptance must also consume this expanded scope, especially the CLI package.

## Dependencies and boundaries

Only T-US-010-001 is a stage dependency: its discovery and inventory are consumed
by this contract. Subsequent repair verification consumes this matrix and scope.
Runtime dependencies are retained task/repair dependencies and accepted validation
obligations, not artificial ordering between unrelated findings. No new task
edges, Navigator work or publication/merge/deployment tasks are created.
Console owns publication, canonical gates, review resolution and exact owner
merge acceptance after native preparation. An ordinary local Git commit preserves
this selected stage. D1/D2 and PR delivery are not established by definitions.

## Runnable record consumer

T-US-010-003 adds the [records verifier and delivery boundary](exhausted-task-records.md).
It consumes this unchanged definition contract; it does not promote definitions
to repair evidence or resolve the provisional findings.

## Verification

Run `bash scripts/verify-exhausted-task-review-contract.sh` for this stage.
The verifier rereads both JSON artifacts, validates every case/disposition and
resolves source functions. Its tests remove each mandatory case, weaken policy,
omit test events, skip/fail cases, omit coverage blocks, and introduce new CLI
and shared functions into discovery. These are contract-tool negative controls,
not the unexecuted production repair mutants.

To evaluate subsequent native artifacts, use the same script with
`--events /path/to/go-test.jsonl --profile /path/to/coverage.out`.
Neither option alone declares the review repaired; all journey assertions,
negative controls and scratch CLI evidence remain necessary.

Compatibility evaluation: no Go production or Go test behavior changes in this
stage; existing .openexec/.uaos loading and migration fallbacks are unchanged.
Fresh execution results belong in the single task entry in NOTES.md.
