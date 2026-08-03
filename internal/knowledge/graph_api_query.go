package knowledge

import (
	"context"
	"sort"
	"strings"
)

type SymbolSearchResult struct {
	Candidates []SymbolCandidate `json:"candidates"`
}

type SymbolDetailResult struct {
	Symbol     GraphSymbol        `json:"symbol"`
	Occurrence SymbolOccurrence   `json:"occurrence"`
	History    []SymbolOccurrence `json:"occurrences"`
	Lineage    []SymbolLineage    `json:"lineage"`
}

type SymbolLineage struct {
	PreviousSymbolID string `json:"previous_symbol_id,omitempty"`
	ContinuityStatus string `json:"continuity_status"`
	ResolutionMethod string `json:"resolution_method"`
	GenerationID     string `json:"generation_id"`
}

type RelationshipResult struct {
	Root  GraphNode   `json:"root"`
	Nodes []GraphNode `json:"nodes"`
	Edges []GraphEdge `json:"edges"`
}

func (s *Store) FindGraphSymbols(ctx context.Context, identity RepositoryIdentity, query, file, kind string, page, pageSize int) (QueryEnvelope[SymbolSearchResult], error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 25
	}
	if pageSize > 100 {
		pageSize = 100
	}
	generation, state, err := s.freshGeneration(ctx, identity)
	if err != nil {
		return QueryEnvelope[SymbolSearchResult]{}, err
	}
	where := ` WHERE o.generation_id = ?`
	args := []any{generation.ID}
	if query != "" {
		where += ` AND (LOWER(s.display_name) LIKE ? OR LOWER(s.qualified_name) LIKE ?)`
		pattern := "%" + strings.ToLower(query) + "%"
		args = append(args, pattern, pattern)
	}
	if file != "" {
		where += ` AND o.file_path = ?`
		args = append(args, file)
	}
	if kind != "" {
		where += ` AND s.kind = ?`
		args = append(args, kind)
	}
	join := ` FROM repository_symbols s JOIN symbol_occurrences o ON o.symbol_id = s.id`
	var total int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*)`+join+where, args...).Scan(&total); err != nil {
		return QueryEnvelope[SymbolSearchResult]{}, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT s.id,s.repository_id,s.language,s.kind,s.display_name,s.qualified_name,o.symbol_id,o.generation_id,o.node_id,o.file_path,o.start_line,o.end_line,o.start_byte,o.end_byte,o.signature,o.file_content_hash,o.source_range_hash,o.exported,o.resolution_status`+join+where+` ORDER BY s.display_name,o.file_path,o.start_line LIMIT ? OFFSET ?`, append(args, pageSize, (page-1)*pageSize)...)
	if err != nil {
		return QueryEnvelope[SymbolSearchResult]{}, err
	}
	defer rows.Close()
	var candidates []SymbolCandidate
	var methods []ResolutionStatus
	for rows.Next() {
		candidate, err := scanSymbolCandidate(rows)
		if err != nil {
			return QueryEnvelope[SymbolSearchResult]{}, err
		}
		candidates = append(candidates, candidate)
		methods = append(methods, candidate.Occurrence.Resolution)
	}
	if err := rows.Err(); err != nil {
		return QueryEnvelope[SymbolSearchResult]{}, err
	}
	pages := 0
	if total > 0 {
		pages = (total + pageSize - 1) / pageSize
	}
	return QueryEnvelope[SymbolSearchResult]{Query: QueryMeta{Type: "find_symbols", Roots: []string{query}}, Generation: state, Result: SymbolSearchResult{Candidates: candidates}, Resolution: ResolutionMeta{Status: "bounded", Methods: uniqueMethods(methods)}, Limitations: generation.Limitations, Truncated: page*pageSize < total, Pagination: &PaginationMeta{Page: page, PageSize: pageSize, Total: total, TotalPages: pages}}, nil
}

func scanSymbolCandidate(row rowScanner) (SymbolCandidate, error) {
	var candidate SymbolCandidate
	var exported int
	err := row.Scan(&candidate.Symbol.ID, &candidate.Symbol.RepositoryID, &candidate.Symbol.Language, &candidate.Symbol.Kind, &candidate.Symbol.DisplayName, &candidate.Symbol.QualifiedName, &candidate.Occurrence.SymbolID, &candidate.Occurrence.GenerationID, &candidate.Occurrence.NodeID, &candidate.Occurrence.FilePath, &candidate.Occurrence.StartLine, &candidate.Occurrence.EndLine, &candidate.Occurrence.StartByte, &candidate.Occurrence.EndByte, &candidate.Occurrence.Signature, &candidate.Occurrence.FileHash, &candidate.Occurrence.RangeHash, &exported, &candidate.Occurrence.Resolution)
	candidate.Occurrence.Exported = exported != 0
	return candidate, err
}

func (s *Store) GraphSymbolDetail(ctx context.Context, identity RepositoryIdentity, symbolID string) (QueryEnvelope[*SymbolDetailResult], error) {
	generation, state, err := s.freshGeneration(ctx, identity)
	if err != nil {
		return QueryEnvelope[*SymbolDetailResult]{}, err
	}
	row := s.db.QueryRowContext(ctx, `SELECT s.id,s.repository_id,s.language,s.kind,s.display_name,s.qualified_name,o.symbol_id,o.generation_id,o.node_id,o.file_path,o.start_line,o.end_line,o.start_byte,o.end_byte,o.signature,o.file_content_hash,o.source_range_hash,o.exported,o.resolution_status FROM repository_symbols s JOIN symbol_occurrences o ON o.symbol_id=s.id WHERE s.id=? AND o.generation_id=?`, symbolID, generation.ID)
	candidate, err := scanSymbolCandidate(row)
	if err != nil {
		return QueryEnvelope[*SymbolDetailResult]{}, err
	}
	result := &SymbolDetailResult{Symbol: candidate.Symbol, Occurrence: candidate.Occurrence}
	occurrences, err := s.db.QueryContext(ctx, `SELECT o.symbol_id,o.generation_id,o.node_id,o.file_path,o.start_line,o.end_line,o.start_byte,o.end_byte,o.signature,o.file_content_hash,o.source_range_hash,o.exported,o.resolution_status FROM symbol_occurrences o JOIN graph_generations g ON g.id=o.generation_id WHERE o.symbol_id=? ORDER BY g.created_at DESC LIMIT 20`, symbolID)
	if err != nil {
		return QueryEnvelope[*SymbolDetailResult]{}, err
	}
	for occurrences.Next() {
		var item SymbolOccurrence
		var exported int
		if err := occurrences.Scan(&item.SymbolID, &item.GenerationID, &item.NodeID, &item.FilePath, &item.StartLine, &item.EndLine, &item.StartByte, &item.EndByte, &item.Signature, &item.FileHash, &item.RangeHash, &exported, &item.Resolution); err != nil {
			occurrences.Close()
			return QueryEnvelope[*SymbolDetailResult]{}, err
		}
		item.Exported = exported != 0
		result.History = append(result.History, item)
	}
	if err := occurrences.Close(); err != nil {
		return QueryEnvelope[*SymbolDetailResult]{}, err
	}
	lineage, err := s.db.QueryContext(ctx, `SELECT COALESCE(previous_symbol_id,''),continuity_status,resolution_method,generation_id FROM symbol_lineage WHERE symbol_id=? ORDER BY created_at DESC LIMIT 20`, symbolID)
	if err != nil {
		return QueryEnvelope[*SymbolDetailResult]{}, err
	}
	defer lineage.Close()
	for lineage.Next() {
		var item SymbolLineage
		if err := lineage.Scan(&item.PreviousSymbolID, &item.ContinuityStatus, &item.ResolutionMethod, &item.GenerationID); err != nil {
			return QueryEnvelope[*SymbolDetailResult]{}, err
		}
		result.Lineage = append(result.Lineage, item)
	}
	return QueryEnvelope[*SymbolDetailResult]{Query: QueryMeta{Type: "symbol_detail", Roots: []string{symbolID}}, Generation: state, Result: result, Resolution: ResolutionMeta{Status: "resolved", Methods: []ResolutionStatus{candidate.Occurrence.Resolution}}, Limitations: generation.Limitations, Truncated: len(result.History) == 20 || len(result.Lineage) == 20}, lineage.Err()
}

func (s *Store) FindSymbolRelationships(ctx context.Context, identity RepositoryIdentity, symbolID string, incoming bool, depth int, edgeTypes []string, limits GraphLimits) (QueryEnvelope[RelationshipResult], error) {
	if limits.MaxDepth <= 0 {
		limits = DefaultGraphLimits()
	}
	if depth < 1 {
		depth = 1
	}
	if depth > limits.MaxDepth {
		depth = limits.MaxDepth
	}
	generation, state, err := s.freshGeneration(ctx, identity)
	if err != nil {
		return QueryEnvelope[RelationshipResult]{}, err
	}
	_, nodeID, _, err := s.loadImpactRoot(ctx, generation.ID, symbolID)
	if err != nil {
		return QueryEnvelope[RelationshipResult]{}, err
	}
	root, err := s.loadGraphNode(ctx, generation.ID, nodeID)
	if err != nil {
		return QueryEnvelope[RelationshipResult]{}, err
	}
	seen := map[string]bool{root.ID: true}
	frontier := []string{root.ID}
	var nodes []GraphNode
	var edges []GraphEdge
	truncated := false
	for level := 0; level < depth && len(frontier) > 0; level++ {
		var next []string
		for _, current := range frontier {
			remaining := limits.MaxEdges - len(edges)
			if remaining <= 0 {
				truncated = true
				break
			}
			var found []GraphEdge
			if incoming {
				found, err = s.loadIncomingImpactEdges(ctx, generation.ID, current, edgeTypes, remaining)
			} else {
				found, err = s.loadOutgoingImpactEdges(ctx, generation.ID, current, edgeTypes, remaining)
			}
			if err != nil {
				return QueryEnvelope[RelationshipResult]{}, err
			}
			for _, edge := range found {
				edges = append(edges, edge)
				target := edge.ToNodeID
				if incoming {
					target = edge.FromNodeID
				}
				if !seen[target] {
					if len(nodes) >= limits.MaxNodes {
						truncated = true
						continue
					}
					node, loadErr := s.loadGraphNode(ctx, generation.ID, target)
					if loadErr != nil {
						return QueryEnvelope[RelationshipResult]{}, loadErr
					}
					seen[target] = true
					nodes = append(nodes, node)
					next = append(next, target)
				}
			}
		}
		frontier = next
	}
	sort.Slice(nodes, func(i, j int) bool { return nodes[i].QualifiedName < nodes[j].QualifiedName })
	sort.Slice(edges, func(i, j int) bool { return edges[i].ID < edges[j].ID })
	direction := "outgoing"
	if incoming {
		direction = "incoming"
	}
	return QueryEnvelope[RelationshipResult]{Query: QueryMeta{Type: "symbol_relationships_" + direction, Roots: []string{symbolID}}, Generation: state, Result: RelationshipResult{Root: root, Nodes: nodes, Edges: edges}, Resolution: ResolutionMeta{Status: "bounded", Methods: edgeMethods(edges)}, Limitations: generation.Limitations, Truncated: truncated}, nil
}
