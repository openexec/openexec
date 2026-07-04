# Multi-Project Chat Runtime TODO

## Goal

OpenExec should become a multi-project AI execution control plane that can be launched from a workspace such as `/Users/perttu/projects` and used to inspect, chat with, and safely modify any project under it.

The desired user experience:

```bash
openexec workspace start /Users/perttu/projects
```

From there, OpenExec discovers projects, tracks their state, lets the user chat with each codebase, performs surgical fixes when safe, and escalates to a full blueprint pipeline only when the task requires it. The goal is to replace ten project terminals with one workspace console.

Core rule:

```text
Use the lightest safe process.
Escalate only when risk or uncertainty requires it.
Never make the user pay full-pipeline cost for a surgical change.
```

## Related Inputs

- `openexec`: runtime, CLI, daemon, MCP tools, policy, state, gates, and UI embedding.
- `blueprints`: first-run project generation and product-category contracts. Its `seeds/` and generated YAML blueprints should guide new project scaffolding, acceptance contracts, and feature completeness tests.
- `openexec-web`: product and UI direction for the visual control plane, especially the two-speed model: light interactive work plus heavy verified blueprint execution.

## P0: Align Implementation With Architecture

1. Create a feature registry.
   - Mark each subsystem as `stable`, `beta`, `experimental`, `planned`, or `dead`.
   - Cover DCP, BitNet, checkpointing, memory, predictive loading, quality gates, SRE tools, MCP tools, UI, TUI, and multi-agent execution.
   - Use the registry in CLI help, docs, web UI, and config validation.

2. Remove or quarantine dead and legacy paths.
   - Clean up skipped legacy pipeline tests.
   - Move non-wired packages behind explicit experimental naming or wire them into a real path.
   - Stop documenting package-level prototypes as shipped user workflows.

3. Make CI strict.
   - Turn `golangci-lint` from advisory into required.
   - Clear or intentionally suppress the historical lint backlog.
   - Add `errcheck`, especially for tests.

4. Clean repo hygiene.
   - Stop tracking binaries, logs, runtime databases, local `.openexec` state, and generated artifacts.
   - Keep dogfood evidence in docs or fixtures only when it is intentionally useful.

## P1: Adaptive Execution Modes

5. Add first-class execution modes.
   - `chat`: read-only discussion.
   - `inspect`: gather context, no edits.
   - `fix`: surgical patch with targeted verification.
   - `task`: scoped implementation with relevant gates.
   - `run`: full blueprint pipeline.
   - `release`: strict build/test/review/audit.
   - `sre`: infrastructure-safe execution with deterministic approvals.

6. Build a conservative mode router.
   - Questions route to `chat` or `inspect`.
   - Localized UI text, small test fixes, and obvious bug fixes route to `fix`.
   - Unknown blast radius routes to `task`.
   - Auth, payments, schema, migrations, security, and cross-service changes route to `run`.
   - Production infrastructure routes to `sre`.

7. Allow explicit user override.
   - `openexec chat`
   - `openexec inspect "..."`
   - `openexec fix "..."`
   - `openexec task "..."`
   - `openexec run "..."`
   - `openexec sre "..."`

8. Define mode-specific gates.
   - `chat`: no gates.
   - `inspect`: no edits; optional index refresh.
   - `fix`: changed-file checks, related tests, cheap typecheck/lint.
   - `task`: package/module tests and relevant quality gates.
   - `run`: full blueprint workflow.
   - `release`: full lint/test/build, review, audit, and artifact checks.
   - `sre`: deterministic plan, allowlisted tools, and human approval where required.

## P2: Workspace Runtime

9. Add workspace discovery.
   - Scan a root such as `/Users/perttu/projects`.
   - Detect Git repos and stacks via `go.mod`, `package.json`, `pyproject.toml`, `Cargo.toml`, `Makefile`, `README.md`, and `.openexec/config.json`.
   - Ignore dependency directories, virtualenvs, build outputs, archives, and generated caches.

10. Create a workspace project registry.
    - Store project path, name, stack, Git branch, dirty state, default commands, health, last run, active sessions, and config path.
    - Keep this in a workspace database separate from per-project state.

11. Make the daemon workspace-aware.
    - Every chat, fix, task, run, and event must carry `project_id`.
    - Remove assumptions that one daemon equals one repository.
    - Support multiple read-only sessions concurrently.

12. Add per-project isolation.
    - Separate working directory, logs, run state, session history, config, and model context.
    - Deny writes outside the selected project by default.
    - Prevent two write sessions against the same project unless explicitly approved.

13. Add workspace CLI commands.
    - `openexec workspace init /Users/perttu/projects`
    - `openexec workspace start`
    - `openexec projects list`
    - `openexec project status openexec`
    - `openexec chat --project openexec`
    - `openexec fix --project openexec "..."`

## P3: Chat With Code

14. Implement project-scoped chat sessions.
    - Load the selected project's `README.md`, `AGENTS.md`, `.openexec/config.json`, package metadata, current branch, and dirty diff.
    - Keep chat read-only unless the user escalates.

15. Add chat-to-fix escalation.
    - From chat, allow: "patch this".
    - Convert gathered context into a `fix` run.
    - Link the chat transcript, diff, commands, and result.

16. Build the surgical patch loop.
    - Gather minimal context.
    - Apply a focused patch.
    - Run targeted verification.
    - Summarize changed files and evidence.
    - Stop without entering the full pipeline unless risk requires escalation.

17. Add changed-file test selection.
    - Go: `go test ./package`.
    - Python: related `pytest` file or package.
    - TypeScript/React: related Vitest, `tsc`, ESLint, or Playwright only when UI behavior changed.
    - Fall back to broader gates when dependency mapping is unclear.

18. Add escalation explanation.
    - Before switching from `fix` to `task` or `run`, show the detected risk, selected mode, planned gates, and expected side effects.

## P4: Blueprints Integration

19. Use `blueprints` during first-run project generation.
    - If the user creates a new project, ask for product category and map it to a blueprint seed or generated blueprint.
    - Use blueprint modules, routes, models, and feature-completeness contracts to create the initial plan.

20. Generate acceptance contracts from blueprints.
    - Convert blueprint operation contracts into runnable tests.
    - Require persistence, authorization, concurrency, and side-effect checks where the blueprint marks them as critical.
    - Use these contracts in `run` and `release` modes, not in tiny `fix` mode unless the touched feature requires it.

21. Add blueprint-aware task routing.
    - If a task touches a blueprint-critical feature, route to `task` or `run`.
    - If a task is outside critical contracts and localized, allow `fix`.

22. Add blueprint provenance to the UI.
    - Show which blueprint category and contracts apply to a project.
    - Show whether a change is inside or outside a critical feature area.

## P5: Workspace Web Console

23. Build a workspace dashboard.
    - Project list.
    - Stack, branch, dirty status, last result, active sessions, and health.
    - Quick actions: Chat, Inspect, Fix, Run, Tests, Open PR.

24. Build a per-project console.
    - Chat panel.
    - Current diff.
    - Run timeline.
    - Test output.
    - Mode and gate explanation.
    - Suggested next actions.

25. Add queue and scheduler views.
    - Global concurrency limit.
    - Per-project write lock.
    - Pending approvals.
    - Waiting, running, blocked, failed, and complete states.

26. Preserve the two-speed model from `openexec-web`.
    - Light mode: fast interactive MCP/chat/fix work.
    - Heavy mode: verified blueprint pipeline.
    - The UI should make the current speed and safety level obvious.

## P6: Safety And Audit

27. Enforce project write boundaries.
    - Resolve symlinks.
    - Deny writes outside the selected project.
    - Require approval for workspace-level edits.

28. Add dirty-worktree policy.
    - Detect user changes before editing.
    - Summarize dirty files.
    - Avoid overwriting.
    - Require approval for risky overlap.

29. Define per-mode permissions.
    - `chat`: read-only.
    - `inspect`: read-only plus index/cache writes.
    - `fix`: write selected project only.
    - `task`: write selected project and run relevant commands.
    - `run`: broader repo automation.
    - `sre`: allowlisted infra tools only.

30. Store an audit trail for all non-chat modes.
    - Prompt.
    - Selected mode and reason.
    - Project ID.
    - Files read.
    - Files changed.
    - Commands run.
    - Test evidence.
    - Approval decisions.

## P7: Dogfood Milestones

31. Milestone 1: workspace discovery.
    - Launch from `/Users/perttu/projects`.
    - List all projects with stack and Git status.

32. Milestone 2: read-only chat.
    - Chat with `openexec`, `blueprints`, and `openexec-web` as separate projects.

33. Milestone 3: surgical fix.
    - Apply a small fix in one project.
    - Run only targeted verification.

34. Milestone 4: concurrent workspace operations.
    - Run two read-only chats and one fix at the same time.
    - Enforce per-project write lock.

35. Milestone 5: adaptive escalation.
    - Prove a small UI copy change stays in `fix`.
    - Prove schema/auth/security change escalates to `run`.

36. Milestone 6: blueprint-generated first run.
    - Create a new project from a `blueprints` category.
    - Generate initial contract tests.
    - Run full blueprint pipeline once.

37. Milestone 7: replace the ten-terminal workflow.
    - Use one OpenExec workspace UI to monitor, chat with, and modify multiple projects.
