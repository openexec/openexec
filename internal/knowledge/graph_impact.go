package knowledge

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
)

type ImpactPath struct {
	FromNodeID string `json:"from_node_id"`
	EdgeID     string `json:"edge_id"`
	EdgeType   string `json:"edge_type"`
	ToNodeID   string `json:"to_node_id"`
	Reason     string `json:"reason"`
}

type ImpactNode struct {
	Node GraphNode    `json:"node"`
	Path []ImpactPath `json:"path"`
}

type RelatedTest struct {
	FilePath string       `json:"file_path"`
	Reason   string       `json:"reason"`
	Path     []ImpactPath `json:"path"`
}

type ValidationRecommendation struct {
	Criterion   string       `json:"criterion"`
	Scope       string       `json:"scope"`
	TestFiles   []string     `json:"test_files,omitempty"`
	GraphPaths  []ImpactPath `json:"graph_paths,omitempty"`
	Limitations []string     `json:"limitations,omitempty"`
}

type ImpactResult struct {
	Changed                   []GraphSymbol              `json:"changed"`
	DirectCallers             []ImpactNode               `json:"direct_callers,omitempty"`
	AffectedCallers           []ImpactNode               `json:"affected_callers,omitempty"`
	ModuleDependants          []ImpactNode               `json:"module_dependants,omitempty"`
	OperationalEffects        []ImpactNode               `json:"operational_effects,omitempty"`
	RelatedTests              []RelatedTest              `json:"related_tests,omitempty"`
	Unresolved                []string                   `json:"unresolved,omitempty"`
	ValidationRecommendations []ValidationRecommendation `json:"validation_recommendations,omitempty"`
}

// ImpactAnalysis returns only explainable structural paths from the current
// generation. Stale or incomplete generations are discovery aids and cannot
// produce impact conclusions.
func (s *Store) ImpactAnalysis(ctx context.Context, identity RepositoryIdentity, symbolIDs []string, maxDepth int, limits GraphLimits) (QueryEnvelope[ImpactResult], error) {
	maxDepth, limits = normalizeImpactBounds(maxDepth, limits)
	generation, state, err := s.freshGeneration(ctx, identity)
	if err != nil {
		return QueryEnvelope[ImpactResult]{}, err
	}
	return s.impactAnalysisForGeneration(ctx, generation, state, symbolIDs, maxDepth, limits)
}

func normalizeImpactBounds(maxDepth int, limits GraphLimits) (int, GraphLimits) {
	if limits.MaxDepth <= 0 {
		limits = DefaultGraphLimits()
	}
	if maxDepth <= 0 {
		maxDepth = 2
	}
	if maxDepth > limits.MaxDepth {
		maxDepth = limits.MaxDepth
	}
	return maxDepth, limits
}

// impactAnalysisForGeneration is the shared traversal behind symbol and
// changed-file impact. The caller has already passed the freshness gate, so a
// changed-file batch resolves every file and traverses every symbol against one
// named generation rather than racing a refresh between those two operations.
func (s *Store) impactAnalysisForGeneration(ctx context.Context, generation GraphGeneration, state RepositoryState, symbolIDs []string, maxDepth int, limits GraphLimits) (QueryEnvelope[ImpactResult], error) {
	if state.Freshness != FreshnessCurrent {
		return QueryEnvelope[ImpactResult]{
			Query: QueryMeta{Type: "impact_analysis", Roots: symbolIDs}, Generation: state,
			Result:      ImpactResult{Unresolved: []string{"impact conclusions require a current complete graph"}},
			Resolution:  ResolutionMeta{Status: "unavailable", Methods: []ResolutionStatus{ResolutionUnresolved}},
			Limitations: []string{"graph is " + string(state.Freshness) + "; use normal repository inspection"},
		}, nil
	}
	result := ImpactResult{Unresolved: append([]string{}, generation.Limitations...)}
	result.Unresolved = append(result.Unresolved, "dynamic dependency injection and runtime registration may not be resolved")
	directCallers := map[string]ImpactNode{}
	affectedCallers := map[string]ImpactNode{}
	dependants := map[string]ImpactNode{}
	effects := map[string]ImpactNode{}
	tests := map[string]RelatedTest{}
	methods := []ResolutionStatus{}
	truncated := false
	collectEffects := func(origin GraphNode, affectedPath []ImpactPath) error {
		edges, edgeLimitReached, err := s.loadOutgoingImpactEdges(ctx, generation.ID, origin.ID, []string{"has_operational_effect"}, limits.MaxEdges)
		if err != nil {
			return err
		}
		if edgeLimitReached {
			truncated = true
			result.Unresolved = appendUniqueString(result.Unresolved, fmt.Sprintf("operational effects for %s exceeded the %d-edge adjacency bound", origin.ID, limits.MaxEdges))
		}
		for _, edge := range edges {
			node, err := s.loadGraphNode(ctx, generation.ID, edge.ToNodeID)
			if err != nil {
				return err
			}
			step := ImpactPath{FromNodeID: edge.FromNodeID, EdgeID: edge.ID, EdgeType: edge.Type, ToNodeID: edge.ToNodeID, Reason: origin.QualifiedName + " can cause " + node.DisplayName}
			keepShortestImpact(effects, ImpactNode{Node: node, Path: append(append([]ImpactPath{}, affectedPath...), step)})
			methods = append(methods, edge.Resolution)
		}
		return nil
	}
	collectBrowserTests := func(origin GraphNode, affectedPath []ImpactPath) error {
		var moduleID string
		if err := s.db.QueryRowContext(ctx, `SELECT from_node_id FROM graph_edges WHERE generation_id = ? AND to_node_id = ? AND edge_type = 'contains' LIMIT 1`, generation.ID, origin.ID).Scan(&moduleID); err == sql.ErrNoRows {
			return nil
		} else if err != nil {
			return err
		}
		edges, edgeLimitReached, err := s.loadOutgoingImpactEdges(ctx, generation.ID, moduleID, []string{"tested_by"}, limits.MaxEdges)
		if err != nil {
			return err
		}
		if edgeLimitReached {
			truncated = true
			result.Unresolved = appendUniqueString(result.Unresolved, fmt.Sprintf("browser/unit evidence for %s exceeded the %d-edge adjacency bound", origin.ID, limits.MaxEdges))
		}
		for _, edge := range edges {
			node, err := s.loadGraphNode(ctx, generation.ID, edge.ToNodeID)
			if err != nil {
				return err
			}
			step := ImpactPath{FromNodeID: edge.FromNodeID, EdgeID: edge.ID, EdgeType: edge.Type, ToNodeID: edge.ToNodeID, Reason: node.QualifiedName + " is registered evidence for affected consumer " + origin.QualifiedName}
			keepShortestTest(tests, RelatedTest{FilePath: node.QualifiedName, Reason: "registered browser or unit fixture for an affected consumer", Path: append(append([]ImpactPath{}, affectedPath...), step)})
			methods = append(methods, edge.Resolution)
		}
		return nil
	}
	for _, symbolID := range symbolIDs {
		symbol, nodeID, moduleNode, err := s.loadImpactRoot(ctx, generation.ID, symbolID)
		if err == sql.ErrNoRows {
			result.Unresolved = append(result.Unresolved, "symbol "+symbolID+" is not present in the current generation")
			continue
		}
		if err != nil {
			return QueryEnvelope[ImpactResult]{}, err
		}
		result.Changed = append(result.Changed, symbol)
		if err := collectEffects(GraphNode{ID: nodeID, QualifiedName: symbol.QualifiedName}, nil); err != nil {
			return QueryEnvelope[ImpactResult]{}, err
		}
		if err := collectBrowserTests(GraphNode{ID: nodeID, QualifiedName: symbol.QualifiedName}, nil); err != nil {
			return QueryEnvelope[ImpactResult]{}, err
		}
		callerBudgetExhausted := false
		callerFrontier := []ImpactNode{{Node: GraphNode{ID: nodeID, QualifiedName: symbol.QualifiedName}}}
		callerSeen := map[string]bool{nodeID: true}
		for depth := 0; depth < maxDepth && len(callerFrontier) > 0; depth++ {
			var next []ImpactNode
			for _, current := range callerFrontier {
				incoming, edgeLimitReached, err := s.loadIncomingImpactEdges(ctx, generation.ID, current.Node.ID, []string{"calls", "references", "implements_http_operation", "uses_http_operation", "http_contract_consumer", "uses_persisted_field", "exercises_surface"}, limits.MaxEdges)
				if err != nil {
					return QueryEnvelope[ImpactResult]{}, err
				}
				if edgeLimitReached {
					truncated = true
					result.Unresolved = appendUniqueString(result.Unresolved, fmt.Sprintf("symbol %s: incoming call/reference edges for %s exceeded the %d-edge adjacency bound", symbolID, current.Node.ID, limits.MaxEdges))
				}
				for _, edge := range incoming {
					if callerSeen[edge.FromNodeID] {
						continue
					}
					_, knownDirect := directCallers[edge.FromNodeID]
					_, knownAffected := affectedCallers[edge.FromNodeID]
					if !knownDirect && !knownAffected && len(directCallers)+len(affectedCallers) >= limits.MaxNodes {
						truncated = true
						callerBudgetExhausted = true
						continue
					}
					node, err := s.loadGraphNode(ctx, generation.ID, edge.FromNodeID)
					if err != nil {
						return QueryEnvelope[ImpactResult]{}, err
					}
					pathStep := ImpactPath{FromNodeID: edge.FromNodeID, EdgeID: edge.ID, EdgeType: edge.Type, ToNodeID: edge.ToNodeID, Reason: node.QualifiedName + " " + edge.Type + " affected symbol " + current.Node.QualifiedName}
					impact := ImpactNode{Node: node, Path: append([]ImpactPath{pathStep}, current.Path...)}
					callerSeen[node.ID] = true
					if depth == 0 {
						keepShortestImpact(directCallers, impact)
						delete(affectedCallers, node.ID)
					} else if _, direct := directCallers[node.ID]; !direct {
						keepShortestImpact(affectedCallers, impact)
					}
					next = append(next, impact)
					methods = append(methods, edge.Resolution)
					if err := collectEffects(node, impact.Path); err != nil {
						return QueryEnvelope[ImpactResult]{}, err
					}
					if err := collectBrowserTests(node, impact.Path); err != nil {
						return QueryEnvelope[ImpactResult]{}, err
					}
					if isTestFile(edge.SourceFilePath) {
						keepShortestTest(tests, RelatedTest{FilePath: edge.SourceFilePath, Reason: "test references a changed or affected symbol", Path: impact.Path})
					}
				}
			}
			callerFrontier = next
		}
		if callerBudgetExhausted {
			result.Unresolved = appendUniqueString(result.Unresolved, fmt.Sprintf("symbol %s: caller propagation incomplete; the %d-node batch bound was exhausted", symbolID, limits.MaxNodes))
		}
		if moduleNode.ID == "" {
			continue
		}
		moduleBudgetExhausted := false
		moduleFrontier := []ImpactNode{{Node: moduleNode}}
		moduleSeen := map[string]bool{moduleNode.ID: true}
		for depth := 0; depth < maxDepth && len(moduleFrontier) > 0; depth++ {
			var next []ImpactNode
			for _, current := range moduleFrontier {
				imports, edgeLimitReached, err := s.loadIncomingImpactEdges(ctx, generation.ID, current.Node.ID, []string{"imports", "generated_from"}, limits.MaxEdges)
				if err != nil {
					return QueryEnvelope[ImpactResult]{}, err
				}
				if edgeLimitReached {
					truncated = true
					result.Unresolved = appendUniqueString(result.Unresolved, fmt.Sprintf("symbol %s: incoming import edges for %s exceeded the %d-edge adjacency bound", symbolID, current.Node.ID, limits.MaxEdges))
				}
				for _, edge := range imports {
					if moduleSeen[edge.FromNodeID] {
						continue
					}
					if _, known := dependants[edge.FromNodeID]; !known && len(dependants) >= limits.MaxNodes {
						truncated = true
						moduleBudgetExhausted = true
						continue
					}
					node, err := s.loadGraphNode(ctx, generation.ID, edge.FromNodeID)
					if err != nil {
						return QueryEnvelope[ImpactResult]{}, err
					}
					pathStep := ImpactPath{FromNodeID: edge.FromNodeID, EdgeID: edge.ID, EdgeType: edge.Type, ToNodeID: edge.ToNodeID, Reason: node.QualifiedName + " imports affected module " + current.Node.QualifiedName}
					impact := ImpactNode{Node: node, Path: append([]ImpactPath{pathStep}, current.Path...)}
					moduleSeen[node.ID] = true
					keepShortestImpact(dependants, impact)
					next = append(next, impact)
					methods = append(methods, edge.Resolution)
					if isTestFile(node.QualifiedName) {
						keepShortestTest(tests, RelatedTest{FilePath: node.QualifiedName, Reason: "test module imports an affected module", Path: impact.Path})
					}
				}
			}
			moduleFrontier = next
		}
		if moduleBudgetExhausted {
			result.Unresolved = appendUniqueString(result.Unresolved, fmt.Sprintf("symbol %s: module-dependant propagation incomplete; the %d-node batch bound was exhausted", symbolID, limits.MaxNodes))
		}
		testEdges, edgeLimitReached, err := s.loadOutgoingImpactEdges(ctx, generation.ID, moduleNode.ID, []string{"tested_by"}, limits.MaxEdges)
		if err != nil {
			return QueryEnvelope[ImpactResult]{}, err
		}
		if edgeLimitReached {
			truncated = true
			result.Unresolved = appendUniqueString(result.Unresolved, fmt.Sprintf("symbol %s: structural test edges for %s exceeded the %d-edge adjacency bound", symbolID, moduleNode.ID, limits.MaxEdges))
		}
		for _, edge := range testEdges {
			node, err := s.loadGraphNode(ctx, generation.ID, edge.ToNodeID)
			if err != nil {
				return QueryEnvelope[ImpactResult]{}, err
			}
			path := ImpactPath{FromNodeID: edge.FromNodeID, EdgeID: edge.ID, EdgeType: edge.Type, ToNodeID: edge.ToNodeID, Reason: node.QualifiedName + " is structurally related to changed module " + moduleNode.QualifiedName}
			keepShortestTest(tests, RelatedTest{FilePath: node.QualifiedName, Reason: "structural test relationship; not behavioral proof", Path: []ImpactPath{path}})
			methods = append(methods, edge.Resolution)
		}
	}
	for _, item := range directCallers {
		result.DirectCallers = append(result.DirectCallers, item)
	}
	for _, item := range affectedCallers {
		result.AffectedCallers = append(result.AffectedCallers, item)
	}
	for _, item := range dependants {
		result.ModuleDependants = append(result.ModuleDependants, item)
	}
	for _, item := range effects {
		result.OperationalEffects = append(result.OperationalEffects, item)
	}
	for _, item := range tests {
		result.RelatedTests = append(result.RelatedTests, item)
	}
	sort.Slice(result.Changed, func(i, j int) bool { return result.Changed[i].QualifiedName < result.Changed[j].QualifiedName })
	sort.Slice(result.DirectCallers, func(i, j int) bool {
		return result.DirectCallers[i].Node.QualifiedName < result.DirectCallers[j].Node.QualifiedName
	})
	sort.Slice(result.AffectedCallers, func(i, j int) bool {
		return result.AffectedCallers[i].Node.QualifiedName < result.AffectedCallers[j].Node.QualifiedName
	})
	sort.Slice(result.ModuleDependants, func(i, j int) bool {
		return result.ModuleDependants[i].Node.QualifiedName < result.ModuleDependants[j].Node.QualifiedName
	})
	sort.Slice(result.OperationalEffects, func(i, j int) bool {
		return result.OperationalEffects[i].Node.QualifiedName < result.OperationalEffects[j].Node.QualifiedName
	})
	sort.Slice(result.RelatedTests, func(i, j int) bool { return result.RelatedTests[i].FilePath < result.RelatedTests[j].FilePath })
	if len(result.RelatedTests) > 0 {
		files := make([]string, 0, len(result.RelatedTests))
		var paths []ImpactPath
		for _, test := range result.RelatedTests {
			files = append(files, test.FilePath)
			paths = append(paths, test.Path...)
		}
		result.ValidationRecommendations = append(result.ValidationRecommendations, ValidationRecommendation{Criterion: "Structurally related tests for changed symbols pass", Scope: "related_tests", TestFiles: files, GraphPaths: paths, Limitations: []string{"test relationship selects candidates but does not prove behavioral coverage"}})
	}
	return QueryEnvelope[ImpactResult]{
		Query: QueryMeta{Type: "impact_analysis", Roots: symbolIDs}, Generation: state,
		Result: result, Resolution: ResolutionMeta{Status: "bounded", Methods: uniqueMethods(methods)},
		Limitations: generation.Limitations, Truncated: truncated,
	}, nil
}

func keepShortestImpact(target map[string]ImpactNode, candidate ImpactNode) {
	current, exists := target[candidate.Node.ID]
	if !exists || len(candidate.Path) < len(current.Path) {
		target[candidate.Node.ID] = candidate
	}
}

func keepShortestTest(target map[string]RelatedTest, candidate RelatedTest) {
	current, exists := target[candidate.FilePath]
	if !exists || len(candidate.Path) < len(current.Path) {
		target[candidate.FilePath] = candidate
	}
}

func appendUniqueString(values []string, candidate string) []string {
	for _, value := range values {
		if value == candidate {
			return values
		}
	}
	return append(values, candidate)
}

func (s *Store) loadImpactRoot(ctx context.Context, generationID, symbolID string) (GraphSymbol, string, GraphNode, error) {
	var symbol GraphSymbol
	var nodeID string
	err := s.db.QueryRowContext(ctx, `SELECT s.id, s.repository_id, s.language, s.kind, s.display_name, s.qualified_name, o.node_id FROM repository_symbols s JOIN symbol_occurrences o ON o.symbol_id = s.id WHERE s.id = ? AND o.generation_id = ?`, symbolID, generationID).Scan(&symbol.ID, &symbol.RepositoryID, &symbol.Language, &symbol.Kind, &symbol.DisplayName, &symbol.QualifiedName, &nodeID)
	if err != nil {
		return GraphSymbol{}, "", GraphNode{}, err
	}
	var moduleID string
	err = s.db.QueryRowContext(ctx, `SELECT from_node_id FROM graph_edges WHERE generation_id = ? AND to_node_id = ? AND edge_type = 'contains' LIMIT 1`, generationID, nodeID).Scan(&moduleID)
	if err == sql.ErrNoRows {
		return symbol, nodeID, GraphNode{}, nil
	}
	if err != nil {
		return GraphSymbol{}, "", GraphNode{}, err
	}
	module, err := s.loadGraphNode(ctx, generationID, moduleID)
	return symbol, nodeID, module, err
}

func (s *Store) loadIncomingImpactEdges(ctx context.Context, generationID, nodeID string, edgeTypes []string, limit int) ([]GraphEdge, bool, error) {
	return s.loadTypedImpactEdges(ctx, generationID, "to_node_id", nodeID, edgeTypes, limit)
}

func (s *Store) loadOutgoingImpactEdges(ctx context.Context, generationID, nodeID string, edgeTypes []string, limit int) ([]GraphEdge, bool, error) {
	return s.loadTypedImpactEdges(ctx, generationID, "from_node_id", nodeID, edgeTypes, limit)
}

func (s *Store) loadTypedImpactEdges(ctx context.Context, generationID, direction, nodeID string, edgeTypes []string, limit int) ([]GraphEdge, bool, error) {
	if limit <= 0 || len(edgeTypes) == 0 {
		return nil, false, nil
	}
	placeholders := "?"
	for range edgeTypes[1:] {
		placeholders += ",?"
	}
	query := fmt.Sprintf(`SELECT id, generation_id, from_node_id, to_node_id, edge_type, resolution_status, source_file_path, source_start_byte, source_end_byte, metadata FROM graph_edges WHERE generation_id = ? AND %s = ? AND edge_type IN (%s) ORDER BY id LIMIT ?`, direction, placeholders)
	args := []any{generationID, nodeID}
	for _, edgeType := range edgeTypes {
		args = append(args, edgeType)
	}
	args = append(args, limit+1)
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, false, err
	}
	defer rows.Close()
	var result []GraphEdge
	for rows.Next() {
		var edge GraphEdge
		var metadata string
		if err := rows.Scan(&edge.ID, &edge.GenerationID, &edge.FromNodeID, &edge.ToNodeID, &edge.Type, &edge.Resolution, &edge.SourceFilePath, &edge.SourceStartByte, &edge.SourceEndByte, &metadata); err != nil {
			return nil, false, err
		}
		result = append(result, edge)
	}
	if err := rows.Err(); err != nil {
		return nil, false, err
	}
	truncated := len(result) > limit
	if truncated {
		result = result[:limit]
	}
	return result, truncated, nil
}
