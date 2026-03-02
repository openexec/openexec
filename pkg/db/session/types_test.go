package session

import (
	"database/sql"
	"testing"
	"time"
)

func TestStatus_IsValid(t *testing.T) {
	tests := []struct {
		name   string
		status Status
		want   bool
	}{
		{"active is valid", StatusActive, true},
		{"paused is valid", StatusPaused, true},
		{"archived is valid", StatusArchived, true},
		{"deleted is valid", StatusDeleted, true},
		{"empty is invalid", Status(""), false},
		{"unknown is invalid", Status("unknown"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.status.IsValid(); got != tt.want {
				t.Errorf("Status.IsValid() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestStatus_String(t *testing.T) {
	if got := StatusActive.String(); got != "active" {
		t.Errorf("StatusActive.String() = %v, want %v", got, "active")
	}
}

func TestRole_IsValid(t *testing.T) {
	tests := []struct {
		name string
		role Role
		want bool
	}{
		{"user is valid", RoleUser, true},
		{"assistant is valid", RoleAssistant, true},
		{"system is valid", RoleSystem, true},
		{"empty is invalid", Role(""), false},
		{"unknown is invalid", Role("unknown"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.role.IsValid(); got != tt.want {
				t.Errorf("Role.IsValid() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRole_String(t *testing.T) {
	if got := RoleUser.String(); got != "user" {
		t.Errorf("RoleUser.String() = %v, want %v", got, "user")
	}
}

func TestToolCallStatus_IsValid(t *testing.T) {
	tests := []struct {
		name   string
		status ToolCallStatus
		want   bool
	}{
		{"pending is valid", ToolCallStatusPending, true},
		{"running is valid", ToolCallStatusRunning, true},
		{"completed is valid", ToolCallStatusCompleted, true},
		{"failed is valid", ToolCallStatusFailed, true},
		{"cancelled is valid", ToolCallStatusCancelled, true},
		{"empty is invalid", ToolCallStatus(""), false},
		{"unknown is invalid", ToolCallStatus("unknown"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.status.IsValid(); got != tt.want {
				t.Errorf("ToolCallStatus.IsValid() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestToolCallStatus_String(t *testing.T) {
	if got := ToolCallStatusPending.String(); got != "pending" {
		t.Errorf("ToolCallStatusPending.String() = %v, want %v", got, "pending")
	}
}

func TestApprovalStatus_IsValid(t *testing.T) {
	tests := []struct {
		name   string
		status ApprovalStatus
		want   bool
	}{
		{"pending is valid", ApprovalStatusPending, true},
		{"approved is valid", ApprovalStatusApproved, true},
		{"rejected is valid", ApprovalStatusRejected, true},
		{"auto_approved is valid", ApprovalStatusAutoApproved, true},
		{"empty is invalid", ApprovalStatus(""), false},
		{"unknown is invalid", ApprovalStatus("unknown"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.status.IsValid(); got != tt.want {
				t.Errorf("ApprovalStatus.IsValid() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestApprovalStatus_String(t *testing.T) {
	if got := ApprovalStatusApproved.String(); got != "approved" {
		t.Errorf("ApprovalStatusApproved.String() = %v, want %v", got, "approved")
	}
}

func TestNewSession(t *testing.T) {
	tests := []struct {
		name        string
		projectPath string
		provider    string
		model       string
		wantErr     bool
	}{
		{
			name:        "valid session",
			projectPath: "/path/to/project",
			provider:    "openai",
			model:       "gpt-4",
			wantErr:     false,
		},
		{
			name:        "missing project path",
			projectPath: "",
			provider:    "openai",
			model:       "gpt-4",
			wantErr:     true,
		},
		{
			name:        "missing provider",
			projectPath: "/path/to/project",
			provider:    "",
			model:       "gpt-4",
			wantErr:     true,
		},
		{
			name:        "missing model",
			projectPath: "/path/to/project",
			provider:    "openai",
			model:       "",
			wantErr:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			session, err := NewSession(tt.projectPath, tt.provider, tt.model)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewSession() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				if session.ID == "" {
					t.Error("NewSession() should generate an ID")
				}
				if session.ProjectPath != tt.projectPath {
					t.Errorf("NewSession() projectPath = %v, want %v", session.ProjectPath, tt.projectPath)
				}
				if session.Provider != tt.provider {
					t.Errorf("NewSession() provider = %v, want %v", session.Provider, tt.provider)
				}
				if session.Model != tt.model {
					t.Errorf("NewSession() model = %v, want %v", session.Model, tt.model)
				}
				if session.Status != StatusActive {
					t.Errorf("NewSession() status = %v, want %v", session.Status, StatusActive)
				}
				if session.CreatedAt.IsZero() {
					t.Error("NewSession() should set CreatedAt")
				}
				if session.UpdatedAt.IsZero() {
					t.Error("NewSession() should set UpdatedAt")
				}
			}
		})
	}
}

func TestSession_Validate(t *testing.T) {
	validSession := &Session{
		ID:          "test-id",
		ProjectPath: "/path/to/project",
		Provider:    "openai",
		Model:       "gpt-4",
		Status:      StatusActive,
	}

	tests := []struct {
		name    string
		session *Session
		wantErr bool
	}{
		{
			name:    "valid session",
			session: validSession,
			wantErr: false,
		},
		{
			name: "missing id",
			session: &Session{
				ProjectPath: "/path",
				Provider:    "openai",
				Model:       "gpt-4",
				Status:      StatusActive,
			},
			wantErr: true,
		},
		{
			name: "missing project path",
			session: &Session{
				ID:       "test-id",
				Provider: "openai",
				Model:    "gpt-4",
				Status:   StatusActive,
			},
			wantErr: true,
		},
		{
			name: "missing provider",
			session: &Session{
				ID:          "test-id",
				ProjectPath: "/path",
				Model:       "gpt-4",
				Status:      StatusActive,
			},
			wantErr: true,
		},
		{
			name: "missing model",
			session: &Session{
				ID:          "test-id",
				ProjectPath: "/path",
				Provider:    "openai",
				Status:      StatusActive,
			},
			wantErr: true,
		},
		{
			name: "invalid status",
			session: &Session{
				ID:          "test-id",
				ProjectPath: "/path",
				Provider:    "openai",
				Model:       "gpt-4",
				Status:      Status("invalid"),
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.session.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Session.Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestSession_IsFork(t *testing.T) {
	session := &Session{
		ID:          "test-id",
		ProjectPath: "/path",
		Provider:    "openai",
		Model:       "gpt-4",
		Status:      StatusActive,
	}

	if session.IsFork() {
		t.Error("Session without parent should not be a fork")
	}

	session.ParentSessionID = sql.NullString{String: "parent-id", Valid: true}
	if !session.IsFork() {
		t.Error("Session with parent should be a fork")
	}
}

func TestSession_SetTitle(t *testing.T) {
	session := &Session{
		ID:        "test-id",
		UpdatedAt: time.Now().Add(-time.Hour),
	}
	oldUpdatedAt := session.UpdatedAt

	session.SetTitle("New Title")

	if session.Title != "New Title" {
		t.Errorf("SetTitle() title = %v, want %v", session.Title, "New Title")
	}
	if !session.UpdatedAt.After(oldUpdatedAt) {
		t.Error("SetTitle() should update UpdatedAt")
	}
}

func TestSession_Archive(t *testing.T) {
	session := &Session{
		ID:        "test-id",
		Status:    StatusActive,
		UpdatedAt: time.Now().Add(-time.Hour),
	}
	oldUpdatedAt := session.UpdatedAt

	session.Archive()

	if session.Status != StatusArchived {
		t.Errorf("Archive() status = %v, want %v", session.Status, StatusArchived)
	}
	if !session.UpdatedAt.After(oldUpdatedAt) {
		t.Error("Archive() should update UpdatedAt")
	}
}

func TestNewMessage(t *testing.T) {
	tests := []struct {
		name      string
		sessionID string
		role      Role
		content   string
		wantErr   bool
	}{
		{
			name:      "valid user message",
			sessionID: "session-id",
			role:      RoleUser,
			content:   "Hello",
			wantErr:   false,
		},
		{
			name:      "valid assistant message",
			sessionID: "session-id",
			role:      RoleAssistant,
			content:   "Hi there",
			wantErr:   false,
		},
		{
			name:      "empty content is allowed",
			sessionID: "session-id",
			role:      RoleUser,
			content:   "",
			wantErr:   false,
		},
		{
			name:      "missing session id",
			sessionID: "",
			role:      RoleUser,
			content:   "Hello",
			wantErr:   true,
		},
		{
			name:      "invalid role",
			sessionID: "session-id",
			role:      Role("invalid"),
			content:   "Hello",
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg, err := NewMessage(tt.sessionID, tt.role, tt.content)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewMessage() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				if msg.ID == "" {
					t.Error("NewMessage() should generate an ID")
				}
				if msg.SessionID != tt.sessionID {
					t.Errorf("NewMessage() sessionID = %v, want %v", msg.SessionID, tt.sessionID)
				}
				if msg.Role != tt.role {
					t.Errorf("NewMessage() role = %v, want %v", msg.Role, tt.role)
				}
				if msg.Content != tt.content {
					t.Errorf("NewMessage() content = %v, want %v", msg.Content, tt.content)
				}
				if msg.CreatedAt.IsZero() {
					t.Error("NewMessage() should set CreatedAt")
				}
			}
		})
	}
}

func TestMessage_Validate(t *testing.T) {
	tests := []struct {
		name    string
		message *Message
		wantErr bool
	}{
		{
			name: "valid message",
			message: &Message{
				ID:        "msg-id",
				SessionID: "session-id",
				Role:      RoleUser,
			},
			wantErr: false,
		},
		{
			name: "missing id",
			message: &Message{
				SessionID: "session-id",
				Role:      RoleUser,
			},
			wantErr: true,
		},
		{
			name: "missing session id",
			message: &Message{
				ID:   "msg-id",
				Role: RoleUser,
			},
			wantErr: true,
		},
		{
			name: "invalid role",
			message: &Message{
				ID:        "msg-id",
				SessionID: "session-id",
				Role:      Role("invalid"),
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.message.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Message.Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestMessage_SetTokenUsage(t *testing.T) {
	msg := &Message{}
	msg.SetTokenUsage(100, 200, 0.05)

	if msg.TokensInput != 100 {
		t.Errorf("SetTokenUsage() TokensInput = %v, want %v", msg.TokensInput, 100)
	}
	if msg.TokensOutput != 200 {
		t.Errorf("SetTokenUsage() TokensOutput = %v, want %v", msg.TokensOutput, 200)
	}
	if msg.CostUSD != 0.05 {
		t.Errorf("SetTokenUsage() CostUSD = %v, want %v", msg.CostUSD, 0.05)
	}
}

func TestNewToolCall(t *testing.T) {
	tests := []struct {
		name      string
		messageID string
		sessionID string
		toolName  string
		toolInput string
		wantErr   bool
	}{
		{
			name:      "valid tool call",
			messageID: "msg-id",
			sessionID: "session-id",
			toolName:  "read_file",
			toolInput: `{"path": "/test.txt"}`,
			wantErr:   false,
		},
		{
			name:      "missing message id",
			messageID: "",
			sessionID: "session-id",
			toolName:  "read_file",
			toolInput: `{}`,
			wantErr:   true,
		},
		{
			name:      "missing session id",
			messageID: "msg-id",
			sessionID: "",
			toolName:  "read_file",
			toolInput: `{}`,
			wantErr:   true,
		},
		{
			name:      "missing tool name",
			messageID: "msg-id",
			sessionID: "session-id",
			toolName:  "",
			toolInput: `{}`,
			wantErr:   true,
		},
		{
			name:      "empty input is allowed",
			messageID: "msg-id",
			sessionID: "session-id",
			toolName:  "read_file",
			toolInput: "",
			wantErr:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tc, err := NewToolCall(tt.messageID, tt.sessionID, tt.toolName, tt.toolInput)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewToolCall() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				if tc.ID == "" {
					t.Error("NewToolCall() should generate an ID")
				}
				if tc.MessageID != tt.messageID {
					t.Errorf("NewToolCall() messageID = %v, want %v", tc.MessageID, tt.messageID)
				}
				if tc.SessionID != tt.sessionID {
					t.Errorf("NewToolCall() sessionID = %v, want %v", tc.SessionID, tt.sessionID)
				}
				if tc.ToolName != tt.toolName {
					t.Errorf("NewToolCall() toolName = %v, want %v", tc.ToolName, tt.toolName)
				}
				if tc.ToolInput != tt.toolInput {
					t.Errorf("NewToolCall() toolInput = %v, want %v", tc.ToolInput, tt.toolInput)
				}
				if tc.Status != ToolCallStatusPending {
					t.Errorf("NewToolCall() status = %v, want %v", tc.Status, ToolCallStatusPending)
				}
			}
		})
	}
}

func TestToolCall_Validate(t *testing.T) {
	tests := []struct {
		name     string
		toolCall *ToolCall
		wantErr  bool
	}{
		{
			name: "valid tool call",
			toolCall: &ToolCall{
				ID:        "tc-id",
				MessageID: "msg-id",
				SessionID: "session-id",
				ToolName:  "read_file",
				Status:    ToolCallStatusPending,
			},
			wantErr: false,
		},
		{
			name: "missing id",
			toolCall: &ToolCall{
				MessageID: "msg-id",
				SessionID: "session-id",
				ToolName:  "read_file",
				Status:    ToolCallStatusPending,
			},
			wantErr: true,
		},
		{
			name: "missing message id",
			toolCall: &ToolCall{
				ID:        "tc-id",
				SessionID: "session-id",
				ToolName:  "read_file",
				Status:    ToolCallStatusPending,
			},
			wantErr: true,
		},
		{
			name: "missing session id",
			toolCall: &ToolCall{
				ID:        "tc-id",
				MessageID: "msg-id",
				ToolName:  "read_file",
				Status:    ToolCallStatusPending,
			},
			wantErr: true,
		},
		{
			name: "missing tool name",
			toolCall: &ToolCall{
				ID:        "tc-id",
				MessageID: "msg-id",
				SessionID: "session-id",
				Status:    ToolCallStatusPending,
			},
			wantErr: true,
		},
		{
			name: "invalid status",
			toolCall: &ToolCall{
				ID:        "tc-id",
				MessageID: "msg-id",
				SessionID: "session-id",
				ToolName:  "read_file",
				Status:    ToolCallStatus("invalid"),
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.toolCall.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("ToolCall.Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestToolCall_Start(t *testing.T) {
	tc := &ToolCall{Status: ToolCallStatusPending}
	tc.Start()

	if tc.Status != ToolCallStatusRunning {
		t.Errorf("Start() status = %v, want %v", tc.Status, ToolCallStatusRunning)
	}
	if !tc.StartedAt.Valid {
		t.Error("Start() should set StartedAt")
	}
}

func TestToolCall_Complete(t *testing.T) {
	tc := &ToolCall{Status: ToolCallStatusRunning}
	tc.Complete(`{"result": "success"}`)

	if tc.Status != ToolCallStatusCompleted {
		t.Errorf("Complete() status = %v, want %v", tc.Status, ToolCallStatusCompleted)
	}
	if !tc.ToolOutput.Valid || tc.ToolOutput.String != `{"result": "success"}` {
		t.Errorf("Complete() output = %v, want %v", tc.ToolOutput, `{"result": "success"}`)
	}
	if !tc.CompletedAt.Valid {
		t.Error("Complete() should set CompletedAt")
	}
}

func TestToolCall_Fail(t *testing.T) {
	tc := &ToolCall{Status: ToolCallStatusRunning}
	tc.Fail("something went wrong")

	if tc.Status != ToolCallStatusFailed {
		t.Errorf("Fail() status = %v, want %v", tc.Status, ToolCallStatusFailed)
	}
	if !tc.Error.Valid || tc.Error.String != "something went wrong" {
		t.Errorf("Fail() error = %v, want %v", tc.Error, "something went wrong")
	}
	if !tc.CompletedAt.Valid {
		t.Error("Fail() should set CompletedAt")
	}
}

func TestToolCall_Cancel(t *testing.T) {
	tc := &ToolCall{Status: ToolCallStatusPending}
	tc.Cancel()

	if tc.Status != ToolCallStatusCancelled {
		t.Errorf("Cancel() status = %v, want %v", tc.Status, ToolCallStatusCancelled)
	}
	if !tc.CompletedAt.Valid {
		t.Error("Cancel() should set CompletedAt")
	}
}

func TestToolCall_Approve(t *testing.T) {
	tc := &ToolCall{}
	tc.Approve("user@example.com")

	if !tc.ApprovalStatus.Valid || tc.ApprovalStatus.String != string(ApprovalStatusApproved) {
		t.Errorf("Approve() approval_status = %v, want %v", tc.ApprovalStatus, ApprovalStatusApproved)
	}
	if !tc.ApprovedBy.Valid || tc.ApprovedBy.String != "user@example.com" {
		t.Errorf("Approve() approved_by = %v, want %v", tc.ApprovedBy, "user@example.com")
	}
	if !tc.ApprovedAt.Valid {
		t.Error("Approve() should set ApprovedAt")
	}
}

func TestToolCall_Reject(t *testing.T) {
	tc := &ToolCall{Status: ToolCallStatusPending}
	tc.Reject("admin@example.com")

	if !tc.ApprovalStatus.Valid || tc.ApprovalStatus.String != string(ApprovalStatusRejected) {
		t.Errorf("Reject() approval_status = %v, want %v", tc.ApprovalStatus, ApprovalStatusRejected)
	}
	if !tc.ApprovedBy.Valid || tc.ApprovedBy.String != "admin@example.com" {
		t.Errorf("Reject() approved_by = %v, want %v", tc.ApprovedBy, "admin@example.com")
	}
	if tc.Status != ToolCallStatusCancelled {
		t.Errorf("Reject() status = %v, want %v", tc.Status, ToolCallStatusCancelled)
	}
}

func TestToolCall_AutoApprove(t *testing.T) {
	tc := &ToolCall{}
	tc.AutoApprove()

	if !tc.ApprovalStatus.Valid || tc.ApprovalStatus.String != string(ApprovalStatusAutoApproved) {
		t.Errorf("AutoApprove() approval_status = %v, want %v", tc.ApprovalStatus, ApprovalStatusAutoApproved)
	}
	if !tc.ApprovedAt.Valid {
		t.Error("AutoApprove() should set ApprovedAt")
	}
}

func TestToolCall_NeedsApproval(t *testing.T) {
	tests := []struct {
		name           string
		approvalStatus sql.NullString
		want           bool
	}{
		{
			name:           "no approval status",
			approvalStatus: sql.NullString{},
			want:           true,
		},
		{
			name:           "pending approval",
			approvalStatus: sql.NullString{String: string(ApprovalStatusPending), Valid: true},
			want:           true,
		},
		{
			name:           "approved",
			approvalStatus: sql.NullString{String: string(ApprovalStatusApproved), Valid: true},
			want:           false,
		},
		{
			name:           "rejected",
			approvalStatus: sql.NullString{String: string(ApprovalStatusRejected), Valid: true},
			want:           false,
		},
		{
			name:           "auto approved",
			approvalStatus: sql.NullString{String: string(ApprovalStatusAutoApproved), Valid: true},
			want:           false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tc := &ToolCall{ApprovalStatus: tt.approvalStatus}
			if got := tc.NeedsApproval(); got != tt.want {
				t.Errorf("ToolCall.NeedsApproval() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestToolCall_IsApproved(t *testing.T) {
	tests := []struct {
		name           string
		approvalStatus sql.NullString
		want           bool
	}{
		{
			name:           "no approval status",
			approvalStatus: sql.NullString{},
			want:           false,
		},
		{
			name:           "pending approval",
			approvalStatus: sql.NullString{String: string(ApprovalStatusPending), Valid: true},
			want:           false,
		},
		{
			name:           "approved",
			approvalStatus: sql.NullString{String: string(ApprovalStatusApproved), Valid: true},
			want:           true,
		},
		{
			name:           "rejected",
			approvalStatus: sql.NullString{String: string(ApprovalStatusRejected), Valid: true},
			want:           false,
		},
		{
			name:           "auto approved",
			approvalStatus: sql.NullString{String: string(ApprovalStatusAutoApproved), Valid: true},
			want:           true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tc := &ToolCall{ApprovalStatus: tt.approvalStatus}
			if got := tc.IsApproved(); got != tt.want {
				t.Errorf("ToolCall.IsApproved() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNewSessionSummary(t *testing.T) {
	tests := []struct {
		name               string
		sessionID          string
		summaryText        string
		messagesSummarized int
		tokensSaved        int
		wantErr            bool
	}{
		{
			name:               "valid summary",
			sessionID:          "session-id",
			summaryText:        "Summary of conversation...",
			messagesSummarized: 10,
			tokensSaved:        5000,
			wantErr:            false,
		},
		{
			name:               "missing session id",
			sessionID:          "",
			summaryText:        "Summary...",
			messagesSummarized: 10,
			tokensSaved:        5000,
			wantErr:            true,
		},
		{
			name:               "missing summary text",
			sessionID:          "session-id",
			summaryText:        "",
			messagesSummarized: 10,
			tokensSaved:        5000,
			wantErr:            true,
		},
		{
			name:               "zero messages summarized",
			sessionID:          "session-id",
			summaryText:        "Summary...",
			messagesSummarized: 0,
			tokensSaved:        5000,
			wantErr:            true,
		},
		{
			name:               "negative messages summarized",
			sessionID:          "session-id",
			summaryText:        "Summary...",
			messagesSummarized: -1,
			tokensSaved:        5000,
			wantErr:            true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			summary, err := NewSessionSummary(tt.sessionID, tt.summaryText, tt.messagesSummarized, tt.tokensSaved)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewSessionSummary() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				if summary.ID == "" {
					t.Error("NewSessionSummary() should generate an ID")
				}
				if summary.SessionID != tt.sessionID {
					t.Errorf("NewSessionSummary() sessionID = %v, want %v", summary.SessionID, tt.sessionID)
				}
				if summary.SummaryText != tt.summaryText {
					t.Errorf("NewSessionSummary() summaryText = %v, want %v", summary.SummaryText, tt.summaryText)
				}
				if summary.MessagesSummarized != tt.messagesSummarized {
					t.Errorf("NewSessionSummary() messagesSummarized = %v, want %v", summary.MessagesSummarized, tt.messagesSummarized)
				}
				if summary.TokensSaved != tt.tokensSaved {
					t.Errorf("NewSessionSummary() tokensSaved = %v, want %v", summary.TokensSaved, tt.tokensSaved)
				}
			}
		})
	}
}

func TestSessionSummary_Validate(t *testing.T) {
	tests := []struct {
		name    string
		summary *SessionSummary
		wantErr bool
	}{
		{
			name: "valid summary",
			summary: &SessionSummary{
				ID:                 "summary-id",
				SessionID:          "session-id",
				SummaryText:        "Summary...",
				MessagesSummarized: 10,
			},
			wantErr: false,
		},
		{
			name: "missing id",
			summary: &SessionSummary{
				SessionID:          "session-id",
				SummaryText:        "Summary...",
				MessagesSummarized: 10,
			},
			wantErr: true,
		},
		{
			name: "missing session id",
			summary: &SessionSummary{
				ID:                 "summary-id",
				SummaryText:        "Summary...",
				MessagesSummarized: 10,
			},
			wantErr: true,
		},
		{
			name: "missing summary text",
			summary: &SessionSummary{
				ID:                 "summary-id",
				SessionID:          "session-id",
				MessagesSummarized: 10,
			},
			wantErr: true,
		},
		{
			name: "zero messages summarized",
			summary: &SessionSummary{
				ID:                 "summary-id",
				SessionID:          "session-id",
				SummaryText:        "Summary...",
				MessagesSummarized: 0,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.summary.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("SessionSummary.Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
