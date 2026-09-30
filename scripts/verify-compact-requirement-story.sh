#!/usr/bin/env bash
# Default runs the frozen complete story script. --step allows the same commands
# to be dispatched separately by a runner without losing their exact results.
set -euo pipefail
root="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd -P)"
export GOCACHE="${GOCACHE:-/tmp/openexec-compact-go-cache}"
export GOWORK=off
export GOENV=off
export GOFLAGS=
cd "$root"
exec python3 "$root/scripts/verification/compact_requirement_story.py" "$@"
