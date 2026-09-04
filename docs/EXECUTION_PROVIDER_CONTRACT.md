# Execution provider contract

**Status:** built (`pkg/execution/`). Normative for callers of that package.

`pkg/execution.Provider` is the public boundary for authorized local or API
execution. Product callers own authorization and workflow; OpenExec owns
runtime construction, containment validation, normalized events, and result
provenance.

## Required transitions

| From | Operation | To | Contract |
| --- | --- | --- | --- |
| unknown | `Probe` | ready/unavailable | A real bounded probe distinguishes missing runtime, authentication, and health failures |
| ready | `Execute` | running | Request ID, canonical working directory, prompt, model, sandbox, and writable roots are validated before starting |
| running | provider output | running | Output is emitted as normalized ordered events |
| running | context cancellation | cancelled | Runtime terminates and returns a cancelled result |
| running | success | succeeded | Final text and native session identity are preserved |
| running | failure | failed | A provenance result and actionable error are both returned |

An implementation must reject a sandbox mode it cannot enforce. Read-only
requests cannot carry writable roots. Workspace-write roots must be absolute.

## Protocol boundary

`openexec execution-stdio` exposes protocol version 1 as JSON Lines. The
command is hidden from interactive help because it is a machine boundary.

Input is one envelope:

```json
{"version":1,"operation":"execute","request":{...}}
```

Output contains zero or more event envelopes followed by exactly one result:

```json
{"version":1,"operation":"event","event":{"type":"assistant.delta","text":"..."}}
{"version":1,"operation":"result","result":{...}}
```

`describe` and `probe` use the same versioned envelope. Consumers must reject
unknown protocol versions rather than guessing field compatibility.

## Acceptance evidence

- Claude resume always sends an explicit `plan` or `acceptEdits` permission.
- Codex always receives an explicit sandbox and bounded writable roots.
- Neither provider uses a dangerous permission bypass.
- Streaming preserves native session identity and final text.
- Context cancellation produces `cancelled`, not a generic failure.
- Authentication output is classified as `needs-login`.
- The stdio protocol rejects version drift and preserves event order.

`APIProvider` adapts OpenAI-compatible and other `pkg/agent` adapters. It
streams text-only turns and runs bounded multi-step tool turns. A tool executor
must validate the working directory, sandbox, and writable roots before the
first model request; merely supporting model tool calls does not enable
workspace-write.
