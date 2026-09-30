#!/usr/bin/env bash
set -euo pipefail
root="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)"
if [[ $# != 2 || $1 != --case ]]; then
    echo 'usage: verify-verification-repair.sh --case study|admitted-tracer|legacy-incident|named-recapture|private-storage|storage-unit-coverage' >&2
    exit 2
fi
case "$2" in
    storage-unit-coverage) exec python3 "$root/scripts/verification/storage_unit_coverage.py" ;;
    private-storage) exec python3 "$root/scripts/verification/private_storage.py" ;;
    legacy-incident|named-recapture) exec python3 "$root/scripts/verification/named_recapture.py" ;;
    admitted-tracer) exec python3 "$root/scripts/verification/admitted_evidence.py" ;;
    study) exec python3 "$root/scripts/verification/repair_study.py" ;;
    *) echo "unimplemented or unknown verification case: $2" >&2; exit 2 ;;
esac
