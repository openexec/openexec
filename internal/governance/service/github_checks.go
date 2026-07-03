package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	"github.com/openexec/openexec/internal/governance"
	"github.com/openexec/openexec/internal/governance/connectors/github"
)

// SyncGitHubChecks fetches the CI check-runs for a change's PR head commit and,
// when they are all green, records a TRUSTED verification evidence row (source
// github) carrying provenance: the commit SHA, how many checks passed, and a
// SHA-256 of the fetched payload (kept in Raw). This is the ONLY producer of
// trusted evidence — RecordEvidence refuses trusted sources — so the auto-merge
// trust signal is always grounded in a real, fetched external check and cannot
// be asserted by hand. Requires a Runner and a recorded PR URL.
func (s *Service) SyncGitHubChecks(ctx context.Context, changeID string) error {
	if s.runner == nil {
		return errMissingRunner
	}
	ch, err := s.getChange(ctx, changeID)
	if err != nil {
		return err
	}
	repo, number, ok := repoFromPRURL(ch.PRURL)
	if !ok {
		return fmt.Errorf("change %s has no parseable PR URL (record the PR first)", changeID)
	}
	sha, err := github.PRHeadSHA(ctx, s.runner, repo, number)
	if err != nil {
		return err
	}
	checks, raw, err := github.FetchCommitChecks(ctx, s.runner, repo, sha)
	if err != nil {
		return err
	}

	total, passed, failed, pending := classifyChecks(checks)
	if total == 0 {
		return fmt.Errorf("no check-runs found for %s@%s; nothing to verify", repo, sha)
	}
	if failed > 0 || pending > 0 {
		return fmt.Errorf("checks not green for %s@%s: %d passed, %d failed, %d pending — no trusted evidence recorded", repo, sha, passed, failed, pending)
	}

	sum := sha256.Sum256(raw)
	hash := hex.EncodeToString(sum[:])
	ev := &governance.Evidence{
		ID:       newID(),
		ChangeID: changeID,
		Kind:     governance.EvidenceKindCI,
		Source:   governance.EvidenceSourceGitHub,
		Summary:  fmt.Sprintf("GitHub checks green: %d/%d passing @ %s (payload sha256 %s)", passed, total, sha, firstN(hash, 12)),
		URL:      fmt.Sprintf("https://github.com/%s/commit/%s/checks", repo, sha),
		Raw:      string(raw),
	}
	if err := s.store.CreateEvidence(ctx, ev); err != nil {
		return fmt.Errorf("record verified checks for change %q: %w", changeID, err)
	}
	return nil
}

func firstN(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

// classifyChecks tallies check-runs into total/passed/failed/pending. neutral
// and skipped count as passed (they do not block); anything not completed is
// pending; any other completed conclusion is a failure.
func classifyChecks(checks []github.CheckRun) (total, passed, failed, pending int) {
	for _, c := range checks {
		total++
		switch {
		case c.Status != "completed":
			pending++
		case c.Conclusion == "success" || c.Conclusion == "neutral" || c.Conclusion == "skipped":
			passed++
		default:
			failed++
		}
	}
	return total, passed, failed, pending
}

// currentChecksGreen re-fetches the check-runs for a change's CURRENT PR head
// commit and reports whether they are all green. Auto-merge uses this instead of
// trusting stored evidence, so a green result recorded before a later push
// cannot satisfy the gate — the checks that matter are the ones on the commit
// about to be merged.
func (s *Service) currentChecksGreen(ctx context.Context, ch *governance.ChangeRecord) (bool, string) {
	repo, number, ok := repoFromPRURL(ch.PRURL)
	if !ok {
		return false, "cannot derive repo/PR from the change's PR URL"
	}
	sha, err := github.PRHeadSHA(ctx, s.runner, repo, number)
	if err != nil {
		return false, fmt.Sprintf("cannot read current PR head: %v", err)
	}
	checks, _, err := github.FetchCommitChecks(ctx, s.runner, repo, sha)
	if err != nil {
		return false, fmt.Sprintf("cannot fetch checks for current PR head %s: %v", firstN(sha, 12), err)
	}
	total, passed, failed, pending := classifyChecks(checks)
	if total == 0 {
		return false, fmt.Sprintf("no CI checks on the current PR head %s", firstN(sha, 12))
	}
	if failed > 0 || pending > 0 {
		return false, fmt.Sprintf("current PR head %s checks not green: %d passed, %d failed, %d pending", firstN(sha, 12), passed, failed, pending)
	}
	return true, ""
}
