package knowledge

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"github.com/google/uuid"
)

var graphIgnoredDirectories = map[string]bool{
	".git": true, ".openexec": true, ".gocache": true, "node_modules": true,
	"vendor": true, "dist": true, "build": true, "coverage": true,
}

type ExtractedSymbol struct {
	Name       string
	Kind       string
	Parent     string
	Signature  string
	StartLine  int
	EndLine    int
	StartByte  int
	EndByte    int
	Exported   bool
	Resolution ResolutionStatus
}

type ExtractedImport struct {
	Target     string
	StartByte  int
	EndByte    int
	Resolution ResolutionStatus
}

type ExtractedReference struct {
	TargetName string
	TargetPath string
	StartByte  int
	EndByte    int
	EdgeType   string
	Resolution ResolutionStatus
}

type ExtractedFile struct {
	Path        string
	Language    string
	PackageName string
	Symbols     []ExtractedSymbol
	Imports     []ExtractedImport
	References  []ExtractedReference
	Limitations []string
}

type ScanResult struct {
	Generation  GraphGeneration `json:"generation"`
	Files       int             `json:"files"`
	Symbols     int             `json:"symbols"`
	Edges       int             `json:"edges"`
	Limitations []string        `json:"limitations,omitempty"`
}

// BuildScanManifest captures the exact source and configuration inputs used by
// a graph generation. It rejects symlinks that resolve outside the repository.
func BuildScanManifest(root string) (ScanManifest, error) {
	root, err := filepath.Abs(root)
	if err != nil {
		return ScanManifest{}, err
	}
	if evaluated, evalErr := filepath.EvalSymlinks(root); evalErr == nil {
		root = evaluated
	}
	var inputs []ScanInput
	// Prefer git's view of the repository: tracked plus unignored untracked
	// files. The hard-coded directory list alone treated gitignored trees
	// (dependency caches, build output) as project source - on one real
	// checkout 5,300 of 6,100 scanned files were an ignored GOPATH cache.
	if listed, ok := gitListedFiles(root); ok {
		for _, rel := range listed {
			ignored := false
			for _, part := range strings.Split(rel, "/") {
				if graphIgnoredDirectories[part] || (strings.HasPrefix(part, ".") && part != ".github") {
					ignored = true
					break
				}
			}
			if ignored {
				continue
			}
			kind, include := graphInputKind(rel)
			if !include {
				continue
			}
			path := filepath.Join(root, filepath.FromSlash(rel))
			info, statErr := os.Lstat(path)
			if statErr != nil {
				continue // tracked but deleted from the worktree
			}
			resolvedPath := path
			symlinkTarget := ""
			if info.Mode()&os.ModeSymlink != 0 {
				resolvedPath, err = filepath.EvalSymlinks(path)
				if err != nil {
					return ScanManifest{}, fmt.Errorf("resolve symlink %s: %w", rel, err)
				}
				if _, err := safeRepositoryRelative(root, resolvedPath); err != nil {
					return ScanManifest{}, fmt.Errorf("symlink %s escapes repository: %w", rel, err)
				}
				symlinkTarget, _ = filepath.Rel(root, resolvedPath)
				symlinkTarget = filepath.ToSlash(symlinkTarget)
				if info, err = os.Stat(resolvedPath); err != nil {
					return ScanManifest{}, err
				}
			}
			data, readErr := os.ReadFile(resolvedPath)
			if readErr != nil {
				return ScanManifest{}, fmt.Errorf("read scan input %s: %w", rel, readErr)
			}
			inputs = append(inputs, ScanInput{FilePath: filepath.ToSlash(rel), InputKind: kind, Size: info.Size(), ContentHash: hashBytes(data), SymlinkTarget: symlinkTarget, Included: true})
		}
		return finishScanManifest(inputs), nil
	}
	err = filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == root {
			return nil
		}
		if entry.IsDir() {
			if graphIgnoredDirectories[entry.Name()] || (strings.HasPrefix(entry.Name(), ".") && entry.Name() != ".github") {
				return filepath.SkipDir
			}
			return nil
		}
		rel, err := safeRepositoryRelative(root, path)
		if err != nil {
			return err
		}
		kind, include := graphInputKind(rel)
		if !include {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		resolvedPath := path
		symlinkTarget := ""
		if info.Mode()&os.ModeSymlink != 0 {
			resolvedPath, err = filepath.EvalSymlinks(path)
			if err != nil {
				return fmt.Errorf("resolve symlink %s: %w", rel, err)
			}
			if _, err := safeRepositoryRelative(root, resolvedPath); err != nil {
				return fmt.Errorf("symlink %s escapes repository: %w", rel, err)
			}
			symlinkTarget, _ = filepath.Rel(root, resolvedPath)
			symlinkTarget = filepath.ToSlash(symlinkTarget)
			info, err = os.Stat(resolvedPath)
			if err != nil {
				return err
			}
		}
		data, err := os.ReadFile(resolvedPath)
		if err != nil {
			return fmt.Errorf("read scan input %s: %w", rel, err)
		}
		inputs = append(inputs, ScanInput{FilePath: rel, InputKind: kind, Size: info.Size(), ContentHash: hashBytes(data), SymlinkTarget: symlinkTarget, Included: true})
		return nil
	})
	if err != nil {
		return ScanManifest{}, err
	}
	return finishScanManifest(inputs), nil
}

// gitListedFiles asks git for tracked and unignored untracked files, so the
// manifest honors .gitignore. ok=false falls back to the directory walk.
func gitListedFiles(root string) ([]string, bool) {
	command := exec.Command("git", "-C", root, "ls-files", "--cached", "--others", "--exclude-standard", "-z")
	output, err := command.Output()
	if err != nil {
		return nil, false
	}
	var files []string
	for _, rel := range strings.Split(string(output), "\x00") {
		if rel != "" {
			files = append(files, filepath.ToSlash(rel))
		}
	}
	return files, len(files) > 0
}

func finishScanManifest(inputs []ScanInput) ScanManifest {
	sort.Slice(inputs, func(i, j int) bool { return inputs[i].FilePath < inputs[j].FilePath })
	manifestHasher := sha256.New()
	configHasher := sha256.New()
	for _, input := range inputs {
		line := strings.Join([]string{input.FilePath, input.InputKind, strconv.FormatInt(input.Size, 10), input.ContentHash, input.SymlinkTarget}, "\x00")
		_, _ = manifestHasher.Write([]byte(line))
		_, _ = manifestHasher.Write([]byte{'\n'})
		if input.InputKind == "configuration" {
			_, _ = configHasher.Write([]byte(line))
			_, _ = configHasher.Write([]byte{'\n'})
		}
	}
	manifestHash := "sha256:" + hex.EncodeToString(manifestHasher.Sum(nil))
	configurationDigest := "sha256:" + hex.EncodeToString(configHasher.Sum(nil))
	return ScanManifest{Inputs: inputs, ManifestHash: manifestHash, WorktreeStateHash: manifestHash, ConfigurationDigest: configurationDigest}
}

func graphInputKind(rel string) (string, bool) {
	base := filepath.Base(rel)
	ext := strings.ToLower(filepath.Ext(base))
	switch ext {
	case ".go", ".ts", ".tsx", ".js", ".jsx":
		return "source", true
	}
	if base == "go.mod" || base == "go.sum" || base == "go.work" || base == "go.work.sum" || base == "package.json" || base == "package-lock.json" || base == "pnpm-lock.yaml" || base == "yarn.lock" || (strings.HasPrefix(base, "tsconfig") && strings.HasSuffix(base, ".json")) {
		return "configuration", true
	}
	return "", false
}

func safeRepositoryRelative(root, path string) (string, error) {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return "", err
	}
	if rel == ".." || filepath.IsAbs(rel) || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path %q is outside repository root", path)
	}
	return filepath.ToSlash(filepath.Clean(rel)), nil
}

// ScanRepository builds, revalidates, and atomically promotes a full graph.
// Incremental refresh uses this result as its correctness oracle.
func (s *Store) ScanRepository(ctx context.Context, root string) (ScanResult, error) {
	return s.scanRepository(ctx, root, nil)
}

func (s *Store) scanRepository(ctx context.Context, root string, beforeRevalidate func()) (ScanResult, error) {
	identity, err := s.EnsureRepositoryIdentity(ctx, root, "")
	if err != nil {
		return ScanResult{}, err
	}
	_, _ = s.MigrateLegacySymbols(ctx, identity)
	manifest, err := BuildScanManifest(identity.RootPath)
	if err != nil {
		return ScanResult{}, fmt.Errorf("build scan manifest: %w", err)
	}
	capabilities := map[string]string{
		"go.definitions": "ast_exact", "go.imports": "ast_exact",
		"typescript.definitions": "pending", "typescript.imports": "pending", "typescript.exports": "pending",
	}
	var limitations []string
	// A cancelled or crashed scan must not leave generations parked in
	// "building" forever - they read as running work that never existed.
	if _, err := s.db.ExecContext(ctx, `UPDATE graph_generations SET status = 'failed', error_message = 'abandoned: superseded by a newer scan', completed_at = CURRENT_TIMESTAMP WHERE repository_id = ? AND status = 'building'`, identity.RepositoryID); err != nil {
		return ScanResult{}, err
	}
	generation, err := s.BeginGeneration(ctx, identity, manifest, capabilities, limitations)
	if err != nil {
		return ScanResult{}, err
	}

	files, tsMethod, extractionLimitations, incomplete, err := extractManifestFiles(ctx, identity.RootPath, manifest)
	if err != nil {
		_ = s.FailGeneration(ctx, generation.ID, GraphFailed, err.Error())
		return ScanResult{}, err
	}
	capabilities["typescript.definitions"] = tsMethod
	capabilities["typescript.imports"] = tsMethod
	capabilities["typescript.exports"] = tsMethod
	generation.Capabilities = capabilities
	generation.Limitations = append(generation.Limitations, extractionLimitations...)
	if err := s.updateGenerationMetadata(ctx, generation.ID, capabilities, generation.Limitations); err != nil {
		_ = s.FailGeneration(ctx, generation.ID, GraphFailed, err.Error())
		return ScanResult{}, err
	}
	filesCount, symbolsCount, edgesCount, err := s.storeExtractedGraph(ctx, identity, generation, manifest, files)
	if err != nil {
		_ = s.FailGeneration(ctx, generation.ID, GraphFailed, err.Error())
		return ScanResult{}, err
	}

	if beforeRevalidate != nil {
		beforeRevalidate()
	}
	after, err := BuildScanManifest(identity.RootPath)
	if err != nil {
		_ = s.FailGeneration(ctx, generation.ID, GraphInconsistent, err.Error())
		return ScanResult{}, fmt.Errorf("revalidate scan manifest: %w", err)
	}
	if manifest.ManifestHash != after.ManifestHash || manifest.ConfigurationDigest != after.ConfigurationDigest {
		message := "repository inputs changed while graph generation was building"
		_ = s.FailGeneration(ctx, generation.ID, GraphInconsistent, message)
		return ScanResult{}, fmt.Errorf("%s", message)
	}
	if incomplete {
		if err := s.completePartialGeneration(ctx, generation.ID, generation.Limitations); err != nil {
			return ScanResult{}, err
		}
		generation.Status = GraphPartial
	} else {
		if err := s.PromoteGeneration(ctx, generation.ID); err != nil {
			return ScanResult{}, err
		}
		generation.Status = GraphCurrent
	}
	return ScanResult{Generation: generation, Files: filesCount, Symbols: symbolsCount, Edges: edgesCount, Limitations: generation.Limitations}, nil
}

func (s *Store) updateGenerationMetadata(ctx context.Context, generationID string, capabilities map[string]string, limitations []string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE graph_generations SET capabilities = ?, limitations = ? WHERE id = ? AND status = 'building'`, jsonText(capabilities, "{}"), jsonText(limitations, "[]"), generationID)
	return err
}

func (s *Store) completePartialGeneration(ctx context.Context, generationID string, limitations []string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE graph_generations SET status = 'partial', limitations = ?, completed_at = CURRENT_TIMESTAMP WHERE id = ? AND status = 'building'`, jsonText(limitations, "[]"), generationID)
	return err
}

func extractManifestFiles(ctx context.Context, root string, manifest ScanManifest) ([]ExtractedFile, string, []string, bool, error) {
	var files []ExtractedFile
	var limitations []string
	var tsFiles []string
	for _, input := range manifest.Inputs {
		if input.InputKind == "source" {
			switch strings.ToLower(filepath.Ext(input.FilePath)) {
			case ".ts", ".tsx", ".js", ".jsx":
				tsFiles = append(tsFiles, input.FilePath)
			}
		}
	}
	tsExtracted, _, tsLimitations, compilerErr := extractTypeScriptWithCompiler(ctx, root, tsFiles)
	tsMethod := string(ResolutionCompilerExact)
	if compilerErr != nil {
		tsMethod = string(ResolutionStaticLexical)
		limitations = append(limitations, "TypeScript compiler extraction unavailable; using lexical definitions/imports: "+compilerErr.Error())
	} else {
		limitations = append(limitations, tsLimitations...)
	}
	incomplete := false
	for _, input := range manifest.Inputs {
		if input.InputKind != "source" {
			continue
		}
		path := filepath.Join(root, filepath.FromSlash(input.FilePath))
		var extracted ExtractedFile
		var err error
		switch strings.ToLower(filepath.Ext(input.FilePath)) {
		case ".go":
			extracted, err = extractGoFile(path, input.FilePath)
		case ".ts", ".tsx", ".js", ".jsx":
			if compilerErr == nil {
				var ok bool
				extracted, ok = tsExtracted[input.FilePath]
				if !ok {
					err = fmt.Errorf("compiler omitted requested source file")
				}
			} else {
				extracted, err = extractTypeScriptLexical(path, input.FilePath)
			}
		}
		if err != nil {
			limitations = append(limitations, fmt.Sprintf("%s: %v", input.FilePath, err))
			incomplete = true
			continue
		}
		files = append(files, extracted)
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	return files, tsMethod, limitations, incomplete, nil
}

func extractGoFile(path, rel string) (ExtractedFile, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
	if err != nil {
		return ExtractedFile{}, err
	}
	result := ExtractedFile{Path: rel, Language: "go", PackageName: file.Name.Name}
	for _, imp := range file.Imports {
		target, _ := strconv.Unquote(imp.Path.Value)
		result.Imports = append(result.Imports, ExtractedImport{Target: target, StartByte: fset.Position(imp.Pos()).Offset, EndByte: fset.Position(imp.End()).Offset, Resolution: ResolutionASTExact})
	}
	for _, decl := range file.Decls {
		switch value := decl.(type) {
		case *ast.FuncDecl:
			kind, parent := "function", ""
			if value.Recv != nil {
				kind, parent = "method", strings.TrimPrefix(extractReceiverType(value.Recv), "*")
			}
			result.Symbols = append(result.Symbols, ExtractedSymbol{Name: value.Name.Name, Kind: kind, Parent: parent, Signature: formatGoSignature(value), StartLine: fset.Position(value.Pos()).Line, EndLine: fset.Position(value.End()).Line, StartByte: fset.Position(value.Pos()).Offset, EndByte: fset.Position(value.End()).Offset, Exported: ast.IsExported(value.Name.Name), Resolution: ResolutionASTExact})
		case *ast.GenDecl:
			for _, spec := range value.Specs {
				switch typed := spec.(type) {
				case *ast.TypeSpec:
					kind := "type"
					switch typed.Type.(type) {
					case *ast.StructType:
						kind = "struct"
					case *ast.InterfaceType:
						kind = "interface"
					}
					result.Symbols = append(result.Symbols, ExtractedSymbol{Name: typed.Name.Name, Kind: kind, Signature: "type " + typed.Name.Name, StartLine: fset.Position(typed.Pos()).Line, EndLine: fset.Position(typed.End()).Line, StartByte: fset.Position(typed.Pos()).Offset, EndByte: fset.Position(typed.End()).Offset, Exported: ast.IsExported(typed.Name.Name), Resolution: ResolutionASTExact})
				case *ast.ValueSpec:
					for _, name := range typed.Names {
						kind := "variable"
						if value.Tok == token.CONST {
							kind = "constant"
						}
						result.Symbols = append(result.Symbols, ExtractedSymbol{Name: name.Name, Kind: kind, Signature: value.Tok.String() + " " + name.Name, StartLine: fset.Position(typed.Pos()).Line, EndLine: fset.Position(typed.End()).Line, StartByte: fset.Position(typed.Pos()).Offset, EndByte: fset.Position(typed.End()).Offset, Exported: ast.IsExported(name.Name), Resolution: ResolutionASTExact})
					}
				}
			}
		}
	}
	ast.Inspect(file, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		name := ""
		switch function := call.Fun.(type) {
		case *ast.Ident:
			name = function.Name
		case *ast.SelectorExpr:
			name = function.Sel.Name
		}
		if name != "" && !isLanguageCallKeyword(name) {
			result.References = append(result.References, ExtractedReference{TargetName: name, StartByte: fset.Position(call.Fun.Pos()).Offset, EndByte: fset.Position(call.Fun.End()).Offset, EdgeType: "calls", Resolution: ResolutionHeuristic})
		}
		return true
	})
	return result, nil
}

var (
	tsImportPattern      = regexp.MustCompile(`(?m)^\s*(?:import|export)\s+(?:[^"']*?\s+from\s+)?["']([^"']+)["']`)
	tsDeclarationPattern = regexp.MustCompile(`(?m)^\s*(export\s+)?(?:default\s+)?(?:abstract\s+)?(async\s+)?(function|class|interface|type|const|let|var)\s+([A-Za-z_$][\w$]*)`)
	tsArrowPattern       = regexp.MustCompile(`(?m)^\s*(export\s+)?(?:default\s+)?const\s+([A-Za-z_$][\w$]*)\s*(?::[^=]+)?=\s*(?:async\s+)?(?:\([^\n]*\)|[A-Za-z_$][\w$]*)\s*=>`)
)

func extractTypeScriptLexical(path, rel string) (ExtractedFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return ExtractedFile{}, err
	}
	result := ExtractedFile{Path: rel, Language: "typescript", PackageName: filepath.Base(filepath.Dir(rel)), Limitations: []string{"symbol extents and relationships use lexical extraction"}}
	for _, match := range tsImportPattern.FindAllSubmatchIndex(data, -1) {
		if len(match) < 4 {
			continue
		}
		result.Imports = append(result.Imports, ExtractedImport{Target: string(data[match[2]:match[3]]), StartByte: match[0], EndByte: match[1], Resolution: ResolutionStaticLexical})
	}
	seen := make(map[string]bool)
	for _, match := range tsDeclarationPattern.FindAllSubmatchIndex(data, -1) {
		kind, name := string(data[match[6]:match[7]]), string(data[match[8]:match[9]])
		kind = mapTSKind(kind)
		if kind == "constant" {
			lineEnd := match[1]
			if relative := strings.IndexByte(string(data[match[1]:]), '\n'); relative >= 0 {
				lineEnd = match[1] + relative
			} else {
				lineEnd = len(data)
			}
			if tsArrowPattern.Match(data[match[0]:lineEnd]) {
				continue
			}
		}
		key := kind + "\x00" + name + "\x00" + strconv.Itoa(match[0])
		seen[key] = true
		startLine, endLine := lineNumberAt(data, match[0]), lineNumberAt(data, match[1])
		exported := match[2] >= 0
		result.Symbols = append(result.Symbols, ExtractedSymbol{Name: name, Kind: kind, Signature: strings.TrimSpace(string(data[match[0]:match[1]])), StartLine: startLine, EndLine: endLine, StartByte: match[0], EndByte: match[1], Exported: exported, Resolution: ResolutionStaticLexical})
	}
	for _, match := range tsArrowPattern.FindAllSubmatchIndex(data, -1) {
		name := string(data[match[4]:match[5]])
		key := "function\x00" + name + "\x00" + strconv.Itoa(match[0])
		if seen[key] {
			continue
		}
		result.Symbols = append(result.Symbols, ExtractedSymbol{Name: name, Kind: "function", Signature: strings.TrimSpace(string(data[match[0]:match[1]])), StartLine: lineNumberAt(data, match[0]), EndLine: lineNumberAt(data, match[1]), StartByte: match[0], EndByte: match[1], Exported: match[2] >= 0, Resolution: ResolutionStaticLexical})
	}
	callPattern := regexp.MustCompile(`\b([A-Za-z_$][\w$]*)\s*\(`)
	for _, match := range callPattern.FindAllSubmatchIndex(data, -1) {
		name := string(data[match[2]:match[3]])
		if isLanguageCallKeyword(name) {
			continue
		}
		result.References = append(result.References, ExtractedReference{TargetName: name, StartByte: match[2], EndByte: match[3], EdgeType: "calls", Resolution: ResolutionHeuristic})
	}
	sort.Slice(result.Symbols, func(i, j int) bool { return result.Symbols[i].StartByte < result.Symbols[j].StartByte })
	return result, nil
}

func isLanguageCallKeyword(name string) bool {
	switch name {
	case "if", "for", "while", "switch", "catch", "function", "func", "return", "new", "make", "delete":
		return true
	default:
		return false
	}
}

func mapTSKind(kind string) string {
	switch kind {
	case "const":
		return "constant"
	case "let", "var":
		return "variable"
	default:
		return kind
	}
}

func lineNumberAt(data []byte, offset int) int {
	if offset > len(data) {
		offset = len(data)
	}
	return 1 + strings.Count(string(data[:offset]), "\n")
}

func (s *Store) storeExtractedGraph(ctx context.Context, identity RepositoryIdentity, generation GraphGeneration, manifest ScanManifest, files []ExtractedFile) (int, int, int, error) {
	inputByPath := make(map[string]ScanInput)
	for _, input := range manifest.Inputs {
		inputByPath[input.FilePath] = input
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, 0, 0, err
	}
	defer func() { _ = tx.Rollback() }()
	repositoryNode := stableID("node", generation.ID, "repository", identity.RepositoryID)
	if _, err := tx.ExecContext(ctx, `INSERT INTO graph_nodes (id, generation_id, repository_id, node_type, display_name, qualified_name) VALUES (?, ?, ?, 'repository', ?, ?)`, repositoryNode, generation.ID, identity.RepositoryID, filepath.Base(identity.RootPath), identity.RepositoryID); err != nil {
		return 0, 0, 0, err
	}
	packageNodes := make(map[string]string)
	moduleNodes := make(map[string]string)
	filePackageKeys := make(map[string]string)
	for _, file := range files {
		packageKey := packageKeyForFile(file)
		filePackageKeys[file.Path] = packageKey
		packageNode := packageNodes[packageKey]
		if packageNode == "" {
			packageNode = stableID("node", generation.ID, "package", packageKey)
			packageNodes[packageKey] = packageNode
			if _, err := tx.ExecContext(ctx, `INSERT INTO graph_nodes (id, generation_id, repository_id, node_type, language, display_name, qualified_name) VALUES (?, ?, ?, 'package', ?, ?, ?)`, packageNode, generation.ID, identity.RepositoryID, file.Language, packageKey, packageKey); err != nil {
				return 0, 0, 0, err
			}
			if err := insertEdge(ctx, tx, generation.ID, repositoryNode, packageNode, "contains", ResolutionConfigurationDerived, "", 0, 0, nil); err != nil {
				return 0, 0, 0, err
			}
		}
		moduleNode := stableID("node", generation.ID, "module", file.Path)
		moduleNodes[file.Path] = moduleNode
		if _, err := tx.ExecContext(ctx, `INSERT INTO graph_nodes (id, generation_id, repository_id, node_type, language, display_name, qualified_name, metadata) VALUES (?, ?, ?, 'module', ?, ?, ?, ?)`, moduleNode, generation.ID, identity.RepositoryID, file.Language, filepath.Base(file.Path), file.Path, jsonText(map[string]string{"package_name": file.PackageName}, "{}")); err != nil {
			return 0, 0, 0, err
		}
		if err := insertEdge(ctx, tx, generation.ID, packageNode, moduleNode, "contains", ResolutionConfigurationDerived, file.Path, 0, 0, nil); err != nil {
			return 0, 0, 0, err
		}
	}

	prior, err := loadPriorSymbols(ctx, tx, identity.RepositoryID, generation.ID)
	if err != nil {
		return 0, 0, 0, err
	}
	type storedSymbol struct {
		nodeID string
		file   string
		name   string
		start  int
		end    int
	}
	symbolsByName := make(map[string][]storedSymbol)
	symbolsByFileName := make(map[string][]storedSymbol)
	symbolsByFile := make(map[string][]storedSymbol)
	symbolCount, edgeCount := 0, len(packageNodes)+len(files)
	for _, file := range files {
		moduleNode := moduleNodes[file.Path]
		input := inputByPath[file.Path]
		data, err := os.ReadFile(filepath.Join(identity.RootPath, filepath.FromSlash(file.Path)))
		if err != nil {
			return 0, 0, 0, err
		}
		for _, extracted := range file.Symbols {
			if extracted.StartByte < 0 || extracted.EndByte < extracted.StartByte || extracted.EndByte > len(data) {
				return 0, 0, 0, fmt.Errorf("invalid source range for %s in %s", extracted.Name, file.Path)
			}
			qualified := qualifySymbol(file, extracted)
			match, err := findOrCreateSymbol(ctx, tx, generation.ID, identity.RepositoryID, file.Language, extracted.Kind, extracted.Name, qualified, file.Path, extracted.Signature, prior)
			if err != nil {
				return 0, 0, 0, err
			}
			symbolID := match.SymbolID
			nodeID := stableID("node", generation.ID, "symbol", symbolID)
			if _, err := tx.ExecContext(ctx, `INSERT INTO graph_nodes (id, generation_id, repository_id, node_type, language, display_name, qualified_name) VALUES (?, ?, ?, 'symbol', ?, ?, ?)`, nodeID, generation.ID, identity.RepositoryID, file.Language, extracted.Name, qualified); err != nil {
				return 0, 0, 0, err
			}
			if _, err := tx.ExecContext(ctx, `INSERT INTO symbol_occurrences (symbol_id, generation_id, node_id, file_path, start_line, end_line, start_byte, end_byte, signature, file_content_hash, source_range_hash, exported, resolution_status) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, symbolID, generation.ID, nodeID, file.Path, extracted.StartLine, extracted.EndLine, extracted.StartByte, extracted.EndByte, extracted.Signature, input.ContentHash, hashBytes(data[extracted.StartByte:extracted.EndByte]), boolInt(extracted.Exported), extracted.Resolution); err != nil {
				return 0, 0, 0, err
			}
			previous := match.PreviousSymbolIDs
			if len(previous) == 0 {
				previous = []string{""}
			}
			for _, previousID := range previous {
				if _, err := tx.ExecContext(ctx, `INSERT INTO symbol_lineage (id, repository_id, symbol_id, previous_symbol_id, continuity_status, resolution_method, generation_id) VALUES (?, ?, ?, NULLIF(?, ''), ?, ?, ?)`, stableID("lineage", generation.ID, symbolID, previousID), identity.RepositoryID, symbolID, previousID, match.Continuity, match.Method, generation.ID); err != nil {
					return 0, 0, 0, err
				}
			}
			if err := insertEdge(ctx, tx, generation.ID, moduleNode, nodeID, "contains", extracted.Resolution, file.Path, extracted.StartByte, extracted.EndByte, nil); err != nil {
				return 0, 0, 0, err
			}
			edgeCount++
			if extracted.Exported {
				if err := insertEdge(ctx, tx, generation.ID, moduleNode, nodeID, "exports", extracted.Resolution, file.Path, extracted.StartByte, extracted.EndByte, nil); err != nil {
					return 0, 0, 0, err
				}
				edgeCount++
			}
			stored := storedSymbol{nodeID: nodeID, file: file.Path, name: extracted.Name, start: extracted.StartByte, end: extracted.EndByte}
			symbolsByName[extracted.Name] = append(symbolsByName[extracted.Name], stored)
			symbolsByFileName[file.Path+"\x00"+extracted.Name] = append(symbolsByFileName[file.Path+"\x00"+extracted.Name], stored)
			symbolsByFile[file.Path] = append(symbolsByFile[file.Path], stored)
			symbolCount++
		}
	}

	externalNodes := make(map[string]string)
	for _, file := range files {
		from := moduleNodes[file.Path]
		for _, imp := range file.Imports {
			to, resolution := resolveImportNode(generation.ID, file, imp.Target, moduleNodes)
			if to == "" {
				to = externalNodes[imp.Target]
				if to == "" {
					to = stableID("node", generation.ID, "external_package", imp.Target)
					externalNodes[imp.Target] = to
					if _, err := tx.ExecContext(ctx, `INSERT INTO graph_nodes (id, generation_id, repository_id, node_type, display_name, qualified_name) VALUES (?, ?, ?, 'external_package', ?, ?)`, to, generation.ID, identity.RepositoryID, imp.Target, imp.Target); err != nil {
						return 0, 0, 0, err
					}
				}
				resolution = ResolutionUnresolved
			}
			if err := insertEdge(ctx, tx, generation.ID, from, to, "imports", resolution, file.Path, imp.StartByte, imp.EndByte, map[string]string{"specifier": imp.Target}); err != nil {
				return 0, 0, 0, err
			}
			edgeCount++
			if isTestFile(file.Path) && moduleNodesByID(moduleNodes, to) {
				if err := insertEdge(ctx, tx, generation.ID, to, from, "tested_by", resolution, file.Path, imp.StartByte, imp.EndByte, map[string]string{"reason": "test module imports module", "test_file": file.Path}); err != nil {
					return 0, 0, 0, err
				}
				edgeCount++
			}
		}
	}
	for _, testFile := range files {
		if !isTestFile(testFile.Path) {
			continue
		}
		testNode := moduleNodes[testFile.Path]
		for _, productionFile := range files {
			if productionFile.Path == testFile.Path || isTestFile(productionFile.Path) || filePackageKeys[productionFile.Path] != filePackageKeys[testFile.Path] {
				continue
			}
			if err := insertEdge(ctx, tx, generation.ID, moduleNodes[productionFile.Path], testNode, "tested_by", ResolutionConfigurationDerived, testFile.Path, 0, 0, map[string]string{"reason": "test module shares package", "test_file": testFile.Path}); err != nil {
				return 0, 0, 0, err
			}
			edgeCount++
		}
	}
	for _, file := range files {
		for _, reference := range file.References {
			var candidates []storedSymbol
			if reference.TargetPath != "" {
				candidates = symbolsByFileName[filepath.ToSlash(reference.TargetPath)+"\x00"+reference.TargetName]
			}
			if len(candidates) == 0 {
				candidates = symbolsByName[reference.TargetName]
			}
			if len(candidates) != 1 {
				continue
			}
			from := moduleNodes[file.Path]
			bestWidth := int(^uint(0) >> 1)
			for _, origin := range symbolsByFile[file.Path] {
				if origin.start <= reference.StartByte && reference.StartByte < origin.end && origin.end-origin.start < bestWidth {
					from = origin.nodeID
					bestWidth = origin.end - origin.start
				}
			}
			edgeType := reference.EdgeType
			if edgeType == "" {
				edgeType = "references"
			}
			if err := insertEdge(ctx, tx, generation.ID, from, candidates[0].nodeID, edgeType, reference.Resolution, file.Path, reference.StartByte, reference.EndByte, map[string]string{"target_name": reference.TargetName}); err != nil {
				return 0, 0, 0, err
			}
			edgeCount++
		}
	}
	if err := tx.Commit(); err != nil {
		return 0, 0, 0, err
	}
	return len(files), symbolCount, edgeCount, nil
}

func moduleNodesByID(nodes map[string]string, id string) bool {
	for _, nodeID := range nodes {
		if nodeID == id {
			return true
		}
	}
	return false
}

func isTestFile(path string) bool {
	base := filepath.Base(path)
	return strings.HasSuffix(base, "_test.go") || strings.Contains(base, ".test.") || strings.Contains(base, ".spec.") || strings.Contains(filepath.ToSlash(path), "/__tests__/") || strings.HasPrefix(filepath.ToSlash(path), "tests/")
}

func packageKeyForFile(file ExtractedFile) string {
	dir := filepath.ToSlash(filepath.Dir(file.Path))
	if file.Language == "go" && file.PackageName != "" {
		return dir + ":" + file.PackageName
	}
	if dir == "." {
		return "root"
	}
	return dir
}

func qualifySymbol(file ExtractedFile, symbol ExtractedSymbol) string {
	parts := []string{strings.TrimSuffix(file.Path, filepath.Ext(file.Path))}
	if symbol.Parent != "" {
		parts = append(parts, symbol.Parent)
	}
	parts = append(parts, symbol.Name)
	return strings.Join(parts, ".")
}

type symbolIdentityMatch struct {
	SymbolID          string
	Continuity        string
	Method            string
	PreviousSymbolIDs []string
}

// priorSymbols is the exact-identity read model for one scan: every live
// symbol keyed by its prior location, loaded in one query instead of one
// query per symbol. Claimed tracks symbols already re-used this generation
// so a duplicate key falls back to the full SQL path, which preserves the
// original NOT EXISTS semantics.
type priorSymbols struct {
	exact   map[string]string
	claimed map[string]bool
}

func priorKey(language, kind, qualified, file string) string {
	return language + "\x00" + kind + "\x00" + qualified + "\x00" + file
}

// loadPriorSymbols builds the exact-match map with the same candidate
// preference as the per-symbol query: current generation first, then newest.
func loadPriorSymbols(ctx context.Context, tx *sql.Tx, repositoryID, generationID string) (*priorSymbols, error) {
	rows, err := tx.QueryContext(ctx, `SELECT s.id, s.language, s.kind, s.qualified_name, o.file_path FROM repository_symbols s JOIN symbol_occurrences o ON o.symbol_id = s.id JOIN graph_generations g ON g.id = o.generation_id WHERE s.repository_id = ? AND s.retired_at IS NULL AND o.generation_id <> ? ORDER BY CASE g.status WHEN 'current' THEN 0 ELSE 1 END, g.created_at DESC`, repositoryID, generationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	prior := &priorSymbols{exact: map[string]string{}, claimed: map[string]bool{}}
	for rows.Next() {
		var id, language, kind, qualified, file string
		if err := rows.Scan(&id, &language, &kind, &qualified, &file); err != nil {
			return nil, err
		}
		key := priorKey(language, kind, qualified, file)
		if _, exists := prior.exact[key]; !exists {
			prior.exact[key] = id
		}
	}
	if len(prior.exact) == 0 {
		return nil, rows.Err() // no history: callers take the fast create path
	}
	return prior, rows.Err()
}

func findOrCreateSymbol(ctx context.Context, tx *sql.Tx, generationID, repositoryID, language, kind, name, qualified, file, signature string, prior *priorSymbols) (symbolIdentityMatch, error) {
	if prior != nil {
		if id := prior.exact[priorKey(language, kind, qualified, file)]; id != "" && !prior.claimed[id] {
			prior.claimed[id] = true
			return symbolIdentityMatch{SymbolID: id, Continuity: "preserved", Method: "exact"}, nil
		}
	}
	if prior == nil {
		newID := "sym_" + uuid.NewString()
		if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO repository_symbols (id, repository_id, language, kind, display_name, qualified_name) VALUES (?, ?, ?, ?, ?, ?)`, newID, repositoryID, language, kind, name, qualified); err != nil {
			return symbolIdentityMatch{}, err
		}
		return symbolIdentityMatch{SymbolID: newID, Continuity: "new", Method: "structural"}, nil
	}
	var symbolID string
	err := tx.QueryRowContext(ctx, `SELECT s.id FROM repository_symbols s JOIN symbol_occurrences o ON o.symbol_id = s.id JOIN graph_generations g ON g.id = o.generation_id WHERE s.repository_id = ? AND s.retired_at IS NULL AND s.language = ? AND s.kind = ? AND s.qualified_name = ? AND o.file_path = ? AND o.generation_id <> ? AND NOT EXISTS (SELECT 1 FROM symbol_occurrences current WHERE current.generation_id = ? AND current.symbol_id = s.id) ORDER BY CASE g.status WHEN 'current' THEN 0 ELSE 1 END, g.created_at DESC LIMIT 1`, repositoryID, language, kind, qualified, file, generationID, generationID).Scan(&symbolID)
	if err == nil {
		return symbolIdentityMatch{SymbolID: symbolID, Continuity: "preserved", Method: "exact"}, nil
	}
	if err != sql.ErrNoRows {
		return symbolIdentityMatch{}, err
	}
	structure := normalizedSymbolStructure(name, signature)
	var candidates []struct{ id, display, file, signature string }
	if structure != "" {
		rows, queryErr := tx.QueryContext(ctx, `SELECT s.id, s.display_name, o.file_path, o.signature FROM repository_symbols s JOIN symbol_occurrences o ON o.symbol_id = s.id JOIN graph_generations g ON g.id = o.generation_id WHERE s.repository_id = ? AND s.retired_at IS NULL AND s.language = ? AND s.kind = ? AND o.generation_id <> ? AND g.status IN ('current','superseded','partial') AND NOT EXISTS (SELECT 1 FROM symbol_occurrences current WHERE current.generation_id = ? AND current.symbol_id = s.id) ORDER BY CASE g.status WHEN 'current' THEN 0 ELSE 1 END, g.created_at DESC`, repositoryID, language, kind, generationID, generationID)
		if queryErr != nil {
			return symbolIdentityMatch{}, queryErr
		}
		seen := map[string]bool{}
		for rows.Next() {
			var candidate struct{ id, display, file, signature string }
			if scanErr := rows.Scan(&candidate.id, &candidate.display, &candidate.file, &candidate.signature); scanErr != nil {
				rows.Close()
				return symbolIdentityMatch{}, scanErr
			}
			if !seen[candidate.id] && normalizedSymbolStructure(candidate.display, candidate.signature) == structure {
				seen[candidate.id] = true
				candidates = append(candidates, candidate)
			}
		}
		if rowsErr := rows.Close(); rowsErr != nil {
			return symbolIdentityMatch{}, rowsErr
		}
	}
	if len(candidates) == 1 {
		candidate := candidates[0]
		continuity := "moved"
		if candidate.display != name {
			continuity = "renamed"
		}
		if _, err := tx.ExecContext(ctx, `UPDATE repository_symbols SET display_name = ?, qualified_name = ?, retired_at = NULL WHERE id = ?`, name, qualified, candidate.id); err != nil {
			return symbolIdentityMatch{}, err
		}
		return symbolIdentityMatch{SymbolID: candidate.id, Continuity: continuity, Method: "structural", PreviousSymbolIDs: []string{candidate.id}}, nil
	}
	symbolID = "sym_" + uuid.NewString()
	_, err = tx.ExecContext(ctx, `INSERT OR IGNORE INTO repository_symbols (id, repository_id, language, kind, display_name, qualified_name) VALUES (?, ?, ?, ?, ?, ?)`, symbolID, repositoryID, language, kind, name, qualified)
	if err != nil {
		return symbolIdentityMatch{}, err
	}
	match := symbolIdentityMatch{SymbolID: symbolID, Continuity: "new", Method: "structural"}
	if len(candidates) > 1 {
		match.Continuity, match.Method = "ambiguous", "heuristic"
		for _, candidate := range candidates {
			match.PreviousSymbolIDs = append(match.PreviousSymbolIDs, candidate.id)
		}
	}
	return match, nil
}

func normalizedSymbolStructure(name, signature string) string {
	signature = strings.TrimSpace(signature)
	if signature == "" {
		return ""
	}
	if name != "" {
		signature = strings.Replace(signature, name, "$symbol", 1)
	}
	return strings.Join(strings.Fields(signature), " ")
}

func insertEdge(ctx context.Context, tx *sql.Tx, generationID, from, to, edgeType string, resolution ResolutionStatus, source string, start, end int, metadata map[string]string) error {
	id := stableID("edge", generationID, from, to, edgeType, source, strconv.Itoa(start))
	// OR IGNORE: the ID is deterministic over the edge's full identity, so a
	// duplicate is the same logical edge emitted twice (e.g. repeated calls
	// from one site), not a conflict. Real repositories hit this; fixtures
	// did not.
	_, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO graph_edges (id, generation_id, from_node_id, to_node_id, edge_type, resolution_status, source_file_path, source_start_byte, source_end_byte, metadata) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, id, generationID, from, to, edgeType, resolution, source, start, end, jsonText(metadata, "{}"))
	return err
}

func resolveImportNode(generationID string, file ExtractedFile, target string, modules map[string]string) (string, ResolutionStatus) {
	if node := modules[target]; node != "" {
		return node, ResolutionCompilerExact
	}
	if strings.HasPrefix(target, ".") {
		base := filepath.ToSlash(filepath.Clean(filepath.Join(filepath.Dir(file.Path), target)))
		for _, candidate := range []string{base, base + ".ts", base + ".tsx", base + ".js", base + ".jsx", base + "/index.ts", base + "/index.tsx", base + "/index.js"} {
			if node := modules[candidate]; node != "" {
				return node, ResolutionStaticLexical
			}
		}
	}
	return "", ResolutionUnresolved
}

func isExportedIdentifier(name string) bool {
	for _, r := range name {
		return unicode.IsUpper(r)
	}
	return false
}
