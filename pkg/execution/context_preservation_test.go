package execution

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/openexec/openexec/pkg/agent"
)

// Every preserved class: not changed, no result stubbed, request bytes equal.
func requireByteIdentical(t *testing.T, name string, request agent.Request, scans map[[2]int]string) {
	t.Helper()
	before, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	compacted, changed := (priorResults{}).compactWith(request, scans)
	after, _ := json.Marshal(compacted)
	if changed || string(before) != string(after) {
		t.Fatalf("%s: changed=%v, request not byte-identical", name, changed)
	}
	for i, m := range compacted.Messages {
		for j, b := range m.Content {
			if b.Type == agent.ContentTypeToolResult && strings.Contains(b.ToolOutput, recallTool) {
				t.Fatalf("%s: result %d/%d was stubbed", name, i, j)
			}
		}
	}
}

func TestOmissionPreservationClasses(t *testing.T) {
	agents := "AGENTS.md: NEVER DEPLOY without review. " + strings.Repeat("authority ", 100)
	owner := agent.Request{System: agents, Messages: []agent.Message{
		agent.NewTextMessage(agent.RoleUser, agents), agent.NewTextMessage(agent.RoleAssistant, agents), agent.NewTextMessage(agent.RoleUser, agents)}}
	requireByteIdentical(t, "system and owner instructions", owner, nil)

	reads := structuralRequest([3]string{"read_file", agents}, [3]string{"read_file", agents + "2"}, [3]string{"read_file", agents + "3"})
	for i := range reads.Messages {
		for j := range reads.Messages[i].Content {
			if reads.Messages[i].Content[j].Type == agent.ContentTypeToolUse {
				reads.Messages[i].Content[j].ToolInput = json.RawMessage(`{"path":"AGENTS.md"}`)
			}
		}
	}
	// Even if every read were recorded as complete, a file read is not eligible.
	allRecorded := recordFor(map[int]string{2: agents, 4: agents + "2", 6: agents + "3"})
	requireByteIdentical(t, "file reads including AGENTS.md", reads, allRecorded)
	requireByteIdentical(t, "file reads without provenance", reads, nil)
	requireByteIdentical(t, "unknown tools",
		structuralRequest([3]string{"mystery", agents}, [3]string{"mystery", agents + "2"}), recordFor(map[int]string{2: agents, 4: agents + "2"}))

	failed := structuralRequest([3]string{"list_directory", agents}, [3]string{"list_directory", agents + "2"}, [3]string{"list_directory", agents + "3"})
	failed.Messages[2].Content[0].ToolError = "denied"
	failed.Messages[4].Content[0].ToolError = "denied"
	requireByteIdentical(t, "failed-tool results", failed, recordFor(map[int]string{2: agents, 4: agents + "2", 6: agents + "3"}))
	failedMeter := &boundedAPIAdapter{trustedDirectoryResults: true}
	failedMeter.recordProvenance("list_directory", 2, agents, false, fmt.Errorf("denied"))
	if len(failedMeter.completeScans) != 0 {
		t.Fatal("a failed listing was recorded")
	}

	// An unmarked listing: right tool, paired, no error, but no provenance.
	unmarked := structuralRequest([3]string{"list_directory", agents}, [3]string{"list_directory", agents + "2"})
	requireByteIdentical(t, "unmarked listing", unmarked, nil)
	// Provenance for a different position or different text does not transfer.
	requireByteIdentical(t, "listing provenance at wrong position", unmarked, recordFor(map[int]string{1: agents, 4: agents + "2"}))
	requireByteIdentical(t, "listing provenance for other text", unmarked, recordFor(map[int]string{2: "other", 4: "other2"}))

	// Unmarked search, and a real scan that ended incomplete: only a complete
	// scan is ever recorded, so the incomplete one has no provenance at all.
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "long.txt"), []byte("needle "+strings.Repeat("y", 2000)+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	output, complete, err := NewWorkspaceToolExecutor().searchFilesWithProvenance(context.Background(), searchRequest(root, "long.txt", "needle"))
	if err != nil || complete {
		t.Fatalf("setup: expected a real incomplete scan, complete=%v err=%v", complete, err)
	}
	meter := &boundedAPIAdapter{trustedDirectoryResults: true}
	meter.recordProvenance("search_files", 2, output, complete, nil)
	meter.recordProvenance("search_files", 4, output, complete, nil)
	if len(meter.completeScans) != 0 {
		t.Fatal("an incomplete scan was recorded")
	}
	padded := output + strings.Repeat("pad ", 100)
	meter.recordProvenance("search_files", 2, padded, complete, nil)
	meter.recordProvenance("search_files", 4, padded+"2", complete, nil)
	incomplete := structuralRequest([3]string{"search_files", padded}, [3]string{"search_files", padded + "2"})
	requireByteIdentical(t, "unmarked search", incomplete, nil)
	requireByteIdentical(t, "real incomplete scan", incomplete, meter.completeScans)

	// Near miss: the same shapes ARE omitted when every fact holds, so the
	// assertions above are decided by the facts and not by the helper.
	ok := structuralRequest([3]string{"list_directory", agents}, [3]string{"list_directory", agents + "2"})
	if _, changed := (priorResults{}).compactWith(ok, recordFor(map[int]string{2: agents, 4: agents + "2"})); !changed {
		t.Fatal("recorded listing should be eligible")
	}
}

// The adapter records a listing only when the executor vouches for listings.
func TestListingProvenanceComesFromVouchingExecutor(t *testing.T) {
	text := strings.Repeat("name\n", 100)
	vouched := &boundedAPIAdapter{trustedDirectoryResults: true}
	vouched.recordProvenance("list_directory", 2, text, false, nil)
	if vouched.completeScans[[2]int{2, 0}] != sha256Hex(text) {
		t.Fatal("vouched listing was not recorded")
	}
	for _, tool := range []string{"read_file", "mystery", "search_files"} {
		other := &boundedAPIAdapter{trustedDirectoryResults: true}
		other.recordProvenance(tool, 2, text, false, nil)
		if len(other.completeScans) != 0 {
			t.Fatalf("%s result was recorded without scan completeness", tool)
		}
	}
	unvouched := &boundedAPIAdapter{}
	unvouched.recordProvenance("list_directory", 2, text, true, nil)
	if len(unvouched.completeScans) != 0 {
		t.Fatal("unvouched listing was recorded")
	}
}

// compact is the test view of a run whose executor recorded every well-formed
// list_directory result, as the adapter does. Search results are never
// recorded here, and production code never calls it.
func (p priorResults) compact(r agent.Request) (agent.Request, bool) {
	names := map[string]string{}
	scans := map[[2]int]string{}
	for i, m := range r.Messages {
		for j, b := range m.Content {
			if b.Type == agent.ContentTypeToolUse {
				names[b.ToolUseID] = b.ToolName
			}
			if b.Type == agent.ContentTypeToolResult && names[b.ToolResultID] == "list_directory" {
				scans[[2]int{i, j}] = sha256Hex(b.ToolOutput)
			}
		}
	}
	return p.compactWith(r, scans)
}
