package knowledge

import (
	"context"
	"fmt"
	"path"
	"sort"
	"strings"
	"time"
)

const (
	MaxChangedImpactFiles   = 50
	MaxChangedImpactSymbols = 200
	MaxChangedImpactDepth   = 3
	changedImpactTimeBudget = 45 * time.Second
)

// ImpactRequestError is a caller-correctable refusal. HTTP maps it to 422 and
// the CLI prints the same reason; bounds and unsafe paths must not degrade into
// a truncated success.
type ImpactRequestError struct {
	Reason string
}

func (e *ImpactRequestError) Error() string { return e.Reason }

type ChangedImpactRequest struct {
	Files     []string `json:"files"`
	SymbolIDs []string `json:"symbol_ids,omitempty"`
	MaxDepth  int      `json:"max_depth,omitempty"`
}

type ImpactProvenance struct {
	GraphVersion      string            `json:"graph_version"`
	BaseCommit        string            `json:"base_commit"`
	WorktreeStateHash string            `json:"worktree_state_hash"`
	ExtractorVersion  string            `json:"extractor_version"`
	Extractors        map[string]string `json:"extractors"`
}

type ImpactPropagation struct {
	DirectCallers      []ImpactNode  `json:"direct_callers"`
	AffectedCallers    []ImpactNode  `json:"affected_callers"`
	ModuleDependants   []ImpactNode  `json:"module_dependants"`
	OperationalEffects []ImpactNode  `json:"operational_effects"`
	RelatedTests       []RelatedTest `json:"related_tests"`
}

// ChangedImpactResponse is the one bounded answer Agent Console needs before
// editing. Unknown files and graph limitations remain first-class output; an
// empty list is never allowed to masquerade as proof of no impact.
type ChangedImpactResponse struct {
	Query                     QueryMeta                  `json:"query"`
	Generation                RepositoryState            `json:"generation"`
	Provenance                ImpactProvenance           `json:"provenance"`
	ChangedSymbols            []GraphSymbol              `json:"changed_symbols"`
	Propagation               ImpactPropagation          `json:"propagation"`
	UnresolvedFiles           []string                   `json:"unresolved_files"`
	Unresolved                []string                   `json:"unresolved"`
	ValidationRecommendations []ValidationRecommendation `json:"validation_recommendations"`
	Resolution                ResolutionMeta             `json:"resolution"`
	Limitations               []string                   `json:"limitations"`
	Truncated                 bool                       `json:"truncated"`
}

// DefaultChangedImpactLimits makes the public depth-3 ceiling explicit at the
// call site. ChangedImpactAnalysis never widens a caller's operational budget.
func DefaultChangedImpactLimits() GraphLimits {
	limits := DefaultGraphLimits()
	limits.MaxDepth = MaxChangedImpactDepth
	return limits
}

func (s *Store) ChangedImpactAnalysis(ctx context.Context, identity RepositoryIdentity, request ChangedImpactRequest, limits GraphLimits) (ChangedImpactResponse, error) {
	if _, hasDeadline := ctx.Deadline(); !hasDeadline {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, changedImpactTimeBudget)
		defer cancel()
	}
	files, err := normalizeChangedImpactFiles(request.Files)
	if err != nil {
		return ChangedImpactResponse{}, err
	}
	symbolIDs := uniqueNonEmpty(request.SymbolIDs)
	if len(files) == 0 && len(symbolIDs) == 0 {
		return ChangedImpactResponse{}, &ImpactRequestError{Reason: "impact analysis requires at least one changed file or symbol id"}
	}
	if len(symbolIDs) > MaxChangedImpactSymbols {
		return ChangedImpactResponse{}, &ImpactRequestError{Reason: fmt.Sprintf("request contains more than %d symbol_ids", MaxChangedImpactSymbols)}
	}
	depth := request.MaxDepth
	if depth <= 0 {
		depth = 2
	}
	if depth > MaxChangedImpactDepth {
		return ChangedImpactResponse{}, &ImpactRequestError{Reason: fmt.Sprintf("max_depth must be at most %d", MaxChangedImpactDepth)}
	}
	if limits.MaxDepth <= 0 {
		limits = DefaultChangedImpactLimits()
	}
	if depth > limits.MaxDepth {
		return ChangedImpactResponse{}, &ImpactRequestError{Reason: fmt.Sprintf("max_depth %d exceeds the caller's traversal limit %d", depth, limits.MaxDepth)}
	}
	depth, limits = normalizeImpactBounds(depth, limits)

	generation, state, err := s.freshGeneration(ctx, identity)
	if err != nil {
		return ChangedImpactResponse{}, err
	}
	resolvedIDs, unresolvedFiles, fileExpansionTruncated, err := s.symbolIDsForFiles(ctx, generation.ID, files, MaxChangedImpactSymbols-len(symbolIDs))
	if err != nil {
		return ChangedImpactResponse{}, err
	}
	symbolIDs = uniqueNonEmpty(append(symbolIDs, resolvedIDs...))

	impact, err := s.impactAnalysisForGeneration(ctx, generation, state, symbolIDs, depth, limits)
	if err != nil {
		return ChangedImpactResponse{}, err
	}
	resolution := impact.Resolution
	if len(symbolIDs) == 0 {
		resolution = ResolutionMeta{Status: "unavailable", Methods: []ResolutionStatus{ResolutionUnresolved}}
	}
	limitations := append([]string{}, impact.Limitations...)
	if len(files) > 0 {
		limitations = appendUniqueString(limitations, "file anchors resolve every symbol in each file; affected reach is a conservative file-granularity over-approximation")
	}
	if fileExpansionTruncated {
		limitations = appendUniqueString(limitations, fmt.Sprintf("file-anchor symbol expansion was truncated at the %d-symbol batch bound", MaxChangedImpactSymbols))
	}
	extractors := make(map[string]string, len(generation.Capabilities))
	for name, grade := range generation.Capabilities {
		extractors[name] = grade
	}
	return ChangedImpactResponse{
		Query:      QueryMeta{Type: "changed_impact", Roots: append(append([]string{}, files...), request.SymbolIDs...)},
		Generation: state,
		Provenance: ImpactProvenance{
			GraphVersion: generation.ID, BaseCommit: generation.BaseCommit,
			WorktreeStateHash: generation.WorktreeStateHash,
			ExtractorVersion:  generation.ExtractorVersion, Extractors: extractors,
		},
		ChangedSymbols: append([]GraphSymbol{}, impact.Result.Changed...),
		Propagation: ImpactPropagation{
			DirectCallers:      append([]ImpactNode{}, impact.Result.DirectCallers...),
			AffectedCallers:    append([]ImpactNode{}, impact.Result.AffectedCallers...),
			ModuleDependants:   append([]ImpactNode{}, impact.Result.ModuleDependants...),
			OperationalEffects: append([]ImpactNode{}, impact.Result.OperationalEffects...),
			RelatedTests:       append([]RelatedTest{}, impact.Result.RelatedTests...),
		},
		UnresolvedFiles:           append([]string{}, unresolvedFiles...),
		Unresolved:                append([]string{}, impact.Result.Unresolved...),
		ValidationRecommendations: append([]ValidationRecommendation{}, impact.Result.ValidationRecommendations...),
		Resolution:                resolution, Limitations: limitations, Truncated: impact.Truncated || fileExpansionTruncated,
	}, nil
}

func normalizeChangedImpactFiles(files []string) ([]string, error) {
	if len(files) > MaxChangedImpactFiles {
		return nil, &ImpactRequestError{Reason: fmt.Sprintf("changed set contains more than %d files", MaxChangedImpactFiles)}
	}
	seen := make(map[string]bool, len(files))
	result := make([]string, 0, len(files))
	for _, raw := range files {
		normalized := path.Clean(strings.ReplaceAll(strings.TrimSpace(raw), "\\", "/"))
		if normalized == "." || normalized == ".." || strings.HasPrefix(normalized, "../") || strings.HasPrefix(normalized, "/") {
			return nil, &ImpactRequestError{Reason: fmt.Sprintf("changed file %q must be repository-relative", raw)}
		}
		if !seen[normalized] {
			seen[normalized] = true
			result = append(result, normalized)
		}
	}
	sort.Strings(result)
	return result, nil
}

func uniqueNonEmpty(values []string) []string {
	seen := make(map[string]bool, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" && !seen[value] {
			seen[value] = true
			result = append(result, value)
		}
	}
	return result
}

func (s *Store) symbolIDsForFiles(ctx context.Context, generationID string, files []string, limit int) ([]string, []string, bool, error) {
	if len(files) == 0 {
		return nil, nil, false, nil
	}
	filePlaceholders := strings.TrimSuffix(strings.Repeat("?,", len(files)), ",")
	baseArgs := make([]any, 0, len(files)+1)
	baseArgs = append(baseArgs, generationID)
	for _, file := range files {
		baseArgs = append(baseArgs, file)
	}
	countQuery := `SELECT o.file_path, COUNT(DISTINCT s.id) FROM repository_symbols s
		JOIN symbol_occurrences o ON o.symbol_id = s.id
		WHERE o.generation_id = ? AND o.file_path IN (` + filePlaceholders + `)
		GROUP BY o.file_path ORDER BY o.file_path`
	countRows, err := s.db.QueryContext(ctx, countQuery, baseArgs...)
	if err != nil {
		return nil, nil, false, err
	}
	totals := make(map[string]int, len(files))
	for countRows.Next() {
		var file string
		var total int
		if err := countRows.Scan(&file, &total); err != nil {
			countRows.Close()
			return nil, nil, false, err
		}
		totals[file] = total
	}
	if err := countRows.Err(); err != nil {
		countRows.Close()
		return nil, nil, false, err
	}
	if err := countRows.Close(); err != nil {
		return nil, nil, false, err
	}

	selectedByFile := make(map[string]int, len(files))
	var symbolIDs []string
	if limit > 0 {
		query := `SELECT o.file_path, s.id FROM repository_symbols s
			JOIN symbol_occurrences o ON o.symbol_id = s.id
			WHERE o.generation_id = ? AND o.file_path IN (` + filePlaceholders + `)
			GROUP BY o.file_path, s.id
			ORDER BY o.file_path, MIN(o.start_line), s.id LIMIT ?`
		args := append(append([]any{}, baseArgs...), limit)
		rows, queryErr := s.db.QueryContext(ctx, query, args...)
		if queryErr != nil {
			return nil, nil, false, queryErr
		}
		for rows.Next() {
			var file, symbolID string
			if err := rows.Scan(&file, &symbolID); err != nil {
				rows.Close()
				return nil, nil, false, err
			}
			selectedByFile[file]++
			symbolIDs = append(symbolIDs, symbolID)
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return nil, nil, false, err
		}
		if err := rows.Close(); err != nil {
			return nil, nil, false, err
		}
	}

	var unresolved []string
	truncated := false
	for _, file := range files {
		total := totals[file]
		selected := selectedByFile[file]
		if total == 0 {
			unresolved = append(unresolved, file+": no symbols in the current graph generation")
		} else if selected < total {
			truncated = true
			unresolved = append(unresolved, fmt.Sprintf("%s: symbol expansion truncated; selected %d of %d symbols at the batch bound", file, selected, total))
		}
	}
	return uniqueNonEmpty(symbolIDs), unresolved, truncated, nil
}
