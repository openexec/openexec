# openexec-ci incident 4081

## Evidence and conclusion

The retained reading at 2026-11-04T15:31:57.622836499Z reports
`JSON observation failed`, exit code -1, without timeout or truncation. It
contains no underlying error, response body, or CI run identifier. Deployment
history, configured default-branch CI history, and logs were not supplied. The
captured change list has no commits or files: its error says there was no served
version reading or delivered release record at incident start. These facts do
not identify the cause of the observer failure or establish that CI failed.

At investigation, candidate HEAD and the local `origin/main` reference were
both `d4e65423d3a1667ab1eebf68d48a3db7d310fd2d`, with no initial worktree
diff. `git log origin/main -n 30 --oneline` and the workflow's history were
inspected. Within that history, `b78f5e87` changes the Linux runner to the
`OPENEXEC_LINUX_RUNNER` repository variable with an `ubuntu-latest` fallback;
the macOS leg remains `macos-latest`. This is a relevant recent CI change, but
the captured reading does not connect it to the failure. There is no failing
package or test to correlate, and no evidence supporting a repository fix.

`.github/workflows/ci.yml` runs on main pushes and pull requests, with Linux and
macOS, Go 1.25 and Node 22. It builds and tests the UI, downloads Go modules,
builds the embedded binary, runs Go tests, the workspace search guard and
compatibility tests, and runs advisory golangci-lint. The workflow does not
define the `openexec-ci` observer or explain what its JSON observation reads.
The configured observer script `scripts/observe/_signal.py` is absent from this
checkout. Local results cannot prove hosted CI or the live signal is healthy.

## Verification

On the candidate, the following completed successfully:

- Host declared `go-test` with `./...`: exit 0, all packages passed or had no
  tests. Host declared `lint`: exit 0, including `go vet ./...` and UI ESLint.
- `npm run build` in `ui`: exit 0; `go build -tags ui_dist -v -o
  /tmp/openexec-ci-4081 ./cmd/openexec`: exit 0.
- `npx vitest run --fileParallelism=false` in `ui`: exit 0, 44 files and 661
  tests passed, with React warnings.
- `make compat-test`: exit 0, including current and legacy project cases.
- `python3 scripts/verify-workspace-search.py`: exit 0; all 16 guard-removal
  negative controls produced their expected runtime failures.

These are local Linux results, not the hosted matrix. The Go test invocation
was non-verbose; it exercised the same package set as CI. No test was changed
because no reproducible repository fault was identified.

## Follow-up outside this repository

Inspect the configured observer's actual command, response and error logs for
the retained timestamp, and the corresponding hosted default-branch CI runs
and runner allocation. Re-run the configured live signal check from its owning
environment and correlate any failure with a specific run or response. This
investigation cannot claim the signal recovered or assign a root cause from
the retained summary alone.
