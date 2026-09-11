# Native context observation: bounded recovery repair

## Observed failure and owning route

Agent Console #166 is deployed as `9116ab7`; OpenExec remains `d4cd6dc`.
The ordinary Fresh run `4b9dcfbd48296fdf09452c6b545b84e0` used the typed
ExecutionPacket and its first tool was `get_closure_record` for the assigned
task `c9ea3388f27c4a2dfd4b05ff776eee6e`. It did not orient through files or chat.

After retrieval the next inference was refused locally:
`context_overflow: assembled request bytes=73636 estimated_tokens=31849 context=32768 dispatched=false`.
The configured output allowance is 2,048. The 31,849 figure is an estimate,
not native token usage. The first completion's reported cumulative native usage
was 18,288 tokens; the refused request added none. No task advancement or
automatic successor was observed during the bounded 90-second observation.
This is not successful disposable-worker recovery.

Receipts remain in `/mnt/data1/work-owned-166`; no transcript contents or
credentials are copied into this review note. Grant 27 is separate and
non-renewing; this repair changes no grant limits, reservations or expiry.

## Smallest candidate and affected scope

The first request still uses the existing padded whole-wire estimate. Following
a successful response with validated native usage, retain only hashes of that
request's fixed shape and message prefix, plus its native input-token count.
For an append-only next request, estimate the unchanged prefix from that observed
count and charge every added serialized byte as a token, plus framing slack.
Changed model, options, tools, system or earlier messages invalidate reuse.

This improves a preflight estimate; it does not establish a mathematically exact
bound for every native template. Native context/output limits, `truncate=false`,
overflow refusal and cumulative reservation/usage enforcement remain unchanged.
All thinking, instructions, tool calls and tool results are preserved. Ollama's
[tool-calling contract](https://docs.ollama.com/capabilities/tool-calling) requires
returning the response fields with tool results; dropping them is not this fix.

There is no persisted context cache, task database, provider fallback or new
authorization mechanism. The observation is disposable provider-local metadata.
Project loading, legacy workspaces, `.openexec`/`.uaos` formats and migration
behavior are unchanged. The OpenExec #48 execution lineage owns this repair.

## Completeness checks and falsifiers

Related paths checked: initial unobserved request, changed request identity,
oversized added messages, successful native protocol reporting, existing malformed
usage refusal, context-overflow propagation and hard-token accounting. Hashes are
immutable; the observation pointer is protected by a mutex.

Focused observation, context, native-budget and execution-token tests passed.
Disabling observation reuse makes the representative second-request test fail.
Disabling the shape/prefix checks makes model, prefix, system, tools and options
negative cases fail. Both mutations were restored and positive tests passed.

The full `go test ./...` suite passed (job `native-context-observation-go`), as
did focused race tests, package vet and CLI build. An independent reviewer
found no blocking issue in this delta, explicitly retaining
the limitation that estimation is not a proof of native fit. Exact committed
review and live second-inference evidence remain necessary.

## Remaining route

After guarded deployment, repeat ordinary Fresh with the existing grant, not a
manually coached prompt. Verify native second-inference success and useful task
state. Then prove automatic replacement and worker/process loss with the same
durable assignment. A provider success is not task completion. The later
structured-result/controller-continuation dependency remains open; this repair
does not claim to implement it or make the broader product Ready.
