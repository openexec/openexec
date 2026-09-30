#!/usr/bin/env bash
set -euo pipefail
root="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd -P)"
if [[ $# != 1 || $1 != discovery && $1 != repair ]]; then
    echo 'usage: verify-compact-requirement-evidence.sh discovery|repair' >&2
    exit 2
fi
exec python3 "$root/scripts/verification/compact_requirement_evidence.py" "$1"
