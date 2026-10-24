package knowledge

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// StaleGraphError is an explicit refusal to answer from pointers whose scan
// inputs no longer match the repository. Callers may fall back to ordinary
// repository inspection, but must not use the outdated graph result.
type StaleGraphError struct {
	State RepositoryState
	Cause error
}

func (e *StaleGraphError) Error() string {
	if e.Cause == nil {
		return "repository graph is stale and could not be refreshed"
	}
	return fmt.Sprintf("repository graph is stale and could not be refreshed: %v", e.Cause)
}

func (e *StaleGraphError) Unwrap() error { return e.Cause }

func IsStaleGraph(err error) bool {
	var stale *StaleGraphError
	return errors.As(err, &stale)
}

// MissingGraphError reports that no graph generation exists for the checkout.
// It is distinct from a symbol that is absent from a current graph, so callers
// can tell "never scanned" from "not found". It unwraps to sql.ErrNoRows to
// keep existing missing-graph handling working.
type MissingGraphError struct {
	State RepositoryState
}

func (e *MissingGraphError) Error() string {
	return "repository graph has not been built for this checkout; run `openexec knowledge graph scan`"
}

func (e *MissingGraphError) Unwrap() error { return sql.ErrNoRows }

func missingGraph(identity RepositoryIdentity) *MissingGraphError {
	return &MissingGraphError{State: RepositoryState{
		RepositoryID: identity.RepositoryID, CheckoutID: identity.CheckoutID,
		WorktreeID: identity.WorktreeID, BaseCommit: identity.BaseCommit, Freshness: FreshnessMissing,
	}}
}

// errRefreshDisabled explains a stale refusal that the reader deliberately did
// not repair.
var errRefreshDisabled = errors.New(
	"this server does not rebuild the graph on read; run `openexec knowledge graph scan` to refresh it")

// SetRefreshOnRead controls whether a read may repair a drifted generation.
//
// The default (true) is the V2.1 contract: a read recomputes and re-resolves
// rather than answering from stale pointers. A server that advertises symbol
// lookup as the cheap alternative to grep sets it false — on a large
// repository the refresh path runs a full extraction, so an advertised-cheap
// lookup would block for minutes. With refresh disabled the reader reports the
// drift as an explicit stale refusal and mutates nothing.
func (s *Store) SetRefreshOnRead(refresh bool) {
	s.graphMu.Lock()
	defer s.graphMu.Unlock()
	s.noRefreshOnRead = !refresh
}

// freshGeneration is the load-bearing V2 read gate. It serializes with graph
// writers, recomputes the current scan manifest at read time, refreshes drifted
// inputs, and returns only a generation built from the current worktree.
func (s *Store) freshGeneration(ctx context.Context, identity RepositoryIdentity) (GraphGeneration, RepositoryState, error) {
	s.graphMu.Lock()
	defer s.graphMu.Unlock()
	if err := ctx.Err(); err != nil {
		return GraphGeneration{}, RepositoryState{}, err
	}
	generation, err := s.activeGeneration(ctx, identity.WorktreeID)
	if err == sql.ErrNoRows {
		missing := missingGraph(identity)
		return GraphGeneration{}, missing.State, missing
	}
	if err != nil {
		return GraphGeneration{}, RepositoryState{}, err
	}
	manifest, manifestErr := BuildScanManifest(identity.RootPath)
	state := generationState(generation)
	if manifestErr != nil {
		state.Freshness = FreshnessStale
		return GraphGeneration{}, state, &StaleGraphError{State: state, Cause: manifestErr}
	}
	inputsDrifted := generation.ManifestHash != manifest.ManifestHash ||
		generation.ConfigurationDigest != manifest.ConfigurationDigest
	// Only a promoted generation answers. With no current one the newest of
	// any status is active, and a failed or abandoned build carries the very
	// manifest it was cancelled on — answering from it served a cancelled
	// extraction as the graph.
	answerable := generation.Status == GraphCurrent || generation.Status == GraphPartial
	if generation.ExtractorVersion == ExtractorVersion && !inputsDrifted && answerable {
		return generation, state, nil
	}
	// Checked before the status write below: a non-refreshing reader must be
	// side-effect free, so it may not even downgrade the stored status.
	if s.noRefreshOnRead {
		state.Freshness = FreshnessStale
		return GraphGeneration{}, state, &StaleGraphError{State: state, Cause: errRefreshDisabled}
	}
	// Only drifted inputs make a generation stale for every reader. An
	// extractor mismatch is this build's view alone: the generation is still
	// current for the extractor that built it, and a rolled-back build reads
	// it again. Labelling it stale made the restored build rescan too.
	if generation.Status == GraphCurrent && inputsDrifted {
		_, _ = s.db.ExecContext(ctx, `UPDATE graph_generations SET status = 'stale' WHERE id = ? AND status = 'current'`, generation.ID)
	}
	// The repair outlives the reader that started it. A full extraction can
	// take longer than a caller's deadline — an extractor migration of this
	// repository measured ~45s against the deploy journey's 30s client — and
	// cancelling it discards the whole generation, so every later reader
	// starts it again and none finishes. Correctness does not rest on the
	// caller: the build revalidates its manifest and promotes atomically.
	refreshCtx := context.WithoutCancel(ctx)
	refresh, refreshErr := s.refreshRepository(refreshCtx, identity.RootPath)
	if refreshErr != nil {
		state.Freshness = FreshnessStale
		return GraphGeneration{}, state, &StaleGraphError{State: state, Cause: refreshErr}
	}
	if !refresh.Changed {
		// A stale status with identical bytes still needs a promoted generation;
		// returning it would make the label rather than the read-time check the
		// authority. A full scan creates that generation atomically.
		if _, scanErr := s.scanRepository(refreshCtx, identity.RootPath, nil); scanErr != nil {
			state.Freshness = FreshnessStale
			return GraphGeneration{}, state, &StaleGraphError{State: state, Cause: scanErr}
		}
	}
	generation, err = s.activeGeneration(refreshCtx, identity.WorktreeID)
	if err == sql.ErrNoRows {
		missing := missingGraph(identity)
		return GraphGeneration{}, missing.State, missing
	}
	if err != nil {
		return GraphGeneration{}, state, err
	}
	current, err := BuildScanManifest(identity.RootPath)
	if err != nil || current.ManifestHash != generation.ManifestHash || current.ConfigurationDigest != generation.ConfigurationDigest {
		if err == nil {
			err = fmt.Errorf("repository changed again while refreshing")
		}
		state = generationState(generation)
		state.Freshness = FreshnessStale
		return GraphGeneration{}, state, &StaleGraphError{State: state, Cause: err}
	}
	return generation, generationState(generation), nil
}
