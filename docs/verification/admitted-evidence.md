# Admitted evidence — US-012 / T-US-012-005

## Story disposition and provenance

**Incomplete: actual Console capture/retention/attachment remains unresolved.**
Local WithOutput compatibility is verified; it cannot establish the accepted
exact-argv/cwd/private-reference contract at the actual adapter. No delivery,
deployment, merge or external coverage claim follows from the local results.

This record replaces the earlier task-local verification summaries. The tested
OpenExec revision is `320a39e0c04716ddf82e17449684a9b90b513c96`, before this
continuation's evidence-only update, on 2026-09-30. Production code is unchanged. Local
commands below ran in the supplied candidate worktree. Console source was read
at the owner-supplied serving revision, not inferred from another checkout's HEAD.

## Console adapter — US-012 / T-US-012-003

Read-only commands against `/mnt/data1/projects/agent-console`:

```sh
git rev-parse 4c819427
git show 4c819427:internal/server/openexec_checks.go
git show 4c819427:go.mod
```

Resolved Console revision: `4c819427643655fc85d830b3a79f337488bd3fc0`.
Its go.mod requires OpenExec `v0.13.2-0.20260930061734-4290c0d23b00`.
Source SHA-256 for openexec_checks.go:
`a7094a264d44a369574e938cf74133c0cfc6ef652abdadc7287e35f5c7447017`;
go.mod SHA-256:
`7c03c602ad132a9ac794804a3e37e3c078f8165dd8331c8b9e3c685bbbb2c8c7`.

`executeOpenExecCheck` sets cmd.Dir to the candidate path, combines both streams
in openExecCheckOutput, and calls VerificationCommandFailureWithOutput with
strings.Join(argv, " ") and output.String(). It does not use EvidenceBuffer,
RetainCommandEvidence or VerificationCommandFailureWithEvidence. Exact argv
boundaries, cwd and separate private streams are therefore not attached by this
source path. The earlier assertion that no Console change was required was
incorrect for the accepted story contract and is removed.

The native WithOutput API remains compatible: command/6000-byte output-tail
receipts survive run-step reload and authorize useful repair even for silent
exits and nil StageResults. That benefit does not invent a private reference or
recover exact argv/cwd absent from the receipt. Local tests use a separate
Console-shaped executor; they do not execute Console's function. Actual adapter
journeys and its coverage are still required after the capture/attachment change
in an authorized Console workspace. Console is readable but outside this stage's
writable roots. No sibling source, dependency, runtime or deployment was changed.

The supplied serving-process observation is revision/start-time context only;
this stage inspected that revision's source, not the process executable/module
attestation. The separate Console checkout HEAD was
`1d8aa1d8f120c8347d0431210c92475085182a2a` and was not used as serving-source
evidence. Fresh permitted Git reads reproduced both source hashes and the
dependency above. The failure is confirmed missing adapter behavior at the
supplied revision, not a sandbox socket refusal or an unrun local check.
Removing it requires the capture/attachment change and actual adapter reload
proof in the Console workspace; this stage cannot write that repository.

## Acceptance criteria and reviewer cases

| Criterion / F2 checklist | Executed proof and persisted assertions | Disposition |
| --- | --- | --- |
| 1. Both engine APIs preserve failed results; F2-results | admitted-tracer engine baseline and isolated ExecuteStage/Execute discard controls check exact result/error preservation and persisted stage evidence. | Local pass |
| 2. Public evidence-bearing wrapper | Public-runtime-only silent tracer uses VerificationCommandFailureWithEvidence; wrapper-removal control fails the missing-reference and diagnostic-free assertions. | Local pass |
| 3. Silent exit 2 and terminal reference; F2-silent | Real lint/test shell processes emit EventBlueprintFailed with a typed 64-hex ref, no StageResult artifact fallback. Manager closes/reopens SQLite and re-reads registered evidence. | Local pass |
| 4. Exact command/cwd/exit and usable repair | Public reload journeys assert argv, cwd==WorkDir, exit 2, bounded separate streams, diagnosticFreeReceipt=false, artifact resolution and persisted repair description after fresh manager/store creation. | Local pass |
| 5. Late diagnostic marker; F2-tail | Oversized stdout/stderr survive bounded head/tail capture, terminal event, store reopen and repair reload. Independent prefix-only capture and public-summary mutants fail their named tail assertions. | Local pass |
| 6. Nil-result/errors, cancellation, launch; F2-results/F2-boundaries | Diagnostic lint/test journeys cover populated/nil results. Public classification/refusal tests cover success, reserved exits, mixed errors, actual launch failure, cancellation of a started process and admission/transport refusal. Unit matrix checks nil/nil pair preservation at the adapter boundary, not arbitrary full-blueprint success. | Local pass |
| 7. Allowlist and secret exclusion; F2-boundaries | Reload tests accept only allowlisted version context; redacted events/repair summaries exclude credential sentinel. Artifact hashes remain separate from classification digest. Unit scope covers forged artifacts, storage refusal, malformed receipts, token-bearing command/output, redaction and authority reset. | Local pass |
| 8. Actual Console adoption; F2-lint-test/F2-adoption | Source/dependency audit above confirms WithOutput but missing private capture/attachment. Eight local compatibility journeys are not actual Console execution. | Incomplete |
| 9. Strict >90% whole-body unit coverage | admitted-unit-coverage instruments the declared scope, including changed/added bodies; rejects missing/altered blocks, profiles, source hashes, required tests and scope omissions. External Console scope is explicitly not measured. | Local pass; external incomplete |
| 10. Isolated discard/attachment negative controls | Each independent temporary source copy must fail the intended assertion after its baseline passes; compile failures, skipped tests and unrelated failures are refused. Candidate sources remain unchanged. | Local pass |
| 11. Consolidated evidence | This record, coverage record and fresh aggregate JSON retain commands, outcomes, reload assertions, limitations and non-completion. | Recorded; not story completion |

Shared capture drains every write and keeps 4096 bytes per stream (first/last
2048 on overflow); direct retention applies the same limit. Public output removes
cut/partial lines, redacts credential values and reserves a truncation marker.
Private capture retains partial lines. Toolchain allowlisting admits only
go_version, node_version, npm_version, python_version and rustc_version, each
capped at 128 bytes; no incidental environment dump is captured. Missing or
invalid evidence never grants repair authority. Protected format compatibility
is unchanged: this task modifies only verification scripts/tests and documents,
not loaders, schema, receipt formats or runtime behavior.

## Commands and results

The original task command `bash scripts/verify-verification-repair.sh --case
admitted-all` reproduced exit 2: the dispatch case did not exist. It now runs
all local members, validates the eight non-skipped WithOutput fixture journeys,
and **exits 1 if actual adapter evidence is missing, stale or incomplete**.
It writes fresh progress/failure evidence before checking external adoption.
`admitted-diagnostics` now dispatches the existing diagnostic/tracer verifier.

The optional external adapter report is passed directly to
`python3 scripts/verification/admitted_story.py --adapter-evidence FILE`.
`check_adoption` documents its schema: exact candidate revision, Console source
revision/hashes and dependency, actual command/exit, no unresolved work, all eight
lint/test × silent/diagnostic × populated/nil scenario assertions (including
private reference and database/repair reload), and >90% actual adapter coverage.
This is an externally supplied verification artifact, not authentication or
binary attestation. The aggregate never fabricates it from fixture results.

Verified in this candidate on 2026-09-30:

- Declared host `lint`: exit 0 (Go vet and UI ESLint).
- Declared host `test`: exit 0 (all Go packages; 635 UI tests in 40 files).
  Existing React act/style warnings and a WebSocket port-in-use diagnostic
  were non-fatal; the declared check returned exit 0.
- `bash scripts/verify-verification-repair.sh --case admitted-all`: exit 1,
  correctly refusing absent actual adapter evidence after all three local members
  exited 0. Fresh report: `/tmp/openexec-admitted-story-hnxvutr7/result.json`.
  This report was reread from disk; status remained failed with all local results
  and the explicit incomplete-adoption reason retained.
- `admitted-tracer`: 16 diagnostic-boundary and 16 silent/classification
  completions, plus the engine baseline. Both engine discard mutants, wrapper
  attachment removal, prefix-only capture and prefix-only public summary were
  rejected at their intended assertions. Exact tail failures were
  `lost prefix/tail` and `unsafe or missing public tail`.
- Aggregate adapter compatibility command:
  `go test ./pkg/manager -run '^TestAdmittedAdapterFailureReloadAndRepair$' -count=1 -timeout=60s -json`:
  all eight journeys and their top-level test passed, with strict no-skip event
  validation. Their store reopen and persisted repair assertions are in the table.
- Fresh unit measurement matched the totals in
  [admitted unit coverage](admitted-unit-coverage.md); all required scoped tests
  and omission/refusal controls passed. Its result.json was reread from disk.
- `python3 -m unittest discover -s scripts/verification -p 'test_*.py'`:
  111 tests passed, including aggregate missing/stale/incomplete adoption,
  failed-local-member and stale-success replacement controls (5.724 seconds).
- Shell syntax and `git diff --check`: passed.

The continuation reproduced the previous failure after successful host checks
and fresh local journeys. No local verification failure remains to repair.
The aggregate must continue refusing completion until actual adapter work is
resolved; substituting the passing fixture would weaken the accepted criterion.
Full canonical gates were not run in the sandbox.
Canonical full gates, actual Console proof, review and publication remain with
their existing owners. Complexity delta: no runtime concepts, transitions,
execution loops or owner decisions; verification composition only. Ordinary Git
in this candidate is the explicitly authorized substitute for safe_commit.
