# Runtime verification handoff — US-010 / T-US-010-001

Status: **preparation incomplete; final runner checks Pending**. No publication,
review request, owner decision, merge or deployment was performed or scheduled.
The sole retained human boundary is T-US-010-002, dependent on T-US-010-001,
with the exact existing decision reason in docs/ARCHITECTURE.md and no decision
reference. Preparation never requires that future decision.

## Candidate identity and evidence location

Implementation inherited from `722a7a7f7892932d68db45b17a94710a5652a729`.
This task changes verification tooling, an external consumer fixture and docs;
no production engine implementation, schema, loader or migration is changed.
Verification source commit: `78eda0bcd827beb12a4e3850be1a9514d0d40f62`.
Candidate used for the short checks below: `68aa7c30337f8dbf65ef8fb87620d3106388465f`.
Compared with the verification source commit, only NOTES.md and this handoff
have changed; no implementation, test or verifier changes were found. The runner
records the exact tested full HEAD in `results.json`, checks cleanliness before
and after each command and refuses changed candidates. Logs and the reopened
JSON report are kept outside the worktree so they cannot dirty or impersonate
candidate source. A later evidence-only commit must identify the tested revision
and show no implementation/test/verifier changes before results are reused.

Runner state directory:
`/tmp/openexec-US-010-T-US-010-001-21d181edd63f7f60fd833e9c2ddb4225`.
On the current continuation, this directory was absent and no matching job record or
attention report existed in the repository runner's `.jobs` / `.attention`.
The previous stage's detached-start message therefore does not establish a
running or completed check. Submission must use Console's repository runner;
a sandbox-local detached process cannot establish durable inbox delivery.
Do not infer PASS from submission, a log's existence, task completion or earlier
US-008 evidence.

## Reproduction

From the clean candidate root, the task verification command is:

```sh
bash -euo pipefail scripts/autonomy-contract/verify-runtime-evidence.sh --case goal-validation
```

For long checks, submit this argv through Console `run_command` to the repository
runner (not a sandbox-local detached launch), retaining the same job name:

```sh
python3 /mnt/data1/projects/owner-model/job_runner.py start \
  --name openexec-US-010-T-US-010-001-21d181edd63f7f60fd833e9c2ddb4225 \
  --project openexec --cwd "$PWD" -- \
  python3 -B scripts/autonomy-contract/goal_validation.py \
  --state-dir /tmp/openexec-US-010-T-US-010-001-21d181edd63f7f60fd833e9c2ddb4225
```

The runner executes one check per chunk, exiting 3 to continue, 0 only when all
six checks pass, and 1 on failure. Collect completion from the inbox; do not poll.
The inventory is architecture, story-evidence (every engine leaf and Python
harness tests), external-consumer, make test, make compat-test and make type-check.
A failed prior result blocks resume until it is inspected; retain its log when
repairing and rerunning. Neither make check nor make pr-gate runs here: the
canonical gate remains Console-owned in the socket-capable repository runner.

Fast verifier tests and the deliberate real-decision refusal can be reproduced:

```sh
python3 -B -m unittest discover -s scripts/autonomy-contract -p 'test_*.py'
bash scripts/autonomy-contract/verify-runtime-evidence.sh --case architecture
bash scripts/autonomy-contract/verify-runtime-evidence.sh --case owner-acceptance
```

The last command must return nonzero without Console's authenticated loader.
It is not a preparation prerequisite. Its success in isolated test fixtures
establishes only verifier behavior, never actual owner acceptance.

## Criterion-by-criterion evidence

| Accepted US-010 criterion | Evidence and current result |
|---|---|
| 1. Final recovery/refusal and reopened persistence | Pending final `engine.log`. Reuses US-008's real admitted A -> repair -> A -> B journey, receipt-boundary restart, replay, refusal matrix and freshly reopened SQLite assertions. Pre-fix reproduction must fail behaviorally against baseline `245746baaa66372c4641d137053e330fbc75f679`. Earlier results remain under scripts/autonomy-contract/evidence/story-evidence; they do not certify this final candidate. |
| 2. Required repository checks | Pending `make-test.log`, `make-compat-test.log`, `make-type-check.log`. Any failure or unrun check remains a delivery blocker. |
| 3. Strictly greater than 90% unit coverage | Pending final report within `engine.log`. Existing AST-based inventory includes every complete added/modified production function relative to the behavioral baseline, requires each function and combined statement scope >90%, excludes integration/recovery tests from the numerator. Historical US-008 result was 325/344 (94.48%), not a final rerun claim. No production scope added here. |
| 4. External consumer | Pending `external-consumer.log`. Builds a separate unpublished module with a local replace to the exact candidate, imports only exported runtime APIs, executes reloaded terminal exit-1/exit-0, stale-binding and cancellation checks. It is an isolated local consumer, not production Console wiring. |
| 5. Architecture and handoff | Architecture command passed: 32 declarations, eight obligations. All 46 architecture command tests passed. Restored APIs replace stale absence claims. Final identity/results collection remains pending. |
| 6. Goal and owner verification | All 78 Python harness tests passed, including five goal orchestration tests and five owner fixture tests. Fixtures cover valid round-trip, absent/malformed, unauthorized actor, trusted-loader failure, and candidate/PR/Goal/task/decision/reference mismatch. Actual owner command exited 1 with authenticated loader absent, as required. |
| 7. Downstream Console evidence | Pending structured producer/projection, production module identity, launch-and-policy matching, native-stage and real-HTTP completion/policy contracts, downstream coverage and repository gates. Engine fixtures cannot satisfy these obligations. |
| 8. Console delivery preparation | This handoff supplies commands, evidence inventory and blockers. Local candidate commits are authorized work persistence. Console retains delivery commit/integration, PR publication, canonical gate, independent review and presentation of the exact merge decision. None performed or scheduled here. |
| 9. Retained HITL | T-US-010-002 identity, dependency, reason and absent decision reference remain unchanged; architecture traceability checks them. |
| 10. D2 | Pending authentic default-branch merge evidence. Engine readiness and owner acceptance cannot prove merge, downstream verification or deployment. |

## Owner-decision trust boundary

`owner_acceptance.verify` consumes a caller-owned authenticated loader, paralleling
the runtime trusted-loader boundary. Console must configure
`OPENEXEC_OWNER_DECISION_LOADER` as a JSON argv array, `OPENEXEC_OWNER_PR` as the
exact published PR identity and `OPENEXEC_OWNER_ID` as the independently authorized
owner identity. These are trusted Console configuration, never worker inputs.
The loader must authenticate the immutable decision's actor/provenance before
returning JSON fields `candidate`, `pull_request`, `actor`, `goal`, `task`,
`decision` and `decision_ref`. Required decision is `accept-merge`, Goal is G-006,
and task is T-US-010-002. Candidate must match the full current HEAD.
The checker itself does not authenticate an arbitrary JSON file: passing an
untrusted file reader as the production loader would violate this boundary.
Console's authentic loader integration remains pending; no adapter or decision
is configured by this repository. There is no file fallback or `authorized`
boolean override. The checker reads only and does not persist decisions.
Temporary fixture loaders exist solely inside isolated tests and are deleted.

## Blockers and collection obligations

Final required checks have no collectable runner result. Console `run_command`
refused the submission against clean candidate
`68aa7c30337f8dbf65ef8fb87620d3106388465f`: "server-minted or provenance-unknown
turns cannot run generic host commands." No job was started by that request.
The sandbox cannot write the repository runner's `.jobs` / `.attention` paths;
a detached-start acknowledgement alone is insufficient, as observed above.
The current continuation retried the prescribed Console submission and received
the same refusal; it did not start a sandbox-local detached substitute.
An authorized repository-runner submission and completion report are required
before collection can proceed. This is an execution restriction, not a request
for an owner merge decision. No delivery action was attempted.

On this continuation, all 78 Python harness tests passed in 3.263 seconds,
architecture and traceability passed (32 declarations, eight obligations), and
owner-acceptance returned exit 1 because the authenticated loader was absent.
`git diff --check` passed. These short checks do not replace the six final checks. Once the runner
reports completion, inspect every log and re-read results.json, replace Pending
entries with exact candidate-bound outcomes, and retain failures explicitly.
Do not mark this task complete until that collection is committed. Any source
change requires rerunning affected checks against the new candidate. Console's
canonical gate, independent review and authentic decision remain separate.

The supplied observed Console process `e6745def`, started
`2026-09-29T16:21:34Z`, identifies Console only. It is not independently verified
OpenExec deployment, module-consumption, downstream test or merge evidence.
Compatibility-sensitive production behavior is unchanged by this task; the
existing protected-project and refusal regression checks remain in the final
inventory. Complexity delta: no product concepts, state, transitions, owner
decisions or native machinery replacements; verification tooling only.
