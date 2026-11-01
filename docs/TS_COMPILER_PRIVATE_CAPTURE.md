# Private capture at the CLI compiler boundary

For the guarded-binary recurrence in Console run 41258644434, see
[the finite diagnosis and next experiment](TS_COMPILER_RECURRENCE_DIAGNOSIS.md).
That capture's replay refused before launch; it must not be retried under these
general instructions.

This is an evidence producer, not a repair for the recurring compiler deadline.
The 45-second child deadline, output limits, corpus and lexical fallback remain
unchanged. Do not repeat a warm Console scan to populate this evidence. Enable
the producer for the next naturally scheduled impact replay, then replay only
that captured request once.

## Enable and retain

`OPENEXEC_TS_COMPILER_CAPTURE_DIR` must name an existing absolute directory with
mode 0700 outside the source repository. A symlink at that final directory,
nonprivate mode, relative path or directory inside source is refused. An invalid
destination prints a generic warning and extraction continues unchanged. No
capture I/O occurs when the variable is unset or empty.

Each extraction creates a unique 0700 `compiler-*` child containing 0600 files:

- `request.json`: exact child stdin, including filenames and exclusions.
- `extractor.mjs`: exact embedded script, retained before child startup.
- `identity.json`: actual running executable hash/build info, source HEAD and
  content/configuration manifest, Node/TypeScript paths and hashes, 45s bound,
  and bounded host/cgroup snapshot before the child.
- `result.json`: child argv, stdin/script/toolchain hashes, Node/TypeScript
  versions, parent/child deadlines and errors, classified ending, exit status,
  elapsed/CPU/resource usage, output byte counts and snapshot after `Wait`.

Absent `result.json` means incomplete evidence, never success. A failed child
still produces a result; a killed whole CLI may leave only the pre-child files.
No source bodies, compiler stdout/stderr, diagnostic messages or environment
dump are saved. Paths and hashes are sensitive too: keep the entire directory
private, outside Git and outside public CI artifacts/logs. Retain it across the
train's normal retry and collect it using the supervisor's existing private
evidence channel. The producer does not upload, rotate, retry or schedule work.

The opt-in manifest and identity hashing run before starting the child's 45s
clock. Version probes run after extraction and each has a 2s timeout, 100ms pipe
wait bound and 4096-byte stdout cap. Two fixed host snapshots each read at most
4096 bytes per known file; unavailable host files are explicitly unknown.
These instrumentation costs are separate from measured child elapsed time.
Snapshots are not a historical load trace or proof of a causal bottleneck.

## Single Console supervisor handoff

At Console tree `ebfada5622ec1a25fb503dc01757c28f17a22a27`,
`.github/workflows/check.yml` quality setup installs Go/Node/dependencies and
then invokes `make ci-quality-${{ matrix.part }}`. It does not install or pin
the external OpenExec CLI. `Makefile:117` launches `cmd/impact-replay` with
`--openexec "$(OPENEXEC)"`; `cmd/impact-replay.runOpenExec` uses
`exec.CommandContext` to launch that executable. Its embedded Go module pin is
not an executable receipt. Neither today's supervisor PATH nor a passing retry
establishes the failed worker's binary identity.

The required cross-repository patch is limited to **Console Makefile and
`.github/workflows/check.yml`** on this task's existing delivery identity. It
must use a reviewed/deployed OpenExec binary containing this producer; no Go
module pin update establishes that prerequisite. Do not rerun/cancel a train.
Prepare the following wiring through the native supervisor boundary; this
OpenExec candidate does not modify the owner Console checkout:

```diff
--- a/Makefile
+++ b/Makefile
@@
+ifneq ($(strip $(IMPACT_REPLAY_CAPTURE_DIR)),)
+impact-replay: export OPENEXEC_TS_COMPILER_CAPTURE_DIR := $(IMPACT_REPLAY_CAPTURE_DIR)
+endif
 impact-replay:
```

Insert this step in `quality.steps` immediately before the existing
`make ci-quality-${{ matrix.part }}` step. Only the impact-replay target exports
the producer variable, so server/package tests do not create unrelated captures.

```yaml
- name: Prepare private natural compiler evidence
  if: steps.part.outputs.run == 'true' && github.event_name == 'push' && matrix.part == 'go-1'
  shell: bash
  run: |
    umask 077
    capture_parent="$HOME/.local/state/openexec/compiler-captures/$GITHUB_RUN_ID/$GITHUB_RUN_ATTEMPT/$RUNNER_NAME"
    mkdir -p "$capture_parent"
    chmod 700 "$capture_parent"
    capture_parent="$(realpath "$capture_parent")"
    echo "IMPACT_REPLAY_CAPTURE_DIR=$capture_parent" >> "$GITHUB_ENV"
```

This location survives runner temp cleanup and separates attempts/runners. Do
not add an `upload-artifact` step on the public repository. The supervisor must
retain the failed attempt's directory privately before housekeeping removes it,
together with the workflow job/run IDs, tested Console SHA and original corpus
report. No raw request should enter the public PR. A future synthetic regression
must be reviewed separately for privacy before publication.

## Replay the captured request once

The existing `TestConsoleNativeRequestCapture` driver accepts
`-compiler-private-capture=<absolute compiler-* directory>`. It does **no scan**:
it reads actual stdin/argv, checks the script, Node/TypeScript hashes and full
source/configuration manifest, then reuses `replayNativeRequest` under the
unchanged 45s bound. Run it on the same worker with the captured checkout and
toolchain still present. Changed inputs or an existing `replay.json` are refused.
Source/toolchain refusals now list each failed manifest or hash comparison by
field name, without printing private values. Older combined refusals cannot be
retroactively resolved by this reporting change.
The original temporary script must be absent; replay restores identical bytes
at its original pathname with exclusive creation and removes them afterward.
The restored script has a new mtime, which is an instrumentation difference.

`replay.json` is reserved before launch and retains the single result. A started
record without a result is incomplete; do not delete it to repeat the probe.
A deadline/nonzero/undecodable replay fails the driver while retaining its
record. An exit-0 replay is a nonreproduction, not repair acceptance. Compare
the captured child/identity records before proposing any causal change.

The existing declared Go test suite exercises this producer, the actual CLI
scan handler, saved graph reopening, synthetic ending/refusal cases and the
single identical-stdin real-compiler replay. These are capability tests; none
claims to reproduce the natural Console worker failure. The older
`TYPESCRIPT_C727_DEADLINE_HANDOFF.md` preserves the earlier nonreproduction only.
