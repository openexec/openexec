package execution

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"
)

// These fields deliberately exclude text, arguments, file paths and endpoints.
// This is an event projection, not a model request or replay fixture.
type d26StructuralEvent struct {
	Sequence        int    `json:"sequence"`
	CreatedAt       string `json:"createdAt"`
	Type            string `json:"type"`
	ToolName        string `json:"toolName,omitempty"`
	Call            int    `json:"call,omitempty"`
	Executable      string `json:"executable,omitempty"`
	OutputTailBytes int    `json:"outputTailBytes,omitempty"`
}

func d26ProjectEvents(t *testing.T, raw []byte) []d26StructuralEvent {
	t.Helper()
	var events []struct {
		Sequence  int
		CreatedAt string
		Type      string
		Tool      struct {
			CallID, Name, OutputTail string
			Input                    struct{ Command []string }
		}
	}
	if json.Unmarshal(raw, &events) != nil {
		t.Fatal("private events did not decode")
	}
	var result []d26StructuralEvent
	calls := map[string]int{}
	for _, e := range events {
		r := d26StructuralEvent{Sequence: e.Sequence, CreatedAt: e.CreatedAt, Type: e.Type}
		if e.Type == "tool.started" {
			if calls[e.Tool.CallID] != 0 {
				t.Fatal("duplicate call ID")
			}
			calls[e.Tool.CallID] = len(calls) + 1
			if len(e.Tool.Input.Command) > 0 {
				r.Executable = e.Tool.Input.Command[0]
			}
		}
		if e.Type == "tool.started" || e.Type == "tool.completed" {
			r.ToolName, r.Call = e.Tool.Name, calls[e.Tool.CallID]
			if r.Call == 0 {
				t.Fatal("completion without start")
			}
		}
		if e.Type == "tool.completed" {
			r.OutputTailBytes = len(e.Tool.OutputTail)
		}
		result = append(result, r)
	}
	return result
}

func TestD26StructuralChronology(t *testing.T) {
	raw, err := os.ReadFile("testdata/d26-structure.json")
	if err != nil {
		t.Fatal(err)
	}
	var events []d26StructuralEvent
	if err := json.Unmarshal(raw, &events); err != nil {
		t.Fatal(err)
	}
	if len(events) != 227 {
		t.Fatal("event count changed")
	}
	types, tools, executables := map[string]int{}, map[string]int{}, map[string]int{}
	starts, completions := map[int]string{}, map[int]string{}
	tailBytes := 0
	phaseBytes := [4]int{}
	last := time.Time{}
	for i, e := range events {
		at, err := time.Parse(time.RFC3339Nano, e.CreatedAt)
		if err != nil || at.Before(last) || e.Sequence != i+1 {
			t.Fatal("event chronology changed")
		}
		last = at
		types[e.Type]++
		if e.Type == "tool.started" {
			tools[e.ToolName]++
			starts[e.Call] = e.ToolName
		}
		if e.Type == "tool.completed" {
			completions[e.Call] = e.ToolName
			tailBytes += e.OutputTailBytes
			phase := 0
			for _, boundary := range []int{41, 119, 187} {
				if e.Sequence > boundary {
					phase++
				}
			}
			phaseBytes[phase] += e.OutputTailBytes
		}
		if e.Executable != "" {
			executables[e.Executable]++
		}
	}
	if !reflect.DeepEqual(types, map[string]int{"turn.started": 1, "base.refreshed": 1, "tool.started": 104, "tool.completed": 104, "assistant.delta": 15, "turn.error": 1, "turn.failed": 1}) ||
		!reflect.DeepEqual(tools, map[string]int{"run_command": 72, "read_file": 22, "search_files": 5, "inspect_repository": 3, "list_directory": 1, "record_route_judgment": 1}) ||
		!reflect.DeepEqual(executables, map[string]int{"grep": 40, "sed": 22, "cat": 4, "git": 3, "ls": 2, "wc": 1}) ||
		!reflect.DeepEqual(starts, completions) || len(starts) != 104 || tailBytes != 92060 || phaseBytes != [4]int{13743, 36632, 21127, 20558} {
		t.Fatal("structural totals or paired tool history changed")
	}
	t.Log("227 ordered events; 104 paired calls; 92060 retained output-tail bytes (not full request growth)")
}

// On the diagnostic host this independently re-reads the original private
// artifacts, with no writes and no inference. Other hosts explicitly skip this
// private evidence check; the public protocol controls still run there.
func TestD26PrivateOccurrenceEvidence(t *testing.T) {
	const root = "/home/perttu/.agent-console/perttu/scratch/20261031T163533Z/"
	raw, err := os.ReadFile(root + "d26-events.json")
	if os.IsNotExist(err) || os.IsPermission(err) {
		t.Skip("private d26 evidence unavailable; public fixture is a structural projection only")
	}
	if err != nil {
		t.Fatal("private events unreadable")
	}
	if fmt.Sprintf("%x", sha256.Sum256(raw)) != "b089c85d67c841d464a7e3a1f0eb199c1b9472682ddb75c683d4f86768260014" {
		t.Fatal("private event artifact identity changed")
	}
	wantRaw, err := os.ReadFile("testdata/d26-structure.json")
	if err != nil {
		t.Fatal(err)
	}
	var want []d26StructuralEvent
	if json.Unmarshal(wantRaw, &want) != nil || !reflect.DeepEqual(d26ProjectEvents(t, raw), want) {
		t.Fatal("private events differ from public structural chronology")
	}
	var terminal []struct{ Type, Text string }
	_ = json.Unmarshal(raw, &terminal)
	for _, e := range terminal {
		if (e.Type == "turn.error" || e.Type == "turn.failed") && e.Text != "context_overflow: assembled request bytes=693918 estimated_tokens=296242 context=0 dispatched=true" {
			t.Fatal("terminal signature changed")
		}
	}
	read := func(path string, value any) {
		t.Helper()
		raw, err := os.ReadFile(path)
		if err != nil || json.Unmarshal(raw, value) != nil {
			t.Fatal("required private artifact unavailable or malformed")
		}
	}
	var capture struct {
		Truncated bool
		TextLimit int
		Dispatch  struct{ Method, URL, Provenance string }
		Request   struct {
			Model    string
			Messages []struct{ Role, Content string }
			Tools    []json.RawMessage
			Options  map[string]any
			Stream   bool
			Truncate bool
		}
	}
	read("/home/perttu/.agent-console/run-requests/d26bd137975612543c90e5b983f60db8.json", &capture)
	u, err := url.Parse(capture.Dispatch.URL)
	if err != nil || u.Path != "/api/chat" || capture.Dispatch.Method != "POST" || capture.Dispatch.Provenance != "source-known-transport-attempt" ||
		!capture.Truncated || capture.TextLimit != 2000 || capture.Request.Model != "qwen38-27b-mtp2-128k" || len(capture.Request.Messages) != 164 || len(capture.Request.Tools) != 22 || len(capture.Request.Options) != 0 || capture.Request.Stream || capture.Request.Truncate {
		t.Fatal("capture fidelity or dispatch facts changed")
	}
	roles := map[string]int{}
	for _, m := range capture.Request.Messages {
		roles[m.Role]++
	}
	if !reflect.DeepEqual(roles, map[string]int{"system": 1, "user": 1, "assistant": 58, "tool": 104}) {
		t.Fatal("captured message roles changed")
	}
	if !strings.Contains(capture.Request.Messages[0].Content, "characters cut") || !strings.Contains(capture.Request.Messages[1].Content, "characters cut") || !strings.Contains(capture.Request.Messages[1].Content, "f081bc00498e1bc6f7ee5b63a0282418") {
		t.Fatal("original input identity or known capture omissions changed")
	}
	var task struct {
		ID, Status, ImplementationRunID, PullRequestURL string
		Description, TriageContract                     string
		Addenda                                         []json.RawMessage
	}
	read(root+"f081bc00498e1bc6f7ee5b63a0282418.json", &task)
	if task.ID != "f081bc00498e1bc6f7ee5b63a0282418" || task.Status != "blocked" || task.ImplementationRunID != "d26bd137975612543c90e5b983f60db8" || task.PullRequestURL != "https://github.com/openexec/agent-console/pull/1682" || len([]rune(task.Description)) != 3858 || len([]rune(task.TriageContract)) != 31564 || len(task.Addenda) != 23 {
		t.Fatal("saved original declaration/blocked-task identity changed")
	}
	var replay struct {
		Reproduced, Partial bool
		Requests            int
		Seconds             float64
		First               struct{ Signature struct{ Cls string } }
		Attempts, Passes    []json.RawMessage
	}
	read(root+"d26-replay-private.json", &replay)
	var summary struct{ ExitCode int }
	read(root+"d26-replay-summary.json", &summary)
	if replay.Reproduced || replay.Partial || replay.Requests != 1 || replay.Seconds != 60.1 || replay.First.Signature.Cls != "timeout" || len(replay.Attempts) != 0 || len(replay.Passes) != 0 || summary.ExitCode != 2 {
		t.Fatal("nonreproduction facts changed")
	}
	t.Log("private originals match projection; truncated native capture; one replay, exit 2, different timeout, NOT REPRODUCED")
}
