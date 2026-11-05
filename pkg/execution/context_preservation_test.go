package execution

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/openexec/openexec/pkg/agent"
)

// Every preserved class: not changed, no result stubbed, request bytes equal.
func requireByteIdentical(t *testing.T, name string, request agent.Request, scans map[[2]int]string, listingsTrusted bool) {
	t.Helper()
	before, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	compacted, changed := (priorResults{}).compactWith(request, scans, listingsTrusted)
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
	requireByteIdentical(t, "system and owner instructions", owner, nil, true)

	requireByteIdentical(t, "file reads including AGENTS.md",
		structuralRequest([3]string{"read_file", agents}, [3]string{"read_file", agents + "2"}, [3]string{"read_file", agents + "3"}), nil, true)
	requireByteIdentical(t, "unknown tools",
		structuralRequest([3]string{"mystery", agents}, [3]string{"mystery", agents + "2"}), nil, true)

	failed := structuralRequest([3]string{"list_directory", agents}, [3]string{"list_directory", agents + "2"}, [3]string{"list_directory", agents + "3"})
	failed.Messages[2].Content[0].ToolError = "denied"
	failed.Messages[4].Content[0].ToolError = "denied"
	requireByteIdentical(t, "failed-tool results", failed, nil, true)

	// A listing from an executor that does not vouch for listings.
	untrusted := structuralRequest([3]string{"list_directory", agents}, [3]string{"list_directory", agents + "2"})
	requireByteIdentical(t, "unvouched listing", untrusted, nil, false)

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
	meter := &boundedAPIAdapter{}
	padded := output + strings.Repeat("pad ", 100)
	incomplete := structuralRequest([3]string{"search_files", padded}, [3]string{"search_files", padded + "2"})
	requireByteIdentical(t, "unmarked search", incomplete, nil, true)
	requireByteIdentical(t, "real incomplete scan", incomplete, meter.completeScans, true)

	// Near miss: the same shapes ARE omitted when every fact holds, so the
	// assertions above are decided by the facts and not by the helper.
	ok := structuralRequest([3]string{"list_directory", agents}, [3]string{"list_directory", agents + "2"})
	if _, changed := (priorResults{}).compactWith(ok, nil, true); !changed {
		t.Fatal("vouched listing should be eligible")
	}
}
