# OpenExec — Intent

This is the working PRD/intent file for the OpenExec system. Planning tools can consume it directly from the project folder.

## Overview
OpenExec provides a modular, production-ready path from PRD to verified code: planning, execution, quality gates, human approvals, and live observability.

## Goals
1. Plan from PRD into a Goal Tree and FWUs, then schedule execution.
2. Execute FWUs with an auditable loop and live status (SSE).
3. Enforce quality via stack-aware gates (Python/JS/Go/Rust) with structured results.
4. Support approvals via Telegram/WhatsApp webhooks and apply decisions to execution.
5. Provide a unified CLI (init/plan/start/status/tui) and a web dashboard for observability.
6. Keep an immutable audit trail with optional encryption and export.

## Constraints
- Security: audit writes are non-bypassable; failures halt execution.
- Portability: local dev via compose or binaries; no hard cloud dependencies.
- Time-to-value: prioritize working solutions over large rewrites.

## Feature: Orchestration Plan CLI
- CLI: `openexec plan <intent>` prints JSON summary and writes tasks.
- Supports ordering and timeout_mode parameters.

## Feature: Execution SSE + Non-Bypassable Audit
- Endpoint `GET /events` with heartbeats; iteration and phase events.
- Audit write errors stop the run with explicit error.
- Flags: `--harness`, `--provider`, `--model`.

## Feature: HITL Webhooks (Interface)
- Routes: `/webhook/telegram` (secret token), `/webhook/whatsapp` (Twilio signature).
- Approve/Reject/Pause map to execution API.

## Feature: OpenExec CLI (init/plan/start/status/tui)
- `openexec init` creates `.openexec` and initializes stores.
- `openexec plan` invokes orchestration plan CLI and outputs summary.
- `openexec start` launches execution with flags.
- `openexec status` subscribes to SSE and prints concise status.
- `openexec tui` shows live dashboard.

## Feature: Web Dashboard (SSE)
- Next.js dashboard shows projects, phases, workers, and recent audit events via SSE.
- Config from `.env.local` for backend endpoints.
