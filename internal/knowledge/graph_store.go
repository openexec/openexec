package knowledge

import (
	"bufio"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
)

func stableID(prefix string, values ...string) string {
	h := sha256.New()
	for _, value := range values {
		_, _ = h.Write([]byte(value))
		_, _ = h.Write([]byte{0})
	}
	return prefix + "_" + hex.EncodeToString(h.Sum(nil)[:16])
}

func hashBytes(data []byte) string {
	sum := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func jsonText(value any, fallback string) string {
	data, err := json.Marshal(value)
	if err != nil {
		return fallback
	}
	return string(data)
}

// BeginGeneration creates an unpublished graph generation.
func (s *Store) BeginGeneration(ctx context.Context, identity RepositoryIdentity, manifest ScanManifest, capabilities map[string]string, limitations []string) (GraphGeneration, error) {
	generation := GraphGeneration{
		ID:                  "graph_" + uuid.NewString(),
		SchemaVersion:       GraphSchemaVersion,
		RepositoryID:        identity.RepositoryID,
		CheckoutID:          identity.CheckoutID,
		WorktreeID:          identity.WorktreeID,
		BaseCommit:          identity.BaseCommit,
		WorktreeStateHash:   manifest.WorktreeStateHash,
		ConfigurationDigest: manifest.ConfigurationDigest,
		ExtractorVersion:    ExtractorVersion,
		ManifestHash:        manifest.ManifestHash,
		Status:              GraphBuilding,
		Capabilities:        capabilities,
		Limitations:         limitations,
		CreatedAt:           time.Now().UTC(),
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return GraphGeneration{}, fmt.Errorf("begin graph generation: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if err := insertGeneration(ctx, tx, generation); err != nil {
		return GraphGeneration{}, err
	}
	for _, input := range manifest.Inputs {
		if _, err := tx.ExecContext(ctx, `INSERT INTO graph_scan_inputs
			(generation_id, file_path, input_kind, size, content_hash, symlink_target, included)
			VALUES (?, ?, ?, ?, ?, ?, ?)`, generation.ID, input.FilePath, input.InputKind, input.Size, input.ContentHash, input.SymlinkTarget, boolInt(input.Included)); err != nil {
			return GraphGeneration{}, fmt.Errorf("insert scan input %s: %w", input.FilePath, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return GraphGeneration{}, fmt.Errorf("commit graph generation: %w", err)
	}
	return generation, nil
}

func insertGeneration(ctx context.Context, tx *sql.Tx, generation GraphGeneration) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO graph_generations
		(id, schema_version, repository_id, checkout_id, worktree_id, base_commit,
		 worktree_state_hash, configuration_digest, extractor_version, manifest_hash,
		 status, capabilities, limitations, error_message)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		generation.ID, generation.SchemaVersion, generation.RepositoryID,
		generation.CheckoutID, generation.WorktreeID, generation.BaseCommit,
		generation.WorktreeStateHash, generation.ConfigurationDigest,
		generation.ExtractorVersion, generation.ManifestHash, generation.Status,
		jsonText(generation.Capabilities, "{}"), jsonText(generation.Limitations, "[]"), generation.ErrorMessage)
	if err != nil {
		return fmt.Errorf("insert graph generation: %w", err)
	}
	return nil
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

// FailGeneration records a terminal generation failure without changing the
// active generation.
func (s *Store) FailGeneration(ctx context.Context, generationID string, status GraphStatus, message string) error {
	if status != GraphFailed && status != GraphInconsistent && status != GraphIncompatible {
		return fmt.Errorf("invalid failed generation status %q", status)
	}
	_, err := s.db.ExecContext(ctx, `UPDATE graph_generations SET status = ?, error_message = ?, completed_at = CURRENT_TIMESTAMP WHERE id = ? AND status = 'building'`, status, message, generationID)
	return err
}

// failGenerationAfterError persists terminal state even when the operation's
// context was cancelled. Without a detached, bounded cleanup context, Ctrl-C
// left generations in "building" until another scan happened to sweep them.
func (s *Store) failGenerationAfterError(ctx context.Context, generationID string, status GraphStatus, message string) error {
	cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	return s.FailGeneration(cleanupCtx, generationID, status, message)
}

// PromoteGeneration atomically supersedes the prior active graph and promotes
// a fully built generation. Partial generations remain diagnostic only.
func (s *Store) PromoteGeneration(ctx context.Context, generationID string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin generation promotion: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	var worktreeID, repositoryID, status string
	if err := tx.QueryRowContext(ctx, `SELECT worktree_id, repository_id, status FROM graph_generations WHERE id = ?`, generationID).Scan(&worktreeID, &repositoryID, &status); err != nil {
		return fmt.Errorf("load generation for promotion: %w", err)
	}
	if status != string(GraphBuilding) {
		return fmt.Errorf("generation %s is %s, not building", generationID, status)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE graph_generations SET status = 'superseded', completed_at = COALESCE(completed_at, CURRENT_TIMESTAMP) WHERE worktree_id = ? AND status = 'current'`, worktreeID); err != nil {
		return fmt.Errorf("supersede current generation: %w", err)
	}
	// Retirement is part of the same transaction as promotion. Partial,
	// inconsistent, and failed generations can never retire canonical symbols.
	if _, err := tx.ExecContext(ctx, `UPDATE repository_symbols SET retired_at = CURRENT_TIMESTAMP WHERE repository_id = ? AND retired_at IS NULL AND NOT EXISTS (SELECT 1 FROM symbol_occurrences o WHERE o.symbol_id = repository_symbols.id AND o.generation_id = ?)`, repositoryID, generationID); err != nil {
		return fmt.Errorf("retire absent symbols: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE repository_symbols SET retired_at = NULL WHERE repository_id = ? AND EXISTS (SELECT 1 FROM symbol_occurrences o WHERE o.symbol_id = repository_symbols.id AND o.generation_id = ?)`, repositoryID, generationID); err != nil {
		return fmt.Errorf("activate current symbols: %w", err)
	}
	result, err := tx.ExecContext(ctx, `UPDATE graph_generations SET status = 'current', completed_at = CURRENT_TIMESTAMP, promoted_at = CURRENT_TIMESTAMP WHERE id = ? AND status = 'building'`, generationID)
	if err != nil {
		return fmt.Errorf("promote generation: %w", err)
	}
	if changed, _ := result.RowsAffected(); changed != 1 {
		return fmt.Errorf("generation %s was not promoted", generationID)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit generation promotion: %w", err)
	}
	return nil
}

func (s *Store) activeGeneration(ctx context.Context, worktreeID string) (GraphGeneration, error) {
	row := s.db.QueryRowContext(ctx, `SELECT id, schema_version, repository_id, checkout_id, worktree_id,
		base_commit, worktree_state_hash, configuration_digest, extractor_version,
		manifest_hash, status, capabilities, limitations, error_message, created_at
		FROM graph_generations WHERE worktree_id = ?
		ORDER BY CASE status WHEN 'current' THEN 0 WHEN 'partial' THEN 1 ELSE 2 END, created_at DESC LIMIT 1`, worktreeID)
	return scanGeneration(row)
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanGeneration(row rowScanner) (GraphGeneration, error) {
	var generation GraphGeneration
	var capabilities, limitations string
	err := row.Scan(&generation.ID, &generation.SchemaVersion, &generation.RepositoryID,
		&generation.CheckoutID, &generation.WorktreeID, &generation.BaseCommit,
		&generation.WorktreeStateHash, &generation.ConfigurationDigest,
		&generation.ExtractorVersion, &generation.ManifestHash, &generation.Status,
		&capabilities, &limitations, &generation.ErrorMessage, &generation.CreatedAt)
	if err != nil {
		return GraphGeneration{}, err
	}
	_ = json.Unmarshal([]byte(capabilities), &generation.Capabilities)
	_ = json.Unmarshal([]byte(limitations), &generation.Limitations)
	return generation, nil
}

func generationState(generation GraphGeneration) RepositoryState {
	freshness := FreshnessMissing
	switch generation.Status {
	case GraphCurrent:
		freshness = FreshnessCurrent
	case GraphStale, GraphSuperseded:
		freshness = FreshnessStale
	case GraphPartial, GraphBuilding, GraphFailed:
		freshness = FreshnessPartial
	case GraphInconsistent:
		freshness = FreshnessInconsistent
	case GraphIncompatible:
		freshness = FreshnessIncompatible
	}
	return RepositoryState{
		RepositoryID: generation.RepositoryID, CheckoutID: generation.CheckoutID,
		WorktreeID: generation.WorktreeID, BaseCommit: generation.BaseCommit,
		WorktreeStateHash:   generation.WorktreeStateHash,
		ConfigurationDigest: generation.ConfigurationDigest,
		ExtractorVersion:    generation.ExtractorVersion, GraphVersion: generation.ID,
		Freshness: freshness,
	}
}

// MigrateLegacySymbols additively copies legacy symbol pointers into a partial
// graph generation. It is idempotent and never deletes or rewrites legacy rows.
func (s *Store) MigrateLegacySymbols(ctx context.Context, identity RepositoryIdentity) (string, error) {
	var existing string
	err := s.db.QueryRowContext(ctx, `SELECT id FROM graph_generations WHERE worktree_id = ? AND extractor_version = 'legacy-pointer-v0' ORDER BY created_at DESC LIMIT 1`, identity.WorktreeID).Scan(&existing)
	if err == nil {
		return existing, nil
	}
	if err != sql.ErrNoRows {
		return "", fmt.Errorf("check legacy migration: %w", err)
	}
	legacy, err := s.ListSymbols()
	if err != nil {
		return "", fmt.Errorf("list legacy symbols: %w", err)
	}
	if len(legacy) == 0 {
		return "", nil
	}

	generation := GraphGeneration{
		ID: "graph_legacy_" + uuid.NewString(), SchemaVersion: GraphSchemaVersion,
		RepositoryID: identity.RepositoryID, CheckoutID: identity.CheckoutID,
		WorktreeID: identity.WorktreeID, BaseCommit: identity.BaseCommit,
		WorktreeStateHash: "legacy-unversioned", ConfigurationDigest: "legacy-unversioned",
		ExtractorVersion: "legacy-pointer-v0", ManifestHash: "legacy-unversioned",
		Status: GraphPartial, Capabilities: map[string]string{"definitions": "legacy"},
		Limitations: []string{"legacy pointers have no scan manifest or complete byte ranges"},
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer func() { _ = tx.Rollback() }()
	if err := insertGeneration(ctx, tx, generation); err != nil {
		return "", err
	}

	sort.Slice(legacy, func(i, j int) bool {
		if legacy[i].FilePath == legacy[j].FilePath {
			return legacy[i].Name < legacy[j].Name
		}
		return legacy[i].FilePath < legacy[j].FilePath
	})
	for _, old := range legacy {
		rel := normalizeStoredPath(identity.RootPath, old.FilePath)
		fileHash, rangeHash, startByte, endByte := hashesForLineRange(identity.RootPath, rel, old.StartLine, old.EndLine)
		kind := old.Kind
		if kind == "" {
			kind = "unknown"
		}
		symbolID := stableID("sym", identity.RepositoryID, "legacy", kind, old.Name, rel)
		nodeID := stableID("node", generation.ID, symbolID)
		if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO repository_symbols (id, repository_id, language, kind, display_name, qualified_name) VALUES (?, ?, 'legacy', ?, ?, ?)`, symbolID, identity.RepositoryID, kind, old.Name, old.Name); err != nil {
			return "", fmt.Errorf("backfill legacy symbol %s: %w", old.Name, err)
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO graph_nodes (id, generation_id, repository_id, node_type, language, display_name, qualified_name) VALUES (?, ?, ?, 'symbol', 'legacy', ?, ?)`, nodeID, generation.ID, identity.RepositoryID, old.Name, old.Name); err != nil {
			return "", fmt.Errorf("backfill legacy node %s: %w", old.Name, err)
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO symbol_occurrences
			(symbol_id, generation_id, node_id, file_path, start_line, end_line, start_byte, end_byte, signature, file_content_hash, source_range_hash, resolution_status)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'heuristic')`, symbolID, generation.ID, nodeID, rel, old.StartLine, old.EndLine, startByte, endByte, old.Signature, fileHash, rangeHash); err != nil {
			return "", fmt.Errorf("backfill legacy occurrence %s: %w", old.Name, err)
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO symbol_lineage (id, repository_id, symbol_id, continuity_status, resolution_method, generation_id) VALUES (?, ?, ?, 'new', 'heuristic', ?)`, stableID("lineage", generation.ID, symbolID), identity.RepositoryID, symbolID, generation.ID); err != nil {
			return "", fmt.Errorf("backfill legacy lineage %s: %w", old.Name, err)
		}
	}
	if _, err := tx.ExecContext(ctx, `UPDATE graph_generations SET completed_at = CURRENT_TIMESTAMP WHERE id = ?`, generation.ID); err != nil {
		return "", err
	}
	if err := tx.Commit(); err != nil {
		return "", fmt.Errorf("commit legacy migration: %w", err)
	}
	return generation.ID, nil
}

func normalizeStoredPath(root, stored string) string {
	stored = filepath.Clean(stored)
	if filepath.IsAbs(stored) {
		// Canonicalize both sides before relativizing: on macOS the temp root
		// crosses the /var -> /private/var symlink, so a legacy absolute path
		// and the evaluated root otherwise disagree and the path stays
		// absolute, leaking the host layout into the graph.
		base := root
		if evaluated, err := filepath.EvalSymlinks(root); err == nil {
			base = evaluated
		}
		candidate := stored
		if evaluated, err := filepath.EvalSymlinks(stored); err == nil {
			candidate = evaluated
		}
		if rel, err := filepath.Rel(base, candidate); err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			stored = rel
		}
	}
	return filepath.ToSlash(stored)
}

func hashesForLineRange(root, rel string, startLine, endLine int) (string, string, int, int) {
	if rel == "" || filepath.IsAbs(rel) || strings.HasPrefix(filepath.Clean(rel), "..") {
		return "", "", 0, 0
	}
	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
	if err != nil {
		return "", "", 0, 0
	}
	start, end := byteRangeForLines(data, startLine, endLine)
	return hashBytes(data), hashBytes(data[start:end]), start, end
}

func byteRangeForLines(data []byte, startLine, endLine int) (int, int) {
	if startLine < 1 {
		startLine = 1
	}
	if endLine < startLine {
		endLine = startLine
	}
	start, end, offset, line := 0, len(data), 0, 1
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	for scanner.Scan() {
		text := scanner.Text()
		lineEnd := offset + len(text)
		if line == startLine {
			start = offset
		}
		if line == endLine {
			end = lineEnd
			break
		}
		offset = lineEnd + 1
		line++
	}
	if start > len(data) {
		start = len(data)
	}
	if end < start || end > len(data) {
		end = len(data)
	}
	return start, end
}

// ResolveGraphSymbol resolves against the current generation or, during
// migration, the newest partial generation. It never chooses an ambiguous name.
func (s *Store) ResolveGraphSymbol(ctx context.Context, identity RepositoryIdentity, name, file, kind string, maxCandidates int) (QueryEnvelope[SymbolResolution], error) {
	if maxCandidates <= 0 || maxCandidates > 100 {
		maxCandidates = 20
	}
	generation, err := s.activeGeneration(ctx, identity.WorktreeID)
	if err == sql.ErrNoRows {
		return QueryEnvelope[SymbolResolution]{
			Query:       QueryMeta{Type: "resolve_symbol", Roots: []string{name}},
			Generation:  RepositoryState{RepositoryID: identity.RepositoryID, CheckoutID: identity.CheckoutID, WorktreeID: identity.WorktreeID, Freshness: FreshnessMissing},
			Result:      SymbolResolution{Status: "unresolved"},
			Resolution:  ResolutionMeta{Status: "unresolved", Methods: []ResolutionStatus{ResolutionUnresolved}},
			Limitations: []string{"repository graph is missing"},
		}, nil
	}
	if err != nil {
		return QueryEnvelope[SymbolResolution]{}, fmt.Errorf("load graph generation: %w", err)
	}

	query := `SELECT s.id, s.repository_id, s.language, s.kind, s.display_name, s.qualified_name,
		o.symbol_id, o.generation_id, o.node_id, o.file_path, o.start_line, o.end_line,
		o.start_byte, o.end_byte, o.signature, o.file_content_hash, o.source_range_hash,
		o.exported, o.resolution_status
		FROM repository_symbols s JOIN symbol_occurrences o ON o.symbol_id = s.id
		WHERE o.generation_id = ? AND (s.display_name = ? OR s.qualified_name = ?)`
	args := []any{generation.ID, name, name}
	if file != "" {
		query += ` AND o.file_path = ?`
		args = append(args, filepath.ToSlash(filepath.Clean(file)))
	}
	if kind != "" {
		query += ` AND s.kind = ?`
		args = append(args, kind)
	}
	query += ` ORDER BY o.file_path, o.start_line LIMIT ?`
	args = append(args, maxCandidates+1)
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return QueryEnvelope[SymbolResolution]{}, fmt.Errorf("resolve graph symbol: %w", err)
	}
	defer rows.Close()
	var candidates []SymbolCandidate
	var methods []ResolutionStatus
	for rows.Next() {
		var candidate SymbolCandidate
		var exported int
		if err := rows.Scan(&candidate.Symbol.ID, &candidate.Symbol.RepositoryID,
			&candidate.Symbol.Language, &candidate.Symbol.Kind, &candidate.Symbol.DisplayName,
			&candidate.Symbol.QualifiedName, &candidate.Occurrence.SymbolID,
			&candidate.Occurrence.GenerationID, &candidate.Occurrence.NodeID,
			&candidate.Occurrence.FilePath, &candidate.Occurrence.StartLine,
			&candidate.Occurrence.EndLine, &candidate.Occurrence.StartByte,
			&candidate.Occurrence.EndByte, &candidate.Occurrence.Signature,
			&candidate.Occurrence.FileHash, &candidate.Occurrence.RangeHash, &exported,
			&candidate.Occurrence.Resolution); err != nil {
			return QueryEnvelope[SymbolResolution]{}, err
		}
		candidate.Occurrence.Exported = exported != 0
		candidates = append(candidates, candidate)
		methods = append(methods, candidate.Occurrence.Resolution)
	}
	if err := rows.Err(); err != nil {
		return QueryEnvelope[SymbolResolution]{}, err
	}

	truncated := len(candidates) > maxCandidates
	if truncated {
		candidates = candidates[:maxCandidates]
	}
	result := SymbolResolution{Status: "unresolved"}
	resolutionStatus := "unresolved"
	if len(candidates) == 1 && !truncated {
		result.Status = "resolved"
		result.Candidate = &candidates[0]
		resolutionStatus = "resolved"
	} else if len(candidates) > 1 || truncated {
		result.Status = "ambiguous"
		result.Candidates = candidates
		resolutionStatus = "ambiguous"
	}
	return QueryEnvelope[SymbolResolution]{
		Query: QueryMeta{Type: "resolve_symbol", Roots: []string{name}}, Generation: generationState(generation),
		Result: result, Resolution: ResolutionMeta{Status: resolutionStatus, Methods: uniqueMethods(methods)},
		Limitations: generation.Limitations, Truncated: truncated,
	}, nil
}

func uniqueMethods(methods []ResolutionStatus) []ResolutionStatus {
	seen := make(map[ResolutionStatus]bool)
	result := make([]ResolutionStatus, 0, len(methods))
	for _, method := range methods {
		if !seen[method] {
			seen[method] = true
			result = append(result, method)
		}
	}
	return result
}
