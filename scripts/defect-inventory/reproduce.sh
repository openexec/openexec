#!/usr/bin/env bash
# Re-run every reproducer in docs/DEFECT_INVENTORY.md against the current
# checkout. A row prints REPRODUCED while its defect still exists and FIXED?
# when its reproducer no longer shows it — then the inventory row is stale and
# must be re-dispositioned (name the fixing commit), not silently dropped.
#
# Exit status: 0 when every confirmed row still reproduces, 1 otherwise.
# Usage: scripts/defect-inventory/reproduce.sh   (from the repository root)
set -u
cd "$(git rev-parse --show-toplevel)" || exit 2

stale=0
report() { # id, outcome(0=reproduced), summary
  if [ "$2" -eq 0 ]; then
    printf 'REPRODUCED  %s  %s\n' "$1" "$3"
  else
    printf 'FIXED?      %s  %s\n' "$1" "$3"
    stale=1
  fi
}

echo "revision $(git rev-parse HEAD)"

# D1: the API runner's file tools write outside the workspace.
go test -count=1 -tags defectrepro -run TestDefectD1WriteFileEscapesWorkDir ./internal/loop/ >/dev/null 2>&1
report D1 $? "API runner write_file accepts ../ and absolute paths (internal/loop/api_runner.go resolvePath)"

# D2: CLI output goes to stderr; stdout is empty.
out_bytes=$(go run ./cmd/openexec version 2>/dev/null | wc -c)
[ "$out_bytes" -eq 0 ]
report D2 $? "openexec version prints ${out_bytes} bytes to stdout (cobra cmd.Printf -> stderr)"

# D3: GitHub CI never runs the UI unit tests.
! grep -Eq 'vitest|npm (run )?test|make test' .github/workflows/ci.yml
report D3 $? ".github/workflows/ci.yml has no UI test step"

# D4: embedded UI approvals hit /api + /api/v1/... and the daemon wires no approval gate.
grep -q 'apiUrl}/api/v1/approvals' ui/src/hooks/useBlueprint.ts &&
  grep -q "?? '/api'" ui/src/App.tsx &&
  ! grep -rq 'WithApprovalGate(' --include='*.go' --exclude='*_test.go' internal cmd pkg/manager
report D4 $? "UI requests /api/api/v1/approvals; no production caller of api.WithApprovalGate"

# D5: Knowledge Hub requests a route no server registers.
grep -q "/api/v1/knowledge/policies" ui/src/pages/KnowledgeHub.tsx &&
  ! grep -rq 'knowledge/policies' --include='*.go' --exclude='*_test.go' internal pkg
report D5 $? "GET /api/v1/knowledge/policies has no handler (404 -> 'Some knowledge endpoints failed to load')"

# D6: UI stage list omits stages the default blueprint runs.
missing=0
for s in fix_lint production_readiness_check; do
  grep -q "Name: *\"$s\"" internal/blueprint/engine.go || continue
  grep -q "'$s'" ui/src/types/blueprint.ts || missing=1
done
[ "$missing" -eq 1 ]
report D6 $? "ui/src/types/blueprint.ts StageName lacks fix_lint/production_readiness_check"

# D7: the deterministic router sends a conceptual question to list_directory.
go test -count=1 -run 'TestDeterministicRouter_ParseIntent_DiffWordBoundary' ./internal/router/ >/dev/null 2>&1 &&
  grep -q '"what is the difference between a list and a tuple?", "list_directory"' internal/router/deterministic_test.go
report D7 $? "'list and a tuple' question routes to list_directory (pinned by deterministic_test.go)"

# D8: build artifacts are tracked.
tracked=$(git ls-files .gocache bin/openexec openexec data/fwus.jso | wc -l)
[ "$tracked" -gt 0 ]
report D8 $? "${tracked} tracked build/cache artifacts (.gocache, bin/openexec, openexec, data/fwus.jso)"

exit "$stale"
