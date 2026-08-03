package knowledge

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"

	statepkg "github.com/openexec/openexec/pkg/db/state"
)

const RepositoryContextSchemaVersion = 1

const (
	defaultRepositoryContextSymbolLimit     = 50
	defaultRepositoryContextDependencyLimit = 100
)

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

// GraphTotals makes the projection honest about its own lossiness: what the
// graph holds versus what this bounded view shows.
type GraphTotals struct {
	Symbols     int `json:"symbols"`
	Modules     int `json:"modules"`
	Edges       int `json:"edges"`
	ImportEdges int `json:"import_edges"`
}

type WorktreeProjection struct {
	State      string `json:"state"`
	DirtyCount int    `json:"dirty_count"`
	StateHash  string `json:"state_hash"`
}

type ProjectionProvenance struct {
	BaseCommit string             `json:"base_commit"`
	Worktree   WorktreeProjection `json:"worktree"`
	Extractors map[string]string  `json:"extractors"`
}

type SelectionScope struct {
	Scope          string `json:"scope"`
	Limit          int    `json:"limit"`
	Returned       int    `json:"returned"`
	Total          int    `json:"total"`
	Truncated      bool   `json:"truncated"`
	TruncatedCount int    `json:"truncated_count"`
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
	Totals             *GraphTotals                `json:"totals,omitempty"`
	Provenance         ProjectionProvenance        `json:"provenance"`
	Selections         map[string]SelectionScope   `json:"selections"`
	DeadCodeCandidates []InsightSymbol             `json:"dead_code_candidates,omitempty"`
	Hotspots           []InsightSymbol             `json:"hotspots,omitempty"`
	ModuleSketch       []ModuleFlow                `json:"module_sketch,omitempty"`
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
		Freshness:          state.Freshness,
		ResolvedSymbols:    []SafeSymbolProjection{},
		ModuleDependencies: []SafeModuleDependency{},
		ValidationSummary: ValidationSummaryProjection{
			Verified:    []string{},
			NotVerified: []string{},
		},
		Provenance: ProjectionProvenance{
			BaseCommit: state.BaseCommit,
			Worktree:   worktreeProjection(ctx, identity.RootPath, state.WorktreeStateHash),
			Extractors: map[string]string{},
		},
		Selections:        map[string]SelectionScope{},
		Limitations:       []string{},
		OpenExecReference: OpenExecReference{TaskID: taskID, RunID: runID, PlanRevisionID: planRevisionID},
	}
	var generationLimitationsJSON string
	if err := s.db.QueryRowContext(ctx, `SELECT limitations FROM graph_generations WHERE id = ?`, state.GraphVersion).Scan(&generationLimitationsJSON); err != nil {
		return RepositoryContextProjection{}, fmt.Errorf("load graph limitations: %w", err)
	}
	if err := json.Unmarshal([]byte(generationLimitationsJSON), &projection.Limitations); err != nil {
		return RepositoryContextProjection{}, fmt.Errorf("decode graph limitations: %w", err)
	}
	var capabilitiesJSON string
	if err := s.db.QueryRowContext(ctx, `SELECT capabilities FROM graph_generations WHERE id = ?`, state.GraphVersion).Scan(&capabilitiesJSON); err != nil {
		return RepositoryContextProjection{}, fmt.Errorf("load graph capabilities: %w", err)
	}
	if err := json.Unmarshal([]byte(capabilitiesJSON), &projection.Provenance.Extractors); err != nil {
		return RepositoryContextProjection{}, fmt.Errorf("decode graph capabilities: %w", err)
	}
	if projection.Limitations == nil {
		projection.Limitations = []string{}
	}
	if err := s.attachRepositoryInsights(ctx, state.GraphVersion, &projection); err != nil {
		return RepositoryContextProjection{}, err
	}

	var candidates []SymbolCandidate
	if len(names) == 0 {
		var truncated bool
		candidates, truncated, err = s.defaultRepositoryContextSymbols(ctx, state.GraphVersion, defaultRepositoryContextSymbolLimit)
		if err != nil {
			return RepositoryContextProjection{}, err
		}
		if truncated {
			projection.Limitations = append(projection.Limitations, "repository overview is bounded to 50 representative symbols")
		}
	} else {
		for _, name := range names {
			resolved, resolveErr := s.ResolveGraphSymbol(ctx, identity, name, "", "", 20)
			if resolveErr != nil {
				projection.Limitations = append(projection.Limitations, name+": resolution failed")
				continue
			}
			if resolved.Result.Candidate == nil {
				projection.Limitations = append(projection.Limitations, name+": "+resolved.Result.Status)
				continue
			}
			candidates = append(candidates, *resolved.Result.Candidate)
		}
	}
	symbolTotal := len(names)
	symbolScope := "explicitly requested symbols"
	symbolLimit := len(names)
	if len(names) == 0 {
		symbolTotal = projection.Totals.Symbols
		symbolScope = "representative symbols selected from all repository symbols"
		symbolLimit = defaultRepositoryContextSymbolLimit
	}
	projection.Selections["resolved_symbols"] = selectionScope(symbolScope, symbolLimit, len(candidates), symbolTotal)

	seenDependencies := make(map[string]bool)
	seenDependencyFiles := make(map[string]bool)
	for _, candidate := range candidates {
		projection.ResolvedSymbols = append(projection.ResolvedSymbols, SafeSymbolProjection{
			SymbolID: candidate.Symbol.ID, DisplayName: candidate.Symbol.DisplayName,
			Kind:             candidate.Symbol.Kind,
			SafeLocation:     candidate.Occurrence.FilePath + ":" + strconv.Itoa(candidate.Occurrence.StartLine),
			ResolutionStatus: string(candidate.Occurrence.Resolution),
		})
		if seenDependencyFiles[candidate.Occurrence.FilePath] || len(projection.ModuleDependencies) >= defaultRepositoryContextDependencyLimit {
			continue
		}
		seenDependencyFiles[candidate.Occurrence.FilePath] = true
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
			if len(projection.ModuleDependencies) >= defaultRepositoryContextDependencyLimit {
				break
			}
		}
	}
	projection.Selections["module_dependencies"] = selectionScope("direct module dependencies selected from repository import edges", defaultRepositoryContextDependencyLimit, len(projection.ModuleDependencies), projection.Totals.ImportEdges)
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

func selectionScope(scope string, limit, returned, total int) SelectionScope {
	if total < returned {
		total = returned
	}
	omitted := total - returned
	return SelectionScope{Scope: scope, Limit: limit, Returned: returned, Total: total, Truncated: omitted > 0, TruncatedCount: omitted}
}

func (s *Store) defaultRepositoryContextSymbols(ctx context.Context, generationID string, limit int) ([]SymbolCandidate, bool, error) {
	if limit <= 0 {
		limit = defaultRepositoryContextSymbolLimit
	}
	load := func(representativeOnly bool) ([]SymbolCandidate, error) {
		filter := ""
		if representativeOnly {
			filter = ` AND (o.exported = 1 OR o.file_path = 'main.go' OR o.file_path LIKE 'cmd/%/main.go' OR o.file_path LIKE '%/main.ts' OR o.file_path LIKE '%/main.tsx' OR o.file_path LIKE '%/index.ts' OR o.file_path LIKE '%/index.tsx')`
		}
		query := `SELECT s.id, s.repository_id, s.language, s.kind, s.display_name, s.qualified_name,
			o.symbol_id, o.generation_id, o.node_id, o.file_path, o.start_line, o.end_line,
			o.start_byte, o.end_byte, o.signature, o.file_content_hash, o.source_range_hash,
			o.exported, o.resolution_status,
			(SELECT COUNT(*) FROM graph_edges e WHERE e.generation_id = o.generation_id AND e.to_node_id = o.node_id AND e.edge_type IN ('calls','references')) AS inbound
			FROM repository_symbols s JOIN symbol_occurrences o ON o.symbol_id = s.id
			WHERE o.generation_id = ?` + filter + `
			ORDER BY
				CASE WHEN o.file_path = 'main.go' OR o.file_path LIKE 'cmd/%/main.go' OR o.file_path LIKE '%/main.ts' OR o.file_path LIKE '%/main.tsx' OR o.file_path LIKE '%/index.ts' OR o.file_path LIKE '%/index.tsx' THEN 0 ELSE 1 END,
				inbound DESC, o.exported DESC, o.file_path, o.start_line
			LIMIT ?`
		rows, err := s.db.QueryContext(ctx, query, generationID, limit+1)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		var candidates []SymbolCandidate
		for rows.Next() {
			var candidate SymbolCandidate
			var exported, inbound int
			if err := rows.Scan(&candidate.Symbol.ID, &candidate.Symbol.RepositoryID,
				&candidate.Symbol.Language, &candidate.Symbol.Kind, &candidate.Symbol.DisplayName,
				&candidate.Symbol.QualifiedName, &candidate.Occurrence.SymbolID,
				&candidate.Occurrence.GenerationID, &candidate.Occurrence.NodeID,
				&candidate.Occurrence.FilePath, &candidate.Occurrence.StartLine,
				&candidate.Occurrence.EndLine, &candidate.Occurrence.StartByte,
				&candidate.Occurrence.EndByte, &candidate.Occurrence.Signature,
				&candidate.Occurrence.FileHash, &candidate.Occurrence.RangeHash, &exported,
				&candidate.Occurrence.Resolution, &inbound); err != nil {
				return nil, err
			}
			candidate.Occurrence.Exported = exported != 0
			candidates = append(candidates, candidate)
		}
		return candidates, rows.Err()
	}

	candidates, err := load(true)
	if err != nil {
		return nil, false, fmt.Errorf("select representative repository symbols: %w", err)
	}
	if len(candidates) == 0 {
		candidates, err = load(false)
		if err != nil {
			return nil, false, fmt.Errorf("select repository symbols: %w", err)
		}
	}
	truncated := len(candidates) > limit
	if truncated {
		candidates = candidates[:limit]
	}
	return candidates, truncated, nil
}
