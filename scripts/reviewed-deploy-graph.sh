#!/usr/bin/env bash
# Build and deploy the exact merge checkout materialized by Agent Console's
# deploy_reviewed_feature to the repository-graph daemon (openexec-graph.service),
# then prove the running build end to end: it reports this build's commit,
# refuses unauthenticated reads, walks the Explore journey (search → explicit
# selection → both call directions → impact → source, plus bearer, checkout,
# fabricated-symbol, direction and bound refusals), and after a second restart
# re-reads the held identity. Both journey files record the commit the daemon
# reported and each answer it gave (source bodies as digests only). Any failure — including a failed restart —
# restores the previous binary. The journey evidence is kept per deploy.
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
# The daemon's own EnvironmentFile: the journey authenticates as Console does.
token_file="$HOME/.agent-console/openexec-evidence.env"
journey_query="Execute"
evidence_root="$HOME/.agent-console/deploy-evidence/openexec-graph"
while [ $# -gt 0 ]; do
  case "$1" in
    --binary-path) binary_path="$2"; shift 2 ;;
    --unit) unit="$2"; shift 2 ;;
    --base-url) base_url="$2"; shift 2 ;;
    --wait-seconds) wait_seconds="$2"; shift 2 ;;
    --token-file) token_file="$2"; shift 2 ;;
    --journey-query) journey_query="$2"; shift 2 ;;
    --evidence-root) evidence_root="$2"; shift 2 ;;
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
if [ ! -r "$token_file" ]; then
  echo "refusing reviewed deployment: graph bearer file $token_file is not readable, so the live journey cannot be walked" >&2
  exit 1
fi

commit="$(git -C "$repo_dir" rev-parse --short HEAD)"
npm --prefix "$repo_dir/ui" ci --no-audit --no-fund
make -C "$repo_dir" build COMMIT="$commit"
built="$repo_dir/bin/openexec"
# The CLI prints its version with cobra cmd.Printf, which writes to stderr.
if ! "$built" version 2>&1 | grep -q "Commit: *$commit\$"; then
  echo "built binary does not report commit $commit" >&2
  exit 1
fi
evidence_dir="$evidence_root/$commit-$(date -u +%Y%m%dT%H%M%SZ)"
mkdir -p "$evidence_dir"

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
# rollback is defined before the swap so every failure after it — the restart
# itself included — restores the build that was running. Without a previous
# binary the unverified one is removed rather than left for the next restart.
rollback() {
  echo "$1; rolling back" >&2
  if [ -f "$previous" ]; then
    mv -f "$previous" "$binary_path"
  else
    rm -f "$binary_path"
  fi
  if ! systemctl --user restart "$unit"; then
    echo "rollback restart of $unit failed; the previous binary is in place but not running" >&2
  fi
  echo "rolled back; evidence kept in $evidence_dir" >&2
  exit 1
}

mkdir -p "$(dirname "$binary_path")"
if [ -f "$binary_path" ]; then
  cp -p "$binary_path" "$previous"
else
  rm -f "$previous"
fi
cp "$built" "$binary_path.new"
chmod 0755 "$binary_path.new"
mv -f "$binary_path.new" "$binary_path"

if ! systemctl --user restart "$unit"; then
  rollback "restart of $unit failed"
fi
if ! wait_for_commit "$commit"; then
  rollback "$unit did not report commit $commit within ${wait_seconds}s (reports: '$(health_commit)')"
fi
# The running build must still refuse graph reads without the bearer.
status="$(curl -sS -o /dev/null -w '%{http_code}' --max-time 5 "$base_url/api/v1/repository-graph/symbols?q=x" || true)"
if [ "$status" != "401" ]; then
  rollback "unauthenticated graph read returned $status, want 401"
fi

if ! "$built" knowledge graph journey --base-url "$base_url" --token-file "$token_file" \
  --want-commit "$commit" --query "$journey_query" --evidence "$evidence_dir/journey.json"; then
  rollback "live Explore journey failed against $unit"
fi
# Reload: restart the daemon and re-read the identity the journey held.
if ! systemctl --user restart "$unit"; then
  rollback "reload restart of $unit failed"
fi
if ! wait_for_commit "$commit"; then
  rollback "$unit did not come back on commit $commit after reload"
fi
if ! "$built" knowledge graph journey --base-url "$base_url" --token-file "$token_file" \
  --want-commit "$commit" --requery "$evidence_dir/journey.json" --evidence "$evidence_dir/requery.json"; then
  rollback "re-query after reload failed against $unit"
fi

echo "$unit running commit $commit at $base_url; journey and re-query after reload passed; evidence: $evidence_dir"
