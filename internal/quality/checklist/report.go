package checklist

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
)

// WriteJSON writes findings as a JSON document to w.
func WriteJSON(findings []Finding, w io.Writer) error {
	payload := struct {
		Summary  map[string]int `json:"summary"`
		Total    int            `json:"total"`
		Findings []Finding      `json:"findings"`
	}{
		Summary:  severityCounts(findings),
		Total:    len(findings),
		Findings: findings,
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(payload)
}

// WriteMarkdown writes a markdown report grouped by severity then file.
func WriteMarkdown(findings []Finding, w io.Writer) error {
	counts := severityCounts(findings)
	var sb strings.Builder
	sb.WriteString("# Production-readiness checklist report\n\n")
	sb.WriteString(fmt.Sprintf("**Total findings:** %d\n\n", len(findings)))
	sb.WriteString("| Severity | Count |\n|---|---|\n")
	for _, sev := range []Severity{SeverityHigh, SeverityWarn} {
		sb.WriteString(fmt.Sprintf("| %s | %d |\n", sev, counts[string(sev)]))
	}
	sb.WriteString("\n")

	if len(findings) == 0 {
		sb.WriteString("_No production-readiness issues detected._\n")
		_, err := io.WriteString(w, sb.String())
		return err
	}

	order := []Severity{SeverityHigh, SeverityWarn}
	bySev := map[Severity][]Finding{}
	for _, f := range findings {
		bySev[f.Severity] = append(bySev[f.Severity], f)
	}

	for _, sev := range order {
		group := bySev[sev]
		if len(group) == 0 {
			continue
		}
		sb.WriteString(fmt.Sprintf("## %s (%d)\n\n", strings.ToUpper(string(sev)), len(group)))
		byFile := map[string][]Finding{}
		for _, f := range group {
			byFile[f.File] = append(byFile[f.File], f)
		}
		files := make([]string, 0, len(byFile))
		for k := range byFile {
			files = append(files, k)
		}
		sort.Strings(files)
		for _, file := range files {
			display := file
			if display == "" {
				display = "(project)"
			}
			sb.WriteString(fmt.Sprintf("### `%s`\n\n", display))
			for _, f := range byFile[file] {
				if f.Line > 0 {
					sb.WriteString(fmt.Sprintf("- **L%d** `%s` — %s\n", f.Line, f.Check, f.Message))
				} else {
					sb.WriteString(fmt.Sprintf("- `%s` — %s\n", f.Check, f.Message))
				}
			}
			sb.WriteString("\n")
		}
	}

	_, err := io.WriteString(w, sb.String())
	return err
}
