# Finite private compiler phase experiment

Status: harness implemented; new retained capture refused context admission.
Natural phase localization and cause remain unknown. No repair or recovery claim.

This implements the previously specified experiment in
[PR185's diagnosis](TS_COMPILER_RECURRENCE_DIAGNOSIS.md), using the existing
`TestConsoleNativeRequestCapture` entry point and exclusive `replay.json`
reservation. Only test files change executable behaviour. Production complexity
delta is zero: no runtime concept, state, transition, owner decision, controller,
compiler policy, capture producer or Console wiring changes.

## Result on the new retained capture

The native admission entry point returned exit status **1**, before manifest
scanning, reservation, reconstruction or child launch:

```text
faithful context unavailable (context.dependencies, context.generated, context.inherited_configuration, context.default_libraries)
```

The four retained artifacts were present with private modes. Their identity
record does not contain historical dependency, generated-context, inherited
configuration or default-library inventories; no separate historical context
packet was supplied. Re-reading the four original files after admission refusal
confirmed identical bytes and unchanged 0700/0600 modes. No `replay.json` or
experiment directory was created. Neither the worker nor an owner source
checkout was scanned or edited. No natural child ran, and no historical capture
reservation was deleted or retried.

The smallest missing evidence is a supervisor-authenticated historical inventory
of those four context domains, bound privately to the retained request and source
manifest. It must include membership and contents, external inherited config,
default libraries, generated declarations, dependency symlinks and their targets,
and the original launch environment/configuration affecting module resolution.
A repository revision, lockfile, compiler module hash or freshly hashed current
worker is insufficient. The harness cannot authenticate the historical origin
of an operator-supplied packet; that authentication belongs to the supervisor's
existing private evidence channel. Do not manufacture it from today's files.

Consequently source/configuration/Node/TypeScript/context equivalence in a
disposable reconstruction is **not established**. Admission stops at the missing
context prerequisite without probing the original worker. The unfinished natural
phase remains unknown; zero decoded diagnostics does not identify it.

## Private operation after faithful context becomes available

Use a disposable supervisor-owned reconstruction preserving the captured absolute
root, toolchain paths, source bytes/order, exclusions and compiler options. Do not
redirect the recorded argv to a present-day checkout. Preserve all four original
artifacts. The context packet is a separate 0600 private test input, outside Git;
it contains `request_sha256`, `manifest_hash`,
`supervisor_attests_complete_historical_closure: true`, and `trees`.
Each of the four domain keys maps absolute tree roots to full path/value maps:
regular-file SHA-256, `directory`, `absent` or `link:<target>`. The packet's
historical provenance must already be authenticated; the boolean records that
prerequisite, rather than establishing it.

`dependencies` must cover `<captured-root>/node_modules`, `generated` the **whole
captured root** (including hidden and newly added generated files), and
`default_libraries` the captured compiler directory. The inventory walker follows
resolved symlink targets once and rejects unresolved links. `.git` directories
are excluded. `inherited_configuration` must explicitly cover the complete
historical external configuration and launch context; an empty/missing domain is
refused. Explicitly absent roots remain comparisons, not a permission to omit
unknown context. Completeness is a supervisor prerequisite: the harness cannot
discover uncaptured historical reads. All required domains and capture binding
are checked before launch, in addition to the existing source/config/toolchain
identity comparisons.

Invoke the existing native test driver with `-compiler-private-capture`,
`-compiler-phase-context` and `-compiler-phase-output`. Each value is an absolute
private path; the output must be a **new separate directory** under an existing
0700 parent, outside source and capture. The original temporary script pathname
is not restored: the transient instrumented script is retained privately in the
experiment directory. This changes script pathname/mtime and adds synchronous
sampling/writing overhead. Compiler require path and original stdin are retained.
Do not treat path, cache or machine-load differences as controlled variables.

For any long compiler/browser invocation use the existing supervisor durable
systemd-backed runner, with a single stable job name, never this turn's cgroup.
Prepare and verify the candidate before handing off; the runner is not another
capture producer or permission to retry. This pass launches no long natural run.

The driver reserves the existing `replay.json` exclusively before child startup.
That reservation also excludes the unchanged replay route. An admitted attempt
is spent even if script/output creation or the child fails. Do not delete any
reservation. Original request/script/identity/result files remain unchanged.
Experiment directories are 0700; instrumented script, side channel and
measurement are 0600. Keep them private; never upload or paste them into CI/PRs.

## Measurements and interpretation

The production extractor is copied in memory and instrumented at exact unique
anchors. A missing/duplicated anchor refuses instrumentation. Its stdout stays
the sole original compiler response, with no progress messages. A synchronous
private fd 3 carries only nine-integer JSON rows:

```text
[phase, group, event, elapsed_ns, user_cpu_us, system_cpu_us,
 diagnostic_count, guard_selected, cumulative_telemetry_overhead_ns]
```

| Phase | Boundary |
| --- | --- |
| 1 | Bootstrap: request read and TypeScript require, after static imports |
| 2 | Configuration discovery, grouping, declarations and output initialization |
| 3 | Program creation per numeric group ordinal |
| 4 | Checker creation per group |
| 5 | Eligible/skipped diagnostics, including diagnostic collection |
| 6 | Complete extraction/reference traversal, even after budget fills |
| 7 | Sorting, serialization and original stdout write call |

Events 1/2 mean entry/exit. Global phases use group 0; program groups start at 1.
Guard selection is 1/0 only for diagnostics and -1 otherwise. The diagnostic count
is the extractor's actual accumulated count, which can exceed 20 inside one
eligible call; this experiment does not cap it. Monotonic elapsed time and
cumulative process CPU yield completed wall/user/system deltas. The last entry
without exit remains unfinished. No events means startup is unknown. Lazy
checker work may occur during diagnostics or traversal; the labels describe
source boundaries, not TypeScript's internal causal stack.

The cumulative overhead counts clock/CPU sampling, numeric serialization and
synchronous writes preceding the current event. It is a **lower bound** excluding
the last event's write, imports, instrumentation setup and other perturbations;
do not subtract it to claim an uninstrumented counterfactual. The private
measurement retains the modified-script hash, classified child ending, exact
45-second bound, completed deltas and unfinished phase even after termination.
Malformed/nonnumeric telemetry is refused and cleared from the side channel;
no path/body is copied into the fixed refusal. Compiler stdout/stderr are not
retained. A successful child with missing required events is refused, while a
killed child never fabricates missing exits.

A successful instrumented child is nonreproduction, never cure. A later repair
requires real compiler fail-before/pass-after/fix-removed failure, graph and
provenance/reopen proof, and unchanged pinned 15/15. Original Stop/accounting,
train controls, `6b→19a`, Work/Metrics and manual gates remain outstanding under
their existing owners. After an actual served repair, the supervisor measures
the original full gate and useful-work recovery separately.

## Reproducible verification and sibling boundaries

The declared Go suite includes the synthetic real-Node/TypeScript native journey,
refusal matrix, deadline retention, required instrumentation, numeric privacy,
context symlink and destination tests in `ts_compiler_phase_test.go`.
`TestCompilerPhaseGuardRemovalControls` builds one Go overlay per admission,
privacy, phase-completeness and reservation guard and requires the corresponding
behavioural assertion to fail; compiler/setup failure cannot count. These
controls run as Go tests, not a standalone verification script. The short forced
deadline fixture changes only its synthetic test bound; the opt-in experiment
always uses the unchanged production 45 seconds.

The ordinary identical-stdin private replay and older reconstructed-file-list
driver remain separate controls. The new route shares `replay.json` admission
with private replay; it never weakens either route's toolchain/manifest checks.
Full scans, incremental refresh, CLI graph extraction and Console's compiler
capture producer still use the single unchanged production extractor. No
fallback, timeout, cap, corpus, anchor, filter, graph, delivery, orchestration,
compatibility or Stop/accounting route changes.
