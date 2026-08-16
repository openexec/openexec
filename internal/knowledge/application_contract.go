package knowledge

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

type openAPIDocument struct {
	Paths map[string]yaml.Node `yaml:"paths"`
}

type surfaceManifest struct {
	Surfaces []struct {
		ID      string   `json:"id"`
		Owner   string   `json:"owner"`
		Sources []string `json:"sources"`
		States  []string `json:"states"`
	} `json:"surfaces"`
}

func extractApplicationContractFile(path, rel string) (ExtractedFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return ExtractedFile{}, err
	}
	result := ExtractedFile{Path: filepath.ToSlash(rel), Language: "configuration", PackageName: filepath.Base(filepath.Dir(rel))}
	base := strings.ToLower(filepath.Base(rel))
	switch base {
	case "openapi.yaml", "openapi.yml":
		var document openAPIDocument
		if err := yaml.Unmarshal(data, &document); err != nil {
			return ExtractedFile{}, fmt.Errorf("parse OpenAPI contract: %w", err)
		}
		for route, pathItem := range document.Paths {
			if pathItem.Kind != yaml.MappingNode {
				result.Limitations = appendUniqueString(result.Limitations, "OpenAPI path items that are not mappings remain unresolved")
				continue
			}
			for index := 0; index+1 < len(pathItem.Content); index += 2 {
				if pathItem.Content[index].Value == "$ref" {
					result.Limitations = appendUniqueString(result.Limitations, "referenced OpenAPI path items remain unresolved")
					continue
				}
				method := strings.ToUpper(pathItem.Content[index].Value)
				if !isHTTPMethod(method) {
					continue
				}
				operationID := yamlMappingValue(pathItem.Content[index+1], "operationId")
				if operationID == nil || strings.TrimSpace(operationID.Value) == "" {
					result.Limitations = appendUniqueString(result.Limitations, "OpenAPI operations without operationId remain unresolved")
					continue
				}
				start := yamlNodeByteOffset(data, operationID)
				end := minInt(len(data), start+len(operationID.Value))
				result.Symbols = append(result.Symbols, ExtractedSymbol{
					Name: operationID.Value, Kind: "http_operation", Signature: method + " " + route,
					StartLine: lineNumberAt(data, start), EndLine: lineNumberAt(data, end), StartByte: start, EndByte: end,
					Exported: true, Resolution: ResolutionConfigurationDerived,
				})
			}
		}
		result.Language = "openapi"
	case "surfaces.json", "usability-manifest.json":
		var manifest surfaceManifest
		if err := json.Unmarshal(data, &manifest); err != nil {
			return ExtractedFile{}, fmt.Errorf("parse UI surface manifest: %w", err)
		}
		for _, surface := range manifest.Surfaces {
			if strings.TrimSpace(surface.ID) == "" {
				continue
			}
			start, end := locateJSONMemberValue(data, "id", surface.ID)
			result.Symbols = append(result.Symbols, ExtractedSymbol{
				Name: surface.ID, Kind: "ui_surface", Signature: "surface " + surface.ID,
				StartLine: lineNumberAt(data, start), EndLine: lineNumberAt(data, end), StartByte: start, EndByte: end,
				Exported: true, Resolution: ResolutionConfigurationDerived,
			})
		}
		result.Language = "ui_manifest"
	default:
		if strings.HasSuffix(base, ".schema.json") {
			var schema struct {
				Properties map[string]json.RawMessage `json:"properties"`
			}
			if err := json.Unmarshal(data, &schema); err != nil {
				return ExtractedFile{}, fmt.Errorf("parse JSON schema: %w", err)
			}
			for name := range schema.Properties {
				start, end := locateJSONKey(data, name)
				result.Symbols = append(result.Symbols, ExtractedSymbol{
					Name: name, Kind: "persisted_field", Signature: "schema property " + name,
					StartLine: lineNumberAt(data, start), EndLine: lineNumberAt(data, end), StartByte: start, EndByte: end,
					Exported: true, Resolution: ResolutionConfigurationDerived,
				})
			}
			result.Language = "json_schema"
		}
	}
	sort.Slice(result.Symbols, func(i, j int) bool { return result.Symbols[i].StartByte < result.Symbols[j].StartByte })
	return result, nil
}

func yamlMappingValue(node *yaml.Node, key string) *yaml.Node {
	if node == nil || node.Kind != yaml.MappingNode {
		return nil
	}
	for index := 0; index+1 < len(node.Content); index += 2 {
		if node.Content[index].Value == key {
			return node.Content[index+1]
		}
	}
	return nil
}

func yamlNodeByteOffset(data []byte, node *yaml.Node) int {
	if node == nil || node.Line < 1 || node.Column < 1 {
		return 0
	}
	line, offset := 1, 0
	for offset < len(data) && line < node.Line {
		if data[offset] == '\n' {
			line++
		}
		offset++
	}
	column := 1
	for offset < len(data) && column < node.Column {
		offset++
		column++
	}
	if offset < len(data) && (data[offset] == '"' || data[offset] == '\'') {
		offset++
	}
	return offset
}

func locateJSONMemberValue(data []byte, key, value string) (int, int) {
	pattern := regexp.MustCompile(`"` + regexp.QuoteMeta(key) + `"\s*:\s*"` + regexp.QuoteMeta(value) + `"`)
	location := pattern.FindIndex(data)
	if location == nil {
		return 0, minInt(len(data), 1)
	}
	segment := data[location[0]:location[1]]
	quotedValue := []byte(`"` + value + `"`)
	relative := strings.LastIndex(string(segment), string(quotedValue))
	if relative < 0 {
		return 0, minInt(len(data), 1)
	}
	start := location[0] + relative + 1
	return start, start + len(value)
}

func locateJSONKey(data []byte, key string) (int, int) {
	pattern := regexp.MustCompile(`"` + regexp.QuoteMeta(key) + `"\s*:`)
	location := pattern.FindIndex(data)
	if location == nil {
		return 0, minInt(len(data), 1)
	}
	start := location[0] + 1
	return start, start + len(key)
}

type contractDeclaration struct {
	file   string
	name   string
	start  int
	kind   string
	method string
	path   string
}

func deriveApplicationContracts(root string, files []ExtractedFile) ([]ExtractedFile, []string, error) {
	contents := make(map[string][]byte, len(files))
	readable := make([]ExtractedFile, 0, len(files))
	var limitations []string
	for _, file := range files {
		data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(file.Path)))
		if err != nil {
			limitations = appendUniqueString(limitations, fmt.Sprintf("application contract derivation could not read %s: %v", file.Path, err))
			continue
		}
		contents[file.Path] = data
		readable = append(readable, file)
	}
	files = readable
	byPath := make(map[string]int, len(files))
	declarations := make(map[string][]contractDeclaration)
	operations := make(map[string]contractDeclaration)
	for index, file := range files {
		byPath[file.Path] = index
		for _, symbol := range file.Symbols {
			declaration := contractDeclaration{file: file.Path, name: symbol.Name, start: symbol.StartByte, kind: symbol.Kind}
			declarations[symbol.Name] = append(declarations[symbol.Name], declaration)
			if symbol.Kind == "http_operation" {
				parts := strings.SplitN(symbol.Signature, " ", 2)
				if len(parts) == 2 {
					declaration.method, declaration.path = parts[0], parts[1]
					operations[httpOperationKey(parts[0], parts[1])] = declaration
				}
			}
		}
	}

	routeHandlers := make(map[string]contractDeclaration)
	generatedContractSeen := false
	for index := range files {
		file := &files[index]
		data := contents[file.Path]
		if file.Language == "go" {
			routes, unsupported := extractGoHTTPRoutes(data)
			if unsupported {
				limitations = appendUniqueString(limitations, "HTTP route registrations that are not method-prefixed ServeMux patterns remain unresolved")
			}
			for _, route := range routes {
				operation, found := operations[httpOperationKey(route.method, route.path)]
				if !found {
					limitations = appendUniqueString(limitations, "registered HTTP routes without an OpenAPI operation remain unresolved")
					continue
				}
				handler, resolved := uniqueHandlerDeclaration(declarations[route.handler], file.Path)
				if !resolved {
					limitations = appendUniqueString(limitations, "HTTP handler names that do not resolve uniquely remain unresolved")
					continue
				}
				handlerIndex := byPath[handler.file]
				files[handlerIndex].References = append(files[handlerIndex].References, ExtractedReference{
					SourceName: handler.name, SourcePath: handler.file, TargetName: operation.name, TargetPath: operation.file,
					StartByte: handler.start, EndByte: handler.start + len(handler.name), EdgeType: "implements_http_operation", Resolution: ResolutionConfigurationDerived,
				})
				routeHandlers[httpOperationKey(route.method, route.path)] = handler
			}
		}
	}

	for index := range files {
		file := &files[index]
		data := contents[file.Path]
		if file.Language == "typescript" || file.Language == "javascript" || file.Language == "svelte" {
			calls, dynamic := extractAPIClientCalls(data)
			if dynamic {
				limitations = appendUniqueString(limitations, "dynamically constructed HTTP client paths remain unresolved")
			}
			for _, call := range calls {
				key := httpOperationKey(call.method, call.path)
				operation, found := operations[key]
				if !found {
					limitations = appendUniqueString(limitations, "HTTP client calls without an OpenAPI operation remain unresolved")
					continue
				}
				file.References = append(file.References, ExtractedReference{
					TargetName: operation.name, TargetPath: operation.file, StartByte: call.start, EndByte: call.end,
					EdgeType: "uses_http_operation", Resolution: ResolutionHeuristic,
				})
				if handler, ok := routeHandlers[key]; ok {
					file.References = append(file.References, ExtractedReference{
						TargetName: handler.name, TargetPath: handler.file, StartByte: call.start, EndByte: call.end,
						EdgeType: "http_contract_consumer", Resolution: ResolutionHeuristic,
					})
				}
			}
		}

		file.Effects = append(file.Effects, extractOperationalEffects(data)...)
	}

	for index := range files {
		file := &files[index]
		base := strings.ToLower(filepath.Base(file.Path))
		if base == "surfaces.json" || base == "usability-manifest.json" {
			manifestRelations, manifestReferences, unresolved, err := deriveSurfaceManifest(contents[file.Path], file.Path, byPath)
			if err != nil {
				return nil, nil, err
			}
			file.Relations = append(file.Relations, manifestRelations...)
			for owner, references := range manifestReferences {
				ownerIndex := byPath[owner]
				files[ownerIndex].References = append(files[ownerIndex].References, references...)
			}
			if unresolved {
				limitations = appendUniqueString(limitations, "UI surfaces without a source or owning browser journey remain unresolved")
			}
		}
		if base == "package.json" {
			relations, err := deriveGeneratedArtifacts(contents[file.Path], file.Path, byPath)
			if err != nil {
				return nil, nil, err
			}
			file.Relations = append(file.Relations, relations...)
			generatedContractSeen = generatedContractSeen || len(relations) > 0
		}
	}
	if generatedContractSeen {
		limitations = appendUniqueString(limitations, "cross-repository generated-package consumers require a portfolio graph and remain unresolved in a single-checkout scan")
	}
	return files, limitations, nil
}

type goHTTPRoute struct{ method, path, handler string }

var goHandleFunc = regexp.MustCompile(`HandleFunc\(\s*"([A-Z]+) ([^"]+)"\s*,\s*([^\n]+)`) // one registration statement
var goHandleCall = regexp.MustCompile(`\b(?:HandleFunc|Handle)\s*\(`)
var goOtherRouterCall = regexp.MustCompile(`\b(?:r|router|mux)\.(?:Get|Post|Put|Patch|Delete|Options|Head)\s*\(`)
var selectorName = regexp.MustCompile(`[A-Za-z_][A-Za-z0-9_]*\.([A-Za-z_][A-Za-z0-9_]*)`)
var bareHandlerName = regexp.MustCompile(`^\s*([A-Za-z_][A-Za-z0-9_]*)\s*[,)]`)

func extractGoHTTPRoutes(data []byte) ([]goHTTPRoute, bool) {
	var routes []goHTTPRoute
	for _, match := range goHandleFunc.FindAllSubmatch(data, -1) {
		selectors := selectorName.FindAllSubmatch(match[3], -1)
		handler := ""
		if len(selectors) > 0 {
			handler = string(selectors[len(selectors)-1][1])
		} else if bare := bareHandlerName.FindSubmatch(match[3]); len(bare) == 2 {
			handler = string(bare[1])
		}
		if handler != "" {
			routes = append(routes, goHTTPRoute{method: string(match[1]), path: string(match[2]), handler: handler})
		}
	}
	unsupported := len(goHandleCall.FindAllIndex(data, -1)) > len(routes) || goOtherRouterCall.Match(data)
	return routes, unsupported
}

type apiClientCall struct {
	method string
	path   string
	start  int
	end    int
}

var apiCallStart = regexp.MustCompile(`\bapi(?:<[^\n>]+>)?\s*\(`)
var apiCallMethod = regexp.MustCompile(`(?i)method\s*:\s*['"]([A-Z]+)['"]`)
var templateExpression = regexp.MustCompile(`\$\{[^}]+\}`)

func extractAPIClientCalls(data []byte) ([]apiClientCall, bool) {
	var calls []apiClientCall
	dynamic := false
	for _, location := range apiCallStart.FindAllIndex(data, -1) {
		start := location[1]
		for start < len(data) && (data[start] == ' ' || data[start] == '\t' || data[start] == '\n') {
			start++
		}
		if start >= len(data) || (data[start] != '\'' && data[start] != '"' && data[start] != '`') {
			dynamic = true
			continue
		}
		quote := data[start]
		end := start + 1
		for end < len(data) {
			if data[end] == quote && data[end-1] != '\\' {
				break
			}
			end++
		}
		if end >= len(data) {
			dynamic = true
			continue
		}
		path := string(data[start+1 : end])
		if !strings.HasPrefix(path, "/api/") && path != "/api" {
			continue
		}
		method := "GET"
		callEnd := findCallClosingParen(data, location[1]-1)
		if callEnd < end {
			callEnd = minInt(len(data), end+1)
		}
		if match := apiCallMethod.FindSubmatch(data[end:callEnd]); len(match) == 2 {
			method = strings.ToUpper(string(match[1]))
		}
		calls = append(calls, apiClientCall{method: method, path: templateExpression.ReplaceAllString(path, "{}"), start: start + 1, end: end})
	}
	return calls, dynamic
}

func findCallClosingParen(data []byte, open int) int {
	depth := 0
	quote := byte(0)
	escaped := false
	for index := open; index < len(data); index++ {
		character := data[index]
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
		case '\'', '"', '`':
			quote = character
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return index
			}
		}
	}
	return len(data)
}

func httpOperationKey(method, path string) string {
	segments := strings.Split(strings.SplitN(path, "?", 2)[0], "/")
	for index, segment := range segments {
		if strings.HasPrefix(segment, "{") && strings.HasSuffix(segment, "}") {
			segments[index] = "{}"
		}
	}
	return strings.ToUpper(method) + " " + strings.Join(segments, "/")
}

func isHTTPMethod(value string) bool {
	switch value {
	case "GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS":
		return true
	}
	return false
}

func uniqueDeclaration(candidates []contractDeclaration, kind string) (contractDeclaration, bool) {
	var found []contractDeclaration
	for _, candidate := range candidates {
		if kind == "" || candidate.kind == kind {
			found = append(found, candidate)
		}
	}
	return firstOrZero(found), len(found) == 1
}

func uniqueHandlerDeclaration(candidates []contractDeclaration, routeFile string) (contractDeclaration, bool) {
	var handlers []contractDeclaration
	for _, candidate := range candidates {
		if (candidate.kind == "function" || candidate.kind == "method") && filepath.Dir(candidate.file) == filepath.Dir(routeFile) {
			handlers = append(handlers, candidate)
		}
	}
	return firstOrZero(handlers), len(handlers) == 1
}

func firstOrZero(values []contractDeclaration) contractDeclaration {
	if len(values) == 0 {
		return contractDeclaration{}
	}
	return values[0]
}

func deriveSurfaceManifest(data []byte, manifestPath string, byPath map[string]int) ([]ExtractedModuleRelation, map[string][]ExtractedReference, bool, error) {
	var manifest surfaceManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return nil, nil, false, err
	}
	base := filepath.ToSlash(filepath.Dir(manifestPath))
	var relations []ExtractedModuleRelation
	references := make(map[string][]ExtractedReference)
	unresolved := false
	for _, surface := range manifest.Surfaces {
		owner := filepath.ToSlash(filepath.Clean(filepath.Join(base, surface.Owner)))
		if _, exists := byPath[owner]; !exists || len(surface.Sources) == 0 {
			unresolved = true
			continue
		}
		references[owner] = append(references[owner], ExtractedReference{
			TargetName: surface.ID, TargetPath: manifestPath, EdgeType: "exercises_surface", Resolution: ResolutionConfigurationDerived,
		})
		for _, source := range surface.Sources {
			source = filepath.ToSlash(filepath.Clean(source))
			if _, sourceExists := byPath[source]; !sourceExists {
				unresolved = true
				continue
			}
			relations = append(relations, ExtractedModuleRelation{
				FromPath: source, TargetPath: owner, EdgeType: "tested_by", Resolution: ResolutionConfigurationDerived,
				Metadata: map[string]string{"reason": "browser surface manifest", "surface": surface.ID, "test_file": owner},
			})
		}
	}
	return relations, references, unresolved, nil
}

var openAPIGenerator = regexp.MustCompile(`openapi-typescript\s+([^\s]+)\s+-o\s+([^\s]+)`)

func deriveGeneratedArtifacts(data []byte, packagePath string, byPath map[string]int) ([]ExtractedModuleRelation, error) {
	var document struct {
		Scripts map[string]string `json:"scripts"`
	}
	if err := json.Unmarshal(data, &document); err != nil {
		return nil, nil // package metadata that is not JSON is already a scan limitation elsewhere
	}
	base := filepath.Dir(packagePath)
	var relations []ExtractedModuleRelation
	for _, command := range document.Scripts {
		match := openAPIGenerator.FindStringSubmatch(command)
		if len(match) != 3 {
			continue
		}
		source := filepath.ToSlash(filepath.Clean(filepath.Join(base, match[1])))
		target := filepath.ToSlash(filepath.Clean(filepath.Join(base, match[2])))
		if _, sourceExists := byPath[source]; !sourceExists {
			continue
		}
		if _, targetExists := byPath[target]; !targetExists {
			continue
		}
		relations = append(relations, ExtractedModuleRelation{
			FromPath: target, TargetPath: source, EdgeType: "generated_from", Resolution: ResolutionConfigurationDerived,
			Metadata: map[string]string{"producer": packagePath, "command": "openapi-typescript"},
		})
	}
	return relations, nil
}

var operationalEffectPatterns = []struct {
	kind     string
	evidence string
	pattern  *regexp.Regexp
}{
	{"git", "git subprocess", regexp.MustCompile(`exec\.Command(?:Context)?\([^\n,]+,\s*"git"`)},
	{"filesystem", "filesystem mutation", regexp.MustCompile(`os\.(?:WriteFile|Remove|RemoveAll|Rename|Mkdir|MkdirAll|Create|OpenFile)\s*\(`)},
	{"subprocess", "subprocess execution", regexp.MustCompile(`exec\.(?:Command|CommandContext)\s*\(`)},
	{"scheduler", "scheduled callback", regexp.MustCompile(`time\.(?:NewTicker|AfterFunc|Tick)\s*\(`)},
	{"restart", "process termination or restart", regexp.MustCompile(`(?:os\.Exit|syscall\.Kill)\s*\(`)},
	{"notification", "notification dispatch", regexp.MustCompile(`(?i)\b(?:notify[A-Za-z0-9_]*|pushNtfy)\s*\(`)},
	{"external_network", "HTTP client request", regexp.MustCompile(`http\.(?:NewRequest|NewRequestWithContext|Get|Post)\s*\(`)},
}

var operationalEffectRegistration = regexp.MustCompile(`openexec:effect\s+(git|filesystem|subprocess|scheduler|restart|notification|external_network)\b`)

func extractOperationalEffects(data []byte) []ExtractedEffect {
	var effects []ExtractedEffect
	for _, candidate := range operationalEffectPatterns {
		for _, match := range candidate.pattern.FindAllIndex(data, -1) {
			effects = append(effects, ExtractedEffect{Kind: candidate.kind, Evidence: candidate.evidence, StartByte: match[0], EndByte: match[1], Resolution: ResolutionStaticLexical})
		}
	}
	for _, match := range operationalEffectRegistration.FindAllSubmatchIndex(data, -1) {
		kind := string(data[match[2]:match[3]])
		start := match[1]
		if nextFunction := strings.Index(string(data[start:]), "func "); nextFunction >= 0 {
			start += nextFunction
		}
		effects = append(effects, ExtractedEffect{Kind: kind, Evidence: "adjacent typed effect registration", StartByte: start, EndByte: start + len(kind), Resolution: ResolutionConfigurationDerived})
	}
	return effects
}

func minInt(left, right int) int {
	if left < right {
		return left
	}
	return right
}
