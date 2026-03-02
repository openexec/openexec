package mcp

import (
	"context"
	"testing"

	"github.com/openexec/openexec/pkg/db/session"
)

// mockSessionRepository is a mock implementation of session.Repository for testing.
type mockSessionRepository struct {
	sessions     map[string]*session.Session
	messages     map[string][]*session.Message
	toolCalls    map[string][]*session.ToolCall
	summaries    map[string][]*session.SessionSummary
	forkCallback func(parentID string, opts *session.ForkOptions) (*session.Session, error)
}

func newMockSessionRepository() *mockSessionRepository {
	return &mockSessionRepository{
		sessions:  make(map[string]*session.Session),
		messages:  make(map[string][]*session.Message),
		toolCalls: make(map[string][]*session.ToolCall),
		summaries: make(map[string][]*session.SessionSummary),
	}
}

func (m *mockSessionRepository) CreateSession(ctx context.Context, s *session.Session) error {
	m.sessions[s.ID] = s
	return nil
}

func (m *mockSessionRepository) GetSession(ctx context.Context, id string) (*session.Session, error) {
	if s, ok := m.sessions[id]; ok {
		return s, nil
	}
	return nil, session.ErrSessionNotFound
}

func (m *mockSessionRepository) UpdateSession(ctx context.Context, s *session.Session) error {
	m.sessions[s.ID] = s
	return nil
}

func (m *mockSessionRepository) DeleteSession(ctx context.Context, id string) error {
	delete(m.sessions, id)
	return nil
}

func (m *mockSessionRepository) ListSessions(ctx context.Context, opts *session.ListSessionsOptions) ([]*session.Session, error) {
	var result []*session.Session
	for _, s := range m.sessions {
		result = append(result, s)
	}
	return result, nil
}

func (m *mockSessionRepository) ListSessionsByProject(ctx context.Context, projectPath string) ([]*session.Session, error) {
	var result []*session.Session
	for _, s := range m.sessions {
		if s.ProjectPath == projectPath {
			result = append(result, s)
		}
	}
	return result, nil
}

func (m *mockSessionRepository) GetSessionForks(ctx context.Context, sessionID string) ([]*session.Session, error) {
	var result []*session.Session
	for _, s := range m.sessions {
		if s.GetParentID() == sessionID {
			result = append(result, s)
		}
	}
	return result, nil
}

func (m *mockSessionRepository) ForkSession(ctx context.Context, parentSessionID string, opts *session.ForkOptions) (*session.Session, error) {
	if m.forkCallback != nil {
		return m.forkCallback(parentSessionID, opts)
	}

	parent, err := m.GetSession(ctx, parentSessionID)
	if err != nil {
		return nil, err
	}

	forked, err := parent.ForkSession(opts)
	if err != nil {
		return nil, err
	}

	m.sessions[forked.ID] = forked

	// Copy messages if requested
	if opts.CopyMessages {
		if msgs, ok := m.messages[parentSessionID]; ok {
			for _, msg := range msgs {
				newMsg := *msg
				newMsg.SessionID = forked.ID
				m.messages[forked.ID] = append(m.messages[forked.ID], &newMsg)
			}
		}
	}

	return forked, nil
}

func (m *mockSessionRepository) GetForkInfo(ctx context.Context, sessionID string) (*session.ForkInfo, error) {
	s, err := m.GetSession(ctx, sessionID)
	if err != nil {
		return nil, err
	}

	// Build ancestor chain
	var ancestorChain []string
	current := s
	for current != nil {
		ancestorChain = append([]string{current.ID}, ancestorChain...)
		if !current.ParentSessionID.Valid {
			break
		}
		current, _ = m.GetSession(ctx, current.ParentSessionID.String)
	}

	// Count children
	childCount := 0
	for _, sess := range m.sessions {
		if sess.GetParentID() == sessionID {
			childCount++
		}
	}

	rootID := ancestorChain[0]
	depth := len(ancestorChain) - 1

	return &session.ForkInfo{
		SessionID:          sessionID,
		ParentSessionID:    s.GetParentID(),
		RootSessionID:      rootID,
		ForkPointMessageID: s.GetForkPointMessageID(),
		ForkDepth:          depth,
		ChildCount:         childCount,
		TotalDescendants:   childCount, // Simplified: only direct children
		AncestorChain:      ancestorChain,
		ForkCreatedAt:      s.CreatedAt,
	}, nil
}

func (m *mockSessionRepository) GetAncestorChain(ctx context.Context, sessionID string) ([]*session.Session, error) {
	var chain []*session.Session
	current, err := m.GetSession(ctx, sessionID)
	if err != nil {
		return nil, err
	}

	for current != nil {
		chain = append([]*session.Session{current}, chain...)
		if !current.ParentSessionID.Valid {
			break
		}
		current, _ = m.GetSession(ctx, current.ParentSessionID.String)
	}

	return chain, nil
}

func (m *mockSessionRepository) GetRootSession(ctx context.Context, sessionID string) (*session.Session, error) {
	chain, err := m.GetAncestorChain(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	if len(chain) > 0 {
		return chain[0], nil
	}
	return nil, session.ErrSessionNotFound
}

func (m *mockSessionRepository) ListDescendants(ctx context.Context, sessionID string) ([]*session.Session, error) {
	var result []*session.Session
	for _, s := range m.sessions {
		if s.GetParentID() == sessionID {
			result = append(result, s)
		}
	}
	return result, nil
}

func (m *mockSessionRepository) IsDescendantOf(ctx context.Context, childSessionID, ancestorSessionID string) (bool, error) {
	current, err := m.GetSession(ctx, childSessionID)
	if err != nil {
		return false, err
	}

	for current != nil && current.ParentSessionID.Valid {
		if current.ParentSessionID.String == ancestorSessionID {
			return true, nil
		}
		current, _ = m.GetSession(ctx, current.ParentSessionID.String)
	}

	return false, nil
}

func (m *mockSessionRepository) CreateMessage(ctx context.Context, msg *session.Message) error {
	m.messages[msg.SessionID] = append(m.messages[msg.SessionID], msg)
	return nil
}

func (m *mockSessionRepository) GetMessage(ctx context.Context, id string) (*session.Message, error) {
	for _, msgs := range m.messages {
		for _, msg := range msgs {
			if msg.ID == id {
				return msg, nil
			}
		}
	}
	return nil, session.ErrMessageNotFound
}

func (m *mockSessionRepository) UpdateMessage(ctx context.Context, msg *session.Message) error {
	return nil
}

func (m *mockSessionRepository) DeleteMessage(ctx context.Context, id string) error {
	return nil
}

func (m *mockSessionRepository) ListMessages(ctx context.Context, sessionID string) ([]*session.Message, error) {
	return m.messages[sessionID], nil
}

func (m *mockSessionRepository) ListMessagesByRole(ctx context.Context, sessionID string, role session.Role) ([]*session.Message, error) {
	var result []*session.Message
	for _, msg := range m.messages[sessionID] {
		if msg.Role == role {
			result = append(result, msg)
		}
	}
	return result, nil
}

func (m *mockSessionRepository) GetMessageCount(ctx context.Context, sessionID string) (int, error) {
	return len(m.messages[sessionID]), nil
}

func (m *mockSessionRepository) ListMessagesUpTo(ctx context.Context, sessionID, upToMessageID string) ([]*session.Message, error) {
	var result []*session.Message
	for _, msg := range m.messages[sessionID] {
		result = append(result, msg)
		if msg.ID == upToMessageID {
			break
		}
	}
	return result, nil
}

func (m *mockSessionRepository) GetFullConversationHistory(ctx context.Context, sessionID string) ([]*session.Message, error) {
	return m.messages[sessionID], nil
}

func (m *mockSessionRepository) CreateToolCall(ctx context.Context, tc *session.ToolCall) error {
	m.toolCalls[tc.SessionID] = append(m.toolCalls[tc.SessionID], tc)
	return nil
}

func (m *mockSessionRepository) GetToolCall(ctx context.Context, id string) (*session.ToolCall, error) {
	return nil, session.ErrToolCallNotFound
}

func (m *mockSessionRepository) UpdateToolCall(ctx context.Context, tc *session.ToolCall) error {
	return nil
}

func (m *mockSessionRepository) DeleteToolCall(ctx context.Context, id string) error {
	return nil
}

func (m *mockSessionRepository) ListToolCalls(ctx context.Context, sessionID string) ([]*session.ToolCall, error) {
	return m.toolCalls[sessionID], nil
}

func (m *mockSessionRepository) ListToolCallsByMessage(ctx context.Context, messageID string) ([]*session.ToolCall, error) {
	return nil, nil
}

func (m *mockSessionRepository) ListToolCallsByStatus(ctx context.Context, sessionID string, status session.ToolCallStatus) ([]*session.ToolCall, error) {
	return nil, nil
}

func (m *mockSessionRepository) ListPendingApprovals(ctx context.Context, sessionID string) ([]*session.ToolCall, error) {
	return nil, nil
}

func (m *mockSessionRepository) CreateSummary(ctx context.Context, s *session.SessionSummary) error {
	m.summaries[s.SessionID] = append(m.summaries[s.SessionID], s)
	return nil
}

func (m *mockSessionRepository) GetSummary(ctx context.Context, id string) (*session.SessionSummary, error) {
	return nil, nil
}

func (m *mockSessionRepository) ListSummaries(ctx context.Context, sessionID string) ([]*session.SessionSummary, error) {
	return m.summaries[sessionID], nil
}

func (m *mockSessionRepository) GetLatestSummary(ctx context.Context, sessionID string) (*session.SessionSummary, error) {
	sums := m.summaries[sessionID]
	if len(sums) > 0 {
		return sums[len(sums)-1], nil
	}
	return nil, nil
}

func (m *mockSessionRepository) DeleteSummary(ctx context.Context, id string) error {
	return nil
}

func (m *mockSessionRepository) GetSessionStats(ctx context.Context, sessionID string) (*session.SessionStats, error) {
	return &session.SessionStats{SessionID: sessionID}, nil
}

func (m *mockSessionRepository) GetUsageByProvider(ctx context.Context) ([]*session.ProviderUsage, error) {
	return nil, nil
}

func (m *mockSessionRepository) Close() error {
	return nil
}

// Test helper to create a session with messages
func createTestSessionWithMessages(repo *mockSessionRepository, id, projectPath, provider, model string, msgCount int) *session.Session {
	s := &session.Session{
		ID:          id,
		ProjectPath: projectPath,
		Provider:    provider,
		Model:       model,
		Status:      session.StatusActive,
	}
	repo.sessions[id] = s

	for i := 0; i < msgCount; i++ {
		msg, _ := session.NewMessage(id, session.RoleUser, "test message")
		repo.messages[id] = append(repo.messages[id], msg)
	}

	return s
}

func TestSessionForkManager_ForkSession(t *testing.T) {
	repo := newMockSessionRepository()
	manager := NewSessionForkManager(repo)

	// Create parent session with messages
	parentSession := createTestSessionWithMessages(repo, "parent-session-id", "/test/project", "anthropic", "claude-3-opus", 5)
	msgs := repo.messages[parentSession.ID]
	forkPointID := msgs[2].ID // Fork at 3rd message

	ctx := context.Background()

	t.Run("successful fork", func(t *testing.T) {
		req := &ForkSessionRequest{
			ParentSessionID:    parentSession.ID,
			ForkPointMessageID: forkPointID,
			Title:              "My Fork",
			CopyMessages:       true,
		}

		result, err := manager.ForkSession(ctx, req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if result.ForkedSessionID == "" {
			t.Error("expected forked session ID to be set")
		}
		if result.ParentSessionID != parentSession.ID {
			t.Errorf("expected parent session ID %s, got %s", parentSession.ID, result.ParentSessionID)
		}
		if result.ForkPointMessageID != forkPointID {
			t.Errorf("expected fork point message ID %s, got %s", forkPointID, result.ForkPointMessageID)
		}
		if result.Title != "My Fork" {
			t.Errorf("expected title 'My Fork', got '%s'", result.Title)
		}
		if result.Provider != "anthropic" {
			t.Errorf("expected provider 'anthropic', got '%s'", result.Provider)
		}
		if result.Model != "claude-3-opus" {
			t.Errorf("expected model 'claude-3-opus', got '%s'", result.Model)
		}
		if result.ForkDepth != 1 {
			t.Errorf("expected fork depth 1, got %d", result.ForkDepth)
		}
	})

	t.Run("fork with provider override", func(t *testing.T) {
		req := &ForkSessionRequest{
			ParentSessionID:    parentSession.ID,
			ForkPointMessageID: forkPointID,
			Provider:           "openai",
			Model:              "gpt-4",
		}

		result, err := manager.ForkSession(ctx, req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if result.Provider != "openai" {
			t.Errorf("expected provider 'openai', got '%s'", result.Provider)
		}
		if result.Model != "gpt-4" {
			t.Errorf("expected model 'gpt-4', got '%s'", result.Model)
		}
	})

	t.Run("fork with missing parent session ID", func(t *testing.T) {
		req := &ForkSessionRequest{
			ForkPointMessageID: forkPointID,
		}

		_, err := manager.ForkSession(ctx, req)
		if err == nil {
			t.Error("expected error for missing parent session ID")
		}
	})

	t.Run("fork with missing fork point message ID", func(t *testing.T) {
		req := &ForkSessionRequest{
			ParentSessionID: parentSession.ID,
		}

		_, err := manager.ForkSession(ctx, req)
		if err == nil {
			t.Error("expected error for missing fork point message ID")
		}
	})

	t.Run("fork nonexistent parent session", func(t *testing.T) {
		req := &ForkSessionRequest{
			ParentSessionID:    "nonexistent-session-id",
			ForkPointMessageID: forkPointID,
		}

		_, err := manager.ForkSession(ctx, req)
		if err == nil {
			t.Error("expected error for nonexistent parent session")
		}
	})
}

func TestSessionForkManager_GetForkInfo(t *testing.T) {
	repo := newMockSessionRepository()
	manager := NewSessionForkManager(repo)

	// Create a chain of forked sessions: root -> child1 -> child2
	root := createTestSessionWithMessages(repo, "root-session", "/test/project", "anthropic", "claude-3", 3)
	msgs := repo.messages[root.ID]

	child1, _ := root.ForkSession(&session.ForkOptions{ForkPointMessageID: msgs[0].ID})
	repo.sessions[child1.ID] = child1

	child2, _ := child1.ForkSession(&session.ForkOptions{ForkPointMessageID: msgs[0].ID})
	repo.sessions[child2.ID] = child2

	ctx := context.Background()

	t.Run("root session info", func(t *testing.T) {
		req := &GetForkInfoRequest{SessionID: root.ID}
		info, err := manager.GetForkInfo(ctx, req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if info.ParentSessionID != "" {
			t.Error("expected root session to have no parent")
		}
		if info.RootSessionID != root.ID {
			t.Errorf("expected root session ID %s, got %s", root.ID, info.RootSessionID)
		}
		if info.ForkDepth != 0 {
			t.Errorf("expected fork depth 0, got %d", info.ForkDepth)
		}
	})

	t.Run("forked session info", func(t *testing.T) {
		req := &GetForkInfoRequest{SessionID: child2.ID}
		info, err := manager.GetForkInfo(ctx, req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if info.ParentSessionID != child1.ID {
			t.Errorf("expected parent session ID %s, got %s", child1.ID, info.ParentSessionID)
		}
		if info.RootSessionID != root.ID {
			t.Errorf("expected root session ID %s, got %s", root.ID, info.RootSessionID)
		}
		if info.ForkDepth != 2 {
			t.Errorf("expected fork depth 2, got %d", info.ForkDepth)
		}
		if len(info.AncestorChain) != 3 {
			t.Errorf("expected 3 ancestors in chain, got %d", len(info.AncestorChain))
		}
	})

	t.Run("missing session ID", func(t *testing.T) {
		req := &GetForkInfoRequest{}
		_, err := manager.GetForkInfo(ctx, req)
		if err == nil {
			t.Error("expected error for missing session ID")
		}
	})
}

func TestSessionForkManager_ListSessionForks(t *testing.T) {
	repo := newMockSessionRepository()
	manager := NewSessionForkManager(repo)

	// Create parent with multiple forks
	parent := createTestSessionWithMessages(repo, "parent-session", "/test/project", "anthropic", "claude-3", 3)
	msgs := repo.messages[parent.ID]

	fork1, _ := parent.ForkSession(&session.ForkOptions{ForkPointMessageID: msgs[0].ID, Title: "Fork 1"})
	repo.sessions[fork1.ID] = fork1

	fork2, _ := parent.ForkSession(&session.ForkOptions{ForkPointMessageID: msgs[1].ID, Title: "Fork 2"})
	repo.sessions[fork2.ID] = fork2

	fork3, _ := parent.ForkSession(&session.ForkOptions{ForkPointMessageID: msgs[2].ID, Title: "Fork 3"})
	repo.sessions[fork3.ID] = fork3

	ctx := context.Background()

	t.Run("list forks of parent session", func(t *testing.T) {
		req := &ListSessionForksRequest{SessionID: parent.ID}
		forks, err := manager.ListSessionForks(ctx, req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(forks) != 3 {
			t.Errorf("expected 3 forks, got %d", len(forks))
		}
	})

	t.Run("list forks of session without forks", func(t *testing.T) {
		req := &ListSessionForksRequest{SessionID: fork1.ID}
		forks, err := manager.ListSessionForks(ctx, req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(forks) != 0 {
			t.Errorf("expected 0 forks, got %d", len(forks))
		}
	})

	t.Run("missing session ID", func(t *testing.T) {
		req := &ListSessionForksRequest{}
		_, err := manager.ListSessionForks(ctx, req)
		if err == nil {
			t.Error("expected error for missing session ID")
		}
	})
}

func TestValidateForkSessionRequest(t *testing.T) {
	tests := []struct {
		name    string
		req     *ForkSessionRequest
		wantErr bool
	}{
		{
			name: "valid request",
			req: &ForkSessionRequest{
				ParentSessionID:    "parent-id",
				ForkPointMessageID: "message-id",
			},
			wantErr: false,
		},
		{
			name: "missing parent session ID",
			req: &ForkSessionRequest{
				ForkPointMessageID: "message-id",
			},
			wantErr: true,
		},
		{
			name: "missing fork point message ID",
			req: &ForkSessionRequest{
				ParentSessionID: "parent-id",
			},
			wantErr: true,
		},
		{
			name:    "empty request",
			req:     &ForkSessionRequest{},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateForkSessionRequest(tt.req)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateForkSessionRequest() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateGetForkInfoRequest(t *testing.T) {
	tests := []struct {
		name    string
		req     *GetForkInfoRequest
		wantErr bool
	}{
		{
			name:    "valid request",
			req:     &GetForkInfoRequest{SessionID: "session-id"},
			wantErr: false,
		},
		{
			name:    "missing session ID",
			req:     &GetForkInfoRequest{},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateGetForkInfoRequest(tt.req)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateGetForkInfoRequest() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateListSessionForksRequest(t *testing.T) {
	tests := []struct {
		name    string
		req     *ListSessionForksRequest
		wantErr bool
	}{
		{
			name:    "valid request",
			req:     &ListSessionForksRequest{SessionID: "session-id"},
			wantErr: false,
		},
		{
			name:    "missing session ID",
			req:     &ListSessionForksRequest{},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateListSessionForksRequest(tt.req)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateListSessionForksRequest() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestForkToolDefinitions(t *testing.T) {
	t.Run("ForkSessionToolDef", func(t *testing.T) {
		def := ForkSessionToolDef()

		if def["name"] != "fork_session" {
			t.Errorf("expected name 'fork_session', got '%s'", def["name"])
		}

		description, ok := def["description"].(string)
		if !ok || description == "" {
			t.Error("expected non-empty description")
		}

		inputSchema, ok := def["inputSchema"].(map[string]interface{})
		if !ok {
			t.Fatal("expected inputSchema to be a map")
		}

		props, ok := inputSchema["properties"].(map[string]interface{})
		if !ok {
			t.Fatal("expected properties to be a map")
		}

		// Check required fields exist
		if _, ok := props["parent_session_id"]; !ok {
			t.Error("expected parent_session_id property")
		}
		if _, ok := props["fork_point_message_id"]; !ok {
			t.Error("expected fork_point_message_id property")
		}

		// Check required list
		required, ok := inputSchema["required"].([]string)
		if !ok {
			t.Fatal("expected required to be a string slice")
		}
		if len(required) != 2 {
			t.Errorf("expected 2 required fields, got %d", len(required))
		}
	})

	t.Run("GetForkInfoToolDef", func(t *testing.T) {
		def := GetForkInfoToolDef()

		if def["name"] != "get_fork_info" {
			t.Errorf("expected name 'get_fork_info', got '%s'", def["name"])
		}

		inputSchema, ok := def["inputSchema"].(map[string]interface{})
		if !ok {
			t.Fatal("expected inputSchema to be a map")
		}

		props, ok := inputSchema["properties"].(map[string]interface{})
		if !ok {
			t.Fatal("expected properties to be a map")
		}

		if _, ok := props["session_id"]; !ok {
			t.Error("expected session_id property")
		}

		required, ok := inputSchema["required"].([]string)
		if !ok {
			t.Fatal("expected required to be a string slice")
		}
		if len(required) != 1 || required[0] != "session_id" {
			t.Error("expected session_id to be required")
		}
	})

	t.Run("ListSessionForksToolDef", func(t *testing.T) {
		def := ListSessionForksToolDef()

		if def["name"] != "list_session_forks" {
			t.Errorf("expected name 'list_session_forks', got '%s'", def["name"])
		}

		inputSchema, ok := def["inputSchema"].(map[string]interface{})
		if !ok {
			t.Fatal("expected inputSchema to be a map")
		}

		props, ok := inputSchema["properties"].(map[string]interface{})
		if !ok {
			t.Fatal("expected properties to be a map")
		}

		if _, ok := props["session_id"]; !ok {
			t.Error("expected session_id property")
		}
	})
}
