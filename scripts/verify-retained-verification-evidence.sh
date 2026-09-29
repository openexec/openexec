#!/usr/bin/env bash
set -euo pipefail
root="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)"
if [[ $# != 2 || $1 != --case ]]; then
    echo 'usage: verify-retained-verification-evidence.sh --case discovery' >&2
    exit 2
fi
case "$2" in
    discovery) exec python3 "$root/scripts/verification/discovery.py" ;;
    *) echo "unimplemented or unknown verification case: $2" >&2; exit 2 ;;
esac
