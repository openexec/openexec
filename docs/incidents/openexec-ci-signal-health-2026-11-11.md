# openexec-ci signal health, 2026-11-11

Status: unverified

## Why unverified

No reading from the real `openexec-ci` observer could be obtained in this pass, so no health of `healthy`, `failing` or `unobservable` is claimed. This pass ran in a sandbox with no network egress and no GitHub authentication. It did not run the observer and has no observer output to cite.

Missing inputs, each needed for a real reading:

- the observer revision (`scripts/observe/_signal.py` and `internal/sre/types_ci.go` are not in this checkout; they are in an Agent Console worktree);
- access to the live Console that runs the observer;
- the resolved values of `OBSERVE_OPENEXEC_REPOSITORY` and `OBSERVE_OPENEXEC_BRANCH`;
- the credential named by `OBSERVE_CI_TOKEN` (not recorded here);
- network access from the place the observer runs.

The observer does not persist response bodies or transport errors, so a current reading can only come from a new observation. The operator or Console lane must run it and replace this record with: the observer revision or invocation, a UTC timestamp, and the sanitized response or error class.

## What is not the evidence

- Hosted run 41862455768 is not the evidence for this status.
- No other GitHub Actions run is the evidence.
- CI results, merge status and merged pull requests are not the evidence. A green or red CI run says nothing about whether the observer can read the signal.

## History only

`docs/incidents/openexec-ci-4265.md` holds a reading of `JSON observation failed`, exit code -1, at 2026-11-05 19:19:43 UTC. That is a historical reading six days old. It is not current health, and this record does not claim recovery.
