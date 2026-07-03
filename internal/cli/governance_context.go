package cli

// gatherRepoContext builds a bounded map of the repository so deep triage can
// reason about which real files a change would touch ("affects this and that")
// instead of producing generic strategy prose. It is deliberately lightweight:
// the tracked-file list (respecting .gitignore) plus a per-top-dir count and the
// heads of a few key files. This is the first-cut of feeding real code context
// to the planner; a richer version would rank files by relevance to the intent.

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

// stopwords are common words dropped from intent keyword extraction.
var stopwords = map[string]bool{
	"without": true, "should": true, "could": true, "would": true, "before": true,
	"after": true, "which": true, "there": true, "their": true, "when": true,
	"then": true, "this": true, "that": true, "with": true, "from": true, "into": true,
	"your": true, "you": true, "the": true, "and": true, "for": true, "not": true,
	"can": true, "will": true, "must": true, "have": true, "does": true,
}

// extractKeywords pulls meaningful terms from a change's intent for relevance
// ranking: lowercased alnum tokens of length >= 4 that aren't stopwords.
func extractKeywords(intent string) []string {
	seen := map[string]bool{}
	var kws []string
	for _, tok := range strings.FieldsFunc(strings.ToLower(intent), func(r rune) bool {
		return !(r >= 'a' && r <= 'z') && !(r >= '0' && r <= '9')
	}) {
		if len(tok) < 4 || stopwords[tok] || seen[tok] {
			continue
		}
		seen[tok] = true
		kws = append(kws, tok)
		if len(kws) >= 10 {
			break
		}
	}
	return kws
}

// gatherRelevantFiles ranks the repo's tracked files by how many intent keywords
// they match (via git grep, plus a filename-match bonus) and returns the heads
// of the top files. This gives the planner and the impact analysis ACTUAL code
// of the likely-affected files, so review can name exact files. Falls back to
// the repo map when nothing matches or there are no keywords.
func gatherRelevantFiles(codeDir, intent string, maxFiles, maxBytes int) string {
	kws := extractKeywords(intent)
	if len(kws) == 0 {
		return gatherRepoContext(codeDir, maxBytes)
	}
	scores := map[string]int{}
	for _, kw := range kws {
		out, err := exec.Command("git", "-C", codeDir, "grep", "-il", "-e", kw).Output()
		if err != nil {
			continue // no match → git grep exits 1
		}
		for _, f := range strings.Split(strings.TrimSpace(string(out)), "\n") {
			if f == "" {
				continue
			}
			scores[f]++
			if strings.Contains(strings.ToLower(f), kw) {
				scores[f] += 2 // filename match is a strong signal
			}
		}
	}
	if len(scores) == 0 {
		return gatherRepoContext(codeDir, maxBytes)
	}
	type fscore struct {
		file  string
		score int
	}
	ranked := make([]fscore, 0, len(scores))
	for f, sc := range scores {
		ranked = append(ranked, fscore{f, sc})
	}
	sort.Slice(ranked, func(i, j int) bool {
		if ranked[i].score != ranked[j].score {
			return ranked[i].score > ranked[j].score
		}
		return ranked[i].file < ranked[j].file
	})

	var b strings.Builder
	b.WriteString("## Relevant repository files (ranked by the change intent)\n")
	for i, r := range ranked {
		if i >= maxFiles || b.Len() > maxBytes {
			break
		}
		data, err := os.ReadFile(filepath.Join(codeDir, r.file))
		if err != nil {
			continue
		}
		head := string(data)
		if len(head) > 2000 {
			head = head[:2000] + "\n…(truncated)"
		}
		fmt.Fprintf(&b, "\n== %s ==\n%s\n", r.file, head)
	}
	return b.String()
}

// gatherRepoContext returns a repo map for baseDir, capped near maxBytes. Empty
// string if baseDir is not a git repo.
func gatherRepoContext(baseDir string, maxBytes int) string {
	out, err := exec.Command("git", "-C", baseDir, "ls-files").Output()
	if err != nil {
		return ""
	}
	files := strings.FieldsFunc(string(out), func(r rune) bool { return r == '\n' })
	if len(files) == 0 {
		return ""
	}

	// Per-top-directory counts, so a large monorepo is summarized rather than
	// dumped wholesale.
	counts := map[string]int{}
	for _, f := range files {
		top := f
		if i := strings.IndexByte(f, '/'); i >= 0 {
			top = f[:i] + "/"
		}
		counts[top]++
	}
	tops := make([]string, 0, len(counts))
	for k := range counts {
		tops = append(tops, k)
	}
	sort.Slice(tops, func(i, j int) bool { return counts[tops[i]] > counts[tops[j]] })

	var b strings.Builder
	b.WriteString("## Repository map\n\nTop-level areas (by tracked-file count):\n")
	for _, t := range tops {
		fmt.Fprintf(&b, "- %s (%d files)\n", t, counts[t])
	}

	// A bounded slice of the actual file paths so the planner can name real
	// files. Prefer source-ish paths; cap by count and bytes.
	b.WriteString("\nTracked files (truncated):\n")
	shown := 0
	for _, f := range files {
		if shown >= 500 || b.Len() > maxBytes {
			fmt.Fprintf(&b, "... (%d more)\n", len(files)-shown)
			break
		}
		b.WriteString(f)
		b.WriteByte('\n')
		shown++
	}

	// Heads of a few key files to convey stack/entry points.
	for _, key := range []string{"README.md", "package.json", "code/package.json", "code/frontend-sveltekit/package.json"} {
		if b.Len() > maxBytes {
			break
		}
		data, err := os.ReadFile(filepath.Join(baseDir, key))
		if err != nil {
			continue
		}
		head := string(data)
		if len(head) > 1200 {
			head = head[:1200] + "\n…(truncated)"
		}
		fmt.Fprintf(&b, "\n## %s\n```\n%s\n```\n", key, head)
	}
	return b.String()
}
