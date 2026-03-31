# Repository Guidelines

## Project Structure & Module Organization
OpenExec is a Go monorepo with a separate React UI. The CLI entry point is `cmd/openexec/main.go`. Core backend code lives under `internal/` for app-private packages and `pkg/` for reusable/public packages. Frontend code is in `ui/src`, end-to-end tests are in `ui/e2e`, and static assets are in `ui/public`. Operational docs live in `docs/`, while helper scripts and release tooling live in `scripts/` and `bin/`.

## Build, Test, and Development Commands
Use the Makefile for the common full-stack workflow:

- `make build`: build the CLI binary to `bin/openexec`
- `make lint`: run `go vet`, optional `golangci-lint`, and UI ESLint
- `make test`: run Go tests and UI Vitest tests
- `make type-check`: compile Go packages and run `tsc --noEmit`
- `make build-all`: build release binaries with `Dockerfile.build`

For UI-only work, use `cd ui && npm run dev`, `npm run build`, `npm run test:coverage`, and `npm run test:e2e`.

## Coding Style & Naming Conventions
Format Go code with `gofmt` and keep packages lowercase and focused. Preserve the existing layout: exported identifiers use `CamelCase`, internal helpers use `camelCase`, and tests stay next to implementation files as `*_test.go`. In the UI, use TypeScript with React function components, `PascalCase` page/component names, and `camelCase` hooks/utilities such as `useSession.ts` and `formatters.ts`. Lint UI changes with `cd ui && npm run lint`.

## Testing Guidelines
Backend tests use Go’s standard `testing` package and run with `go test ./...`. Frontend unit tests use Vitest; end-to-end coverage uses Playwright. Name UI tests `*.spec.ts` or `*.test.ts(x)` and keep them close to the feature or under `ui/e2e` for browser flows. Run targeted checks before opening a PR, then finish with `make test`, `make compat-test`, and `make type-check` when the change touches project loading, migration, self-healing, or legacy behavior.

## Anti-Regression Policy
Do not merge a change that modifies compatibility-sensitive behavior without proving support was preserved. For each such change, do one of these: add or update automated coverage that directly exercises the changed path, or record a short compatibility evaluation explaining why existing project support cannot have dropped. Treat existing `.openexec` projects, legacy `.uaos` projects, and migration fallbacks such as `.openexec/tasks.json` as protected behavior until explicitly deprecated.

## Commit & Pull Request Guidelines
Recent history follows short Conventional Commit prefixes such as `fix:`, `docs:`, and `release:`. Keep subjects imperative and under one line, for example `fix: restore quality gate wiring`. PRs should describe scope, note any config or schema changes, link related issues, and include screenshots or recordings for visible UI changes. Call out any skipped tests or follow-up work explicitly.

## Security & Configuration Tips
Do not commit secrets, local caches, or generated artifacts. The repo already contains transient directories like `ui/dist`, `ui/coverage`, and `.gocache`; treat them as disposable output, not source.
