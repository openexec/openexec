# Security Audit — Action Items

> Audit date: 2026-06-11. Scope: command/process execution, HTTP/WebSocket API, secrets,
> Telegram/MCP untrusted-input surfaces, React UI + dependencies.
>
> Context: OpenExec is a local-first AI-agent daemon that executes commands and edits files by
> design. Many "no auth" gaps are tolerable for a single trusted localhost user, but become
> **unauthenticated RCE** once reachable over the network (`docker-compose.yml` exposes `0.0.0.0:8080`).
> The most urgent localhost-only risk is a malicious web page driving the daemon (C2).
>
> Confidence: C1–C4 were re-read and verified against source. C5 and the H-items are from review
> with file:line citations, spot-checked but not exhaustively re-verified.

## Fix order (highest priority first)
1. C2, C4, C3 — exploitable on a default localhost install
2. C1 — add auth / loopback bind before any network exposure
3. C5, H5 — mandatory Telegram auth; allowlist MCP binaries
4. H1–H4 — stop `sh -c` on LLM/config strings, filter env, wire approval gate
5. H6 + Medium/Low — timeouts, rate limiting, binaries out of git

---

## Critical

- [ ] **C1 — API has no authentication or authorization.** `internal/server/server.go:589` wraps the
  mux only in `loggingMiddleware`. All endpoints open: `POST /api/v1/runs`, `.../start`,
  `.../resume`, session create/delete, approval `approve`/`reject` (self-approval possible). No TLS
  (`ListenAndServe`, never `...TLS`). Default bind is all interfaces (`Addr: ":<port>"`); compose maps
  `8080:8080`. **Fix:** mandatory shared-secret/bearer token; bind `127.0.0.1` by default; add TLS option.

- [ ] **C2 — WebSocket origin check bypassable (malicious site → daemon).** `pkg/api/ws.go:21-45` uses
  `strings.Contains(origin, "localhost")`, so `Origin: http://localhost.attacker.com` passes; empty/`null`
  origin also allowed. A visited web page can open `ws://localhost:8080/ws` and send
  `subscribe`/`pause`/`stop`/`resume` on arbitrary session IDs (`ws.go:231-237`, no ownership check).
  DNS-rebinding extends to REST (no CSRF tokens, no `SameSite`). **Fix:** exact-match origins against an
  allowlist; don't blanket-allow empty origin on a network listener.

- [ ] **C3 — Unguarded path traversal + `sh -c` in API tool handler.** `internal/loop/api_runner.go:328-333`
  `resolvePath` returns absolute paths as-is and does not reject `../`. Feeds `read_file`, `write_file`,
  `git_apply_patch`, and `run_shell_command` (`api_runner.go:289` = `sh -c req.Command`). Note: the hardened
  validator at `internal/mcp/path.go` guards a *different* path and is NOT used here. **Fix:** jail under a
  workspace root with `filepath.Clean` + prefix check; reject absolute paths; reuse `internal/mcp/path.go`.

- [ ] **C4 — Directory-listing endpoint walks whole filesystem.** `pkg/api/files.go:14-29` reads the raw
  `path` query param via `os.ReadDir` (code comment admits it doesn't enforce a root).
  `GET /api/directories?path=/etc` enumerates anything. Unauthenticated (see C1). **Fix:** confine to
  `ProjectsDir` with clean + prefix check.

- [ ] **C5 — Telegram auth optional / missing on some handlers.** `internal/telegram/webhook.go:74-82`
  checks auth only `if h.authMiddleware != nil`. `handleWizardInput` (`commands.go:1330`) gates only on
  `HasActiveWizard(userID)`, not the allowlist. **Fix:** make auth mandatory (fail construction without it);
  re-check allowlist inside every handler.

## High

- [ ] **H1 — Shell allowlist ineffective.** `internal/mcp/broker.go:195-234` allowlists base commands but
  includes `bash`/`sh`/`awk` (arbitrary code via their own args); fully bypassed in `ModeFullAuto`.
  **Fix:** remove interpreters from allowlist, or drop the allowlist as a security control.

- [ ] **H2 — `run_shell_command` skips approval gate.** `handleRunShellCommand` (`internal/mcp/server.go:~821`)
  runs `bash -c` without `RequestToolApproval`, unlike infra apply (`internal/mcp/infra.go:251` fails closed).
  Approval gate is also fail-open when unconfigured (`approval_gate.go`). **Fix:** call the gate; fail closed
  in Task mode when no gate is configured.

- [ ] **H3 — Full environment inherited by spawned shells.** `cmd.Env = os.Environ()` in
  `blueprint/executor.go:215`, `mcp/server.go`, `loop/process.go` — agent shells can read API keys/creds via
  `env`. **Fix:** pass a filtered env allowlist to child processes.

- [ ] **H4 — Shell injection via stored/config command strings.** `bash -c <string>` sinks fed by
  release story `VerificationScript` (`cli/release.go:1627`), gate `Command` from `openexec.yaml`
  (`execution/gates/runner.go:126`), knowledge-store `DeploySteps` (`tools/deploy_tool.go:76`).
  **Fix:** use argv arrays instead of shell strings.

- [ ] **H5 — MCP server `command` executed unvalidated.** `mcp.json` entries spawn `command`+`args` via
  `exec` (no shell — good) but the binary path isn't allowlisted; a poisoned `.openexec/mcp.json` runs an
  arbitrary executable. **Fix:** allowlist known server binaries; require absolute path + exists/executable check.

- [ ] **H6 — No request timeouts / body-size limits / rate limiting.** `http.Server` built without
  `ReadTimeout`/`WriteTimeout`/`MaxHeaderBytes` (`server.go:222`); "max 10 parallel runs" is per-request only.
  **Fix:** set server timeouts, body-size limit, and per-IP/token rate limiting.

## Medium / Low

- [ ] **M1 — Committed platform binaries in `bin/`** (~20 MB each + `axon`, `openexec-engine`, …) — supply-chain
  hygiene. Ship via signed releases with checksums; gitignore `bin/`. Remove stray `bin/.!77512!openexec`.
- [ ] **M2 — Service worker caches `/api/*`** network-first without per-user partitioning (`ui/src/sw.ts`) —
  cross-user leak on shared devices. Disable API caching or partition per user.
- [ ] **M3 — npm dev-toolchain advisories** (esbuild dev-server SSRF, picomatch ReDoS, postcss, ws).
  `npm audit --omit=dev` = 0 in prod bundle. Run `npm audit fix`; no prod impact.
- [ ] **M4 — `/api/health` discloses runner command/model** (`server.go:375-405`) — minor recon aid.
- [ ] **M5 — `context.Background()` (no timeout) on Telegram auth checks** — DoS amplifier if user store hangs.
- [ ] **M6 — Tracked SQLite `*.db-wal`/`*.db-shm`** under `code/tract-home/stores/uaos/` — `git rm --cached`.

---

## Verified-good (no action needed)
- UI XSS: clean — no `dangerouslySetInnerHTML`/raw-HTML markdown; LLM output rendered as React text nodes.
- No committed live secrets; `.gitignore` covers `.env`, `.openexec/`, `*.db`, `*.log`; compose uses `${...}` expansion.
- SQL is parameterized (`internal/knowledge/store.go`, `pkg/db/...`).
- Secret-redaction layer (`internal/mcp/redact.go`, `pkg/telemetry/redact.go`), 34+ patterns.
- Correct deny-by-default model exists for infra tools (`internal/infra`) and one file-tool path
  (`internal/mcp/path.go`) — gaps are non-uniform application, not absence of the pattern.
