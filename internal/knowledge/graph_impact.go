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
	RelatedTests              []RelatedTest              `json:"related_tests,omitempty"`
	Unresolved                []string                   `json:"unresolved,omitempty"`
	ValidationRecommendations []ValidationRecommendation `json:"validation_recommendations,omitempty"`
}

// ImpactAnalysis returns only explainable structural paths from the current
// generation. Stale or incomplete generations are discovery aids and cannot
// produce impact conclusions.
func (s *Store) ImpactAnalysis(ctx context.Context, identity RepositoryIdentity, symbolIDs []string, maxDepth int, limits GraphLimits) (QueryEnvelope[ImpactResult], error) {
	if limits.MaxDepth <= 0 {
		limits = DefaultGraphLimits()
	}
	if maxDepth <= 0 {
		maxDepth = 2
	}
	if maxDepth > limits.MaxDepth {
		maxDepth = limits.MaxDepth
	}
	generation, state, err := s.freshGeneration(ctx, identity)
	if err != nil {
		return QueryEnvelope[ImpactResult]{}, err
	}
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
	tests := map[string]RelatedTest{}
	methods := []ResolutionStatus{}
	truncated := false
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
		callerFrontier := []ImpactNode{{Node: GraphNode{ID: nodeID, QualifiedName: symbol.QualifiedName}}}
		callerSeen := map[string]bool{nodeID: true}
		for depth := 0; depth < maxDepth && len(callerFrontier) > 0; depth++ {
			var next []ImpactNode
			for _, current := range callerFrontier {
				incoming, err := s.loadIncomingImpactEdges(ctx, generation.ID, current.Node.ID, []string{"calls", "references"}, limits.MaxEdges)
				if err != nil {
					return QueryEnvelope[ImpactResult]{}, err
				}
				for _, edge := range incoming {
					if len(directCallers)+len(affectedCallers) >= limits.MaxNodes {
						truncated = true
						break
					}
					if callerSeen[edge.FromNodeID] {
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
						directCallers[node.ID] = impact
					} else {
						affectedCallers[node.ID] = impact
					}
					next = append(next, impact)
					methods = append(methods, edge.Resolution)
					if isTestFile(edge.SourceFilePath) {
						tests[edge.SourceFilePath] = RelatedTest{FilePath: edge.SourceFilePath, Reason: "test references a changed or affected symbol", Path: impact.Path}
					}
				}
			}
			callerFrontier = next
		}
		if moduleNode.ID == "" {
			continue
		}
		moduleFrontier := []ImpactNode{{Node: moduleNode}}
		moduleSeen := map[string]bool{moduleNode.ID: true}
		for depth := 0; depth < maxDepth && len(moduleFrontier) > 0; depth++ {
			var next []ImpactNode
			for _, current := range moduleFrontier {
				imports, err := s.loadIncomingImpactEdges(ctx, generation.ID, current.Node.ID, []string{"imports"}, limits.MaxEdges)
				if err != nil {
					return QueryEnvelope[ImpactResult]{}, err
				}
				for _, edge := range imports {
					if len(dependants) >= limits.MaxNodes {
						truncated = true
						break
					}
					if moduleSeen[edge.FromNodeID] {
						continue
					}
					node, err := s.loadGraphNode(ctx, generation.ID, edge.FromNodeID)
					if err != nil {
						return QueryEnvelope[ImpactResult]{}, err
					}
					pathStep := ImpactPath{FromNodeID: edge.FromNodeID, EdgeID: edge.ID, EdgeType: edge.Type, ToNodeID: edge.ToNodeID, Reason: node.QualifiedName + " imports affected module " + current.Node.QualifiedName}
					impact := ImpactNode{Node: node, Path: append([]ImpactPath{pathStep}, current.Path...)}
					moduleSeen[node.ID] = true
					dependants[node.ID] = impact
					next = append(next, impact)
					methods = append(methods, edge.Resolution)
					if isTestFile(node.QualifiedName) {
						tests[node.QualifiedName] = RelatedTest{FilePath: node.QualifiedName, Reason: "test module imports an affected module", Path: impact.Path}
					}
				}
			}
			moduleFrontier = next
		}
		testEdges, err := s.loadOutgoingImpactEdges(ctx, generation.ID, moduleNode.ID, []string{"tested_by"}, limits.MaxEdges)
		if err != nil {
			return QueryEnvelope[ImpactResult]{}, err
		}
		for _, edge := range testEdges {
			node, err := s.loadGraphNode(ctx, generation.ID, edge.ToNodeID)
			if err != nil {
				return QueryEnvelope[ImpactResult]{}, err
			}
			path := ImpactPath{FromNodeID: edge.FromNodeID, EdgeID: edge.ID, EdgeType: edge.Type, ToNodeID: edge.ToNodeID, Reason: node.QualifiedName + " is structurally related to changed module " + moduleNode.QualifiedName}
			tests[node.QualifiedName] = RelatedTest{FilePath: node.QualifiedName, Reason: "structural test relationship; not behavioral proof", Path: []ImpactPath{path}}
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

func (s *Store) loadIncomingImpactEdges(ctx context.Context, generationID, nodeID string, edgeTypes []string, limit int) ([]GraphEdge, error) {
	return s.loadTypedImpactEdges(ctx, generationID, "to_node_id", nodeID, edgeTypes, limit)
}

func (s *Store) loadOutgoingImpactEdges(ctx context.Context, generationID, nodeID string, edgeTypes []string, limit int) ([]GraphEdge, error) {
	return s.loadTypedImpactEdges(ctx, generationID, "from_node_id", nodeID, edgeTypes, limit)
}

func (s *Store) loadTypedImpactEdges(ctx context.Context, generationID, direction, nodeID string, edgeTypes []string, limit int) ([]GraphEdge, error) {
	if limit <= 0 || len(edgeTypes) == 0 {
		return nil, nil
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
	args = append(args, limit)
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []GraphEdge
	for rows.Next() {
		var edge GraphEdge
		var metadata string
		if err := rows.Scan(&edge.ID, &edge.GenerationID, &edge.FromNodeID, &edge.ToNodeID, &edge.Type, &edge.Resolution, &edge.SourceFilePath, &edge.SourceStartByte, &edge.SourceEndByte, &metadata); err != nil {
			return nil, err
		}
		result = append(result, edge)
	}
	return result, rows.Err()
}
