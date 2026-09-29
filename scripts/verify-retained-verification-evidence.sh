#!/usr/bin/env bash
set -euo pipefail
root="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)"
if [[ $# != 2 || $1 != --case ]]; then
    echo 'usage: verify-retained-verification-evidence.sh --case discovery|legacy-recapture|retained-result|evidence-boundaries|retention-unit-coverage|retention-mutations|retention-story|recapture-boundaries|recapture-unit-coverage|recapture-compatibility|recapture-story|full|delivery-ready' >&2
    exit 2
fi
case "$2" in
    full|delivery-ready) exec python3 "$root/scripts/verification/delivery.py" --case "$2" ;;
    legacy-recapture) exec python3 "$root/scripts/verification/legacy_recapture.py" ;;
    recapture-boundaries|recapture-unit-coverage|recapture-compatibility|recapture-story)
        exec python3 "$root/scripts/verification/recapture_story.py" --case "$2"
        ;;
    discovery) exec python3 "$root/scripts/verification/discovery.py" ;;
    retained-result|evidence-boundaries|retention-unit-coverage|retention-mutations|retention-story)
        exec python3 "$root/scripts/verification/retention_story.py" --case "$2"
        ;;
    *) echo "unimplemented or unknown verification case: $2" >&2; exit 2 ;;
esac
