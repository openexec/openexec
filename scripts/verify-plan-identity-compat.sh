#!/usr/bin/env bash
# Full run may take several minutes: dispatch through the repository job runner.
# Individual phases allow bounded diagnosis; only the default runs the whole gate.
set -euo pipefail
root="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd -P)"
export GOCACHE="${GOCACHE:-/tmp/openexec-identity-go-cache}"
export GOWORK=off
export GOENV=off
export GOFLAGS=
cd "$root"
phase="${1:-all}"
if [[ $# -gt 1 ]] || [[ ! "$phase" =~ ^(all|targeted|coverage|mutations|test|compat-test|type-check|restored)$ ]]; then
    echo "usage: $0 [targeted|coverage|mutations|test|compat-test|type-check|restored]" >&2
    exit 2
fi
python3 scripts/verification/plan_identity_compat_test.py
run_phase() {
    echo "BEGIN identity compatibility: $1"
    case "$1" in
        targeted|restored) python3 scripts/verification/plan_identity_compat.py targeted ;;
        coverage) bash scripts/verify-plan-identity-unit-coverage.sh ;;
        mutations)
            python3 scripts/verification/plan_identity_unit_coverage_test.py
            python3 scripts/verification/plan_identity_mutations.py
            ;;
        test) make test ;;
        compat-test) python3 scripts/verification/plan_identity_compat.py compat-test ;;
        type-check) make type-check ;;
    esac
    echo "PASS identity compatibility: $1"
}
if [[ "$phase" == all ]]; then
    for step in targeted coverage test compat-test type-check mutations restored; do
        run_phase "$step"
    done
    echo "PASS T-US-011-003: complete plan identity compatibility"
else
    run_phase "$phase"
fi
