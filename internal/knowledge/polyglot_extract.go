package knowledge

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

var (
	pythonDeclaration = regexp.MustCompile(`(?m)^([ \t]*)(?:async[ \t]+)?(def|class)[ \t]+([A-Za-z_][A-Za-z0-9_]*)[ \t]*(?:\(|:)`)
	pythonImport      = regexp.MustCompile(`(?m)^[ \t]*(?:from[ \t]+([A-Za-z_][A-Za-z0-9_.]*)[ \t]+import|import[ \t]+([A-Za-z_][A-Za-z0-9_.]*))`)
	pythonCall        = regexp.MustCompile(`\b([A-Za-z_][A-Za-z0-9_]*)[ \t]*\(`)
)

func extractPythonFile(root, path, rel string) (ExtractedFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return ExtractedFile{}, err
	}
	result := ExtractedFile{Path: rel, Language: "python", PackageName: filepath.Base(filepath.Dir(rel)), Limitations: []string{"Python definitions and imports use static lexical extraction; decorators, dynamic dispatch, and ORM effects are not fully resolved"}}
	for _, match := range pythonImport.FindAllSubmatchIndex(data, -1) {
		start, end := match[2], match[3]
		if start < 0 {
			start, end = match[4], match[5]
		}
		target := string(data[start:end])
		if resolved := resolvePythonModule(root, path, target); resolved != "" {
			target = resolved
		}
		result.Imports = append(result.Imports, ExtractedImport{Target: target, StartByte: match[0], EndByte: match[1], Resolution: ResolutionStaticLexical})
	}
	declarations := pythonDeclaration.FindAllSubmatchIndex(data, -1)
	for _, match := range declarations {
		indent := string(data[match[2]:match[3]])
		kind := string(data[match[4]:match[5]])
		name := string(data[match[6]:match[7]])
		start := match[0]
		headerEnd := pythonDeclarationHeaderEnd(data, start)
		end := pythonDeclarationEnd(data, headerEnd, len(indent))
		if kind == "def" {
			kind = "function"
			if len(indent) > 0 {
				kind = "method"
			}
		}
		result.Symbols = append(result.Symbols, ExtractedSymbol{
			Name: name, Kind: kind, Signature: strings.TrimSpace(string(data[start:headerEnd])),
			StartLine: lineNumberAt(data, start), EndLine: lineNumberAt(data, end), StartByte: start, EndByte: end,
			Exported: !strings.HasPrefix(name, "_"), Resolution: ResolutionStaticLexical,
		})
		// A registering decorator is a use. @router.get("/") hands the function
		// to FastAPI, @celery.task to a worker, @pytest.fixture to pytest — the
		// framework calls it and no line of code ever does. Without this the
		// cleanup list offered every HTTP handler in the service for deletion.
		if at := registeringDecorator(data, start); at >= 0 {
			result.References = append(result.References, ExtractedReference{
				TargetName: name, StartByte: at, EndByte: at + len(name),
				EdgeType: "references", Resolution: ResolutionConfigurationDerived,
			})
		}
		for _, call := range pythonCall.FindAllSubmatchIndex(data[start:end], -1) {
			target := string(data[start+call[2] : start+call[3]])
			if isLanguageCallKeyword(target) || target == name {
				continue
			}
			// session.commit() names a method on an object of unknown type;
			// matching it by name alone made commit, post and count the
			// repository's most depended-on symbols. Evidence of use, but not
			// evidence of importance.
			result.References = append(result.References, ExtractedReference{TargetName: target, StartByte: start + call[2], EndByte: start + call[3], EdgeType: "calls", Resolution: callResolution(data, start+call[2])})
		}
	}
	// Calls found inside declarations were the only references recorded, so a
	// class named in a type hint, a constant read at module level and anything
	// used outside a def left no trace at all — and Python puts a great deal at
	// module level. Every mention outside a declaration header counts, which
	// errs towards calling a symbol live; that is the safe direction when the
	// answer decides whether the owner is invited to delete it.
	claimed := map[int]bool{}
	for _, symbol := range result.Symbols {
		claimed[symbol.StartByte] = true
	}
	for _, match := range identifierPattern.FindAllSubmatchIndex(data, -1) {
		name := string(data[match[2]:match[3]])
		if isLanguageCallKeyword(name) || claimed[match[2]] || declaresSymbolAt(result.Symbols, match[2]) {
			continue
		}
		result.References = append(result.References, ExtractedReference{TargetName: name, StartByte: match[2], EndByte: match[3], EdgeType: "references", Resolution: ResolutionStaticLexical})
	}
	return result, nil
}

func pythonDeclarationHeaderEnd(data []byte, start int) int {
	depth := 0
	quote := byte(0)
	escaped := false
	comment := false
	for index := start; index < len(data); index++ {
		character := data[index]
		if comment {
			if character == '\n' {
				comment = false
			}
			continue
		}
		if quote != 0 {
			if escaped {
				escaped = false
				continue
			}
			if character == '\\' {
				escaped = true
				continue
			}
			if character == quote {
				quote = 0
			}
			continue
		}
		switch character {
		case '#':
			comment = true
		case '\'', '"':
			quote = character
		case '(', '[', '{':
			depth++
		case ')', ']', '}':
			if depth > 0 {
				depth--
			}
		case ':':
			if depth == 0 {
				return index + 1
			}
		}
	}
	return len(data)
}

func pythonDeclarationEnd(data []byte, headerEnd, indent int) int {
	headerLineEnd := strings.IndexByte(string(data[headerEnd:]), '\n')
	if headerLineEnd < 0 {
		return len(data)
	}
	headerLineEnd += headerEnd
	// A simple one-line function/class owns the rest of its declaration line.
	if trailing := strings.TrimSpace(string(data[headerEnd:headerLineEnd])); trailing != "" && !strings.HasPrefix(trailing, "#") {
		return headerLineEnd
	}
	for offset := headerLineEnd + 1; offset < len(data); {
		next := strings.IndexByte(string(data[offset:]), '\n')
		if next < 0 {
			next = len(data) - offset
		}
		line := data[offset : offset+next]
		trimmed := strings.TrimSpace(string(line))
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			offset += next + 1
			continue
		}
		spaces := 0
		for spaces < len(line) && (line[spaces] == ' ' || line[spaces] == '\t') {
			spaces++
		}
		if spaces <= indent {
			return trimPythonDeclarationEnd(data, offset)
		}
		offset += next + 1
	}
	return trimPythonDeclarationEnd(data, len(data))
}

func trimPythonDeclarationEnd(data []byte, end int) int {
	for end > 0 {
		switch data[end-1] {
		case ' ', '\t', '\r', '\n':
			end--
		default:
			return end
		}
	}
	return end
}

func resolvePythonModule(root, sourcePath, module string) string {
	relative := filepath.FromSlash(strings.ReplaceAll(module, ".", "/"))
	for directory := filepath.Dir(sourcePath); ; directory = filepath.Dir(directory) {
		for _, suffix := range []string{relative + ".py", filepath.Join(relative, "__init__.py")} {
			candidate := filepath.Join(directory, suffix)
			if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
				if rel, err := filepath.Rel(root, candidate); err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
					return filepath.ToSlash(rel)
				}
			}
		}
		if directory == root || filepath.Dir(directory) == directory || !strings.HasPrefix(directory, root) {
			break
		}
	}
	return ""
}

func extractSvelteFile(path, rel string) (ExtractedFile, error) {
	result, err := extractTypeScriptLexical(path, rel)
	if err != nil {
		return ExtractedFile{}, err
	}
	result.Language = "svelte"
	result.Limitations = []string{"Svelte component scripts use static lexical extraction; reactive markup and generated component code are not fully resolved"}
	data, err := os.ReadFile(path)
	if err != nil {
		return ExtractedFile{}, err
	}
	if route := svelteRouteName(rel); route != "" {
		result.Symbols = append(result.Symbols, ExtractedSymbol{Name: route, Kind: "route", Signature: route, StartLine: 1, EndLine: lineNumberAt(data, len(data)), StartByte: 0, EndByte: len(data), Exported: true, Resolution: ResolutionConfigurationDerived})
	}
	sort.Slice(result.Symbols, func(i, j int) bool { return result.Symbols[i].StartByte < result.Symbols[j].StartByte })
	return result, nil
}

func svelteRouteName(rel string) string {
	path := filepath.ToSlash(rel)
	marker := "/src/routes/"
	index := strings.Index(path, marker)
	if index < 0 || !strings.HasPrefix(filepath.Base(path), "+") {
		return ""
	}
	route := strings.TrimSuffix(path[index+len(marker):], "/"+filepath.Base(path))
	parts := strings.Split(route, "/")
	var visible []string
	for _, part := range parts {
		if part != "" && !(strings.HasPrefix(part, "(") && strings.HasSuffix(part, ")")) {
			visible = append(visible, part)
		}
	}
	return "/" + strings.Join(visible, "/") + " [" + filepath.Base(path) + "]"
}

// callResolution grades a lexical call site: a bare name is a plausible
// pointer to a symbol declared here, a dotted one is a method on a receiver
// whose type this extractor cannot know.
func callResolution(data []byte, offset int) ResolutionStatus {
	for i := offset - 1; i >= 0; i-- {
		switch data[i] {
		case ' ', '\t':
			continue
		case '.':
			return ResolutionStaticLexical
		}
		break
	}
	return ResolutionHeuristic
}

// registeringDecorator reports the offset of a decorator that hands the
// following definition to something else, or -1. A bare @property or
// @staticmethod only changes how the definition behaves; a dotted or called
// decorator — @router.get(...), @app.task, @pytest.fixture — registers it
// somewhere this extractor cannot see.
func registeringDecorator(data []byte, declaration int) int {
	line := declaration
	for line > 0 {
		previousEnd := line - 1
		if previousEnd > 0 && data[previousEnd-1] == '\r' {
			previousEnd--
		}
		start := previousEnd
		for start > 0 && data[start-1] != '\n' {
			start--
		}
		text := strings.TrimSpace(string(data[start:previousEnd]))
		if text == "" {
			line = start
			continue
		}
		if !strings.HasPrefix(text, "@") {
			return -1
		}
		body := strings.TrimPrefix(text, "@")
		if strings.ContainsAny(body, ".(") {
			return start
		}
		line = start
	}
	return -1
}
