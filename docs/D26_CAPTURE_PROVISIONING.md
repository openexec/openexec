# D26 private capture: hash-verified provisioning

Metadata only. No capture content is recorded here, and none may be synthesized.
Provenance search and the retained requirement: [D26_CAPTURE_PROVENANCE_SEARCH.md](D26_CAPTURE_PROVENANCE_SEARCH.md)
(the authentic-capture requirement is **retained**; the standard gate stays blocked until an authentic file exists).

- Expected SHA-256: `0cbc9da8b5a6ed7d03db2b4a0a8ac16818f1c648f3db776090c83651bba20708`
- Target path: `/home/perttu/.agent-console/run-requests/d26bd137975612543c90e5b983f60db8.json`

## Procedure (repeatable)

1. Obtain a candidate file from a recorded authentic source. Never create one by hand or from a replay.
2. Run `sh scripts/provision-d26-capture.sh /path/to/candidate`.
3. The script hashes the candidate. On any mismatch, or if the candidate is missing, it exits non-zero, prints a fixed message (never file content) and installs nothing.
4. On an exact match it copies to a temporary name beside the target, re-verifies the copy's hash, then renames it into place and exits 0.
5. Run `go test ./pkg/execution -run TestD26 -v` and confirm `TestD26PrivateOccurrenceEvidence` runs and is not skipped.

Rule: verify hash, then install, never synthesize.

## Where the hash is enforced

`TestD26PrivateOccurrenceEvidence` does **not** hash the capture; it checks parsed fields
(164 messages, 22 tools, role counts, dispatch facts). A wrongly sourced file with matching
fields would pass the test. The hash is enforced only by the provisioning step above. Pinning
it inside the test is a separate change, deliberately not made here (owner default: keep in the
provisioning step).

## Fail-closed demonstration

`pkg/execution/d26_provisioning_test.go` (run by `go test ./pkg/execution`):

- `TestD26ProvisioningScript`: script refuses a wrong file and a missing file under the real hash,
  refuses a one-character-off hash, and installs only on an exact match, using throwaway files in a temp dir.
- `TestD26ScratchBadCapturesFailClosed`: copies the unchanged `d26_evidence_test.go` into a scratch
  module, rewrites the absolute paths and the events hash in the copy only, presents a throwaway
  events file plus (a) non-JSON wrong bytes, (b) valid JSON with wrong fields, (c) no capture, and
  asserts the test FAILS (not skips) with its fixed message and no planted content in the output.
  A control shows the test skips when the events file is absent, which is why (c) keeps it present.

Captured output: [verification/d26-provisioning-scratch-demo.txt](verification/d26-provisioning-scratch-demo.txt).
Bare `go test ./pkg/execution -run TestD26 -v` on this host: [verification/d26-bare-run.txt](verification/d26-bare-run.txt)
(`TestD26PrivateOccurrenceEvidence` fails here because no authentic capture exists; unchanged by this work).
The real target path and real test were never touched by the demonstration.
