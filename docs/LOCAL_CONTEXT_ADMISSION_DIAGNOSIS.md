# Local context admission diagnosis

The 2026-11-02 Solar failure reported 14,229 request bytes, an estimated
6,318 tokens and context 4,096. In the #196 baseline, a 7,001-token remaining
grant selects 4,096: admission reserves at least 2,048 output tokens and rounds
the available context down to a power of two. The failure-shaped fixture in
`pkg/execution/context_grant_regression_test.go` exercises the native serializer.
This is a possible route to the signature, not proof of the historical cause:
the run's remaining grant, request override and effective provider configuration
were not recovered. The dispatched occurrence also cannot be explained by this
pre-dispatch fixture alone. The Console native Goal path inspected during triage
does not set an override. #196 added checkpointing but retained this selection.

The provider window, the 65,536 harness ceiling, an explicit request ceiling,
and the cumulative run grant are separate limits. A 7,001 grant cannot reserve
the fixture's estimated prompt and the minimum output without exceeding the
grant. A larger grant can still be stranded by power-of-two rounding; admission
now retries a pre-dispatch refusal with the required context within the original
limits. An undersized binding provider `context_tokens` is diagnosed when the
local model's independently reported metadata confirms a much larger window.
Unavailable or ambiguous metadata does not establish a misconfiguration.
