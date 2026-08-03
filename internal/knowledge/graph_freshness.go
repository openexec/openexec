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
	if err != nil {
		return GraphGeneration{}, RepositoryState{}, err
	}
	manifest, manifestErr := BuildScanManifest(identity.RootPath)
	state := generationState(generation)
	if manifestErr != nil {
		state.Freshness = FreshnessStale
		return GraphGeneration{}, state, &StaleGraphError{State: state, Cause: manifestErr}
	}
	if generation.ExtractorVersion == ExtractorVersion &&
		generation.ManifestHash == manifest.ManifestHash &&
		generation.ConfigurationDigest == manifest.ConfigurationDigest &&
		generation.Status != GraphStale && generation.Status != GraphSuperseded {
		return generation, state, nil
	}
	if generation.Status == GraphCurrent {
		_, _ = s.db.ExecContext(ctx, `UPDATE graph_generations SET status = 'stale' WHERE id = ? AND status = 'current'`, generation.ID)
	}
	refresh, refreshErr := s.refreshRepository(ctx, identity.RootPath)
	if refreshErr != nil {
		state.Freshness = FreshnessStale
		return GraphGeneration{}, state, &StaleGraphError{State: state, Cause: refreshErr}
	}
	if !refresh.Changed {
		// A stale status with identical bytes still needs a promoted generation;
		// returning it would make the label rather than the read-time check the
		// authority. A full scan creates that generation atomically.
		if _, scanErr := s.scanRepository(ctx, identity.RootPath, nil); scanErr != nil {
			state.Freshness = FreshnessStale
			return GraphGeneration{}, state, &StaleGraphError{State: state, Cause: scanErr}
		}
	}
	generation, err = s.activeGeneration(ctx, identity.WorktreeID)
	if err != nil {
		if err == sql.ErrNoRows {
			state.Freshness = FreshnessMissing
		}
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
