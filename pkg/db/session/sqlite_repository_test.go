package session

import (
	"context"
	"database/sql"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

func setupTestRepository(t *testing.T) *SQLiteRepository {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:?_foreign_keys=1")
	if err != nil {
		t.Fatalf("Failed to open in-memory database: %v", err)
	}

	repo, err := NewSQLiteRepository(db)
	if err != nil {
		t.Fatalf("Failed to create repository: %v", err)
	}

	return repo
}

func TestNewSQLiteRepository(t *testing.T) {
	t.Run("creates repository with valid db", func(t *testing.T) {
		db, _ := sql.Open("sqlite", ":memory:")
		defer db.Close()

		repo, err := NewSQLiteRepository(db)
		if err != nil {
			t.Errorf("NewSQLiteRepository() error = %v", err)
		}
		if repo == nil {
			t.Error("NewSQLiteRepository() returned nil")
		}
	})

	t.Run("returns error for nil db", func(t *testing.T) {
		_, err := NewSQLiteRepository(nil)
		if err == nil {
			t.Error("NewSQLiteRepository(nil) should return error")
		}
	})
}

// Session tests

func TestSQLiteRepository_CreateSession(t *testing.T) {
	repo := setupTestRepository(t)
	defer repo.Close()
	ctx := context.Background()

	t.Run("creates valid session", func(t *testing.T) {
		session, _ := NewSession("/path/to/project", "openai", "gpt-4")
		session.Title = "Test Session"

		err := repo.CreateSession(ctx, session)
		if err != nil {
			t.Errorf("CreateSession() error = %v", err)
		}

		// Verify session was created
		retrieved, err := repo.GetSession(ctx, session.ID)
		if err != nil {
			t.Errorf("GetSession() error = %v", err)
		}
		if retrieved.ID != session.ID {
			t.Errorf("GetSession() ID = %v, want %v", retrieved.ID, session.ID)
		}
		if retrieved.ProjectPath != session.ProjectPath {
			t.Errorf("GetSession() ProjectPath = %v, want %v", retrieved.ProjectPath, session.ProjectPath)
		}
		if retrieved.Title != session.Title {
			t.Errorf("GetSession() Title = %v, want %v", retrieved.Title, session.Title)
		}
	})

	t.Run("returns error for duplicate session", func(t *testing.T) {
		session, _ := NewSession("/path/to/project", "openai", "gpt-4")
		_ = repo.CreateSession(ctx, session)

		err := repo.CreateSession(ctx, session)
		if err != ErrSessionAlreadyExist {
			t.Errorf("CreateSession() error = %v, want %v", err, ErrSessionAlreadyExist)
		}
	})

	t.Run("returns error for invalid session", func(t *testing.T) {
		session := &Session{ID: "test-id"} // missing required fields
		err := repo.CreateSession(ctx, session)
		if err == nil {
			t.Error("CreateSession() should return error for invalid session")
		}
	})
}

func TestSQLiteRepository_GetSession(t *testing.T) {
	repo := setupTestRepository(t)
	defer repo.Close()
	ctx := context.Background()

	t.Run("returns session not found", func(t *testing.T) {
		_, err := repo.GetSession(ctx, "non-existent")
		if err != ErrSessionNotFound {
			t.Errorf("GetSession() error = %v, want %v", err, ErrSessionNotFound)
		}
	})
}

func TestSQLiteRepository_UpdateSession(t *testing.T) {
	repo := setupTestRepository(t)
	defer repo.Close()
	ctx := context.Background()

	t.Run("updates existing session", func(t *testing.T) {
		session, _ := NewSession("/path/to/project", "openai", "gpt-4")
		_ = repo.CreateSession(ctx, session)

		session.Title = "Updated Title"
		session.Status = StatusPaused
		session.UpdatedAt = time.Now().UTC()

		err := repo.UpdateSession(ctx, session)
		if err != nil {
			t.Errorf("UpdateSession() error = %v", err)
		}

		retrieved, _ := repo.GetSession(ctx, session.ID)
		if retrieved.Title != "Updated Title" {
			t.Errorf("UpdateSession() Title = %v, want %v", retrieved.Title, "Updated Title")
		}
		if retrieved.Status != StatusPaused {
			t.Errorf("UpdateSession() Status = %v, want %v", retrieved.Status, StatusPaused)
		}
	})

	t.Run("returns error for non-existent session", func(t *testing.T) {
		session, _ := NewSession("/path", "openai", "gpt-4")
		session.ID = "non-existent"

		err := repo.UpdateSession(ctx, session)
		if err != ErrSessionNotFound {
			t.Errorf("UpdateSession() error = %v, want %v", err, ErrSessionNotFound)
		}
	})
}

func TestSQLiteRepository_DeleteSession(t *testing.T) {
	repo := setupTestRepository(t)
	defer repo.Close()
	ctx := context.Background()

	t.Run("deletes existing session", func(t *testing.T) {
		session, _ := NewSession("/path/to/project", "openai", "gpt-4")
		_ = repo.CreateSession(ctx, session)

		err := repo.DeleteSession(ctx, session.ID)
		if err != nil {
			t.Errorf("DeleteSession() error = %v", err)
		}

		_, err = repo.GetSession(ctx, session.ID)
		if err != ErrSessionNotFound {
			t.Errorf("GetSession() after delete should return ErrSessionNotFound")
		}
	})

	t.Run("returns error for non-existent session", func(t *testing.T) {
		err := repo.DeleteSession(ctx, "non-existent")
		if err != ErrSessionNotFound {
			t.Errorf("DeleteSession() error = %v, want %v", err, ErrSessionNotFound)
		}
	})
}

func TestSQLiteRepository_ListSessions(t *testing.T) {
	repo := setupTestRepository(t)
	defer repo.Close()
	ctx := context.Background()

	t.Run("returns empty list when no sessions", func(t *testing.T) {
		sessions, err := repo.ListSessions(ctx, nil)
		if err != nil {
			t.Errorf("ListSessions() error = %v", err)
		}
		if len(sessions) != 0 {
			t.Errorf("ListSessions() count = %d, want 0", len(sessions))
		}
	})

	t.Run("returns all sessions", func(t *testing.T) {
		for i := 0; i < 3; i++ {
			session, _ := NewSession("/path", "openai", "gpt-4")
			_ = repo.CreateSession(ctx, session)
		}

		sessions, err := repo.ListSessions(ctx, nil)
		if err != nil {
			t.Errorf("ListSessions() error = %v", err)
		}
		if len(sessions) != 3 {
			t.Errorf("ListSessions() count = %d, want 3", len(sessions))
		}
	})

	t.Run("filters by status", func(t *testing.T) {
		session, _ := NewSession("/path", "anthropic", "claude-3")
		session.Status = StatusArchived
		_ = repo.CreateSession(ctx, session)

		opts := &ListSessionsOptions{Status: StatusArchived}
		sessions, err := repo.ListSessions(ctx, opts)
		if err != nil {
			t.Errorf("ListSessions() error = %v", err)
		}
		if len(sessions) != 1 {
			t.Errorf("ListSessions() count = %d, want 1", len(sessions))
		}
	})

	t.Run("respects limit and offset", func(t *testing.T) {
		opts := &ListSessionsOptions{Limit: 2, Offset: 1}
		sessions, err := repo.ListSessions(ctx, opts)
		if err != nil {
			t.Errorf("ListSessions() error = %v", err)
		}
		if len(sessions) > 2 {
			t.Errorf("ListSessions() should respect limit")
		}
	})
}

func TestSQLiteRepository_ListSessionsByProject(t *testing.T) {
	repo := setupTestRepository(t)
	defer repo.Close()
	ctx := context.Background()

	session1, _ := NewSession("/project/a", "openai", "gpt-4")
	session2, _ := NewSession("/project/a", "openai", "gpt-4")
	session3, _ := NewSession("/project/b", "openai", "gpt-4")
	_ = repo.CreateSession(ctx, session1)
	_ = repo.CreateSession(ctx, session2)
	_ = repo.CreateSession(ctx, session3)

	sessions, err := repo.ListSessionsByProject(ctx, "/project/a")
	if err != nil {
		t.Errorf("ListSessionsByProject() error = %v", err)
	}
	if len(sessions) != 2 {
		t.Errorf("ListSessionsByProject() count = %d, want 2", len(sessions))
	}
}

func TestSQLiteRepository_GetSessionForks(t *testing.T) {
	repo := setupTestRepository(t)
	defer repo.Close()
	ctx := context.Background()

	parent, _ := NewSession("/path", "openai", "gpt-4")
	_ = repo.CreateSession(ctx, parent)

	fork1, _ := NewSession("/path", "openai", "gpt-4")
	fork1.ParentSessionID = sql.NullString{String: parent.ID, Valid: true}
	_ = repo.CreateSession(ctx, fork1)

	fork2, _ := NewSession("/path", "openai", "gpt-4")
	fork2.ParentSessionID = sql.NullString{String: parent.ID, Valid: true}
	_ = repo.CreateSession(ctx, fork2)

	forks, err := repo.GetSessionForks(ctx, parent.ID)
	if err != nil {
		t.Errorf("GetSessionForks() error = %v", err)
	}
	if len(forks) != 2 {
		t.Errorf("GetSessionForks() count = %d, want 2", len(forks))
	}
}

// Message tests

func TestSQLiteRepository_CreateMessage(t *testing.T) {
	repo := setupTestRepository(t)
	defer repo.Close()
	ctx := context.Background()

	session, _ := NewSession("/path", "openai", "gpt-4")
	_ = repo.CreateSession(ctx, session)

	t.Run("creates valid message", func(t *testing.T) {
		msg, _ := NewMessage(session.ID, RoleUser, "Hello")

		err := repo.CreateMessage(ctx, msg)
		if err != nil {
			t.Errorf("CreateMessage() error = %v", err)
		}

		retrieved, err := repo.GetMessage(ctx, msg.ID)
		if err != nil {
			t.Errorf("GetMessage() error = %v", err)
		}
		if retrieved.Content != "Hello" {
			t.Errorf("GetMessage() Content = %v, want %v", retrieved.Content, "Hello")
		}
	})

	t.Run("returns error for non-existent session", func(t *testing.T) {
		msg, _ := NewMessage("non-existent", RoleUser, "Hello")
		err := repo.CreateMessage(ctx, msg)
		if err != ErrSessionNotFound {
			t.Errorf("CreateMessage() error = %v, want %v", err, ErrSessionNotFound)
		}
	})
}

func TestSQLiteRepository_UpdateMessage(t *testing.T) {
	repo := setupTestRepository(t)
	defer repo.Close()
	ctx := context.Background()

	session, _ := NewSession("/path", "openai", "gpt-4")
	_ = repo.CreateSession(ctx, session)
	msg, _ := NewMessage(session.ID, RoleAssistant, "Hello")
	_ = repo.CreateMessage(ctx, msg)

	t.Run("updates token usage", func(t *testing.T) {
		msg.SetTokenUsage(100, 200, 0.05)
		err := repo.UpdateMessage(ctx, msg)
		if err != nil {
			t.Errorf("UpdateMessage() error = %v", err)
		}

		retrieved, _ := repo.GetMessage(ctx, msg.ID)
		if retrieved.TokensInput != 100 {
			t.Errorf("UpdateMessage() TokensInput = %v, want 100", retrieved.TokensInput)
		}
		if retrieved.TokensOutput != 200 {
			t.Errorf("UpdateMessage() TokensOutput = %v, want 200", retrieved.TokensOutput)
		}
	})

	t.Run("returns error for non-existent message", func(t *testing.T) {
		nonExistent, _ := NewMessage(session.ID, RoleUser, "test")
		nonExistent.ID = "non-existent"
		err := repo.UpdateMessage(ctx, nonExistent)
		if err != ErrMessageNotFound {
			t.Errorf("UpdateMessage() error = %v, want %v", err, ErrMessageNotFound)
		}
	})
}

func TestSQLiteRepository_DeleteMessage(t *testing.T) {
	repo := setupTestRepository(t)
	defer repo.Close()
	ctx := context.Background()

	session, _ := NewSession("/path", "openai", "gpt-4")
	_ = repo.CreateSession(ctx, session)
	msg, _ := NewMessage(session.ID, RoleUser, "Hello")
	_ = repo.CreateMessage(ctx, msg)

	t.Run("deletes existing message", func(t *testing.T) {
		err := repo.DeleteMessage(ctx, msg.ID)
		if err != nil {
			t.Errorf("DeleteMessage() error = %v", err)
		}

		_, err = repo.GetMessage(ctx, msg.ID)
		if err != ErrMessageNotFound {
			t.Error("GetMessage() should return ErrMessageNotFound after delete")
		}
	})

	t.Run("returns error for non-existent message", func(t *testing.T) {
		err := repo.DeleteMessage(ctx, "non-existent")
		if err != ErrMessageNotFound {
			t.Errorf("DeleteMessage() error = %v, want %v", err, ErrMessageNotFound)
		}
	})
}

func TestSQLiteRepository_ListMessages(t *testing.T) {
	repo := setupTestRepository(t)
	defer repo.Close()
	ctx := context.Background()

	session, _ := NewSession("/path", "openai", "gpt-4")
	_ = repo.CreateSession(ctx, session)

	msg1, _ := NewMessage(session.ID, RoleUser, "Hello")
	msg2, _ := NewMessage(session.ID, RoleAssistant, "Hi there")
	_ = repo.CreateMessage(ctx, msg1)
	time.Sleep(10 * time.Millisecond) // Ensure ordering
	_ = repo.CreateMessage(ctx, msg2)

	messages, err := repo.ListMessages(ctx, session.ID)
	if err != nil {
		t.Errorf("ListMessages() error = %v", err)
	}
	if len(messages) != 2 {
		t.Errorf("ListMessages() count = %d, want 2", len(messages))
	}
}

func TestSQLiteRepository_ListMessagesByRole(t *testing.T) {
	repo := setupTestRepository(t)
	defer repo.Close()
	ctx := context.Background()

	session, _ := NewSession("/path", "openai", "gpt-4")
	_ = repo.CreateSession(ctx, session)

	msg1, _ := NewMessage(session.ID, RoleUser, "Hello")
	msg2, _ := NewMessage(session.ID, RoleAssistant, "Hi")
	msg3, _ := NewMessage(session.ID, RoleUser, "How are you?")
	_ = repo.CreateMessage(ctx, msg1)
	_ = repo.CreateMessage(ctx, msg2)
	_ = repo.CreateMessage(ctx, msg3)

	userMessages, err := repo.ListMessagesByRole(ctx, session.ID, RoleUser)
	if err != nil {
		t.Errorf("ListMessagesByRole() error = %v", err)
	}
	if len(userMessages) != 2 {
		t.Errorf("ListMessagesByRole() count = %d, want 2", len(userMessages))
	}
}

func TestSQLiteRepository_GetMessageCount(t *testing.T) {
	repo := setupTestRepository(t)
	defer repo.Close()
	ctx := context.Background()

	session, _ := NewSession("/path", "openai", "gpt-4")
	_ = repo.CreateSession(ctx, session)

	for i := 0; i < 5; i++ {
		msg, _ := NewMessage(session.ID, RoleUser, "msg")
		_ = repo.CreateMessage(ctx, msg)
	}

	count, err := repo.GetMessageCount(ctx, session.ID)
	if err != nil {
		t.Errorf("GetMessageCount() error = %v", err)
	}
	if count != 5 {
		t.Errorf("GetMessageCount() = %d, want 5", count)
	}
}

// Tool call tests

func TestSQLiteRepository_CreateToolCall(t *testing.T) {
	repo := setupTestRepository(t)
	defer repo.Close()
	ctx := context.Background()

	session, _ := NewSession("/path", "openai", "gpt-4")
	_ = repo.CreateSession(ctx, session)
	msg, _ := NewMessage(session.ID, RoleAssistant, "Let me read that file")
	_ = repo.CreateMessage(ctx, msg)

	t.Run("creates valid tool call", func(t *testing.T) {
		tc, _ := NewToolCall(msg.ID, session.ID, "read_file", `{"path": "/test.txt"}`)

		err := repo.CreateToolCall(ctx, tc)
		if err != nil {
			t.Errorf("CreateToolCall() error = %v", err)
		}

		retrieved, err := repo.GetToolCall(ctx, tc.ID)
		if err != nil {
			t.Errorf("GetToolCall() error = %v", err)
		}
		if retrieved.ToolName != "read_file" {
			t.Errorf("GetToolCall() ToolName = %v, want %v", retrieved.ToolName, "read_file")
		}
	})

	t.Run("returns error for non-existent message", func(t *testing.T) {
		tc, _ := NewToolCall("non-existent", session.ID, "read_file", `{}`)
		err := repo.CreateToolCall(ctx, tc)
		if err != ErrMessageNotFound {
			t.Errorf("CreateToolCall() error = %v, want %v", err, ErrMessageNotFound)
		}
	})
}

func TestSQLiteRepository_UpdateToolCall(t *testing.T) {
	repo := setupTestRepository(t)
	defer repo.Close()
	ctx := context.Background()

	session, _ := NewSession("/path", "openai", "gpt-4")
	_ = repo.CreateSession(ctx, session)
	msg, _ := NewMessage(session.ID, RoleAssistant, "test")
	_ = repo.CreateMessage(ctx, msg)
	tc, _ := NewToolCall(msg.ID, session.ID, "read_file", `{}`)
	_ = repo.CreateToolCall(ctx, tc)

	t.Run("updates tool call status", func(t *testing.T) {
		tc.Start()
		err := repo.UpdateToolCall(ctx, tc)
		if err != nil {
			t.Errorf("UpdateToolCall() error = %v", err)
		}

		retrieved, _ := repo.GetToolCall(ctx, tc.ID)
		if retrieved.Status != ToolCallStatusRunning {
			t.Errorf("UpdateToolCall() Status = %v, want %v", retrieved.Status, ToolCallStatusRunning)
		}
	})

	t.Run("updates tool call completion", func(t *testing.T) {
		tc.Complete(`{"content": "file content"}`)
		err := repo.UpdateToolCall(ctx, tc)
		if err != nil {
			t.Errorf("UpdateToolCall() error = %v", err)
		}

		retrieved, _ := repo.GetToolCall(ctx, tc.ID)
		if retrieved.Status != ToolCallStatusCompleted {
			t.Errorf("UpdateToolCall() Status = %v, want %v", retrieved.Status, ToolCallStatusCompleted)
		}
		if !retrieved.ToolOutput.Valid || retrieved.ToolOutput.String != `{"content": "file content"}` {
			t.Errorf("UpdateToolCall() ToolOutput = %v", retrieved.ToolOutput)
		}
	})

	t.Run("returns error for non-existent tool call", func(t *testing.T) {
		nonExistent, _ := NewToolCall(msg.ID, session.ID, "test_tool", `{}`)
		nonExistent.ID = "non-existent"
		err := repo.UpdateToolCall(ctx, nonExistent)
		if err != ErrToolCallNotFound {
			t.Errorf("UpdateToolCall() error = %v, want %v", err, ErrToolCallNotFound)
		}
	})

	t.Run("updates tool call with error", func(t *testing.T) {
		tcFail, _ := NewToolCall(msg.ID, session.ID, "fail_tool", `{}`)
		_ = repo.CreateToolCall(ctx, tcFail)
		tcFail.Fail("operation failed")
		err := repo.UpdateToolCall(ctx, tcFail)
		if err != nil {
			t.Errorf("UpdateToolCall() error = %v", err)
		}

		retrieved, _ := repo.GetToolCall(ctx, tcFail.ID)
		if retrieved.Status != ToolCallStatusFailed {
			t.Errorf("UpdateToolCall() Status = %v, want %v", retrieved.Status, ToolCallStatusFailed)
		}
		if !retrieved.Error.Valid || retrieved.Error.String != "operation failed" {
			t.Errorf("UpdateToolCall() Error = %v, want 'operation failed'", retrieved.Error)
		}
	})
}

func TestSQLiteRepository_DeleteToolCall(t *testing.T) {
	repo := setupTestRepository(t)
	defer repo.Close()
	ctx := context.Background()

	session, _ := NewSession("/path", "openai", "gpt-4")
	_ = repo.CreateSession(ctx, session)
	msg, _ := NewMessage(session.ID, RoleAssistant, "test")
	_ = repo.CreateMessage(ctx, msg)
	tc, _ := NewToolCall(msg.ID, session.ID, "read_file", `{}`)
	_ = repo.CreateToolCall(ctx, tc)

	t.Run("deletes existing tool call", func(t *testing.T) {
		err := repo.DeleteToolCall(ctx, tc.ID)
		if err != nil {
			t.Errorf("DeleteToolCall() error = %v", err)
		}

		_, err = repo.GetToolCall(ctx, tc.ID)
		if err != ErrToolCallNotFound {
			t.Error("GetToolCall() should return ErrToolCallNotFound after delete")
		}
	})

	t.Run("returns error for non-existent tool call", func(t *testing.T) {
		err := repo.DeleteToolCall(ctx, "non-existent")
		if err != ErrToolCallNotFound {
			t.Errorf("DeleteToolCall() error = %v, want %v", err, ErrToolCallNotFound)
		}
	})
}

func TestSQLiteRepository_ListToolCalls(t *testing.T) {
	repo := setupTestRepository(t)
	defer repo.Close()
	ctx := context.Background()

	session, _ := NewSession("/path", "openai", "gpt-4")
	_ = repo.CreateSession(ctx, session)
	msg, _ := NewMessage(session.ID, RoleAssistant, "test")
	_ = repo.CreateMessage(ctx, msg)

	tc1, _ := NewToolCall(msg.ID, session.ID, "read_file", `{}`)
	tc2, _ := NewToolCall(msg.ID, session.ID, "write_file", `{}`)
	_ = repo.CreateToolCall(ctx, tc1)
	_ = repo.CreateToolCall(ctx, tc2)

	toolCalls, err := repo.ListToolCalls(ctx, session.ID)
	if err != nil {
		t.Errorf("ListToolCalls() error = %v", err)
	}
	if len(toolCalls) != 2 {
		t.Errorf("ListToolCalls() count = %d, want 2", len(toolCalls))
	}
}

func TestSQLiteRepository_ListToolCallsByMessage(t *testing.T) {
	repo := setupTestRepository(t)
	defer repo.Close()
	ctx := context.Background()

	session, _ := NewSession("/path", "openai", "gpt-4")
	_ = repo.CreateSession(ctx, session)
	msg1, _ := NewMessage(session.ID, RoleAssistant, "test1")
	msg2, _ := NewMessage(session.ID, RoleAssistant, "test2")
	_ = repo.CreateMessage(ctx, msg1)
	_ = repo.CreateMessage(ctx, msg2)

	tc1, _ := NewToolCall(msg1.ID, session.ID, "read_file", `{}`)
	tc2, _ := NewToolCall(msg1.ID, session.ID, "write_file", `{}`)
	tc3, _ := NewToolCall(msg2.ID, session.ID, "run_command", `{}`)
	_ = repo.CreateToolCall(ctx, tc1)
	_ = repo.CreateToolCall(ctx, tc2)
	_ = repo.CreateToolCall(ctx, tc3)

	toolCalls, err := repo.ListToolCallsByMessage(ctx, msg1.ID)
	if err != nil {
		t.Errorf("ListToolCallsByMessage() error = %v", err)
	}
	if len(toolCalls) != 2 {
		t.Errorf("ListToolCallsByMessage() count = %d, want 2", len(toolCalls))
	}
}

func TestSQLiteRepository_ListToolCallsByStatus(t *testing.T) {
	repo := setupTestRepository(t)
	defer repo.Close()
	ctx := context.Background()

	session, _ := NewSession("/path", "openai", "gpt-4")
	_ = repo.CreateSession(ctx, session)
	msg, _ := NewMessage(session.ID, RoleAssistant, "test")
	_ = repo.CreateMessage(ctx, msg)

	tc1, _ := NewToolCall(msg.ID, session.ID, "read_file", `{}`)
	tc2, _ := NewToolCall(msg.ID, session.ID, "write_file", `{}`)
	tc2.Start()
	_ = repo.CreateToolCall(ctx, tc1)
	_ = repo.CreateToolCall(ctx, tc2)
	_ = repo.UpdateToolCall(ctx, tc2)

	pending, err := repo.ListToolCallsByStatus(ctx, session.ID, ToolCallStatusPending)
	if err != nil {
		t.Errorf("ListToolCallsByStatus() error = %v", err)
	}
	if len(pending) != 1 {
		t.Errorf("ListToolCallsByStatus(pending) count = %d, want 1", len(pending))
	}

	running, err := repo.ListToolCallsByStatus(ctx, session.ID, ToolCallStatusRunning)
	if err != nil {
		t.Errorf("ListToolCallsByStatus() error = %v", err)
	}
	if len(running) != 1 {
		t.Errorf("ListToolCallsByStatus(running) count = %d, want 1", len(running))
	}
}

func TestSQLiteRepository_ListPendingApprovals(t *testing.T) {
	repo := setupTestRepository(t)
	defer repo.Close()
	ctx := context.Background()

	session, _ := NewSession("/path", "openai", "gpt-4")
	_ = repo.CreateSession(ctx, session)
	msg, _ := NewMessage(session.ID, RoleAssistant, "test")
	_ = repo.CreateMessage(ctx, msg)

	tc1, _ := NewToolCall(msg.ID, session.ID, "read_file", `{}`)
	tc2, _ := NewToolCall(msg.ID, session.ID, "write_file", `{}`)
	tc2.Approve("user@example.com")
	_ = repo.CreateToolCall(ctx, tc1)
	_ = repo.CreateToolCall(ctx, tc2)
	_ = repo.UpdateToolCall(ctx, tc2)

	pending, err := repo.ListPendingApprovals(ctx, session.ID)
	if err != nil {
		t.Errorf("ListPendingApprovals() error = %v", err)
	}
	if len(pending) != 1 {
		t.Errorf("ListPendingApprovals() count = %d, want 1", len(pending))
	}
}

// Session summary tests

func TestSQLiteRepository_CreateSummary(t *testing.T) {
	repo := setupTestRepository(t)
	defer repo.Close()
	ctx := context.Background()

	session, _ := NewSession("/path", "openai", "gpt-4")
	_ = repo.CreateSession(ctx, session)

	t.Run("creates valid summary", func(t *testing.T) {
		summary, _ := NewSessionSummary(session.ID, "Summary of conversation...", 10, 5000)

		err := repo.CreateSummary(ctx, summary)
		if err != nil {
			t.Errorf("CreateSummary() error = %v", err)
		}

		retrieved, err := repo.GetSummary(ctx, summary.ID)
		if err != nil {
			t.Errorf("GetSummary() error = %v", err)
		}
		if retrieved.SummaryText != "Summary of conversation..." {
			t.Errorf("GetSummary() SummaryText = %v", retrieved.SummaryText)
		}
	})

	t.Run("returns error for non-existent session", func(t *testing.T) {
		summary, _ := NewSessionSummary("non-existent", "Summary...", 10, 5000)
		err := repo.CreateSummary(ctx, summary)
		if err != ErrSessionNotFound {
			t.Errorf("CreateSummary() error = %v, want %v", err, ErrSessionNotFound)
		}
	})
}

func TestSQLiteRepository_ListSummaries(t *testing.T) {
	repo := setupTestRepository(t)
	defer repo.Close()
	ctx := context.Background()

	session, _ := NewSession("/path", "openai", "gpt-4")
	_ = repo.CreateSession(ctx, session)

	sum1, _ := NewSessionSummary(session.ID, "Summary 1", 5, 1000)
	sum2, _ := NewSessionSummary(session.ID, "Summary 2", 10, 2000)
	_ = repo.CreateSummary(ctx, sum1)
	time.Sleep(10 * time.Millisecond)
	_ = repo.CreateSummary(ctx, sum2)

	summaries, err := repo.ListSummaries(ctx, session.ID)
	if err != nil {
		t.Errorf("ListSummaries() error = %v", err)
	}
	if len(summaries) != 2 {
		t.Errorf("ListSummaries() count = %d, want 2", len(summaries))
	}
}

func TestSQLiteRepository_GetLatestSummary(t *testing.T) {
	repo := setupTestRepository(t)
	defer repo.Close()
	ctx := context.Background()

	session, _ := NewSession("/path", "openai", "gpt-4")
	_ = repo.CreateSession(ctx, session)

	t.Run("returns nil when no summaries", func(t *testing.T) {
		summary, err := repo.GetLatestSummary(ctx, session.ID)
		if err != nil {
			t.Errorf("GetLatestSummary() error = %v", err)
		}
		if summary != nil {
			t.Errorf("GetLatestSummary() should return nil when no summaries")
		}
	})

	t.Run("returns latest summary", func(t *testing.T) {
		sum1, _ := NewSessionSummary(session.ID, "Old summary", 5, 1000)
		sum1.CreatedAt = time.Now().UTC().Add(-time.Hour) // Ensure old timestamp
		_ = repo.CreateSummary(ctx, sum1)

		sum2, _ := NewSessionSummary(session.ID, "Latest summary", 10, 2000)
		sum2.CreatedAt = time.Now().UTC() // Current timestamp
		_ = repo.CreateSummary(ctx, sum2)

		latest, err := repo.GetLatestSummary(ctx, session.ID)
		if err != nil {
			t.Errorf("GetLatestSummary() error = %v", err)
		}
		if latest.SummaryText != "Latest summary" {
			t.Errorf("GetLatestSummary() SummaryText = %v, want 'Latest summary'", latest.SummaryText)
		}
	})
}

func TestSQLiteRepository_DeleteSummary(t *testing.T) {
	repo := setupTestRepository(t)
	defer repo.Close()
	ctx := context.Background()

	session, _ := NewSession("/path", "openai", "gpt-4")
	_ = repo.CreateSession(ctx, session)

	t.Run("deletes existing summary", func(t *testing.T) {
		summary, _ := NewSessionSummary(session.ID, "Summary to delete", 5, 1000)
		_ = repo.CreateSummary(ctx, summary)

		err := repo.DeleteSummary(ctx, summary.ID)
		if err != nil {
			t.Errorf("DeleteSummary() error = %v", err)
		}

		// Verify summary was deleted
		summaries, _ := repo.ListSummaries(ctx, session.ID)
		for _, s := range summaries {
			if s.ID == summary.ID {
				t.Errorf("Summary should have been deleted")
			}
		}
	})

	t.Run("does not error when deleting non-existent summary", func(t *testing.T) {
		// DeleteSummary doesn't check existence, just runs DELETE
		err := repo.DeleteSummary(ctx, "non-existent")
		if err != nil {
			t.Errorf("DeleteSummary() should not error for non-existent: %v", err)
		}
	})
}

// Aggregation tests

func TestSQLiteRepository_GetSessionStats(t *testing.T) {
	repo := setupTestRepository(t)
	defer repo.Close()
	ctx := context.Background()

	session, _ := NewSession("/path", "openai", "gpt-4")
	_ = repo.CreateSession(ctx, session)

	// Add messages with token usage
	msg1, _ := NewMessage(session.ID, RoleUser, "Hello")
	msg1.SetTokenUsage(100, 0, 0.01)
	msg2, _ := NewMessage(session.ID, RoleAssistant, "Hi there")
	msg2.SetTokenUsage(100, 200, 0.03)
	_ = repo.CreateMessage(ctx, msg1)
	_ = repo.CreateMessage(ctx, msg2)

	// Add tool calls
	tc, _ := NewToolCall(msg2.ID, session.ID, "read_file", `{}`)
	_ = repo.CreateToolCall(ctx, tc)

	// Add summary
	sum, _ := NewSessionSummary(session.ID, "Summary", 2, 500)
	_ = repo.CreateSummary(ctx, sum)

	stats, err := repo.GetSessionStats(ctx, session.ID)
	if err != nil {
		t.Errorf("GetSessionStats() error = %v", err)
	}
	if stats.MessageCount != 2 {
		t.Errorf("GetSessionStats() MessageCount = %d, want 2", stats.MessageCount)
	}
	if stats.ToolCallCount != 1 {
		t.Errorf("GetSessionStats() ToolCallCount = %d, want 1", stats.ToolCallCount)
	}
	if stats.TotalTokensInput != 200 {
		t.Errorf("GetSessionStats() TotalTokensInput = %d, want 200", stats.TotalTokensInput)
	}
	if stats.TotalTokensOutput != 200 {
		t.Errorf("GetSessionStats() TotalTokensOutput = %d, want 200", stats.TotalTokensOutput)
	}
	if stats.SummaryCount != 1 {
		t.Errorf("GetSessionStats() SummaryCount = %d, want 1", stats.SummaryCount)
	}
	if stats.TokensSaved != 500 {
		t.Errorf("GetSessionStats() TokensSaved = %d, want 500", stats.TokensSaved)
	}
}

func TestSQLiteRepository_GetUsageByProvider(t *testing.T) {
	repo := setupTestRepository(t)
	defer repo.Close()
	ctx := context.Background()

	// Create sessions with different providers
	s1, _ := NewSession("/path", "openai", "gpt-4")
	s2, _ := NewSession("/path", "openai", "gpt-4")
	s3, _ := NewSession("/path", "anthropic", "claude-3")
	_ = repo.CreateSession(ctx, s1)
	_ = repo.CreateSession(ctx, s2)
	_ = repo.CreateSession(ctx, s3)

	// Add messages
	msg1, _ := NewMessage(s1.ID, RoleUser, "test")
	msg1.SetTokenUsage(100, 200, 0.05)
	msg2, _ := NewMessage(s3.ID, RoleUser, "test")
	msg2.SetTokenUsage(50, 100, 0.02)
	_ = repo.CreateMessage(ctx, msg1)
	_ = repo.CreateMessage(ctx, msg2)

	usage, err := repo.GetUsageByProvider(ctx)
	if err != nil {
		t.Errorf("GetUsageByProvider() error = %v", err)
	}
	if len(usage) != 2 {
		t.Errorf("GetUsageByProvider() count = %d, want 2", len(usage))
	}
}

// Additional repository tests

func TestSQLiteRepository_GetToolCall_NotFound(t *testing.T) {
	repo := setupTestRepository(t)
	defer repo.Close()
	ctx := context.Background()

	_, err := repo.GetToolCall(ctx, "non-existent")
	if err != ErrToolCallNotFound {
		t.Errorf("GetToolCall() error = %v, want %v", err, ErrToolCallNotFound)
	}
}

func TestSQLiteRepository_GetMessage_NotFound(t *testing.T) {
	repo := setupTestRepository(t)
	defer repo.Close()
	ctx := context.Background()

	_, err := repo.GetMessage(ctx, "non-existent")
	if err != ErrMessageNotFound {
		t.Errorf("GetMessage() error = %v, want %v", err, ErrMessageNotFound)
	}
}

func TestSQLiteRepository_GetSummary_NotFound(t *testing.T) {
	repo := setupTestRepository(t)
	defer repo.Close()
	ctx := context.Background()

	_, err := repo.GetSummary(ctx, "non-existent")
	if err == nil {
		t.Error("GetSummary() should return error for non-existent summary")
	}
}

func TestSQLiteRepository_CreateInvalidMessage(t *testing.T) {
	repo := setupTestRepository(t)
	defer repo.Close()
	ctx := context.Background()

	session, _ := NewSession("/path", "openai", "gpt-4")
	_ = repo.CreateSession(ctx, session)

	// Message with invalid role
	msg := &Message{
		ID:        "test-id",
		SessionID: session.ID,
		Role:      Role("invalid"),
		Content:   "test",
	}
	err := repo.CreateMessage(ctx, msg)
	if err == nil {
		t.Error("CreateMessage() should return error for invalid message")
	}
}

func TestSQLiteRepository_CreateInvalidToolCall(t *testing.T) {
	repo := setupTestRepository(t)
	defer repo.Close()
	ctx := context.Background()

	session, _ := NewSession("/path", "openai", "gpt-4")
	_ = repo.CreateSession(ctx, session)
	msg, _ := NewMessage(session.ID, RoleAssistant, "test")
	_ = repo.CreateMessage(ctx, msg)

	// Tool call with invalid status
	tc := &ToolCall{
		ID:        "test-id",
		MessageID: msg.ID,
		SessionID: session.ID,
		ToolName:  "test_tool",
		ToolInput: "{}",
		Status:    ToolCallStatus("invalid"),
	}
	err := repo.CreateToolCall(ctx, tc)
	if err == nil {
		t.Error("CreateToolCall() should return error for invalid tool call")
	}
}

func TestSQLiteRepository_CreateInvalidSummary(t *testing.T) {
	repo := setupTestRepository(t)
	defer repo.Close()
	ctx := context.Background()

	session, _ := NewSession("/path", "openai", "gpt-4")
	_ = repo.CreateSession(ctx, session)

	// Summary with invalid data (missing summary text)
	summary := &SessionSummary{
		ID:                 "test-id",
		SessionID:          session.ID,
		SummaryText:        "",
		MessagesSummarized: 5,
		TokensSaved:        1000,
	}
	err := repo.CreateSummary(ctx, summary)
	if err == nil {
		t.Error("CreateSummary() should return error for invalid summary")
	}
}

func TestSQLiteRepository_UpdateInvalidMessage(t *testing.T) {
	repo := setupTestRepository(t)
	defer repo.Close()
	ctx := context.Background()

	session, _ := NewSession("/path", "openai", "gpt-4")
	_ = repo.CreateSession(ctx, session)
	msg, _ := NewMessage(session.ID, RoleUser, "test")
	_ = repo.CreateMessage(ctx, msg)

	// Make the message invalid
	msg.Role = Role("invalid")
	err := repo.UpdateMessage(ctx, msg)
	if err == nil {
		t.Error("UpdateMessage() should return error for invalid message")
	}
}

func TestSQLiteRepository_UpdateInvalidToolCall(t *testing.T) {
	repo := setupTestRepository(t)
	defer repo.Close()
	ctx := context.Background()

	session, _ := NewSession("/path", "openai", "gpt-4")
	_ = repo.CreateSession(ctx, session)
	msg, _ := NewMessage(session.ID, RoleAssistant, "test")
	_ = repo.CreateMessage(ctx, msg)
	tc, _ := NewToolCall(msg.ID, session.ID, "test_tool", `{}`)
	_ = repo.CreateToolCall(ctx, tc)

	// Make the tool call invalid
	tc.Status = ToolCallStatus("invalid")
	err := repo.UpdateToolCall(ctx, tc)
	if err == nil {
		t.Error("UpdateToolCall() should return error for invalid tool call")
	}
}

func TestSQLiteRepository_UpdateInvalidSession(t *testing.T) {
	repo := setupTestRepository(t)
	defer repo.Close()
	ctx := context.Background()

	session, _ := NewSession("/path", "openai", "gpt-4")
	_ = repo.CreateSession(ctx, session)

	// Make the session invalid
	session.Status = Status("invalid")
	err := repo.UpdateSession(ctx, session)
	if err == nil {
		t.Error("UpdateSession() should return error for invalid session")
	}
}

func TestSQLiteRepository_ListSessions_OrderBy(t *testing.T) {
	repo := setupTestRepository(t)
	defer repo.Close()
	ctx := context.Background()

	s1, _ := NewSession("/path", "openai", "gpt-4")
	s1.Title = "AAA"
	_ = repo.CreateSession(ctx, s1)
	time.Sleep(10 * time.Millisecond)

	s2, _ := NewSession("/path", "openai", "gpt-4")
	s2.Title = "BBB"
	_ = repo.CreateSession(ctx, s2)

	// Test ordering by updated_at ASC
	opts := &ListSessionsOptions{OrderBy: "updated_at", OrderDir: "ASC"}
	sessions, err := repo.ListSessions(ctx, opts)
	if err != nil {
		t.Errorf("ListSessions() error = %v", err)
	}
	if len(sessions) < 2 {
		t.Errorf("ListSessions() count = %d, want at least 2", len(sessions))
	}
}

func TestSQLiteRepository_Close(t *testing.T) {
	db, _ := sql.Open("sqlite", ":memory:")
	repo, _ := NewSQLiteRepository(db)

	err := repo.Close()
	if err != nil {
		t.Errorf("Close() error = %v", err)
	}
}

// Fork tests

func TestSQLiteRepository_ForkSession(t *testing.T) {
	repo := setupTestRepository(t)
	defer repo.Close()
	ctx := context.Background()

	t.Run("forks session at specific message", func(t *testing.T) {
		// Create parent session with messages
		parent, _ := NewSession("/path/to/project", "openai", "gpt-4")
		parent.Title = "Parent Session"
		_ = repo.CreateSession(ctx, parent)

		// Use explicit timestamps to ensure proper ordering
		now := time.Now().UTC()
		msg1, _ := NewMessage(parent.ID, RoleUser, "Hello")
		msg1.CreatedAt = now.Add(-3 * time.Second)
		_ = repo.CreateMessage(ctx, msg1)

		msg2, _ := NewMessage(parent.ID, RoleAssistant, "Hi there")
		msg2.CreatedAt = now.Add(-2 * time.Second)
		_ = repo.CreateMessage(ctx, msg2)

		msg3, _ := NewMessage(parent.ID, RoleUser, "How are you?")
		msg3.CreatedAt = now.Add(-1 * time.Second)
		_ = repo.CreateMessage(ctx, msg3)

		// Fork at msg2 (should include msg1 and msg2, not msg3)
		opts := &ForkOptions{
			ForkPointMessageID: msg2.ID,
			Title:              "Forked Session",
			CopyMessages:       true,
			CopyToolCalls:      true,
		}
		fork, err := repo.ForkSession(ctx, parent.ID, opts)
		if err != nil {
			t.Fatalf("ForkSession() error = %v", err)
		}

		// Verify fork was created
		if fork == nil {
			t.Fatal("ForkSession() returned nil")
		}
		if !fork.IsFork() {
			t.Error("Fork session should report IsFork() == true")
		}
		if fork.GetParentID() != parent.ID {
			t.Errorf("Fork ParentSessionID = %v, want %v", fork.GetParentID(), parent.ID)
		}
		if fork.GetForkPointMessageID() != msg2.ID {
			t.Errorf("Fork ForkPointMessageID = %v, want %v", fork.GetForkPointMessageID(), msg2.ID)
		}
		if fork.Title != "Forked Session" {
			t.Errorf("Fork Title = %v, want 'Forked Session'", fork.Title)
		}

		// Verify messages were copied (should have 2 messages from msg1 and msg2)
		forkMessages, err := repo.ListMessages(ctx, fork.ID)
		if err != nil {
			t.Fatalf("ListMessages() error = %v", err)
		}
		if len(forkMessages) != 2 {
			t.Errorf("Fork should have 2 messages, got %d", len(forkMessages))
		}
	})

	t.Run("forks session with provider override", func(t *testing.T) {
		parent, _ := NewSession("/path", "openai", "gpt-4")
		_ = repo.CreateSession(ctx, parent)

		msg, _ := NewMessage(parent.ID, RoleUser, "test")
		_ = repo.CreateMessage(ctx, msg)

		opts := &ForkOptions{
			ForkPointMessageID: msg.ID,
			Provider:           "anthropic",
			Model:              "claude-3-opus",
		}
		fork, err := repo.ForkSession(ctx, parent.ID, opts)
		if err != nil {
			t.Fatalf("ForkSession() error = %v", err)
		}

		if fork.Provider != "anthropic" {
			t.Errorf("Fork Provider = %v, want 'anthropic'", fork.Provider)
		}
		if fork.Model != "claude-3-opus" {
			t.Errorf("Fork Model = %v, want 'claude-3-opus'", fork.Model)
		}
	})

	t.Run("returns error for invalid fork point", func(t *testing.T) {
		parent, _ := NewSession("/path", "openai", "gpt-4")
		_ = repo.CreateSession(ctx, parent)

		opts := &ForkOptions{
			ForkPointMessageID: "non-existent-message",
		}
		_, err := repo.ForkSession(ctx, parent.ID, opts)
		if err != ErrInvalidForkPoint {
			t.Errorf("ForkSession() error = %v, want %v", err, ErrInvalidForkPoint)
		}
	})

	t.Run("returns error for non-existent parent", func(t *testing.T) {
		opts := &ForkOptions{
			ForkPointMessageID: "some-message",
		}
		_, err := repo.ForkSession(ctx, "non-existent-parent", opts)
		if err == nil {
			t.Error("ForkSession() should return error for non-existent parent")
		}
	})

	t.Run("returns error for message from different session", func(t *testing.T) {
		session1, _ := NewSession("/path", "openai", "gpt-4")
		session2, _ := NewSession("/path", "openai", "gpt-4")
		_ = repo.CreateSession(ctx, session1)
		_ = repo.CreateSession(ctx, session2)

		msg, _ := NewMessage(session2.ID, RoleUser, "test")
		_ = repo.CreateMessage(ctx, msg)

		opts := &ForkOptions{
			ForkPointMessageID: msg.ID, // Message belongs to session2, not session1
		}
		_, err := repo.ForkSession(ctx, session1.ID, opts)
		if err != ErrInvalidForkPoint {
			t.Errorf("ForkSession() error = %v, want %v", err, ErrInvalidForkPoint)
		}
	})
}

func TestSQLiteRepository_GetForkInfo(t *testing.T) {
	repo := setupTestRepository(t)
	defer repo.Close()
	ctx := context.Background()

	t.Run("returns info for root session", func(t *testing.T) {
		session, _ := NewSession("/path", "openai", "gpt-4")
		_ = repo.CreateSession(ctx, session)

		info, err := repo.GetForkInfo(ctx, session.ID)
		if err != nil {
			t.Fatalf("GetForkInfo() error = %v", err)
		}

		if info.SessionID != session.ID {
			t.Errorf("ForkInfo.SessionID = %v, want %v", info.SessionID, session.ID)
		}
		if info.ParentSessionID != "" {
			t.Errorf("Root session should have empty ParentSessionID")
		}
		if info.RootSessionID != session.ID {
			t.Errorf("Root session's RootSessionID should be itself")
		}
		if info.ForkDepth != 0 {
			t.Errorf("Root session ForkDepth = %d, want 0", info.ForkDepth)
		}
		if !info.IsRoot() {
			t.Error("Root session should report IsRoot() == true")
		}
	})

	t.Run("returns info for forked session", func(t *testing.T) {
		parent, _ := NewSession("/path", "openai", "gpt-4")
		_ = repo.CreateSession(ctx, parent)

		msg, _ := NewMessage(parent.ID, RoleUser, "test")
		_ = repo.CreateMessage(ctx, msg)

		opts := &ForkOptions{ForkPointMessageID: msg.ID}
		fork, _ := repo.ForkSession(ctx, parent.ID, opts)

		info, err := repo.GetForkInfo(ctx, fork.ID)
		if err != nil {
			t.Fatalf("GetForkInfo() error = %v", err)
		}

		if info.ParentSessionID != parent.ID {
			t.Errorf("ForkInfo.ParentSessionID = %v, want %v", info.ParentSessionID, parent.ID)
		}
		if info.RootSessionID != parent.ID {
			t.Errorf("ForkInfo.RootSessionID = %v, want %v", info.RootSessionID, parent.ID)
		}
		if info.ForkDepth != 1 {
			t.Errorf("ForkInfo.ForkDepth = %d, want 1", info.ForkDepth)
		}
		if info.ForkPointMessageID != msg.ID {
			t.Errorf("ForkInfo.ForkPointMessageID = %v, want %v", info.ForkPointMessageID, msg.ID)
		}
		if len(info.AncestorChain) != 2 {
			t.Errorf("AncestorChain length = %d, want 2", len(info.AncestorChain))
		}
	})

	t.Run("returns correct descendant counts", func(t *testing.T) {
		root, _ := NewSession("/path", "openai", "gpt-4")
		_ = repo.CreateSession(ctx, root)

		msg, _ := NewMessage(root.ID, RoleUser, "test")
		_ = repo.CreateMessage(ctx, msg)

		// Create two forks from root
		opts := &ForkOptions{ForkPointMessageID: msg.ID}
		fork1, _ := repo.ForkSession(ctx, root.ID, opts)
		fork2, _ := repo.ForkSession(ctx, root.ID, opts)

		// Create message in fork1 for sub-fork
		msg2, _ := NewMessage(fork1.ID, RoleUser, "test2")
		_ = repo.CreateMessage(ctx, msg2)

		// Create a sub-fork from fork1
		opts2 := &ForkOptions{ForkPointMessageID: msg2.ID}
		_, _ = repo.ForkSession(ctx, fork1.ID, opts2)

		// Suppress unused variable warning
		_ = fork2

		// Check root's info
		rootInfo, _ := repo.GetForkInfo(ctx, root.ID)
		if rootInfo.ChildCount != 2 {
			t.Errorf("Root ChildCount = %d, want 2", rootInfo.ChildCount)
		}
		if rootInfo.TotalDescendants != 3 {
			t.Errorf("Root TotalDescendants = %d, want 3", rootInfo.TotalDescendants)
		}

		// Check fork1's info
		fork1Info, _ := repo.GetForkInfo(ctx, fork1.ID)
		if fork1Info.ChildCount != 1 {
			t.Errorf("Fork1 ChildCount = %d, want 1", fork1Info.ChildCount)
		}
		if fork1Info.TotalDescendants != 1 {
			t.Errorf("Fork1 TotalDescendants = %d, want 1", fork1Info.TotalDescendants)
		}
	})
}

func TestSQLiteRepository_GetAncestorChain(t *testing.T) {
	repo := setupTestRepository(t)
	defer repo.Close()
	ctx := context.Background()

	// Create a chain: root -> fork1 -> fork2
	root, _ := NewSession("/path", "openai", "gpt-4")
	root.Title = "Root"
	_ = repo.CreateSession(ctx, root)

	msg1, _ := NewMessage(root.ID, RoleUser, "test")
	_ = repo.CreateMessage(ctx, msg1)

	opts := &ForkOptions{ForkPointMessageID: msg1.ID}
	fork1, _ := repo.ForkSession(ctx, root.ID, opts)

	msg2, _ := NewMessage(fork1.ID, RoleUser, "test2")
	_ = repo.CreateMessage(ctx, msg2)

	opts2 := &ForkOptions{ForkPointMessageID: msg2.ID}
	fork2, _ := repo.ForkSession(ctx, fork1.ID, opts2)

	chain, err := repo.GetAncestorChain(ctx, fork2.ID)
	if err != nil {
		t.Fatalf("GetAncestorChain() error = %v", err)
	}

	if len(chain) != 3 {
		t.Fatalf("GetAncestorChain() length = %d, want 3", len(chain))
	}
	if chain[0].ID != root.ID {
		t.Errorf("First in chain should be root")
	}
	if chain[1].ID != fork1.ID {
		t.Errorf("Second in chain should be fork1")
	}
	if chain[2].ID != fork2.ID {
		t.Errorf("Third in chain should be fork2")
	}
}

func TestSQLiteRepository_GetRootSession(t *testing.T) {
	repo := setupTestRepository(t)
	defer repo.Close()
	ctx := context.Background()

	root, _ := NewSession("/path", "openai", "gpt-4")
	_ = repo.CreateSession(ctx, root)

	msg, _ := NewMessage(root.ID, RoleUser, "test")
	_ = repo.CreateMessage(ctx, msg)

	opts := &ForkOptions{ForkPointMessageID: msg.ID}
	fork, _ := repo.ForkSession(ctx, root.ID, opts)

	t.Run("returns root for forked session", func(t *testing.T) {
		rootSession, err := repo.GetRootSession(ctx, fork.ID)
		if err != nil {
			t.Fatalf("GetRootSession() error = %v", err)
		}
		if rootSession.ID != root.ID {
			t.Errorf("GetRootSession() ID = %v, want %v", rootSession.ID, root.ID)
		}
	})

	t.Run("returns self for root session", func(t *testing.T) {
		rootSession, err := repo.GetRootSession(ctx, root.ID)
		if err != nil {
			t.Fatalf("GetRootSession() error = %v", err)
		}
		if rootSession.ID != root.ID {
			t.Errorf("GetRootSession() ID = %v, want %v", rootSession.ID, root.ID)
		}
	})
}

func TestSQLiteRepository_ListDescendants(t *testing.T) {
	repo := setupTestRepository(t)
	defer repo.Close()
	ctx := context.Background()

	root, _ := NewSession("/path", "openai", "gpt-4")
	_ = repo.CreateSession(ctx, root)

	msg, _ := NewMessage(root.ID, RoleUser, "test")
	_ = repo.CreateMessage(ctx, msg)

	opts := &ForkOptions{ForkPointMessageID: msg.ID}
	fork1, _ := repo.ForkSession(ctx, root.ID, opts)
	fork2, _ := repo.ForkSession(ctx, root.ID, opts)

	msg2, _ := NewMessage(fork1.ID, RoleUser, "test2")
	_ = repo.CreateMessage(ctx, msg2)

	opts2 := &ForkOptions{ForkPointMessageID: msg2.ID}
	subFork, _ := repo.ForkSession(ctx, fork1.ID, opts2)

	t.Run("lists all descendants of root", func(t *testing.T) {
		descendants, err := repo.ListDescendants(ctx, root.ID)
		if err != nil {
			t.Fatalf("ListDescendants() error = %v", err)
		}
		if len(descendants) != 3 {
			t.Errorf("ListDescendants() count = %d, want 3", len(descendants))
		}

		// Verify all expected descendants are present
		ids := make(map[string]bool)
		for _, d := range descendants {
			ids[d.ID] = true
		}
		if !ids[fork1.ID] || !ids[fork2.ID] || !ids[subFork.ID] {
			t.Error("ListDescendants() missing expected descendants")
		}
	})

	t.Run("lists descendants of fork1", func(t *testing.T) {
		descendants, err := repo.ListDescendants(ctx, fork1.ID)
		if err != nil {
			t.Fatalf("ListDescendants() error = %v", err)
		}
		if len(descendants) != 1 {
			t.Errorf("ListDescendants() count = %d, want 1", len(descendants))
		}
		if descendants[0].ID != subFork.ID {
			t.Errorf("ListDescendants() should return subFork")
		}
	})

	t.Run("returns empty for leaf session", func(t *testing.T) {
		descendants, err := repo.ListDescendants(ctx, subFork.ID)
		if err != nil {
			t.Fatalf("ListDescendants() error = %v", err)
		}
		if len(descendants) != 0 {
			t.Errorf("ListDescendants() count = %d, want 0", len(descendants))
		}
	})
}

func TestSQLiteRepository_IsDescendantOf(t *testing.T) {
	repo := setupTestRepository(t)
	defer repo.Close()
	ctx := context.Background()

	root, _ := NewSession("/path", "openai", "gpt-4")
	_ = repo.CreateSession(ctx, root)

	msg, _ := NewMessage(root.ID, RoleUser, "test")
	_ = repo.CreateMessage(ctx, msg)

	opts := &ForkOptions{ForkPointMessageID: msg.ID}
	fork1, _ := repo.ForkSession(ctx, root.ID, opts)

	msg2, _ := NewMessage(fork1.ID, RoleUser, "test2")
	_ = repo.CreateMessage(ctx, msg2)

	opts2 := &ForkOptions{ForkPointMessageID: msg2.ID}
	subFork, _ := repo.ForkSession(ctx, fork1.ID, opts2)

	unrelated, _ := NewSession("/path", "openai", "gpt-4")
	_ = repo.CreateSession(ctx, unrelated)

	t.Run("returns true for direct descendant", func(t *testing.T) {
		isDesc, err := repo.IsDescendantOf(ctx, fork1.ID, root.ID)
		if err != nil {
			t.Fatalf("IsDescendantOf() error = %v", err)
		}
		if !isDesc {
			t.Error("fork1 should be descendant of root")
		}
	})

	t.Run("returns true for indirect descendant", func(t *testing.T) {
		isDesc, err := repo.IsDescendantOf(ctx, subFork.ID, root.ID)
		if err != nil {
			t.Fatalf("IsDescendantOf() error = %v", err)
		}
		if !isDesc {
			t.Error("subFork should be descendant of root")
		}
	})

	t.Run("returns false for non-descendant", func(t *testing.T) {
		isDesc, err := repo.IsDescendantOf(ctx, unrelated.ID, root.ID)
		if err != nil {
			t.Fatalf("IsDescendantOf() error = %v", err)
		}
		if isDesc {
			t.Error("unrelated should not be descendant of root")
		}
	})

	t.Run("returns false for same session", func(t *testing.T) {
		isDesc, err := repo.IsDescendantOf(ctx, root.ID, root.ID)
		if err != nil {
			t.Fatalf("IsDescendantOf() error = %v", err)
		}
		if isDesc {
			t.Error("session should not be descendant of itself")
		}
	})

	t.Run("returns false for ancestor (reverse direction)", func(t *testing.T) {
		isDesc, err := repo.IsDescendantOf(ctx, root.ID, fork1.ID)
		if err != nil {
			t.Fatalf("IsDescendantOf() error = %v", err)
		}
		if isDesc {
			t.Error("root should not be descendant of fork1")
		}
	})
}

func TestSQLiteRepository_ListMessagesUpTo(t *testing.T) {
	repo := setupTestRepository(t)
	defer repo.Close()
	ctx := context.Background()

	session, _ := NewSession("/path", "openai", "gpt-4")
	_ = repo.CreateSession(ctx, session)

	// Use explicit timestamps to ensure proper ordering
	now := time.Now().UTC()
	msg1, _ := NewMessage(session.ID, RoleUser, "First")
	msg1.CreatedAt = now.Add(-2 * time.Second)
	_ = repo.CreateMessage(ctx, msg1)

	msg2, _ := NewMessage(session.ID, RoleAssistant, "Second")
	msg2.CreatedAt = now.Add(-1 * time.Second)
	_ = repo.CreateMessage(ctx, msg2)

	msg3, _ := NewMessage(session.ID, RoleUser, "Third")
	msg3.CreatedAt = now
	_ = repo.CreateMessage(ctx, msg3)

	t.Run("returns messages up to specified message", func(t *testing.T) {
		messages, err := repo.ListMessagesUpTo(ctx, session.ID, msg2.ID)
		if err != nil {
			t.Fatalf("ListMessagesUpTo() error = %v", err)
		}
		if len(messages) != 2 {
			t.Errorf("ListMessagesUpTo() count = %d, want 2", len(messages))
		}
		if len(messages) >= 2 {
			if messages[0].Content != "First" || messages[1].Content != "Second" {
				t.Error("ListMessagesUpTo() returned wrong messages")
			}
		}
	})

	t.Run("returns error for non-existent message", func(t *testing.T) {
		_, err := repo.ListMessagesUpTo(ctx, session.ID, "non-existent")
		if err != ErrMessageNotFound {
			t.Errorf("ListMessagesUpTo() error = %v, want %v", err, ErrMessageNotFound)
		}
	})
}

func TestSQLiteRepository_GetFullConversationHistory(t *testing.T) {
	repo := setupTestRepository(t)
	defer repo.Close()
	ctx := context.Background()

	t.Run("returns all messages for root session", func(t *testing.T) {
		session, _ := NewSession("/path", "openai", "gpt-4")
		_ = repo.CreateSession(ctx, session)

		msg1, _ := NewMessage(session.ID, RoleUser, "Hello")
		msg2, _ := NewMessage(session.ID, RoleAssistant, "Hi")
		_ = repo.CreateMessage(ctx, msg1)
		time.Sleep(10 * time.Millisecond)
		_ = repo.CreateMessage(ctx, msg2)

		history, err := repo.GetFullConversationHistory(ctx, session.ID)
		if err != nil {
			t.Fatalf("GetFullConversationHistory() error = %v", err)
		}
		if len(history) != 2 {
			t.Errorf("GetFullConversationHistory() count = %d, want 2", len(history))
		}
	})

	t.Run("returns combined history for forked session without copied messages", func(t *testing.T) {
		parent, _ := NewSession("/path", "openai", "gpt-4")
		_ = repo.CreateSession(ctx, parent)

		msg1, _ := NewMessage(parent.ID, RoleUser, "Parent msg 1")
		_ = repo.CreateMessage(ctx, msg1)
		time.Sleep(10 * time.Millisecond)

		msg2, _ := NewMessage(parent.ID, RoleAssistant, "Parent msg 2")
		_ = repo.CreateMessage(ctx, msg2)
		time.Sleep(10 * time.Millisecond)

		// Fork without copying messages
		opts := &ForkOptions{
			ForkPointMessageID: msg2.ID,
			CopyMessages:       false,
		}
		fork, _ := repo.ForkSession(ctx, parent.ID, opts)

		// Add new messages to fork
		forkMsg1, _ := NewMessage(fork.ID, RoleUser, "Fork msg 1")
		_ = repo.CreateMessage(ctx, forkMsg1)
		time.Sleep(10 * time.Millisecond)

		forkMsg2, _ := NewMessage(fork.ID, RoleAssistant, "Fork msg 2")
		_ = repo.CreateMessage(ctx, forkMsg2)

		history, err := repo.GetFullConversationHistory(ctx, fork.ID)
		if err != nil {
			t.Fatalf("GetFullConversationHistory() error = %v", err)
		}

		// Should have: Parent msg 1, Parent msg 2, Fork msg 1, Fork msg 2
		if len(history) != 4 {
			t.Errorf("GetFullConversationHistory() count = %d, want 4", len(history))
		}

		// Verify order
		expectedContents := []string{"Parent msg 1", "Parent msg 2", "Fork msg 1", "Fork msg 2"}
		for i, expected := range expectedContents {
			if i < len(history) && history[i].Content != expected {
				t.Errorf("Message %d content = %v, want %v", i, history[i].Content, expected)
			}
		}
	})
}

func TestSQLiteRepository_ForkWithToolCalls(t *testing.T) {
	repo := setupTestRepository(t)
	defer repo.Close()
	ctx := context.Background()

	parent, _ := NewSession("/path", "openai", "gpt-4")
	_ = repo.CreateSession(ctx, parent)

	msg, _ := NewMessage(parent.ID, RoleAssistant, "Let me read that file")
	_ = repo.CreateMessage(ctx, msg)

	tc, _ := NewToolCall(msg.ID, parent.ID, "read_file", `{"path": "/test.txt"}`)
	tc.Complete(`{"content": "file content"}`)
	_ = repo.CreateToolCall(ctx, tc)
	_ = repo.UpdateToolCall(ctx, tc)

	// Fork with tool call copying
	opts := &ForkOptions{
		ForkPointMessageID: msg.ID,
		CopyMessages:       true,
		CopyToolCalls:      true,
	}
	fork, err := repo.ForkSession(ctx, parent.ID, opts)
	if err != nil {
		t.Fatalf("ForkSession() error = %v", err)
	}

	// Verify tool calls were copied
	toolCalls, err := repo.ListToolCalls(ctx, fork.ID)
	if err != nil {
		t.Fatalf("ListToolCalls() error = %v", err)
	}
	if len(toolCalls) != 1 {
		t.Errorf("Fork should have 1 tool call, got %d", len(toolCalls))
	}
	if len(toolCalls) > 0 && toolCalls[0].ToolName != "read_file" {
		t.Errorf("Copied tool call ToolName = %v, want 'read_file'", toolCalls[0].ToolName)
	}
}

func TestSQLiteRepository_Session_IsFork_Integration(t *testing.T) {
	t.Run("returns false for non-fork session", func(t *testing.T) {
		session, _ := NewSession("/path", "openai", "gpt-4")
		if session.IsFork() {
			t.Error("NewSession() should not be a fork")
		}
	})

	t.Run("returns true for fork session", func(t *testing.T) {
		session, _ := NewSession("/path", "openai", "gpt-4")
		session.SetForkParent("parent-id", "message-id")
		if !session.IsFork() {
			t.Error("Session with parent should be a fork")
		}
	})
}

func TestForkOptions_Validate(t *testing.T) {
	t.Run("returns error for empty fork point", func(t *testing.T) {
		opts := &ForkOptions{}
		err := opts.Validate()
		if err == nil {
			t.Error("Validate() should return error for empty ForkPointMessageID")
		}
	})

	t.Run("passes for valid options", func(t *testing.T) {
		opts := &ForkOptions{ForkPointMessageID: "msg-id"}
		err := opts.Validate()
		if err != nil {
			t.Errorf("Validate() error = %v", err)
		}
	})
}

func TestForkInfo_IsRoot(t *testing.T) {
	t.Run("returns true when no parent", func(t *testing.T) {
		info := &ForkInfo{SessionID: "test", ParentSessionID: ""}
		if !info.IsRoot() {
			t.Error("ForkInfo with empty parent should be root")
		}
	})

	t.Run("returns false when has parent", func(t *testing.T) {
		info := &ForkInfo{SessionID: "test", ParentSessionID: "parent"}
		if info.IsRoot() {
			t.Error("ForkInfo with parent should not be root")
		}
	})
}

// Cascade delete test

func TestSQLiteRepository_CascadeDelete(t *testing.T) {
	repo := setupTestRepository(t)
	defer repo.Close()
	ctx := context.Background()

	session, _ := NewSession("/path", "openai", "gpt-4")
	_ = repo.CreateSession(ctx, session)

	msg, _ := NewMessage(session.ID, RoleAssistant, "test")
	_ = repo.CreateMessage(ctx, msg)

	tc, _ := NewToolCall(msg.ID, session.ID, "read_file", `{}`)
	_ = repo.CreateToolCall(ctx, tc)

	sum, _ := NewSessionSummary(session.ID, "Summary", 1, 100)
	_ = repo.CreateSummary(ctx, sum)

	// Delete session should cascade
	err := repo.DeleteSession(ctx, session.ID)
	if err != nil {
		t.Errorf("DeleteSession() error = %v", err)
	}

	// Verify cascade
	messages, _ := repo.ListMessages(ctx, session.ID)
	if len(messages) != 0 {
		t.Errorf("Messages should be cascade deleted")
	}

	toolCalls, _ := repo.ListToolCalls(ctx, session.ID)
	if len(toolCalls) != 0 {
		t.Errorf("ToolCalls should be cascade deleted")
	}

	summaries, _ := repo.ListSummaries(ctx, session.ID)
	if len(summaries) != 0 {
		t.Errorf("Summaries should be cascade deleted")
	}
}
