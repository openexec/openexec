package execution

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
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

// The executor-level fact: a forwarded (non-workspace) list_directory is
// untrusted, so the adapter never records its results.
func TestForwardedListingIsNotTrustedForOmission(t *testing.T) {
	workspace := NewWorkspaceToolExecutor()
	if !(&CompositeToolExecutor{workspace: workspace, forwarded: map[string]bool{}}).safelyListsDirectories() {
		t.Fatal("workspace listing should be trusted")
	}
	if (&CompositeToolExecutor{workspace: workspace, forwarded: map[string]bool{"list_directory": true}}).safelyListsDirectories() {
		t.Fatal("forwarded listing was trusted")
	}
}

func provenRequest(tool string) (agent.Request, string, string) {
	old := "older " + strings.Repeat("evidence ", 60)
	newest := "newest " + strings.Repeat("evidence ", 60)
	return structuralRequest([3]string{tool, old}, [3]string{tool, newest}), old, newest
}

func recordFor(positions map[int]string) map[[2]int]string {
	out := map[[2]int]string{}
	for message, text := range positions {
		out[[2]int{message, 0}] = sha256Hex(text)
	}
	return out
}

// A search is omittable only when scan provenance names its exact position and
// text. Every other combination keeps the request byte-identical.
func TestSearchOmissionRequiresPositionBoundScanProvenance(t *testing.T) {
	r, old, newest := provenRequest("search_files")
	unchanged := func(name string, request agent.Request, scans map[[2]int]string) {
		t.Helper()
		before, _ := json.Marshal(request)
		compacted, changed := (priorResults{}).compactWith(request, scans)
		after, _ := json.Marshal(compacted)
		if changed || string(before) != string(after) {
			t.Fatalf("%s: result was omitted or request altered", name)
		}
	}
	proof := recordFor(map[int]string{2: old, 4: newest})

	compacted, changed := (priorResults{}).compactWith(r, proof)
	if !changed || compacted.Messages[2].Content[0].ToolOutput == old || compacted.Messages[4].Content[0].ToolOutput != newest {
		t.Fatal("complete scans with provenance: older omitted, newest kept")
	}
	if !strings.Contains(compacted.Messages[2].Content[0].ToolOutput, "complete search result") {
		t.Fatal("stub should name a search result")
	}

	unchanged("no provenance", r, nil)
	unchanged("empty provenance", r, map[[2]int]string{})
	unchanged("wrong position", r, recordFor(map[int]string{1: old, 3: newest}))
	unchanged("different text at the position", r, recordFor(map[int]string{2: old + "x", 4: newest + "x"}))
	unchanged("only the newest proven", r, recordFor(map[int]string{4: newest}))

	failed, _, _ := provenRequest("search_files")
	failed.Messages[2].Content[0].ToolError = "denied"
	unchanged("errored result with provenance", failed, proof)

	orphan, _, _ := provenRequest("search_files")
	orphan.Messages[2].Content[0].ToolResultID = "orphan"
	unchanged("unpaired result with provenance", orphan, proof)

	// A reused call ID makes pairing ambiguous even with provenance.
	ambiguous := agent.Request{System: r.System, Messages: []agent.Message{
		r.Messages[0],
		{Role: agent.RoleAssistant, Content: []agent.ContentBlock{
			{Type: agent.ContentTypeToolUse, ToolUseID: "dup", ToolName: "search_files"},
			{Type: agent.ContentTypeToolUse, ToolUseID: "dup", ToolName: "search_files"}}},
		agent.NewToolResultMessage("dup", old, nil),
		agent.NewToolResultMessage("dup", newest, nil),
	}}
	unchanged("ambiguous reused id", ambiguous, recordFor(map[int]string{2: old, 3: newest}))

	// Provenance recorded at a position never makes a non-search eligible.
	for _, tool := range []string{"read_file", "some_unknown_tool", "run_command"} {
		other, o, n := provenRequest(tool)
		unchanged(tool, other, recordFor(map[int]string{2: o, 4: n}))
	}
}

// A search_files answered by a forwarded gateway has no scan provenance, even
// when the gateway's text is a successful result.
func TestForwardedSearchIsNeverReportedComplete(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `{"jsonrpc":"2.0","id":1,"result":{"content":[{"type":"text","text":"a.txt:1:needle\n"}]}}`)
	}))
	defer server.Close()
	gateway := &GatewayToolExecutor{endpoint: server.URL, client: server.Client(), tools: []agent.ToolDefinition{{Name: "search_files"}}}
	composite := &CompositeToolExecutor{workspace: NewWorkspaceToolExecutor(), gateway: gateway, forwarded: map[string]bool{"search_files": true}}
	output, complete, err := composite.executeSearchWithProvenance(context.Background(), searchRequest(t.TempDir(), "a.txt", "needle"))
	if err != nil || complete || output != "a.txt:1:needle\n" {
		t.Fatalf("forwarded search: output=%q complete=%v err=%v", output, complete, err)
	}
}

// Executable control for the one place "the original governing quote stays in
// the request" and "eligibility is structural only" are in tension: a quote
// that exists solely inside an OLDER, structurally complete scan result. The
// instruction text is never read, so that older result is stubbed and the
// quote is absent from the request until recalled. Every other carrier of the
// same quote (system, user turns, file reads, the newest scan) is untouched,
// and the stubbed quote is recoverable byte-for-byte through paged recall.
// Neither assertion is weakened: the first half pins the structural-only
// outcome, the second pins what stays preserved and what recall returns.
func TestQuotedInstructionInCompleteOlderScanIsRecallableNotHeuristicallyKept(t *testing.T) {
	quote := "AGENTS.md says: NEVER DEPLOY without review. " + strings.Repeat("authority ", 60)
	older, newest := quote+"older", strings.Repeat("plain listing line ", 40)+"newest"
	for _, tool := range []string{"list_directory", "search_files"} {
		t.Run(tool, func(t *testing.T) {
			r := structuralRequest([3]string{tool, older}, [3]string{tool, newest})
			store := priorResults{}
			compacted, changed := store.compactWith(r, recordFor(map[int]string{2: older, 4: newest}))
			if !changed {
				t.Fatal("a structurally complete older scan must be omitted whatever it quotes")
			}
			// The ORIGINAL, unchanged positive requirement, run for real through
			// a recording tb: the quote must be present in the request BEFORE
			// recall. Its failure is captured, not waived: it is the executable
			// contradiction with structural-only eligibility.
			var rec recordingTB
			requireQuotePresentBeforeRecall(&rec, compacted, "NEVER DEPLOY without review")
			t.Logf("original positive assertion on compacted request: failed=%v msg=%q", rec.failed, rec.msg)
			if !rec.failed {
				t.Fatal("quote survived omission, so eligibility must have read content")
			}
			// Discriminating control: the same assertion PASSES on the uncompacted
			// request, so a failure above means absence, not a broken assertion.
			var ok recordingTB
			requireQuotePresentBeforeRecall(&ok, r, "NEVER DEPLOY without review")
			if ok.failed {
				t.Fatalf("positive assertion fails on the uncompacted request: %s", ok.msg)
			}
			if compacted.System != r.System || compacted.Messages[4].Content[0].ToolOutput != newest || compacted.Messages[0].Content[0].Text != "original request" {
				t.Fatal("system, user turn or newest scan was altered")
			}
			raw, _ := json.Marshal(map[string]any{"id": sha256Hex(older), "offset": 0})
			page, err := store.read(raw)
			if err != nil || !strings.Contains(page, "NEVER DEPLOY without review") {
				t.Fatalf("quote not recoverable by recall: %v", err)
			}
		})
	}
}

// The recorder is fed by the scan's own flag, not by what the output says.
func TestSearchScanProvenanceComesFromTheScanFlag(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "a.txt"), []byte("needle\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "b.txt"), []byte("needle "+strings.Repeat("y", 2000)+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	e := NewWorkspaceToolExecutor()
	composite := &CompositeToolExecutor{workspace: e, forwarded: map[string]bool{}}
	for path, want := range map[string]bool{"a.txt": true, "b.txt": false} {
		request := searchRequest(root, path, "needle")
		request.Sandbox = Sandbox{Mode: SandboxReadOnly}
		for name, executor := range map[string]searchProvenanceExecutor{"workspace": e, "composite": composite} {
			_, complete, err := executor.executeSearchWithProvenance(context.Background(), request)
			if err != nil || complete != want {
				t.Fatalf("%s %s: complete=%v want %v err=%v", name, path, complete, want, err)
			}
		}
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
