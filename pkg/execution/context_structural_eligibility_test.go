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

// structuralRequest builds user text, then one assistant call and one result
// per entry ({tool, output, input}); every call is answered by a later
// assistant turn, so only the structural facts decide eligibility. A final
// assistant turn is appended for that reason.
func structuralRequest(scanComplete bool, calls ...[3]string) agent.Request {
	r := agent.Request{System: "Owner: never deploy without review.", Messages: []agent.Message{agent.NewTextMessage(agent.RoleUser, "original request")}}
	for i, c := range calls {
		id := fmt.Sprintf("call-%d", i)
		result := agent.NewToolResultMessage(id, c[1], nil)
		result.Content[0].ScanComplete = scanComplete
		input := json.RawMessage(c[2])
		if c[2] == "" {
			input = json.RawMessage(`{}`)
		}
		r.Messages = append(r.Messages,
			agent.Message{Role: agent.RoleAssistant, Content: []agent.ContentBlock{{Type: agent.ContentTypeToolUse, ToolUseID: id, ToolName: c[0], ToolInput: input}}},
			result)
	}
	r.Messages = append(r.Messages, agent.NewTextMessage(agent.RoleAssistant, "answered"))
	return r
}

func requireByteIdentical(t *testing.T, name string, request agent.Request, trustDirectories bool) {
	t.Helper()
	before, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	compacted, changed := (priorResults{}).compactWith(request, trustDirectories)
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

// Every preserved class: not changed, no result stubbed, request bytes equal.
func TestOmissionPreservationClasses(t *testing.T) {
	agents := "AGENTS.md: NEVER DEPLOY without review. " + strings.Repeat("authority ", 100)
	owner := agent.Request{System: agents, Messages: []agent.Message{
		agent.NewTextMessage(agent.RoleUser, agents), agent.NewTextMessage(agent.RoleAssistant, agents), agent.NewTextMessage(agent.RoleUser, agents)}}
	requireByteIdentical(t, "system and owner instructions", owner, true)

	// Even when a file read carries a scan mark, a file read is not eligible.
	reads := structuralRequest(true, [3]string{"read_file", agents, `{"path":"AGENTS.md"}`}, [3]string{"read_file", agents + "2", `{"path":"AGENTS.md"}`}, [3]string{"read_file", agents + "3", `{"path":"AGENTS.md"}`})
	requireByteIdentical(t, "file reads including AGENTS.md (marked)", reads, true)
	reads = structuralRequest(false, [3]string{"read_file", agents, `{"path":"AGENTS.md"}`}, [3]string{"read_file", agents + "2", `{"path":"AGENTS.md"}`})
	requireByteIdentical(t, "file reads including AGENTS.md", reads, true)
	requireByteIdentical(t, "unknown tools",
		structuralRequest(true, [3]string{"mystery", agents, ""}, [3]string{"mystery", agents + "2", ""}), true)

	for _, tool := range []string{"list_directory", "search_files"} {
		failed := structuralRequest(true, [3]string{tool, agents, ""}, [3]string{tool, agents + "2", ""}, [3]string{tool, agents + "3", ""})
		failed.Messages[2].Content[0].ToolError = "denied"
		failed.Messages[4].Content[0].ToolError = "denied"
		failed.Messages[6].Content[0].ToolError = "denied"
		requireByteIdentical(t, tool+" failed-tool results", failed, true)
	}

	// Right tool, paired, no error, but no scan mark: an unmarked search.
	requireByteIdentical(t, "unmarked search",
		structuralRequest(false, [3]string{"search_files", agents, ""}, [3]string{"search_files", agents + "2", ""}), true)
	// A directory listing from an executor that does not vouch for listings.
	requireByteIdentical(t, "unvouched listing",
		structuralRequest(false, [3]string{"list_directory", agents, ""}, [3]string{"list_directory", agents + "2", ""}), false)

	// A real scan that ended incomplete reports complete=false, and that is
	// the only thing the harness records onto the block.
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "long.txt"), []byte("needle "+strings.Repeat("y", 2000)+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	output, complete, err := NewWorkspaceToolExecutor().searchFilesWithProvenance(context.Background(), searchRequest(root, "long.txt", "needle"))
	if err != nil || complete {
		t.Fatalf("setup: expected a real incomplete scan, complete=%v err=%v", complete, err)
	}
	padded := output + strings.Repeat("pad ", 100)
	requireByteIdentical(t, "real incomplete scan",
		structuralRequest(complete, [3]string{"search_files", padded, ""}, [3]string{"search_files", padded + "2", ""}), true)

	// Near misses: the same shapes ARE omitted when every fact holds, so the
	// assertions above are decided by the facts and not by the helper.
	if _, changed := (priorResults{}).compactWith(structuralRequest(false, [3]string{"list_directory", agents, ""}, [3]string{"list_directory", agents + "2", ""}), true); !changed {
		t.Fatal("vouched listing should be eligible")
	}
	if _, changed := (priorResults{}).compactWith(structuralRequest(true, [3]string{"search_files", agents, ""}, [3]string{"search_files", agents + "2", ""}), false); !changed {
		t.Fatal("complete search should be eligible")
	}
}

// A governing instruction quoted in a result never makes it omittable, and
// eligibility does not depend on what the text or the tool's spelling says.
func TestOmissionEligibilityIgnoresQuotedInstructionsAndFilenames(t *testing.T) {
	quoted := "AGENTS.md says: NEVER DEPLOY without review. " + strings.Repeat("authority ", 100)
	for _, tool := range []string{"read_file", "some_unknown_tool", "list_directory_v2", "List_Directory", "Search_Files", "run_command"} {
		t.Run(tool, func(t *testing.T) {
			requireByteIdentical(t, tool, structuralRequest(true, [3]string{tool, quoted, ""}, [3]string{tool, quoted + "2", ""}, [3]string{tool, quoted + "3", ""}), true)
		})
	}
	// Near miss: the same quoted text in a real listing is decided by the tool
	// alone. The older listing is omitted; the newest stays byte-identical.
	r := structuralRequest(false, [3]string{"list_directory", quoted, ""}, [3]string{"list_directory", quoted + "2", ""})
	compacted, changed := (priorResults{}).compactWith(r, true)
	if !changed || compacted.Messages[2].Content[0].ToolOutput == r.Messages[2].Content[0].ToolOutput || compacted.Messages[4].Content[0].ToolOutput != r.Messages[4].Content[0].ToolOutput {
		t.Fatal("known listing tool was not the only eligibility fact")
	}
}

// Completeness claimed by text is not provenance: outputs that merely say a
// scan finished, or that a real complete scan once produced, stay preserved
// unless the harness itself marked the block.
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
		requireByteIdentical(t, fmt.Sprintf("%.20q", text),
			structuralRequest(false, [3]string{"search_files", text, ""}, [3]string{"search_files", text + "b", ""}), true)
	}
	// Near miss: the identical output with the harness mark is omitted.
	if _, changed := (priorResults{}).compactWith(structuralRequest(true, [3]string{"search_files", output + strings.Repeat(" pad", 80), ""}, [3]string{"search_files", output + "b", ""}), true); !changed {
		t.Fatal("marked complete search should be eligible")
	}
}

// Pairing, error, size and encoding facts: each one alone makes a listing
// ineligible.
func TestOmissionEligibilityRequiresPairingNoErrorSizeAndUTF8(t *testing.T) {
	big := strings.Repeat("name\n", 100)
	fresh := func() agent.Request {
		return structuralRequest(false, [3]string{"list_directory", big, ""}, [3]string{"list_directory", big + "x", ""})
	}
	r := fresh()
	r.Messages[2].Content[0].ToolError = "denied"
	requireByteIdentical(t, "failed listing", r, true)
	requireByteIdentical(t, "short listing",
		structuralRequest(false, [3]string{"list_directory", "short", ""}, [3]string{"list_directory", "short2", ""}), true)
	r = fresh()
	r.Messages[2].Content[0].ToolResultID = "orphan"
	requireByteIdentical(t, "unpaired result", r, true)
	requireByteIdentical(t, "invalid UTF-8 listing",
		structuralRequest(false, [3]string{"list_directory", big + "\xff", ""}, [3]string{"list_directory", big + "\xfe", ""}), true)
	if _, changed := (priorResults{}).compactWith(fresh(), true); !changed {
		t.Fatal("near miss: a well-formed listing should be eligible")
	}
}

// A search is omitted only when the harness marked it complete AND a later
// assistant turn answered it; a result the model has not yet seen stays.
func TestSearchOmissionRequiresMarkAndLaterAnswer(t *testing.T) {
	old := "older " + strings.Repeat("evidence ", 60)
	r := structuralRequest(true, [3]string{"search_files", old, `{"text":"needle"}`})
	compacted, changed := (priorResults{}).compactWith(r, false)
	if !changed || !strings.Contains(compacted.Messages[2].Content[0].ToolOutput, "NOT been inspected") {
		t.Fatal("marked, answered search should be omitted with a not-inspected notice")
	}
	unanswered := structuralRequest(true, [3]string{"search_files", old, `{"text":"needle"}`})
	unanswered.Messages = unanswered.Messages[:len(unanswered.Messages)-1]
	requireByteIdentical(t, "search the model has not yet answered", unanswered, false)
	requireByteIdentical(t, "unmarked search", structuralRequest(false, [3]string{"search_files", old, `{"text":"needle"}`}), false)
}

// Executable control for the one place "the original governing quote stays in
// the request" and "eligibility is structural only" are in tension: a quote
// that exists solely inside an older, structurally eligible result. The text
// is never read, so that result is stubbed and the quote is absent from the
// request until recalled. Every other carrier of the quote (system, user
// turns, file reads, the newest scan) is untouched, and the stubbed quote is
// recoverable byte-for-byte through paged recall. Neither assertion is
// weakened: the first pins the structural-only outcome through the original
// positive assertion, run for real; the second pins what recall returns.
func TestQuotedInstructionInEligibleOlderScanIsRecallableNotHeuristicallyKept(t *testing.T) {
	quote := "AGENTS.md says: NEVER DEPLOY without review. " + strings.Repeat("authority ", 60)
	older, newest := quote+"older", strings.Repeat("plain listing line ", 40)+"newest"
	for _, tool := range []string{"list_directory", "search_files"} {
		t.Run(tool, func(t *testing.T) {
			r := structuralRequest(true, [3]string{tool, older, ""}, [3]string{tool, newest, ""})
			store := priorResults{}
			compacted, changed := store.compactWith(r, true)
			if !changed {
				t.Fatal("a structurally eligible older result must be omitted whatever it quotes")
			}
			var rec recordingTB
			requireQuotePresentBeforeRecall(&rec, compacted, "NEVER DEPLOY without review")
			t.Logf("original positive assertion on compacted request: failed=%v msg=%q", rec.failed, rec.msg)
			if !rec.failed {
				t.Fatal("quote survived omission, so eligibility must have read content")
			}
			// Discriminating control: the same assertion PASSES on the
			// uncompacted request, so the failure above means absence.
			var ok recordingTB
			requireQuotePresentBeforeRecall(&ok, r, "NEVER DEPLOY without review")
			if ok.failed {
				t.Fatalf("positive assertion fails on the uncompacted request: %s", ok.msg)
			}
			if compacted.System != r.System || compacted.Messages[0].Content[0].Text != "original request" {
				t.Fatal("system or user turn was altered")
			}
			if tool == "list_directory" && compacted.Messages[4].Content[0].ToolOutput != newest {
				t.Fatal("newest listing was altered")
			}
			recalled := false
			for id := range store {
				raw, _ := json.Marshal(map[string]any{"id": id, "offset": 0})
				page, err := store.read(raw)
				if err != nil {
					t.Fatalf("recall failed: %v", err)
				}
				recalled = recalled || strings.Contains(page, "NEVER DEPLOY without review")
			}
			if !recalled {
				t.Fatal("quote not recoverable by recall")
			}
		})
	}
}

// recordingTB captures a failure of the original positive assertion without
// failing the suite, so the contradiction stays executable and visible.
type recordingTB struct {
	failed bool
	msg    string
}

func (r *recordingTB) Helper() {}
func (r *recordingTB) Errorf(format string, args ...any) {
	r.failed = true
	r.msg = fmt.Sprintf(format, args...)
}

// requireQuotePresentBeforeRecall is the unchanged requirement: the original
// quoted governing instruction is present in the request before any recall.
func requireQuotePresentBeforeRecall(tb interface {
	Helper()
	Errorf(string, ...any)
}, req agent.Request, quote string) {
	tb.Helper()
	found := strings.Contains(req.System, quote)
	for _, m := range req.Messages {
		for _, b := range m.Content {
			found = found || strings.Contains(b.ToolOutput, quote) || strings.Contains(b.Text, quote)
		}
	}
	if !found {
		tb.Errorf("quote %q absent from request before recall", quote)
	}
}
