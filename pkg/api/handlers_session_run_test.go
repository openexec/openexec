package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/openexec/openexec/pkg/db/session"
)

// mockSessionRepo implements a minimal session.Repository for testing.
type mockSessionRepo struct {
	sessions map[string]*session.Session
	messages map[string][]*session.Message
}

func newMockSessionRepo() *mockSessionRepo {
	return &mockSessionRepo{
		sessions: make(map[string]*session.Session),
		messages: make(map[string][]*session.Message),
	}
}

func (m *mockSessionRepo) GetSession(ctx context.Context, id string) (*session.Session, error) {
	if s, ok := m.sessions[id]; ok {
		return s, nil
	}
	return nil, session.ErrSessionNotFound
}

func (m *mockSessionRepo) GetFullConversationHistory(ctx context.Context, sessionID string) ([]*session.Message, error) {
	if msgs, ok := m.messages[sessionID]; ok {
		return msgs, nil
	}
	return []*session.Message{}, nil
}

// Stub implementations for other Repository methods
func (m *mockSessionRepo) CreateSession(ctx context.Context, s *session.Session) error     { return nil }
func (m *mockSessionRepo) UpdateSession(ctx context.Context, s *session.Session) error     { return nil }
func (m *mockSessionRepo) DeleteSession(ctx context.Context, id string) error              { return nil }
func (m *mockSessionRepo) ListSessions(ctx context.Context, opts *session.ListSessionsOptions) ([]*session.Session, error) {
	return nil, nil
}
func (m *mockSessionRepo) ListSessionsByProject(ctx context.Context, projectPath string) ([]*session.Session, error) {
	return nil, nil
}
func (m *mockSessionRepo) GetSessionForks(ctx context.Context, sessionID string) ([]*session.Session, error) {
	return nil, nil
}
func (m *mockSessionRepo) ForkSession(ctx context.Context, parentSessionID string, opts *session.ForkOptions) (*session.Session, error) {
	return nil, nil
}
func (m *mockSessionRepo) GetForkInfo(ctx context.Context, sessionID string) (*session.ForkInfo, error) {
	return nil, nil
}
func (m *mockSessionRepo) GetAncestorChain(ctx context.Context, sessionID string) ([]*session.Session, error) {
	return nil, nil
}
func (m *mockSessionRepo) GetRootSession(ctx context.Context, sessionID string) (*session.Session, error) {
	return nil, nil
}
func (m *mockSessionRepo) ListDescendants(ctx context.Context, sessionID string) ([]*session.Session, error) {
	return nil, nil
}
func (m *mockSessionRepo) IsDescendantOf(ctx context.Context, childSessionID, ancestorSessionID string) (bool, error) {
	return false, nil
}
func (m *mockSessionRepo) CreateMessage(ctx context.Context, msg *session.Message) error   { return nil }
func (m *mockSessionRepo) GetMessage(ctx context.Context, id string) (*session.Message, error) {
	return nil, nil
}
func (m *mockSessionRepo) UpdateMessage(ctx context.Context, msg *session.Message) error   { return nil }
func (m *mockSessionRepo) DeleteMessage(ctx context.Context, id string) error              { return nil }
func (m *mockSessionRepo) ListMessages(ctx context.Context, sessionID string) ([]*session.Message, error) {
	return m.messages[sessionID], nil
}
func (m *mockSessionRepo) ListMessagesByRole(ctx context.Context, sessionID string, role session.Role) ([]*session.Message, error) {
	return nil, nil
}
func (m *mockSessionRepo) GetMessageCount(ctx context.Context, sessionID string) (int, error) {
	return len(m.messages[sessionID]), nil
}
func (m *mockSessionRepo) ListMessagesUpTo(ctx context.Context, sessionID, upToMessageID string) ([]*session.Message, error) {
	return nil, nil
}
func (m *mockSessionRepo) CreateToolCall(ctx context.Context, tc *session.ToolCall) error  { return nil }
func (m *mockSessionRepo) GetToolCall(ctx context.Context, id string) (*session.ToolCall, error) {
	return nil, nil
}
func (m *mockSessionRepo) UpdateToolCall(ctx context.Context, tc *session.ToolCall) error  { return nil }
func (m *mockSessionRepo) DeleteToolCall(ctx context.Context, id string) error             { return nil }
func (m *mockSessionRepo) ListToolCalls(ctx context.Context, sessionID string) ([]*session.ToolCall, error) {
	return nil, nil
}
func (m *mockSessionRepo) ListToolCallsByMessage(ctx context.Context, messageID string) ([]*session.ToolCall, error) {
	return nil, nil
}
func (m *mockSessionRepo) ListToolCallsByStatus(ctx context.Context, sessionID string, status session.ToolCallStatus) ([]*session.ToolCall, error) {
	return nil, nil
}
func (m *mockSessionRepo) ListPendingApprovals(ctx context.Context, sessionID string) ([]*session.ToolCall, error) {
	return nil, nil
}
func (m *mockSessionRepo) CreateSummary(ctx context.Context, sum *session.SessionSummary) error {
	return nil
}
func (m *mockSessionRepo) GetSummary(ctx context.Context, id string) (*session.SessionSummary, error) {
	return nil, nil
}
func (m *mockSessionRepo) GetLatestSummary(ctx context.Context, sessionID string) (*session.SessionSummary, error) {
	return nil, nil
}
func (m *mockSessionRepo) ListSummaries(ctx context.Context, sessionID string) ([]*session.SessionSummary, error) {
	return nil, nil
}
func (m *mockSessionRepo) DeleteSummary(ctx context.Context, id string) error {
	return nil
}
func (m *mockSessionRepo) GetSessionStats(ctx context.Context, sessionID string) (*session.SessionStats, error) {
	return nil, nil
}
func (m *mockSessionRepo) GetUsageByProvider(ctx context.Context) ([]*session.ProviderUsage, error) {
	return nil, nil
}
func (m *mockSessionRepo) Close() error { return nil }

func TestHandleStartRunFromSession_Success(t *testing.T) {
	repo := newMockSessionRepo()

	// Create a test session with messages
	sess := &session.Session{
		ID:          "test-session-1",
		ProjectPath: "/test/project",
		Provider:    "openai",
		Model:       "gpt-4",
		Status:      session.StatusActive,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	repo.sessions[sess.ID] = sess
	repo.messages[sess.ID] = []*session.Message{
		{ID: "msg-1", SessionID: sess.ID, Role: session.RoleUser, Content: "Hello"},
		{ID: "msg-2", SessionID: sess.ID, Role: session.RoleAssistant, Content: "Hi there"},
		{ID: "msg-3", SessionID: sess.ID, Role: session.RoleUser, Content: "Add user authentication to the app"},
	}

	// Create server with mock repo (no manager, so Start will fail, but we can test the handler logic)
	s := &Server{
		SessionRepo: repo,
	}

	// Test with explicit task_description
	t.Run("with explicit task_description", func(t *testing.T) {
		body := map[string]interface{}{
			"task_description": "Implement login feature",
			"blueprint_id":     "quick_fix",
			"mode":             "read-only",
		}
		jsonBody, _ := json.Marshal(body)

		req := httptest.NewRequest("POST", "/api/v1/sessions/test-session-1/run", bytes.NewReader(jsonBody))
		req.SetPathValue("id", "test-session-1")
		w := httptest.NewRecorder()

		s.handleStartRunFromSession(w, req)

		// Should get 503 Service Unavailable (manager not available) rather than 400/404
		// This confirms validation passed
		if w.Code == http.StatusBadRequest || w.Code == http.StatusNotFound {
			t.Errorf("Expected to get past validation (503 for nil manager), got %d: %s", w.Code, w.Body.String())
		}
		if w.Code != http.StatusServiceUnavailable {
			t.Errorf("Expected 503 ServiceUnavailable for nil manager, got %d: %s", w.Code, w.Body.String())
		}
	})

	t.Run("derives task from session messages", func(t *testing.T) {
		body := map[string]interface{}{
			"blueprint_id": "standard_task",
		}
		jsonBody, _ := json.Marshal(body)

		req := httptest.NewRequest("POST", "/api/v1/sessions/test-session-1/run", bytes.NewReader(jsonBody))
		req.SetPathValue("id", "test-session-1")
		w := httptest.NewRecorder()

		s.handleStartRunFromSession(w, req)

		// Should get 503 (manager not available), confirming task derivation succeeded
		if w.Code == http.StatusBadRequest {
			t.Errorf("Expected to derive task description, got %d: %s", w.Code, w.Body.String())
		}
		if w.Code != http.StatusServiceUnavailable {
			t.Errorf("Expected 503 ServiceUnavailable for nil manager, got %d: %s", w.Code, w.Body.String())
		}
	})
}

func TestHandleStartRunFromSession_SessionNotFound(t *testing.T) {
	repo := newMockSessionRepo()

	s := &Server{
		SessionRepo: repo,
	}

	body := map[string]interface{}{
		"task_description": "Test task",
	}
	jsonBody, _ := json.Marshal(body)

	req := httptest.NewRequest("POST", "/api/v1/sessions/nonexistent/run", bytes.NewReader(jsonBody))
	req.SetPathValue("id", "nonexistent")
	w := httptest.NewRecorder()

	s.handleStartRunFromSession(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("Expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleStartRunFromSession_InvalidBlueprint(t *testing.T) {
	repo := newMockSessionRepo()
	repo.sessions["test-session"] = &session.Session{
		ID:        "test-session",
		Status:    session.StatusActive,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	s := &Server{
		SessionRepo: repo,
	}

	body := map[string]interface{}{
		"task_description": "Test task",
		"blueprint_id":     "invalid_blueprint",
	}
	jsonBody, _ := json.Marshal(body)

	req := httptest.NewRequest("POST", "/api/v1/sessions/test-session/run", bytes.NewReader(jsonBody))
	req.SetPathValue("id", "test-session")
	w := httptest.NewRecorder()

	s.handleStartRunFromSession(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected 400 for invalid blueprint, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleStartRunFromSession_InvalidMode(t *testing.T) {
	repo := newMockSessionRepo()
	repo.sessions["test-session"] = &session.Session{
		ID:        "test-session",
		Status:    session.StatusActive,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	s := &Server{
		SessionRepo: repo,
	}

	body := map[string]interface{}{
		"task_description": "Test task",
		"mode":             "invalid-mode",
	}
	jsonBody, _ := json.Marshal(body)

	req := httptest.NewRequest("POST", "/api/v1/sessions/test-session/run", bytes.NewReader(jsonBody))
	req.SetPathValue("id", "test-session")
	w := httptest.NewRecorder()

	s.handleStartRunFromSession(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected 400 for invalid mode, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleStartRunFromSession_NoTaskDescription(t *testing.T) {
	repo := newMockSessionRepo()
	repo.sessions["test-session"] = &session.Session{
		ID:        "test-session",
		Status:    session.StatusActive,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	// No messages, so no task can be derived
	repo.messages["test-session"] = []*session.Message{}

	s := &Server{
		SessionRepo: repo,
	}

	body := map[string]interface{}{}
	jsonBody, _ := json.Marshal(body)

	req := httptest.NewRequest("POST", "/api/v1/sessions/test-session/run", bytes.NewReader(jsonBody))
	req.SetPathValue("id", "test-session")
	w := httptest.NewRecorder()

	s.handleStartRunFromSession(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected 400 when no task can be derived, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleStartRunFromSession_MissingSessionID(t *testing.T) {
	s := &Server{}

	req := httptest.NewRequest("POST", "/api/v1/sessions//run", nil)
	req.SetPathValue("id", "")
	w := httptest.NewRecorder()

	s.handleStartRunFromSession(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected 400 for missing session ID, got %d", w.Code)
	}
}

func TestDeriveTaskFromSession(t *testing.T) {
	repo := newMockSessionRepo()
	repo.messages["test-session"] = []*session.Message{
		{ID: "msg-1", SessionID: "test-session", Role: session.RoleUser, Content: "First question"},
		{ID: "msg-2", SessionID: "test-session", Role: session.RoleAssistant, Content: "First answer"},
		{ID: "msg-3", SessionID: "test-session", Role: session.RoleUser, Content: "Second question"},
		{ID: "msg-4", SessionID: "test-session", Role: session.RoleAssistant, Content: "Second answer"},
		{ID: "msg-5", SessionID: "test-session", Role: session.RoleUser, Content: "Add user auth"},
	}

	t.Run("returns last user message when messages=0", func(t *testing.T) {
		task := deriveTaskFromSession(context.Background(), repo, "test-session", 0)
		if task != "Add user auth" {
			t.Errorf("Expected 'Add user auth', got %q", task)
		}
	})

	t.Run("returns last user message when messages=1", func(t *testing.T) {
		task := deriveTaskFromSession(context.Background(), repo, "test-session", 1)
		if task != "Add user auth" {
			t.Errorf("Expected 'Add user auth', got %q", task)
		}
	})

	t.Run("includes context when messages>1", func(t *testing.T) {
		task := deriveTaskFromSession(context.Background(), repo, "test-session", 3)
		if task == "" {
			t.Error("Expected non-empty task")
		}
		if len(task) <= len("Add user auth") {
			t.Error("Expected context to be included")
		}
	})

	t.Run("returns empty for no messages", func(t *testing.T) {
		repo.messages["empty-session"] = []*session.Message{}
		task := deriveTaskFromSession(context.Background(), repo, "empty-session", 0)
		if task != "" {
			t.Errorf("Expected empty task, got %q", task)
		}
	})

	t.Run("returns empty for only assistant messages", func(t *testing.T) {
		repo.messages["assistant-only"] = []*session.Message{
			{ID: "msg-1", SessionID: "assistant-only", Role: session.RoleAssistant, Content: "Hello"},
		}
		task := deriveTaskFromSession(context.Background(), repo, "assistant-only", 0)
		if task != "" {
			t.Errorf("Expected empty task, got %q", task)
		}
	})
}

func TestTruncateString(t *testing.T) {
	tests := []struct {
		input  string
		maxLen int
		want   string
	}{
		{"hello", 10, "hello"},
		{"hello world", 8, "hello..."},
		{"ab", 5, "ab"},
		{"", 5, ""},
		{"abc", 3, "abc"},
		{"abcd", 3, "abc"},
	}

	for _, tc := range tests {
		got := truncateString(tc.input, tc.maxLen)
		if got != tc.want {
			t.Errorf("truncateString(%q, %d) = %q, want %q", tc.input, tc.maxLen, got, tc.want)
		}
	}
}
