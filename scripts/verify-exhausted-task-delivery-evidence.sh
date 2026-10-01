#!/usr/bin/env bash
set -euo pipefail
root=$(cd "$(dirname "$0")/.." && pwd)
cd "$root"
if [[ "${1:-}" == "--self-test" ]]; then
    exec python3 -m unittest discover -s scripts/verification -p test_exhausted_task_delivery.py
fi
exec python3 scripts/verification/exhausted_task_delivery.py "$@"
