package approval

import (
	"database/sql"
	"testing"
	"time"
)

// TestRiskLevel tests the RiskLevel type and its methods.
func TestRiskLevel(t *testing.T) {
	t.Run("IsValid returns true for valid risk levels", func(t *testing.T) {
		validLevels := []RiskLevel{RiskLevelLow, RiskLevelMedium, RiskLevelHigh, RiskLevelCritical}
		for _, level := range validLevels {
			if !level.IsValid() {
				t.Errorf("expected %s to be valid", level)
			}
		}
	})

	t.Run("IsValid returns false for invalid risk levels", func(t *testing.T) {
		invalid := RiskLevel("invalid")
		if invalid.IsValid() {
			t.Error("expected 'invalid' to be invalid")
		}
	})

	t.Run("String returns correct representation", func(t *testing.T) {
		if RiskLevelHigh.String() != "high" {
			t.Errorf("expected 'high', got %s", RiskLevelHigh.String())
		}
	})

	t.Run("Priority returns correct ordering", func(t *testing.T) {
		if RiskLevelLow.Priority() >= RiskLevelMedium.Priority() {
			t.Error("low should have lower priority than medium")
		}
		if RiskLevelMedium.Priority() >= RiskLevelHigh.Priority() {
			t.Error("medium should have lower priority than high")
		}
		if RiskLevelHigh.Priority() >= RiskLevelCritical.Priority() {
			t.Error("high should have lower priority than critical")
		}
	})
}

// TestRequestStatus tests the RequestStatus type and its methods.
func TestRequestStatus(t *testing.T) {
	t.Run("IsValid returns true for valid statuses", func(t *testing.T) {
		validStatuses := []RequestStatus{
			RequestStatusPending,
			RequestStatusApproved,
			RequestStatusRejected,
			RequestStatusAutoApproved,
			RequestStatusExpired,
			RequestStatusCancelled,
		}
		for _, status := range validStatuses {
			if !status.IsValid() {
				t.Errorf("expected %s to be valid", status)
			}
		}
	})

	t.Run("IsValid returns false for invalid statuses", func(t *testing.T) {
		invalid := RequestStatus("invalid")
		if invalid.IsValid() {
			t.Error("expected 'invalid' to be invalid")
		}
	})

	t.Run("IsFinal returns true for terminal statuses", func(t *testing.T) {
		finalStatuses := []RequestStatus{
			RequestStatusApproved,
			RequestStatusRejected,
			RequestStatusAutoApproved,
			RequestStatusExpired,
			RequestStatusCancelled,
		}
		for _, status := range finalStatuses {
			if !status.IsFinal() {
				t.Errorf("expected %s to be final", status)
			}
		}
	})

	t.Run("IsFinal returns false for pending status", func(t *testing.T) {
		if RequestStatusPending.IsFinal() {
			t.Error("expected pending to not be final")
		}
	})
}

// TestApprovalMode tests the ApprovalMode type and its methods.
func TestApprovalMode(t *testing.T) {
	t.Run("IsValid returns true for valid modes", func(t *testing.T) {
		validModes := []ApprovalMode{
			ApprovalModeAlwaysAsk,
			ApprovalModeAutoApprove,
			ApprovalModeRiskBased,
			ApprovalModeSessionTrust,
		}
		for _, mode := range validModes {
			if !mode.IsValid() {
				t.Errorf("expected %s to be valid", mode)
			}
		}
	})

	t.Run("IsValid returns false for invalid modes", func(t *testing.T) {
		invalid := ApprovalMode("invalid")
		if invalid.IsValid() {
			t.Error("expected 'invalid' to be invalid")
		}
	})
}

// TestNewApprovalRequest tests ApprovalRequest creation.
func TestNewApprovalRequest(t *testing.T) {
	t.Run("creates valid request", func(t *testing.T) {
		request, err := NewApprovalRequest(
			"session-1",
			"toolcall-1",
			"write_file",
			`{"path": "/test.txt"}`,
			RiskLevelMedium,
			"user-1",
		)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if request.ID == "" {
			t.Error("expected ID to be generated")
		}
		if request.SessionID != "session-1" {
			t.Errorf("expected session_id 'session-1', got %s", request.SessionID)
		}
		if request.Status != RequestStatusPending {
			t.Errorf("expected status 'pending', got %s", request.Status)
		}
		if request.RiskLevel != RiskLevelMedium {
			t.Errorf("expected risk_level 'medium', got %s", request.RiskLevel)
		}
	})

	t.Run("requires session_id", func(t *testing.T) {
		_, err := NewApprovalRequest("", "toolcall-1", "write_file", "{}", RiskLevelMedium, "user-1")
		if err == nil {
			t.Error("expected error for empty session_id")
		}
	})

	t.Run("requires tool_call_id", func(t *testing.T) {
		_, err := NewApprovalRequest("session-1", "", "write_file", "{}", RiskLevelMedium, "user-1")
		if err == nil {
			t.Error("expected error for empty tool_call_id")
		}
	})

	t.Run("requires tool_name", func(t *testing.T) {
		_, err := NewApprovalRequest("session-1", "toolcall-1", "", "{}", RiskLevelMedium, "user-1")
		if err == nil {
			t.Error("expected error for empty tool_name")
		}
	})

	t.Run("requires valid risk_level", func(t *testing.T) {
		_, err := NewApprovalRequest("session-1", "toolcall-1", "write_file", "{}", RiskLevel("invalid"), "user-1")
		if err == nil {
			t.Error("expected error for invalid risk_level")
		}
	})

	t.Run("requires requested_by", func(t *testing.T) {
		_, err := NewApprovalRequest("session-1", "toolcall-1", "write_file", "{}", RiskLevelMedium, "")
		if err == nil {
			t.Error("expected error for empty requested_by")
		}
	})
}

// TestApprovalRequest_Validate tests ApprovalRequest validation.
func TestApprovalRequest_Validate(t *testing.T) {
	t.Run("validates correct request", func(t *testing.T) {
		request, _ := NewApprovalRequest("session-1", "toolcall-1", "write_file", "{}", RiskLevelMedium, "user-1")
		if err := request.Validate(); err != nil {
			t.Errorf("unexpected validation error: %v", err)
		}
	})

	t.Run("rejects request without id", func(t *testing.T) {
		request := &ApprovalRequest{
			SessionID:   "session-1",
			ToolCallID:  "toolcall-1",
			ToolName:    "write_file",
			RiskLevel:   RiskLevelMedium,
			Status:      RequestStatusPending,
			RequestedBy: "user-1",
		}
		if err := request.Validate(); err == nil {
			t.Error("expected validation error for missing id")
		}
	})

	t.Run("rejects request with invalid status", func(t *testing.T) {
		request, _ := NewApprovalRequest("session-1", "toolcall-1", "write_file", "{}", RiskLevelMedium, "user-1")
		request.Status = RequestStatus("invalid")
		if err := request.Validate(); err == nil {
			t.Error("expected validation error for invalid status")
		}
	})
}

// TestApprovalRequest_StatusTransitions tests status transition methods.
func TestApprovalRequest_StatusTransitions(t *testing.T) {
	t.Run("Approve transitions pending to approved", func(t *testing.T) {
		request, _ := NewApprovalRequest("session-1", "toolcall-1", "write_file", "{}", RiskLevelMedium, "user-1")
		if err := request.Approve(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if request.Status != RequestStatusApproved {
			t.Errorf("expected status 'approved', got %s", request.Status)
		}
	})

	t.Run("Reject transitions pending to rejected", func(t *testing.T) {
		request, _ := NewApprovalRequest("session-1", "toolcall-1", "write_file", "{}", RiskLevelMedium, "user-1")
		if err := request.Reject(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if request.Status != RequestStatusRejected {
			t.Errorf("expected status 'rejected', got %s", request.Status)
		}
	})

	t.Run("AutoApprove transitions pending to auto_approved", func(t *testing.T) {
		request, _ := NewApprovalRequest("session-1", "toolcall-1", "write_file", "{}", RiskLevelMedium, "user-1")
		if err := request.AutoApprove(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if request.Status != RequestStatusAutoApproved {
			t.Errorf("expected status 'auto_approved', got %s", request.Status)
		}
	})

	t.Run("Expire transitions pending to expired", func(t *testing.T) {
		request, _ := NewApprovalRequest("session-1", "toolcall-1", "write_file", "{}", RiskLevelMedium, "user-1")
		if err := request.Expire(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if request.Status != RequestStatusExpired {
			t.Errorf("expected status 'expired', got %s", request.Status)
		}
	})

	t.Run("Cancel transitions pending to cancelled", func(t *testing.T) {
		request, _ := NewApprovalRequest("session-1", "toolcall-1", "write_file", "{}", RiskLevelMedium, "user-1")
		if err := request.Cancel(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if request.Status != RequestStatusCancelled {
			t.Errorf("expected status 'cancelled', got %s", request.Status)
		}
	})

	t.Run("cannot transition from final state", func(t *testing.T) {
		request, _ := NewApprovalRequest("session-1", "toolcall-1", "write_file", "{}", RiskLevelMedium, "user-1")
		request.Approve()

		if err := request.Reject(); err != ErrAlreadyDecided {
			t.Errorf("expected ErrAlreadyDecided, got %v", err)
		}
		if err := request.Expire(); err != ErrAlreadyDecided {
			t.Errorf("expected ErrAlreadyDecided, got %v", err)
		}
	})
}

// TestApprovalRequest_Expiration tests expiration handling.
func TestApprovalRequest_Expiration(t *testing.T) {
	t.Run("SetExpiration sets the expiration time", func(t *testing.T) {
		request, _ := NewApprovalRequest("session-1", "toolcall-1", "write_file", "{}", RiskLevelMedium, "user-1")
		expTime := time.Now().Add(5 * time.Minute)
		request.SetExpiration(expTime)

		if !request.ExpiresAt.Valid {
			t.Error("expected ExpiresAt to be valid")
		}
	})

	t.Run("IsExpired returns false when no expiration set", func(t *testing.T) {
		request, _ := NewApprovalRequest("session-1", "toolcall-1", "write_file", "{}", RiskLevelMedium, "user-1")
		if request.IsExpired() {
			t.Error("expected request without expiration to not be expired")
		}
	})

	t.Run("IsExpired returns true after expiration time", func(t *testing.T) {
		request, _ := NewApprovalRequest("session-1", "toolcall-1", "write_file", "{}", RiskLevelMedium, "user-1")
		pastTime := time.Now().Add(-5 * time.Minute)
		request.ExpiresAt = sql.NullTime{Time: pastTime, Valid: true}

		if !request.IsExpired() {
			t.Error("expected request to be expired")
		}
	})

	t.Run("IsExpired returns false before expiration time", func(t *testing.T) {
		request, _ := NewApprovalRequest("session-1", "toolcall-1", "write_file", "{}", RiskLevelMedium, "user-1")
		futureTime := time.Now().Add(5 * time.Minute)
		request.ExpiresAt = sql.NullTime{Time: futureTime, Valid: true}

		if request.IsExpired() {
			t.Error("expected request to not be expired")
		}
	})
}

// TestApprovalRequest_StatusHelpers tests status helper methods.
func TestApprovalRequest_StatusHelpers(t *testing.T) {
	t.Run("IsPending returns true for pending status", func(t *testing.T) {
		request, _ := NewApprovalRequest("session-1", "toolcall-1", "write_file", "{}", RiskLevelMedium, "user-1")
		if !request.IsPending() {
			t.Error("expected IsPending to return true")
		}
	})

	t.Run("IsPending returns false for approved status", func(t *testing.T) {
		request, _ := NewApprovalRequest("session-1", "toolcall-1", "write_file", "{}", RiskLevelMedium, "user-1")
		request.Approve()
		if request.IsPending() {
			t.Error("expected IsPending to return false")
		}
	})

	t.Run("IsApproved returns true for approved status", func(t *testing.T) {
		request, _ := NewApprovalRequest("session-1", "toolcall-1", "write_file", "{}", RiskLevelMedium, "user-1")
		request.Approve()
		if !request.IsApproved() {
			t.Error("expected IsApproved to return true")
		}
	})

	t.Run("IsApproved returns true for auto_approved status", func(t *testing.T) {
		request, _ := NewApprovalRequest("session-1", "toolcall-1", "write_file", "{}", RiskLevelMedium, "user-1")
		request.AutoApprove()
		if !request.IsApproved() {
			t.Error("expected IsApproved to return true for auto_approved")
		}
	})

	t.Run("IsApproved returns false for rejected status", func(t *testing.T) {
		request, _ := NewApprovalRequest("session-1", "toolcall-1", "write_file", "{}", RiskLevelMedium, "user-1")
		request.Reject()
		if request.IsApproved() {
			t.Error("expected IsApproved to return false")
		}
	})
}

// TestNewApprovalPolicy tests ApprovalPolicy creation.
func TestNewApprovalPolicy(t *testing.T) {
	t.Run("creates valid policy", func(t *testing.T) {
		policy, err := NewApprovalPolicy("Test Policy", ApprovalModeRiskBased, RiskLevelLow)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if policy.ID == "" {
			t.Error("expected ID to be generated")
		}
		if policy.Name != "Test Policy" {
			t.Errorf("expected name 'Test Policy', got %s", policy.Name)
		}
		if policy.Mode != ApprovalModeRiskBased {
			t.Errorf("expected mode 'risk_based', got %s", policy.Mode)
		}
		if policy.TimeoutSeconds != 300 {
			t.Errorf("expected timeout_seconds 300, got %d", policy.TimeoutSeconds)
		}
		if !policy.IsActive {
			t.Error("expected is_active to be true")
		}
	})

	t.Run("requires name", func(t *testing.T) {
		_, err := NewApprovalPolicy("", ApprovalModeRiskBased, RiskLevelLow)
		if err == nil {
			t.Error("expected error for empty name")
		}
	})

	t.Run("requires valid mode", func(t *testing.T) {
		_, err := NewApprovalPolicy("Test", ApprovalMode("invalid"), RiskLevelLow)
		if err == nil {
			t.Error("expected error for invalid mode")
		}
	})

	t.Run("requires valid auto_approve_risk_level", func(t *testing.T) {
		_, err := NewApprovalPolicy("Test", ApprovalModeRiskBased, RiskLevel("invalid"))
		if err == nil {
			t.Error("expected error for invalid risk level")
		}
	})
}

// TestApprovalPolicy_Validate tests ApprovalPolicy validation.
func TestApprovalPolicy_Validate(t *testing.T) {
	t.Run("validates correct policy", func(t *testing.T) {
		policy, _ := NewApprovalPolicy("Test Policy", ApprovalModeRiskBased, RiskLevelLow)
		if err := policy.Validate(); err != nil {
			t.Errorf("unexpected validation error: %v", err)
		}
	})

	t.Run("rejects policy without id", func(t *testing.T) {
		policy := &ApprovalPolicy{
			Name:                 "Test",
			Mode:                 ApprovalModeRiskBased,
			AutoApproveRiskLevel: RiskLevelLow,
		}
		if err := policy.Validate(); err == nil {
			t.Error("expected validation error for missing id")
		}
	})

	t.Run("rejects negative timeout", func(t *testing.T) {
		policy, _ := NewApprovalPolicy("Test", ApprovalModeRiskBased, RiskLevelLow)
		policy.TimeoutSeconds = -1
		if err := policy.Validate(); err == nil {
			t.Error("expected validation error for negative timeout")
		}
	})
}

// TestApprovalPolicy_ShouldAutoApprove tests auto-approve logic.
func TestApprovalPolicy_ShouldAutoApprove(t *testing.T) {
	t.Run("always_ask mode never auto-approves", func(t *testing.T) {
		policy, _ := NewApprovalPolicy("Test", ApprovalModeAlwaysAsk, RiskLevelLow)
		if policy.ShouldAutoApprove("read_file", RiskLevelLow) {
			t.Error("expected always_ask to not auto-approve")
		}
	})

	t.Run("auto_approve mode always auto-approves", func(t *testing.T) {
		policy, _ := NewApprovalPolicy("Test", ApprovalModeAutoApprove, RiskLevelLow)
		if !policy.ShouldAutoApprove("run_shell_command", RiskLevelCritical) {
			t.Error("expected auto_approve to auto-approve all")
		}
	})

	t.Run("risk_based mode auto-approves below threshold", func(t *testing.T) {
		policy, _ := NewApprovalPolicy("Test", ApprovalModeRiskBased, RiskLevelMedium)

		if !policy.ShouldAutoApprove("read_file", RiskLevelLow) {
			t.Error("expected low risk to be auto-approved")
		}
		if !policy.ShouldAutoApprove("write_file", RiskLevelMedium) {
			t.Error("expected medium risk to be auto-approved at threshold")
		}
		if policy.ShouldAutoApprove("run_shell_command", RiskLevelHigh) {
			t.Error("expected high risk to not be auto-approved")
		}
	})

	t.Run("session_trust mode does not auto-approve (handled externally)", func(t *testing.T) {
		policy, _ := NewApprovalPolicy("Test", ApprovalModeSessionTrust, RiskLevelLow)
		if policy.ShouldAutoApprove("read_file", RiskLevelLow) {
			t.Error("expected session_trust to not auto-approve directly")
		}
	})
}

// TestApprovalPolicy_Activation tests activation methods.
func TestApprovalPolicy_Activation(t *testing.T) {
	t.Run("Deactivate sets is_active to false", func(t *testing.T) {
		policy, _ := NewApprovalPolicy("Test", ApprovalModeRiskBased, RiskLevelLow)
		policy.Deactivate()
		if policy.IsActive {
			t.Error("expected is_active to be false")
		}
	})

	t.Run("Activate sets is_active to true", func(t *testing.T) {
		policy, _ := NewApprovalPolicy("Test", ApprovalModeRiskBased, RiskLevelLow)
		policy.Deactivate()
		policy.Activate()
		if !policy.IsActive {
			t.Error("expected is_active to be true")
		}
	})
}

// TestApprovalPolicy_GetTimeout tests timeout calculation.
func TestApprovalPolicy_GetTimeout(t *testing.T) {
	t.Run("returns correct duration", func(t *testing.T) {
		policy, _ := NewApprovalPolicy("Test", ApprovalModeRiskBased, RiskLevelLow)
		policy.TimeoutSeconds = 120

		expected := 120 * time.Second
		if policy.GetTimeout() != expected {
			t.Errorf("expected %v, got %v", expected, policy.GetTimeout())
		}
	})
}

// TestNewApprovalDecision tests ApprovalDecision creation.
func TestNewApprovalDecision(t *testing.T) {
	t.Run("creates valid decision", func(t *testing.T) {
		decision, err := NewApprovalDecision("request-1", RequestStatusApproved, "user-1")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if decision.ID == "" {
			t.Error("expected ID to be generated")
		}
		if decision.RequestID != "request-1" {
			t.Errorf("expected request_id 'request-1', got %s", decision.RequestID)
		}
		if decision.Decision != RequestStatusApproved {
			t.Errorf("expected decision 'approved', got %s", decision.Decision)
		}
	})

	t.Run("requires request_id", func(t *testing.T) {
		_, err := NewApprovalDecision("", RequestStatusApproved, "user-1")
		if err == nil {
			t.Error("expected error for empty request_id")
		}
	})

	t.Run("requires valid decision", func(t *testing.T) {
		_, err := NewApprovalDecision("request-1", RequestStatus("invalid"), "user-1")
		if err == nil {
			t.Error("expected error for invalid decision")
		}
	})

	t.Run("requires final decision status", func(t *testing.T) {
		_, err := NewApprovalDecision("request-1", RequestStatusPending, "user-1")
		if err == nil {
			t.Error("expected error for non-final decision")
		}
	})

	t.Run("requires decided_by", func(t *testing.T) {
		_, err := NewApprovalDecision("request-1", RequestStatusApproved, "")
		if err == nil {
			t.Error("expected error for empty decided_by")
		}
	})
}

// TestApprovalDecision_Validate tests ApprovalDecision validation.
func TestApprovalDecision_Validate(t *testing.T) {
	t.Run("validates correct decision", func(t *testing.T) {
		decision, _ := NewApprovalDecision("request-1", RequestStatusApproved, "user-1")
		if err := decision.Validate(); err != nil {
			t.Errorf("unexpected validation error: %v", err)
		}
	})

	t.Run("rejects decision without id", func(t *testing.T) {
		decision := &ApprovalDecision{
			RequestID: "request-1",
			Decision:  RequestStatusApproved,
			DecidedBy: "user-1",
		}
		if err := decision.Validate(); err == nil {
			t.Error("expected validation error for missing id")
		}
	})
}

// TestApprovalDecision_IsManual tests manual decision detection.
func TestApprovalDecision_IsManual(t *testing.T) {
	t.Run("returns true for user decisions", func(t *testing.T) {
		decision, _ := NewApprovalDecision("request-1", RequestStatusApproved, "user-123")
		if !decision.IsManual() {
			t.Error("expected user decision to be manual")
		}
	})

	t.Run("returns false for system decisions", func(t *testing.T) {
		decision, _ := NewApprovalDecision("request-1", RequestStatusApproved, "system")
		if decision.IsManual() {
			t.Error("expected system decision to not be manual")
		}
	})

	t.Run("returns false for policy decisions", func(t *testing.T) {
		decision, _ := NewApprovalDecision("request-1", RequestStatusAutoApproved, "policy")
		if decision.IsManual() {
			t.Error("expected policy decision to not be manual")
		}
	})
}

// TestDefaultToolRiskMappings tests the default risk mappings.
func TestDefaultToolRiskMappings(t *testing.T) {
	t.Run("returns mappings for expected tools", func(t *testing.T) {
		mappings := DefaultToolRiskMappings()
		if len(mappings) == 0 {
			t.Error("expected non-empty mappings")
		}

		// Check some expected mappings
		toolLevels := make(map[string]RiskLevel)
		for _, m := range mappings {
			toolLevels[m.ToolName] = m.RiskLevel
		}

		if level, ok := toolLevels["read_file"]; !ok || level != RiskLevelLow {
			t.Error("expected read_file to have low risk")
		}
		if level, ok := toolLevels["write_file"]; !ok || level != RiskLevelMedium {
			t.Error("expected write_file to have medium risk")
		}
		if level, ok := toolLevels["run_shell_command"]; !ok || level != RiskLevelHigh {
			t.Error("expected run_shell_command to have high risk")
		}
		if level, ok := toolLevels["modify_orchestrator"]; !ok || level != RiskLevelCritical {
			t.Error("expected modify_orchestrator to have critical risk")
		}
	})

	t.Run("all mappings have valid risk levels", func(t *testing.T) {
		mappings := DefaultToolRiskMappings()
		for _, m := range mappings {
			if !m.RiskLevel.IsValid() {
				t.Errorf("tool %s has invalid risk level: %s", m.ToolName, m.RiskLevel)
			}
		}
	})
}
