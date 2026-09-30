# Planner schema discovery

US-007 / T-US-007-002. Offline discovery for the planner schema incident;
production scalar enforcement and bounded correction remain US-008 work.
Project context and the Simple Loop contract were read. OpenExec's existing
Planner and public runtime facade own this boundary; no controller is added.

Run from the candidate repository root:

```sh
scripts/verify-planner-schema-recovery.sh --case discovery
```

The verifier emits JSON with the actual candidate root, HEAD, hashes of candidate
inputs, fixture provenance, executed test count and observed diagnostics. It
checks the Git root, Go module and resolved public runtime package directory;
disables ambient workspaces, Go settings and GOFLAGS; and detects changed inputs
during verification. It accepts uncommitted candidate changes and hashes their
bytes. This identifies what ran without claiming a clean commit or deployment.
Wrong working directories, unknown cases, command failures/timeouts, missing or
duplicated tests, skipped tests and incomplete execution fail closed. Both test
listing and JSON execution events must contain the required tests; a Go exit 0
with no tests does not pass.

The [fixture manifest](../../pkg/runtime/testdata/planner-schema/provenance.json)
binds sanitized fixtures to the source digest established in
[inspection evidence](planner-schema-inspection.md). The original source digest
was rechecked before sanitizing. Discovery needs neither the original private
artifacts nor `/tmp` reproducer files. It verifies each fixture digest, incident
mappings, isolated variants and that the scalar control changes only
`requirement_id`. Text, tasks and commands are synthetic. The scalar selection
is an experimental control, not an authorized way to discard or join mappings.

`pkg/runtime/planner_schema_discovery_test.go` uses only the public runtime API.
It exercises bare story arrays and object envelopes through public JSON decoding,
GeneratePlan, GenerateCompactPlan and RefinePlan with a deterministic fixture
provider. Scalar success preserves all story fields and refinement goals, writes
the plan to a temporary file, reloads it and compares the entire public plan.
No network, model inference, imports, owner decisions or deployment occur.

The current public `PlanStory` has a custom decoder that **accepts arrays**.
Discovery reports that enforcement gap rather than pretending the historical
failure still exists. A method-free defined type derived from `runtime.PlanStory`
preserves its actual public fields/tags and asserts the declared scalar schema
rejects incident, empty, multi-value and single-value arrays. Its typed
`json.UnmarshalTypeError` must identify `requirement_id`, source `array` and
destination `string`. The emitted Go diagnostic is:

```text
json: cannot unmarshal array into Go struct field discoveryScalarStory.requirement_id of type string
```

That is a schema diagnostic, not a claim that the current public decoder emits
it. Public array results are observations; public scalar results are assertions.
Neither array acceptance nor the misleading historical `no stories found` error
is required to remain. The same discovery can run after scalar enforcement or
bounded correction is implemented. Later recovery verification must assert public
array rejection/correction and retained budgets; this case does not certify them.

Verification performed on this candidate:

- Discovery: exit 0; exactly 57 required test nodes passed, four underlying Go
  schema diagnostics emitted, current public array coercion observed.
- `go test ./internal/planner ./pkg/runtime -count=1 -timeout=60s`: exit 0.
  Existing tests were unchanged; the new discovery tests do not replace the
  existing list-coercion assertion, which belongs to the later schema repair.
- `python3 -m unittest discover -s scripts/verification -p planner_schema_discovery_test.py -v`:
  eight controls passed, covering wrong modules, replacement modules, no tests,
  skipped/failed tests, nonzero commands and fixture tampering.
- Actual verifier invocations refused an unknown case (exit 2), another working
  directory (exit 1), a modified empty-array fixture (exit 1) and removal of the
  required schema test name (exit 1). Files were restored and discovery passed
  again. These were controlled negative experiments, not production edits.
- Temporarily removed array coercion, then separately masked the planner error
  as `no stories found`: discovery passed both variants with 32 public array
  rejections and all scalar routes passing. This exercises repair-compatible
  discovery and independent diagnostic exposure. Production source was restored
  byte-for-byte and baseline discovery passed again.
- Host `run_declared_check(check="lint")`: exit 0, Go vet and UI ESLint.
- Shell syntax and `git diff --check`: exit 0.

Compatibility evaluation: only fixtures, tests, verification scripts and docs
changed; `.openexec`, `.uaos`, tasks.json loading/migration, runtime schemas and
production execution remain unchanged. Complexity delta: no new runtime
concepts, persistent state, transitions, owner decisions or execution machinery.
Canonical full gates, publication and delivery remain Console/runner-owned.
