# openexec-graph deployed from main: ef828b15

`scripts/reviewed-deploy-graph.sh` was run from a clean, detached `origin/main`
worktree at ef828b15 on 2026-10-11 at 15:52Z. It built and installed the
binary, restarted `openexec-graph.service`, walked the authenticated journey,
restarted again and re-queried. `/api/health` on :8765 reports commit
`ef828b15` (version `0.13.1-153-gef828b15`).

The first attempt from 83d1cab0 refused before installing anything: the
script read `openexec version` from stdout, but the CLI prints it to stderr.
That fix is #108 (ef828b15).

Graph version `graph_a35cff1c-b215-41f7-8630-bfe7fda3fef3`, symbol `CanExecute`, source sha256
`54aa314419b0116b95289fd4a9ee98ef165493891bf3e1f57928e1a0285142c8`. Source bodies are recorded as digests only.

## Journey (2026-10-11T15:52:15.753041228Z)

| Step | Path | Status | Wanted |
|---|---|---|---|
| running build | `/api/health` | 200 | 200 |
| repository context | `/api/v1/repository-context` | 200 | 200 |
| search | `/api/v1/repository-graph/symbols?page_size=5&q=Execute` | 200 | 200 |
| select | `/api/v1/repository-graph/symbols/sym_93594c39-8cdd-4424-bf11-8f516a8c5` | 200 | 200 |
| calls outgoing | `/api/v1/repository-graph/symbols/sym_93594c39-8cdd-4424-bf11-8f516a8c5` | 200 | 200 |
| calls incoming | `/api/v1/repository-graph/symbols/sym_93594c39-8cdd-4424-bf11-8f516a8c5` | 200 | 200 |
| dependencies | `/api/v1/repository-graph/symbols/sym_93594c39-8cdd-4424-bf11-8f516a8c5` | 200 | 200 |
| impact | `/api/v1/repository-graph/symbols/sym_93594c39-8cdd-4424-bf11-8f516a8c5` | 200 | 200 |
| source | `/api/v1/repository-graph/symbols/sym_93594c39-8cdd-4424-bf11-8f516a8c5` | 200 | 200 |
| refuse missing bearer | `/api/v1/repository-graph/symbols/sym_93594c39-8cdd-4424-bf11-8f516a8c5` | 401 | 401 |
| refuse wrong bearer | `/api/v1/repository-graph/symbols/sym_93594c39-8cdd-4424-bf11-8f516a8c5` | 401 | 401 |
| refuse other checkout | `/api/v1/repository-graph/symbols/sym_93594c39-8cdd-4424-bf11-8f516a8c5` | 403 | 403 |
| refuse missing checkout | `/api/v1/repository-graph/symbols/sym_93594c39-8cdd-4424-bf11-8f516a8c5` | 403 | 403 |
| refuse fabricated symbol | `/api/v1/repository-graph/symbols/fabricated:..%2F..%2Fetc%2Fpasswd/sou` | 404 | 404 |
| refuse invalid direction | `/api/v1/repository-graph/symbols/sym_93594c39-8cdd-4424-bf11-8f516a8c5` | 400 | 400 |
| refuse malformed depth | `/api/v1/repository-graph/symbols/sym_93594c39-8cdd-4424-bf11-8f516a8c5` | 400 | 400 |

## Re-query after restart (2026-10-11T15:52:22.124555485Z)

| Step | Path | Status | Wanted |
|---|---|---|---|
| running build | `/api/health` | 200 | 200 |
| requery source | `/api/v1/repository-graph/symbols/sym_93594c39-8cdd-4424-bf11-8f516a8c5` | 200 | 0 |
