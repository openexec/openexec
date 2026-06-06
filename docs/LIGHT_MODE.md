# Light Mode: working the backlog from Claude Code

OpenExec is a two-speed system:

- **Heavy mode** (`openexec run`, the daemon): turns intent into a backlog of
  stories and tasks, executes blueprint pipelines, enforces gates, records
  audit state.
- **Light mode** (this document): an ephemeral, interactive session — e.g.
  Claude Code — that reads that backlog over MCP and works it **one story at
  a time**, without booting the daemon.

The seam between the two speeds is the story backlog in
`.openexec/openexec.db`. Heavy mode writes it; light mode claims and
completes against it.

## Setup

Add OpenExec as an MCP server in your project's `.mcp.json` (Claude Code):

```json
{
  "mcpServers": {
    "openexec": {
      "command": "openexec",
      "args": ["mcp-serve"]
    }
  }
}
```

`mcp-serve` is a plain stdio process scoped to the current directory. It does
**not** start the daemon, bind ports, or hold locks at startup — the backlog
database is opened lazily on first tool use.

## Tools

| Tool | Side effects | Purpose |
|------|--------------|---------|
| `backlog_list_stories` | none | See what work exists (optionally filtered by status) |
| `backlog_get_story` | none | Full story: tasks, acceptance criteria, verification scripts, afk/hitl modes |
| `backlog_claim_story` | backlog state | Mark a story in_progress. **One story at a time** — refused while another story is in progress |
| `backlog_complete_task` | backlog state | Mark a task done after its verification passes |
| `backlog_complete_story` | backlog state | Mark a story done. Refused while tasks remain unfinished |
| `backlog_add_task` | backlog state | File surgical work into the rolling maintenance story so light-mode fixes leave a record. Phase-neutral; default mode hitl |
| `memory_read` | none | OpenExec's merged layered memory (decisions, patterns, preferences from prior runs) |
| `skill_propose` | candidate file | Capture a durable project lesson as a **candidate** skill. Never active until a human runs `openexec skills approve <name>` |

## Semantics worth knowing

- **Backlog writes are allowed in every permission mode, including read-only
  chat.** They mutate orchestrator bookkeeping (the `.openexec` database),
  not workspace files. This is a deliberate, documented exception to "chat
  has no side effects" — managing the backlog is the light client's job.
- **Claiming is advisory coordination, not a lock.** The backlog database is
  shared (WAL mode with a 5s busy timeout — enforced by a regression test);
  if the daemon is also executing work, coordinate through story status. The
  one-story rule is enforced at claim time against current database state.
- **Do not run heavy and light edits simultaneously on the same working
  tree.** Backlog state is safe to share (WAL serializes the bookkeeping),
  but both lanes edit the same source files: concurrent file edits from the
  daemon and a light client can conflict. The intended lifecycle is
  sequential — heavy run finishes, then light mode works; heavy parallel
  runs isolate themselves in git worktrees, interactive light sessions do
  not.
- **State is never cached stale.** Every backlog tool call reloads from the
  database, so changes made by the daemon, `openexec run`, or another client
  are visible immediately.
- An uninitialized workspace (no `.openexec/` directory) lists as an empty
  backlog; mutating tools explain that no project exists yet.
- **`backlog_list_stories` reports the project phase** (`new` → `planned` →
  `building` → `maintaining`). Once the phase is `maintaining` — the initial
  heavy build has worked off the backlog — light mode is the default lane;
  reach for `openexec run` only when the next big feature or refactor needs
  the full pipeline.

## The once-heavy-then-light lifecycle

Per project, the heavy pipeline typically runs **once** — a greenfield build,
or one refactoring pass on a freshly cloned codebase — and light mode is the
default forever after:

1. **Heavy run** — `openexec plan` + `openexec run`. The study/terminus
   stories write `docs/ARCHITECTURE.md` so the run's understanding is durable
   and readable here (the knowledge base alone is not readable outside
   OpenExec).
2. **The hitl handoff** — the heavy pipeline never auto-runs hitl tasks
   (manual QA is always hitl), so the run ends with them pending and the
   phase stuck in `building`. The backlog listing shows `hitl_pending` and
   tells you: claim the story, do the work, `backlog_complete_task` each one.
   Then the phase reaches `maintaining`.
3. **Light mode forever** — surgical fixes here. File them with
   `backlog_add_task` so they stay on the record (the maintenance story is
   phase-neutral and never blocks claims). Capture lessons with
   `skill_propose`; promote the cross-project ones with
   `openexec skills promote <name>` so the next project's heavy run starts
   smarter.
4. **Next epic** — run planning again; colliding IDs remap and the new
   stories append (`docs/CLAUDE.md`, re-planning).

## Typical light-mode session

1. `backlog_list_stories` (status=pending) — see what is queued
2. `backlog_get_story` — read tasks, modes, and verification scripts
3. `backlog_claim_story` — take it (one at a time)
4. Implement task by task with the client's own tools; run each task's
   `verification_script`; `backlog_complete_task` as each one passes
5. `backlog_complete_story` when acceptance criteria are met
6. hitl-tagged tasks are yours by definition — do them, don't delegate them
7. Learned a durable project quirk along the way? `skill_propose` it, then
   review with `openexec skills proposals` and approve or reject. Proposals
   never activate themselves — that is the point.
