# Inspection-only native runs ending at the caller budget: diagnosis

Signature `sig-5f0828d325c3`. Status: **cause unknown; NOT REPRODUCED; no
repair claimed.** This note is scrubbed metadata only: no request, prompt,
response, project data or credential appears here. It changes no limit, model,
provider, tool, timeout, cap, allowance or admission.

Sources are the task's supervisor-supplied facts (version 0.21.4, installed
native `3e3550d2`). The host-private full traces were **not** opened: no
authorized agent route retrieves them, and none was invented.

## Observed

| Run | Tools | Captured last request | Capture written | Console terminal |
| --- | --- | --- | --- | --- |
| `f8fd4b76…` (metrics) | 67 | 138595 B, 67 tool / 42 assistant msgs | 01:34:00Z | 01:34:01Z `inconclusive:budget_exhausted` |
| `95540794…` (delivery) | 94 | 199717 B, 94 tool / 57 assistant msgs | 01:17:28Z | 01:17:33Z `inconclusive:budget_exhausted` |

Later recurrences from the same signature (supervisor reads, same pattern):

- `7e84a97b…` 73 tools and `e5ac01a2…` 77 tools, both ended 04:12Z on
  `inconclusive:budget_exhausted`; worktrees clean at the starting commit.
  Last requests 327061 B and 284248 B, ending `terminated signal`.
- `ea5c7d7e…` (55 tools: 25 read_file, 21 search_files, 8 observational
  run_command, 1 list_directory; 3.658 s recorded tool wall time) and
  `75bb759a…` (44 tools, 14.426 s; one `git merge --no-commit --no-ff`
  leaving three conflicts, no resolution). Both ended 06:26–06:27Z.

Common to every capture:

- Truncated at `textLimit` 2000; no `<run>.trace.json` beside any of them.
- Final request advertises 22 functions **including** `write_file`,
  `run_command` and `run_declared_check`. Tool *advertisement* in the final
  request is therefore not the gap. This does not prove effective permission,
  nor what earlier requests advertised.
- Capture outcome is `failed` / `terminated signal received`; Console's
  classification is `inconclusive:budget_exhausted`. The capture is written
  1–5 s before the Console terminal.
- Recorded tool wall time is a few seconds (3.658 s, 14.426 s) against a
  two-hour run, so tool execution does not account for the elapsed time.
- Terminal "changed files" counts (36, 75, …) include staged merge content or
  are not authored work; worktrees are clean. They are not evidence of authoring.
- One `go test … -run TestNativeIncident` run in `7e84a97b` is a baseline
  regression pass, not a new failing test or candidate.

## Not reproduced

Last-request replays of the `f8fd` and `e5ac` captures (existing replay route,
same configured endpoint, `--timeout 45 --budget 1`) did not reproduce:
the replay failed with `request timed out after 45s`, a different class from the
captured `terminated signal`. Per the acceptance wording a different endpoint
error is NOT REPRODUCED. A single last request is not the tool loop, so even a
reproduction would not have shown the divergence. No minimum or passing change
exists. No repair, fail-before/pass-after test or authoring demonstration is
offered.

## Hypotheses (unconfirmed)

1. **Slow model rounds under the caller's wall-clock deadline.** Few seconds of
   tool time over two hours leaves the time in model requests or queueing. Final
   waits of 8 m 10 s and 26 s on large requests are compatible with this but do
   not show causal model latency.
2. **Provider/runtime/queueing delay** outside OpenExec's visible counters.
3. **Model never choosing to author** despite advertised tools (ordering of
   early requests unknown).

Hypothesis 3 cannot be separated from 1 and 2 with last-request captures.
`budget_exhausted` is produced by the caller's context cause, not by OpenExec
(see `QWEN_BUDGET_EXHAUSTION_DIAGNOSIS.md`); the capture/terminal class mismatch
fits caller cancellation of an in-flight request, which is also a hypothesis.

## Missing evidence and next route

Gap: the untruncated per-round request/response sequence with timestamps (which
tools each round advertised, when each round started and returned, which served
model identity answered). Without it, divergence from intended execution cannot
be placed.

Smallest next evidence route: for the next run of this signature, obtain a
full `.trace.json` through whatever existing owner-authorized diagnostics route
the owner already holds, then compare per-round durations and the first round
at which a write-capable tool was both advertised and unused. No new runtime
recorder, controller, timeout or cap is proposed. Do not re-admit an unchanged
exhausted parent; its split children and accounting are untouched.
