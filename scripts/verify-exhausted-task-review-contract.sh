#!/usr/bin/env bash
set -euo pipefail
root=$(cd "$(dirname "$0")/.." && pwd)
cd "$root"
python3 -m unittest discover -s scripts/verification -p test_exhausted_task_review_contract.py
exec python3 scripts/verification/exhausted_task_review_contract.py "$@"
