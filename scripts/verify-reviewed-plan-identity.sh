#!/usr/bin/env bash
set -euo pipefail
root="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd -P)"
export GOCACHE="${GOCACHE:-/tmp/openexec-identity-go-cache}"
export GOWORK=off
export GOENV=off
export GOFLAGS=
cd "$root"
python3 scripts/verification/reviewed_plan_identity_test.py
exec python3 scripts/verification/reviewed_plan_identity.py
