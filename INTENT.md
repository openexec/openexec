# AI Project Operating System

## Vision

A conversational, tool-using OS for software projects. Engineers use a chat-first interface (like Claude Code) to plan, code, run, and fix projects—including improving the orchestrator itself—safely and repeatably.

## Goals

- Multi-project chat with persistent sessions bound to workspaces.
- Provider-agnostic agent loop (OpenAI/Gemini/Anthropic) with model picker.
- Tool-use via MCP (filesystem, shell, git) with explicit approvals and audit.
- Auto-context injection (INTENT.md, tasks.json, recent logs).
- Self-healing/meta: agent can locate, edit, build, and request restart of Orchestrator.

## Pillars

- Interactive UI: Extend claudecodeui (chat history, streaming markdown, spinners).
- Tool-Calling Loop: Function-calling → MCP tools; provider adapters.
- Auto-Context: Inject per-turn project brain (intent, task state, log tail).
- Meta-Engineering: Workspace-aware edits + build/restart prompts.

## Non-Goals

- Full IDE replacement; binary debugging; unrestricted exec without approval.

## Constraints

- All write/exec goes through approval; paths validated within WORKSPACES_ROOT.
- Secrets redacted; audit logs persisted.

## Deliverables

- Sessions DB (SQLite): sessions, messages, tool_calls, project_path, provider, model.
- Provider drivers (OpenAI/Gemini) with streaming and tool schema translation.
- MCP tools: read_file, write_file, run_shell_command, git_apply_patch.
- UI features: model picker, session fork, patch preview/apply/rollback, and cost/token dashboard.
- Advanced Engine: Automated session history summarization and cost/budget visibility.
- Docs: Updated Orchestrator README and documentation declaring conversational orchestration as a built-in feature.

## Phases

1. Read-only chat with auto-context. 
2. Tool-enabled with approvals. 
3. Multi-provider forking, history summarization, and cost/limits UI. 
4. Meta self-fix pathways and final documentation migration.

## Acceptance Criteria

- Create/resume/fork sessions per project; pick provider/model.
- Tool-calls execute via MCP with audit records and redaction.
- Auto-context present and truncated safely; logs accurate.
- Self-fix demo: agent edits orchestrator file, builds binary, requests restart.
