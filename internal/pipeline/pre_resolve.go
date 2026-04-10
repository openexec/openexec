package pipeline

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"unicode"
)

// DefaultSliceLines is the maximum number of source lines to include per
// resolved symbol (signature + first N lines of body).
const DefaultSliceLines = 20

// PreResolver looks up likely symbol references from a task description in the
// knowledge.Store symbols table and returns a formatted context block containing
// the signature and first few lines of each matched symbol's body.
type PreResolver struct {
	SliceLines int // max source lines per symbol; 0 → DefaultSliceLines
}

// symbolRow mirrors a row from the symbols table.
type symbolRow struct {
	Name      string
	Kind      string
	FilePath  string
	StartLine int
	EndLine   int
	Signature string
}

// Resolve extracts candidate symbol names from taskDescription, queries db for
// matches, reads the relevant source slices from disk, and returns a formatted
// context block. Returns "" if no symbols matched or the table does not exist.
func (pr *PreResolver) Resolve(ctx context.Context, taskDescription, workDir string, db *sql.DB) string {
	if db == nil || taskDescription == "" {
		return ""
	}

	candidates := extractCandidateNames(taskDescription)
	if len(candidates) == 0 {
		return ""
	}

	sliceLines := pr.SliceLines
	if sliceLines <= 0 {
		sliceLines = DefaultSliceLines
	}

	// Check that the symbols table exists before querying.
	if !tableExists(db, "symbols") {
		log.Printf("[PreResolver] symbols table does not exist yet; skipping pre-resolution")
		return ""
	}

	var matches []symbolRow
	for _, name := range candidates {
		rows, err := db.QueryContext(ctx,
			`SELECT name, kind, file_path, start_line, end_line, signature FROM symbols WHERE name = ?`,
			name,
		)
		if err != nil {
			// Table might not exist or other transient error — fail gracefully.
			log.Printf("[PreResolver] query error for %q: %v", name, err)
			continue
		}
		for rows.Next() {
			var r symbolRow
			if err := rows.Scan(&r.Name, &r.Kind, &r.FilePath, &r.StartLine, &r.EndLine, &r.Signature); err != nil {
				continue
			}
			matches = append(matches, r)
		}
		rows.Close()
	}

	log.Printf("[PreResolver] Resolved %d symbols from task description (%d candidates checked)", len(matches), len(candidates))

	if len(matches) == 0 {
		return ""
	}

	var buf strings.Builder
	buf.WriteString(fmt.Sprintf("--- Pre-Resolved Symbols (%d matches) ---\n", len(matches)))

	for _, m := range matches {
		// Build the absolute path; stored file_path may already be absolute or
		// relative to workDir.
		absPath := m.FilePath
		if !filepath.IsAbs(absPath) {
			absPath = filepath.Join(workDir, absPath)
		}
		relPath := m.FilePath
		if filepath.IsAbs(relPath) {
			if r, err := filepath.Rel(workDir, relPath); err == nil {
				relPath = r
			}
		}

		slice := readFileSlice(absPath, m.StartLine, m.EndLine, sliceLines)

		lang := langFromPath(relPath)
		buf.WriteString(fmt.Sprintf("\n### %s (%s) at %s:%d\n", m.Name, m.Kind, relPath, m.StartLine))
		buf.WriteString(fmt.Sprintf("```%s\n", lang))
		buf.WriteString(slice)
		if !strings.HasSuffix(slice, "\n") {
			buf.WriteString("\n")
		}
		buf.WriteString("```\n")
	}

	buf.WriteString("---\n")
	return buf.String()
}

// extractCandidateNames tokenises a task description into deduplicated
// identifier-like tokens (length >= 3, starts with letter/underscore).
func extractCandidateNames(desc string) []string {
	seen := make(map[string]struct{})
	var result []string

	add := func(tok string) {
		if _, ok := seen[tok]; ok {
			return
		}
		if isIdentifier(tok) && len(tok) >= 3 {
			seen[tok] = struct{}{}
			result = append(result, tok)
		}
	}

	// Split on whitespace and common separators.
	tokens := splitOnSeparators(desc)
	for _, tok := range tokens {
		add(tok)
		// Also extract PascalCase / camelCase sub-words.
		for _, sub := range splitCamelCase(tok) {
			add(sub)
		}
	}

	return result
}

// splitOnSeparators splits s on whitespace and punctuation commonly found in
// task descriptions (commas, periods, parentheses, quotes, backticks, etc.)
// while preserving underscores within tokens.
func splitOnSeparators(s string) []string {
	return strings.FieldsFunc(s, func(r rune) bool {
		if r == '_' {
			return false // keep underscores inside identifiers
		}
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
}

// splitCamelCase breaks a PascalCase or camelCase token into its sub-words.
// e.g. "RunBlueprintMode" → ["Run", "Blueprint", "Mode", "RunBlueprintMode"]
func splitCamelCase(s string) []string {
	if len(s) < 4 {
		return nil
	}
	var parts []string
	start := 0
	runes := []rune(s)
	for i := 1; i < len(runes); i++ {
		if unicode.IsUpper(runes[i]) && (i+1 < len(runes) && unicode.IsLower(runes[i+1]) || unicode.IsLower(runes[i-1])) {
			part := string(runes[start:i])
			if len(part) >= 3 {
				parts = append(parts, part)
			}
			start = i
		}
	}
	// Last segment.
	if start > 0 && start < len(runes) {
		part := string(runes[start:])
		if len(part) >= 3 {
			parts = append(parts, part)
		}
	}
	return parts
}

// isIdentifier reports whether s looks like a programming identifier.
func isIdentifier(s string) bool {
	if len(s) == 0 {
		return false
	}
	first := rune(s[0])
	if !unicode.IsLetter(first) && first != '_' {
		return false
	}
	for _, r := range s[1:] {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '_' {
			return false
		}
	}
	return true
}

// readFileSlice reads lines [startLine, min(startLine+sliceLines, endLine)]
// from the file at absPath. Line numbers are 1-based.
func readFileSlice(absPath string, startLine, endLine, sliceLines int) string {
	data, err := os.ReadFile(absPath)
	if err != nil {
		return fmt.Sprintf("// error reading file: %v", err)
	}
	lines := strings.Split(string(data), "\n")

	// Convert to 0-based indices.
	start := startLine - 1
	if start < 0 {
		start = 0
	}
	end := start + sliceLines
	if endLine > 0 && endLine < start+sliceLines+1 {
		end = endLine // endLine is 1-based inclusive; using as 0-based exclusive works
	}
	if end > len(lines) {
		end = len(lines)
	}

	return strings.Join(lines[start:end], "\n")
}

// tableExists checks whether a table exists in the database.
func tableExists(db *sql.DB, name string) bool {
	var n int
	err := db.QueryRow(
		`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?`, name,
	).Scan(&n)
	return err == nil && n > 0
}

// langFromPath returns a markdown code-fence language hint from a file extension.
func langFromPath(path string) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".py":
		return "python"
	case ".go":
		return "go"
	case ".js":
		return "javascript"
	case ".ts":
		return "typescript"
	case ".rs":
		return "rust"
	case ".java":
		return "java"
	case ".rb":
		return "ruby"
	case ".c", ".h":
		return "c"
	case ".cpp", ".hpp", ".cc":
		return "cpp"
	default:
		return ""
	}
}
