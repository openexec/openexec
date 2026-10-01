#!/usr/bin/env bash
set -euo pipefail
root="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd -P)"
cd "$root"
if [[ "${1:-}" == "--self-test" ]]; then
    shift
    python3 scripts/verification/plan_identity_delivery_test.py "$@"
else
    python3 scripts/verification/plan_identity_delivery.py "$@"
fi
