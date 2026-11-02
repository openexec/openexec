package execution

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/openexec/openexec/pkg/agent"
)

// The handover document (owner, 11-02): "with Qwen it is important to know what
// it needs to be doing and where the information is found". One per run, on
// disk, mode 0600, in a private directory next to the run's request capture.
// Large things stay on disk as saved tool outputs and are indexed here.
//
// The harness writes the mechanical sections itself from what it observed —
// files touched, commands and their exit codes, test status, the saved-output
// index — so a model that writes nothing still leaves a usable document. The
// model is asked to maintain Done, Next steps, Decisions and Open questions at
// checkpoints (update_handover); an empty one is filled from the log and flagged.
const (
	HandoverFile  = "HANDOVER.md"
	handoverState = "handover.json"

	updateHandoverTool = "update_handover"
	readSavedTool      = "read_saved_output"

	handoverTaskLimit    = 12 << 10
	handoverSectionLimit = 4 << 10
	handoverListLimit    = 40
	savedHead, savedTail = 2 << 10, 1 << 10
	savedPageLines       = 200
	savedPageBytes       = 8 << 10
)

type savedOutput struct {
	ID      string `json:"id"`
	Tool    string `json:"tool"`
	Summary string `json:"summary"`
	Bytes   int    `json:"bytes"`
	Lines   int    `json:"lines"`
}

type commandRecord struct {
	Tool    string `json:"tool"`
	Command string `json:"command"`
	Exit    string `json:"exit"`
	Failed  bool   `json:"failed"`
	Output  string `json:"output,omitempty"`
}

// handoverData is everything that survives a pass.
type handoverData struct {
	Status      string          `json:"status"` // running, checkpointed, complete
	Task        string          `json:"task"`
	Constraints string          `json:"constraints"`
	WorkingDir  string          `json:"working_dir"`
	Outputs     []savedOutput   `json:"outputs"`
	Reads       []string        `json:"reads"`
	Writes      []string        `json:"writes"`
	Commands    []commandRecord `json:"commands"`
	Done        string          `json:"done"`
	Next        string          `json:"next"`
	Decisions   string          `json:"decisions"`
	Questions   string          `json:"questions"`
	Extra       string          `json:"extra_constraints"`
	LastText    string          `json:"last_text"`
	Chains      int             `json:"chains"`
	Checkpoints int             `json:"checkpoints"`
	Passes      int             `json:"passes"`
	Seq         int             `json:"seq"`
	UpdatedAt   time.Time       `json:"updated_at"`
}

// runState is the harness's record of one run: its handover and saved outputs.
type runState struct {
	mu     sync.Mutex
	policy ContextPolicy
	dir    string
	data   handoverData
	// written: something is on disk, so the document's final status is updated.
	written bool
}

func newRunState(request Request, policy ContextPolicy) *runState {
	s := &runState{policy: policy, dir: runDirFor(os.Getenv(RequestCaptureEnv))}
	s.data = handoverData{
		Status: "running", Task: request.Prompt, WorkingDir: request.WorkingDir, Passes: 1,
		Constraints: constraintsOf(request),
	}
	return s
}

// runDirFor is the run's private directory: the request capture's path without
// its extension, plus ".run" (<state>/run-requests/<job>.run). Empty when the
// caller asked for no capture; the directory is then made on first use.
func runDirFor(capture string) string {
	capture = strings.TrimSpace(capture)
	if capture == "" {
		return ""
	}
	return strings.TrimSuffix(capture, filepath.Ext(capture)) + ".run"
}

func constraintsOf(r Request) string {
	var b strings.Builder
	fmt.Fprintf(&b, "- Sandbox: %s", r.Sandbox.Mode)
	if r.Sandbox.Mode == SandboxReadOnly {
		b.WriteString(" (you cannot change files)")
	}
	b.WriteString("\n")
	if len(r.WritableRoots) > 0 {
		fmt.Fprintf(&b, "- Writable roots: %s\n", strings.Join(r.WritableRoots, ", "))
	}
	if len(r.ReadableRoots) > 0 {
		fmt.Fprintf(&b, "- Read-only roots: %s\n", strings.Join(r.ReadableRoots, ", "))
	}
	b.WriteString("- Do not widen the task beyond the contract; do not touch files outside it.\n")
	b.WriteString("- Do not repeat work listed under Done so far; do not re-read what a saved output or a file range already gave you.\n")
	b.WriteString("- Never claim a test passed unless the command log below shows it passed.\n")
	return b.String()
}

func (s *runState) ensureDir() (string, error) {
	if s.dir == "" {
		dir, err := os.MkdirTemp("", "openexec-run-")
		if err != nil {
			return "", err
		}
		s.dir = dir
	}
	if err := os.MkdirAll(s.dir, 0o700); err != nil {
		return "", err
	}
	return s.dir, nil
}

// load continues a run a previous pass checkpointed. A document of a finished
// or still-running run is not resumed: only a deliberate checkpoint is.
func (s *runState) load() bool {
	if s.dir == "" {
		return false
	}
	raw, err := os.ReadFile(filepath.Join(s.dir, handoverState))
	if err != nil {
		return false
	}
	var saved handoverData
	if json.Unmarshal(raw, &saved) != nil || saved.Status != "checkpointed" {
		return false
	}
	saved.Passes++
	saved.Status = "running"
	if strings.TrimSpace(s.data.Task) != "" {
		saved.Task = s.data.Task
	}
	saved.WorkingDir, saved.Constraints = s.data.WorkingDir, s.data.Constraints
	s.data = saved
	return true
}

// save writes the document and its machine state. Best effort: a handover that
// cannot be written changes nothing about the run, and says so in the text.
func (s *runState) save(status string) error {
	s.data.UpdatedAt = time.Now().UTC()
	if status != "" {
		s.data.Status = status
	}
	dir, err := s.ensureDir()
	if err != nil {
		return err
	}
	raw, _ := json.Marshal(s.data)
	if err := writePrivate(filepath.Join(dir, handoverState), raw); err != nil {
		return err
	}
	s.written = true
	return writePrivate(filepath.Join(dir, HandoverFile), []byte(s.render()))
}

// closeOut records how the run ended when it left a document behind.
func (s *runState) closeOut(status string) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.written {
		_ = s.save(status)
	}
}

// Path is where the document lives, or "" before anything was written.
func (s *runState) path() string {
	if s.dir == "" {
		return ""
	}
	return filepath.Join(s.dir, HandoverFile)
}

func bound(text string, limit int) string {
	text = strings.TrimSpace(text)
	if len(text) <= limit {
		return text
	}
	head, tail, omitted := headTail(text, limit*3/4, limit/4)
	return head + fmt.Sprintf("\n[... %d bytes omitted ...]\n", omitted) + tail
}

var exitPattern = regexp.MustCompile(`(?i)\bexit(?:ed with)?(?:[ _-]?(?:code|status))?[ :=]+(-?\d+)\b`)

// recordCall learns the mechanical facts from one finished tool call.
func (s *runState) recordCall(name string, input json.RawMessage, output string, toolErr error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var args map[string]any
	_ = json.Unmarshal(input, &args)
	str := func(key string) string { v, _ := args[key].(string); return v }
	switch name {
	case updateHandoverTool, readSavedTool:
		return
	case "read_file":
		ref := str("path")
		if n, ok := args["start_line"].(float64); ok {
			ref += fmt.Sprintf(" (from line %d)", int(n))
		} else if n, ok := args["offset_bytes"].(float64); ok && n > 0 {
			ref += fmt.Sprintf(" (from byte %d)", int(n))
		}
		s.data.Reads = addUnique(s.data.Reads, ref)
	case "search_files":
		s.data.Reads = addUnique(s.data.Reads, fmt.Sprintf("search %q in %s", str("text"), str("path")))
	case "list_directory":
		s.data.Reads = addUnique(s.data.Reads, "list "+str("path"))
	case "write_file":
		if toolErr == nil {
			s.data.Writes = addUnique(s.data.Writes, str("path"))
		}
	default:
		command := str("command")
		if command == "" {
			command = strings.TrimSpace(str("check") + " " + strings.Join(stringsOf(args["args"]), " "))
		}
		if command == "" {
			// Not a command: another tool's call is a place the information
			// came from, not something to re-run.
			s.data.Reads = addUnique(s.data.Reads, fmt.Sprintf("%s %s", name, bound(string(input), 100)))
			return
		}
		record := commandRecord{Tool: name, Command: bound(command, 300), Failed: toolErr != nil}
		if m := exitPattern.FindStringSubmatch(tail(output, 400)); m != nil {
			record.Exit = m[1]
			record.Failed = record.Failed || m[1] != "0"
		} else if toolErr != nil {
			record.Exit = "error"
		} else {
			record.Exit = "0 (no exit code reported)"
		}
		if toolErr != nil {
			record.Output = bound(toolErr.Error(), 300)
		}
		s.data.Commands = append(s.data.Commands, record)
		if len(s.data.Commands) > handoverListLimit {
			s.data.Commands = s.data.Commands[len(s.data.Commands)-handoverListLimit:]
		}
	}
}

func tail(text string, n int) string {
	if len(text) <= n {
		return text
	}
	return text[len(text)-n:]
}

func stringsOf(v any) []string {
	var out []string
	if list, ok := v.([]any); ok {
		for _, item := range list {
			if s, ok := item.(string); ok {
				out = append(out, s)
			}
		}
	}
	return out
}

func addUnique(list []string, item string) []string {
	for _, existing := range list {
		if existing == item {
			return list
		}
	}
	list = append(list, item)
	if len(list) > handoverListLimit {
		list = list[len(list)-handoverListLimit:]
	}
	return list
}

// compactResult returns the result as it enters a request. Whole files and long
// command output do not: the full text is saved in the run directory and the
// request carries a short head, a short tail and a pointer to read lines.
func (s *runState) compactResult(name string, input json.RawMessage, output string) string {
	if len(output) <= s.policy.MaxInlineToolBytes || name == readSavedTool || name == updateHandoverTool {
		return output
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data.Seq++
	id := fmt.Sprintf("out-%04d", s.data.Seq)
	lines := strings.Count(output, "\n") + 1
	summary := strings.TrimSpace(strings.SplitN(output, "\n", 2)[0])
	if len(summary) > 120 {
		summary = summary[:120]
	}
	dir, err := s.ensureDir()
	if err == nil {
		err = writePrivate(filepath.Join(dir, id+".txt"), []byte(output))
	}
	if err != nil {
		// Without the file the pointer would lie; keep a bounded view and say so.
		head, tail, omitted := headTail(output, savedHead, savedTail)
		return fmt.Sprintf("[output of %d bytes cut to its head and tail (%d bytes omitted); it could not be saved: %v]\n%s\n[...]\n%s", len(output), omitted, err, head, tail)
	}
	s.data.Outputs = append(s.data.Outputs, savedOutput{ID: id, Tool: name, Summary: summary, Bytes: len(output), Lines: lines})
	head, tailText, omitted := headTail(output, savedHead, savedTail)
	return fmt.Sprintf("[%s output: %d bytes, %d lines; the full output is saved as %s. Read lines a-b with %s {\"id\":%q,\"from_line\":a,\"to_line\":b}.]\n%s\n[... %d bytes omitted ...]\n%s",
		name, len(output), lines, id, readSavedTool, id, head, omitted, tailText)
}

// readSaved serves read_saved_output.
func (s *runState) readSaved(raw json.RawMessage) (string, error) {
	var in struct {
		ID   string `json:"id"`
		From int    `json:"from_line"`
		To   int    `json:"to_line"`
	}
	if err := json.Unmarshal(raw, &in); err != nil {
		return "", fmt.Errorf("invalid read_saved_output request")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	known := false
	for _, o := range s.data.Outputs {
		known = known || o.ID == in.ID
	}
	if !known || s.dir == "" {
		return "", fmt.Errorf("unknown saved output %q", in.ID)
	}
	body, err := os.ReadFile(filepath.Join(s.dir, in.ID+".txt"))
	if err != nil {
		return "", fmt.Errorf("saved output %s is unavailable", in.ID)
	}
	lines := strings.Split(string(body), "\n")
	from := max(in.From, 1)
	to := in.To
	if to < from {
		to = from + savedPageLines - 1
	}
	to = min(to, from+savedPageLines-1, len(lines))
	if from > len(lines) {
		return "", fmt.Errorf("%s has %d lines", in.ID, len(lines))
	}
	page := strings.Join(lines[from-1:to], "\n")
	if len(page) > savedPageBytes {
		head, _, _ := headTail(page, savedPageBytes, 0)
		page = head
		to = from + strings.Count(head, "\n")
	}
	return fmt.Sprintf("%s lines %d-%d of %d:\n%s", in.ID, from, to, len(lines), page), nil
}

type textOrList string

func (t *textOrList) UnmarshalJSON(raw []byte) error {
	var list []string
	if json.Unmarshal(raw, &list) == nil {
		*t = textOrList("- " + strings.Join(list, "\n- "))
		return nil
	}
	var text string
	if err := json.Unmarshal(raw, &text); err != nil {
		return err
	}
	*t = textOrList(text)
	return nil
}

// update serves update_handover: the model's own sections replace the old ones
// when given.
func (s *runState) update(raw json.RawMessage) (string, error) {
	var in struct {
		Done      *textOrList `json:"done"`
		Next      *textOrList `json:"next_steps"`
		Decisions *textOrList `json:"decisions"`
		Questions *textOrList `json:"open_questions"`
		Extra     *textOrList `json:"constraints"`
	}
	if err := json.Unmarshal(raw, &in); err != nil {
		return "", fmt.Errorf("invalid update_handover request: %v", err)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	set := func(dst *string, v *textOrList) {
		if v != nil {
			*dst = bound(string(*v), handoverSectionLimit)
		}
	}
	set(&s.data.Done, in.Done)
	set(&s.data.Next, in.Next)
	set(&s.data.Decisions, in.Decisions)
	set(&s.data.Questions, in.Questions)
	set(&s.data.Extra, in.Extra)
	if err := s.save(""); err != nil {
		return "", fmt.Errorf("handover could not be written: %v", err)
	}
	return "Handover updated.", nil
}

func (s *runState) noteText(text string) {
	if strings.TrimSpace(text) == "" {
		return
	}
	s.mu.Lock()
	s.data.LastText = bound(text, 1500)
	s.mu.Unlock()
}

// tools are the two harness-served tools, offered once they have a use.
func handoverToolDefinition() agent.ToolDefinition {
	return agent.ToolDefinition{Name: updateHandoverTool,
		Description: "Update the run's handover document, which carries this task across context checkpoints. Give what is done, the ordered next steps, decisions and why, and open questions; each field replaces the previous text. Files, commands and test results are recorded for you.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"done":{"type":"string"},"next_steps":{"type":"string"},"decisions":{"type":"string"},"open_questions":{"type":"string"},"constraints":{"type":"string"}},"additionalProperties":false}`)}
}

func readSavedDefinition() agent.ToolDefinition {
	return agent.ToolDefinition{Name: readSavedTool,
		Description: "Read a range of lines of a large tool output the harness saved to disk (an id such as out-0003 from an earlier result or from the handover index).",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"id":{"type":"string"},"from_line":{"type":"integer","minimum":1},"to_line":{"type":"integer","minimum":1}},"required":["id","from_line"],"additionalProperties":false}`)}
}

// ---- rendering ------------------------------------------------------------

var testCommand = regexp.MustCompile(`(?i)\b(go test|pytest|npm (run )?test|make (test|check)|cargo test|vitest|jest|run_declared_check|\btest\b)`)

func (s *runState) render() string {
	d := s.data
	var b strings.Builder
	fmt.Fprintf(&b, "# Handover\n\nStatus: %s · pass %d · chains %d · checkpoints %d · updated %s\n", d.Status, d.Passes, d.Chains, d.Checkpoints, d.UpdatedAt.Format(time.RFC3339))
	b.WriteString("\n## Task\n\n")
	b.WriteString(bound(d.Task, handoverTaskLimit))
	if len(d.Task) > handoverTaskLimit {
		b.WriteString("\n(The task above is shortened; the full text is the run's first message.)")
	}
	b.WriteString("\n\n## Constraints and do-not list\n\n")
	b.WriteString(strings.TrimSpace(d.Constraints))
	if strings.TrimSpace(d.Extra) != "" {
		b.WriteString("\n" + strings.TrimSpace(d.Extra))
	}

	b.WriteString("\n\n## Done so far\n\n")
	if strings.TrimSpace(d.Done) != "" {
		b.WriteString(strings.TrimSpace(d.Done) + "\n")
	} else {
		b.WriteString("FLAG: the model left this empty; the harness filled it from the log.\n")
		if len(d.Writes) == 0 && len(d.Commands) == 0 && len(d.Reads) == 0 {
			b.WriteString("- Nothing recorded yet.\n")
		}
		if len(d.Writes) > 0 {
			b.WriteString("- Files written: " + strings.Join(d.Writes, ", ") + "\n")
		}
		fmt.Fprintf(&b, "- %d command(s) run, %d file/search reads.\n", len(d.Commands), len(d.Reads))
		if strings.TrimSpace(d.LastText) != "" {
			b.WriteString("- Last thing the model said: " + strings.ReplaceAll(strings.TrimSpace(d.LastText), "\n", " ") + "\n")
		}
	}
	b.WriteString("\n### Recorded by the harness\n\n")
	b.WriteString(s.mechanical())

	b.WriteString("\n## Next steps\n\n")
	if strings.TrimSpace(d.Next) != "" {
		b.WriteString(strings.TrimSpace(d.Next) + "\n")
	} else {
		b.WriteString("FLAG: the model left this empty; do these in order:\n1. Re-read the Task above and compare it with the changes in `git diff --stat` below.\n2. Re-run the last failing command or the last test command listed below.\n3. Continue the part of the Task that no listed change covers, then run the tests again.\n")
	}

	b.WriteString("\n## Where the information is\n\n")
	b.WriteString(s.where())

	b.WriteString("\n## Decisions made and why\n\n")
	b.WriteString(orNone(d.Decisions, "None recorded by the model."))
	b.WriteString("\n\n## Open questions\n\n")
	b.WriteString(orNone(d.Questions, "None recorded by the model."))
	b.WriteString("\n")
	return b.String()
}

func orNone(text, none string) string {
	if strings.TrimSpace(text) == "" {
		return none
	}
	return strings.TrimSpace(text)
}

// mechanical is the part no model writes: files from git, commands and exits,
// test status.
func (s *runState) mechanical() string {
	var b strings.Builder
	b.WriteString("Files touched (git):\n" + gitSummary(s.data.WorkingDir) + "\n")
	b.WriteString("Commands run (last " + fmt.Sprint(handoverListLimit) + ", newest last):\n")
	if len(s.data.Commands) == 0 {
		b.WriteString("- none\n")
	}
	lastTest := ""
	for _, c := range s.data.Commands {
		fmt.Fprintf(&b, "- `%s` (%s) -> exit %s\n", strings.ReplaceAll(c.Command, "\n", " "), c.Tool, c.Exit)
		if c.Output != "" {
			fmt.Fprintf(&b, "  error: %s\n", strings.ReplaceAll(c.Output, "\n", " "))
		}
		if testCommand.MatchString(c.Command) {
			status := "passed"
			if c.Failed {
				status = "FAILED"
			}
			lastTest = fmt.Sprintf("`%s` %s (exit %s)", c.Command, status, c.Exit)
		}
	}
	if lastTest == "" {
		lastTest = "no test command has been run"
	}
	b.WriteString("Test status: " + lastTest + "\n")
	return b.String()
}

func (s *runState) where() string {
	var b strings.Builder
	b.WriteString("Files and searches already read (re-read a range with read_file start_line=N):\n")
	if len(s.data.Reads) == 0 {
		b.WriteString("- none\n")
	}
	for _, r := range s.data.Reads {
		b.WriteString("- " + r + "\n")
	}
	b.WriteString("Commands to re-run: the list under Recorded by the harness; the working directory is " + orNone(s.data.WorkingDir, "the run's directory") + ".\n")
	b.WriteString("Saved tool outputs (read lines with " + readSavedTool + " {id, from_line, to_line}):\n")
	if len(s.data.Outputs) == 0 {
		b.WriteString("- none\n")
	}
	for _, o := range s.data.Outputs {
		fmt.Fprintf(&b, "- %s: %s, %d bytes, %d lines, begins %q\n", o.ID, o.Tool, o.Bytes, o.Lines, o.Summary)
	}
	return b.String()
}

// gitSummary is `git status` and `git diff --stat` of the working directory,
// bounded. It reads only; a directory that is not a repository says so.
func gitSummary(dir string) string {
	if dir == "" {
		return "- (no working directory)\n"
	}
	run := func(args ...string) (string, error) {
		ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, "git", append([]string{"-C", dir, "-c", "core.fsmonitor=false", "-c", "core.hooksPath=/dev/null"}, args...)...)
		cmd.Env = append(os.Environ(), "GIT_OPTIONAL_LOCKS=0")
		var out bytes.Buffer
		cmd.Stdout = &out
		err := cmd.Run()
		return out.String(), err
	}
	status, err := run("status", "--porcelain")
	if err != nil {
		return "- (not a git repository or git unavailable)\n"
	}
	var b strings.Builder
	lines := strings.Split(strings.TrimSpace(status), "\n")
	if strings.TrimSpace(status) == "" {
		b.WriteString("- no changes\n")
	}
	for i, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		if i >= 60 {
			fmt.Fprintf(&b, "- ... and %d more\n", len(lines)-i)
			break
		}
		b.WriteString("- " + line + "\n")
	}
	if stat, err := run("diff", "--stat"); err == nil && strings.TrimSpace(stat) != "" {
		b.WriteString("```\n" + bound(stat, 2<<10) + "\n```\n")
	}
	return b.String()
}

// freshMessages is the whole of a new request chain besides the system prompt:
// one user message holding the checkpoint notice and the handover.
func (s *runState) freshMessages(final bool) []agent.Message {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data.Chains++
	notice := fmt.Sprintf("Context checkpoint %d. The earlier conversation was ended to keep requests small and fast. Everything you need is in the handover document below; its Recorded-by-the-harness parts come from the real tool log. ", s.data.Chains)
	if final {
		notice += "Answer the user's request now with the best complete final response the handover supports; no tools are available."
	} else {
		notice += "Continue from \"Next steps\"; do not redo what is listed as done. Read a file range or a saved output only when you need it, and keep the handover current with " + updateHandoverTool + "."
	}
	_ = s.save("")
	return []agent.Message{agent.NewTextMessage(agent.RoleUser, notice+"\n\n"+s.render())}
}
