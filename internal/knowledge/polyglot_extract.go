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
		// A decorated definition is not offered for deletion. Something holds a
		// reference to it — a router, a worker, a test runner — and which one
		// needs framework knowledge this extractor lacks. Wrong here costs
		// working code, so anything decorated counts as used.
		if at := decoratedDeclaration(data, start); at >= 0 {
			// TargetPath pins the reference to this file's definition. Without
			// it a handler named `index` in one module would keep every
			// same-named symbol in the repository alive.
			result.References = append(result.References, ExtractedReference{
				TargetName: name, TargetPath: rel, StartByte: at, EndByte: at + len(name),
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

// decoratedDeclaration reports the offset of the decorator block above a
// definition, or -1.
//
// It does not try to tell a registering decorator from a behavioural one. The
// question this answers is "may a deletion tool offer this?", and for that the
// safe answer is no whenever anything decorates the definition: @router.get
// hands it to FastAPI, @fixture hands it to pytest under a bare imported name,
// and a stack can mix both so the nearest one proves nothing. Distinguishing
// them needs framework knowledge this extractor does not have; being wrong
// here costs working code.
//
// Decorator arguments span lines, so the scan walks up through a balanced
// bracket region rather than reading only the line above.
func decoratedDeclaration(data []byte, declaration int) int {
	line := declaration
	depth := 0
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
		// Count brackets right-to-left: a line closing more than it opens is
		// the tail of a multi-line decorator argument list.
		opens, closes := strings.Count(text, "(")+strings.Count(text, "["), strings.Count(text, ")")+strings.Count(text, "]")
		depth += closes - opens
		if strings.HasPrefix(text, "@") && depth <= 0 {
			return start
		}
		if depth > 0 {
			line = start
			continue
		}
		return -1
	}
	return -1
}
