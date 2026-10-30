# Hard token admission

A nonzero execution `TokenBudget` is a cumulative input-plus-output maximum,
not an estimate or an instruction to the model. CLI adapters cannot satisfy it
from usage reported after a turn. The API executor refuses unsupported adapters
before inference.

The configured loopback Ollama 0.32.13 route negotiates its native protocol via
`/api/version`, without inference. Unknown versions and hosted endpoints do not
claim this capability. Each `/api/chat` call carries explicit positive `num_ctx`
and `num_predict` bounds whose sum fits the remaining grant, with truncation
turned off. The model's template and tools are included by the native server;
media is refused by this bounded route. The executor debits the worst case before
the call, including empty-response retries and final synthesis. Missing or invalid
usage retains that reservation. Complete native prompt/output counts release only
unused capacity. Usage events contain observed cumulative counts. Unknown usage leaves the final
flag false so Agent Console retains the full run reservation. The controller
refuses another inference below a 4096-token grant, preserving the native
2048-token minimum context even for vision-capable models receiving text.

The reviewed native protocol is Ollama v0.32.13:
- https://github.com/ollama/ollama/blob/v0.32.13/server/prompt.go
- https://github.com/ollama/ollama/blob/v0.32.13/llm/server.go

Compatibility is negotiated rather than guessed from an OpenAI-compatible URL.
The same provider's ordinary unbounded execution retains its existing API path.
No API credentials, model configuration or native CLI session are transferred.

A live test-only binary exercised the configured `qwen-27b` endpoint with an
8192-token grant on 2026-09-08. Negotiation returned `hard_token_budget: true`;
the response was `ready`, with 412 input and 21 output tokens. This proves the
native execution path, not Agent Console deployment or generation-24 acceptance.

Live refusal checks returned HTTP 400 for 8052 input tokens against a 2048
context bound, and exactly 8 output tokens with done_reason length for an
8-token output cap. These probes do not establish generation-24 acceptance.

## Shrinking-context repair (2026-09-10)

Run `8fdc19bbf162a4da425eacf00d001b71` reserved 65,536 tokens. Two
requests used 20,700 + 1,298 and 23,416 + 2,048 tokens, totaling 47,462.
The next admission reduced `num_ctx` to 16,026; the native server instantiated
16,128 and rejected the 23,462-token prompt at 2026-09-08T11:32:11Z.
The user-service Ollama journal retains that diagnostic. The adapter discarded
the HTTP response body; this is not a recovered response-body receipt.

The executor now retains one context allowance throughout the run, selected
from 2,048 through 32,768 in powers of two within its initial reservation,
with a 2,048-token output allowance. If the remaining capacity cannot reserve
that same allowance, execution yields before HTTP with finalized known usage.
The existing Console per-run yield can schedule a separately reserved successor
within the unchanged generation. No prompt is truncated and no grant is enlarged.
Unknown provider failures still retain uncertain reservations.

The historical-sequence regression proves two requests, 47,462 finalized tokens
and no third request, including the empty-response recovery path. Removing the
admission guard makes it fail. Agent and execution package suites passed.
This is local repair evidence, not deployment or a new successful live request.

## Reply room for native planning

Thinking models share their output allowance between reasoning and the visible
answer. A fixed 2,048-token allowance caused native planning to return reasoning
without a task plan; the one empty-answer recovery then repeated the same cap.

The bounded executor now reserves output up to its admitted context ceiling
(currently at most 65,536 tokens), within the original remaining total grant.
An explicit request `MaxTokens` remains a restrictive ceiling. Native context
preflight reduces output to leave room for the complete assembled prompt before
any inference is submitted. The input context stays stable, and only unused
capacity from an unsubmitted preparation is returned. A dispatched refusal is
never retried or refunded by this preparation step. Exact duplicate-result
compaction keeps the same admission checks.

The execution tests cover a native task-plan response after more than 2,048
output tokens, caller limits, scarce remaining capacity, stable input context,
unsubmitted preparation refunds and retained reservations after dispatched
failures. These controls establish bounded allocation; a production Goal still
requires its own persisted task plan and independent completion evidence.
