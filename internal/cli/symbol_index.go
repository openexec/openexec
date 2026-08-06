package cli

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/openexec/openexec/internal/knowledge"
	"github.com/openexec/openexec/internal/mcp"
	"github.com/openexec/openexec/internal/repository"
)

// graphSymbolIndex adapts internal/knowledge to the mcp.SymbolIndex seam.
//
// It lives in the composition root so internal/mcp never imports the graph
// implementation — the same arrangement as the memory_read loader. The
// adapter only translates shapes: every freshness decision, resolution tier
// and bound stays the knowledge layer's to make.
type graphSymbolIndex struct {
	store *knowledge.Store
}

// errNoGraph tells the caller how to create the index rather than reporting a
// bare "no rows" from the generation lookup.
var errNoGraph = errors.New("this workspace has no repository graph yet — run `openexec knowledge graph scan` to build one")

func newGraphSymbolIndex(store *knowledge.Store) *graphSymbolIndex {
	return &graphSymbolIndex{store: store}
}

// symbolIndexRoot picks the workspace root to index from the MCP server's
// already-resolved roots.
//
// It must be the server's roots rather than the process working directory: the
// server prefers WORKSPACE_ROOT, and a client that spawns `openexec mcp-serve`
// from an unrelated cwd — Agent Console does — would otherwise be told there
// is no index while the server scopes itself to the real workspace.
//
// The test is for a servable graph generation, not for the .openexec directory
// and not merely for the database file. `openexec init` creates the directory,
// and other features create the database, so either can exist with no graph in
// it — advertising tools that can only answer "no graph" is worse than not
// advertising them. Opening a store to find out would itself create and
// migrate a database and insert identity rows, so the probe below is a
// read-only connection that writes nothing.
func symbolIndexRoot(roots []string) string {
	if len(roots) == 0 || roots[0] == "" {
		return ""
	}
	if !hasServableGraph(roots[0]) {
		return ""
	}
	return roots[0]
}

// hasServableGraph reports whether root has a graph generation worth serving.
//
// It opens the database read-only (mode=ro fails rather than creating, and
// forbids writes), so probing an uninitialized or graph-less workspace leaves
// it byte-identical. Any error — missing file, missing table, unreadable —
// means "nothing to serve", never "assume yes".
func hasServableGraph(root string) bool {
	path := filepath.Join(root, ".openexec", "openexec.db")
	if _, err := os.Stat(path); err != nil {
		return false
	}
	db, err := sql.Open("sqlite", "file:"+path+"?mode=ro")
	if err != nil {
		return false
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var count int
	err = db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM graph_generations g
		JOIN worktrees w ON w.id = g.worktree_id
		WHERE w.root_path = ? AND g.status IN ('current', 'partial')`, root).Scan(&count)
	if err != nil {
		return false
	}
	return count > 0
}

// identity resolves the repository/checkout/worktree identity for a root.
func (g *graphSymbolIndex) identity(ctx context.Context, root string) (knowledge.RepositoryIdentity, error) {
	identity, err := g.store.EnsureRepositoryIdentity(ctx, root, "")
	if err != nil {
		return knowledge.RepositoryIdentity{}, fmt.Errorf("resolve repository identity: %w", err)
	}
	return identity, nil
}

// translateGeneration maps errors from a lookup whose only missing row can be
// the generation itself. A stale-graph refusal passes through intact: it is
// the contract, not a failure to hide.
func translateGeneration(err error) error {
	if errors.Is(err, sql.ErrNoRows) {
		return errNoGraph
	}
	return err
}

// translateSymbol maps errors from a lookup keyed by a symbol_id the caller
// already holds. Reporting "no graph" there would be wrong and would send the
// agent to rebuild an index that exists: the far likelier cause is a symbol_id
// from an older generation.
func translateSymbol(err error, symbolID string) error {
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("symbol %q is not in the current graph generation — it may have been renamed, moved or removed; call symbol_find again for a current id", symbolID)
	}
	return err
}

func pointerFrom(candidate knowledge.SymbolCandidate) mcp.SymbolPointer {
	return mcp.SymbolPointer{
		SymbolID:      candidate.Symbol.ID,
		Name:          candidate.Symbol.DisplayName,
		QualifiedName: candidate.Symbol.QualifiedName,
		Kind:          candidate.Symbol.Kind,
		Language:      candidate.Symbol.Language,
		File:          candidate.Occurrence.FilePath,
		StartLine:     candidate.Occurrence.StartLine,
		EndLine:       candidate.Occurrence.EndLine,
		Signature:     candidate.Occurrence.Signature,
		Resolution:    string(candidate.Occurrence.Resolution),
		Exported:      candidate.Occurrence.Exported,
	}
}

func (g *graphSymbolIndex) FindSymbols(ctx context.Context, root, query, file, kind string, page, pageSize int) (mcp.SymbolPointerPage, error) {
	identity, err := g.identity(ctx, root)
	if err != nil {
		return mcp.SymbolPointerPage{}, err
	}
	envelope, err := g.store.FindGraphSymbols(ctx, identity, query, file, kind, page, pageSize)
	if err != nil {
		return mcp.SymbolPointerPage{}, translateGeneration(err)
	}

	out := mcp.SymbolPointerPage{
		GraphVersion: envelope.Generation.GraphVersion,
		Freshness:    string(envelope.Generation.Freshness),
		Truncated:    envelope.Truncated,
		Limitations:  envelope.Limitations,
	}
	for _, candidate := range envelope.Result.Candidates {
		out.Pointers = append(out.Pointers, pointerFrom(candidate))
	}
	if envelope.Pagination != nil {
		out.Page = envelope.Pagination.Page
		out.PageSize = envelope.Pagination.PageSize
		out.Total = envelope.Pagination.Total
		out.TotalPages = envelope.Pagination.TotalPages
	} else {
		out.Total = len(out.Pointers)
	}
	return out, nil
}

func (g *graphSymbolIndex) ReadSymbol(ctx context.Context, root, symbolID string) (mcp.SymbolSourceRead, error) {
	identity, err := g.identity(ctx, root)
	if err != nil {
		return mcp.SymbolSourceRead{}, err
	}
	// The rooted reader is the repository source authority: it confines reads
	// to the authorized worktree and bounds their size.
	reader, err := repository.NewRootedReader(root, identity.RepositoryID, identity.WorktreeID,
		knowledge.DefaultGraphLimits().MaxBytes)
	if err != nil {
		return mcp.SymbolSourceRead{}, err
	}
	envelope, err := g.store.ReadGraphSymbol(ctx, identity, symbolID, reader)
	if err != nil {
		return mcp.SymbolSourceRead{}, translateSymbol(err, symbolID)
	}
	if envelope.Result == nil {
		return mcp.SymbolSourceRead{}, fmt.Errorf("symbol %q is not present in the current graph generation", symbolID)
	}
	return mcp.SymbolSourceRead{
		Pointer: pointerFrom(knowledge.SymbolCandidate{
			Symbol:     envelope.Result.Symbol,
			Occurrence: envelope.Result.Occurrence,
		}),
		Content:      envelope.Result.Source.Content,
		GraphVersion: envelope.Generation.GraphVersion,
		Freshness:    string(envelope.Generation.Freshness),
		Limitations:  envelope.Limitations,
	}, nil
}

func (g *graphSymbolIndex) SymbolRelations(ctx context.Context, root, symbolID, direction string, depth int) (mcp.SymbolRelationsRead, error) {
	identity, err := g.identity(ctx, root)
	if err != nil {
		return mcp.SymbolRelationsRead{}, err
	}
	limits := knowledge.DefaultGraphLimits()
	if depth < 1 {
		depth = 1
	}
	if depth > limits.MaxDepth {
		depth = limits.MaxDepth
	}
	incoming := direction != "outgoing"
	envelope, err := g.store.FindSymbolRelationships(ctx, identity, symbolID, incoming, depth,
		[]string{"calls", "references"}, limits)
	if err != nil {
		return mcp.SymbolRelationsRead{}, translateSymbol(err, symbolID)
	}

	// Edges name nodes by ID; the agent needs the qualified name and location.
	names := map[string]knowledge.GraphNode{}
	for _, node := range envelope.Result.Nodes {
		names[node.ID] = node
	}
	names[envelope.Result.Root.ID] = envelope.Result.Root

	rootName := envelope.Result.Root.QualifiedName
	if rootName == "" {
		rootName = envelope.Result.Root.DisplayName
	}

	out := mcp.SymbolRelationsRead{
		Root:           rootName,
		DepthRequested: depth,
		GraphVersion:   envelope.Generation.GraphVersion,
		Freshness:      string(envelope.Generation.Freshness),
		Truncated:      envelope.Truncated,
		Limitations:    envelope.Limitations,
	}
	for _, edge := range envelope.Result.Edges {
		// For incoming edges the interesting end is the caller (From); for
		// outgoing it is the callee (To).
		otherID := edge.FromNodeID
		if !incoming {
			otherID = edge.ToNodeID
		}
		other := names[otherID]
		name := other.QualifiedName
		if name == "" {
			name = other.DisplayName
		}
		if name == "" {
			name = otherID
		}
		// No line: edges carry SourceStartByte, and converting every offset
		// would mean reading each referenced file. File is what the graph
		// actually knows.
		out.Relations = append(out.Relations, mcp.SymbolRelation{
			Direction:     direction,
			EdgeType:      edge.Type,
			QualifiedName: name,
			File:          edge.SourceFilePath,
			Resolution:    string(edge.Resolution),
		})
	}
	return out, nil
}
