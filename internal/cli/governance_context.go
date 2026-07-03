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
