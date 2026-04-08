package contracts

import (
	"embed"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"text/template"
	"unicode"
)

//go:embed templates/*.tmpl
var templateFS embed.FS

// templateData is the rendering context passed to each block template.
type templateData struct {
	Feature          string
	OpID             string
	Method           string
	Path             string
	CollectionPath   string
	Mutates          []string
	HasAuthorization bool
}

var funcMap = template.FuncMap{
	"titleCase": titleCase,
	"camelCase": camelCase,
	"tableName": tableName,
}

// Generate produces Vitest contract test files for every operation in
// the contract file and writes them under
// <targetDir>/tests/contracts/<feature>/<opID>.test.ts.
//
// It returns the list of absolute paths that were written. If the
// contract file is empty, it returns nil, nil.
func Generate(cf *ContractFile, targetDir string) ([]string, error) {
	if cf == nil || cf.IsEmpty() {
		return nil, nil
	}

	tmpl, err := loadTemplates()
	if err != nil {
		return nil, err
	}

	var written []string

	// Stable iteration for deterministic output.
	features := make([]string, 0, len(cf.Features))
	for f := range cf.Features {
		features = append(features, f)
	}
	sort.Strings(features)

	for _, feature := range features {
		ops := cf.Features[feature]
		hasPOST, hasLifecyclePeer := featureLifecycleShape(ops)

		for _, op := range ops {
			if op.ID == "" || op.Method == "" || op.Path == "" {
				continue
			}
			data := templateData{
				Feature:          sanitize(feature),
				OpID:             op.ID,
				Method:           strings.ToUpper(op.Method),
				Path:             op.Path,
				CollectionPath:   collectionPath(op.Path),
				Mutates:          op.Mutates,
				HasAuthorization: op.Authorization != "",
			}

			var buf strings.Builder
			if err := tmpl.ExecuteTemplate(&buf, "header.tmpl", data); err != nil {
				return nil, fmt.Errorf("render header for %s: %w", op.ID, err)
			}

			// Decide which blocks apply.
			includeRoundtrip := op.MustPersist || len(op.Mutates) > 0
			includeMutation := len(op.Mutates) > 0
			includeNegative := op.IsMutating()
			// Concurrency applies to mutating ops that must persist (proves no
			// silent double-writes) and to read-only GETs that hit the DB
			// (proves no race in the read path).
			includeConcurrency := op.MustPersist || (strings.ToUpper(op.Method) == "GET" && op.ReadsFromDB)
			includeLifecycle := hasPOST && hasLifecyclePeer && op.IsMutating()

			if includeRoundtrip {
				if err := tmpl.ExecuteTemplate(&buf, "roundtrip.tmpl", data); err != nil {
					return nil, fmt.Errorf("render roundtrip for %s: %w", op.ID, err)
				}
			}
			if includeMutation {
				if err := tmpl.ExecuteTemplate(&buf, "mutation.tmpl", data); err != nil {
					return nil, fmt.Errorf("render mutation for %s: %w", op.ID, err)
				}
			}
			if includeLifecycle {
				if err := tmpl.ExecuteTemplate(&buf, "lifecycle.tmpl", data); err != nil {
					return nil, fmt.Errorf("render lifecycle for %s: %w", op.ID, err)
				}
			}
			if includeNegative {
				if err := tmpl.ExecuteTemplate(&buf, "negative.tmpl", data); err != nil {
					return nil, fmt.Errorf("render negative for %s: %w", op.ID, err)
				}
			}
			if includeConcurrency {
				if err := tmpl.ExecuteTemplate(&buf, "concurrency.tmpl", data); err != nil {
					return nil, fmt.Errorf("render concurrency for %s: %w", op.ID, err)
				}
			}

			if err := tmpl.ExecuteTemplate(&buf, "footer.tmpl", data); err != nil {
				return nil, fmt.Errorf("render footer for %s: %w", op.ID, err)
			}

			outDir := filepath.Join(targetDir, "tests", "contracts", sanitizeSegment(feature))
			if err := os.MkdirAll(outDir, 0o755); err != nil {
				return nil, fmt.Errorf("mkdir %s: %w", outDir, err)
			}
			outPath := filepath.Join(outDir, sanitizeSegment(op.ID)+".test.ts")
			if err := os.WriteFile(outPath, []byte(buf.String()), 0o644); err != nil {
				return nil, fmt.Errorf("write %s: %w", outPath, err)
			}
			written = append(written, outPath)
		}
	}

	sort.Strings(written)
	return written, nil
}

func loadTemplates() (*template.Template, error) {
	tmpl := template.New("contracts").Funcs(funcMap)
	entries, err := templateFS.ReadDir("templates")
	if err != nil {
		return nil, fmt.Errorf("read embedded templates: %w", err)
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".tmpl") {
			continue
		}
		b, err := templateFS.ReadFile("templates/" + e.Name())
		if err != nil {
			return nil, fmt.Errorf("read template %s: %w", e.Name(), err)
		}
		if _, err := tmpl.New(e.Name()).Parse(string(b)); err != nil {
			return nil, fmt.Errorf("parse template %s: %w", e.Name(), err)
		}
	}
	return tmpl, nil
}

// featureLifecycleShape inspects a feature's operations and returns
// (hasCreate, hasReadOrMutatePeer).
func featureLifecycleShape(ops []Operation) (bool, bool) {
	var hasPOST, hasPeer bool
	for _, op := range ops {
		switch strings.ToUpper(op.Method) {
		case "POST":
			hasPOST = true
		case "GET", "PUT", "PATCH", "DELETE":
			hasPeer = true
		}
	}
	return hasPOST, hasPeer
}

// collectionPath strips a trailing `/:id` or `/{id}` from a path so it
// can be used as a collection URL in the lifecycle template.
func collectionPath(p string) string {
	p = strings.TrimRight(p, "/")
	// Strip trailing dynamic segment(s).
	for {
		idx := strings.LastIndex(p, "/")
		if idx < 0 {
			return p
		}
		last := p[idx+1:]
		if len(last) > 0 && (last[0] == ':' || last[0] == '{') {
			p = p[:idx]
			continue
		}
		return p
	}
}

// titleCase converts "listings" / "current_high_bid" to "Listings" /
// "CurrentHighBid", suitable for use in a TypeScript identifier.
func titleCase(s string) string {
	parts := splitIdent(s)
	for i, p := range parts {
		if p == "" {
			continue
		}
		r := []rune(p)
		r[0] = unicode.ToUpper(r[0])
		for j := 1; j < len(r); j++ {
			r[j] = unicode.ToLower(r[j])
		}
		parts[i] = string(r)
	}
	return strings.Join(parts, "")
}

// camelCase converts "current_high_bid" to "currentHighBid".
func camelCase(s string) string {
	t := titleCase(s)
	if t == "" {
		return t
	}
	r := []rune(t)
	r[0] = unicode.ToLower(r[0])
	return string(r)
}

// tableName extracts a Drizzle table identifier from a mutates target
// such as "listings.current_high_bid" → "listings". If no dot is
// present the value is returned unchanged.
func tableName(s string) string {
	if i := strings.Index(s, "."); i >= 0 {
		return s[:i]
	}
	return s
}

func splitIdent(s string) []string {
	return strings.FieldsFunc(s, func(r rune) bool {
		return r == '_' || r == '-' || r == '.' || r == '/' || unicode.IsSpace(r)
	})
}

// sanitize produces a version of the string that is safe to embed in a
// TS describe block (no newlines or backticks).
func sanitize(s string) string {
	s = strings.ReplaceAll(s, "`", "'")
	s = strings.ReplaceAll(s, "\n", " ")
	return s
}

// sanitizeSegment produces a version of the string that is safe to use
// as a filesystem segment.
func sanitizeSegment(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '_', r == '-':
			b.WriteRune(r)
		default:
			b.WriteRune('_')
		}
	}
	out := b.String()
	if out == "" {
		return "op"
	}
	return out
}
