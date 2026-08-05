package knowledge

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

//go:embed ts_compiler_extractor.mjs
var typeScriptCompilerScript []byte

type tsCompilerRequest struct {
	Files []string `json:"files"`
}

type tsCompilerResponse struct {
	Configuration string `json:"configuration"`
	Files         []struct {
		Path     string `json:"path"`
		Language string `json:"language"`
		Symbols  []struct {
			Name      string `json:"name"`
			Kind      string `json:"kind"`
			Parent    string `json:"parent"`
			Signature string `json:"signature"`
			StartLine int    `json:"start_line"`
			EndLine   int    `json:"end_line"`
			StartByte int    `json:"start_byte"`
			EndByte   int    `json:"end_byte"`
			Exported  bool   `json:"exported"`
		} `json:"symbols"`
		Imports []struct {
			Target       string `json:"target"`
			ResolvedPath string `json:"resolved_path"`
			StartByte    int    `json:"start_byte"`
			EndByte      int    `json:"end_byte"`
		} `json:"imports"`
		References []struct {
			TargetName string `json:"target_name"`
			TargetPath string `json:"target_path"`
			StartByte  int    `json:"start_byte"`
			EndByte    int    `json:"end_byte"`
			EdgeType   string `json:"edge_type"`
		} `json:"references"`
	} `json:"files"`
	Diagnostics []struct {
		File    string `json:"file"`
		Message string `json:"message"`
	} `json:"diagnostics"`
}

func extractTypeScriptWithCompiler(ctx context.Context, root string, files []string) (map[string]ExtractedFile, string, []string, error) {
	if len(files) == 0 {
		return map[string]ExtractedFile{}, "compiler_exact", nil, nil
	}
	node, err := findCompatibleNode(ctx)
	if err != nil {
		return nil, "", nil, err
	}
	compiler := findTypeScriptCompiler(root, files)
	if compiler == "" {
		return nil, "", nil, fmt.Errorf("local TypeScript compiler unavailable")
	}
	script, err := os.CreateTemp("", "openexec-ts-*.mjs")
	if err != nil {
		return nil, "", nil, err
	}
	scriptPath := script.Name()
	defer os.Remove(scriptPath)
	if _, err := script.Write(typeScriptCompilerScript); err != nil {
		_ = script.Close()
		return nil, "", nil, err
	}
	if err := script.Close(); err != nil {
		return nil, "", nil, err
	}
	request, _ := json.Marshal(tsCompilerRequest{Files: files})
	commandCtx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	cmd := exec.CommandContext(commandCtx, node, scriptPath, compiler, root)
	cmd.Stdin = bytes.NewReader(request)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		return nil, "", nil, fmt.Errorf("TypeScript compiler extractor: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	var response tsCompilerResponse
	if err := json.Unmarshal(stdout.Bytes(), &response); err != nil {
		return nil, "", nil, fmt.Errorf("decode TypeScript compiler output: %w", err)
	}
	result := make(map[string]ExtractedFile, len(response.Files))
	for _, file := range response.Files {
		extracted := ExtractedFile{Path: file.Path, Language: "typescript", PackageName: filepath.Base(filepath.Dir(file.Path))}
		for _, symbol := range file.Symbols {
			extracted.Symbols = append(extracted.Symbols, ExtractedSymbol{Name: symbol.Name, Kind: symbol.Kind, Parent: symbol.Parent, Signature: symbol.Signature, StartLine: symbol.StartLine, EndLine: symbol.EndLine, StartByte: symbol.StartByte, EndByte: symbol.EndByte, Exported: symbol.Exported, Resolution: ResolutionCompilerExact})
		}
		for _, imp := range file.Imports {
			metadataTarget := imp.Target
			if imp.ResolvedPath != "" {
				metadataTarget = imp.ResolvedPath
			}
			extracted.Imports = append(extracted.Imports, ExtractedImport{Target: metadataTarget, StartByte: imp.StartByte, EndByte: imp.EndByte, Resolution: ResolutionCompilerExact})
		}
		for _, reference := range file.References {
			extracted.References = append(extracted.References, ExtractedReference{TargetName: reference.TargetName, TargetPath: reference.TargetPath, StartByte: reference.StartByte, EndByte: reference.EndByte, EdgeType: reference.EdgeType, Resolution: ResolutionCompilerExact})
		}
		result[file.Path] = extracted
	}
	var limitations []string
	for _, diagnostic := range response.Diagnostics {
		limitations = append(limitations, fmt.Sprintf("%s: %s", diagnostic.File, diagnostic.Message))
		if len(limitations) == 20 {
			limitations = append(limitations, "additional TypeScript diagnostics truncated")
			break
		}
	}
	return result, response.Configuration, limitations, nil
}

func findCompatibleNode(ctx context.Context) (string, error) {
	var candidates []string
	for _, directory := range filepath.SplitList(os.Getenv("PATH")) {
		if directory != "" {
			candidates = append(candidates, filepath.Join(directory, "node"), filepath.Join(directory, "node.exe"))
		}
	}
	for _, directory := range []string{os.Getenv("NVM_BIN"), filepath.Join(os.Getenv("VOLTA_HOME"), "bin"), "/opt/homebrew/bin", "/usr/local/bin", "/usr/bin"} {
		if directory != "" && directory != "bin" {
			candidates = append(candidates, filepath.Join(directory, "node"), filepath.Join(directory, "node.exe"))
		}
	}
	if nvmRoot := os.Getenv("NVM_DIR"); nvmRoot != "" {
		matches, _ := filepath.Glob(filepath.Join(nvmRoot, "versions", "node", "*", "bin", "node"))
		candidates = append(candidates, matches...)
	}

	seen := make(map[string]bool)
	var found []string
	for _, candidate := range candidates {
		candidate = filepath.Clean(candidate)
		if seen[candidate] {
			continue
		}
		seen[candidate] = true
		if info, err := os.Stat(candidate); err != nil || info.IsDir() {
			continue
		}
		probeCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
		output, err := exec.CommandContext(probeCtx, candidate, "-e", `process.stdout.write(process.versions.node.split('.')[0])`).CombinedOutput()
		cancel()
		if err != nil {
			continue
		}
		major, err := strconv.Atoi(strings.TrimSpace(string(output)))
		if err != nil {
			continue
		}
		if major >= 18 {
			return candidate, nil
		}
		found = append(found, fmt.Sprintf("%s (v%d)", candidate, major))
	}
	if len(found) > 0 {
		return "", fmt.Errorf("Node.js 18+ runtime unavailable; incompatible runtimes: %s", strings.Join(found, ", "))
	}
	return "", fmt.Errorf("Node.js 18+ runtime unavailable")
}

// findTypeScriptCompiler locates a TypeScript compiler to analyse with. It is
// the difference between this extractor and an IDE: with the compiler, a
// reference resolves to the symbol it actually names, and "which functions call
// this" and "is this export used" are answered exactly. Without it, everything
// degrades to matching identifiers by spelling.
//
// The repository is searched first — a project's own pinned version analyses it
// most faithfully — but a repository that has never run `npm install` used to
// get the lexical fallback silently. A compiler installed anywhere the host can
// see is far better than none.
func findTypeScriptCompiler(root string, files []string) string {
	candidates := []string{filepath.Join(root, "node_modules", "typescript", "lib", "typescript.js")}
	for _, rel := range files {
		dir := filepath.Dir(filepath.Join(root, filepath.FromSlash(rel)))
		for {
			candidates = append(candidates, filepath.Join(dir, "node_modules", "typescript", "lib", "typescript.js"))
			if dir == root || filepath.Dir(dir) == dir || !strings.HasPrefix(dir, root) {
				break
			}
			dir = filepath.Dir(dir)
		}
	}
	candidates = append(candidates, hostTypeScriptCandidates()...)
	seen := make(map[string]bool)
	for _, candidate := range candidates {
		candidate = filepath.Clean(candidate)
		if seen[candidate] {
			continue
		}
		seen[candidate] = true
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate
		}
	}
	return ""
}

// hostTypeScriptCandidates lists compilers installed outside the repository:
// an explicit override first, then NODE_PATH, then the usual global roots.
func hostTypeScriptCandidates() []string {
	const lib = "typescript/lib/typescript.js"
	var out []string
	if explicit := strings.TrimSpace(os.Getenv("OPENEXEC_TYPESCRIPT_LIB")); explicit != "" {
		out = append(out, explicit)
	}
	for _, entry := range filepath.SplitList(os.Getenv("NODE_PATH")) {
		if entry != "" {
			out = append(out, filepath.Join(entry, lib))
		}
	}
	roots := []string{
		"/usr/lib/node_modules", "/usr/local/lib/node_modules",
		"/opt/homebrew/lib/node_modules",
	}
	if home, err := os.UserHomeDir(); err == nil {
		roots = append(roots,
			filepath.Join(home, ".npm-global", "lib", "node_modules"),
			filepath.Join(home, "node_modules"),
			filepath.Join(home, ".local", "lib", "node_modules"),
		)
	}
	for _, root := range roots {
		out = append(out, filepath.Join(root, lib))
	}
	return out
}
