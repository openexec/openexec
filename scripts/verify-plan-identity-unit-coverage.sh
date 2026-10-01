#!/usr/bin/env bash
set -euo pipefail
root="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd -P)"
export GOCACHE="${GOCACHE:-/tmp/openexec-identity-go-cache}"
export GOWORK=off
export GOENV=off
export GOFLAGS=
cd "$root"
python3 scripts/verification/plan_identity_discovery_test.py
python3 scripts/verification/reviewed_plan_identity_test.py
python3 scripts/verification/plan_identity_unit_coverage_test.py
python3 scripts/verification/plan_identity_unit_coverage.py
if [[ "${1:-}" == "--mutations" ]]; then
    python3 scripts/verification/plan_identity_mutations.py
elif [[ $# != 0 ]]; then
    echo "usage: $0 [--mutations]" >&2
    exit 2
fi
