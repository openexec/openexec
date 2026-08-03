package knowledge

import (
	"context"
	"encoding/json"
	"sort"
	"strconv"

	statepkg "github.com/openexec/openexec/pkg/db/state"
)

const RepositoryContextSchemaVersion = 1

type SafeSymbolProjection struct {
	SymbolID         string `json:"symbol_id"`
	DisplayName      string `json:"display_name"`
	Kind             string `json:"kind"`
	SafeLocation     string `json:"safe_location"`
	ResolutionStatus string `json:"resolution_status"`
}

type SafeModuleDependency struct {
	From             string `json:"from"`
	To               string `json:"to"`
	ResolutionStatus string `json:"resolution_status"`
}

type ValidationSummaryProjection struct {
	Verified    []string `json:"verified"`
	NotVerified []string `json:"not_verified"`
	CanComplete bool     `json:"can_complete"`
}

type OpenExecReference struct {
	TaskID          string `json:"task_id,omitempty"`
	RunID           string `json:"run_id,omitempty"`
	PlanRevisionID  string `json:"plan_revision_id,omitempty"`
	ResourceVersion string `json:"resource_version"`
}

type RepositoryContextProjection struct {
	SchemaVersion      int                         `json:"schema_version"`
	SourceSystem       string                      `json:"source_system"`
	RepositoryID       string                      `json:"repository_id"`
	CheckoutID         string                      `json:"checkout_id"`
	WorktreeID         string                      `json:"worktree_id"`
	GraphVersion       string                      `json:"graph_version"`
	Freshness          Freshness                   `json:"freshness"`
	ResolvedSymbols    []SafeSymbolProjection      `json:"resolved_symbols"`
	ModuleDependencies []SafeModuleDependency      `json:"module_dependencies"`
	ValidationSummary  ValidationSummaryProjection `json:"validation_summary"`
	Limitations        []string                    `json:"limitations"`
	OpenExecReference  OpenExecReference           `json:"openexec_reference"`
}

func (s *Store) BuildRepositoryContext(ctx context.Context, identity RepositoryIdentity, names []string, taskID, runID, planRevisionID string, report *statepkg.CompletionReport) (RepositoryContextProjection, error) {
	state, err := s.CurrentRepositoryState(ctx, identity)
	if err != nil {
		return RepositoryContextProjection{}, err
	}
	projection := RepositoryContextProjection{
		SchemaVersion: RepositoryContextSchemaVersion, SourceSystem: "openexec",
		RepositoryID: identity.RepositoryID, CheckoutID: identity.CheckoutID,
		WorktreeID: identity.WorktreeID, GraphVersion: state.GraphVersion,
		Freshness:         state.Freshness,
		OpenExecReference: OpenExecReference{TaskID: taskID, RunID: runID, PlanRevisionID: planRevisionID},
	}
	seenDependencies := make(map[string]bool)
	for _, name := range names {
		resolved, err := s.ResolveGraphSymbol(ctx, identity, name, "", "", 20)
		if err != nil {
			projection.Limitations = append(projection.Limitations, name+": resolution failed")
			continue
		}
		if resolved.Result.Candidate == nil {
			projection.Limitations = append(projection.Limitations, name+": "+resolved.Result.Status)
			continue
		}
		candidate := resolved.Result.Candidate
		projection.ResolvedSymbols = append(projection.ResolvedSymbols, SafeSymbolProjection{
			SymbolID: candidate.Symbol.ID, DisplayName: candidate.Symbol.DisplayName,
			Kind:             candidate.Symbol.Kind,
			SafeLocation:     candidate.Occurrence.FilePath + ":" + strconv.Itoa(candidate.Occurrence.StartLine),
			ResolutionStatus: string(candidate.Occurrence.Resolution),
		})
		dependencies, err := s.FindModuleDependencies(ctx, identity, candidate.Occurrence.FilePath, false, 1, DefaultGraphLimits())
		if err != nil {
			projection.Limitations = append(projection.Limitations, candidate.Occurrence.FilePath+": module dependencies unavailable")
			continue
		}
		for _, edge := range dependencies.Result.Edges {
			var to string
			for _, node := range dependencies.Result.Nodes {
				if node.ID == edge.ToNodeID {
					to = node.QualifiedName
					break
				}
			}
			if to == "" {
				continue
			}
			key := candidate.Occurrence.FilePath + "\x00" + to
			if seenDependencies[key] {
				continue
			}
			seenDependencies[key] = true
			projection.ModuleDependencies = append(projection.ModuleDependencies, SafeModuleDependency{From: candidate.Occurrence.FilePath, To: to, ResolutionStatus: string(edge.Resolution)})
		}
	}
	if report != nil {
		projection.ValidationSummary.CanComplete = report.CanComplete
		for _, claim := range report.Verified {
			projection.ValidationSummary.Verified = append(projection.ValidationSummary.Verified, claim.Criterion)
		}
		for _, claim := range report.NotVerified {
			projection.ValidationSummary.NotVerified = append(projection.ValidationSummary.NotVerified, claim.Criterion+" ("+claim.Status+")")
		}
	}
	if state.Freshness != FreshnessCurrent {
		projection.Limitations = append(projection.Limitations, "graph is "+string(state.Freshness)+"; use repository inspection for conclusions")
	}
	sort.Slice(projection.ResolvedSymbols, func(i, j int) bool {
		return projection.ResolvedSymbols[i].SafeLocation < projection.ResolvedSymbols[j].SafeLocation
	})
	sort.Slice(projection.ModuleDependencies, func(i, j int) bool {
		if projection.ModuleDependencies[i].From == projection.ModuleDependencies[j].From {
			return projection.ModuleDependencies[i].To < projection.ModuleDependencies[j].To
		}
		return projection.ModuleDependencies[i].From < projection.ModuleDependencies[j].From
	})
	// The resource version identifies the complete lossy projection, not merely
	// its graph generation. Different symbol selections or claim details must
	// never reuse an ETag for different bytes.
	encoded, err := json.Marshal(projection)
	if err != nil {
		return RepositoryContextProjection{}, err
	}
	projection.OpenExecReference.ResourceVersion = stableID("resource", string(encoded))
	return projection, nil
}
