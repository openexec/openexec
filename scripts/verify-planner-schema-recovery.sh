#!/usr/bin/env bash
set -euo pipefail
root="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd -P)"
if [[ $# != 2 || $1 != --case || $2 != discovery ]]; then
    echo 'usage: verify-planner-schema-recovery.sh --case discovery' >&2
    exit 2
fi
exec python3 "$root/scripts/verification/planner_schema_discovery.py"
