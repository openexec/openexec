package telegram

import (
	"context"
	"strings"
	"testing"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/openexec/openexec/internal/protocol"
	"github.com/openexec/openexec/internal/user"
)

// mockSender is a mock implementation of MessageSender for testing.
type mockSender struct {
	sentMessages []tgbotapi.Chattable
}

func newMockSender() *mockSender {
	return &mockSender{
		sentMessages: make([]tgbotapi.Chattable, 0),
	}
}

func (m *mockSender) Send(c tgbotapi.Chattable) (tgbotapi.Message, error) {
	m.sentMessages = append(m.sentMessages, c)
	return tgbotapi.Message{}, nil
}

func (m *mockSender) lastMessage() *tgbotapi.MessageConfig {
	if len(m.sentMessages) == 0 {
		return nil
	}
	msg, ok := m.sentMessages[len(m.sentMessages)-1].(tgbotapi.MessageConfig)
	if !ok {
		return nil
	}
	return &msg
}

func TestHandleStartAuthorizedUser(t *testing.T) {
	// Setup
	store := user.NewMockStore()
	testUser, _ := user.NewUser(12345, user.RoleCustomer)
	_ = store.Create(context.Background(), testUser)

	authMiddleware := NewAuthMiddleware(store)
	sender := newMockSender()
	handler := NewCommandHandler(sender, authMiddleware)

	// Create a /start command update
	update := createStartCommandUpdate(12345, 67890, "John", "Doe", "johndoe")

	// Execute
	handler.HandleUpdate(context.Background(), update)

	// Verify
	msg := sender.lastMessage()
	if msg == nil {
		t.Fatal("Expected a message to be sent")
	}

	if msg.ChatID != 67890 {
		t.Errorf("Expected chat ID 67890, got %d", msg.ChatID)
	}

	if !strings.Contains(msg.Text, "Welcome") {
		t.Errorf("Expected welcome message, got: %s", msg.Text)
	}

	if !strings.Contains(msg.Text, "John Doe") {
		t.Errorf("Expected user name in message, got: %s", msg.Text)
	}

	if !strings.Contains(msg.Text, "customer") {
		t.Errorf("Expected role in message, got: %s", msg.Text)
	}
}

func TestHandleStartUnauthorizedUser(t *testing.T) {
	// Setup - empty store means no authorized users
	store := user.NewMockStore()
	authMiddleware := NewAuthMiddleware(store)
	sender := newMockSender()
	handler := NewCommandHandler(sender, authMiddleware)

	// Create a /start command update from unauthorized user
	update := createStartCommandUpdate(99999, 67890, "Unknown", "User", "unknown")

	// Execute
	handler.HandleUpdate(context.Background(), update)

	// Verify
	msg := sender.lastMessage()
	if msg == nil {
		t.Fatal("Expected a message to be sent")
	}

	if !strings.Contains(msg.Text, "Access denied") {
		t.Errorf("Expected access denied message, got: %s", msg.Text)
	}

	if !strings.Contains(msg.Text, "not authorized") {
		t.Errorf("Expected 'not authorized' in message, got: %s", msg.Text)
	}
}

func TestHandleStartWithDifferentRoles(t *testing.T) {
	tests := []struct {
		name         string
		role         user.Role
		expectedRole string
	}{
		{
			name:         "customer role",
			role:         user.RoleCustomer,
			expectedRole: "customer",
		},
		{
			name:         "provider role",
			role:         user.RoleProvider,
			expectedRole: "provider",
		},
		{
			name:         "admin role",
			role:         user.RoleAdmin,
			expectedRole: "admin",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := user.NewMockStore()
			testUser, _ := user.NewUser(12345, tt.role)
			_ = store.Create(context.Background(), testUser)

			authMiddleware := NewAuthMiddleware(store)
			sender := newMockSender()
			handler := NewCommandHandler(sender, authMiddleware)

			update := createStartCommandUpdate(12345, 67890, "Test", "User", "testuser")

			handler.HandleUpdate(context.Background(), update)

			msg := sender.lastMessage()
			if msg == nil {
				t.Fatal("Expected a message to be sent")
			}

			if !strings.Contains(msg.Text, tt.expectedRole) {
				t.Errorf("Expected role '%s' in message, got: %s", tt.expectedRole, msg.Text)
			}
		})
	}
}

func TestHandleUpdateIgnoresNonCommandMessages(t *testing.T) {
	store := user.NewMockStore()
	authMiddleware := NewAuthMiddleware(store)
	sender := newMockSender()
	handler := NewCommandHandler(sender, authMiddleware)

	// Create a regular text message (not a command)
	update := tgbotapi.Update{
		UpdateID: 1,
		Message: &tgbotapi.Message{
			MessageID: 1,
			Text:      "Hello, this is a regular message",
			Chat:      &tgbotapi.Chat{ID: 67890},
			From:      &tgbotapi.User{ID: 12345},
		},
	}

	handler.HandleUpdate(context.Background(), update)

	if len(sender.sentMessages) != 0 {
		t.Errorf("Expected no messages to be sent for non-command, got %d", len(sender.sentMessages))
	}
}

func TestHandleUpdateIgnoresNilMessage(t *testing.T) {
	store := user.NewMockStore()
	authMiddleware := NewAuthMiddleware(store)
	sender := newMockSender()
	handler := NewCommandHandler(sender, authMiddleware)

	// Create an update with no message
	update := tgbotapi.Update{
		UpdateID: 1,
		Message:  nil,
	}

	handler.HandleUpdate(context.Background(), update)

	if len(sender.sentMessages) != 0 {
		t.Errorf("Expected no messages to be sent for nil message, got %d", len(sender.sentMessages))
	}
}

func TestHandleUpdateIgnoresUnknownCommands(t *testing.T) {
	store := user.NewMockStore()
	authMiddleware := NewAuthMiddleware(store)
	sender := newMockSender()
	handler := NewCommandHandler(sender, authMiddleware)

	// Create an unknown command
	update := tgbotapi.Update{
		UpdateID: 1,
		Message: &tgbotapi.Message{
			MessageID: 1,
			Text:      "/unknowncommand",
			Chat:      &tgbotapi.Chat{ID: 67890},
			From:      &tgbotapi.User{ID: 12345},
			Entities: []tgbotapi.MessageEntity{
				{
					Type:   "bot_command",
					Offset: 0,
					Length: 15,
				},
			},
		},
	}

	handler.HandleUpdate(context.Background(), update)

	if len(sender.sentMessages) != 0 {
		t.Errorf("Expected no messages to be sent for unknown command, got %d", len(sender.sentMessages))
	}
}

func TestGetUserDisplayName(t *testing.T) {
	tests := []struct {
		name     string
		user     *tgbotapi.User
		expected string
	}{
		{
			name:     "nil user",
			user:     nil,
			expected: "User",
		},
		{
			name: "full name",
			user: &tgbotapi.User{
				FirstName: "John",
				LastName:  "Doe",
				UserName:  "johndoe",
			},
			expected: "John Doe",
		},
		{
			name: "first name only",
			user: &tgbotapi.User{
				FirstName: "John",
			},
			expected: "John",
		},
		{
			name: "username fallback",
			user: &tgbotapi.User{
				UserName: "johndoe",
			},
			expected: "@johndoe",
		},
		{
			name:     "empty user",
			user:     &tgbotapi.User{},
			expected: "User",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getUserDisplayName(tt.user)
			if result != tt.expected {
				t.Errorf("Expected '%s', got '%s'", tt.expected, result)
			}
		})
	}
}

func TestHandleStartWithNilFrom(t *testing.T) {
	store := user.NewMockStore()
	authMiddleware := NewAuthMiddleware(store)
	sender := newMockSender()
	handler := NewCommandHandler(sender, authMiddleware)

	// Create a /start command with nil From field
	update := tgbotapi.Update{
		UpdateID: 1,
		Message: &tgbotapi.Message{
			MessageID: 1,
			Text:      "/start",
			Chat:      &tgbotapi.Chat{ID: 67890},
			From:      nil, // No user info
			Entities: []tgbotapi.MessageEntity{
				{
					Type:   "bot_command",
					Offset: 0,
					Length: 6,
				},
			},
		},
	}

	handler.HandleUpdate(context.Background(), update)

	msg := sender.lastMessage()
	if msg == nil {
		t.Fatal("Expected a message to be sent")
	}

	if !strings.Contains(msg.Text, "Error") {
		t.Errorf("Expected error message, got: %s", msg.Text)
	}
}

// Helper function to create a /start command update.
func createStartCommandUpdate(userID, chatID int64, firstName, lastName, username string) tgbotapi.Update {
	return tgbotapi.Update{
		UpdateID: 1,
		Message: &tgbotapi.Message{
			MessageID: 1,
			Text:      "/start",
			Chat:      &tgbotapi.Chat{ID: chatID},
			From: &tgbotapi.User{
				ID:        userID,
				FirstName: firstName,
				LastName:  lastName,
				UserName:  username,
			},
			Entities: []tgbotapi.MessageEntity{
				{
					Type:   "bot_command",
					Offset: 0,
					Length: 6,
				},
			},
		},
	}
}

// Helper function to create a /status command update.
func createStatusCommandUpdate(userID, chatID int64, firstName, lastName, username string) tgbotapi.Update {
	return tgbotapi.Update{
		UpdateID: 1,
		Message: &tgbotapi.Message{
			MessageID: 1,
			Text:      "/status",
			Chat:      &tgbotapi.Chat{ID: chatID},
			From: &tgbotapi.User{
				ID:        userID,
				FirstName: firstName,
				LastName:  lastName,
				UserName:  username,
			},
			Entities: []tgbotapi.MessageEntity{
				{
					Type:   "bot_command",
					Offset: 0,
					Length: 7,
				},
			},
		},
	}
}

// mockStatusProvider is a mock implementation of StatusProvider for testing.
type mockStatusProvider struct {
	status *protocol.StatusResponse
}

func (m *mockStatusProvider) GetStatus() *protocol.StatusResponse {
	return m.status
}

func TestHandleStatusAuthorizedUser(t *testing.T) {
	// Setup
	store := user.NewMockStore()
	testUser, _ := user.NewUser(12345, user.RoleAdmin)
	_ = store.Create(context.Background(), testUser)

	authMiddleware := NewAuthMiddleware(store)
	sender := newMockSender()
	handler := NewCommandHandler(sender, authMiddleware)

	// Set up status provider
	statusProvider := &mockStatusProvider{
		status: &protocol.StatusResponse{
			BaseMessage: protocol.BaseMessage{
				Type:      protocol.TypeStatusResponse,
				RequestID: "test-123",
			},
			Status:  protocol.StatusOK,
			Message: "All systems operational",
			Version: "1.0.0",
		},
	}
	handler.SetStatusProvider(statusProvider)

	// Create a /status command update
	update := createStatusCommandUpdate(12345, 67890, "Admin", "User", "admin")

	// Execute
	handler.HandleUpdate(context.Background(), update)

	// Verify
	msg := sender.lastMessage()
	if msg == nil {
		t.Fatal("Expected a message to be sent")
	}

	if msg.ChatID != 67890 {
		t.Errorf("Expected chat ID 67890, got %d", msg.ChatID)
	}

	if !strings.Contains(msg.Text, "OK") {
		t.Errorf("Expected status OK in message, got: %s", msg.Text)
	}

	if !strings.Contains(msg.Text, "All systems operational") {
		t.Errorf("Expected status message in response, got: %s", msg.Text)
	}

	if !strings.Contains(msg.Text, "1.0.0") {
		t.Errorf("Expected version in message, got: %s", msg.Text)
	}
}

func TestHandleStatusUnauthorizedUser(t *testing.T) {
	// Setup - empty store means no authorized users
	store := user.NewMockStore()
	authMiddleware := NewAuthMiddleware(store)
	sender := newMockSender()
	handler := NewCommandHandler(sender, authMiddleware)

	// Create a /status command update from unauthorized user
	update := createStatusCommandUpdate(99999, 67890, "Unknown", "User", "unknown")

	// Execute
	handler.HandleUpdate(context.Background(), update)

	// Verify
	msg := sender.lastMessage()
	if msg == nil {
		t.Fatal("Expected a message to be sent")
	}

	if !strings.Contains(msg.Text, "Access denied") {
		t.Errorf("Expected access denied message, got: %s", msg.Text)
	}

	if !strings.Contains(msg.Text, "not authorized") {
		t.Errorf("Expected 'not authorized' in message, got: %s", msg.Text)
	}
}

func TestHandleStatusNoProvider(t *testing.T) {
	// Setup
	store := user.NewMockStore()
	testUser, _ := user.NewUser(12345, user.RoleAdmin)
	_ = store.Create(context.Background(), testUser)

	authMiddleware := NewAuthMiddleware(store)
	sender := newMockSender()
	handler := NewCommandHandler(sender, authMiddleware)
	// Intentionally not setting status provider

	// Create a /status command update
	update := createStatusCommandUpdate(12345, 67890, "Admin", "User", "admin")

	// Execute
	handler.HandleUpdate(context.Background(), update)

	// Verify
	msg := sender.lastMessage()
	if msg == nil {
		t.Fatal("Expected a message to be sent")
	}

	if !strings.Contains(msg.Text, "not available") {
		t.Errorf("Expected 'not available' in message, got: %s", msg.Text)
	}
}

func TestHandleStatusNilStatus(t *testing.T) {
	// Setup
	store := user.NewMockStore()
	testUser, _ := user.NewUser(12345, user.RoleAdmin)
	_ = store.Create(context.Background(), testUser)

	authMiddleware := NewAuthMiddleware(store)
	sender := newMockSender()
	handler := NewCommandHandler(sender, authMiddleware)

	// Set up status provider that returns nil
	statusProvider := &mockStatusProvider{
		status: nil,
	}
	handler.SetStatusProvider(statusProvider)

	// Create a /status command update
	update := createStatusCommandUpdate(12345, 67890, "Admin", "User", "admin")

	// Execute
	handler.HandleUpdate(context.Background(), update)

	// Verify
	msg := sender.lastMessage()
	if msg == nil {
		t.Fatal("Expected a message to be sent")
	}

	if !strings.Contains(msg.Text, "Unable to retrieve") {
		t.Errorf("Expected error message, got: %s", msg.Text)
	}
}

func TestHandleStatusWithDegradedStatus(t *testing.T) {
	// Setup
	store := user.NewMockStore()
	testUser, _ := user.NewUser(12345, user.RoleAdmin)
	_ = store.Create(context.Background(), testUser)

	authMiddleware := NewAuthMiddleware(store)
	sender := newMockSender()
	handler := NewCommandHandler(sender, authMiddleware)

	// Set up degraded status
	statusProvider := &mockStatusProvider{
		status: &protocol.StatusResponse{
			BaseMessage: protocol.BaseMessage{
				Type:      protocol.TypeStatusResponse,
				RequestID: "test-123",
			},
			Status:  protocol.StatusDegraded,
			Message: "Some services are slow",
		},
	}
	handler.SetStatusProvider(statusProvider)

	// Create a /status command update
	update := createStatusCommandUpdate(12345, 67890, "Admin", "User", "admin")

	// Execute
	handler.HandleUpdate(context.Background(), update)

	// Verify
	msg := sender.lastMessage()
	if msg == nil {
		t.Fatal("Expected a message to be sent")
	}

	if !strings.Contains(msg.Text, "DEGRADED") {
		t.Errorf("Expected DEGRADED status in message, got: %s", msg.Text)
	}

	if !strings.Contains(msg.Text, "Some services are slow") {
		t.Errorf("Expected status message in response, got: %s", msg.Text)
	}
}

func TestHandleStatusWithErrorStatus(t *testing.T) {
	// Setup
	store := user.NewMockStore()
	testUser, _ := user.NewUser(12345, user.RoleAdmin)
	_ = store.Create(context.Background(), testUser)

	authMiddleware := NewAuthMiddleware(store)
	sender := newMockSender()
	handler := NewCommandHandler(sender, authMiddleware)

	// Set up error status
	statusProvider := &mockStatusProvider{
		status: &protocol.StatusResponse{
			BaseMessage: protocol.BaseMessage{
				Type:      protocol.TypeStatusResponse,
				RequestID: "test-123",
			},
			Status:  protocol.StatusError,
			Message: "Critical failure",
		},
	}
	handler.SetStatusProvider(statusProvider)

	// Create a /status command update
	update := createStatusCommandUpdate(12345, 67890, "Admin", "User", "admin")

	// Execute
	handler.HandleUpdate(context.Background(), update)

	// Verify
	msg := sender.lastMessage()
	if msg == nil {
		t.Fatal("Expected a message to be sent")
	}

	if !strings.Contains(msg.Text, "ERROR") {
		t.Errorf("Expected ERROR status in message, got: %s", msg.Text)
	}
}

func TestHandleStatusWithConnections(t *testing.T) {
	// Setup
	store := user.NewMockStore()
	testUser, _ := user.NewUser(12345, user.RoleAdmin)
	_ = store.Create(context.Background(), testUser)

	authMiddleware := NewAuthMiddleware(store)
	sender := newMockSender()
	handler := NewCommandHandler(sender, authMiddleware)

	// Set up status with connections
	statusProvider := &mockStatusProvider{
		status: &protocol.StatusResponse{
			BaseMessage: protocol.BaseMessage{
				Type:      protocol.TypeStatusResponse,
				RequestID: "test-123",
			},
			Status: protocol.StatusOK,
			Connections: &protocol.ConnectionInfo{
				TotalConnections:         10,
				AuthenticatedConnections: 5,
			},
		},
	}
	handler.SetStatusProvider(statusProvider)

	// Create a /status command update
	update := createStatusCommandUpdate(12345, 67890, "Admin", "User", "admin")

	// Execute
	handler.HandleUpdate(context.Background(), update)

	// Verify
	msg := sender.lastMessage()
	if msg == nil {
		t.Fatal("Expected a message to be sent")
	}

	if !strings.Contains(msg.Text, "Connections: 10") {
		t.Errorf("Expected connection count in message, got: %s", msg.Text)
	}

	if !strings.Contains(msg.Text, "authenticated: 5") {
		t.Errorf("Expected authenticated count in message, got: %s", msg.Text)
	}
}

func TestHandleStatusWithMetrics(t *testing.T) {
	// Setup
	store := user.NewMockStore()
	testUser, _ := user.NewUser(12345, user.RoleAdmin)
	_ = store.Create(context.Background(), testUser)

	authMiddleware := NewAuthMiddleware(store)
	sender := newMockSender()
	handler := NewCommandHandler(sender, authMiddleware)

	// Set up status with metrics
	statusProvider := &mockStatusProvider{
		status: &protocol.StatusResponse{
			BaseMessage: protocol.BaseMessage{
				Type:      protocol.TypeStatusResponse,
				RequestID: "test-123",
			},
			Status: protocol.StatusOK,
			Metrics: &protocol.Metrics{
				UptimeSeconds:    3600,
				MessagesReceived: 100,
				MessagesSent:     50,
			},
		},
	}
	handler.SetStatusProvider(statusProvider)

	// Create a /status command update
	update := createStatusCommandUpdate(12345, 67890, "Admin", "User", "admin")

	// Execute
	handler.HandleUpdate(context.Background(), update)

	// Verify
	msg := sender.lastMessage()
	if msg == nil {
		t.Fatal("Expected a message to be sent")
	}

	if !strings.Contains(msg.Text, "Uptime: 3600s") {
		t.Errorf("Expected uptime in message, got: %s", msg.Text)
	}

	if !strings.Contains(msg.Text, "100 received") {
		t.Errorf("Expected messages received in message, got: %s", msg.Text)
	}

	if !strings.Contains(msg.Text, "50 sent") {
		t.Errorf("Expected messages sent in message, got: %s", msg.Text)
	}
}

func TestHandleStatusWithNilFrom(t *testing.T) {
	// Setup
	store := user.NewMockStore()
	authMiddleware := NewAuthMiddleware(store)
	sender := newMockSender()
	handler := NewCommandHandler(sender, authMiddleware)

	// Create a /status command with nil From field
	update := tgbotapi.Update{
		UpdateID: 1,
		Message: &tgbotapi.Message{
			MessageID: 1,
			Text:      "/status",
			Chat:      &tgbotapi.Chat{ID: 67890},
			From:      nil, // No user info
			Entities: []tgbotapi.MessageEntity{
				{
					Type:   "bot_command",
					Offset: 0,
					Length: 7,
				},
			},
		},
	}

	// Execute
	handler.HandleUpdate(context.Background(), update)

	// Verify
	msg := sender.lastMessage()
	if msg == nil {
		t.Fatal("Expected a message to be sent")
	}

	if !strings.Contains(msg.Text, "Error") {
		t.Errorf("Expected error message, got: %s", msg.Text)
	}
}

func TestFormatStatusMessage(t *testing.T) {
	tests := []struct {
		name     string
		status   *protocol.StatusResponse
		contains []string
	}{
		{
			name: "basic ok status",
			status: &protocol.StatusResponse{
				Status:  protocol.StatusOK,
				Message: "All good",
			},
			contains: []string{"OK", "All good"},
		},
		{
			name: "degraded with version",
			status: &protocol.StatusResponse{
				Status:  protocol.StatusDegraded,
				Version: "2.0.0",
			},
			contains: []string{"DEGRADED", "2.0.0"},
		},
		{
			name: "error status",
			status: &protocol.StatusResponse{
				Status:  protocol.StatusError,
				Message: "Something broke",
			},
			contains: []string{"ERROR", "Something broke"},
		},
		{
			name: "with connections no auth",
			status: &protocol.StatusResponse{
				Status: protocol.StatusOK,
				Connections: &protocol.ConnectionInfo{
					TotalConnections:         15,
					AuthenticatedConnections: 0,
				},
			},
			contains: []string{"Connections: 15"},
		},
		{
			name: "with metrics no messages",
			status: &protocol.StatusResponse{
				Status: protocol.StatusOK,
				Metrics: &protocol.Metrics{
					UptimeSeconds:    7200,
					MessagesReceived: 0,
					MessagesSent:     0,
				},
			},
			contains: []string{"Uptime: 7200s"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatStatusMessage(tt.status)
			for _, expected := range tt.contains {
				if !strings.Contains(result, expected) {
					t.Errorf("Expected '%s' in result, got: %s", expected, result)
				}
			}
		})
	}
}

// mockAggregatedStatusProvider is a mock implementation of AggregatedStatusProvider for testing.
type mockAggregatedStatusProvider struct {
	status *AggregatedClientStatus
	err    error
}

func (m *mockAggregatedStatusProvider) BroadcastStatus(ctx context.Context, includeMetrics bool) (*AggregatedClientStatus, error) {
	return m.status, m.err
}

// Helper function to create a /clients command update.
func createClientsCommandUpdate(userID, chatID int64, firstName, lastName, username string) tgbotapi.Update {
	return tgbotapi.Update{
		UpdateID: 1,
		Message: &tgbotapi.Message{
			MessageID: 1,
			Text:      "/clients",
			Chat:      &tgbotapi.Chat{ID: chatID},
			From: &tgbotapi.User{
				ID:        userID,
				FirstName: firstName,
				LastName:  lastName,
				UserName:  username,
			},
			Entities: []tgbotapi.MessageEntity{
				{
					Type:   "bot_command",
					Offset: 0,
					Length: 8,
				},
			},
		},
	}
}

func TestHandleClientsAuthorizedUserWithClients(t *testing.T) {
	// Setup
	store := user.NewMockStore()
	testUser, _ := user.NewUser(12345, user.RoleAdmin)
	_ = store.Create(context.Background(), testUser)

	authMiddleware := NewAuthMiddleware(store)
	sender := newMockSender()
	handler := NewCommandHandler(sender, authMiddleware)

	// Set up aggregated status provider with clients
	provider := &mockAggregatedStatusProvider{
		status: &AggregatedClientStatus{
			TotalClients:  2,
			Responded:     2,
			TimedOut:      0,
			OverallStatus: protocol.StatusOK,
			ClientStatuses: []ClientStatusInfo{
				{
					ClientID:  "client-1",
					ProjectID: "project-a",
					MachineID: "machine-x",
					Status:    protocol.StatusOK,
					Message:   "Running",
				},
				{
					ClientID:  "client-2",
					ProjectID: "project-b",
					Status:    protocol.StatusDegraded,
					Message:   "High load",
				},
			},
		},
	}
	handler.SetAggregatedStatusProvider(provider)

	// Create a /clients command update
	update := createClientsCommandUpdate(12345, 67890, "Admin", "User", "admin")

	// Execute
	handler.HandleUpdate(context.Background(), update)

	// Verify - there should be 2 messages: "Querying..." and the result
	if len(sender.sentMessages) < 2 {
		t.Fatalf("Expected at least 2 messages, got %d", len(sender.sentMessages))
	}

	msg := sender.lastMessage()
	if msg == nil {
		t.Fatal("Expected a message to be sent")
	}

	if msg.ChatID != 67890 {
		t.Errorf("Expected chat ID 67890, got %d", msg.ChatID)
	}

	// Verify content
	if !strings.Contains(msg.Text, "OK") {
		t.Errorf("Expected OK status in message, got: %s", msg.Text)
	}

	if !strings.Contains(msg.Text, "2 total") {
		t.Errorf("Expected '2 total' in message, got: %s", msg.Text)
	}

	if !strings.Contains(msg.Text, "project-a") {
		t.Errorf("Expected 'project-a' in message, got: %s", msg.Text)
	}
}

func TestHandleClientsUnauthorizedUser(t *testing.T) {
	// Setup - empty store means no authorized users
	store := user.NewMockStore()
	authMiddleware := NewAuthMiddleware(store)
	sender := newMockSender()
	handler := NewCommandHandler(sender, authMiddleware)

	// Create a /clients command update from unauthorized user
	update := createClientsCommandUpdate(99999, 67890, "Unknown", "User", "unknown")

	// Execute
	handler.HandleUpdate(context.Background(), update)

	// Verify
	msg := sender.lastMessage()
	if msg == nil {
		t.Fatal("Expected a message to be sent")
	}

	if !strings.Contains(msg.Text, "Access denied") {
		t.Errorf("Expected access denied message, got: %s", msg.Text)
	}
}

func TestHandleClientsNoProvider(t *testing.T) {
	// Setup
	store := user.NewMockStore()
	testUser, _ := user.NewUser(12345, user.RoleAdmin)
	_ = store.Create(context.Background(), testUser)

	authMiddleware := NewAuthMiddleware(store)
	sender := newMockSender()
	handler := NewCommandHandler(sender, authMiddleware)
	// Intentionally not setting aggregated status provider

	// Create a /clients command update
	update := createClientsCommandUpdate(12345, 67890, "Admin", "User", "admin")

	// Execute
	handler.HandleUpdate(context.Background(), update)

	// Verify
	msg := sender.lastMessage()
	if msg == nil {
		t.Fatal("Expected a message to be sent")
	}

	if !strings.Contains(msg.Text, "not available") {
		t.Errorf("Expected 'not available' in message, got: %s", msg.Text)
	}
}

func TestHandleClientsNoClients(t *testing.T) {
	// Setup
	store := user.NewMockStore()
	testUser, _ := user.NewUser(12345, user.RoleAdmin)
	_ = store.Create(context.Background(), testUser)

	authMiddleware := NewAuthMiddleware(store)
	sender := newMockSender()
	handler := NewCommandHandler(sender, authMiddleware)

	// Set up aggregated status provider with no clients
	provider := &mockAggregatedStatusProvider{
		status: &AggregatedClientStatus{
			TotalClients:   0,
			Responded:      0,
			TimedOut:       0,
			OverallStatus:  protocol.StatusOK,
			ClientStatuses: []ClientStatusInfo{},
		},
	}
	handler.SetAggregatedStatusProvider(provider)

	// Create a /clients command update
	update := createClientsCommandUpdate(12345, 67890, "Admin", "User", "admin")

	// Execute
	handler.HandleUpdate(context.Background(), update)

	// Verify - should have "Querying..." and "No OpenExec clients connected."
	msg := sender.lastMessage()
	if msg == nil {
		t.Fatal("Expected a message to be sent")
	}

	if !strings.Contains(msg.Text, "No OpenExec clients connected") {
		t.Errorf("Expected 'No OpenExec clients connected' in message, got: %s", msg.Text)
	}
}

func TestHandleClientsWithError(t *testing.T) {
	// Setup
	store := user.NewMockStore()
	testUser, _ := user.NewUser(12345, user.RoleAdmin)
	_ = store.Create(context.Background(), testUser)

	authMiddleware := NewAuthMiddleware(store)
	sender := newMockSender()
	handler := NewCommandHandler(sender, authMiddleware)

	// Set up aggregated status provider that returns an error
	provider := &mockAggregatedStatusProvider{
		status: nil,
		err:    context.DeadlineExceeded,
	}
	handler.SetAggregatedStatusProvider(provider)

	// Create a /clients command update
	update := createClientsCommandUpdate(12345, 67890, "Admin", "User", "admin")

	// Execute
	handler.HandleUpdate(context.Background(), update)

	// Verify
	msg := sender.lastMessage()
	if msg == nil {
		t.Fatal("Expected a message to be sent")
	}

	if !strings.Contains(msg.Text, "Error") {
		t.Errorf("Expected error message, got: %s", msg.Text)
	}
}

func TestHandleClientsWithNilFrom(t *testing.T) {
	// Setup
	store := user.NewMockStore()
	authMiddleware := NewAuthMiddleware(store)
	sender := newMockSender()
	handler := NewCommandHandler(sender, authMiddleware)

	// Create a /clients command with nil From field
	update := tgbotapi.Update{
		UpdateID: 1,
		Message: &tgbotapi.Message{
			MessageID: 1,
			Text:      "/clients",
			Chat:      &tgbotapi.Chat{ID: 67890},
			From:      nil, // No user info
			Entities: []tgbotapi.MessageEntity{
				{
					Type:   "bot_command",
					Offset: 0,
					Length: 8,
				},
			},
		},
	}

	// Execute
	handler.HandleUpdate(context.Background(), update)

	// Verify
	msg := sender.lastMessage()
	if msg == nil {
		t.Fatal("Expected a message to be sent")
	}

	if !strings.Contains(msg.Text, "Error") {
		t.Errorf("Expected error message, got: %s", msg.Text)
	}
}

func TestFormatAggregatedStatus(t *testing.T) {
	tests := []struct {
		name     string
		status   *AggregatedClientStatus
		contains []string
	}{
		{
			name: "no clients",
			status: &AggregatedClientStatus{
				TotalClients:   0,
				ClientStatuses: []ClientStatusInfo{},
			},
			contains: []string{"No OpenExec clients connected"},
		},
		{
			name: "all ok",
			status: &AggregatedClientStatus{
				TotalClients:  2,
				Responded:     2,
				TimedOut:      0,
				OverallStatus: protocol.StatusOK,
				ClientStatuses: []ClientStatusInfo{
					{ClientID: "c1", ProjectID: "proj1", Status: protocol.StatusOK},
					{ClientID: "c2", ProjectID: "proj2", Status: protocol.StatusOK},
				},
			},
			contains: []string{"OK", "2 total", "2 responded"},
		},
		{
			name: "with timeout",
			status: &AggregatedClientStatus{
				TotalClients:  3,
				Responded:     2,
				TimedOut:      1,
				OverallStatus: protocol.StatusDegraded,
				ClientStatuses: []ClientStatusInfo{
					{ClientID: "c1", Status: protocol.StatusOK},
					{ClientID: "c2", Status: protocol.StatusOK},
					{ClientID: "c3", Error: "timeout"},
				},
			},
			contains: []string{"DEGRADED", "3 total", "1 timed out", "timeout"},
		},
		{
			name: "with error status",
			status: &AggregatedClientStatus{
				TotalClients:  1,
				Responded:     1,
				TimedOut:      0,
				OverallStatus: protocol.StatusError,
				ClientStatuses: []ClientStatusInfo{
					{ClientID: "c1", Status: protocol.StatusError, Message: "Critical failure"},
				},
			},
			contains: []string{"ERROR", "Critical failure"},
		},
		{
			name: "with project and machine IDs",
			status: &AggregatedClientStatus{
				TotalClients:  1,
				Responded:     1,
				TimedOut:      0,
				OverallStatus: protocol.StatusOK,
				ClientStatuses: []ClientStatusInfo{
					{ClientID: "abc12345-long-uuid", ProjectID: "my-project", MachineID: "server-1", Status: protocol.StatusOK},
				},
			},
			contains: []string{"my-project", "server-1"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatAggregatedStatus(tt.status)
			for _, expected := range tt.contains {
				if !strings.Contains(result, expected) {
					t.Errorf("Expected '%s' in result, got: %s", expected, result)
				}
			}
		})
	}
}

func TestTruncateID(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"short", "short"},
		{"12345678", "12345678"},
		{"123456789", "12345678..."},
		{"abc12345-1234-5678-9abc-def012345678", "abc12345..."},
	}

	for _, tt := range tests {
		result := truncateID(tt.input)
		if result != tt.expected {
			t.Errorf("truncateID(%q) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}

// mockRunCommandSender is a mock implementation of RunCommandSender for testing.
type mockRunCommandSender struct {
	result        *RunCommandResult
	err           error
	lastTaskID    string
	lastProjectID string
	callCount     int
}

func (m *mockRunCommandSender) SendRunCommand(ctx context.Context, taskID, projectID string) (*RunCommandResult, error) {
	m.lastTaskID = taskID
	m.lastProjectID = projectID
	m.callCount++
	return m.result, m.err
}

// Helper function to create a /run command update.
func createRunCommandUpdate(userID, chatID int64, firstName, lastName, username, args string) tgbotapi.Update {
	text := "/run"
	if args != "" {
		text = "/run " + args
	}
	return tgbotapi.Update{
		UpdateID: 1,
		Message: &tgbotapi.Message{
			MessageID: 1,
			Text:      text,
			Chat:      &tgbotapi.Chat{ID: chatID},
			From: &tgbotapi.User{
				ID:        userID,
				FirstName: firstName,
				LastName:  lastName,
				UserName:  username,
			},
			Entities: []tgbotapi.MessageEntity{
				{
					Type:   "bot_command",
					Offset: 0,
					Length: 4,
				},
			},
		},
	}
}

func TestHandleRunAuthorizedUserWithValidTaskID(t *testing.T) {
	// Setup
	store := user.NewMockStore()
	testUser, _ := user.NewUser(12345, user.RoleAdmin)
	_ = store.Create(context.Background(), testUser)

	authMiddleware := NewAuthMiddleware(store)
	sender := newMockSender()
	handler := NewCommandHandler(sender, authMiddleware)

	// Set up run command sender
	runSender := &mockRunCommandSender{
		result: &RunCommandResult{
			TaskID:    "task-123",
			Status:    protocol.RunStatusAccepted,
			Message:   "Task accepted for execution",
			ProjectID: "my-project",
		},
	}
	handler.SetRunCommandSender(runSender)

	// Create a /run command update with task ID
	update := createRunCommandUpdate(12345, 67890, "Admin", "User", "admin", "task-123")

	// Execute
	handler.HandleUpdate(context.Background(), update)

	// Verify - should have 2 messages: "Sending..." and the result
	if len(sender.sentMessages) < 2 {
		t.Fatalf("Expected at least 2 messages, got %d", len(sender.sentMessages))
	}

	msg := sender.lastMessage()
	if msg == nil {
		t.Fatal("Expected a message to be sent")
	}

	if msg.ChatID != 67890 {
		t.Errorf("Expected chat ID 67890, got %d", msg.ChatID)
	}

	// Verify content
	if !strings.Contains(msg.Text, "task-123") {
		t.Errorf("Expected task ID in message, got: %s", msg.Text)
	}

	if !strings.Contains(msg.Text, "ACCEPTED") {
		t.Errorf("Expected ACCEPTED status in message, got: %s", msg.Text)
	}

	if !strings.Contains(msg.Text, "my-project") {
		t.Errorf("Expected project ID in message, got: %s", msg.Text)
	}
}

func TestHandleRunUnauthorizedUser(t *testing.T) {
	// Setup - empty store means no authorized users
	store := user.NewMockStore()
	authMiddleware := NewAuthMiddleware(store)
	sender := newMockSender()
	handler := NewCommandHandler(sender, authMiddleware)

	// Create a /run command update from unauthorized user
	update := createRunCommandUpdate(99999, 67890, "Unknown", "User", "unknown", "task-123")

	// Execute
	handler.HandleUpdate(context.Background(), update)

	// Verify
	msg := sender.lastMessage()
	if msg == nil {
		t.Fatal("Expected a message to be sent")
	}

	if !strings.Contains(msg.Text, "Access denied") {
		t.Errorf("Expected access denied message, got: %s", msg.Text)
	}
}

func TestHandleRunNoProvider(t *testing.T) {
	// Setup
	store := user.NewMockStore()
	testUser, _ := user.NewUser(12345, user.RoleAdmin)
	_ = store.Create(context.Background(), testUser)

	authMiddleware := NewAuthMiddleware(store)
	sender := newMockSender()
	handler := NewCommandHandler(sender, authMiddleware)
	// Intentionally not setting run command sender

	// Create a /run command update
	update := createRunCommandUpdate(12345, 67890, "Admin", "User", "admin", "task-123")

	// Execute
	handler.HandleUpdate(context.Background(), update)

	// Verify
	msg := sender.lastMessage()
	if msg == nil {
		t.Fatal("Expected a message to be sent")
	}

	if !strings.Contains(msg.Text, "not available") {
		t.Errorf("Expected 'not available' in message, got: %s", msg.Text)
	}
}

func TestHandleRunMissingTaskID(t *testing.T) {
	// Setup
	store := user.NewMockStore()
	testUser, _ := user.NewUser(12345, user.RoleAdmin)
	_ = store.Create(context.Background(), testUser)

	authMiddleware := NewAuthMiddleware(store)
	sender := newMockSender()
	handler := NewCommandHandler(sender, authMiddleware)

	// Set up run command sender (won't be used)
	runSender := &mockRunCommandSender{}
	handler.SetRunCommandSender(runSender)

	// Create a /run command update WITHOUT task ID
	update := createRunCommandUpdate(12345, 67890, "Admin", "User", "admin", "")

	// Execute
	handler.HandleUpdate(context.Background(), update)

	// Verify
	msg := sender.lastMessage()
	if msg == nil {
		t.Fatal("Expected a message to be sent")
	}

	if !strings.Contains(msg.Text, "Usage:") {
		t.Errorf("Expected usage message, got: %s", msg.Text)
	}

	if !strings.Contains(msg.Text, "/run <task-id>") {
		t.Errorf("Expected usage format in message, got: %s", msg.Text)
	}

	if !strings.Contains(msg.Text, "[project-id]") {
		t.Errorf("Expected project-id hint in message, got: %s", msg.Text)
	}
}

func TestHandleRunWithError(t *testing.T) {
	// Setup
	store := user.NewMockStore()
	testUser, _ := user.NewUser(12345, user.RoleAdmin)
	_ = store.Create(context.Background(), testUser)

	authMiddleware := NewAuthMiddleware(store)
	sender := newMockSender()
	handler := NewCommandHandler(sender, authMiddleware)

	// Set up run command sender that returns an error
	runSender := &mockRunCommandSender{
		result: nil,
		err:    context.DeadlineExceeded,
	}
	handler.SetRunCommandSender(runSender)

	// Create a /run command update
	update := createRunCommandUpdate(12345, 67890, "Admin", "User", "admin", "task-123")

	// Execute
	handler.HandleUpdate(context.Background(), update)

	// Verify
	msg := sender.lastMessage()
	if msg == nil {
		t.Fatal("Expected a message to be sent")
	}

	if !strings.Contains(msg.Text, "Error") {
		t.Errorf("Expected error message, got: %s", msg.Text)
	}
}

func TestHandleRunWithNilFrom(t *testing.T) {
	// Setup
	store := user.NewMockStore()
	authMiddleware := NewAuthMiddleware(store)
	sender := newMockSender()
	handler := NewCommandHandler(sender, authMiddleware)

	// Create a /run command with nil From field
	update := tgbotapi.Update{
		UpdateID: 1,
		Message: &tgbotapi.Message{
			MessageID: 1,
			Text:      "/run task-123",
			Chat:      &tgbotapi.Chat{ID: 67890},
			From:      nil, // No user info
			Entities: []tgbotapi.MessageEntity{
				{
					Type:   "bot_command",
					Offset: 0,
					Length: 4,
				},
			},
		},
	}

	// Execute
	handler.HandleUpdate(context.Background(), update)

	// Verify
	msg := sender.lastMessage()
	if msg == nil {
		t.Fatal("Expected a message to be sent")
	}

	if !strings.Contains(msg.Text, "Error") {
		t.Errorf("Expected error message, got: %s", msg.Text)
	}
}

func TestHandleRunWithFailedStatus(t *testing.T) {
	// Setup
	store := user.NewMockStore()
	testUser, _ := user.NewUser(12345, user.RoleAdmin)
	_ = store.Create(context.Background(), testUser)

	authMiddleware := NewAuthMiddleware(store)
	sender := newMockSender()
	handler := NewCommandHandler(sender, authMiddleware)

	// Set up run command sender with failed status
	runSender := &mockRunCommandSender{
		result: &RunCommandResult{
			TaskID:    "task-123",
			Status:    protocol.RunStatusFailed,
			Message:   "Task execution failed",
			Error:     "Internal error",
			ProjectID: "my-project",
		},
	}
	handler.SetRunCommandSender(runSender)

	// Create a /run command update
	update := createRunCommandUpdate(12345, 67890, "Admin", "User", "admin", "task-123")

	// Execute
	handler.HandleUpdate(context.Background(), update)

	// Verify
	msg := sender.lastMessage()
	if msg == nil {
		t.Fatal("Expected a message to be sent")
	}

	if !strings.Contains(msg.Text, "FAILED") {
		t.Errorf("Expected FAILED status in message, got: %s", msg.Text)
	}

	if !strings.Contains(msg.Text, "Internal error") {
		t.Errorf("Expected error message in response, got: %s", msg.Text)
	}
}

func TestHandleRunWithRejectedStatus(t *testing.T) {
	// Setup
	store := user.NewMockStore()
	testUser, _ := user.NewUser(12345, user.RoleAdmin)
	_ = store.Create(context.Background(), testUser)

	authMiddleware := NewAuthMiddleware(store)
	sender := newMockSender()
	handler := NewCommandHandler(sender, authMiddleware)

	// Set up run command sender with rejected status
	runSender := &mockRunCommandSender{
		result: &RunCommandResult{
			TaskID:  "task-123",
			Status:  protocol.RunStatusRejected,
			Message: "Task was rejected",
			Error:   "No clients available",
		},
	}
	handler.SetRunCommandSender(runSender)

	// Create a /run command update
	update := createRunCommandUpdate(12345, 67890, "Admin", "User", "admin", "task-123")

	// Execute
	handler.HandleUpdate(context.Background(), update)

	// Verify
	msg := sender.lastMessage()
	if msg == nil {
		t.Fatal("Expected a message to be sent")
	}

	if !strings.Contains(msg.Text, "REJECTED") {
		t.Errorf("Expected REJECTED status in message, got: %s", msg.Text)
	}
}

func TestHandleRunWithProjectID(t *testing.T) {
	// Setup
	store := user.NewMockStore()
	testUser, _ := user.NewUser(12345, user.RoleAdmin)
	_ = store.Create(context.Background(), testUser)

	authMiddleware := NewAuthMiddleware(store)
	sender := newMockSender()
	handler := NewCommandHandler(sender, authMiddleware)

	// Set up run command sender
	runSender := &mockRunCommandSender{
		result: &RunCommandResult{
			TaskID:    "task-456",
			Status:    protocol.RunStatusAccepted,
			Message:   "Task accepted for execution",
			ProjectID: "target-project",
		},
	}
	handler.SetRunCommandSender(runSender)

	// Create a /run command update with task ID AND project ID
	update := createRunCommandUpdate(12345, 67890, "Admin", "User", "admin", "task-456 target-project")

	// Execute
	handler.HandleUpdate(context.Background(), update)

	// Verify the run sender received the correct parameters
	if runSender.lastTaskID != "task-456" {
		t.Errorf("Expected task ID 'task-456', got '%s'", runSender.lastTaskID)
	}

	if runSender.lastProjectID != "target-project" {
		t.Errorf("Expected project ID 'target-project', got '%s'", runSender.lastProjectID)
	}

	// Verify message content
	msg := sender.lastMessage()
	if msg == nil {
		t.Fatal("Expected a message to be sent")
	}

	if !strings.Contains(msg.Text, "target-project") {
		t.Errorf("Expected project ID in message, got: %s", msg.Text)
	}
}

func TestHandleRunWithoutProjectID(t *testing.T) {
	// Setup
	store := user.NewMockStore()
	testUser, _ := user.NewUser(12345, user.RoleAdmin)
	_ = store.Create(context.Background(), testUser)

	authMiddleware := NewAuthMiddleware(store)
	sender := newMockSender()
	handler := NewCommandHandler(sender, authMiddleware)

	// Set up run command sender
	runSender := &mockRunCommandSender{
		result: &RunCommandResult{
			TaskID:    "task-789",
			Status:    protocol.RunStatusAccepted,
			Message:   "Task accepted for execution",
			ProjectID: "auto-resolved-project",
		},
	}
	handler.SetRunCommandSender(runSender)

	// Create a /run command update with ONLY task ID (no project)
	update := createRunCommandUpdate(12345, 67890, "Admin", "User", "admin", "task-789")

	// Execute
	handler.HandleUpdate(context.Background(), update)

	// Verify the run sender received empty project ID
	if runSender.lastTaskID != "task-789" {
		t.Errorf("Expected task ID 'task-789', got '%s'", runSender.lastTaskID)
	}

	if runSender.lastProjectID != "" {
		t.Errorf("Expected empty project ID, got '%s'", runSender.lastProjectID)
	}
}

func TestHandleRunWithExecutorRole(t *testing.T) {
	// Setup - user with executor role should be allowed to run commands
	store := user.NewMockStore()
	testUser, _ := user.NewUser(12345, user.RoleExecutor)
	_ = store.Create(context.Background(), testUser)

	authMiddleware := NewAuthMiddleware(store)
	sender := newMockSender()
	handler := NewCommandHandler(sender, authMiddleware)

	// Set up run command sender
	runSender := &mockRunCommandSender{
		result: &RunCommandResult{
			TaskID:  "task-123",
			Status:  protocol.RunStatusAccepted,
			Message: "Task accepted for execution",
		},
	}
	handler.SetRunCommandSender(runSender)

	// Create a /run command update
	update := createRunCommandUpdate(12345, 67890, "Executor", "User", "executor", "task-123")

	// Execute
	handler.HandleUpdate(context.Background(), update)

	// Verify the command was processed (run sender was called)
	if runSender.callCount != 1 {
		t.Errorf("Expected run sender to be called once, got %d calls", runSender.callCount)
	}

	// Verify last message contains the result, not a permission error
	msg := sender.lastMessage()
	if msg == nil {
		t.Fatal("Expected a message to be sent")
	}

	if strings.Contains(msg.Text, "Permission denied") {
		t.Errorf("Expected run to succeed for executor role, got: %s", msg.Text)
	}

	if !strings.Contains(msg.Text, "ACCEPTED") {
		t.Errorf("Expected ACCEPTED status in message, got: %s", msg.Text)
	}
}

func TestHandleRunWithCustomerRoleDenied(t *testing.T) {
	// Setup - user with customer role should be denied
	store := user.NewMockStore()
	testUser, _ := user.NewUser(12345, user.RoleCustomer)
	_ = store.Create(context.Background(), testUser)

	authMiddleware := NewAuthMiddleware(store)
	sender := newMockSender()
	handler := NewCommandHandler(sender, authMiddleware)

	// Set up run command sender (should NOT be called)
	runSender := &mockRunCommandSender{
		result: &RunCommandResult{
			TaskID:  "task-123",
			Status:  protocol.RunStatusAccepted,
			Message: "Task accepted",
		},
	}
	handler.SetRunCommandSender(runSender)

	// Create a /run command update
	update := createRunCommandUpdate(12345, 67890, "Customer", "User", "customer", "task-123")

	// Execute
	handler.HandleUpdate(context.Background(), update)

	// Verify the run sender was NOT called
	if runSender.callCount != 0 {
		t.Errorf("Expected run sender NOT to be called for customer role, got %d calls", runSender.callCount)
	}

	// Verify permission denied message
	msg := sender.lastMessage()
	if msg == nil {
		t.Fatal("Expected a message to be sent")
	}

	if !strings.Contains(msg.Text, "Permission denied") {
		t.Errorf("Expected 'Permission denied' message, got: %s", msg.Text)
	}

	if !strings.Contains(msg.Text, "executor") || !strings.Contains(msg.Text, "admin") {
		t.Errorf("Expected message to mention required roles, got: %s", msg.Text)
	}
}

func TestHandleRunWithProviderRoleDenied(t *testing.T) {
	// Setup - user with provider role should be denied
	store := user.NewMockStore()
	testUser, _ := user.NewUser(12345, user.RoleProvider)
	_ = store.Create(context.Background(), testUser)

	authMiddleware := NewAuthMiddleware(store)
	sender := newMockSender()
	handler := NewCommandHandler(sender, authMiddleware)

	// Set up run command sender (should NOT be called)
	runSender := &mockRunCommandSender{
		result: &RunCommandResult{
			TaskID:  "task-123",
			Status:  protocol.RunStatusAccepted,
			Message: "Task accepted",
		},
	}
	handler.SetRunCommandSender(runSender)

	// Create a /run command update
	update := createRunCommandUpdate(12345, 67890, "Provider", "User", "provider", "task-123")

	// Execute
	handler.HandleUpdate(context.Background(), update)

	// Verify the run sender was NOT called
	if runSender.callCount != 0 {
		t.Errorf("Expected run sender NOT to be called for provider role, got %d calls", runSender.callCount)
	}

	// Verify permission denied message
	msg := sender.lastMessage()
	if msg == nil {
		t.Fatal("Expected a message to be sent")
	}

	if !strings.Contains(msg.Text, "Permission denied") {
		t.Errorf("Expected 'Permission denied' message, got: %s", msg.Text)
	}
}

func TestFormatRunResult(t *testing.T) {
	tests := []struct {
		name     string
		result   *RunCommandResult
		contains []string
	}{
		{
			name: "accepted status",
			result: &RunCommandResult{
				TaskID:    "task-123",
				Status:    protocol.RunStatusAccepted,
				Message:   "Task accepted",
				ProjectID: "project-a",
			},
			contains: []string{"task-123", "ACCEPTED", "project-a", "Task accepted"},
		},
		{
			name: "running status",
			result: &RunCommandResult{
				TaskID: "task-456",
				Status: protocol.RunStatusRunning,
			},
			contains: []string{"task-456", "RUNNING"},
		},
		{
			name: "completed status",
			result: &RunCommandResult{
				TaskID:  "task-789",
				Status:  protocol.RunStatusCompleted,
				Message: "All done!",
			},
			contains: []string{"task-789", "COMPLETED", "All done!"},
		},
		{
			name: "failed with error",
			result: &RunCommandResult{
				TaskID: "task-fail",
				Status: protocol.RunStatusFailed,
				Error:  "Something went wrong",
			},
			contains: []string{"task-fail", "FAILED", "Error:", "Something went wrong"},
		},
		{
			name: "rejected status",
			result: &RunCommandResult{
				TaskID: "task-reject",
				Status: protocol.RunStatusRejected,
				Error:  "Not allowed",
			},
			contains: []string{"task-reject", "REJECTED", "Not allowed"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatRunResult(tt.result)
			for _, expected := range tt.contains {
				if !strings.Contains(result, expected) {
					t.Errorf("Expected '%s' in result, got: %s", expected, result)
				}
			}
		})
	}
}

// mockLogsCommandSender is a mock implementation of LogsCommandSender for testing.
type mockLogsCommandSender struct {
	result        *LogsCommandResult
	err           error
	lastTaskID    string
	lastProjectID string
	callCount     int
}

func (m *mockLogsCommandSender) SendLogsCommand(ctx context.Context, taskID, projectID string) (*LogsCommandResult, error) {
	m.lastTaskID = taskID
	m.lastProjectID = projectID
	m.callCount++
	return m.result, m.err
}

// Helper function to create a /logs command update.
func createLogsCommandUpdate(userID, chatID int64, firstName, lastName, username, args string) tgbotapi.Update {
	text := "/logs"
	if args != "" {
		text = "/logs " + args
	}
	return tgbotapi.Update{
		UpdateID: 1,
		Message: &tgbotapi.Message{
			MessageID: 1,
			Text:      text,
			Chat:      &tgbotapi.Chat{ID: chatID},
			From: &tgbotapi.User{
				ID:        userID,
				FirstName: firstName,
				LastName:  lastName,
				UserName:  username,
			},
			Entities: []tgbotapi.MessageEntity{
				{
					Type:   "bot_command",
					Offset: 0,
					Length: 5,
				},
			},
		},
	}
}

func TestHandleLogsAuthorizedUserWithValidTaskID(t *testing.T) {
	// Setup
	store := user.NewMockStore()
	testUser, _ := user.NewUser(12345, user.RoleAdmin)
	_ = store.Create(context.Background(), testUser)

	authMiddleware := NewAuthMiddleware(store)
	sender := newMockSender()
	handler := NewCommandHandler(sender, authMiddleware)

	// Set up logs command sender
	logsSender := &mockLogsCommandSender{
		result: &LogsCommandResult{
			TaskID:    "task-123",
			ProjectID: "my-project",
			Entries: []protocol.LogEntry{
				{
					Timestamp: "2024-01-15T10:30:00Z",
					Level:     protocol.LogLevelInfo,
					Message:   "Task started",
				},
				{
					Timestamp: "2024-01-15T10:30:05Z",
					Level:     protocol.LogLevelInfo,
					Message:   "Task completed",
				},
			},
		},
	}
	handler.SetLogsCommandSender(logsSender)

	// Create a /logs command update with task ID
	update := createLogsCommandUpdate(12345, 67890, "Admin", "User", "admin", "task-123")

	// Execute
	handler.HandleUpdate(context.Background(), update)

	// Verify - should have 2 messages: "Fetching..." and the result
	if len(sender.sentMessages) < 2 {
		t.Fatalf("Expected at least 2 messages, got %d", len(sender.sentMessages))
	}

	msg := sender.lastMessage()
	if msg == nil {
		t.Fatal("Expected a message to be sent")
	}

	if msg.ChatID != 67890 {
		t.Errorf("Expected chat ID 67890, got %d", msg.ChatID)
	}

	// Verify content
	if !strings.Contains(msg.Text, "task-123") {
		t.Errorf("Expected task ID in message, got: %s", msg.Text)
	}

	if !strings.Contains(msg.Text, "my-project") {
		t.Errorf("Expected project ID in message, got: %s", msg.Text)
	}

	if !strings.Contains(msg.Text, "2 log entries") {
		t.Errorf("Expected '2 log entries' in message, got: %s", msg.Text)
	}

	if !strings.Contains(msg.Text, "Task started") {
		t.Errorf("Expected log message in response, got: %s", msg.Text)
	}
}

func TestHandleLogsUnauthorizedUser(t *testing.T) {
	// Setup - empty store means no authorized users
	store := user.NewMockStore()
	authMiddleware := NewAuthMiddleware(store)
	sender := newMockSender()
	handler := NewCommandHandler(sender, authMiddleware)

	// Create a /logs command update from unauthorized user
	update := createLogsCommandUpdate(99999, 67890, "Unknown", "User", "unknown", "task-123")

	// Execute
	handler.HandleUpdate(context.Background(), update)

	// Verify
	msg := sender.lastMessage()
	if msg == nil {
		t.Fatal("Expected a message to be sent")
	}

	if !strings.Contains(msg.Text, "Access denied") {
		t.Errorf("Expected access denied message, got: %s", msg.Text)
	}
}

func TestHandleLogsNoProvider(t *testing.T) {
	// Setup
	store := user.NewMockStore()
	testUser, _ := user.NewUser(12345, user.RoleAdmin)
	_ = store.Create(context.Background(), testUser)

	authMiddleware := NewAuthMiddleware(store)
	sender := newMockSender()
	handler := NewCommandHandler(sender, authMiddleware)
	// Intentionally not setting logs command sender

	// Create a /logs command update
	update := createLogsCommandUpdate(12345, 67890, "Admin", "User", "admin", "task-123")

	// Execute
	handler.HandleUpdate(context.Background(), update)

	// Verify
	msg := sender.lastMessage()
	if msg == nil {
		t.Fatal("Expected a message to be sent")
	}

	if !strings.Contains(msg.Text, "not available") {
		t.Errorf("Expected 'not available' in message, got: %s", msg.Text)
	}
}

func TestHandleLogsMissingTaskID(t *testing.T) {
	// Setup
	store := user.NewMockStore()
	testUser, _ := user.NewUser(12345, user.RoleAdmin)
	_ = store.Create(context.Background(), testUser)

	authMiddleware := NewAuthMiddleware(store)
	sender := newMockSender()
	handler := NewCommandHandler(sender, authMiddleware)

	// Set up logs command sender (won't be used)
	logsSender := &mockLogsCommandSender{}
	handler.SetLogsCommandSender(logsSender)

	// Create a /logs command update WITHOUT task ID
	update := createLogsCommandUpdate(12345, 67890, "Admin", "User", "admin", "")

	// Execute
	handler.HandleUpdate(context.Background(), update)

	// Verify
	msg := sender.lastMessage()
	if msg == nil {
		t.Fatal("Expected a message to be sent")
	}

	if !strings.Contains(msg.Text, "Usage:") {
		t.Errorf("Expected usage message, got: %s", msg.Text)
	}

	if !strings.Contains(msg.Text, "/logs <task-id>") {
		t.Errorf("Expected usage format in message, got: %s", msg.Text)
	}

	if !strings.Contains(msg.Text, "[project-id]") {
		t.Errorf("Expected project-id hint in message, got: %s", msg.Text)
	}
}

func TestHandleLogsWithError(t *testing.T) {
	// Setup
	store := user.NewMockStore()
	testUser, _ := user.NewUser(12345, user.RoleAdmin)
	_ = store.Create(context.Background(), testUser)

	authMiddleware := NewAuthMiddleware(store)
	sender := newMockSender()
	handler := NewCommandHandler(sender, authMiddleware)

	// Set up logs command sender that returns an error
	logsSender := &mockLogsCommandSender{
		result: nil,
		err:    context.DeadlineExceeded,
	}
	handler.SetLogsCommandSender(logsSender)

	// Create a /logs command update
	update := createLogsCommandUpdate(12345, 67890, "Admin", "User", "admin", "task-123")

	// Execute
	handler.HandleUpdate(context.Background(), update)

	// Verify
	msg := sender.lastMessage()
	if msg == nil {
		t.Fatal("Expected a message to be sent")
	}

	if !strings.Contains(msg.Text, "Error") {
		t.Errorf("Expected error message, got: %s", msg.Text)
	}
}

func TestHandleLogsWithNilFrom(t *testing.T) {
	// Setup
	store := user.NewMockStore()
	authMiddleware := NewAuthMiddleware(store)
	sender := newMockSender()
	handler := NewCommandHandler(sender, authMiddleware)

	// Create a /logs command with nil From field
	update := tgbotapi.Update{
		UpdateID: 1,
		Message: &tgbotapi.Message{
			MessageID: 1,
			Text:      "/logs task-123",
			Chat:      &tgbotapi.Chat{ID: 67890},
			From:      nil, // No user info
			Entities: []tgbotapi.MessageEntity{
				{
					Type:   "bot_command",
					Offset: 0,
					Length: 5,
				},
			},
		},
	}

	// Execute
	handler.HandleUpdate(context.Background(), update)

	// Verify
	msg := sender.lastMessage()
	if msg == nil {
		t.Fatal("Expected a message to be sent")
	}

	if !strings.Contains(msg.Text, "Error") {
		t.Errorf("Expected error message, got: %s", msg.Text)
	}
}

func TestHandleLogsWithProjectID(t *testing.T) {
	// Setup
	store := user.NewMockStore()
	testUser, _ := user.NewUser(12345, user.RoleAdmin)
	_ = store.Create(context.Background(), testUser)

	authMiddleware := NewAuthMiddleware(store)
	sender := newMockSender()
	handler := NewCommandHandler(sender, authMiddleware)

	// Set up logs command sender
	logsSender := &mockLogsCommandSender{
		result: &LogsCommandResult{
			TaskID:    "task-456",
			ProjectID: "target-project",
			Entries: []protocol.LogEntry{
				{
					Timestamp: "2024-01-15T10:30:00Z",
					Level:     protocol.LogLevelInfo,
					Message:   "Log entry",
				},
			},
		},
	}
	handler.SetLogsCommandSender(logsSender)

	// Create a /logs command update with task ID AND project ID
	update := createLogsCommandUpdate(12345, 67890, "Admin", "User", "admin", "task-456 target-project")

	// Execute
	handler.HandleUpdate(context.Background(), update)

	// Verify the logs sender received the correct parameters
	if logsSender.lastTaskID != "task-456" {
		t.Errorf("Expected task ID 'task-456', got '%s'", logsSender.lastTaskID)
	}

	if logsSender.lastProjectID != "target-project" {
		t.Errorf("Expected project ID 'target-project', got '%s'", logsSender.lastProjectID)
	}

	// Verify message content
	msg := sender.lastMessage()
	if msg == nil {
		t.Fatal("Expected a message to be sent")
	}

	if !strings.Contains(msg.Text, "target-project") {
		t.Errorf("Expected project ID in message, got: %s", msg.Text)
	}
}

func TestHandleLogsWithExecutorRole(t *testing.T) {
	// Setup - user with executor role should be allowed to view logs
	store := user.NewMockStore()
	testUser, _ := user.NewUser(12345, user.RoleExecutor)
	_ = store.Create(context.Background(), testUser)

	authMiddleware := NewAuthMiddleware(store)
	sender := newMockSender()
	handler := NewCommandHandler(sender, authMiddleware)

	// Set up logs command sender
	logsSender := &mockLogsCommandSender{
		result: &LogsCommandResult{
			TaskID: "task-123",
			Entries: []protocol.LogEntry{
				{
					Timestamp: "2024-01-15T10:30:00Z",
					Level:     protocol.LogLevelInfo,
					Message:   "Test log",
				},
			},
		},
	}
	handler.SetLogsCommandSender(logsSender)

	// Create a /logs command update
	update := createLogsCommandUpdate(12345, 67890, "Executor", "User", "executor", "task-123")

	// Execute
	handler.HandleUpdate(context.Background(), update)

	// Verify the command was processed (logs sender was called)
	if logsSender.callCount != 1 {
		t.Errorf("Expected logs sender to be called once, got %d calls", logsSender.callCount)
	}

	// Verify last message contains the result, not a permission error
	msg := sender.lastMessage()
	if msg == nil {
		t.Fatal("Expected a message to be sent")
	}

	if strings.Contains(msg.Text, "Permission denied") {
		t.Errorf("Expected logs to succeed for executor role, got: %s", msg.Text)
	}

	if !strings.Contains(msg.Text, "Test log") {
		t.Errorf("Expected log entry in message, got: %s", msg.Text)
	}
}

func TestHandleLogsWithCustomerRoleDenied(t *testing.T) {
	// Setup - user with customer role should be denied
	store := user.NewMockStore()
	testUser, _ := user.NewUser(12345, user.RoleCustomer)
	_ = store.Create(context.Background(), testUser)

	authMiddleware := NewAuthMiddleware(store)
	sender := newMockSender()
	handler := NewCommandHandler(sender, authMiddleware)

	// Set up logs command sender (should NOT be called)
	logsSender := &mockLogsCommandSender{
		result: &LogsCommandResult{
			TaskID:  "task-123",
			Entries: []protocol.LogEntry{},
		},
	}
	handler.SetLogsCommandSender(logsSender)

	// Create a /logs command update
	update := createLogsCommandUpdate(12345, 67890, "Customer", "User", "customer", "task-123")

	// Execute
	handler.HandleUpdate(context.Background(), update)

	// Verify the logs sender was NOT called
	if logsSender.callCount != 0 {
		t.Errorf("Expected logs sender NOT to be called for customer role, got %d calls", logsSender.callCount)
	}

	// Verify permission denied message
	msg := sender.lastMessage()
	if msg == nil {
		t.Fatal("Expected a message to be sent")
	}

	if !strings.Contains(msg.Text, "Permission denied") {
		t.Errorf("Expected 'Permission denied' message, got: %s", msg.Text)
	}

	if !strings.Contains(msg.Text, "executor") || !strings.Contains(msg.Text, "admin") {
		t.Errorf("Expected message to mention required roles, got: %s", msg.Text)
	}
}

func TestHandleLogsWithProviderRoleDenied(t *testing.T) {
	// Setup - user with provider role should be denied
	store := user.NewMockStore()
	testUser, _ := user.NewUser(12345, user.RoleProvider)
	_ = store.Create(context.Background(), testUser)

	authMiddleware := NewAuthMiddleware(store)
	sender := newMockSender()
	handler := NewCommandHandler(sender, authMiddleware)

	// Set up logs command sender (should NOT be called)
	logsSender := &mockLogsCommandSender{
		result: &LogsCommandResult{
			TaskID:  "task-123",
			Entries: []protocol.LogEntry{},
		},
	}
	handler.SetLogsCommandSender(logsSender)

	// Create a /logs command update
	update := createLogsCommandUpdate(12345, 67890, "Provider", "User", "provider", "task-123")

	// Execute
	handler.HandleUpdate(context.Background(), update)

	// Verify the logs sender was NOT called
	if logsSender.callCount != 0 {
		t.Errorf("Expected logs sender NOT to be called for provider role, got %d calls", logsSender.callCount)
	}

	// Verify permission denied message
	msg := sender.lastMessage()
	if msg == nil {
		t.Fatal("Expected a message to be sent")
	}

	if !strings.Contains(msg.Text, "Permission denied") {
		t.Errorf("Expected 'Permission denied' message, got: %s", msg.Text)
	}
}

func TestHandleLogsNoEntries(t *testing.T) {
	// Setup
	store := user.NewMockStore()
	testUser, _ := user.NewUser(12345, user.RoleAdmin)
	_ = store.Create(context.Background(), testUser)

	authMiddleware := NewAuthMiddleware(store)
	sender := newMockSender()
	handler := NewCommandHandler(sender, authMiddleware)

	// Set up logs command sender with no entries
	logsSender := &mockLogsCommandSender{
		result: &LogsCommandResult{
			TaskID:  "task-123",
			Entries: []protocol.LogEntry{},
		},
	}
	handler.SetLogsCommandSender(logsSender)

	// Create a /logs command update
	update := createLogsCommandUpdate(12345, 67890, "Admin", "User", "admin", "task-123")

	// Execute
	handler.HandleUpdate(context.Background(), update)

	// Verify
	msg := sender.lastMessage()
	if msg == nil {
		t.Fatal("Expected a message to be sent")
	}

	if !strings.Contains(msg.Text, "No log entries found") {
		t.Errorf("Expected 'No log entries found' in message, got: %s", msg.Text)
	}
}

func TestHandleLogsWithResultError(t *testing.T) {
	// Setup
	store := user.NewMockStore()
	testUser, _ := user.NewUser(12345, user.RoleAdmin)
	_ = store.Create(context.Background(), testUser)

	authMiddleware := NewAuthMiddleware(store)
	sender := newMockSender()
	handler := NewCommandHandler(sender, authMiddleware)

	// Set up logs command sender with an error in the result
	logsSender := &mockLogsCommandSender{
		result: &LogsCommandResult{
			TaskID: "task-123",
			Error:  "Task not found",
		},
	}
	handler.SetLogsCommandSender(logsSender)

	// Create a /logs command update
	update := createLogsCommandUpdate(12345, 67890, "Admin", "User", "admin", "task-123")

	// Execute
	handler.HandleUpdate(context.Background(), update)

	// Verify
	msg := sender.lastMessage()
	if msg == nil {
		t.Fatal("Expected a message to be sent")
	}

	if !strings.Contains(msg.Text, "Task not found") {
		t.Errorf("Expected error in message, got: %s", msg.Text)
	}
}

func TestHandleLogsWithHasMore(t *testing.T) {
	// Setup
	store := user.NewMockStore()
	testUser, _ := user.NewUser(12345, user.RoleAdmin)
	_ = store.Create(context.Background(), testUser)

	authMiddleware := NewAuthMiddleware(store)
	sender := newMockSender()
	handler := NewCommandHandler(sender, authMiddleware)

	// Set up logs command sender with HasMore flag
	logsSender := &mockLogsCommandSender{
		result: &LogsCommandResult{
			TaskID:  "task-123",
			HasMore: true,
			Entries: []protocol.LogEntry{
				{
					Timestamp: "2024-01-15T10:30:00Z",
					Level:     protocol.LogLevelInfo,
					Message:   "First entry",
				},
			},
		},
	}
	handler.SetLogsCommandSender(logsSender)

	// Create a /logs command update
	update := createLogsCommandUpdate(12345, 67890, "Admin", "User", "admin", "task-123")

	// Execute
	handler.HandleUpdate(context.Background(), update)

	// Verify
	msg := sender.lastMessage()
	if msg == nil {
		t.Fatal("Expected a message to be sent")
	}

	if !strings.Contains(msg.Text, "more entries available") {
		t.Errorf("Expected 'more entries available' in message, got: %s", msg.Text)
	}
}

func TestFormatLogsResult(t *testing.T) {
	tests := []struct {
		name     string
		result   *LogsCommandResult
		contains []string
	}{
		{
			name: "with entries",
			result: &LogsCommandResult{
				TaskID:    "task-123",
				ProjectID: "project-a",
				Entries: []protocol.LogEntry{
					{
						Timestamp: "2024-01-15T10:30:00Z",
						Level:     protocol.LogLevelInfo,
						Message:   "Test message",
					},
				},
			},
			contains: []string{"task-123", "project-a", "1 log entries", "Test message"},
		},
		{
			name: "no entries",
			result: &LogsCommandResult{
				TaskID:  "task-456",
				Entries: []protocol.LogEntry{},
			},
			contains: []string{"task-456", "No log entries found"},
		},
		{
			name: "with error",
			result: &LogsCommandResult{
				TaskID: "task-789",
				Error:  "Something went wrong",
			},
			contains: []string{"task-789", "Error:", "Something went wrong"},
		},
		{
			name: "with has more",
			result: &LogsCommandResult{
				TaskID:  "task-abc",
				HasMore: true,
				Entries: []protocol.LogEntry{
					{
						Timestamp: "2024-01-15T10:30:00Z",
						Level:     protocol.LogLevelInfo,
						Message:   "Entry",
					},
				},
			},
			contains: []string{"task-abc", "more entries available"},
		},
		{
			name: "with different log levels",
			result: &LogsCommandResult{
				TaskID: "task-levels",
				Entries: []protocol.LogEntry{
					{
						Timestamp: "2024-01-15T10:30:00Z",
						Level:     protocol.LogLevelDebug,
						Message:   "Debug message",
					},
					{
						Timestamp: "2024-01-15T10:30:01Z",
						Level:     protocol.LogLevelWarn,
						Message:   "Warning message",
					},
					{
						Timestamp: "2024-01-15T10:30:02Z",
						Level:     protocol.LogLevelError,
						Message:   "Error message",
					},
				},
			},
			contains: []string{"Debug message", "Warning message", "Error message"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatLogsResult(tt.result)
			for _, expected := range tt.contains {
				if !strings.Contains(result, expected) {
					t.Errorf("Expected '%s' in result, got: %s", expected, result)
				}
			}
		})
	}
}

// mockFileSender is a mock implementation of FileSender for testing.
type mockFileSender struct {
	*mockSender
	sentDocuments []documentInfo
	sendDocErr    error
}

type documentInfo struct {
	chatID   int64
	fileName string
	fileData []byte
	caption  string
}

func newMockFileSender() *mockFileSender {
	return &mockFileSender{
		mockSender:    newMockSender(),
		sentDocuments: make([]documentInfo, 0),
	}
}

func (m *mockFileSender) SendDocument(chatID int64, fileName string, fileData []byte, caption string) (tgbotapi.Message, error) {
	if m.sendDocErr != nil {
		return tgbotapi.Message{}, m.sendDocErr
	}
	m.sentDocuments = append(m.sentDocuments, documentInfo{
		chatID:   chatID,
		fileName: fileName,
		fileData: fileData,
		caption:  caption,
	})
	return tgbotapi.Message{}, nil
}

func (m *mockFileSender) lastDocument() *documentInfo {
	if len(m.sentDocuments) == 0 {
		return nil
	}
	return &m.sentDocuments[len(m.sentDocuments)-1]
}

func TestHandleLogsWithLargePayloadSendsFile(t *testing.T) {
	// Setup
	store := user.NewMockStore()
	testUser, _ := user.NewUser(12345, user.RoleAdmin)
	_ = store.Create(context.Background(), testUser)

	authMiddleware := NewAuthMiddleware(store)
	fileSender := newMockFileSender()
	handler := NewCommandHandler(fileSender, authMiddleware)
	handler.SetFileSender(fileSender)

	// Set low threshold on formatter to trigger file mode
	handler.logFormatter.SetFileThreshold(100)

	// Set up logs command sender with large payload
	logsSender := &mockLogsCommandSender{
		result: &LogsCommandResult{
			TaskID:    "task-large",
			ProjectID: "my-project",
			Entries: []protocol.LogEntry{
				{
					Timestamp: "2024-01-15T10:30:00Z",
					Level:     protocol.LogLevelInfo,
					Message:   "This is a long log entry that will exceed our small threshold",
				},
				{
					Timestamp: "2024-01-15T10:30:01Z",
					Level:     protocol.LogLevelWarn,
					Message:   "Another long entry to ensure we go over the threshold limit",
				},
			},
		},
	}
	handler.SetLogsCommandSender(logsSender)

	// Create a /logs command update
	update := createLogsCommandUpdate(12345, 67890, "Admin", "User", "admin", "task-large")

	// Execute
	handler.HandleUpdate(context.Background(), update)

	// Verify a document was sent
	doc := fileSender.lastDocument()
	if doc == nil {
		t.Fatal("Expected a document to be sent")
	}

	if doc.chatID != 67890 {
		t.Errorf("Expected chat ID 67890, got %d", doc.chatID)
	}

	if !strings.Contains(doc.fileName, "task-large") {
		t.Errorf("Expected task ID in filename, got: %s", doc.fileName)
	}

	if !strings.HasSuffix(doc.fileName, ".txt") {
		t.Errorf("Expected .txt extension, got: %s", doc.fileName)
	}

	if len(doc.fileData) == 0 {
		t.Error("Expected non-empty file data")
	}

	// Verify caption mentions it's a file attachment
	if !strings.Contains(doc.caption, "attached as file") {
		t.Errorf("Expected file attachment notice in caption, got: %s", doc.caption)
	}
}

func TestHandleLogsWithSmallPayloadSendsText(t *testing.T) {
	// Setup
	store := user.NewMockStore()
	testUser, _ := user.NewUser(12345, user.RoleAdmin)
	_ = store.Create(context.Background(), testUser)

	authMiddleware := NewAuthMiddleware(store)
	fileSender := newMockFileSender()
	handler := NewCommandHandler(fileSender, authMiddleware)
	handler.SetFileSender(fileSender)

	// Keep default high threshold to stay in text mode
	handler.logFormatter.SetFileThreshold(10000)

	// Set up logs command sender with small payload
	logsSender := &mockLogsCommandSender{
		result: &LogsCommandResult{
			TaskID: "task-small",
			Entries: []protocol.LogEntry{
				{
					Timestamp: "2024-01-15T10:30:00Z",
					Level:     protocol.LogLevelInfo,
					Message:   "Short log",
				},
			},
		},
	}
	handler.SetLogsCommandSender(logsSender)

	// Create a /logs command update
	update := createLogsCommandUpdate(12345, 67890, "Admin", "User", "admin", "task-small")

	// Execute
	handler.HandleUpdate(context.Background(), update)

	// Verify no document was sent
	doc := fileSender.lastDocument()
	if doc != nil {
		t.Error("Expected no document to be sent for small payload")
	}

	// Verify text message was sent
	msg := fileSender.lastMessage()
	if msg == nil {
		t.Fatal("Expected a text message to be sent")
	}

	if !strings.Contains(msg.Text, "Short log") {
		t.Errorf("Expected log content in text, got: %s", msg.Text)
	}
}

func TestHandleLogsWithNoFileSenderFallsBackToText(t *testing.T) {
	// Setup
	store := user.NewMockStore()
	testUser, _ := user.NewUser(12345, user.RoleAdmin)
	_ = store.Create(context.Background(), testUser)

	authMiddleware := NewAuthMiddleware(store)
	sender := newMockSender() // Regular sender without file support
	handler := NewCommandHandler(sender, authMiddleware)
	// Intentionally NOT setting file sender

	// Set low threshold on formatter to trigger file mode
	handler.logFormatter.SetFileThreshold(100)

	// Set up logs command sender with large payload
	logsSender := &mockLogsCommandSender{
		result: &LogsCommandResult{
			TaskID: "task-large",
			Entries: []protocol.LogEntry{
				{
					Timestamp: "2024-01-15T10:30:00Z",
					Level:     protocol.LogLevelInfo,
					Message:   "This is a long log entry that will exceed our small threshold",
				},
				{
					Timestamp: "2024-01-15T10:30:01Z",
					Level:     protocol.LogLevelWarn,
					Message:   "Another long entry to ensure we go over the threshold limit",
				},
			},
		},
	}
	handler.SetLogsCommandSender(logsSender)

	// Create a /logs command update
	update := createLogsCommandUpdate(12345, 67890, "Admin", "User", "admin", "task-large")

	// Execute
	handler.HandleUpdate(context.Background(), update)

	// Verify text message was sent with fallback notice
	msg := sender.lastMessage()
	if msg == nil {
		t.Fatal("Expected a text message to be sent")
	}

	if !strings.Contains(msg.Text, "File attachment not available") {
		t.Errorf("Expected fallback notice in text, got: %s", msg.Text)
	}
}

func TestHandleLogsWithFileSendError(t *testing.T) {
	// Setup
	store := user.NewMockStore()
	testUser, _ := user.NewUser(12345, user.RoleAdmin)
	_ = store.Create(context.Background(), testUser)

	authMiddleware := NewAuthMiddleware(store)
	fileSender := newMockFileSender()
	fileSender.sendDocErr = context.DeadlineExceeded // Simulate file send error
	handler := NewCommandHandler(fileSender, authMiddleware)
	handler.SetFileSender(fileSender)

	// Set low threshold on formatter to trigger file mode
	handler.logFormatter.SetFileThreshold(100)

	// Set up logs command sender with large payload
	logsSender := &mockLogsCommandSender{
		result: &LogsCommandResult{
			TaskID: "task-large",
			Entries: []protocol.LogEntry{
				{
					Timestamp: "2024-01-15T10:30:00Z",
					Level:     protocol.LogLevelInfo,
					Message:   "This is a long log entry that will exceed our small threshold",
				},
				{
					Timestamp: "2024-01-15T10:30:01Z",
					Level:     protocol.LogLevelWarn,
					Message:   "Another long entry to ensure we go over the threshold limit",
				},
			},
		},
	}
	handler.SetLogsCommandSender(logsSender)

	// Create a /logs command update
	update := createLogsCommandUpdate(12345, 67890, "Admin", "User", "admin", "task-large")

	// Execute
	handler.HandleUpdate(context.Background(), update)

	// Verify fallback text message was sent
	msg := fileSender.lastMessage()
	if msg == nil {
		t.Fatal("Expected a fallback text message to be sent")
	}

	if !strings.Contains(msg.Text, "Failed to send log file") {
		t.Errorf("Expected file send error notice in text, got: %s", msg.Text)
	}
}

func TestSetFileSender(t *testing.T) {
	sender := newMockSender()
	store := user.NewMockStore()
	authMiddleware := NewAuthMiddleware(store)
	handler := NewCommandHandler(sender, authMiddleware)

	// Initially no file sender
	if handler.fileSender != nil {
		t.Error("Expected no file sender initially")
	}

	// Set file sender
	fileSender := newMockFileSender()
	handler.SetFileSender(fileSender)

	if handler.fileSender == nil {
		t.Error("Expected file sender to be set")
	}
}

func TestCommandHandlerHasLogFormatter(t *testing.T) {
	sender := newMockSender()
	store := user.NewMockStore()
	authMiddleware := NewAuthMiddleware(store)
	handler := NewCommandHandler(sender, authMiddleware)

	if handler.logFormatter == nil {
		t.Error("Expected log formatter to be initialized")
	}
}

// mockCancelCommandSender is a mock implementation of CancelCommandSender for testing.
type mockCancelCommandSender struct {
	result        *CancelCommandResult
	err           error
	lastTaskID    string
	lastProjectID string
	callCount     int
}

func (m *mockCancelCommandSender) SendCancelCommand(ctx context.Context, taskID, projectID string) (*CancelCommandResult, error) {
	m.lastTaskID = taskID
	m.lastProjectID = projectID
	m.callCount++
	return m.result, m.err
}

// Helper function to create a /cancel command update.
func createCancelCommandUpdate(userID, chatID int64, firstName, lastName, username, args string) tgbotapi.Update {
	text := "/cancel"
	if args != "" {
		text = "/cancel " + args
	}
	return tgbotapi.Update{
		UpdateID: 1,
		Message: &tgbotapi.Message{
			MessageID: 1,
			Text:      text,
			Chat:      &tgbotapi.Chat{ID: chatID},
			From: &tgbotapi.User{
				ID:        userID,
				FirstName: firstName,
				LastName:  lastName,
				UserName:  username,
			},
			Entities: []tgbotapi.MessageEntity{
				{
					Type:   "bot_command",
					Offset: 0,
					Length: 7,
				},
			},
		},
	}
}

func TestHandleCancelAuthorizedUserWithValidTaskID(t *testing.T) {
	// Setup
	store := user.NewMockStore()
	testUser, _ := user.NewUser(12345, user.RoleAdmin)
	_ = store.Create(context.Background(), testUser)

	authMiddleware := NewAuthMiddleware(store)
	sender := newMockSender()
	handler := NewCommandHandler(sender, authMiddleware)

	// Set up cancel command sender
	cancelSender := &mockCancelCommandSender{
		result: &CancelCommandResult{
			TaskID:    "task-123",
			Status:    protocol.CancelStatusCompleted,
			Message:   "Task successfully cancelled",
			ProjectID: "my-project",
		},
	}
	handler.SetCancelCommandSender(cancelSender)

	// Create a /cancel command update with task ID
	update := createCancelCommandUpdate(12345, 67890, "Admin", "User", "admin", "task-123")

	// Execute
	handler.HandleUpdate(context.Background(), update)

	// Verify - should have 2 messages: "Sending cancel request..." and the result
	if len(sender.sentMessages) < 2 {
		t.Fatalf("Expected at least 2 messages (immediate feedback + result), got %d", len(sender.sentMessages))
	}

	// Verify the first message is the immediate feedback
	firstMsg, ok := sender.sentMessages[0].(tgbotapi.MessageConfig)
	if !ok {
		t.Fatal("Expected first message to be MessageConfig")
	}
	if !strings.Contains(firstMsg.Text, "Sending cancel request") {
		t.Errorf("Expected immediate feedback message, got: %s", firstMsg.Text)
	}

	// Verify the final result message
	msg := sender.lastMessage()
	if msg == nil {
		t.Fatal("Expected a final message to be sent")
	}

	if msg.ChatID != 67890 {
		t.Errorf("Expected chat ID 67890, got %d", msg.ChatID)
	}

	// Verify content - task ID should be present
	if !strings.Contains(msg.Text, "task-123") {
		t.Errorf("Expected task ID in message, got: %s", msg.Text)
	}

	// Verify status is shown in uppercase
	if !strings.Contains(msg.Text, "COMPLETED") {
		t.Errorf("Expected COMPLETED status in message, got: %s", msg.Text)
	}

	// Verify project ID is shown
	if !strings.Contains(msg.Text, "my-project") {
		t.Errorf("Expected project ID in message, got: %s", msg.Text)
	}

	// Verify the cancel sender was called with correct arguments
	if cancelSender.lastTaskID != "task-123" {
		t.Errorf("Expected task ID 'task-123', got '%s'", cancelSender.lastTaskID)
	}
}

func TestHandleCancelWithProjectID(t *testing.T) {
	// Setup
	store := user.NewMockStore()
	testUser, _ := user.NewUser(12345, user.RoleAdmin)
	_ = store.Create(context.Background(), testUser)

	authMiddleware := NewAuthMiddleware(store)
	sender := newMockSender()
	handler := NewCommandHandler(sender, authMiddleware)

	// Set up cancel command sender
	cancelSender := &mockCancelCommandSender{
		result: &CancelCommandResult{
			TaskID:    "task-456",
			Status:    protocol.CancelStatusAccepted,
			Message:   "Cancel request accepted",
			ProjectID: "specific-project",
		},
	}
	handler.SetCancelCommandSender(cancelSender)

	// Create a /cancel command with task ID AND project ID
	update := createCancelCommandUpdate(12345, 67890, "Admin", "User", "admin", "task-456 specific-project")

	// Execute
	handler.HandleUpdate(context.Background(), update)

	// Verify the cancel sender was called with both arguments
	if cancelSender.lastTaskID != "task-456" {
		t.Errorf("Expected task ID 'task-456', got '%s'", cancelSender.lastTaskID)
	}
	if cancelSender.lastProjectID != "specific-project" {
		t.Errorf("Expected project ID 'specific-project', got '%s'", cancelSender.lastProjectID)
	}

	// Verify the first message mentions the project
	firstMsg, ok := sender.sentMessages[0].(tgbotapi.MessageConfig)
	if !ok {
		t.Fatal("Expected first message to be MessageConfig")
	}
	if !strings.Contains(firstMsg.Text, "specific-project") {
		t.Errorf("Expected project ID in feedback message, got: %s", firstMsg.Text)
	}
}

func TestHandleCancelUnauthorizedUser(t *testing.T) {
	// Setup - empty store means no authorized users
	store := user.NewMockStore()
	authMiddleware := NewAuthMiddleware(store)
	sender := newMockSender()
	handler := NewCommandHandler(sender, authMiddleware)

	// Create a /cancel command update from unauthorized user
	update := createCancelCommandUpdate(99999, 67890, "Unknown", "User", "unknown", "task-123")

	// Execute
	handler.HandleUpdate(context.Background(), update)

	// Verify only one message (access denied)
	if len(sender.sentMessages) != 1 {
		t.Errorf("Expected exactly 1 message for unauthorized user, got %d", len(sender.sentMessages))
	}

	msg := sender.lastMessage()
	if msg == nil {
		t.Fatal("Expected a message to be sent")
	}

	if !strings.Contains(msg.Text, "Access denied") {
		t.Errorf("Expected access denied message, got: %s", msg.Text)
	}
}

func TestHandleCancelInsufficientRole(t *testing.T) {
	// Setup - user with 'customer' role cannot cancel tasks
	store := user.NewMockStore()
	testUser, _ := user.NewUser(12345, user.RoleCustomer)
	_ = store.Create(context.Background(), testUser)

	authMiddleware := NewAuthMiddleware(store)
	sender := newMockSender()
	handler := NewCommandHandler(sender, authMiddleware)

	// Create a /cancel command update
	update := createCancelCommandUpdate(12345, 67890, "Customer", "User", "customer", "task-123")

	// Execute
	handler.HandleUpdate(context.Background(), update)

	// Verify permission denied message
	msg := sender.lastMessage()
	if msg == nil {
		t.Fatal("Expected a message to be sent")
	}

	if !strings.Contains(msg.Text, "Permission denied") {
		t.Errorf("Expected permission denied message, got: %s", msg.Text)
	}

	if !strings.Contains(msg.Text, "executor") || !strings.Contains(msg.Text, "admin") {
		t.Errorf("Expected role requirements in message, got: %s", msg.Text)
	}
}

func TestHandleCancelNoProvider(t *testing.T) {
	// Setup
	store := user.NewMockStore()
	testUser, _ := user.NewUser(12345, user.RoleAdmin)
	_ = store.Create(context.Background(), testUser)

	authMiddleware := NewAuthMiddleware(store)
	sender := newMockSender()
	handler := NewCommandHandler(sender, authMiddleware)
	// Intentionally NOT setting cancel command sender

	// Create a /cancel command update
	update := createCancelCommandUpdate(12345, 67890, "Admin", "User", "admin", "task-123")

	// Execute
	handler.HandleUpdate(context.Background(), update)

	// Verify "not available" message is sent
	msg := sender.lastMessage()
	if msg == nil {
		t.Fatal("Expected a message to be sent")
	}

	if !strings.Contains(msg.Text, "not available") {
		t.Errorf("Expected 'not available' in message, got: %s", msg.Text)
	}
}

func TestHandleCancelNoTaskID(t *testing.T) {
	// Setup
	store := user.NewMockStore()
	testUser, _ := user.NewUser(12345, user.RoleAdmin)
	_ = store.Create(context.Background(), testUser)

	authMiddleware := NewAuthMiddleware(store)
	sender := newMockSender()
	handler := NewCommandHandler(sender, authMiddleware)
	handler.SetCancelCommandSender(&mockCancelCommandSender{})

	// Create a /cancel command without task ID
	update := createCancelCommandUpdate(12345, 67890, "Admin", "User", "admin", "")

	// Execute
	handler.HandleUpdate(context.Background(), update)

	// Verify usage message is sent
	msg := sender.lastMessage()
	if msg == nil {
		t.Fatal("Expected a message to be sent")
	}

	if !strings.Contains(msg.Text, "Usage:") {
		t.Errorf("Expected usage instructions, got: %s", msg.Text)
	}

	if !strings.Contains(msg.Text, "/cancel") {
		t.Errorf("Expected /cancel in usage message, got: %s", msg.Text)
	}
}

func TestHandleCancelWithError(t *testing.T) {
	// Setup
	store := user.NewMockStore()
	testUser, _ := user.NewUser(12345, user.RoleAdmin)
	_ = store.Create(context.Background(), testUser)

	authMiddleware := NewAuthMiddleware(store)
	sender := newMockSender()
	handler := NewCommandHandler(sender, authMiddleware)

	// Set up cancel command sender that returns an error
	cancelSender := &mockCancelCommandSender{
		result: nil,
		err:    context.DeadlineExceeded,
	}
	handler.SetCancelCommandSender(cancelSender)

	// Create a /cancel command update
	update := createCancelCommandUpdate(12345, 67890, "Admin", "User", "admin", "task-123")

	// Execute
	handler.HandleUpdate(context.Background(), update)

	// Verify error message is sent
	msg := sender.lastMessage()
	if msg == nil {
		t.Fatal("Expected a message to be sent")
	}

	if !strings.Contains(msg.Text, "Error") {
		t.Errorf("Expected error message, got: %s", msg.Text)
	}
}

func TestHandleCancelWithNilFrom(t *testing.T) {
	// Setup
	store := user.NewMockStore()
	authMiddleware := NewAuthMiddleware(store)
	sender := newMockSender()
	handler := NewCommandHandler(sender, authMiddleware)

	// Create a /cancel command with nil From field
	update := tgbotapi.Update{
		UpdateID: 1,
		Message: &tgbotapi.Message{
			MessageID: 1,
			Text:      "/cancel task-123",
			Chat:      &tgbotapi.Chat{ID: 67890},
			From:      nil, // No user info
			Entities: []tgbotapi.MessageEntity{
				{
					Type:   "bot_command",
					Offset: 0,
					Length: 7,
				},
			},
		},
	}

	// Execute
	handler.HandleUpdate(context.Background(), update)

	// Verify error message
	msg := sender.lastMessage()
	if msg == nil {
		t.Fatal("Expected a message to be sent")
	}

	if !strings.Contains(msg.Text, "Error") {
		t.Errorf("Expected error message, got: %s", msg.Text)
	}
}

func TestHandleCancelAllStatuses(t *testing.T) {
	tests := []struct {
		name           string
		status         protocol.CancelStatus
		expectedIcon   string
		expectedStatus string
	}{
		{
			name:           "accepted status",
			status:         protocol.CancelStatusAccepted,
			expectedIcon:   "⏳",
			expectedStatus: "ACCEPTED",
		},
		{
			name:           "completed status",
			status:         protocol.CancelStatusCompleted,
			expectedIcon:   "✅",
			expectedStatus: "COMPLETED",
		},
		{
			name:           "failed status",
			status:         protocol.CancelStatusFailed,
			expectedIcon:   "❌",
			expectedStatus: "FAILED",
		},
		{
			name:           "rejected status",
			status:         protocol.CancelStatusRejected,
			expectedIcon:   "🚫",
			expectedStatus: "REJECTED",
		},
		{
			name:           "not_found status",
			status:         protocol.CancelStatusNotFound,
			expectedIcon:   "❓",
			expectedStatus: "NOT_FOUND",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := user.NewMockStore()
			testUser, _ := user.NewUser(12345, user.RoleAdmin)
			_ = store.Create(context.Background(), testUser)

			authMiddleware := NewAuthMiddleware(store)
			sender := newMockSender()
			handler := NewCommandHandler(sender, authMiddleware)

			cancelSender := &mockCancelCommandSender{
				result: &CancelCommandResult{
					TaskID: "task-status-test",
					Status: tt.status,
				},
			}
			handler.SetCancelCommandSender(cancelSender)

			update := createCancelCommandUpdate(12345, 67890, "Admin", "User", "admin", "task-status-test")
			handler.HandleUpdate(context.Background(), update)

			msg := sender.lastMessage()
			if msg == nil {
				t.Fatal("Expected a message to be sent")
			}

			// Verify correct icon is used
			if !strings.Contains(msg.Text, tt.expectedIcon) {
				t.Errorf("Expected icon '%s' for status %s, got: %s", tt.expectedIcon, tt.status, msg.Text)
			}

			// Verify status text is present
			if !strings.Contains(msg.Text, tt.expectedStatus) {
				t.Errorf("Expected status '%s' in message, got: %s", tt.expectedStatus, msg.Text)
			}
		})
	}
}

func TestFormatCancelResult(t *testing.T) {
	tests := []struct {
		name     string
		result   *CancelCommandResult
		contains []string
	}{
		{
			name: "completed with message",
			result: &CancelCommandResult{
				TaskID:    "task-1",
				Status:    protocol.CancelStatusCompleted,
				Message:   "Task was cancelled successfully",
				ProjectID: "proj-1",
			},
			contains: []string{"✅", "task-1", "COMPLETED", "proj-1", "Task was cancelled successfully"},
		},
		{
			name: "failed with error",
			result: &CancelCommandResult{
				TaskID:  "task-2",
				Status:  protocol.CancelStatusFailed,
				Error:   "Task is not running",
				Message: "",
			},
			contains: []string{"❌", "task-2", "FAILED", "Task is not running"},
		},
		{
			name: "rejected - no clients",
			result: &CancelCommandResult{
				TaskID:    "task-3",
				Status:    protocol.CancelStatusRejected,
				Error:     "no clients connected",
				Message:   "Cannot cancel task: no OpenExec clients are currently connected.",
				ProjectID: "",
			},
			contains: []string{"🚫", "task-3", "REJECTED", "no clients connected"},
		},
		{
			name: "not found",
			result: &CancelCommandResult{
				TaskID:  "task-404",
				Status:  protocol.CancelStatusNotFound,
				Message: "Task was not found in the system",
			},
			contains: []string{"❓", "task-404", "NOT_FOUND"},
		},
		{
			name: "accepted - in progress",
			result: &CancelCommandResult{
				TaskID:    "task-pending",
				Status:    protocol.CancelStatusAccepted,
				Message:   "Cancellation in progress",
				ProjectID: "active-project",
			},
			contains: []string{"⏳", "task-pending", "ACCEPTED", "active-project"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatCancelResult(tt.result)
			for _, expected := range tt.contains {
				if !strings.Contains(result, expected) {
					t.Errorf("Expected '%s' in result, got: %s", expected, result)
				}
			}
		})
	}
}

func TestHandleCancelExecutorRole(t *testing.T) {
	// Setup - executor role should be able to cancel
	store := user.NewMockStore()
	testUser, _ := user.NewUser(12345, user.RoleExecutor)
	_ = store.Create(context.Background(), testUser)

	authMiddleware := NewAuthMiddleware(store)
	sender := newMockSender()
	handler := NewCommandHandler(sender, authMiddleware)

	cancelSender := &mockCancelCommandSender{
		result: &CancelCommandResult{
			TaskID: "task-executor",
			Status: protocol.CancelStatusCompleted,
		},
	}
	handler.SetCancelCommandSender(cancelSender)

	update := createCancelCommandUpdate(12345, 67890, "Executor", "User", "executor", "task-executor")
	handler.HandleUpdate(context.Background(), update)

	// Verify the cancel was processed (2 messages: feedback + result)
	if len(sender.sentMessages) < 2 {
		t.Fatalf("Expected at least 2 messages for executor role, got %d", len(sender.sentMessages))
	}

	// Verify command sender was called
	if cancelSender.callCount != 1 {
		t.Errorf("Expected cancel sender to be called once, got %d", cancelSender.callCount)
	}
}

func TestSetCancelCommandSender(t *testing.T) {
	sender := newMockSender()
	store := user.NewMockStore()
	authMiddleware := NewAuthMiddleware(store)
	handler := NewCommandHandler(sender, authMiddleware)

	// Initially no cancel command sender
	if handler.cancelCommandSender != nil {
		t.Error("Expected no cancel command sender initially")
	}

	// Set cancel command sender
	cancelSender := &mockCancelCommandSender{}
	handler.SetCancelCommandSender(cancelSender)

	if handler.cancelCommandSender == nil {
		t.Error("Expected cancel command sender to be set")
	}
}

// mockCreateTaskCommandSender is a mock implementation of CreateTaskCommandSender for testing.
type mockCreateTaskCommandSender struct {
	result          *CreateTaskCommandResult
	err             error
	lastTitle       string
	lastDescription string
	lastProjectID   string
	callCount       int
}

func (m *mockCreateTaskCommandSender) SendCreateTaskCommand(ctx context.Context, title, description, projectID string) (*CreateTaskCommandResult, error) {
	m.lastTitle = title
	m.lastDescription = description
	m.lastProjectID = projectID
	m.callCount++
	return m.result, m.err
}

// Helper function to create a /create_task command update.
func createCreateTaskCommandUpdate(userID, chatID int64, firstName, lastName, username, args string) tgbotapi.Update {
	text := "/create_task"
	if args != "" {
		text = "/create_task " + args
	}
	return tgbotapi.Update{
		UpdateID: 1,
		Message: &tgbotapi.Message{
			MessageID: 1,
			Text:      text,
			Chat:      &tgbotapi.Chat{ID: chatID},
			From: &tgbotapi.User{
				ID:        userID,
				FirstName: firstName,
				LastName:  lastName,
				UserName:  username,
			},
			Entities: []tgbotapi.MessageEntity{
				{
					Type:   "bot_command",
					Offset: 0,
					Length: 12,
				},
			},
		},
	}
}

func TestHandleCreateTaskAuthorizedUserSuccess(t *testing.T) {
	// Setup
	store := user.NewMockStore()
	testUser, _ := user.NewUser(12345, user.RoleAdmin)
	_ = store.Create(context.Background(), testUser)

	authMiddleware := NewAuthMiddleware(store)
	sender := newMockSender()
	handler := NewCommandHandler(sender, authMiddleware)

	// Set up create task command sender with success result
	createTaskSender := &mockCreateTaskCommandSender{
		result: &CreateTaskCommandResult{
			TaskID:        "task-new-001",
			Status:        protocol.CreateTaskStatusCreated,
			Message:       "Task created successfully",
			ProjectID:     "my-project",
			QueuePosition: 0,
		},
	}
	handler.SetCreateTaskCommandSender(createTaskSender)

	// Create a /create_task command update with title
	update := createCreateTaskCommandUpdate(12345, 67890, "Admin", "User", "admin", "MyNewTask")

	// Execute
	handler.HandleUpdate(context.Background(), update)

	// Verify - should have 2 messages: "Creating..." and the result
	if len(sender.sentMessages) < 2 {
		t.Fatalf("Expected at least 2 messages, got %d", len(sender.sentMessages))
	}

	msg := sender.lastMessage()
	if msg == nil {
		t.Fatal("Expected a message to be sent")
	}

	if msg.ChatID != 67890 {
		t.Errorf("Expected chat ID 67890, got %d", msg.ChatID)
	}

	// Verify content
	if !strings.Contains(msg.Text, "CREATED") {
		t.Errorf("Expected CREATED status in message, got: %s", msg.Text)
	}

	if !strings.Contains(msg.Text, "task-new-001") {
		t.Errorf("Expected task ID 'task-new-001' in message, got: %s", msg.Text)
	}

	// Verify command sender was called
	if createTaskSender.callCount != 1 {
		t.Errorf("Expected create task sender to be called once, got %d", createTaskSender.callCount)
	}

	if createTaskSender.lastTitle != "MyNewTask" {
		t.Errorf("Expected title 'MyNewTask', got '%s'", createTaskSender.lastTitle)
	}
}

func TestHandleCreateTaskUnauthorizedUser(t *testing.T) {
	// Setup - empty store means no authorized users
	store := user.NewMockStore()
	authMiddleware := NewAuthMiddleware(store)
	sender := newMockSender()
	handler := NewCommandHandler(sender, authMiddleware)

	// Create a /create_task command update from unauthorized user
	update := createCreateTaskCommandUpdate(99999, 67890, "Unknown", "User", "unknown", "MyTask")

	// Execute
	handler.HandleUpdate(context.Background(), update)

	// Verify
	msg := sender.lastMessage()
	if msg == nil {
		t.Fatal("Expected a message to be sent")
	}

	if !strings.Contains(msg.Text, "Access denied") {
		t.Errorf("Expected access denied message, got: %s", msg.Text)
	}
}

func TestHandleCreateTaskInsufficientPermissions(t *testing.T) {
	// Setup - user with customer role (cannot execute)
	store := user.NewMockStore()
	testUser, _ := user.NewUser(12345, user.RoleCustomer)
	_ = store.Create(context.Background(), testUser)

	authMiddleware := NewAuthMiddleware(store)
	sender := newMockSender()
	handler := NewCommandHandler(sender, authMiddleware)

	createTaskSender := &mockCreateTaskCommandSender{}
	handler.SetCreateTaskCommandSender(createTaskSender)

	// Create a /create_task command update
	update := createCreateTaskCommandUpdate(12345, 67890, "Customer", "User", "customer", "MyTask")

	// Execute
	handler.HandleUpdate(context.Background(), update)

	// Verify
	msg := sender.lastMessage()
	if msg == nil {
		t.Fatal("Expected a message to be sent")
	}

	if !strings.Contains(msg.Text, "Permission denied") {
		t.Errorf("Expected permission denied message, got: %s", msg.Text)
	}

	// Verify command sender was NOT called
	if createTaskSender.callCount != 0 {
		t.Errorf("Expected create task sender to not be called, got %d calls", createTaskSender.callCount)
	}
}

func TestHandleCreateTaskNoSenderConfigured(t *testing.T) {
	// Setup
	store := user.NewMockStore()
	testUser, _ := user.NewUser(12345, user.RoleAdmin)
	_ = store.Create(context.Background(), testUser)

	authMiddleware := NewAuthMiddleware(store)
	sender := newMockSender()
	handler := NewCommandHandler(sender, authMiddleware)
	// Intentionally not setting create task command sender

	// Create a /create_task command update
	update := createCreateTaskCommandUpdate(12345, 67890, "Admin", "User", "admin", "MyTask")

	// Execute
	handler.HandleUpdate(context.Background(), update)

	// Verify
	msg := sender.lastMessage()
	if msg == nil {
		t.Fatal("Expected a message to be sent")
	}

	if !strings.Contains(msg.Text, "not available") {
		t.Errorf("Expected 'not available' in message, got: %s", msg.Text)
	}
}

func TestHandleCreateTaskNoTitle(t *testing.T) {
	// Setup
	store := user.NewMockStore()
	testUser, _ := user.NewUser(12345, user.RoleAdmin)
	_ = store.Create(context.Background(), testUser)

	authMiddleware := NewAuthMiddleware(store)
	sender := newMockSender()
	handler := NewCommandHandler(sender, authMiddleware)

	createTaskSender := &mockCreateTaskCommandSender{}
	handler.SetCreateTaskCommandSender(createTaskSender)

	// Create a /create_task command update WITHOUT title
	update := createCreateTaskCommandUpdate(12345, 67890, "Admin", "User", "admin", "")

	// Execute
	handler.HandleUpdate(context.Background(), update)

	// Verify
	msg := sender.lastMessage()
	if msg == nil {
		t.Fatal("Expected a message to be sent")
	}

	if !strings.Contains(msg.Text, "Usage:") {
		t.Errorf("Expected usage message, got: %s", msg.Text)
	}

	if !strings.Contains(msg.Text, "/create_task <title>") {
		t.Errorf("Expected usage format in message, got: %s", msg.Text)
	}

	// Verify command sender was NOT called
	if createTaskSender.callCount != 0 {
		t.Errorf("Expected create task sender to not be called, got %d calls", createTaskSender.callCount)
	}
}

func TestHandleCreateTaskWithError(t *testing.T) {
	// Setup
	store := user.NewMockStore()
	testUser, _ := user.NewUser(12345, user.RoleAdmin)
	_ = store.Create(context.Background(), testUser)

	authMiddleware := NewAuthMiddleware(store)
	sender := newMockSender()
	handler := NewCommandHandler(sender, authMiddleware)

	// Set up create task command sender that returns an error
	createTaskSender := &mockCreateTaskCommandSender{
		result: nil,
		err:    context.DeadlineExceeded,
	}
	handler.SetCreateTaskCommandSender(createTaskSender)

	// Create a /create_task command update
	update := createCreateTaskCommandUpdate(12345, 67890, "Admin", "User", "admin", "MyTask")

	// Execute
	handler.HandleUpdate(context.Background(), update)

	// Verify
	msg := sender.lastMessage()
	if msg == nil {
		t.Fatal("Expected a message to be sent")
	}

	if !strings.Contains(msg.Text, "Error") {
		t.Errorf("Expected error message, got: %s", msg.Text)
	}
}

func TestHandleCreateTaskQueued(t *testing.T) {
	// Setup
	store := user.NewMockStore()
	testUser, _ := user.NewUser(12345, user.RoleAdmin)
	_ = store.Create(context.Background(), testUser)

	authMiddleware := NewAuthMiddleware(store)
	sender := newMockSender()
	handler := NewCommandHandler(sender, authMiddleware)

	// Set up create task command sender with queued result
	createTaskSender := &mockCreateTaskCommandSender{
		result: &CreateTaskCommandResult{
			TaskID:        "task-queued-001",
			Status:        protocol.CreateTaskStatusQueued,
			Message:       "Task queued for execution",
			ProjectID:     "my-project",
			QueuePosition: 5,
		},
	}
	handler.SetCreateTaskCommandSender(createTaskSender)

	// Create a /create_task command update
	update := createCreateTaskCommandUpdate(12345, 67890, "Admin", "User", "admin", "MyQueuedTask")

	// Execute
	handler.HandleUpdate(context.Background(), update)

	// Verify
	msg := sender.lastMessage()
	if msg == nil {
		t.Fatal("Expected a message to be sent")
	}

	if !strings.Contains(msg.Text, "QUEUED") {
		t.Errorf("Expected QUEUED status in message, got: %s", msg.Text)
	}

	if !strings.Contains(msg.Text, "Queue Position: 5") {
		t.Errorf("Expected queue position '5' in message, got: %s", msg.Text)
	}
}

func TestHandleCreateTaskRejected(t *testing.T) {
	// Setup
	store := user.NewMockStore()
	testUser, _ := user.NewUser(12345, user.RoleAdmin)
	_ = store.Create(context.Background(), testUser)

	authMiddleware := NewAuthMiddleware(store)
	sender := newMockSender()
	handler := NewCommandHandler(sender, authMiddleware)

	// Set up create task command sender with rejected result
	createTaskSender := &mockCreateTaskCommandSender{
		result: &CreateTaskCommandResult{
			Status:  protocol.CreateTaskStatusRejected,
			Message: "Task was rejected",
			Error:   "No clients available",
		},
	}
	handler.SetCreateTaskCommandSender(createTaskSender)

	// Create a /create_task command update
	update := createCreateTaskCommandUpdate(12345, 67890, "Admin", "User", "admin", "MyTask")

	// Execute
	handler.HandleUpdate(context.Background(), update)

	// Verify
	msg := sender.lastMessage()
	if msg == nil {
		t.Fatal("Expected a message to be sent")
	}

	if !strings.Contains(msg.Text, "REJECTED") {
		t.Errorf("Expected REJECTED status in message, got: %s", msg.Text)
	}
}

func TestHandleCreateTaskWithProjectID(t *testing.T) {
	// Setup
	store := user.NewMockStore()
	testUser, _ := user.NewUser(12345, user.RoleAdmin)
	_ = store.Create(context.Background(), testUser)

	authMiddleware := NewAuthMiddleware(store)
	sender := newMockSender()
	handler := NewCommandHandler(sender, authMiddleware)

	// Set up create task command sender with success result
	createTaskSender := &mockCreateTaskCommandSender{
		result: &CreateTaskCommandResult{
			TaskID:    "task-proj-001",
			Status:    protocol.CreateTaskStatusCreated,
			Message:   "Task created successfully",
			ProjectID: "target-project",
		},
	}
	handler.SetCreateTaskCommandSender(createTaskSender)

	// Create a /create_task command update with title AND project ID
	update := createCreateTaskCommandUpdate(12345, 67890, "Admin", "User", "admin", "MyTask target-project")

	// Execute
	handler.HandleUpdate(context.Background(), update)

	// Verify the create task sender received the correct parameters
	if createTaskSender.lastTitle != "MyTask" {
		t.Errorf("Expected title 'MyTask', got '%s'", createTaskSender.lastTitle)
	}

	if createTaskSender.lastProjectID != "target-project" {
		t.Errorf("Expected project ID 'target-project', got '%s'", createTaskSender.lastProjectID)
	}

	// Verify the result includes project info
	msg := sender.lastMessage()
	if msg == nil {
		t.Fatal("Expected a message to be sent")
	}

	if !strings.Contains(msg.Text, "target-project") {
		t.Errorf("Expected 'target-project' in message, got: %s", msg.Text)
	}
}

func TestHandleCreateTaskWithNilFrom(t *testing.T) {
	// Setup
	store := user.NewMockStore()
	authMiddleware := NewAuthMiddleware(store)
	sender := newMockSender()
	handler := NewCommandHandler(sender, authMiddleware)

	// Create a /create_task command with nil From field
	update := tgbotapi.Update{
		UpdateID: 1,
		Message: &tgbotapi.Message{
			MessageID: 1,
			Text:      "/create_task MyTask",
			Chat:      &tgbotapi.Chat{ID: 67890},
			From:      nil, // No user info
			Entities: []tgbotapi.MessageEntity{
				{
					Type:   "bot_command",
					Offset: 0,
					Length: 12,
				},
			},
		},
	}

	// Execute
	handler.HandleUpdate(context.Background(), update)

	// Verify
	msg := sender.lastMessage()
	if msg == nil {
		t.Fatal("Expected a message to be sent")
	}

	if !strings.Contains(msg.Text, "Error") {
		t.Errorf("Expected error message, got: %s", msg.Text)
	}
}

func TestHandleCreateTaskExecutorRole(t *testing.T) {
	// Setup - user with executor role (can execute)
	store := user.NewMockStore()
	testUser, _ := user.NewUser(12345, user.RoleExecutor) // Executor role can execute
	_ = store.Create(context.Background(), testUser)

	authMiddleware := NewAuthMiddleware(store)
	sender := newMockSender()
	handler := NewCommandHandler(sender, authMiddleware)

	createTaskSender := &mockCreateTaskCommandSender{
		result: &CreateTaskCommandResult{
			TaskID:  "task-exec-001",
			Status:  protocol.CreateTaskStatusCreated,
			Message: "Task created successfully",
		},
	}
	handler.SetCreateTaskCommandSender(createTaskSender)

	// Create a /create_task command update
	update := createCreateTaskCommandUpdate(12345, 67890, "Executor", "User", "executor", "MyTask")

	// Execute
	handler.HandleUpdate(context.Background(), update)

	// Verify command sender was called
	if createTaskSender.callCount != 1 {
		t.Errorf("Expected create task sender to be called once, got %d", createTaskSender.callCount)
	}
}

func TestSetCreateTaskCommandSender(t *testing.T) {
	sender := newMockSender()
	store := user.NewMockStore()
	authMiddleware := NewAuthMiddleware(store)
	handler := NewCommandHandler(sender, authMiddleware)

	// Initially no create task command sender
	if handler.createTaskCommandSender != nil {
		t.Error("Expected no create task command sender initially")
	}

	// Set create task command sender
	createTaskSender := &mockCreateTaskCommandSender{}
	handler.SetCreateTaskCommandSender(createTaskSender)

	if handler.createTaskCommandSender == nil {
		t.Error("Expected create task command sender to be set")
	}
}

func TestFormatCreateTaskResult(t *testing.T) {
	tests := []struct {
		name     string
		result   *CreateTaskCommandResult
		contains []string
	}{
		{
			name: "created status",
			result: &CreateTaskCommandResult{
				TaskID:    "task-001",
				Status:    protocol.CreateTaskStatusCreated,
				Message:   "Task created successfully",
				ProjectID: "project-a",
			},
			contains: []string{"CREATED", "task-001", "project-a", "Task created successfully"},
		},
		{
			name: "queued status with position",
			result: &CreateTaskCommandResult{
				TaskID:        "task-002",
				Status:        protocol.CreateTaskStatusQueued,
				Message:       "Task queued",
				QueuePosition: 3,
			},
			contains: []string{"QUEUED", "task-002", "Queue Position: 3"},
		},
		{
			name: "rejected status",
			result: &CreateTaskCommandResult{
				Status: protocol.CreateTaskStatusRejected,
				Error:  "Validation failed",
			},
			contains: []string{"REJECTED", "Error:", "Validation failed"},
		},
		{
			name: "failed status",
			result: &CreateTaskCommandResult{
				Status:  protocol.CreateTaskStatusFailed,
				Message: "Internal error occurred",
				Error:   "Database connection failed",
			},
			contains: []string{"FAILED", "Internal error occurred", "Database connection failed"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatCreateTaskResult(tt.result)
			for _, expected := range tt.contains {
				if !strings.Contains(result, expected) {
					t.Errorf("Expected '%s' in result, got: %s", expected, result)
				}
			}
		})
	}
}

// mockProjectsProvider is a mock implementation of ProjectsProvider for testing.
type mockProjectsProvider struct {
	projects []ProjectInfo
}

func (m *mockProjectsProvider) GetProjects() []ProjectInfo {
	return m.projects
}

// Helper function to create a /projects command update.
func createProjectsCommandUpdate(userID, chatID int64, firstName, lastName, username string) tgbotapi.Update {
	return tgbotapi.Update{
		UpdateID: 1,
		Message: &tgbotapi.Message{
			MessageID: 1,
			Text:      "/projects",
			Chat:      &tgbotapi.Chat{ID: chatID},
			From: &tgbotapi.User{
				ID:        userID,
				FirstName: firstName,
				LastName:  lastName,
				UserName:  username,
			},
			Entities: []tgbotapi.MessageEntity{
				{
					Type:   "bot_command",
					Offset: 0,
					Length: 9,
				},
			},
		},
	}
}

func TestHandleProjectsAuthorizedUser(t *testing.T) {
	// Setup
	store := user.NewMockStore()
	testUser, _ := user.NewUser(12345, user.RoleAdmin)
	_ = store.Create(context.Background(), testUser)

	authMiddleware := NewAuthMiddleware(store)
	sender := newMockSender()
	handler := NewCommandHandler(sender, authMiddleware)

	// Set up projects provider
	projectsProvider := &mockProjectsProvider{
		projects: []ProjectInfo{
			{
				ProjectID:   "project-alpha",
				ClientCount: 2,
				MachineIDs:  []string{"machine-1", "machine-2"},
			},
			{
				ProjectID:   "project-beta",
				ClientCount: 1,
				MachineIDs:  []string{"machine-3"},
			},
		},
	}
	handler.SetProjectsProvider(projectsProvider)

	// Create a /projects command update
	update := createProjectsCommandUpdate(12345, 67890, "Admin", "User", "admin")

	// Execute
	handler.HandleUpdate(context.Background(), update)

	// Verify
	msg := sender.lastMessage()
	if msg == nil {
		t.Fatal("Expected a message to be sent")
	}

	if msg.ChatID != 67890 {
		t.Errorf("Expected chat ID 67890, got %d", msg.ChatID)
	}

	if !strings.Contains(msg.Text, "project-alpha") {
		t.Errorf("Expected project-alpha in message, got: %s", msg.Text)
	}

	if !strings.Contains(msg.Text, "project-beta") {
		t.Errorf("Expected project-beta in message, got: %s", msg.Text)
	}

	if !strings.Contains(msg.Text, "machine-1") {
		t.Errorf("Expected machine-1 in message, got: %s", msg.Text)
	}

	if !strings.Contains(msg.Text, "Connected Projects: 2") {
		t.Errorf("Expected 'Connected Projects: 2' in message, got: %s", msg.Text)
	}
}

func TestHandleProjectsUnauthorizedUser(t *testing.T) {
	// Setup - empty store means no authorized users
	store := user.NewMockStore()
	authMiddleware := NewAuthMiddleware(store)
	sender := newMockSender()
	handler := NewCommandHandler(sender, authMiddleware)

	// Create a /projects command update from unauthorized user
	update := createProjectsCommandUpdate(99999, 67890, "Unknown", "User", "unknown")

	// Execute
	handler.HandleUpdate(context.Background(), update)

	// Verify
	msg := sender.lastMessage()
	if msg == nil {
		t.Fatal("Expected a message to be sent")
	}

	if !strings.Contains(msg.Text, "Access denied") {
		t.Errorf("Expected access denied message, got: %s", msg.Text)
	}

	if !strings.Contains(msg.Text, "not authorized") {
		t.Errorf("Expected 'not authorized' in message, got: %s", msg.Text)
	}
}

func TestHandleProjectsNoProvider(t *testing.T) {
	// Setup
	store := user.NewMockStore()
	testUser, _ := user.NewUser(12345, user.RoleAdmin)
	_ = store.Create(context.Background(), testUser)

	authMiddleware := NewAuthMiddleware(store)
	sender := newMockSender()
	handler := NewCommandHandler(sender, authMiddleware)
	// Intentionally not setting projects provider

	// Create a /projects command update
	update := createProjectsCommandUpdate(12345, 67890, "Admin", "User", "admin")

	// Execute
	handler.HandleUpdate(context.Background(), update)

	// Verify
	msg := sender.lastMessage()
	if msg == nil {
		t.Fatal("Expected a message to be sent")
	}

	if !strings.Contains(msg.Text, "not available") {
		t.Errorf("Expected 'not available' in message, got: %s", msg.Text)
	}
}

func TestHandleProjectsEmptyList(t *testing.T) {
	// Setup
	store := user.NewMockStore()
	testUser, _ := user.NewUser(12345, user.RoleAdmin)
	_ = store.Create(context.Background(), testUser)

	authMiddleware := NewAuthMiddleware(store)
	sender := newMockSender()
	handler := NewCommandHandler(sender, authMiddleware)

	// Set up projects provider with empty list
	projectsProvider := &mockProjectsProvider{
		projects: []ProjectInfo{},
	}
	handler.SetProjectsProvider(projectsProvider)

	// Create a /projects command update
	update := createProjectsCommandUpdate(12345, 67890, "Admin", "User", "admin")

	// Execute
	handler.HandleUpdate(context.Background(), update)

	// Verify
	msg := sender.lastMessage()
	if msg == nil {
		t.Fatal("Expected a message to be sent")
	}

	if !strings.Contains(msg.Text, "No projects connected") {
		t.Errorf("Expected 'No projects connected' in message, got: %s", msg.Text)
	}
}

func TestHandleProjectsWithNilFrom(t *testing.T) {
	store := user.NewMockStore()
	authMiddleware := NewAuthMiddleware(store)
	sender := newMockSender()
	handler := NewCommandHandler(sender, authMiddleware)

	// Create a /projects command with nil From field
	update := tgbotapi.Update{
		UpdateID: 1,
		Message: &tgbotapi.Message{
			MessageID: 1,
			Text:      "/projects",
			Chat:      &tgbotapi.Chat{ID: 67890},
			From:      nil,
			Entities: []tgbotapi.MessageEntity{
				{
					Type:   "bot_command",
					Offset: 0,
					Length: 9,
				},
			},
		},
	}

	handler.HandleUpdate(context.Background(), update)

	msg := sender.lastMessage()
	if msg == nil {
		t.Fatal("Expected a message to be sent")
	}

	if !strings.Contains(msg.Text, "Error") {
		t.Errorf("Expected error message, got: %s", msg.Text)
	}
}

func TestFormatProjectsList(t *testing.T) {
	tests := []struct {
		name     string
		projects []ProjectInfo
		contains []string
	}{
		{
			name:     "empty list",
			projects: []ProjectInfo{},
			contains: []string{"No projects connected"},
		},
		{
			name: "single project",
			projects: []ProjectInfo{
				{
					ProjectID:   "project-one",
					ClientCount: 1,
					MachineIDs:  []string{"machine-a"},
				},
			},
			contains: []string{"Connected Projects: 1", "project-one", "Connections: 1", "machine-a"},
		},
		{
			name: "multiple projects",
			projects: []ProjectInfo{
				{
					ProjectID:   "project-x",
					ClientCount: 3,
					MachineIDs:  []string{"m1", "m2"},
				},
				{
					ProjectID:   "project-y",
					ClientCount: 1,
					MachineIDs:  []string{},
				},
			},
			contains: []string{"Connected Projects: 2", "project-x", "project-y", "Connections: 3", "m1", "m2"},
		},
		{
			name: "project without machines",
			projects: []ProjectInfo{
				{
					ProjectID:   "orphan-project",
					ClientCount: 2,
					MachineIDs:  []string{},
				},
			},
			contains: []string{"orphan-project", "Connections: 2"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatProjectsList(tt.projects)
			for _, expected := range tt.contains {
				if !strings.Contains(result, expected) {
					t.Errorf("Expected '%s' in result, got: %s", expected, result)
				}
			}
		})
	}
}

func TestSetProjectsProvider(t *testing.T) {
	store := user.NewMockStore()
	authMiddleware := NewAuthMiddleware(store)
	sender := newMockSender()
	handler := NewCommandHandler(sender, authMiddleware)

	if handler.projectsProvider != nil {
		t.Error("Expected no projects provider initially")
	}

	// Set projects provider
	projectsProvider := &mockProjectsProvider{}
	handler.SetProjectsProvider(projectsProvider)

	if handler.projectsProvider == nil {
		t.Error("Expected projects provider to be set")
	}
}

// Helper function to create a /switch command update.
func createSwitchCommandUpdate(userID, chatID int64, firstName, lastName, username, args string) tgbotapi.Update {
	text := "/switch"
	commandLen := 7
	if args != "" {
		text = "/switch " + args
	}
	return tgbotapi.Update{
		UpdateID: 1,
		Message: &tgbotapi.Message{
			MessageID: 1,
			Text:      text,
			Chat:      &tgbotapi.Chat{ID: chatID},
			From: &tgbotapi.User{
				ID:        userID,
				FirstName: firstName,
				LastName:  lastName,
				UserName:  username,
			},
			Entities: []tgbotapi.MessageEntity{
				{
					Type:   "bot_command",
					Offset: 0,
					Length: commandLen,
				},
			},
		},
	}
}

func TestHandleSwitchAuthorizedUserWithProject(t *testing.T) {
	// Setup
	store := user.NewMockStore()
	testUser, _ := user.NewUser(12345, user.RoleAdmin)
	_ = store.Create(context.Background(), testUser)

	authMiddleware := NewAuthMiddleware(store)
	sender := newMockSender()
	handler := NewCommandHandler(sender, authMiddleware)

	// Set up user store and projects provider
	handler.SetUserStore(store)
	projectsProvider := &mockProjectsProvider{
		projects: []ProjectInfo{
			{ProjectID: "project-alpha", ClientCount: 1},
			{ProjectID: "project-beta", ClientCount: 1},
		},
	}
	handler.SetProjectsProvider(projectsProvider)

	// Create a /switch command update
	update := createSwitchCommandUpdate(12345, 67890, "Admin", "User", "admin", "project-alpha")

	// Execute
	handler.HandleUpdate(context.Background(), update)

	// Verify
	msg := sender.lastMessage()
	if msg == nil {
		t.Fatal("Expected a message to be sent")
	}

	if msg.ChatID != 67890 {
		t.Errorf("Expected chat ID 67890, got %d", msg.ChatID)
	}

	if !strings.Contains(msg.Text, "Switched to project") {
		t.Errorf("Expected 'Switched to project' in message, got: %s", msg.Text)
	}

	if !strings.Contains(msg.Text, "project-alpha") {
		t.Errorf("Expected 'project-alpha' in message, got: %s", msg.Text)
	}

	// Verify user's current project was updated
	updatedUser, _ := store.GetByTelegramID(context.Background(), 12345)
	if updatedUser.CurrentProjectID != "project-alpha" {
		t.Errorf("Expected user's current project to be 'project-alpha', got: %s", updatedUser.CurrentProjectID)
	}
}

func TestHandleSwitchClearProject(t *testing.T) {
	// Setup
	store := user.NewMockStore()
	testUser, _ := user.NewUser(12345, user.RoleAdmin)
	testUser.SetCurrentProject("project-alpha")
	_ = store.Create(context.Background(), testUser)

	authMiddleware := NewAuthMiddleware(store)
	sender := newMockSender()
	handler := NewCommandHandler(sender, authMiddleware)

	// Set up user store
	handler.SetUserStore(store)

	// Create a /switch command update without arguments (to clear)
	update := createSwitchCommandUpdate(12345, 67890, "Admin", "User", "admin", "")

	// Execute
	handler.HandleUpdate(context.Background(), update)

	// Verify
	msg := sender.lastMessage()
	if msg == nil {
		t.Fatal("Expected a message to be sent")
	}

	if !strings.Contains(msg.Text, "cleared") {
		t.Errorf("Expected 'cleared' in message, got: %s", msg.Text)
	}

	// Verify user's current project was cleared
	updatedUser, _ := store.GetByTelegramID(context.Background(), 12345)
	if updatedUser.CurrentProjectID != "" {
		t.Errorf("Expected user's current project to be empty, got: %s", updatedUser.CurrentProjectID)
	}
}

func TestHandleSwitchUnauthorizedUser(t *testing.T) {
	// Setup - empty store means no authorized users
	store := user.NewMockStore()
	authMiddleware := NewAuthMiddleware(store)
	sender := newMockSender()
	handler := NewCommandHandler(sender, authMiddleware)

	// Create a /switch command update from unauthorized user
	update := createSwitchCommandUpdate(99999, 67890, "Unknown", "User", "unknown", "project-alpha")

	// Execute
	handler.HandleUpdate(context.Background(), update)

	// Verify
	msg := sender.lastMessage()
	if msg == nil {
		t.Fatal("Expected a message to be sent")
	}

	if !strings.Contains(msg.Text, "Access denied") {
		t.Errorf("Expected access denied message, got: %s", msg.Text)
	}

	if !strings.Contains(msg.Text, "not authorized") {
		t.Errorf("Expected 'not authorized' in message, got: %s", msg.Text)
	}
}

func TestHandleSwitchNoUserStore(t *testing.T) {
	// Setup
	store := user.NewMockStore()
	testUser, _ := user.NewUser(12345, user.RoleAdmin)
	_ = store.Create(context.Background(), testUser)

	authMiddleware := NewAuthMiddleware(store)
	sender := newMockSender()
	handler := NewCommandHandler(sender, authMiddleware)
	// Intentionally not setting user store

	// Create a /switch command update
	update := createSwitchCommandUpdate(12345, 67890, "Admin", "User", "admin", "project-alpha")

	// Execute
	handler.HandleUpdate(context.Background(), update)

	// Verify
	msg := sender.lastMessage()
	if msg == nil {
		t.Fatal("Expected a message to be sent")
	}

	if !strings.Contains(msg.Text, "not available") {
		t.Errorf("Expected 'not available' in message, got: %s", msg.Text)
	}
}

func TestHandleSwitchProjectNotFound(t *testing.T) {
	// Setup
	store := user.NewMockStore()
	testUser, _ := user.NewUser(12345, user.RoleAdmin)
	_ = store.Create(context.Background(), testUser)

	authMiddleware := NewAuthMiddleware(store)
	sender := newMockSender()
	handler := NewCommandHandler(sender, authMiddleware)

	// Set up user store and projects provider with different projects
	handler.SetUserStore(store)
	projectsProvider := &mockProjectsProvider{
		projects: []ProjectInfo{
			{ProjectID: "project-alpha", ClientCount: 1},
		},
	}
	handler.SetProjectsProvider(projectsProvider)

	// Create a /switch command update for non-existent project
	update := createSwitchCommandUpdate(12345, 67890, "Admin", "User", "admin", "project-nonexistent")

	// Execute
	handler.HandleUpdate(context.Background(), update)

	// Verify
	msg := sender.lastMessage()
	if msg == nil {
		t.Fatal("Expected a message to be sent")
	}

	if !strings.Contains(msg.Text, "not found") {
		t.Errorf("Expected 'not found' in message, got: %s", msg.Text)
	}

	if !strings.Contains(msg.Text, "project-nonexistent") {
		t.Errorf("Expected project name in message, got: %s", msg.Text)
	}
}

func TestHandleSwitchWithNilFrom(t *testing.T) {
	store := user.NewMockStore()
	authMiddleware := NewAuthMiddleware(store)
	sender := newMockSender()
	handler := NewCommandHandler(sender, authMiddleware)

	// Create a /switch command with nil From field
	update := tgbotapi.Update{
		UpdateID: 1,
		Message: &tgbotapi.Message{
			MessageID: 1,
			Text:      "/switch project-alpha",
			Chat:      &tgbotapi.Chat{ID: 67890},
			From:      nil,
			Entities: []tgbotapi.MessageEntity{
				{
					Type:   "bot_command",
					Offset: 0,
					Length: 7,
				},
			},
		},
	}

	handler.HandleUpdate(context.Background(), update)

	msg := sender.lastMessage()
	if msg == nil {
		t.Fatal("Expected a message to be sent")
	}

	if !strings.Contains(msg.Text, "Error") {
		t.Errorf("Expected error message, got: %s", msg.Text)
	}
}

func TestHandleSwitchWithoutProjectsProvider(t *testing.T) {
	// Setup - switch should work even without projects provider (no validation)
	store := user.NewMockStore()
	testUser, _ := user.NewUser(12345, user.RoleAdmin)
	_ = store.Create(context.Background(), testUser)

	authMiddleware := NewAuthMiddleware(store)
	sender := newMockSender()
	handler := NewCommandHandler(sender, authMiddleware)

	// Set up user store but not projects provider
	handler.SetUserStore(store)

	// Create a /switch command update
	update := createSwitchCommandUpdate(12345, 67890, "Admin", "User", "admin", "any-project")

	// Execute
	handler.HandleUpdate(context.Background(), update)

	// Verify - should succeed without validation
	msg := sender.lastMessage()
	if msg == nil {
		t.Fatal("Expected a message to be sent")
	}

	if !strings.Contains(msg.Text, "Switched to project") {
		t.Errorf("Expected 'Switched to project' in message, got: %s", msg.Text)
	}

	// Verify user's current project was updated
	updatedUser, _ := store.GetByTelegramID(context.Background(), 12345)
	if updatedUser.CurrentProjectID != "any-project" {
		t.Errorf("Expected user's current project to be 'any-project', got: %s", updatedUser.CurrentProjectID)
	}
}

func TestSetUserStore(t *testing.T) {
	store := user.NewMockStore()
	authMiddleware := NewAuthMiddleware(store)
	sender := newMockSender()
	handler := NewCommandHandler(sender, authMiddleware)

	if handler.userStore != nil {
		t.Error("Expected no user store initially")
	}

	// Set user store
	handler.SetUserStore(store)

	if handler.userStore == nil {
		t.Error("Expected user store to be set")
	}
}

// mockDeployCommandSender is a mock implementation of DeployCommandSender for testing.
type mockDeployCommandSender struct {
	result        *DeployCommandResult
	err           error
	lastProjectID string
	lastEnv       string
	callCount     int
}

func (m *mockDeployCommandSender) SendDeployCommand(ctx context.Context, projectID, environment string) (*DeployCommandResult, error) {
	m.lastProjectID = projectID
	m.lastEnv = environment
	m.callCount++
	return m.result, m.err
}

// Helper function to create a /deploy command update.
func createDeployCommandUpdate(userID, chatID int64, firstName, lastName, username, args string) tgbotapi.Update {
	text := "/deploy"
	if args != "" {
		text = "/deploy " + args
	}
	return tgbotapi.Update{
		UpdateID: 1,
		Message: &tgbotapi.Message{
			MessageID: 1,
			Text:      text,
			Chat:      &tgbotapi.Chat{ID: chatID},
			From: &tgbotapi.User{
				ID:        userID,
				FirstName: firstName,
				LastName:  lastName,
				UserName:  username,
			},
			Entities: []tgbotapi.MessageEntity{
				{
					Type:   "bot_command",
					Offset: 0,
					Length: 7,
				},
			},
		},
	}
}

func TestHandleDeployAuthorizedUserWithProjectID(t *testing.T) {
	// Setup
	store := user.NewMockStore()
	testUser, _ := user.NewUser(12345, user.RoleAdmin)
	_ = store.Create(context.Background(), testUser)

	authMiddleware := NewAuthMiddleware(store)
	sender := newMockSender()
	handler := NewCommandHandler(sender, authMiddleware)

	// Set up deploy command sender
	deploySender := &mockDeployCommandSender{
		result: &DeployCommandResult{
			ProjectID:   "my-project",
			Status:      protocol.DeployStatusAccepted,
			Message:     "Deployment started",
			Environment: "production",
		},
	}
	handler.SetDeployCommandSender(deploySender)

	// Create a /deploy command update with project ID
	update := createDeployCommandUpdate(12345, 67890, "Admin", "User", "admin", "my-project")

	// Execute
	handler.HandleUpdate(context.Background(), update)

	// Verify - should have 2 messages: "Deploying..." and the result
	if len(sender.sentMessages) < 2 {
		t.Fatalf("Expected at least 2 messages, got %d", len(sender.sentMessages))
	}

	msg := sender.lastMessage()
	if msg == nil {
		t.Fatal("Expected a message to be sent")
	}

	if msg.ChatID != 67890 {
		t.Errorf("Expected chat ID 67890, got %d", msg.ChatID)
	}

	if !strings.Contains(msg.Text, "ACCEPTED") {
		t.Errorf("Expected 'ACCEPTED' in message, got: %s", msg.Text)
	}

	if !strings.Contains(msg.Text, "my-project") {
		t.Errorf("Expected project ID in message, got: %s", msg.Text)
	}

	// Verify the sender received correct arguments
	if deploySender.lastProjectID != "my-project" {
		t.Errorf("Expected project ID 'my-project', got %s", deploySender.lastProjectID)
	}
}

func TestHandleDeployWithProjectAndEnvironment(t *testing.T) {
	// Setup
	store := user.NewMockStore()
	testUser, _ := user.NewUser(12345, user.RoleAdmin)
	_ = store.Create(context.Background(), testUser)

	authMiddleware := NewAuthMiddleware(store)
	sender := newMockSender()
	handler := NewCommandHandler(sender, authMiddleware)

	// Set up deploy command sender
	deploySender := &mockDeployCommandSender{
		result: &DeployCommandResult{
			ProjectID:   "my-project",
			Status:      protocol.DeployStatusRunning,
			Message:     "Deployment in progress",
			Environment: "staging",
		},
	}
	handler.SetDeployCommandSender(deploySender)

	// Create a /deploy command update with project ID and environment
	update := createDeployCommandUpdate(12345, 67890, "Admin", "User", "admin", "my-project staging")

	// Execute
	handler.HandleUpdate(context.Background(), update)

	// Verify the sender received correct arguments
	if deploySender.lastProjectID != "my-project" {
		t.Errorf("Expected project ID 'my-project', got %s", deploySender.lastProjectID)
	}
	if deploySender.lastEnv != "staging" {
		t.Errorf("Expected environment 'staging', got %s", deploySender.lastEnv)
	}

	msg := sender.lastMessage()
	if msg == nil {
		t.Fatal("Expected a message to be sent")
	}

	if !strings.Contains(msg.Text, "RUNNING") {
		t.Errorf("Expected 'RUNNING' in message, got: %s", msg.Text)
	}
}

func TestHandleDeployWithEnvironmentOnly(t *testing.T) {
	// Setup
	store := user.NewMockStore()
	testUser, _ := user.NewUser(12345, user.RoleAdmin)
	_ = store.Create(context.Background(), testUser)

	authMiddleware := NewAuthMiddleware(store)
	sender := newMockSender()
	handler := NewCommandHandler(sender, authMiddleware)

	// Set up deploy command sender
	deploySender := &mockDeployCommandSender{
		result: &DeployCommandResult{
			ProjectID:   "",
			Status:      protocol.DeployStatusAccepted,
			Message:     "Deployment started",
			Environment: "production",
		},
	}
	handler.SetDeployCommandSender(deploySender)

	// Create a /deploy command update with environment only (recognized as env name)
	update := createDeployCommandUpdate(12345, 67890, "Admin", "User", "admin", "production")

	// Execute
	handler.HandleUpdate(context.Background(), update)

	// Verify the sender received correct arguments
	// "production" should be recognized as an environment name
	if deploySender.lastEnv != "production" {
		t.Errorf("Expected environment 'production', got %s", deploySender.lastEnv)
	}
}

func TestHandleDeployUnauthorizedUser(t *testing.T) {
	// Setup - empty store means no authorized users
	store := user.NewMockStore()
	authMiddleware := NewAuthMiddleware(store)
	sender := newMockSender()
	handler := NewCommandHandler(sender, authMiddleware)

	// Create a /deploy command update from unauthorized user
	update := createDeployCommandUpdate(99999, 67890, "Unknown", "User", "unknown", "my-project")

	// Execute
	handler.HandleUpdate(context.Background(), update)

	// Verify
	msg := sender.lastMessage()
	if msg == nil {
		t.Fatal("Expected a message to be sent")
	}

	if !strings.Contains(msg.Text, "Access denied") {
		t.Errorf("Expected access denied message, got: %s", msg.Text)
	}

	if !strings.Contains(msg.Text, "not authorized") {
		t.Errorf("Expected 'not authorized' in message, got: %s", msg.Text)
	}
}

func TestHandleDeployNoSenderConfigured(t *testing.T) {
	// Setup
	store := user.NewMockStore()
	testUser, _ := user.NewUser(12345, user.RoleAdmin)
	_ = store.Create(context.Background(), testUser)

	authMiddleware := NewAuthMiddleware(store)
	sender := newMockSender()
	handler := NewCommandHandler(sender, authMiddleware)
	// Intentionally not setting deploy command sender

	// Create a /deploy command update
	update := createDeployCommandUpdate(12345, 67890, "Admin", "User", "admin", "my-project")

	// Execute
	handler.HandleUpdate(context.Background(), update)

	// Verify
	msg := sender.lastMessage()
	if msg == nil {
		t.Fatal("Expected a message to be sent")
	}

	if !strings.Contains(msg.Text, "not available") {
		t.Errorf("Expected 'not available' in message, got: %s", msg.Text)
	}
}

func TestHandleDeployPermissionDeniedForCustomer(t *testing.T) {
	// Setup - customer role cannot execute deploy
	store := user.NewMockStore()
	testUser, _ := user.NewUser(12345, user.RoleCustomer)
	_ = store.Create(context.Background(), testUser)

	authMiddleware := NewAuthMiddleware(store)
	sender := newMockSender()
	handler := NewCommandHandler(sender, authMiddleware)

	deploySender := &mockDeployCommandSender{
		result: &DeployCommandResult{
			ProjectID: "my-project",
			Status:    protocol.DeployStatusAccepted,
		},
	}
	handler.SetDeployCommandSender(deploySender)

	// Create a /deploy command update
	update := createDeployCommandUpdate(12345, 67890, "Customer", "User", "customer", "my-project")

	// Execute
	handler.HandleUpdate(context.Background(), update)

	// Verify - customer should be denied
	msg := sender.lastMessage()
	if msg == nil {
		t.Fatal("Expected a message to be sent")
	}

	if !strings.Contains(msg.Text, "Permission denied") {
		t.Errorf("Expected 'Permission denied' in message, got: %s", msg.Text)
	}

	if !strings.Contains(msg.Text, "admin") {
		t.Errorf("Expected 'admin' role requirement in message, got: %s", msg.Text)
	}
}

func TestHandleDeployExecutorRoleDenied(t *testing.T) {
	// Setup - executor role cannot execute deploy (admin only)
	store := user.NewMockStore()
	testUser, _ := user.NewUser(12345, user.RoleExecutor)
	_ = store.Create(context.Background(), testUser)

	authMiddleware := NewAuthMiddleware(store)
	sender := newMockSender()
	handler := NewCommandHandler(sender, authMiddleware)

	deploySender := &mockDeployCommandSender{
		result: &DeployCommandResult{
			ProjectID:   "my-project",
			Status:      protocol.DeployStatusCompleted,
			Message:     "Deployment completed",
			Environment: "production",
			Version:     "v1.2.3",
		},
	}
	handler.SetDeployCommandSender(deploySender)

	// Create a /deploy command update
	update := createDeployCommandUpdate(12345, 67890, "Executor", "User", "executor", "my-project")

	// Execute
	handler.HandleUpdate(context.Background(), update)

	// Verify - executor should be denied (deploy requires admin only)
	msg := sender.lastMessage()
	if msg == nil {
		t.Fatal("Expected a message to be sent")
	}

	if !strings.Contains(msg.Text, "Permission denied") {
		t.Errorf("Expected 'Permission denied' in message, got: %s", msg.Text)
	}

	if !strings.Contains(msg.Text, "admin") {
		t.Errorf("Expected 'admin' role requirement in message, got: %s", msg.Text)
	}

	// Verify deploy command sender was NOT called
	if deploySender.callCount != 0 {
		t.Errorf("Expected deploy command not to be called for executor role, got %d calls", deploySender.callCount)
	}
}

func TestHandleDeployProviderRoleDenied(t *testing.T) {
	// Setup - provider role cannot execute deploy (admin only)
	store := user.NewMockStore()
	testUser, _ := user.NewUser(12345, user.RoleProvider)
	_ = store.Create(context.Background(), testUser)

	authMiddleware := NewAuthMiddleware(store)
	sender := newMockSender()
	handler := NewCommandHandler(sender, authMiddleware)

	deploySender := &mockDeployCommandSender{
		result: &DeployCommandResult{
			ProjectID: "my-project",
			Status:    protocol.DeployStatusAccepted,
		},
	}
	handler.SetDeployCommandSender(deploySender)

	// Create a /deploy command update
	update := createDeployCommandUpdate(12345, 67890, "Provider", "User", "provider", "my-project")

	// Execute
	handler.HandleUpdate(context.Background(), update)

	// Verify - provider should be denied (deploy requires admin only)
	msg := sender.lastMessage()
	if msg == nil {
		t.Fatal("Expected a message to be sent")
	}

	if !strings.Contains(msg.Text, "Permission denied") {
		t.Errorf("Expected 'Permission denied' in message, got: %s", msg.Text)
	}

	if !strings.Contains(msg.Text, "admin") {
		t.Errorf("Expected 'admin' role requirement in message, got: %s", msg.Text)
	}

	// Verify deploy command sender was NOT called
	if deploySender.callCount != 0 {
		t.Errorf("Expected deploy command not to be called for provider role, got %d calls", deploySender.callCount)
	}
}

func TestHandleDeployWithError(t *testing.T) {
	// Setup
	store := user.NewMockStore()
	testUser, _ := user.NewUser(12345, user.RoleAdmin)
	_ = store.Create(context.Background(), testUser)

	authMiddleware := NewAuthMiddleware(store)
	sender := newMockSender()
	handler := NewCommandHandler(sender, authMiddleware)

	deploySender := &mockDeployCommandSender{
		result: &DeployCommandResult{
			ProjectID: "my-project",
			Status:    protocol.DeployStatusFailed,
			Error:     "Build failed: compilation error",
		},
	}
	handler.SetDeployCommandSender(deploySender)

	// Create a /deploy command update
	update := createDeployCommandUpdate(12345, 67890, "Admin", "User", "admin", "my-project")

	// Execute
	handler.HandleUpdate(context.Background(), update)

	// Verify
	msg := sender.lastMessage()
	if msg == nil {
		t.Fatal("Expected a message to be sent")
	}

	if !strings.Contains(msg.Text, "FAILED") {
		t.Errorf("Expected 'FAILED' in message, got: %s", msg.Text)
	}

	if !strings.Contains(msg.Text, "Build failed") {
		t.Errorf("Expected error message in response, got: %s", msg.Text)
	}
}

func TestHandleDeployNoArguments(t *testing.T) {
	// Setup
	store := user.NewMockStore()
	testUser, _ := user.NewUser(12345, user.RoleAdmin)
	_ = store.Create(context.Background(), testUser)

	authMiddleware := NewAuthMiddleware(store)
	sender := newMockSender()
	handler := NewCommandHandler(sender, authMiddleware)

	deploySender := &mockDeployCommandSender{
		result: &DeployCommandResult{
			ProjectID: "",
			Status:    protocol.DeployStatusAccepted,
			Message:   "Deployment started",
		},
	}
	handler.SetDeployCommandSender(deploySender)

	// Create a /deploy command update without arguments
	update := createDeployCommandUpdate(12345, 67890, "Admin", "User", "admin", "")

	// Execute
	handler.HandleUpdate(context.Background(), update)

	// Verify - should proceed with empty project ID (auto-resolve)
	if deploySender.callCount != 1 {
		t.Errorf("Expected deploy command to be called once, got %d", deploySender.callCount)
	}
}

func TestFormatDeployResult(t *testing.T) {
	tests := []struct {
		name     string
		result   *DeployCommandResult
		expected []string
	}{
		{
			name: "accepted status",
			result: &DeployCommandResult{
				ProjectID: "test-project",
				Status:    protocol.DeployStatusAccepted,
				Message:   "Starting deployment",
			},
			expected: []string{"ACCEPTED", "test-project", "Starting deployment"},
		},
		{
			name: "running status",
			result: &DeployCommandResult{
				ProjectID:   "test-project",
				Status:      protocol.DeployStatusRunning,
				Environment: "staging",
			},
			expected: []string{"RUNNING", "test-project", "staging"},
		},
		{
			name: "completed status with version",
			result: &DeployCommandResult{
				ProjectID:   "test-project",
				Status:      protocol.DeployStatusCompleted,
				Environment: "production",
				Version:     "v2.0.0",
			},
			expected: []string{"COMPLETED", "test-project", "production", "v2.0.0"},
		},
		{
			name: "failed status with error",
			result: &DeployCommandResult{
				ProjectID: "test-project",
				Status:    protocol.DeployStatusFailed,
				Error:     "Connection refused",
			},
			expected: []string{"FAILED", "test-project", "Connection refused"},
		},
		{
			name: "rejected status",
			result: &DeployCommandResult{
				ProjectID: "test-project",
				Status:    protocol.DeployStatusRejected,
				Error:     "Insufficient permissions",
			},
			expected: []string{"REJECTED", "test-project", "Insufficient permissions"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatDeployResult(tt.result)
			for _, exp := range tt.expected {
				if !strings.Contains(result, exp) {
					t.Errorf("Expected '%s' in result, got: %s", exp, result)
				}
			}
		})
	}
}

func TestIsEnvironmentName(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		{"staging", "staging", true},
		{"production", "production", true},
		{"prod", "prod", true},
		{"dev", "dev", true},
		{"development", "development", true},
		{"test", "test", true},
		{"qa", "qa", true},
		{"uat", "uat", true},
		{"case insensitive", "PRODUCTION", true},
		{"mixed case", "Staging", true},
		{"project id", "my-project", false},
		{"random string", "xyz123", false},
		{"empty", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isEnvironmentName(tt.input)
			if result != tt.expected {
				t.Errorf("isEnvironmentName(%q) = %v, want %v", tt.input, result, tt.expected)
			}
		})
	}
}

func TestSetDeployCommandSender(t *testing.T) {
	store := user.NewMockStore()
	authMiddleware := NewAuthMiddleware(store)
	sender := newMockSender()
	handler := NewCommandHandler(sender, authMiddleware)

	if handler.deployCommandSender != nil {
		t.Error("Expected no deploy command sender initially")
	}

	// Set deploy command sender
	deploySender := &mockDeployCommandSender{}
	handler.SetDeployCommandSender(deploySender)

	if handler.deployCommandSender == nil {
		t.Error("Expected deploy command sender to be set")
	}
}
