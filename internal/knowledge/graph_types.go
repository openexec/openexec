package knowledge

import "time"

const (
	GraphSchemaVersion = 1
	ExtractorVersion   = "repository-graph-v2.2"
)

type GraphStatus string

const (
	GraphBuilding     GraphStatus = "building"
	GraphCurrent      GraphStatus = "current"
	GraphStale        GraphStatus = "stale"
	GraphPartial      GraphStatus = "partial"
	GraphInconsistent GraphStatus = "inconsistent"
	GraphFailed       GraphStatus = "failed"
	GraphIncompatible GraphStatus = "incompatible"
	GraphSuperseded   GraphStatus = "superseded"
)

type Freshness string

const (
	FreshnessCurrent      Freshness = "current"
	FreshnessStale        Freshness = "stale"
	FreshnessPartial      Freshness = "partial"
	FreshnessInconsistent Freshness = "inconsistent"
	FreshnessIncompatible Freshness = "incompatible"
	FreshnessMissing      Freshness = "missing"
)

type ResolutionStatus string

const (
	ResolutionCompilerExact        ResolutionStatus = "compiler_exact"
	ResolutionASTExact             ResolutionStatus = "ast_exact"
	ResolutionConfigurationDerived ResolutionStatus = "configuration_derived"
	ResolutionStaticLexical        ResolutionStatus = "static_lexical"
	ResolutionHeuristic            ResolutionStatus = "heuristic"
	ResolutionUnresolved           ResolutionStatus = "unresolved"
	ResolutionAmbiguous            ResolutionStatus = "ambiguous"
)

type RepositoryIdentity struct {
	RepositoryID string `json:"repository_id"`
	CheckoutID   string `json:"checkout_id"`
	WorktreeID   string `json:"worktree_id"`
	RootPath     string `json:"root_path"`
	BaseCommit   string `json:"base_commit"`
}

type RepositoryState struct {
	RepositoryID        string    `json:"repository_id"`
	CheckoutID          string    `json:"checkout_id"`
	WorktreeID          string    `json:"worktree_id"`
	BaseCommit          string    `json:"base_commit"`
	WorktreeStateHash   string    `json:"worktree_state_hash"`
	ConfigurationDigest string    `json:"configuration_digest"`
	ExtractorVersion    string    `json:"extractor_version"`
	GraphVersion        string    `json:"graph_version"`
	Freshness           Freshness `json:"freshness"`
}

type ScanInput struct {
	FilePath      string `json:"file_path"`
	InputKind     string `json:"input_kind"`
	Size          int64  `json:"size"`
	ContentHash   string `json:"content_hash"`
	SymlinkTarget string `json:"symlink_target,omitempty"`
	Included      bool   `json:"included"`
}

type ScanManifest struct {
	Inputs              []ScanInput `json:"inputs"`
	ManifestHash        string      `json:"manifest_hash"`
	WorktreeStateHash   string      `json:"worktree_state_hash"`
	ConfigurationDigest string      `json:"configuration_digest"`
}

type GraphGeneration struct {
	ID                  string            `json:"id"`
	SchemaVersion       int               `json:"schema_version"`
	RepositoryID        string            `json:"repository_id"`
	CheckoutID          string            `json:"checkout_id"`
	WorktreeID          string            `json:"worktree_id"`
	BaseCommit          string            `json:"base_commit"`
	WorktreeStateHash   string            `json:"worktree_state_hash"`
	ConfigurationDigest string            `json:"configuration_digest"`
	ExtractorVersion    string            `json:"extractor_version"`
	ManifestHash        string            `json:"manifest_hash"`
	Status              GraphStatus       `json:"status"`
	Capabilities        map[string]string `json:"capabilities"`
	Limitations         []string          `json:"limitations"`
	ErrorMessage        string            `json:"error_message,omitempty"`
	CreatedAt           time.Time         `json:"created_at"`
}

type GraphNode struct {
	ID            string            `json:"id"`
	GenerationID  string            `json:"generation_id"`
	RepositoryID  string            `json:"repository_id"`
	NodeType      string            `json:"node_type"`
	Language      string            `json:"language,omitempty"`
	DisplayName   string            `json:"display_name"`
	QualifiedName string            `json:"qualified_name,omitempty"`
	Metadata      map[string]string `json:"metadata,omitempty"`
}

type GraphSymbol struct {
	ID            string `json:"symbol_id"`
	RepositoryID  string `json:"repository_id"`
	Language      string `json:"language"`
	Kind          string `json:"kind"`
	DisplayName   string `json:"display_name"`
	QualifiedName string `json:"qualified_name,omitempty"`
}

type SymbolOccurrence struct {
	SymbolID     string           `json:"symbol_id"`
	GenerationID string           `json:"generation_id"`
	NodeID       string           `json:"node_id"`
	FilePath     string           `json:"file_path"`
	StartLine    int              `json:"start_line"`
	EndLine      int              `json:"end_line"`
	StartByte    int              `json:"start_byte"`
	EndByte      int              `json:"end_byte"`
	Signature    string           `json:"signature,omitempty"`
	FileHash     string           `json:"file_content_hash"`
	RangeHash    string           `json:"source_range_hash"`
	Exported     bool             `json:"exported"`
	Resolution   ResolutionStatus `json:"resolution_status"`
}

type GraphEdge struct {
	ID              string            `json:"id"`
	GenerationID    string            `json:"generation_id"`
	FromNodeID      string            `json:"from_node_id"`
	ToNodeID        string            `json:"to_node_id"`
	Type            string            `json:"edge_type"`
	Resolution      ResolutionStatus  `json:"resolution_status"`
	SourceFilePath  string            `json:"source_file_path,omitempty"`
	SourceStartByte int               `json:"source_start_byte,omitempty"`
	SourceEndByte   int               `json:"source_end_byte,omitempty"`
	Metadata        map[string]string `json:"metadata,omitempty"`
}

type SymbolCandidate struct {
	Symbol     GraphSymbol      `json:"symbol"`
	Occurrence SymbolOccurrence `json:"occurrence"`
}

type QueryMeta struct {
	Type  string   `json:"type"`
	Roots []string `json:"roots,omitempty"`
}

type ResolutionMeta struct {
	Status  string             `json:"status"`
	Methods []ResolutionStatus `json:"methods,omitempty"`
}

type QueryEnvelope[T any] struct {
	Query       QueryMeta       `json:"query"`
	Generation  RepositoryState `json:"generation"`
	Result      T               `json:"result"`
	Resolution  ResolutionMeta  `json:"resolution"`
	Limitations []string        `json:"limitations,omitempty"`
	Truncated   bool            `json:"truncated"`
	Pagination  *PaginationMeta `json:"pagination,omitempty"`
}

type PaginationMeta struct {
	Page       int `json:"page"`
	PageSize   int `json:"page_size"`
	Total      int `json:"total"`
	TotalPages int `json:"total_pages"`
}

type SymbolResolution struct {
	Status     string            `json:"resolution_status"`
	Candidate  *SymbolCandidate  `json:"candidate,omitempty"`
	Candidates []SymbolCandidate `json:"candidates,omitempty"`
}

type GraphLimits struct {
	MaxDepth int
	MaxNodes int
	MaxEdges int
	MaxBytes int
}

func DefaultGraphLimits() GraphLimits {
	return GraphLimits{MaxDepth: 2, MaxNodes: 200, MaxEdges: 400, MaxBytes: 256 * 1024}
}
