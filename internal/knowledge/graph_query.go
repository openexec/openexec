package knowledge

import (
	"context"
	"database/sql"
	"encoding/json"
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
	generation, err := s.activeGeneration(ctx, identity.WorktreeID)
	if err == sql.ErrNoRows {
		return RepositoryState{RepositoryID: identity.RepositoryID, CheckoutID: identity.CheckoutID, WorktreeID: identity.WorktreeID, BaseCommit: identity.BaseCommit, Freshness: FreshnessMissing}, nil
	}
	if err != nil {
		return RepositoryState{}, err
	}
	state := generationState(generation)
	if generation.ManifestHash == "legacy-unversioned" {
		return state, nil
	}
	manifest, err := BuildScanManifest(identity.RootPath)
	if err != nil {
		state.Freshness = FreshnessInconsistent
		return state, nil
	}
	if manifest.ManifestHash != generation.ManifestHash || manifest.ConfigurationDigest != generation.ConfigurationDigest {
		state.Freshness = FreshnessStale
		if generation.Status == GraphCurrent {
			_, _ = s.db.ExecContext(ctx, `UPDATE graph_generations SET status = 'stale' WHERE id = ? AND status = 'current'`, generation.ID)
		}
	}
	return state, nil
}

func (s *Store) FindModuleDependencies(ctx context.Context, identity RepositoryIdentity, module string, reverse bool, depth int, limits GraphLimits) (QueryEnvelope[ModuleDependencyResult], error) {
	if limits.MaxDepth <= 0 {
		limits = DefaultGraphLimits()
	}
	if depth <= 0 {
		depth = 1
	}
	if depth > limits.MaxDepth {
		depth = limits.MaxDepth
	}
	generation, err := s.activeGeneration(ctx, identity.WorktreeID)
	if err != nil {
		return QueryEnvelope[ModuleDependencyResult]{}, err
	}
	root, err := s.findModuleNode(ctx, generation.ID, module)
	if err != nil {
		return QueryEnvelope[ModuleDependencyResult]{}, err
	}
	seen := map[string]bool{root.ID: true}
	frontier := []string{root.ID}
	nodes := []GraphNode{}
	edges := []GraphEdge{}
	truncated := false
	for level := 0; level < depth && len(frontier) > 0; level++ {
		var next []string
		for _, nodeID := range frontier {
			found, err := s.loadImportEdges(ctx, generation.ID, nodeID, reverse, limits.MaxEdges-len(edges))
			if err != nil {
				return QueryEnvelope[ModuleDependencyResult]{}, err
			}
			for _, edge := range found {
				if len(edges) >= limits.MaxEdges {
					truncated = true
					break
				}
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
	state, _ := s.CurrentRepositoryState(ctx, identity)
	return QueryEnvelope[ModuleDependencyResult]{
		Query: QueryMeta{Type: queryType, Roots: []string{root.ID}}, Generation: state,
		Result:      ModuleDependencyResult{Root: root, Nodes: nodes, Edges: edges},
		Resolution:  ResolutionMeta{Status: "bounded", Methods: edgeMethods(edges)},
		Limitations: generation.Limitations, Truncated: truncated,
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

func (s *Store) loadImportEdges(ctx context.Context, generationID, nodeID string, reverse bool, limit int) ([]GraphEdge, error) {
	if limit <= 0 {
		return nil, nil
	}
	direction := "from_node_id"
	if reverse {
		direction = "to_node_id"
	}
	query := fmt.Sprintf(`SELECT id, generation_id, from_node_id, to_node_id, edge_type, resolution_status, source_file_path, source_start_byte, source_end_byte, metadata FROM graph_edges WHERE generation_id = ? AND edge_type = 'imports' AND %s = ? ORDER BY id LIMIT ?`, direction)
	rows, err := s.db.QueryContext(ctx, query, generationID, nodeID, limit)
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
		edge.Metadata = map[string]string{}
		_ = jsonUnmarshalStringMap(metadata, &edge.Metadata)
		result = append(result, edge)
	}
	return result, rows.Err()
}

func edgeMethods(edges []GraphEdge) []ResolutionStatus {
	methods := make([]ResolutionStatus, 0, len(edges))
	for _, edge := range edges {
		methods = append(methods, edge.Resolution)
	}
	return uniqueMethods(methods)
}
