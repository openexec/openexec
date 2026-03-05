# Repository Guidelines

## Project Structure & Module Organization
- `cmd/` CLI entrypoints (Go), main binary built from `./cmd/openexec`.
- `internal/`, `pkg/` core engine packages; `bin/` houses built tools (e.g., `axon`).
- `ui/` Vite + React (TypeScript). Unit tests live beside code under `ui/src/**/__tests__`. Playwright e2e in `ui/e2e/`.
- `docs/`, `scripts/`, `.openexec/` (local data; audit DB at `.openexec/data/audit.db`).

## Build, Test, and Development Commands
- Go build: `go build -o bin/openexec ./cmd/openexec`
- Go tests: `go test ./...` (backend unit tests)
- Backend run (dev): `./bin/axon serve --audit-db .openexec/data/audit.db --projects-dir .. --port 8080`
- UI setup: `cd ui && npm install`
- UI dev server: `cd ui && npm run dev` (defaults to port 3001)
- UI tests: `cd ui && npm test` | coverage: `npm run test:coverage`
- E2E: `cd ui && npm run test:e2e:list`

## Coding Style & Naming Conventions
- Go: idiomatic Go, `gofmt` enforced; package names lowercase; tests `*_test.go`.
- TypeScript/React: 2-space indent; components `PascalCase.tsx`, hooks `useX.ts`; tests `*.test.ts(x)` colocated under `__tests__/`.
- Linting: Go via `golangci-lint run` (if installed). UI via `npm run lint` and `npm run type-check`.

## Testing Guidelines
- Frameworks: Go `testing` for backend; Vitest + Testing Library for UI; Playwright for e2e.
- UI tests: prefer `screen.findBy*` for async elements, use `@testing-library/user-event`, and wait for state transitions with `waitFor()`; avoid manual `setTimeout`.
- Coverage: use `npm run test:coverage` for UI; ensure critical flows in `ui/src/components/chat/**` and `ui/src/hooks/**` are covered.

## Commit & Pull Request Guidelines
- Commits: imperative, concise subject; scope prefix when helpful (e.g., `ui:`, `engine:`). Reference issues (`#123`) when applicable.
- PRs: include summary, linked issues, test evidence (unit/e2e output), and screenshots for UI changes. Note any config or migration steps.

## Agent-Specific Instructions
- Verify API shapes before UI work (`internal/**`), keep mocks in sync (snake_case vs camelCase).
- If a test fails: increase verbosity, form a hypothesis, try once; if not fixed, revert and attempt a different approach.
- Capture notable lessons to `.openexec/engram/learning_log.json`.
