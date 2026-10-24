package knowledge

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
)

type ModuleDependencyResult struct {
	Root  GraphNode   `json:"root"`
	Nodes []GraphNode `json:"nodes"`
	Edges []GraphEdge `json:"edges"`
}

// CurrentRepositoryState compares the current generation with the current
// worktree manifest. A mismatch marks the generation stale before returning.
func (s *Store) CurrentRepositoryState(ctx context.Context, identity RepositoryIdentity) (RepositoryState, error) {
	_, state, err := s.freshGeneration(ctx, identity)
	if errors.Is(err, sql.ErrNoRows) {
		return RepositoryState{RepositoryID: identity.RepositoryID, CheckoutID: identity.CheckoutID, WorktreeID: identity.WorktreeID, BaseCommit: identity.BaseCommit, Freshness: FreshnessMissing}, nil
	}
	if err != nil {
		return RepositoryState{}, err
	}
	return state, nil
}

func (s *Store) FindModuleDependencies(ctx context.Context, identity RepositoryIdentity, module string, reverse bool, depth int, limits GraphLimits) (QueryEnvelope[ModuleDependencyResult], error) {
	generation, state, err := s.freshGeneration(ctx, identity)
	if err != nil {
		return QueryEnvelope[ModuleDependencyResult]{}, err
	}
	return s.moduleDependenciesIn(ctx, generation, state, module, reverse, depth, limits)
}

// moduleDependenciesIn walks import edges within one already-gated generation.
// A clamped depth and any import edge the edge budget left unread are disclosed
// through Limitations/Truncated, so a bounded walk is never read as the full
// reach; a graph that fits the edge budget exactly stays complete.
func (s *Store) moduleDependenciesIn(ctx context.Context, generation GraphGeneration, state RepositoryState, module string, reverse bool, depth int, limits GraphLimits) (QueryEnvelope[ModuleDependencyResult], error) {
	if limits.MaxDepth <= 0 {
		limits = DefaultGraphLimits()
	}
	if depth <= 0 {
		depth = 1
	}
	var depthLimitation string
	if depth > limits.MaxDepth {
		depthLimitation = depthLimitationNote(depth, limits.MaxDepth)
		depth = limits.MaxDepth
	}
	limitations := append([]string(nil), generation.Limitations...)
	if depthLimitation != "" {
		limitations = append(limitations, depthLimitation)
	}
	root, err := s.findModuleNode(ctx, generation.ID, module)
	if err != nil {
		return QueryEnvelope[ModuleDependencyResult]{}, err
	}
	// Each node enters the frontier once (seen), so its edges are read at most
	// once: an edge reported as omitted is never one already returned through
	// a cycle.
	seen := map[string]bool{root.ID: true}
	frontier := []string{root.ID}
	nodes := []GraphNode{}
	edges := []GraphEdge{}
	truncated := false
	for level := 0; level < depth && len(frontier) > 0; level++ {
		var next []string
		for _, nodeID := range frontier {
			remaining := limits.MaxEdges - len(edges)
			if remaining <= 0 {
				if truncated {
					break
				}
				// The budget is spent exactly; only an unread edge on a node
				// still to be expanded makes the result incomplete.
				unread, _, err := s.loadImportEdges(ctx, generation.ID, nodeID, reverse, 1)
				if err != nil {
					return QueryEnvelope[ModuleDependencyResult]{}, err
				}
				if len(unread) > 0 {
					truncated = true
					break
				}
				continue
			}
			found, edgeLimitReached, err := s.loadImportEdges(ctx, generation.ID, nodeID, reverse, remaining)
			if err != nil {
				return QueryEnvelope[ModuleDependencyResult]{}, err
			}
			if edgeLimitReached {
				truncated = true
			}
			for _, edge := range found {
				edges = append(edges, edge)
				target := edge.ToNodeID
				if reverse {
					target = edge.FromNodeID
				}
				if !seen[target] {
					if len(nodes) >= limits.MaxNodes {
						truncated = true
						continue
					}
					node, err := s.loadGraphNode(ctx, generation.ID, target)
					if err != nil {
						return QueryEnvelope[ModuleDependencyResult]{}, err
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
	queryType := "find_module_dependencies"
	if reverse {
		queryType = "find_module_dependants"
	}
	return QueryEnvelope[ModuleDependencyResult]{
		Query: QueryMeta{Type: queryType, Roots: []string{root.ID}}, Generation: state,
		Result:      ModuleDependencyResult{Root: root, Nodes: nodes, Edges: edges},
		Resolution:  ResolutionMeta{Status: "bounded", Methods: edgeMethods(edges)},
		Limitations: limitations, Truncated: truncated || depthLimitation != "",
	}, nil
}

func (s *Store) findModuleNode(ctx context.Context, generationID, module string) (GraphNode, error) {
	row := s.db.QueryRowContext(ctx, `SELECT id, generation_id, repository_id, node_type, language, display_name, qualified_name, metadata FROM graph_nodes WHERE generation_id = ? AND node_type = 'module' AND (id = ? OR qualified_name = ?)`, generationID, module, module)
	return scanGraphNode(row)
}

func (s *Store) loadGraphNode(ctx context.Context, generationID, id string) (GraphNode, error) {
	return scanGraphNode(s.db.QueryRowContext(ctx, `SELECT id, generation_id, repository_id, node_type, language, display_name, qualified_name, metadata FROM graph_nodes WHERE generation_id = ? AND id = ?`, generationID, id))
}

func scanGraphNode(row rowScanner) (GraphNode, error) {
	var node GraphNode
	var metadata string
	if err := row.Scan(&node.ID, &node.GenerationID, &node.RepositoryID, &node.NodeType, &node.Language, &node.DisplayName, &node.QualifiedName, &metadata); err != nil {
		return GraphNode{}, err
	}
	node.Metadata = map[string]string{}
	_ = jsonUnmarshalStringMap(metadata, &node.Metadata)
	return node, nil
}

func jsonUnmarshalStringMap(data string, target *map[string]string) error {
	if data == "" {
		return nil
	}
	return json.Unmarshal([]byte(data), target)
}

// loadImportEdges returns up to limit import edges of one node in ID order and
// reports whether more exist. It reads one row past the limit (as
// loadTypedImpactEdges does) so an omitted edge is observed rather than
// inferred from a spent budget. Unlike that loader it keeps edge metadata (the
// import specifier), which the public module-dependency result carries.
func (s *Store) loadImportEdges(ctx context.Context, generationID, nodeID string, reverse bool, limit int) ([]GraphEdge, bool, error) {
	if limit <= 0 {
		return nil, false, nil
	}
	direction := "from_node_id"
	if reverse {
		direction = "to_node_id"
	}
	query := fmt.Sprintf(`SELECT id, generation_id, from_node_id, to_node_id, edge_type, resolution_status, source_file_path, source_start_byte, source_end_byte, metadata FROM graph_edges WHERE generation_id = ? AND edge_type = 'imports' AND %s = ? ORDER BY id LIMIT ?`, direction)
	rows, err := s.db.QueryContext(ctx, query, generationID, nodeID, limit+1)
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
		edge.Metadata = map[string]string{}
		_ = jsonUnmarshalStringMap(metadata, &edge.Metadata)
		result = append(result, edge)
	}
	if err := rows.Err(); err != nil {
		return nil, false, err
	}
	if len(result) > limit {
		return result[:limit], true, nil
	}
	return result, false, nil
}

func edgeMethods(edges []GraphEdge) []ResolutionStatus {
	methods := make([]ResolutionStatus, 0, len(edges))
	for _, edge := range edges {
		methods = append(methods, edge.Resolution)
	}
	return uniqueMethods(methods)
}
