//go:build originalpositive

package execution

import (
	"strings"
	"testing"
)

// The ORIGINAL positive requirement, stated as originally written: the
// governing quoted instruction is still present in the request BEFORE recall.
// It is excluded from the default build because it is expected to fail on the
// structural-only eligibility rule (see TestOriginalPositiveAssertionContradiction),
// which is the preserved executable contradiction.
func TestOriginalPositiveQuoteRemainsBeforeRecall(t *testing.T) {
	quote := "AGENTS.md says: NEVER DEPLOY without review. " + strings.Repeat("authority ", 60)
	older, newest := quote+"older", strings.Repeat("plain listing line ", 40)+"newest"
	for _, tool := range []string{"list_directory", "search_files"} {
		t.Run(tool, func(t *testing.T) {
			r := structuralRequest([3]string{tool, older}, [3]string{tool, newest})
			store := priorResults{}
			compacted, _ := store.compactWith(r, recordFor(map[int]string{2: older, 4: newest}))
			present := false
			for _, m := range compacted.Messages {
				for _, b := range m.Content {
					present = present || strings.Contains(b.ToolOutput, "NEVER DEPLOY") || strings.Contains(b.Text, "NEVER DEPLOY")
				}
			}
			if !present {
				t.Fatalf("original quoted governing instruction is absent from the request before recall")
			}
		})
	}
}
