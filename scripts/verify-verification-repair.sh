#!/usr/bin/env bash
set -euo pipefail
root="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)"
if [[ $# != 2 || $1 != --case ]]; then
    echo 'usage: verify-verification-repair.sh --case study' >&2
    exit 2
fi
case "$2" in
    study) exec python3 "$root/scripts/verification/repair_study.py" ;;
    *) echo "unimplemented or unknown verification case: $2" >&2; exit 2 ;;
esac
