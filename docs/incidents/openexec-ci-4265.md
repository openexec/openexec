# Incident 4265: openexec-ci observation failure

## Evidence and reproduction

The captured reading at 2026-11-05 19:19:43 UTC reported `JSON observation failed`, exit code -1, without timeout or truncation. The captured change list contained no commits, releases or files: it reported that no served-version reading or delivered release record was available at incident start. The incident context supplied no CI history or logs. This is an observation failure, not evidence that a GitHub Actions job failed. The exact observer failure cannot be reproduced from that record alone.

`.agent-console/channels.yaml` declares `openexec-ci` as type `ci`, with repository and branch supplied by `OBSERVE_OPENEXEC_REPOSITORY` and `OBSERVE_OPENEXEC_BRANCH`, token named by `OBSERVE_CI_TOKEN`, a 10-minute schedule and 10-second timeout. Parsing it with Python's YAML parser and checking those fields succeeded. This checks declaration syntax, not the Console's resolved configuration or token.

The local `origin/main` history has #216 (`8a64631f`, 18:24:03 UTC) and #217 (`cdeff8d9`, 18:44:11 UTC), both before the reading. #216 separated and sequenced macOS, changed trigger paths and preserved Linux build steps; #217 changed cache policy. Neither diff establishes why the observer returned -1. Git history does not contain Actions run results. `gh run list --branch main` could not run: the configured CLI path was invalid, and with a clean temporary config the CLI reported no authentication. A direct GitHub API request was blocked by the sandbox network policy. Thus the default branch's hosted CI state at the incident is unverified.

Bare `go build ./...` and `go vet ./...` exited 0 in the candidate. Bare `go test ./...` could not complete in the sandbox because a provider request to api.anthropic.com was blocked. The declared host `go-vet` check exited 0; the declared host `go-test` check with `./...` subsequently exited 0 (some packages used cached results). These checks cannot establish the state of the incident's hosted run or the Console observer.

## Cause and follow-up

The immediate observed failure is the Console's JSON observation failure; its underlying cause and whether it is repository-side remain undetermined. The available record contains neither the observer's diagnostic nor the resolved repository/branch, response status/body, or the relevant GitHub Actions run. There is no evidence for a speculative declaration or workflow change. An operator with access to the Console's observation logs and its resolved channel configuration should inspect the original failure and the corresponding authenticated GitHub Actions runs, without exposing the token. If they identify a repository failure, reproduce that specific run and fix its cause; if the observer or its credentials/configuration failed, repair that external boundary. A fresh signal check is needed to establish current health.
