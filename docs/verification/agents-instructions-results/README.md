# Instruction command execution evidence

Raw stdout/stderr and exit statuses from a disposable, initially clean clone
of `4c6c99f2` (the instruction-consolidation candidate, based on supplied main
`5baa04a6`). `01-clone.txt`, `02-identity.txt` and `03-head.txt` record checkout
creation, its clean status before setup, and its revision. This pass subsequently
clarifies only `npm test` terminal prerequisites in the instruction documents;
application code, lockfiles and build/test configuration are unchanged.

These are captured execution results, not automated regression tests or claims
that the declared checks execute every command below. A read-only reviewer can
inspect the raw files without creating a checkout or running a helper script.
`manifest.json` records each argv, working directory, exit status, termination
reason and SHA-256 of the corresponding unmodified transcript. The temporary
collector is not part of the candidate. This evidence supplements the existing
declared `test` and `lint` checks; it does not expand those checks.

The clone has its own `npm ci` installation; it does not use or modify the
Console's shared `ui/node_modules`. Modules and browser installation may use
host download caches. Versions are recorded in `version-*.txt`: Go 1.26.5,
Node 22.23.2, npm 10.9.8, Python 3.12.3 and Git 2.43.0.

| Command (root unless prefixed `ui/`) | Transcript | Observed result |
| --- | --- | --- |
| `go mod download` | [04-modules.txt](04-modules.txt) | Exit 0 |
| `ui/ npm ci` | [05-npm-ci.txt](05-npm-ci.txt) | Exit 0 |
| `ui/ npx playwright install chromium` | [06-chromium.txt](06-chromium.txt) | Exit 0 |
| `make build` | [07-build.txt](07-build.txt) | Exit 0 |
| `go build -buildvcs=false -o openexec ./cmd/openexec` | [08-direct-build.txt](08-direct-build.txt) | Exit 0 |
| `make test` | [09-test.txt](09-test.txt) | Exit 0 |
| `go test ./...` | [10-go-test.txt](10-go-test.txt) | Exit 0 |
| `go test -v ./internal/loop/... -run TestLoop` | [11-focused-loop.txt](11-focused-loop.txt) | Exit 0 |
| `make compat-test` | [12-compat.txt](12-compat.txt) | Exit 0 |
| `make lint` | [13-lint.txt](13-lint.txt) | Exit 0 |
| `make type-check` | [14-type-check.txt](14-type-check.txt) | Exit 0 |
| `ui/ npm run build` | [15-ui-build.txt](15-ui-build.txt) | Exit 0 |
| `ui/ npm run lint` | [16-ui-lint.txt](16-ui-lint.txt) | Exit 0 |
| `ui/ npx vitest run --fileParallelism=false --maxWorkers=1 --minWorkers=1` | [17-ui-serial.txt](17-ui-serial.txt) | Exit 0 |
| `ui/ npm run test:coverage` | [18-coverage.txt](18-coverage.txt) | Exit 0 |
| `ui/ npm test` (noninteractive) | [19-watch.txt](19-watch.txt) | Exit 0 after running tests once; no watch readiness. Collector's readiness deadline expired. |
| `ui/ npm test` (terminal, `CI` unset) | [24-watch-terminal.txt](24-watch-terminal.txt) | Watch readiness observed; controlled SIGINT, exit -2 |
| `ui/ npm run dev -- --port 3001` | [20-dev.txt](20-dev.txt) | Vite ready; controlled SIGINT, exit -2 |
| `make build-all` | [21-docker.txt](21-docker.txt) | Exit 2; `golang:1.25-bullseye: not found` |
| `./bin/openexec init --yes` | [22-init.txt](22-init.txt) | Exit 0 in disposable clone |
| `ui/ npm run test:e2e` | [23-generic-e2e.txt](23-generic-e2e.txt) | Exit 1; `2 failed`, `71 passed` |

For generic E2E, the collector created an empty disposable sibling `initial`
directory, initialized only the disposable checkout, and checked that ports
8080 and 3001 were free before starting Playwright. The host's CI environment
was retained (the output includes retries); `PLAYWRIGHT_HTML_OPEN=never`
prevented the failure report from opening a blocking server. The two failures
are the D4 journey under the generic configuration and project initialization;
their assertions and stack traces are in the transcript. No owner project
data was used. The terminal watch run explicitly removed `CI` from the child
environment and allocated a PTY, matching interactive developer use.

Docker release build and generic E2E remain failing baseline commands. Successful
execution of every documented command is **not proven**. This documentation-only
change records those failures rather than repairing unrelated build/test behavior.
The runtime CLI examples in the architecture overview require initialized state,
providers and effect authority; they were not invoked as build/setup checks.
