#!/usr/bin/env bash
# Build and deploy the exact merge checkout materialized by Agent Console's
# deploy_reviewed_feature to the repository-graph daemon (openexec-graph.service),
# then verify the running service reports this build's commit. A service that
# does not come back on the new commit is rolled back to the previous binary.
#
# Owner profile (~/.agent-console/env):
#   AGENT_CONSOLE_REVIEWED_DEPLOYMENTS='{"openexec/openexec":["bash","scripts/reviewed-deploy-graph.sh"]}'
set -euo pipefail

repo_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
source_repo="${AGENT_CONSOLE_DEPLOY_REPOSITORY:-}"
deploy_worktree="${AGENT_CONSOLE_DEPLOY_WORKTREE:-}"

binary_path="$HOME/.local/bin/openexec"
unit="openexec-graph.service"
base_url="http://127.0.0.1:8765"
wait_seconds=60
while [ $# -gt 0 ]; do
  case "$1" in
    --binary-path) binary_path="$2"; shift 2 ;;
    --unit) unit="$2"; shift 2 ;;
    --base-url) base_url="$2"; shift 2 ;;
    --wait-seconds) wait_seconds="$2"; shift 2 ;;
    *) echo "unknown argument: $1" >&2; exit 2 ;;
  esac
done

cleanup_worktree() {
  local status=$?
  trap - EXIT
  if [ -n "$source_repo" ] && [ -n "$deploy_worktree" ]; then
    cd /
    if ! git -C "$source_repo" worktree remove --force "$deploy_worktree"; then
      echo "reviewed deployment completed, but its managed worktree could not be removed: $deploy_worktree" >&2
      status=1
    fi
    git -C "$source_repo" worktree prune || true
  fi
  exit "$status"
}
trap cleanup_worktree EXIT

if [ -n "$deploy_worktree" ] && [ "$repo_dir" != "$deploy_worktree" ]; then
  echo "refusing reviewed deployment outside its server-minted worktree" >&2
  exit 1
fi
if git -C "$repo_dir" symbolic-ref --quiet HEAD >/dev/null 2>&1; then
  echo "refusing reviewed deployment from a branch checkout" >&2
  exit 1
fi
if [ -n "$(git -C "$repo_dir" status --porcelain --untracked-files=normal)" ]; then
  echo "refusing reviewed deployment from a dirty exact-merge worktree" >&2
  exit 1
fi

commit="$(git -C "$repo_dir" rev-parse --short HEAD)"
npm --prefix "$repo_dir/ui" ci --no-audit --no-fund
make -C "$repo_dir" build COMMIT="$commit"
built="$repo_dir/bin/openexec"
if ! "$built" version | grep -q "Commit: *$commit\$"; then
  echo "built binary does not report commit $commit" >&2
  exit 1
fi

# health_commit prints the commit the running service reports, or nothing.
health_commit() {
  curl -fsS --max-time 5 "$base_url/api/health" 2>/dev/null |
    sed -n 's/.*"commit":"\([^"]*\)".*/\1/p'
}

wait_for_commit() {
  local want="$1" deadline=$((SECONDS + wait_seconds))
  while [ "$SECONDS" -lt "$deadline" ]; do
    if [ "$(health_commit)" = "$want" ]; then
      return 0
    fi
    sleep 2
  done
  return 1
}

previous="$binary_path.previous"
mkdir -p "$(dirname "$binary_path")"
if [ -f "$binary_path" ]; then
  cp -p "$binary_path" "$previous"
fi
cp "$built" "$binary_path.new"
chmod 0755 "$binary_path.new"
mv -f "$binary_path.new" "$binary_path"
systemctl --user restart "$unit"

rollback() {
  echo "$1; rolling back" >&2
  if [ -f "$previous" ]; then
    mv -f "$previous" "$binary_path"
    systemctl --user restart "$unit" || true
  fi
  exit 1
}

if ! wait_for_commit "$commit"; then
  rollback "$unit did not report commit $commit within ${wait_seconds}s (reports: '$(health_commit)')"
fi
# The running build must still refuse graph reads without the bearer.
status="$(curl -sS -o /dev/null -w '%{http_code}' --max-time 5 "$base_url/api/v1/repository-graph/symbols?q=x" || true)"
if [ "$status" != "401" ]; then
  rollback "unauthenticated graph read returned $status, want 401"
fi

echo "$unit running commit $commit at $base_url; unauthenticated graph read refused (401)"
