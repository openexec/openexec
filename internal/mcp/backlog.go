package mcp

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/openexec/openexec/internal/memory"
	"github.com/openexec/openexec/internal/release"
)

// errNoBacklog indicates the workspace has no .openexec project directory —
// no plan has ever been generated here.
var errNoBacklog = errors.New("no OpenExec project in this workspace — run `openexec init` and `openexec plan` to create a backlog")

// Backlog tools expose the OpenExec story/task backlog to external MCP
// clients (e.g. Claude Code acting as the lightweight interactive client),
// without booting the daemon. The heavy pipeline writes the backlog; the
// light client reads it and works one story at a time.
//
// NOTE ON SIDE EFFECTS: backlog_claim_story / backlog_complete_task /
// backlog_complete_story mutate orchestrator bookkeeping (the .openexec
// database), NOT workspace files. They are deliberately allowed in every
// permission mode, including read-only chat — this is the documented
// light-mode exception to "chat has no side effects".

// backlogManager lazily opens the release store for backlog tools.
//
// The Manager caches state in memory, so every handler must call Load()
// before reading: mcp-serve is a long-lived stdio server, and other
// processes (the daemon, `openexec run`) write the same database.
func (s *Server) backlogManager() (*release.Manager, error) {
	s.backlogMu.Lock()
	defer s.backlogMu.Unlock()

	if s.backlogMgr != nil {
		return s.backlogMgr, nil
	}
	if len(s.workspaceRoots) == 0 || s.workspaceRoots[0] == "" {
		return nil, fmt.Errorf("no workspace root configured")
	}
	// A workspace without .openexec has never been initialized: report that
	// clearly instead of failing to create a database in a missing directory.
	if _, err := os.Stat(filepath.Join(s.workspaceRoots[0], ".openexec")); os.IsNotExist(err) {
		return nil, errNoBacklog
	}
	// DefaultConfig keeps git integration off — backlog tools only manage
	// story/task state, never branches.
	mgr, err := release.NewManager(s.workspaceRoots[0], nil)
	if err != nil {
		return nil, fmt.Errorf("open backlog store: %w", err)
	}
	s.backlogMgr = mgr
	return mgr, nil
}

// loadedBacklog returns the release manager with a fresh Load() so handlers
// never serve a stale snapshot.
func (s *Server) loadedBacklog() (*release.Manager, error) {
	mgr, err := s.backlogManager()
	if err != nil {
		return nil, err
	}
	if err := mgr.Load(); err != nil {
		return nil, fmt.Errorf("load backlog: %w", err)
	}
	return mgr, nil
}

// --- Tool definitions ---

func BacklogListStoriesToolDef() map[string]interface{} {
	return map[string]interface{}{
		"name": "backlog_list_stories",
		"description": "List stories in the OpenExec backlog with id, title, status, dependencies, and per-status task counts. Use this to see what work exists and what to pick up next. An empty list means no plan has been generated for this project yet.",
		"inputSchema": map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"status": map[string]interface{}{
					"type":        "string",
					"enum":        []string{"pending", "in_progress", "ready_for_review", "approved", "done"},
					"description": "Only return stories with this lifecycle status. Omit to list all stories.",
				},
			},
		},
	}
}

func BacklogGetStoryToolDef() map[string]interface{} {
	return map[string]interface{}{
		"name": "backlog_get_story",
		"description": "Get one story in full: description, acceptance criteria, verification script, dependencies, and every task with its status, execution mode (afk = agent-runnable, hitl = needs a human), and verification script. Call this before starting work on a story.",
		"inputSchema": map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"story_id": map[string]interface{}{
					"type":        "string",
					"description": "Story identifier, e.g. \"US-001\".",
				},
			},
			"required": []string{"story_id"},
		},
	}
}

func BacklogClaimStoryToolDef() map[string]interface{} {
	return map[string]interface{}{
		"name": "backlog_claim_story",
		"description": "Claim a story to work on: marks it in_progress. Enforces one story in progress at a time — the call is refused if a different story is already in_progress. Claiming is advisory coordination over the shared backlog database, not a lock; if the OpenExec daemon is also executing work, coordinate through story status. Re-claiming the story you already hold is a no-op.",
		"inputSchema": map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"story_id": map[string]interface{}{
					"type":        "string",
					"description": "Story identifier to claim, e.g. \"US-001\".",
				},
			},
			"required": []string{"story_id"},
		},
	}
}

func BacklogCompleteTaskToolDef() map[string]interface{} {
	return map[string]interface{}{
		"name": "backlog_complete_task",
		"description": "Mark one task as done after its work is finished and its verification script (if any) passes. Complete tasks individually as you finish them so progress is visible to the orchestrator and other clients.",
		"inputSchema": map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"task_id": map[string]interface{}{
					"type":        "string",
					"description": "Task identifier, e.g. \"T-US-001-002\".",
				},
			},
			"required": []string{"task_id"},
		},
	}
}

func BacklogCompleteStoryToolDef() map[string]interface{} {
	return map[string]interface{}{
		"name": "backlog_complete_story",
		"description": "Mark a story as done. Refused unless every task in the story is done or approved — complete the remaining tasks first (the error lists them). Call this when the story's acceptance criteria are met.",
		"inputSchema": map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"story_id": map[string]interface{}{
					"type":        "string",
					"description": "Story identifier to complete, e.g. \"US-001\".",
				},
			},
			"required": []string{"story_id"},
		},
	}
}

func MemoryReadToolDef() map[string]interface{} {
	return map[string]interface{}{
		"name": "memory_read",
		"description": "Read OpenExec's merged layered memory for this project (managed → user → project → local, later layers override earlier ones). Contains decisions, patterns, and preferences learned in previous runs. Returns empty content when no memory exists yet.",
		"inputSchema": map[string]interface{}{
			"type":       "object",
			"properties": map[string]interface{}{},
		},
	}
}

// --- Handlers ---

type storyIDArgs struct {
	StoryID string `json:"story_id"`
}

type taskIDArgs struct {
	TaskID string `json:"task_id"`
}

func taskSummary(t *release.Task) map[string]interface{} {
	return map[string]interface{}{
		"id":                  t.ID,
		"title":               t.Title,
		"description":         t.Description,
		"status":              t.Status,
		"mode":                t.ExecutionMode(),
		"depends_on":          t.DependsOn,
		"verification_script": t.VerificationScript,
	}
}

func storySummary(mgr *release.Manager, st *release.Story) map[string]interface{} {
	counts := map[string]int{}
	for _, t := range mgr.GetTasksForStory(st.ID) {
		counts[t.Status]++
	}
	return map[string]interface{}{
		"id":          st.ID,
		"title":       st.Title,
		"status":      st.Status,
		"depends_on":  st.DependsOn,
		"task_counts": counts,
	}
}

func (s *Server) handleBacklogListStories(req Request, params toolsCallParams) {
	var args struct {
		Status string `json:"status"`
	}
	if len(params.Arguments) > 0 {
		if err := json.Unmarshal(params.Arguments, &args); err != nil {
			s.writeError(req.ID, -32602, fmt.Sprintf("invalid backlog_list_stories arguments: %v", err))
			return
		}
	}

	mgr, err := s.loadedBacklog()
	if errors.Is(err, errNoBacklog) {
		// Listing an uninitialized workspace is not an error — there is
		// simply nothing to work on yet.
		s.writeResult(req.ID, map[string]interface{}{
			"content": []interface{}{
				map[string]interface{}{"type": "text", "text": "Backlog is empty — no OpenExec project has been initialized in this workspace yet."},
			},
			"stories": []interface{}{},
		})
		return
	}
	if err != nil {
		s.writeToolError(req.ID, err.Error())
		return
	}

	var stories []map[string]interface{}
	var lines []string
	for _, st := range mgr.GetStories() {
		if args.Status != "" && st.Status != args.Status {
			continue
		}
		stories = append(stories, storySummary(mgr, st))
		lines = append(lines, fmt.Sprintf("- %s [%s] %s", st.ID, st.Status, st.Title))
	}

	phase := mgr.Phase()

	text := fmt.Sprintf("%d stories (project phase: %s)", len(stories), phase)
	if len(lines) > 0 {
		text += "\n" + strings.Join(lines, "\n")
	} else if args.Status == "" {
		text = "Backlog is empty — no plan has been generated for this project yet."
	}
	if phase == release.PhaseMaintaining {
		text += "\nInitial build is complete: the heavy pipeline has worked off the backlog. New small tasks fit this light mode; reach for `openexec run` only for the next big feature or refactor."
	}

	s.writeResult(req.ID, map[string]interface{}{
		"content": []interface{}{
			map[string]interface{}{"type": "text", "text": text},
		},
		"stories": stories,
		"phase":   phase,
	})
}

func (s *Server) handleBacklogGetStory(req Request, params toolsCallParams) {
	var args storyIDArgs
	if err := json.Unmarshal(params.Arguments, &args); err != nil || args.StoryID == "" {
		s.writeError(req.ID, -32602, "backlog_get_story requires story_id")
		return
	}

	mgr, err := s.loadedBacklog()
	if err != nil {
		s.writeToolError(req.ID, err.Error())
		return
	}

	st := mgr.GetStory(args.StoryID)
	if st == nil {
		s.writeToolError(req.ID, fmt.Sprintf("story %s not found", args.StoryID))
		return
	}

	var tasks []map[string]interface{}
	var lines []string
	for _, t := range mgr.GetTasksForStory(st.ID) {
		tasks = append(tasks, taskSummary(t))
		lines = append(lines, fmt.Sprintf("- %s [%s/%s] %s", t.ID, t.Status, t.ExecutionMode(), t.Title))
	}

	text := fmt.Sprintf("%s [%s] %s\n%s\n\nTasks:\n%s",
		st.ID, st.Status, st.Title, st.Description, strings.Join(lines, "\n"))

	s.writeResult(req.ID, map[string]interface{}{
		"content": []interface{}{
			map[string]interface{}{"type": "text", "text": text},
		},
		"story": map[string]interface{}{
			"id":                  st.ID,
			"title":               st.Title,
			"description":         st.Description,
			"status":              st.Status,
			"depends_on":          st.DependsOn,
			"acceptance_criteria": st.AcceptanceCriteria,
			"verification_script": st.VerificationScript,
		},
		"tasks": tasks,
	})
}

func (s *Server) handleBacklogClaimStory(req Request, params toolsCallParams) {
	var args storyIDArgs
	if err := json.Unmarshal(params.Arguments, &args); err != nil || args.StoryID == "" {
		s.writeError(req.ID, -32602, "backlog_claim_story requires story_id")
		return
	}

	mgr, err := s.loadedBacklog()
	if err != nil {
		s.writeToolError(req.ID, err.Error())
		return
	}

	st := mgr.GetStory(args.StoryID)
	if st == nil {
		s.writeToolError(req.ID, fmt.Sprintf("story %s not found", args.StoryID))
		return
	}

	switch st.Status {
	case release.StoryStatusDone, release.StoryStatusApproved:
		s.writeToolError(req.ID, fmt.Sprintf("story %s is already %s", st.ID, st.Status))
		return
	}

	// One story at a time: refuse when a different story is already claimed.
	for _, other := range mgr.GetStories() {
		if other.ID != st.ID && other.Status == release.StoryStatusInProgress {
			s.writeToolError(req.ID, fmt.Sprintf(
				"story %s (%q) is already in progress — complete it before claiming %s (one story at a time)",
				other.ID, other.Title, st.ID))
			return
		}
	}

	if st.Status != release.StoryStatusInProgress {
		if err := mgr.SetStoryStatus(st.ID, release.StoryStatusInProgress); err != nil {
			s.writeToolError(req.ID, fmt.Sprintf("failed to claim story: %v", err))
			return
		}
	}

	var lines []string
	var tasks []map[string]interface{}
	for _, t := range mgr.GetTasksForStory(st.ID) {
		tasks = append(tasks, taskSummary(t))
		lines = append(lines, fmt.Sprintf("- %s [%s/%s] %s", t.ID, t.Status, t.ExecutionMode(), t.Title))
	}

	s.writeResult(req.ID, map[string]interface{}{
		"content": []interface{}{
			map[string]interface{}{
				"type": "text",
				"text": fmt.Sprintf("Claimed %s (%s). Tasks:\n%s", st.ID, st.Title, strings.Join(lines, "\n")),
			},
		},
		"story_id": st.ID,
		"status":   release.StoryStatusInProgress,
		"tasks":    tasks,
	})
}

func (s *Server) handleBacklogCompleteTask(req Request, params toolsCallParams) {
	var args taskIDArgs
	if err := json.Unmarshal(params.Arguments, &args); err != nil || args.TaskID == "" {
		s.writeError(req.ID, -32602, "backlog_complete_task requires task_id")
		return
	}

	mgr, err := s.loadedBacklog()
	if err != nil {
		s.writeToolError(req.ID, err.Error())
		return
	}

	t := mgr.GetTask(args.TaskID)
	if t == nil {
		s.writeToolError(req.ID, fmt.Sprintf("task %s not found", args.TaskID))
		return
	}

	if err := mgr.SetTaskStatus(t.ID, release.TaskStatusDone); err != nil {
		s.writeToolError(req.ID, fmt.Sprintf("failed to complete task: %v", err))
		return
	}

	s.writeResult(req.ID, map[string]interface{}{
		"content": []interface{}{
			map[string]interface{}{
				"type": "text",
				"text": fmt.Sprintf("Task %s marked done.", t.ID),
			},
		},
		"task_id": t.ID,
		"status":  release.TaskStatusDone,
	})
}

func (s *Server) handleBacklogCompleteStory(req Request, params toolsCallParams) {
	var args storyIDArgs
	if err := json.Unmarshal(params.Arguments, &args); err != nil || args.StoryID == "" {
		s.writeError(req.ID, -32602, "backlog_complete_story requires story_id")
		return
	}

	mgr, err := s.loadedBacklog()
	if err != nil {
		s.writeToolError(req.ID, err.Error())
		return
	}

	st := mgr.GetStory(args.StoryID)
	if st == nil {
		s.writeToolError(req.ID, fmt.Sprintf("story %s not found", args.StoryID))
		return
	}

	var remaining []string
	for _, t := range mgr.GetTasksForStory(st.ID) {
		if t.Status != release.TaskStatusDone && t.Status != release.TaskStatusApproved {
			remaining = append(remaining, fmt.Sprintf("%s [%s]", t.ID, t.Status))
		}
	}
	if len(remaining) > 0 {
		s.writeToolError(req.ID, fmt.Sprintf(
			"story %s has unfinished tasks: %s — complete them first",
			st.ID, strings.Join(remaining, ", ")))
		return
	}

	if err := mgr.SetStoryStatus(st.ID, release.StoryStatusDone); err != nil {
		s.writeToolError(req.ID, fmt.Sprintf("failed to complete story: %v", err))
		return
	}

	s.writeResult(req.ID, map[string]interface{}{
		"content": []interface{}{
			map[string]interface{}{
				"type": "text",
				"text": fmt.Sprintf("Story %s marked done.", st.ID),
			},
		},
		"story_id": st.ID,
		"status":   release.StoryStatusDone,
	})
}

func (s *Server) handleMemoryRead(req Request, params toolsCallParams) {
	if len(s.workspaceRoots) == 0 || s.workspaceRoots[0] == "" {
		s.writeToolError(req.ID, "no workspace root configured")
		return
	}

	ms := memory.NewMemorySystem(s.workspaceRoots[0])
	merged, err := ms.LoadMerged()
	if err != nil {
		s.writeToolError(req.ID, fmt.Sprintf("failed to load memory: %v", err))
		return
	}

	text := merged
	if strings.TrimSpace(text) == "" {
		text = "No memory recorded for this project yet."
	}

	s.writeResult(req.ID, map[string]interface{}{
		"content": []interface{}{
			map[string]interface{}{"type": "text", "text": text},
		},
	})
}
