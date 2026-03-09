# Repository Guidelines

## Project Structure & Module Organization
- `cmd/` Go CLI entrypoints. Main binary: `./cmd/openexec` (built to `bin/`).
- `internal/`, `pkg/` core engine packages. Binaries in `bin/`.
- `ui/` Vite + React (TypeScript). Unit tests in `ui/src/**/__tests__/`; Playwright e2e in `ui/e2e/`. Built assets in `ui/dist/` (embedded/served by backend).
- `docs/`, `scripts/`, `.openexec/` (local data; audit DB at `.openexec/data/audit.db`).

## Build, Test, and Development Commands
- Build backend: `go build -o bin/openexec ./cmd/openexec`.
- Run backend: `./bin/openexec start` (serves on `:8080`; add `--daemon` to background).
- Go tests: `go test ./...`.
- UI dev server: `cd ui && npm install && VITE_API_TARGET=http://127.0.0.1:8080 npm run dev`.
- UI tests: `cd ui && npm test`; coverage: `npm run test:coverage`.
- E2E tests: `cd ui && npm run test:e2e:list`.

## Coding Style & Naming Conventions
- Go: idiomatic; format with `gofmt`. Package names lowercase. Tests: `*_test.go`.
- TypeScript/React: 2-space indent. Components `PascalCase.tsx`; hooks `useX.ts`; tests `*.test.ts(x)` in `__tests__/`.
- Linting: Go via `golangci-lint run` (if available). UI: `npm run lint` and `npm run type-check`.

## Testing Guidelines
- Frameworks: Go `testing` (backend); Vitest + Testing Library (UI); Playwright (e2e).
- UI tests: prefer `screen.findBy*`, use `@testing-library/user-event`, and `waitFor()` for async; avoid manual delays.
- Coverage: `npm run test:coverage`; prioritize `ui/src/components/chat/**` and `ui/src/hooks/**`.

## Commit & Pull Request Guidelines
- Commits: imperative subject; optional scope (e.g., `ui:`, `engine:`); reference issues (e.g., `#123`).
- PRs: clear description, linked issues, test evidence (unit/e2e), and screenshots/GIFs for UI changes. Note config or migration steps.

## Security & Configuration Tips
- Defaults: API on `:8080`; audit DB at `.openexec/data/audit.db`.
- UI env: `VITE_API_TARGET` for dev proxy; `VITE_API_BASE` and `VITE_WS_URL` to override fetch/WS origins.
- Do not commit secrets; prefer environment variables. Validate provider availability via `/api/providers`.

## Agent-Specific Instructions
- Verify backend DTOs before UI work; keep mocks in sync (snake_case vs camelCase).
- When tests fail: increase verbosity, form a hypothesis, try once; if unresolved, revert and try a different approach.
- Capture lessons in `.openexec/engram/learning_log.json` for complex issues.

