package knowledge

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type InvalidationScope string
type InvalidationCause string

const (
	InvalidationFile                InvalidationScope = "file"
	InvalidationReverseDependencies InvalidationScope = "reverse_dependencies"
	InvalidationPackage             InvalidationScope = "package"
	InvalidationRepository          InvalidationScope = "repository"

	CauseSourceChange     InvalidationCause = "source_change"
	CauseConfigChange     InvalidationCause = "config_change"
	CauseDependencyChange InvalidationCause = "dependency_change"
	CauseExtractorChange  InvalidationCause = "extractor_change"
)

type InvalidationPlan struct {
	Scope    InvalidationScope `json:"scope"`
	Cause    InvalidationCause `json:"cause"`
	Added    []string          `json:"added,omitempty"`
	Modified []string          `json:"modified,omitempty"`
	Deleted  []string          `json:"deleted,omitempty"`
	Renamed  map[string]string `json:"renamed,omitempty"`
	FullScan bool              `json:"full_scan"`
}

type RefreshResult struct {
	ScanResult
	Invalidation InvalidationPlan `json:"invalidation"`
	Changed      bool             `json:"changed"`
}

func PlanInvalidation(previous, current ScanManifest, extractorChanged bool) InvalidationPlan {
	plan := InvalidationPlan{Scope: InvalidationFile, Cause: CauseSourceChange, Renamed: map[string]string{}}
	if extractorChanged {
		plan.Scope, plan.Cause, plan.FullScan = InvalidationRepository, CauseExtractorChange, true
		return plan
	}
	if previous.ConfigurationDigest != current.ConfigurationDigest {
		plan.Scope, plan.Cause, plan.FullScan = InvalidationRepository, CauseConfigChange, true
	}
	oldInputs := make(map[string]ScanInput, len(previous.Inputs))
	newInputs := make(map[string]ScanInput, len(current.Inputs))
	for _, input := range previous.Inputs {
		oldInputs[input.FilePath] = input
	}
	for _, input := range current.Inputs {
		newInputs[input.FilePath] = input
	}
	for path, old := range oldInputs {
		newInput, exists := newInputs[path]
		if !exists {
			plan.Deleted = append(plan.Deleted, path)
		} else if old.ContentHash != newInput.ContentHash || old.SymlinkTarget != newInput.SymlinkTarget {
			plan.Modified = append(plan.Modified, path)
		}
	}
	for path := range newInputs {
		if _, exists := oldInputs[path]; !exists {
			plan.Added = append(plan.Added, path)
		}
	}
	// Rename evidence is exact content equality. It is diagnostic and does not
	// force symbol continuity by itself.
	for _, deleted := range plan.Deleted {
		for _, added := range plan.Added {
			if oldInputs[deleted].ContentHash != "" && oldInputs[deleted].ContentHash == newInputs[added].ContentHash {
				plan.Renamed[deleted] = added
			}
		}
	}
	for _, paths := range [][]string{plan.Added, plan.Modified, plan.Deleted} {
		sort.Strings(paths)
	}
	if !plan.FullScan && len(plan.Added)+len(plan.Modified)+len(plan.Deleted) > 0 {
		plan.Scope = InvalidationReverseDependencies
	}
	return plan
}

// RefreshRepository reparses changed source files and carries forward unchanged
// extraction into a new atomic generation. Configuration and extractor changes
// deliberately use a full scan.
func (s *Store) RefreshRepository(ctx context.Context, root string) (RefreshResult, error) {
	identity, err := s.EnsureRepositoryIdentity(ctx, root, "")
	if err != nil {
		return RefreshResult{}, err
	}
	previousGeneration, err := s.activeGeneration(ctx, identity.WorktreeID)
	if err == sql.ErrNoRows || previousGeneration.ExtractorVersion == "legacy-pointer-v0" {
		result, scanErr := s.ScanRepository(ctx, root)
		return RefreshResult{ScanResult: result, Invalidation: InvalidationPlan{Scope: InvalidationRepository, Cause: CauseExtractorChange, FullScan: true}, Changed: scanErr == nil}, scanErr
	}
	if err != nil {
		return RefreshResult{}, err
	}
	previousManifest, err := s.loadGenerationManifest(ctx, previousGeneration.ID)
	if err != nil {
		return RefreshResult{}, err
	}
	currentManifest, err := BuildScanManifest(identity.RootPath)
	if err != nil {
		return RefreshResult{}, err
	}
	plan := PlanInvalidation(previousManifest, currentManifest, previousGeneration.ExtractorVersion != ExtractorVersion)
	if previousManifest.ManifestHash == currentManifest.ManifestHash && !plan.FullScan {
		files, symbols, edges, err := s.generationCounts(ctx, previousGeneration.ID)
		return RefreshResult{ScanResult: ScanResult{Generation: previousGeneration, Files: files, Symbols: symbols, Edges: edges, Limitations: previousGeneration.Limitations}, Invalidation: plan, Changed: false}, err
	}
	if plan.FullScan {
		result, scanErr := s.ScanRepository(ctx, root)
		return RefreshResult{ScanResult: result, Invalidation: plan, Changed: scanErr == nil}, scanErr
	}

	oldFiles, err := s.loadExtractedFiles(ctx, previousGeneration.ID)
	if err != nil {
		return RefreshResult{}, err
	}
	changed := make(map[string]bool)
	for _, path := range append(append(append([]string{}, plan.Added...), plan.Modified...), plan.Deleted...) {
		changed[path] = true
	}
	combined := make(map[string]ExtractedFile)
	for path, file := range oldFiles {
		if !changed[path] {
			combined[path] = file
		}
	}
	changedManifest := ScanManifest{}
	for _, input := range currentManifest.Inputs {
		if input.InputKind == "configuration" || changed[input.FilePath] {
			changedManifest.Inputs = append(changedManifest.Inputs, input)
		}
	}
	parsed, tsMethod, limitations, incomplete, err := extractManifestFiles(ctx, identity.RootPath, changedManifest)
	if err != nil {
		return RefreshResult{}, err
	}
	for _, file := range parsed {
		combined[file.Path] = file
	}
	files := make([]ExtractedFile, 0, len(combined))
	for _, file := range combined {
		files = append(files, file)
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	capabilities := previousGeneration.Capabilities
	if capabilities == nil {
		capabilities = map[string]string{}
	}
	if len(parsed) > 0 {
		capabilities["typescript.definitions"] = tsMethod
		capabilities["typescript.imports"] = tsMethod
		capabilities["typescript.exports"] = tsMethod
	}
	generation, err := s.BeginGeneration(ctx, identity, currentManifest, capabilities, limitations)
	if err != nil {
		return RefreshResult{}, err
	}
	fileCount, symbolCount, edgeCount, err := s.storeExtractedGraph(ctx, identity, generation, currentManifest, files)
	if err != nil {
		_ = s.FailGeneration(ctx, generation.ID, GraphFailed, err.Error())
		return RefreshResult{}, err
	}
	after, err := BuildScanManifest(identity.RootPath)
	if err != nil || after.ManifestHash != currentManifest.ManifestHash {
		message := "repository inputs changed while incremental graph generation was building"
		if err != nil {
			message = err.Error()
		}
		_ = s.FailGeneration(ctx, generation.ID, GraphInconsistent, message)
		return RefreshResult{}, fmt.Errorf("%s", message)
	}
	if incomplete {
		if err := s.completePartialGeneration(ctx, generation.ID, limitations); err != nil {
			return RefreshResult{}, err
		}
		generation.Status = GraphPartial
	} else {
		if err := s.PromoteGeneration(ctx, generation.ID); err != nil {
			return RefreshResult{}, err
		}
		generation.Status = GraphCurrent
	}
	return RefreshResult{ScanResult: ScanResult{Generation: generation, Files: fileCount, Symbols: symbolCount, Edges: edgeCount, Limitations: limitations}, Invalidation: plan, Changed: true}, nil
}

func (s *Store) loadGenerationManifest(ctx context.Context, generationID string) (ScanManifest, error) {
	var manifest ScanManifest
	if err := s.db.QueryRowContext(ctx, `SELECT manifest_hash, worktree_state_hash, configuration_digest FROM graph_generations WHERE id = ?`, generationID).Scan(&manifest.ManifestHash, &manifest.WorktreeStateHash, &manifest.ConfigurationDigest); err != nil {
		return ScanManifest{}, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT file_path, input_kind, size, content_hash, symlink_target, included FROM graph_scan_inputs WHERE generation_id = ? ORDER BY file_path`, generationID)
	if err != nil {
		return ScanManifest{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var input ScanInput
		var included int
		if err := rows.Scan(&input.FilePath, &input.InputKind, &input.Size, &input.ContentHash, &input.SymlinkTarget, &included); err != nil {
			return ScanManifest{}, err
		}
		input.Included = included != 0
		manifest.Inputs = append(manifest.Inputs, input)
	}
	return manifest, rows.Err()
}

func (s *Store) loadExtractedFiles(ctx context.Context, generationID string) (map[string]ExtractedFile, error) {
	result := make(map[string]ExtractedFile)
	rows, err := s.db.QueryContext(ctx, `SELECT qualified_name, language, metadata FROM graph_nodes WHERE generation_id = ? AND node_type = 'module' ORDER BY qualified_name`, generationID)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var path, language, metadata string
		if err := rows.Scan(&path, &language, &metadata); err != nil {
			rows.Close()
			return nil, err
		}
		values := map[string]string{}
		_ = json.Unmarshal([]byte(metadata), &values)
		packageName := values["package_name"]
		if packageName == "" {
			packageName = filepath.Base(filepath.Dir(path))
		}
		result[path] = ExtractedFile{Path: path, Language: language, PackageName: packageName}
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}

	symbolRows, err := s.db.QueryContext(ctx, `SELECT o.file_path, s.display_name, s.kind, s.qualified_name, o.signature, o.start_line, o.end_line, o.start_byte, o.end_byte, o.exported, o.resolution_status FROM symbol_occurrences o JOIN repository_symbols s ON s.id = o.symbol_id WHERE o.generation_id = ? ORDER BY o.file_path, o.start_byte`, generationID)
	if err != nil {
		return nil, err
	}
	for symbolRows.Next() {
		var path, qualified string
		var symbol ExtractedSymbol
		var exported int
		if err := symbolRows.Scan(&path, &symbol.Name, &symbol.Kind, &qualified, &symbol.Signature, &symbol.StartLine, &symbol.EndLine, &symbol.StartByte, &symbol.EndByte, &exported, &symbol.Resolution); err != nil {
			symbolRows.Close()
			return nil, err
		}
		symbol.Exported = exported != 0
		parts := strings.Split(qualified, ".")
		if len(parts) >= 2 && parts[len(parts)-1] == symbol.Name {
			// A parent exists only when the qualifier has an extra component beyond
			// the path-derived module qualifier.
			moduleParts := strings.Split(strings.TrimSuffix(path, filepath.Ext(path)), "/")
			if len(parts) > len(moduleParts)+1 {
				symbol.Parent = parts[len(parts)-2]
			}
		}
		file := result[path]
		file.Symbols = append(file.Symbols, symbol)
		result[path] = file
	}
	if err := symbolRows.Close(); err != nil {
		return nil, err
	}

	importRows, err := s.db.QueryContext(ctx, `SELECT n.qualified_name, e.source_start_byte, e.source_end_byte, e.resolution_status, e.metadata FROM graph_edges e JOIN graph_nodes n ON n.id = e.from_node_id WHERE e.generation_id = ? AND e.edge_type = 'imports' ORDER BY n.qualified_name, e.source_start_byte`, generationID)
	if err != nil {
		return nil, err
	}
	defer importRows.Close()
	for importRows.Next() {
		var path, resolution, metadata string
		var start, end int
		if err := importRows.Scan(&path, &start, &end, &resolution, &metadata); err != nil {
			return nil, err
		}
		var values map[string]string
		_ = json.Unmarshal([]byte(metadata), &values)
		target := values["specifier"]
		file := result[path]
		file.Imports = append(file.Imports, ExtractedImport{Target: target, StartByte: start, EndByte: end, Resolution: ResolutionStatus(resolution)})
		result[path] = file
	}
	if err := importRows.Err(); err != nil {
		return nil, err
	}
	referenceRows, err := s.db.QueryContext(ctx, `SELECT e.source_file_path, e.source_start_byte, e.source_end_byte, e.edge_type, e.resolution_status, e.metadata, COALESCE(o.file_path, '') FROM graph_edges e LEFT JOIN symbol_occurrences o ON o.node_id = e.to_node_id AND o.generation_id = e.generation_id WHERE e.generation_id = ? AND e.edge_type IN ('calls','references') ORDER BY e.source_file_path, e.source_start_byte`, generationID)
	if err != nil {
		return nil, err
	}
	defer referenceRows.Close()
	for referenceRows.Next() {
		var path, edgeType, resolution, metadata, targetPath string
		var start, end int
		if err := referenceRows.Scan(&path, &start, &end, &edgeType, &resolution, &metadata, &targetPath); err != nil {
			return nil, err
		}
		values := map[string]string{}
		_ = json.Unmarshal([]byte(metadata), &values)
		file := result[path]
		file.References = append(file.References, ExtractedReference{TargetName: values["target_name"], TargetPath: targetPath, StartByte: start, EndByte: end, EdgeType: edgeType, Resolution: ResolutionStatus(resolution)})
		result[path] = file
	}
	return result, referenceRows.Err()
}

func (s *Store) generationCounts(ctx context.Context, generationID string) (int, int, int, error) {
	var files, symbols, edges int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM graph_nodes WHERE generation_id = ? AND node_type = 'module'`, generationID).Scan(&files); err != nil {
		return 0, 0, 0, err
	}
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM symbol_occurrences WHERE generation_id = ?`, generationID).Scan(&symbols); err != nil {
		return 0, 0, 0, err
	}
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM graph_edges WHERE generation_id = ?`, generationID).Scan(&edges); err != nil {
		return 0, 0, 0, err
	}
	return files, symbols, edges, nil
}

func sourceInputPaths(inputs []ScanInput) map[string]bool {
	result := make(map[string]bool)
	for _, input := range inputs {
		if input.InputKind == "source" {
			result[input.FilePath] = true
		}
	}
	return result
}

func fileExists(root, rel string) bool {
	_, err := os.Stat(filepath.Join(root, filepath.FromSlash(rel)))
	return err == nil
}
