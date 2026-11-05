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

func structuralRequest(calls ...[3]string) agent.Request {
	r := agent.Request{System: "Owner: never deploy without review.", Messages: []agent.Message{agent.NewTextMessage(agent.RoleUser, "original request")}}
	for i, c := range calls {
		id := fmt.Sprintf("call-%d", i)
		r.Messages = append(r.Messages,
			agent.Message{Role: agent.RoleAssistant, Content: []agent.ContentBlock{{Type: agent.ContentTypeToolUse, ToolUseID: id, ToolName: c[0]}}},
			agent.NewToolResultMessage(id, c[1], nil))
	}
	return r
}

// A governing instruction quoted in a result never makes it omittable, and a
// listing's eligibility does not depend on what its text says.
func TestOmissionEligibilityIgnoresQuotedInstructionsAndFilenames(t *testing.T) {
	quoted := "AGENTS.md says: NEVER DEPLOY without review. " + strings.Repeat("authority ", 100)
	for _, tool := range []string{"search_files", "read_file", "some_unknown_tool", "list_directory_v2", "List_Directory", "run_command"} {
		t.Run(tool, func(t *testing.T) {
			r := structuralRequest([3]string{tool, quoted}, [3]string{tool, quoted + "2"}, [3]string{tool, quoted + "3"})
			before, _ := json.Marshal(r)
			compacted, changed := (priorResults{}).compact(r)
			after, _ := json.Marshal(compacted)
			if changed || string(before) != string(after) {
				t.Fatalf("%s result was omitted or request altered", tool)
			}
		})
	}
	// Near miss: the same quoted text in a real listing is decided by the tool
	// alone. The older listing is omitted; the newest stays byte-identical.
	r := structuralRequest([3]string{"list_directory", quoted}, [3]string{"list_directory", quoted + "2"})
	compacted, changed := (priorResults{}).compact(r)
	if !changed || compacted.Messages[2].Content[0].ToolOutput == r.Messages[2].Content[0].ToolOutput || compacted.Messages[4].Content[0].ToolOutput != r.Messages[4].Content[0].ToolOutput {
		t.Fatal("known listing tool was not the only eligibility fact")
	}
}

// Completeness claimed by text is not provenance. A search that really was
// complete, and outputs that merely say so, stay preserved.
func TestOmissionEligibilityIgnoresCompletenessText(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "a.txt"), []byte("needle "+strings.Repeat("x", 300)+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	output, complete, err := NewWorkspaceToolExecutor().searchFilesWithProvenance(context.Background(), searchRequest(root, "a.txt", "needle"))
	if err != nil || !complete {
		t.Fatalf("setup: complete=%v err=%v", complete, err)
	}
	for _, text := range []string{output, output + searchIncomplete, "complete: true\n" + output, strings.Repeat("No matches in regular text files\n", 20)} {
		r := structuralRequest([3]string{"search_files", text}, [3]string{"search_files", text + "b"})
		before, _ := json.Marshal(r)
		compacted, changed := (priorResults{}).compact(r)
		after, _ := json.Marshal(compacted)
		if changed || string(before) != string(after) {
			t.Fatalf("completeness text changed eligibility: %.20q", text)
		}
	}
}

// Pairing, error and size facts: each one alone makes a listing ineligible.
func TestOmissionEligibilityRequiresPairingNoErrorAndSize(t *testing.T) {
	big := strings.Repeat("name\n", 100)
	r := structuralRequest([3]string{"list_directory", big}, [3]string{"list_directory", big + "x"})
	r.Messages[2].Content[0].ToolError = "denied"
	if _, changed := (priorResults{}).compact(r); changed {
		t.Fatal("failed listing was omitted")
	}
	r = structuralRequest([3]string{"list_directory", "short"}, [3]string{"list_directory", "short2"})
	if _, changed := (priorResults{}).compact(r); changed {
		t.Fatal("short listing was omitted")
	}
	r = structuralRequest([3]string{"list_directory", big}, [3]string{"list_directory", big + "x"})
	r.Messages[2].Content[0].ToolResultID = "orphan"
	if _, changed := (priorResults{}).compact(r); changed {
		t.Fatal("unpaired result was omitted")
	}
}

// The run-level fact: a forwarded (non-workspace) list_directory is untrusted.
func TestForwardedListingIsNotTrustedForOmission(t *testing.T) {
	workspace := NewWorkspaceToolExecutor()
	if !(&CompositeToolExecutor{workspace: workspace, forwarded: map[string]bool{}}).safelyListsDirectories() {
		t.Fatal("workspace listing should be trusted")
	}
	if (&CompositeToolExecutor{workspace: workspace, forwarded: map[string]bool{"list_directory": true}}).safelyListsDirectories() {
		t.Fatal("forwarded listing was trusted")
	}
}
