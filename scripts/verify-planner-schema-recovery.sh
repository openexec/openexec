#!/usr/bin/env bash
set -euo pipefail
root="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd -P)"
if [[ $# != 2 || $1 != --case ]]; then
    echo 'usage: verify-planner-schema-recovery.sh --case discovery|recovery|replay' >&2
    exit 2
fi
case "$2" in
    discovery) exec python3 "$root/scripts/verification/planner_schema_discovery.py" ;;
    replay)
        python3 -m unittest discover -s "$root/scripts/verification" -p planner_schema_replay_test.py
        exec python3 "$root/scripts/verification/planner_schema_replay.py" ;;
    recovery) exec python3 "$root/scripts/verification/planner_schema_recovery.py" ;;
    *) echo 'unknown planner verification case' >&2; exit 2 ;;
esac
