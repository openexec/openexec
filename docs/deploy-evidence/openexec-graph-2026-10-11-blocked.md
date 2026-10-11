# openexec-graph deploy of 83d1cab0 — blocked (2026-10-11)

Task: "Outcome not met: openexec-graph still serves the 09-28 build after #106 —
run the reviewed deploy from main". Target: install origin/main `83d1cab0`
(#106 merge, contains #103 `b418f957` and #104 `ffa361e1`) via
`scripts/reviewed-deploy-graph.sh`, then record `journey.json`/`requery.json`.

**Not deployed. No journey record exists.** Every execution route offered to
this unattended, server-minted turn refused before the script could run:

| Route | Call | Exact refusal |
|---|---|---|
| Generic host command | `run_command ["git","worktree","add","--detach","/tmp/openexec-graph-deploy-83d1cab0","83d1cab0"]` | `Refused: server-minted or provenance-unknown turns cannot run generic host commands. Use the provider sandbox for routine work and typed reviewed-feature actions for merge or deployment. A lane task's pass runs a check the sandbox cannot do (a stack, host ports, Docker) with run_declared_check, in this task's worktree — call it with an unknown name to list what this repository declares, and report its output as the evidence.` |
| Declared host check | `run_declared_check list` | `Refused: "list" is not a declared check for openexec/openexec. Declared: lint, test.` (no deploy check is declared) |
| Provider sandbox | `Bash` (systemctl status, health curl, `openexec version`) | `Refused: server-minted or provenance-unknown turns cannot acquire generic host execution. Use the provider sandbox for routine work and the typed reviewed-feature actions for merge or deployment.` |
| Typed reviewed deploy | `deploy_reviewed_feature feature_key=e917b46291b289d67f626d4fc52fa921` | `Refused: not found.` (this task carries no server-minted feature key for #106) |
| Missing-binding observation | `observe_current_outcome` | `Source observation refused: current invocation, source, scope or execution boundary is not eligible. No acceptance or selection was changed.` |

The earlier verification pass also reported, from its own sandbox:
`systemctl --user status openexec-graph` → `Failed to connect to bus: No medium found`.

## What unblocks it

One of:

1. `deploy_reviewed_feature` invoked with the server-minted feature key of the
   #106 feature task (the owner profile
   `AGENT_CONSOLE_REVIEWED_DEPLOYMENTS={"openexec/openexec":["bash","scripts/reviewed-deploy-graph.sh"]}`
   then runs the script in a clean exact-merge worktree), or
2. a host-execution-capable turn running, from a detached clean checkout of
   origin/main: `bash scripts/reviewed-deploy-graph.sh`.

Done when `GET 127.0.0.1:8765/api/health` reports `"commit":"83d1cab0"` (or the
then-current origin/main short SHA) and
`~/.agent-console/deploy-evidence/openexec-graph/<commit>-<ts>/{journey,requery}.json`
exist and are copied here.
