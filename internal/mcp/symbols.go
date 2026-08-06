package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// Symbol tools expose the repository knowledge graph to coding agents.
//
// WHY THESE EXIST: without them an agent locates code by grepping and then
// reading whole files — thousands of tokens per lookup, most of them
// irrelevant. The graph already stores the pointer (file, line range,
// signature, resolution tier), so "where is X" costs a few dozen tokens and
// "what calls X" is a query rather than a repository-wide search.
//
// These never write workspace files and are available in every permission
// mode, like read_file.
//
// THEY ARE NOT UNCONDITIONALLY SIDE-EFFECT FREE. The V2.1 read gate
// (knowledge.freshGeneration) recomputes a drifted generation rather than
// answering from stale pointers, which writes .openexec bookkeeping and, on a
// large repository, can run a full extraction — turning an advertised-cheap
// lookup into a long blocking call. A server that offers these tools as the
// cheap alternative to grep should therefore call Store.SetRefreshOnRead(false)
// and let a drifted graph return an explicit stale refusal instead. That is
// what the symbols-only profile does (`openexec mcp-serve --profile symbols`).
//
// FRESHNESS IS NOT PAPERED OVER: whichever way the reader is configured,
// handlers surface the freshness state, the refusal, and every stated
// limitation verbatim. A pointer the agent cannot trust is worse than no
// pointer.

// errNoSymbolIndex reports that the composition root did not wire a symbol
// index — the tools are advertised only when one exists, so this is a
// programming error rather than a user-facing state.
var errNoSymbolIndex = errors.New("no repository symbol index is configured for this server")

// SymbolPointer is one index entry: enough to locate a symbol and judge
// whether it is the right one, without reading the file.
type SymbolPointer struct {
	SymbolID      string `json:"symbol_id"`
	Name          string `json:"name"`
	QualifiedName string `json:"qualified_name,omitempty"`
	Kind          string `json:"kind,omitempty"`
	Language      string `json:"language,omitempty"`
	File          string `json:"file"`
	StartLine     int    `json:"start_line"`
	EndLine       int    `json:"end_line"`
	Signature     string `json:"signature,omitempty"`
	Resolution    string `json:"resolution_status,omitempty"`
	Exported      bool   `json:"exported"`
}

// SymbolPointerPage is a bounded page of pointers plus the provenance the
// caller needs to decide whether to trust them.
type SymbolPointerPage struct {
	Pointers     []SymbolPointer `json:"pointers"`
	Total        int             `json:"total"`
	Page         int             `json:"page"`
	PageSize     int             `json:"page_size"`
	TotalPages   int             `json:"total_pages"`
	GraphVersion string          `json:"graph_version,omitempty"`
	Freshness    string          `json:"freshness,omitempty"`
	Truncated    bool            `json:"truncated"`
	Limitations  []string        `json:"limitations,omitempty"`
}

// SymbolSourceRead is hash-validated current source for one symbol.
type SymbolSourceRead struct {
	Pointer      SymbolPointer `json:"pointer"`
	Content      string        `json:"content"`
	GraphVersion string        `json:"graph_version,omitempty"`
	Freshness    string        `json:"freshness,omitempty"`
	Limitations  []string      `json:"limitations,omitempty"`
}

// SymbolRelation is one edge from or to the queried symbol.
//
// There is no line number: the graph stores a byte offset for an edge, and
// converting it would mean reading every referenced file. The file is the
// honest granularity — promising a call-site line the graph does not hold
// would send an agent to a fabricated location.
type SymbolRelation struct {
	Direction     string `json:"direction"`
	EdgeType      string `json:"edge_type"`
	QualifiedName string `json:"qualified_name"`
	File          string `json:"file,omitempty"`
	Resolution    string `json:"resolution_status,omitempty"`
}

// SymbolRelationsRead is the bounded neighbourhood of one symbol.
//
// DepthRequested is what the traversal was allowed to use, not what it
// consumed: the query layer does not report the depth actually reached, so
// calling it "reached" would overstate what the result proves. Truncated is
// the field that says whether the bound was actually hit.
type SymbolRelationsRead struct {
	Root           string           `json:"root"`
	Relations      []SymbolRelation `json:"relations"`
	DepthRequested int              `json:"depth_requested"`
	GraphVersion   string           `json:"graph_version,omitempty"`
	Freshness      string           `json:"freshness,omitempty"`
	Truncated      bool             `json:"truncated"`
	Limitations    []string         `json:"limitations,omitempty"`
}

// SymbolIndex is the seam to the repository knowledge graph. The composition
// root adapts internal/knowledge to it, so this package does not import the
// graph implementation (same pattern as memoryLoader).
//
// An implementation must never widen a resolution tier and must report
// freshness rather than silently answering from a stale generation.
type SymbolIndex interface {
	FindSymbols(ctx context.Context, workspaceRoot, query, file, kind string, page, pageSize int) (SymbolPointerPage, error)
	ReadSymbol(ctx context.Context, workspaceRoot, symbolID string) (SymbolSourceRead, error)
	SymbolRelations(ctx context.Context, workspaceRoot, symbolID, direction string, depth int) (SymbolRelationsRead, error)
}

// SetSymbolIndex wires the repository symbol index. Tools are advertised only
// when this has been called.
func (s *Server) SetSymbolIndex(index SymbolIndex) {
	s.symbolIndex = index
}

// SetSymbolsOnly restricts this server to the repository symbol tools. The
// broker holds the authoritative deny; advertisement follows it.
func (s *Server) SetSymbolsOnly(only bool) {
	s.broker.SetSymbolsOnly(only)
}

// --- Tool definitions ---

// readOnlyToolAnnotations declares, in the protocol rather than in prose, that
// a tool only reads. Clients gate execution on this: Claude Code's plan mode
// blocks any MCP tool it cannot tell is safe, so without the annotation an
// agent sees the symbol tools, refuses to call them, and falls back to grep —
// the exact behaviour these tools exist to replace.
func readOnlyToolAnnotations(title string) map[string]interface{} {
	return map[string]interface{}{
		"title":           title,
		"readOnlyHint":    true,
		"destructiveHint": false,
		"idempotentHint":  true,
		"openWorldHint":   false,
	}
}

// SymbolFindRequest is the argument struct for symbol_find.
type SymbolFindRequest struct {
	Query    string `json:"query"`
	File     string `json:"file,omitempty"`
	Kind     string `json:"kind,omitempty"`
	Page     int    `json:"page,omitempty"`
	PageSize int    `json:"page_size,omitempty"`
}

func SymbolFindToolDef() map[string]interface{} {
	return map[string]interface{}{
		"name": "symbol_find",
		"description": "Locate a function, type, method or class in this repository by name, returning its exact file, line range and signature. " +
			"Prefer this over grep/ripgrep or reading files to find where something is defined: it answers from a precomputed index in a few dozen tokens instead of scanning the repository. " +
			"Returns the symbol_id needed by symbol_read and symbol_relations. Results state graph freshness and how each symbol was resolved.",
		"inputSchema": map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"query": map[string]interface{}{
					"type":        "string",
					"description": "Symbol name or fragment to search for, matched case-insensitively against display and qualified names.",
				},
				"file": map[string]interface{}{
					"type":        "string",
					"description": "Optional repository-relative file path used to disambiguate symbols that share a name.",
				},
				"kind": map[string]interface{}{
					"type":        "string",
					"description": "Optional symbol kind to filter by, for example function, method, struct, class or interface.",
				},
				"page": map[string]interface{}{
					"type":        "integer",
					"description": "1-based page number for paging through matches. Defaults to the first page.",
				},
				"page_size": map[string]interface{}{
					"type":        "integer",
					"description": "Number of matches per page, capped by the server. Keep small to save context.",
				},
			},
			"required": []string{"query"},
		},
		"annotations": readOnlyToolAnnotations("Find a symbol"),
	}
}

// SymbolReadRequest is the argument struct for symbol_read.
type SymbolReadRequest struct {
	SymbolID string `json:"symbol_id"`
}

func SymbolReadToolDef() map[string]interface{} {
	return map[string]interface{}{
		"name": "symbol_read",
		"description": "Read the exact current source of one symbol by its symbol_id, bounded to that symbol's line range and validated against the file's content hash. " +
			"Use this instead of read_file when you only need one function or type: it returns the definition alone rather than the whole file. " +
			"Call symbol_find first to obtain a symbol_id.",
		"inputSchema": map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"symbol_id": map[string]interface{}{
					"type":        "string",
					"description": "Opaque symbol identifier returned by symbol_find.",
				},
			},
			"required": []string{"symbol_id"},
		},
		"annotations": readOnlyToolAnnotations("Read a symbol's source"),
	}
}

// SymbolRelationsRequest is the argument struct for symbol_relations.
type SymbolRelationsRequest struct {
	SymbolID  string `json:"symbol_id"`
	Direction string `json:"direction,omitempty"`
	Depth     int    `json:"depth,omitempty"`
}

func SymbolRelationsToolDef() map[string]interface{} {
	return map[string]interface{}{
		"name": "symbol_relations",
		"description": "List what calls or references a symbol (incoming) or what it calls (outgoing), naming the file each edge comes from. " +
			"Use this before changing a function to see which call sites and tests are affected, and to decide which tests to run. " +
			"Traversal is bounded: the response reports the depth it was allowed to search and whether results were truncated.",
		"inputSchema": map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"symbol_id": map[string]interface{}{
					"type":        "string",
					"description": "Opaque symbol identifier returned by symbol_find.",
				},
				"direction": map[string]interface{}{
					"type":        "string",
					"enum":        []string{"incoming", "outgoing"},
					"description": "incoming lists callers and references to this symbol; outgoing lists what it calls. Defaults to incoming.",
				},
				"depth": map[string]interface{}{
					"type":        "integer",
					"description": "How many edges to traverse outward from the symbol, bounded by the server maximum.",
				},
			},
			"required": []string{"symbol_id"},
		},
		"annotations": readOnlyToolAnnotations("List a symbol's callers or callees"),
	}
}

// --- Handlers ---

// symbolWorkspace returns the workspace root symbol tools operate on.
func (s *Server) symbolWorkspace() (string, error) {
	if len(s.workspaceRoots) == 0 || s.workspaceRoots[0] == "" {
		return "", fmt.Errorf("no workspace root configured")
	}
	return s.workspaceRoots[0], nil
}

func (s *Server) handleSymbolFind(req Request, params toolsCallParams) {
	var args SymbolFindRequest
	if len(params.Arguments) > 0 {
		if err := json.Unmarshal(params.Arguments, &args); err != nil {
			s.writeError(req.ID, -32602, fmt.Sprintf("invalid symbol_find arguments: %v", err))
			return
		}
	}
	if strings.TrimSpace(args.Query) == "" {
		s.writeToolError(req.ID, "symbol_find requires a query")
		return
	}
	if s.symbolIndex == nil {
		s.writeToolError(req.ID, errNoSymbolIndex.Error())
		return
	}
	root, err := s.symbolWorkspace()
	if err != nil {
		s.writeToolError(req.ID, err.Error())
		return
	}

	page, err := s.symbolIndex.FindSymbols(s.toolContext(), root, args.Query, args.File, args.Kind, args.Page, args.PageSize)
	if err != nil {
		s.writeToolError(req.ID, err.Error())
		return
	}

	var lines []string
	for _, p := range page.Pointers {
		name := p.QualifiedName
		if name == "" {
			name = p.Name
		}
		line := fmt.Sprintf("- %s", name)
		if p.Kind != "" {
			line += " (" + p.Kind + ")"
		}
		line += fmt.Sprintf(" %s:%d-%d", p.File, p.StartLine, p.EndLine)
		if p.Resolution != "" {
			line += " [" + p.Resolution + "]"
		}
		line += " id=" + p.SymbolID
		if p.Signature != "" {
			line += "\n    " + p.Signature
		}
		lines = append(lines, line)
	}

	text := fmt.Sprintf("%d match(es) for %q", page.Total, args.Query)
	if page.TotalPages > 1 {
		text += fmt.Sprintf(" — page %d of %d", page.Page, page.TotalPages)
	}
	text += symbolProvenance(page.GraphVersion, page.Freshness)
	if len(lines) > 0 {
		text += "\n" + strings.Join(lines, "\n")
	} else {
		text += "\nNo symbol matched. The graph may not cover this language, or the name may differ — fall back to a file search."
	}
	text += symbolCaveats(page.Truncated, page.Limitations)

	s.writeResult(req.ID, map[string]interface{}{
		"content": []interface{}{map[string]interface{}{"type": "text", "text": text}},
		"result":  page,
	})
}

func (s *Server) handleSymbolRead(req Request, params toolsCallParams) {
	var args SymbolReadRequest
	if len(params.Arguments) > 0 {
		if err := json.Unmarshal(params.Arguments, &args); err != nil {
			s.writeError(req.ID, -32602, fmt.Sprintf("invalid symbol_read arguments: %v", err))
			return
		}
	}
	if strings.TrimSpace(args.SymbolID) == "" {
		s.writeToolError(req.ID, "symbol_read requires a symbol_id (call symbol_find first)")
		return
	}
	if s.symbolIndex == nil {
		s.writeToolError(req.ID, errNoSymbolIndex.Error())
		return
	}
	root, err := s.symbolWorkspace()
	if err != nil {
		s.writeToolError(req.ID, err.Error())
		return
	}

	read, err := s.symbolIndex.ReadSymbol(s.toolContext(), root, args.SymbolID)
	if err != nil {
		s.writeToolError(req.ID, err.Error())
		return
	}

	name := read.Pointer.QualifiedName
	if name == "" {
		name = read.Pointer.Name
	}
	text := fmt.Sprintf("%s (%s) %s:%d-%d", name, read.Pointer.Kind,
		read.Pointer.File, read.Pointer.StartLine, read.Pointer.EndLine)
	text += symbolProvenance(read.GraphVersion, read.Freshness)
	// Source is the payload here: caveats go after it, so a client that
	// truncates a long result loses the disclosure rather than the code.
	text += "\n\n" + read.Content
	text += symbolCaveats(false, read.Limitations)

	s.writeResult(req.ID, map[string]interface{}{
		"content": []interface{}{map[string]interface{}{"type": "text", "text": text}},
		"result":  read,
	})
}

func (s *Server) handleSymbolRelations(req Request, params toolsCallParams) {
	var args SymbolRelationsRequest
	if len(params.Arguments) > 0 {
		if err := json.Unmarshal(params.Arguments, &args); err != nil {
			s.writeError(req.ID, -32602, fmt.Sprintf("invalid symbol_relations arguments: %v", err))
			return
		}
	}
	if strings.TrimSpace(args.SymbolID) == "" {
		s.writeToolError(req.ID, "symbol_relations requires a symbol_id (call symbol_find first)")
		return
	}
	direction := strings.TrimSpace(args.Direction)
	if direction == "" {
		direction = "incoming"
	}
	if direction != "incoming" && direction != "outgoing" {
		s.writeToolError(req.ID, `direction must be "incoming" or "outgoing"`)
		return
	}
	if s.symbolIndex == nil {
		s.writeToolError(req.ID, errNoSymbolIndex.Error())
		return
	}
	root, err := s.symbolWorkspace()
	if err != nil {
		s.writeToolError(req.ID, err.Error())
		return
	}

	rel, err := s.symbolIndex.SymbolRelations(s.toolContext(), root, args.SymbolID, direction, args.Depth)
	if err != nil {
		s.writeToolError(req.ID, err.Error())
		return
	}

	var lines []string
	for _, r := range rel.Relations {
		line := fmt.Sprintf("- %s %s", r.EdgeType, r.QualifiedName)
		if r.File != "" {
			line += fmt.Sprintf(" (%s)", r.File)
		}
		if r.Resolution != "" {
			line += " [" + r.Resolution + "]"
		}
		lines = append(lines, line)
	}

	label := "callers and references"
	if direction == "outgoing" {
		label = "outgoing calls and references"
	}
	text := fmt.Sprintf("%d %s of %s (searched to depth %d)", len(rel.Relations), label, rel.Root, rel.DepthRequested)
	text += symbolProvenance(rel.GraphVersion, rel.Freshness)
	if len(lines) > 0 {
		text += "\n" + strings.Join(lines, "\n")
	} else if direction == "incoming" {
		text += "\nNothing in the graph references this symbol. It may be an entry point, dead code, or reached only dynamically — check the limitations below before concluding it is unused."
	}
	text += symbolCaveats(rel.Truncated, rel.Limitations)

	s.writeResult(req.ID, map[string]interface{}{
		"content": []interface{}{map[string]interface{}{"type": "text", "text": text}},
		"result":  rel,
	})
}

// symbolProvenance renders graph version and freshness. Freshness is never
// omitted: an answer whose staleness the caller cannot see is the failure
// mode these tools exist to avoid.
func symbolProvenance(graphVersion, freshness string) string {
	parts := []string{}
	if freshness != "" {
		parts = append(parts, "freshness: "+freshness)
	}
	if graphVersion != "" {
		parts = append(parts, "graph "+graphVersion)
	}
	if len(parts) == 0 {
		return ""
	}
	return " — " + strings.Join(parts, ", ")
}

// maxRenderedCaveats bounds how many limitations are spelled out. Graph
// limitations are generation-wide — a repository with an unbuildable vendored
// template contributes dozens of diagnostics that have nothing to do with the
// symbol being looked up. Printing them all costs more context than the file
// read these tools exist to avoid, so the list is capped and the remainder is
// counted. The count is what keeps a capped answer honest.
const maxRenderedCaveats = 5

// symbolCaveats appends truncation and limitations so a bounded answer never
// presents itself as complete.
func symbolCaveats(truncated bool, limitations []string) string {
	out := ""
	if truncated {
		out += "\n\nTruncated: results were cut off by the server bound; narrow the query rather than assuming this is the full set."
	}
	if len(limitations) == 0 {
		return out
	}
	shown := limitations
	if len(shown) > maxRenderedCaveats {
		shown = shown[:maxRenderedCaveats]
	}
	out += "\n\nLimitations:\n- " + strings.Join(shown, "\n- ")
	if remaining := len(limitations) - len(shown); remaining > 0 {
		out += fmt.Sprintf("\n- …and %d more limitation(s) on this graph generation.", remaining)
	}
	return out
}
