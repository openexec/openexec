# Repository Guidelines

## Project Structure & Module Organization
- `cmd/` CLI entrypoints (Go). Main binary: `./cmd/openexec`.
- `internal/`, `pkg/` core engine packages. Built binaries go to `bin/`.
- `ui/` Vite + React (TypeScript). Tests under `ui/src/**/__tests__`; Playwright e2e in `ui/e2e/`. Built assets in `ui/dist/` (embedded and served by the server).
- `docs/`, `scripts/`, `.openexec/` (local data; audit DB at `.openexec/data/audit.db`).

## Build, Test, and Development Commands
- Build (all OS): `go build -o bin/openexec ./cmd/openexec`
- Backend start: `./bin/openexec start` (default `:8080`; add `--daemon` to background)
- Go tests: `go test ./...`
- UI dev: `cd ui && npm install && VITE_API_TARGET=http://127.0.0.1:8080 npm run dev`
- UI tests: `cd ui && npm test` | coverage: `npm run test:coverage`
- E2E: `cd ui && npm run test:e2e:list`

## Coding Style & Naming Conventions
- Go: idiomatic Go; format with `gofmt`; package names lowercase; tests `*_test.go`.
- TypeScript/React: 2-space indent; components `PascalCase.tsx`, hooks `useX.ts`; tests `*.test.ts(x)` in `__tests__/`.
- Linting: Go via `golangci-lint run` (if available). UI via `npm run lint` and `npm run type-check`.

## Testing Guidelines
- Frameworks: Go `testing` (backend); Vitest + Testing Library (UI); Playwright (e2e).
- UI: prefer `screen.findBy*` for async, use `@testing-library/user-event` for interactions, and `waitFor()` for state transitions; avoid manual delays.
- Coverage: `npm run test:coverage`; cover `ui/src/components/chat/**` and `ui/src/hooks/**` critical paths.

## Commit & Pull Request Guidelines
- Commits: imperative subject; optional scope (e.g., `ui:`, `engine:`); reference issues (e.g., `#123`).
- PRs: clear description, linked issues, test evidence (unit/e2e), and screenshots/GIFs for UI changes. Note config or migration steps.

## Security & Configuration Tips
- Defaults: API on `:8080`, audit DB at `.openexec/data/audit.db`.
- UI env: `VITE_API_TARGET` for dev proxy; `VITE_API_BASE` and `VITE_WS_URL` override fetch/WS origins when needed.
- Never commit secrets; prefer env vars. Validate provider availability via `/api/providers`.

## Agent-Specific Instructions
- Verify backend DTOs before UI work; keep mocks in sync (snake_case vs camelCase).
- For failing tests: increase verbosity, form a hypothesis, try once; if not fixed, revert and try a different strategy.
- Log lessons in `.openexec/engram/learning_log.json` when solving complex issues.
