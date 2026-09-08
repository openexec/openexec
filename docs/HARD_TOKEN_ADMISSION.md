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
