package mcp

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/openexec/openexec/internal/governance/mcpgov"
	"github.com/openexec/openexec/internal/memory"
	"github.com/openexec/openexec/internal/release"
)

// newBacklogTestServer builds a server rooted at projDir in suggest
// (read-only) mode — the same condition as a light-mode chat client.
// workspaceRoots is overridden directly because TestMain sets WORKSPACE_ROOT
// to the global temp dir, which would otherwise win over cfg.WorkDir.
func newBacklogTestServer(t *testing.T, projDir string) (*Server, *bytes.Buffer) {
	t.Helper()
	out := new(bytes.Buffer)
	srv, err := NewServerWithConfig(strings.NewReader(""), out, ServerConfig{
		WorkDir: projDir,
		Mode:    "suggest",
	})
	if err != nil {
		t.Fatalf("NewServerWithConfig: %v", err)
	}
	srv.workspaceRoots = []string{projDir}
	// The composition root wires these in production; tests do the same. (Test
	// imports of module packages don't count against the core/module boundary.)
	srv.SetMemoryLoader(func(root string) (string, error) {
		return memory.NewMemorySystem(root).LoadMerged()
	})
	srv.RegisterProvider(mcpgov.New())
	return srv, out
}

// callTool dispatches a single tools/call and decodes the response.
func callTool(t *testing.T, srv *Server, out *bytes.Buffer, name string, args map[string]interface{}) map[string]interface{} {
	t.Helper()
	out.Reset()

	argsJSON, err := json.Marshal(args)
	if err != nil {
		t.Fatalf("marshal args: %v", err)
	}
	paramsJSON, err := json.Marshal(map[string]interface{}{
		"name":      name,
		"arguments": json.RawMessage(argsJSON),
	})
	if err != nil {
		t.Fatalf("marshal params: %v", err)
	}

	srv.dispatch(Request{JSONRPC: "2.0", ID: json.RawMessage(`1`), Method: "tools/call", Params: paramsJSON})

	var resp Response
	if err := json.Unmarshal(bytes.TrimSpace(out.Bytes()), &resp); err != nil {
		t.Fatalf("decode response for %s: %v\nraw: %s", name, err, out.String())
	}
	if resp.Error != nil {
		t.Fatalf("%s returned RPC error: %s", name, resp.Error.Message)
	}

	resultJSON, err := json.Marshal(resp.Result)
	if err != nil {
		t.Fatalf("re-marshal result: %v", err)
	}
	var result map[string]interface{}
	if err := json.Unmarshal(resultJSON, &result); err != nil {
		t.Fatalf("decode result map: %v", err)
	}
	return result
}

func isToolError(result map[string]interface{}) bool {
	v, _ := result["isError"].(bool)
	return v
}

func resultText(result map[string]interface{}) string {
	content, _ := result["content"].([]interface{})
	if len(content) == 0 {
		return ""
	}
	item, _ := content[0].(map[string]interface{})
	text, _ := item["text"].(string)
	return text
}

// seedBacklog creates a project backlog with two stories via a separate
// release.Manager instance (as the heavy pipeline would).
func seedBacklog(t *testing.T, projDir string) *release.Manager {
	t.Helper()
	// Simulate an initialized project: `openexec init` creates .openexec/.
	if err := os.MkdirAll(filepath.Join(projDir, ".openexec"), 0o755); err != nil {
		t.Fatalf("mkdir .openexec: %v", err)
	}
	mgr, err := release.NewManager(projDir, nil)
	if err != nil {
		t.Fatalf("release.NewManager: %v", err)
	}
	if err := mgr.Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}

	stories := []*release.Story{
		{ID: "US-001", Title: "First feature", Status: release.StoryStatusPending},
		{ID: "US-002", Title: "Second feature", Status: release.StoryStatusPending},
	}
	for _, st := range stories {
		if err := mgr.CreateStory(st); err != nil {
			t.Fatalf("CreateStory %s: %v", st.ID, err)
		}
	}

	tasks := []*release.Task{
		{ID: "T-US-001-001", StoryID: "US-001", Title: "Implement slice", Status: release.TaskStatusPending},
		{ID: "T-US-001-002", StoryID: "US-001", Title: "Manual QA", Status: release.TaskStatusPending,
			Metadata: map[string]interface{}{"mode": release.TaskModeHITL}},
		{ID: "T-US-002-001", StoryID: "US-002", Title: "Other work", Status: release.TaskStatusPending},
	}
	for _, task := range tasks {
		if err := mgr.CreateTask(task); err != nil {
			t.Fatalf("CreateTask %s: %v", task.ID, err)
		}
	}
	return mgr
}

// TestBacklogLightModeFlow proves the full light-mode story flow works in
// suggest (read-only) mode: backlog writes are the documented exception.
func TestBacklogLightModeFlow(t *testing.T) {
	projDir := t.TempDir()
	seedBacklog(t, projDir)
	srv, out := newBacklogTestServer(t, projDir)

	// List shows both stories.
	result := callTool(t, srv, out, "backlog_list_stories", nil)
	if isToolError(result) {
		t.Fatalf("list failed: %s", resultText(result))
	}
	stories, _ := result["stories"].([]interface{})
	if len(stories) != 2 {
		t.Fatalf("expected 2 stories, got %d", len(stories))
	}

	// Get story includes tasks with execution modes.
	result = callTool(t, srv, out, "backlog_get_story", map[string]interface{}{"story_id": "US-001"})
	if isToolError(result) {
		t.Fatalf("get_story failed: %s", resultText(result))
	}
	tasks, _ := result["tasks"].([]interface{})
	if len(tasks) != 2 {
		t.Fatalf("expected 2 tasks in US-001, got %d", len(tasks))
	}
	modes := map[string]string{}
	for _, raw := range tasks {
		task := raw.(map[string]interface{})
		modes[task["id"].(string)] = task["mode"].(string)
	}
	if modes["T-US-001-001"] != "afk" || modes["T-US-001-002"] != "hitl" {
		t.Fatalf("unexpected task modes: %v", modes)
	}

	// Claim US-001 (a write, in read-only mode — the documented exception).
	result = callTool(t, srv, out, "backlog_claim_story", map[string]interface{}{"story_id": "US-001"})
	if isToolError(result) {
		t.Fatalf("claim failed: %s", resultText(result))
	}

	// Claiming a second story is refused: one story at a time.
	result = callTool(t, srv, out, "backlog_claim_story", map[string]interface{}{"story_id": "US-002"})
	if !isToolError(result) {
		t.Fatal("expected second claim to be refused")
	}
	if !strings.Contains(resultText(result), "one story at a time") {
		t.Fatalf("unexpected refusal message: %s", resultText(result))
	}

	// Re-claiming the held story is a no-op success.
	result = callTool(t, srv, out, "backlog_claim_story", map[string]interface{}{"story_id": "US-001"})
	if isToolError(result) {
		t.Fatalf("re-claim should succeed: %s", resultText(result))
	}

	// Completing the story is refused while tasks remain.
	result = callTool(t, srv, out, "backlog_complete_story", map[string]interface{}{"story_id": "US-001"})
	if !isToolError(result) {
		t.Fatal("expected complete_story to be refused with unfinished tasks")
	}

	// Complete both tasks, then the story.
	for _, id := range []string{"T-US-001-001", "T-US-001-002"} {
		result = callTool(t, srv, out, "backlog_complete_task", map[string]interface{}{"task_id": id})
		if isToolError(result) {
			t.Fatalf("complete_task %s failed: %s", id, resultText(result))
		}
	}
	result = callTool(t, srv, out, "backlog_complete_story", map[string]interface{}{"story_id": "US-001"})
	if isToolError(result) {
		t.Fatalf("complete_story failed: %s", resultText(result))
	}

	// US-002 is claimable now.
	result = callTool(t, srv, out, "backlog_claim_story", map[string]interface{}{"story_id": "US-002"})
	if isToolError(result) {
		t.Fatalf("claim of US-002 after completing US-001 failed: %s", resultText(result))
	}
}

// TestBacklogSeesExternalWrites proves handlers never serve a stale snapshot:
// a separate Manager instance (the daemon, in production) mutates the DB after
// the server has already loaded once.
func TestBacklogSeesExternalWrites(t *testing.T) {
	projDir := t.TempDir()
	external := seedBacklog(t, projDir)
	srv, out := newBacklogTestServer(t, projDir)

	// Prime the server's manager with a first read.
	result := callTool(t, srv, out, "backlog_list_stories", nil)
	stories, _ := result["stories"].([]interface{})
	if len(stories) != 2 {
		t.Fatalf("expected 2 stories before external write, got %d", len(stories))
	}

	// External process adds a story.
	if err := external.CreateStory(&release.Story{ID: "US-003", Title: "Added externally", Status: release.StoryStatusPending}); err != nil {
		t.Fatalf("external CreateStory: %v", err)
	}

	// The server must see it on the next call.
	result = callTool(t, srv, out, "backlog_list_stories", nil)
	stories, _ = result["stories"].([]interface{})
	if len(stories) != 3 {
		t.Fatalf("expected 3 stories after external write, got %d — stale cache", len(stories))
	}
}

// TestBacklogEmptyProject verifies graceful behavior with no plan yet.
func TestBacklogEmptyProject(t *testing.T) {
	projDir := t.TempDir()
	srv, out := newBacklogTestServer(t, projDir)

	result := callTool(t, srv, out, "backlog_list_stories", nil)
	if isToolError(result) {
		t.Fatalf("list on empty project should not error: %s", resultText(result))
	}
	if !strings.Contains(resultText(result), "empty") {
		t.Fatalf("expected empty-backlog message, got: %s", resultText(result))
	}
}

// TestMemoryRead verifies merged project memory is exposed.
func TestMemoryRead(t *testing.T) {
	projDir := t.TempDir()
	srv, out := newBacklogTestServer(t, projDir)

	// No memory yet.
	result := callTool(t, srv, out, "memory_read", nil)
	if isToolError(result) {
		t.Fatalf("memory_read failed: %s", resultText(result))
	}
	if !strings.Contains(resultText(result), "No memory") {
		t.Fatalf("expected no-memory message, got: %s", resultText(result))
	}

	// Project-layer memory file is picked up.
	memDir := filepath.Join(projDir, ".openexec")
	if err := os.MkdirAll(memDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(memDir, "MEMORY.md"), []byte("- Prefer table-driven tests"), 0o644); err != nil {
		t.Fatalf("write memory: %v", err)
	}

	result = callTool(t, srv, out, "memory_read", nil)
	if !strings.Contains(resultText(result), "table-driven tests") {
		t.Fatalf("expected memory content, got: %s", resultText(result))
	}
}

// TestSkillPropose verifies the propose-then-approve seam over MCP: proposals
// land as candidates (registry-invisible) and hostile names are rejected.
func TestSkillPropose(t *testing.T) {
	projDir := t.TempDir()
	srv, out := newBacklogTestServer(t, projDir)

	result := callTool(t, srv, out, "skill_propose", map[string]interface{}{
		"name":        "pg-testcontainer-setup",
		"description": "How this repo boots Postgres testcontainers",
		"when_to_use": "When writing integration tests that need a database",
		"content":     "## Rule\nReuse the shared container helper in tests/db.",
	})
	if isToolError(result) {
		t.Fatalf("skill_propose failed: %s", resultText(result))
	}
	if active, _ := result["active"].(bool); active {
		t.Fatal("proposal must not be active")
	}
	candidate := filepath.Join(projDir, ".openexec", "skills", "_candidates", "pg-testcontainer-setup", "SKILL.md")
	if _, err := os.Stat(candidate); err != nil {
		t.Fatalf("candidate file not written: %v", err)
	}

	// Hostile name → tool error, nothing written outside _candidates.
	result = callTool(t, srv, out, "skill_propose", map[string]interface{}{
		"name":        "../../evil",
		"description": "x",
		"content":     "x",
	})
	if !isToolError(result) {
		t.Fatal("expected hostile skill name to be rejected")
	}
}

// TestBacklogPhaseReporting: the list response carries the project phase so
// clients (and the UI) can default to the right lane.
func TestBacklogPhaseReporting(t *testing.T) {
	projDir := t.TempDir()
	external := seedBacklog(t, projDir)
	srv, out := newBacklogTestServer(t, projDir)

	// Plan exists, nothing done yet.
	result := callTool(t, srv, out, "backlog_list_stories", nil)
	if result["phase"] != "planned" {
		t.Fatalf("expected phase=planned, got %v", result["phase"])
	}

	// External completion of the whole backlog → maintaining + light-mode hint.
	for _, id := range []string{"T-US-001-001", "T-US-001-002", "T-US-002-001"} {
		if err := external.SetTaskStatus(id, release.TaskStatusDone); err != nil {
			t.Fatalf("SetTaskStatus: %v", err)
		}
	}
	for _, id := range []string{"US-001", "US-002"} {
		if err := external.SetStoryStatus(id, release.StoryStatusDone); err != nil {
			t.Fatalf("SetStoryStatus: %v", err)
		}
	}

	result = callTool(t, srv, out, "backlog_list_stories", nil)
	if result["phase"] != "maintaining" {
		t.Fatalf("expected phase=maintaining, got %v", result["phase"])
	}
	if !strings.Contains(resultText(result), "Initial build is complete") {
		t.Fatalf("expected light-mode guidance in maintaining phase, got: %s", resultText(result))
	}
}

// TestBacklogAddTask: surgical work filed from light mode lands in the rolling
// maintenance story without disturbing phase or the one-story claim rule.
func TestBacklogAddTask(t *testing.T) {
	projDir := t.TempDir()
	external := seedBacklog(t, projDir)
	srv, out := newBacklogTestServer(t, projDir)

	// Complete the whole initial backlog → maintaining.
	for _, id := range []string{"T-US-001-001", "T-US-001-002", "T-US-002-001"} {
		_ = external.SetTaskStatus(id, release.TaskStatusDone)
	}
	for _, id := range []string{"US-001", "US-002"} {
		_ = external.SetStoryStatus(id, release.StoryStatusDone)
	}

	// File a surgical task (default mode hitl).
	result := callTool(t, srv, out, "backlog_add_task", map[string]interface{}{
		"title":       "Fix retention query off-by-one",
		"description": "Week-4 window starts a day late.",
	})
	if isToolError(result) {
		t.Fatalf("add_task failed: %s", resultText(result))
	}
	taskID, _ := result["task_id"].(string)
	if taskID == "" || result["story_id"] != "US-MAINT" {
		t.Fatalf("unexpected add_task result: %v", result)
	}
	if result["mode"] != "hitl" {
		t.Fatalf("default mode must be hitl, got %v", result["mode"])
	}

	// Phase must stay maintaining despite the new pending task.
	result = callTool(t, srv, out, "backlog_list_stories", nil)
	if result["phase"] != "maintaining" {
		t.Fatalf("maintenance task must not change phase, got %v", result["phase"])
	}

	// The open maintenance story must not block claiming a real story.
	if err := external.CreateStory(&release.Story{ID: "US-003", Title: "New epic story", Status: release.StoryStatusPending}); err != nil {
		t.Fatalf("CreateStory: %v", err)
	}
	result = callTool(t, srv, out, "backlog_claim_story", map[string]interface{}{"story_id": "US-003"})
	if isToolError(result) {
		t.Fatalf("maintenance story blocked a real claim: %s", resultText(result))
	}

	// Maintenance story itself never completes.
	result = callTool(t, srv, out, "backlog_complete_task", map[string]interface{}{"task_id": taskID})
	if isToolError(result) {
		t.Fatalf("complete maintenance task failed: %s", resultText(result))
	}
	result = callTool(t, srv, out, "backlog_complete_story", map[string]interface{}{"story_id": "US-MAINT"})
	if !isToolError(result) {
		t.Fatal("completing the maintenance story must be refused")
	}

	// Second add gets the next free ID.
	result = callTool(t, srv, out, "backlog_add_task", map[string]interface{}{"title": "Another fix", "mode": "afk"})
	if isToolError(result) {
		t.Fatalf("second add_task failed: %s", resultText(result))
	}
	if result["task_id"] == taskID {
		t.Fatal("task IDs must not collide")
	}
	if result["mode"] != "afk" {
		t.Fatalf("explicit afk mode not honored, got %v", result["mode"])
	}
}

// TestBacklogHitlPendingSurfaced: pending hitl tasks block maintaining and the
// listing must say so.
func TestBacklogHitlPendingSurfaced(t *testing.T) {
	projDir := t.TempDir()
	external := seedBacklog(t, projDir) // T-US-001-002 is hitl
	srv, out := newBacklogTestServer(t, projDir)

	// Heavy run finished everything except the hitl task.
	_ = external.SetTaskStatus("T-US-001-001", release.TaskStatusDone)
	_ = external.SetTaskStatus("T-US-002-001", release.TaskStatusDone)
	_ = external.SetStoryStatus("US-002", release.StoryStatusDone)

	result := callTool(t, srv, out, "backlog_list_stories", nil)
	hitl, _ := result["hitl_pending"].(float64)
	if int(hitl) != 1 {
		t.Fatalf("expected hitl_pending=1, got %v", result["hitl_pending"])
	}
	if !strings.Contains(resultText(result), "await a human") {
		t.Fatalf("expected hitl handoff guidance, got: %s", resultText(result))
	}
}
