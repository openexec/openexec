package knowledge

import "context"

type SourceReadRequest struct {
	RepositoryID string `json:"repository_id"`
	WorktreeID   string `json:"worktree_id"`
	FilePath     string `json:"file_path"`
	StartByte    int    `json:"start_byte"`
	EndByte      int    `json:"end_byte"`
	FileHash     string `json:"file_content_hash"`
	RangeHash    string `json:"source_range_hash"`
}

type SourceReadResult struct {
	FilePath  string `json:"file_path"`
	StartByte int    `json:"start_byte"`
	EndByte   int    `json:"end_byte"`
	Content   string `json:"content"`
	FileHash  string `json:"file_content_hash"`
	RangeHash string `json:"source_range_hash"`
}

// RepositoryReader is the source-access authority used after graph resolution.
// Implementations enforce repository scope independently of graph pointers.
type RepositoryReader interface {
	ReadRange(context.Context, SourceReadRequest) (SourceReadResult, error)
}

type SymbolSource struct {
	Symbol     GraphSymbol      `json:"symbol"`
	Occurrence SymbolOccurrence `json:"occurrence"`
	Source     SourceReadResult `json:"source"`
}

func (s *Store) ReadGraphSymbol(ctx context.Context, identity RepositoryIdentity, symbolID string, reader RepositoryReader) (QueryEnvelope[*SymbolSource], error) {
	generation, err := s.activeGeneration(ctx, identity.WorktreeID)
	if err != nil {
		return QueryEnvelope[*SymbolSource]{}, err
	}
	var candidate SymbolCandidate
	var exported int
	err = s.db.QueryRowContext(ctx, `SELECT s.id, s.repository_id, s.language, s.kind, s.display_name, s.qualified_name,
		o.symbol_id, o.generation_id, o.node_id, o.file_path, o.start_line, o.end_line,
		o.start_byte, o.end_byte, o.signature, o.file_content_hash, o.source_range_hash,
		o.exported, o.resolution_status
		FROM repository_symbols s JOIN symbol_occurrences o ON o.symbol_id = s.id
		WHERE s.id = ? AND o.generation_id = ?`, symbolID, generation.ID).Scan(
		&candidate.Symbol.ID, &candidate.Symbol.RepositoryID, &candidate.Symbol.Language,
		&candidate.Symbol.Kind, &candidate.Symbol.DisplayName, &candidate.Symbol.QualifiedName,
		&candidate.Occurrence.SymbolID, &candidate.Occurrence.GenerationID,
		&candidate.Occurrence.NodeID, &candidate.Occurrence.FilePath,
		&candidate.Occurrence.StartLine, &candidate.Occurrence.EndLine,
		&candidate.Occurrence.StartByte, &candidate.Occurrence.EndByte,
		&candidate.Occurrence.Signature, &candidate.Occurrence.FileHash,
		&candidate.Occurrence.RangeHash, &exported, &candidate.Occurrence.Resolution)
	if err != nil {
		return QueryEnvelope[*SymbolSource]{}, err
	}
	candidate.Occurrence.Exported = exported != 0
	read, err := reader.ReadRange(ctx, SourceReadRequest{
		RepositoryID: identity.RepositoryID, WorktreeID: identity.WorktreeID,
		FilePath: candidate.Occurrence.FilePath, StartByte: candidate.Occurrence.StartByte,
		EndByte: candidate.Occurrence.EndByte, FileHash: candidate.Occurrence.FileHash,
		RangeHash: candidate.Occurrence.RangeHash,
	})
	if err != nil {
		return QueryEnvelope[*SymbolSource]{}, err
	}
	return QueryEnvelope[*SymbolSource]{
		Query:       QueryMeta{Type: "read_symbol", Roots: []string{symbolID}},
		Generation:  generationState(generation),
		Result:      &SymbolSource{Symbol: candidate.Symbol, Occurrence: candidate.Occurrence, Source: read},
		Resolution:  ResolutionMeta{Status: "resolved", Methods: []ResolutionStatus{candidate.Occurrence.Resolution}},
		Limitations: generation.Limitations,
	}, nil
}
