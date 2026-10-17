#!/usr/bin/env bash
set -euo pipefail
script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
if [[ $# != 2 || "$1" != --case ]]; then
  echo 'expected --case architecture, architecture-contracts, architecture-traceability, behavioral-reproduction, runtime-boundary, native-recovery-tracer exit-1-slice recovery-matrix compatibility-refusal implementation-unit-coverage native-recovery engine-recovery story-evidence goal-validation or owner-acceptance' >&2
  exit 2
fi
case "$2" in
  goal-validation)
    exec python3 -B "$script_dir/goal_validation.py"
    ;;
  owner-acceptance)
    exec python3 -B "$script_dir/owner_acceptance.py"
    ;;
  native-recovery|engine-recovery|story-evidence)
    exec python3 -B "$script_dir/aggregate_evidence.py" --case "$2"
    ;;
  implementation-unit-coverage)
    exec python3 -B "$script_dir/implementation_coverage.py"
    ;;
  behavioral-reproduction|runtime-boundary|native-recovery-tracer|exit-1-slice|recovery-matrix|compatibility-refusal)
    exec python3 -B "$script_dir/runtime_evidence.py" --case "$2"
    ;;
  architecture)
    python3 -B "$script_dir/architecture_contracts.py" "$script_dir/../.."
    python3 -B "$script_dir/architecture_traceability.py" "$script_dir/../.."
    echo 'architecture: PASS (contracts and traceability; discovery only)'
    exit 0
    ;;
  architecture-contracts) checker=architecture_contracts.py ;;
  architecture-traceability) checker=architecture_traceability.py ;;
  *) echo "unsupported case: $2" >&2; exit 2 ;;
esac
exec python3 -B "$script_dir/$checker" "$script_dir/../.."
