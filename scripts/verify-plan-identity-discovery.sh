#!/usr/bin/env bash
set -euo pipefail
root="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd -P)"
export GOCACHE="${GOCACHE:-/tmp/openexec-identity-go-cache}"
export GOWORK=off
export GOENV=off
export GOFLAGS=
export PYTHONDONTWRITEBYTECODE=1
cd "$root"
python3 scripts/verification/plan_identity_discovery_test.py
exec python3 scripts/verification/plan_identity_discovery.py
