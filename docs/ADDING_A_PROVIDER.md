# Adding a provider

A one-line request ("add GitHub Copilot as a provider") hides a fork that must
be decided before any code: **which route does the provider take?** Guessing
the route (an OpenAI-compatible URL with a static key and model IDs from
memory) produced openexec#200. Write the brief below first; raise one owner
decision only when step 1 has a real fork.

## 1. Choose the route (the fork)

| Route | Use when | Where it lives |
| --- | --- | --- |
| CLI host provider | The vendor ships an authenticated CLI (claude, codex, gemini) and its login, token refresh and model list belong to that CLI | `pkg/execution/agent_cli_provider.go` (`AgentCLIConfig`, `arguments`, `parseCLIEvent`), CLI choice in `internal/cli/init.go`, readiness hint in `internal/cli/doctor.go` |
| Direct API provider | The vendor has a documented HTTP API with a stable credential | `pkg/agent/<name>_provider.go` implementing `ProviderAdapter`, registered in `pkg/agent/registry.go` (`ProviderName`, factory) |
| API with a token exchange | A long-lived credential is traded for short-lived tokens (for example an OAuth device flow, then a session token) | the direct route plus an explicit refresh step, expiry handling and a readiness probe that distinguishes missing login from expired token |

State the recommendation and why. The choices differ in who owns login, token
storage, rate limits and terms of service, so a documented-API route is not
assumed to exist: confirm the vendor documents it for this use.

## 2. Evidence to collect, not recall

- The vendor's current endpoint, auth scheme and supported-client terms (link them in the PR).
- Model IDs from the vendor's models endpoint or documentation, never from memory. `pkg/agent/models.go` holds the catalogue and families; `internal/cli/init.go` the init choices.
- The context window the model really admits per request. Set `context_tokens` in the provider config; admission (`docs/HARD_TOKEN_ADMISSION.md`, `pkg/agent/context_admission.go`) refuses what does not fit.

## 3. Implementation checklist

1. `ProviderName` constant and factory in `pkg/agent/registry.go` (API route), or CLI config in `pkg/execution/agent_cli_provider.go` (CLI route).
2. Provider adapter: request mapping, streaming, tool-call schema (`pkg/agent/tool_schema.go`), usage and cost reporting, a hard token grant honoured (`docs/EXECUTION_PROVIDER_CONTRACT.md`).
3. Models and families in `pkg/agent/models.go`; telemetry attribute in `pkg/telemetry/genai.go`.
4. `Probe`/readiness (`pkg/agent/readiness.go`): a bounded, real check that tells apart missing runtime, missing login, expired token and unreachable endpoint. No inference needed to report "not logged in".
5. Config and secrets: key or token location in `internal/project/project.go` config, never logged; `doctor` message that names the one command that fixes the failure.
6. Sandbox: reject a mode the provider cannot enforce; read-only carries no writable roots.
7. Tests against a fake server covering success, auth failure, expired token, rate limit, truncated stream and cancellation. A stub that only echoes proves nothing about the route.
8. Docs: add the provider to `docs/CONFIGURATION.md`.

## 4. The brief (what a task-intake pass produces)

Route and recommendation; acceptance checks (each runnable); risks (terms,
credentials, model IDs, rate limits); size estimate. If the estimate does not
fit one run, split it: readiness and config first, then the adapter, then
models and tests, each its own task with one output.
