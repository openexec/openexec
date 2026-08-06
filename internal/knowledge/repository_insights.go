package knowledge

import (
	"context"
	"fmt"
	"sort"
	"strings"
)

// Repository insights: the graph's conclusions, not its inventory. The first
// panel iteration listed 50 arbitrary symbols and answered no question; these
// three answers replace it. Every insight is bounded and static-analysis
// honest — heuristics are disclosed as limitations, never presented as fact.

const (
	deadCodeCandidateLimit = 15
	hotspotLimit           = 10
	moduleSketchLimit      = 15
)

// InsightSymbol is one symbol-shaped conclusion with its safe location.
type InsightSymbol struct {
	DisplayName  string `json:"display_name"`
	Kind         string `json:"kind"`
	SafeLocation string `json:"safe_location"`
	Inbound      int    `json:"inbound,omitempty"`
}

// ModuleFlow is one aggregated top-level dependency direction.
type ModuleFlow struct {
	From   string `json:"from"`
	To     string `json:"to"`
	Weight int    `json:"weight"`
}

// deadCodeCandidates lists exported symbols with no inbound call or reference
// edges in the current generation. Static reachability only: entry points,
// tests, and reflective use are not excluded — the caller must disclose that.
func (s *Store) deadCodeCandidates(ctx context.Context, generationID string) ([]InsightSymbol, int, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT COALESCE(NULLIF(rs.qualified_name, ''), rs.display_name), rs.kind, o.file_path, o.start_line
		FROM symbol_occurrences o
		JOIN graph_nodes n ON n.id = o.node_id
		JOIN repository_symbols rs ON rs.id = o.symbol_id
		WHERE o.generation_id = ? AND o.exported = 1
		  AND rs.display_name NOT IN ('main', 'init', 'TestMain')
		  AND o.file_path NOT LIKE '%_test.go' AND o.file_path NOT LIKE '%.test.ts%' AND o.file_path NOT LIKE '%.spec.ts%'
		  AND NOT EXISTS (
			SELECT 1 FROM graph_edges e
			WHERE e.generation_id = o.generation_id AND e.to_node_id = n.id
			  AND e.edge_type IN ('calls', 'references'))
		ORDER BY o.file_path, o.start_line LIMIT ?`, generationID, deadCodeCandidateLimit+1)
	if err != nil {
		return nil, 0, fmt.Errorf("dead-code candidates: %w", err)
	}
	defer rows.Close()
	var out []InsightSymbol
	for rows.Next() {
		var name, kind, file string
		var line int
		if err := rows.Scan(&name, &kind, &file, &line); err != nil {
			return nil, 0, err
		}
		out = append(out, InsightSymbol{DisplayName: name, Kind: kind, SafeLocation: fmt.Sprintf("%s:%d", file, line)})
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	var total int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM symbol_occurrences o JOIN graph_nodes n ON n.id = o.node_id JOIN repository_symbols rs ON rs.id = o.symbol_id WHERE o.generation_id = ? AND o.exported = 1 AND rs.display_name NOT IN ('main','init','TestMain') AND o.file_path NOT LIKE '%_test.go' AND o.file_path NOT LIKE '%.test.ts%' AND o.file_path NOT LIKE '%.spec.ts%' AND NOT EXISTS (SELECT 1 FROM graph_edges e WHERE e.generation_id = o.generation_id AND e.to_node_id = n.id AND e.edge_type IN ('calls','references'))`, generationID).Scan(&total); err != nil {
		return nil, 0, err
	}
	if len(out) > deadCodeCandidateLimit {
		out = out[:deadCodeCandidateLimit]
	}
	return out, total, nil
}

// hotspots ranks symbols by inbound call/reference count — the "touch with
// care" map an agent should read before scoping any change.
//
// Two exclusions keep the ranking meaningful rather than merely large. Bare
// lexical mentions are counted for dead-code review, where over-linking is the
// safe error, but they cannot rank importance: every occurrence of the word
// "count" or "title" would link to a symbol of that name, and the panel's
// headline conclusion became "post is the highest-ranked hotspot". And a name
// under three characters is not a conclusion a reader can use — a codebase
// whose most-depended-on symbol is `t` has been told nothing, however true the
// number is.
func (s *Store) hotspots(ctx context.Context, generationID string) ([]InsightSymbol, int, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT COALESCE(NULLIF(rs.qualified_name, ''), rs.display_name), rs.kind, o.file_path, o.start_line, COUNT(e.id) AS inbound
		FROM graph_edges e
		JOIN graph_nodes n ON n.id = e.to_node_id AND n.node_type = 'symbol'
		JOIN symbol_occurrences o ON o.node_id = n.id AND o.generation_id = e.generation_id
		JOIN repository_symbols rs ON rs.id = o.symbol_id
		WHERE e.generation_id = ? AND e.edge_type IN ('calls', 'references')
		  AND e.resolution_status NOT IN ('ambiguous', 'static_lexical')
		  AND LENGTH(rs.display_name) >= 3
		GROUP BY n.id ORDER BY inbound DESC, rs.display_name`, generationID)
	if err != nil {
		return nil, 0, fmt.Errorf("hotspots: %w", err)
	}
	defer rows.Close()
	var out []InsightSymbol
	for rows.Next() {
		var entry InsightSymbol
		var file string
		var line int
		if err := rows.Scan(&entry.DisplayName, &entry.Kind, &file, &line, &entry.Inbound); err != nil {
			return nil, 0, err
		}
		entry.SafeLocation = fmt.Sprintf("%s:%d", file, line)
		out = append(out, entry)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	total := len(out)
	if len(out) > hotspotLimit {
		out = out[:hotspotLimit]
	}
	return out, total, nil
}

// moduleSketch aggregates module import edges up to top-level areas, so the
// panel can draw a readable dependency sketch instead of quoting a count.
func (s *Store) moduleSketch(ctx context.Context, generationID string) ([]ModuleFlow, int, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT nf.qualified_name, nt.qualified_name
		FROM graph_edges e
		JOIN graph_nodes nf ON nf.id = e.from_node_id AND nf.node_type = 'module'
		JOIN graph_nodes nt ON nt.id = e.to_node_id AND nt.node_type = 'module'
		WHERE e.generation_id = ? AND e.edge_type = 'imports'`, generationID)
	if err != nil {
		return nil, 0, fmt.Errorf("module sketch: %w", err)
	}
	defer rows.Close()
	pairs := [][2]string{}
	for rows.Next() {
		var from, to string
		if err := rows.Scan(&from, &to); err != nil {
			return nil, 0, err
		}
		pairs = append(pairs, [2]string{from, to})
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	// How deep an "area" is depends on how the repository is laid out. Two
	// segments describes src/components and src/lib, and collapses
	// code/backend/app/services and code/backend/app/repositories into one
	// node whose every edge points at itself — which is why a repository with
	// 732 module imports produced no architecture at all. Go deeper until the
	// layout actually separates, and no deeper than it needs to.
	weights := map[[2]string]int{}
	for depth := 2; depth <= 4; depth++ {
		weights = map[[2]string]int{}
		for _, pair := range pairs {
			key := [2]string{moduleArea(pair[0], depth), moduleArea(pair[1], depth)}
			if key[0] == key[1] {
				continue
			}
			weights[key]++
		}
		if len(weights) > 0 {
			break
		}
	}
	out := make([]ModuleFlow, 0, len(weights))
	for key, weight := range weights {
		out = append(out, ModuleFlow{From: key[0], To: key[1], Weight: weight})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Weight != out[j].Weight {
			return out[i].Weight > out[j].Weight
		}
		return out[i].From+out[i].To < out[j].From+out[j].To
	})
	total := len(out)
	if total > moduleSketchLimit {
		out = out[:moduleSketchLimit]
	}
	return out, total, nil
}

// attachRepositoryInsights fills totals and the three conclusions, disclosing
// heuristic boundaries as limitations rather than presenting them as fact.
func (s *Store) attachRepositoryInsights(ctx context.Context, generationID string, projection *RepositoryContextProjection) error {
	totals := &GraphTotals{}
	if err := s.db.QueryRowContext(ctx, `SELECT
		(SELECT COUNT(*) FROM symbol_occurrences WHERE generation_id = ?),
		(SELECT COUNT(*) FROM graph_nodes WHERE generation_id = ? AND node_type = 'module'),
		(SELECT COUNT(*) FROM graph_edges WHERE generation_id = ?),
		(SELECT COUNT(*) FROM graph_edges WHERE generation_id = ? AND edge_type = 'imports')`,
		generationID, generationID, generationID, generationID).Scan(
		&totals.Symbols, &totals.Modules, &totals.Edges, &totals.ImportEdges); err != nil {
		return fmt.Errorf("graph totals: %w", err)
	}
	projection.Totals = totals
	dead, deadTotal, err := s.deadCodeCandidates(ctx, generationID)
	if err != nil {
		return err
	}
	projection.DeadCodeCandidates = dead
	if len(dead) > 0 {
		note := "dead-code candidates are static-reference heuristics; entry points, generated code, and reflective use are not excluded"
		if deadTotal > len(dead) {
			note += " (list truncated)"
		}
		projection.Limitations = append(projection.Limitations, note)
	}
	projection.Selections["dead_code_candidates"] = selectionScope("exported symbols without static inbound call or reference edges", deadCodeCandidateLimit, len(dead), deadTotal)
	var hotspotTotal int
	if projection.Hotspots, hotspotTotal, err = s.hotspots(ctx, generationID); err != nil {
		return err
	}
	projection.Selections["hotspots"] = selectionScope("symbols ranked by static inbound calls and references", hotspotLimit, len(projection.Hotspots), hotspotTotal)
	var sketchTotal int
	if projection.ModuleSketch, sketchTotal, err = s.moduleSketch(ctx, generationID); err != nil {
		return err
	}
	projection.Selections["module_sketch"] = selectionScope("aggregated top-level module import directions", moduleSketchLimit, len(projection.ModuleSketch), sketchTotal)
	return nil
}

// moduleArea groups a module path into the area a reader would name, at the
// requested depth. A path shallower than the depth is its own area.
func moduleArea(path string, depth int) string {
	parts := strings.Split(path, "/")
	if len(parts) > depth {
		parts = parts[:depth]
	} else if len(parts) > 1 {
		parts = parts[:len(parts)-1]
	}
	return strings.Join(parts, "/")
}
