package approval

import (
	"context"
	"database/sql"
	"testing"

	_ "github.com/mattn/go-sqlite3"
)

// mockOrchestratorChecker is a mock implementation for testing.
type mockOrchestratorChecker struct {
	orchestratorPaths map[string]bool
}

func newMockOrchestratorChecker(paths ...string) *mockOrchestratorChecker {
	checker := &mockOrchestratorChecker{
		orchestratorPaths: make(map[string]bool),
	}
	for _, p := range paths {
		checker.orchestratorPaths[p] = true
	}
	return checker
}

func (m *mockOrchestratorChecker) IsOrchestratorPath(path string) bool {
	return m.orchestratorPaths[path]
}

func TestOrchestratorRiskEscalator_EscalateRiskLevel(t *testing.T) {
	tests := []struct {
		name            string
		checker         OrchestratorEditChecker
		toolName        string
		toolInput       string
		baseRiskLevel   RiskLevel
		wantRiskLevel   RiskLevel
		wantEscalated   bool
	}{
		{
			name:            "no checker returns base level",
			checker:         nil,
			toolName:        "write_file",
			toolInput:       `{"path": "/app/main.go"}`,
			baseRiskLevel:   RiskLevelMedium,
			wantRiskLevel:   RiskLevelMedium,
			wantEscalated:   false,
		},
		{
			name:            "non-modification tool not escalated",
			checker:         newMockOrchestratorChecker("/internal/mcp/server.go"),
			toolName:        "read_file",
			toolInput:       `{"path": "/internal/mcp/server.go"}`,
			baseRiskLevel:   RiskLevelLow,
			wantRiskLevel:   RiskLevelLow,
			wantEscalated:   false,
		},
		{
			name:            "orchestrator write_file escalated to critical",
			checker:         newMockOrchestratorChecker("/internal/mcp/server.go"),
			toolName:        "write_file",
			toolInput:       `{"path": "/internal/mcp/server.go"}`,
			baseRiskLevel:   RiskLevelMedium,
			wantRiskLevel:   RiskLevelCritical,
			wantEscalated:   true,
		},
		{
			name:            "orchestrator edit_file escalated to critical",
			checker:         newMockOrchestratorChecker("/internal/approval/manager.go"),
			toolName:        "edit_file",
			toolInput:       `{"path": "/internal/approval/manager.go"}`,
			baseRiskLevel:   RiskLevelMedium,
			wantRiskLevel:   RiskLevelCritical,
			wantEscalated:   true,
		},
		{
			name:            "orchestrator delete_file escalated to critical",
			checker:         newMockOrchestratorChecker("/internal/loop/executor.go"),
			toolName:        "delete_file",
			toolInput:       `{"path": "/internal/loop/executor.go"}`,
			baseRiskLevel:   RiskLevelMedium,
			wantRiskLevel:   RiskLevelCritical,
			wantEscalated:   true,
		},
		{
			name:            "non-orchestrator path not escalated",
			checker:         newMockOrchestratorChecker("/internal/mcp/server.go"),
			toolName:        "write_file",
			toolInput:       `{"path": "/projects/myapp/main.go"}`,
			baseRiskLevel:   RiskLevelMedium,
			wantRiskLevel:   RiskLevelMedium,
			wantEscalated:   false,
		},
		{
			name:            "already critical not escalated again",
			checker:         newMockOrchestratorChecker("/internal/mcp/server.go"),
			toolName:        "write_file",
			toolInput:       `{"path": "/internal/mcp/server.go"}`,
			baseRiskLevel:   RiskLevelCritical,
			wantRiskLevel:   RiskLevelCritical,
			wantEscalated:   false,
		},
		{
			name:            "file_path key also detected",
			checker:         newMockOrchestratorChecker("/internal/agent/provider.go"),
			toolName:        "write_file",
			toolInput:       `{"file_path": "/internal/agent/provider.go"}`,
			baseRiskLevel:   RiskLevelMedium,
			wantRiskLevel:   RiskLevelCritical,
			wantEscalated:   true,
		},
		{
			name:            "git_apply_patch with orchestrator paths",
			checker:         newMockOrchestratorChecker("internal/mcp/server.go"),
			toolName:        "git_apply_patch",
			toolInput:       `{"patch": "diff --git a/internal/mcp/server.go b/internal/mcp/server.go\n--- a/internal/mcp/server.go\n+++ b/internal/mcp/server.go\n@@ -1,3 +1,4 @@\n+// New comment\n package mcp"}`,
			baseRiskLevel:   RiskLevelMedium,
			wantRiskLevel:   RiskLevelCritical,
			wantEscalated:   true,
		},
		{
			name:            "git_apply_patch with working_dir",
			checker:         newMockOrchestratorChecker("/internal"),
			toolName:        "git_apply_patch",
			toolInput:       `{"working_dir": "/internal", "patch": "some patch"}`,
			baseRiskLevel:   RiskLevelMedium,
			wantRiskLevel:   RiskLevelCritical,
			wantEscalated:   true,
		},
		{
			name:            "rename_file source is orchestrator path",
			checker:         newMockOrchestratorChecker("/internal/old.go"),
			toolName:        "rename_file",
			toolInput:       `{"source": "/internal/old.go", "destination": "/other/new.go"}`,
			baseRiskLevel:   RiskLevelMedium,
			wantRiskLevel:   RiskLevelCritical,
			wantEscalated:   true,
		},
		{
			name:            "rename_file destination is orchestrator path",
			checker:         newMockOrchestratorChecker("/internal/new.go"),
			toolName:        "rename_file",
			toolInput:       `{"source": "/other/old.go", "destination": "/internal/new.go"}`,
			baseRiskLevel:   RiskLevelMedium,
			wantRiskLevel:   RiskLevelCritical,
			wantEscalated:   true,
		},
		{
			name:            "empty tool input not escalated",
			checker:         newMockOrchestratorChecker("/internal/mcp/server.go"),
			toolName:        "write_file",
			toolInput:       "",
			baseRiskLevel:   RiskLevelMedium,
			wantRiskLevel:   RiskLevelMedium,
			wantEscalated:   false,
		},
		{
			name:            "invalid JSON not escalated",
			checker:         newMockOrchestratorChecker("/internal/mcp/server.go"),
			toolName:        "write_file",
			toolInput:       "not valid json",
			baseRiskLevel:   RiskLevelMedium,
			wantRiskLevel:   RiskLevelMedium,
			wantEscalated:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			escalator := NewOrchestratorRiskEscalator(tt.checker)
			gotRiskLevel, gotEscalated := escalator.EscalateRiskLevel(tt.toolName, tt.toolInput, tt.baseRiskLevel)

			if gotRiskLevel != tt.wantRiskLevel {
				t.Errorf("EscalateRiskLevel() risk level = %v, want %v", gotRiskLevel, tt.wantRiskLevel)
			}
			if gotEscalated != tt.wantEscalated {
				t.Errorf("EscalateRiskLevel() escalated = %v, want %v", gotEscalated, tt.wantEscalated)
			}
		})
	}
}

func TestOrchestratorRiskEscalator_IsOrchestratorEdit(t *testing.T) {
	tests := []struct {
		name      string
		checker   OrchestratorEditChecker
		toolName  string
		toolInput string
		want      bool
	}{
		{
			name:      "no checker returns false",
			checker:   nil,
			toolName:  "write_file",
			toolInput: `{"path": "/internal/mcp/server.go"}`,
			want:      false,
		},
		{
			name:      "orchestrator write detected",
			checker:   newMockOrchestratorChecker("/internal/mcp/server.go"),
			toolName:  "write_file",
			toolInput: `{"path": "/internal/mcp/server.go"}`,
			want:      true,
		},
		{
			name:      "non-orchestrator write not detected",
			checker:   newMockOrchestratorChecker("/internal/mcp/server.go"),
			toolName:  "write_file",
			toolInput: `{"path": "/projects/app/main.go"}`,
			want:      false,
		},
		{
			name:      "read operations not detected",
			checker:   newMockOrchestratorChecker("/internal/mcp/server.go"),
			toolName:  "read_file",
			toolInput: `{"path": "/internal/mcp/server.go"}`,
			want:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			escalator := NewOrchestratorRiskEscalator(tt.checker)
			got := escalator.IsOrchestratorEdit(tt.toolName, tt.toolInput)

			if got != tt.want {
				t.Errorf("IsOrchestratorEdit() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestExtractPathsFromToolInput(t *testing.T) {
	tests := []struct {
		name      string
		toolName  string
		toolInput string
		wantPaths []string
	}{
		{
			name:      "write_file with path",
			toolName:  "write_file",
			toolInput: `{"path": "/foo/bar.go", "content": "test"}`,
			wantPaths: []string{"/foo/bar.go"},
		},
		{
			name:      "write_file with file_path",
			toolName:  "write_file",
			toolInput: `{"file_path": "/foo/bar.go"}`,
			wantPaths: []string{"/foo/bar.go"},
		},
		{
			name:      "edit_file with path",
			toolName:  "edit_file",
			toolInput: `{"path": "/foo/bar.go", "changes": "test"}`,
			wantPaths: []string{"/foo/bar.go"},
		},
		{
			name:      "rename_file with source and destination",
			toolName:  "rename_file",
			toolInput: `{"source": "/old.go", "destination": "/new.go"}`,
			wantPaths: []string{"/old.go", "/new.go"},
		},
		{
			name:      "rename_file with from and to",
			toolName:  "rename_file",
			toolInput: `{"from": "/old.go", "to": "/new.go"}`,
			wantPaths: []string{"/old.go", "/new.go"},
		},
		{
			name:      "create_directory with path",
			toolName:  "create_directory",
			toolInput: `{"path": "/new/dir"}`,
			wantPaths: []string{"/new/dir"},
		},
		{
			name:      "create_directory with directory",
			toolName:  "create_directory",
			toolInput: `{"directory": "/new/dir"}`,
			wantPaths: []string{"/new/dir"},
		},
		{
			name:      "empty input",
			toolName:  "write_file",
			toolInput: "",
			wantPaths: nil,
		},
		{
			name:      "invalid JSON",
			toolName:  "write_file",
			toolInput: "not json",
			wantPaths: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractPathsFromToolInput(tt.toolName, tt.toolInput)

			if len(got) != len(tt.wantPaths) {
				t.Errorf("extractPathsFromToolInput() got %d paths, want %d", len(got), len(tt.wantPaths))
				return
			}

			for i, p := range got {
				if p != tt.wantPaths[i] {
					t.Errorf("extractPathsFromToolInput() path[%d] = %v, want %v", i, p, tt.wantPaths[i])
				}
			}
		})
	}
}

func TestExtractPathsFromPatch(t *testing.T) {
	tests := []struct {
		name      string
		patch     string
		wantPaths []string
	}{
		{
			name: "standard unified diff",
			patch: `diff --git a/internal/mcp/server.go b/internal/mcp/server.go
--- a/internal/mcp/server.go
+++ b/internal/mcp/server.go
@@ -1,3 +1,4 @@
+// New comment
 package mcp`,
			wantPaths: []string{"internal/mcp/server.go"},
		},
		{
			name: "multiple files in patch",
			patch: `diff --git a/internal/mcp/server.go b/internal/mcp/server.go
--- a/internal/mcp/server.go
+++ b/internal/mcp/server.go
@@ -1,3 +1,4 @@
+// New comment
 package mcp
diff --git a/internal/loop/executor.go b/internal/loop/executor.go
--- a/internal/loop/executor.go
+++ b/internal/loop/executor.go
@@ -1,3 +1,4 @@
+// Another comment
 package loop`,
			wantPaths: []string{"internal/mcp/server.go", "internal/loop/executor.go"},
		},
		{
			name: "new file patch",
			patch: `diff --git a/internal/new_file.go b/internal/new_file.go
--- /dev/null
+++ b/internal/new_file.go
@@ -0,0 +1,3 @@
+package internal`,
			wantPaths: []string{"internal/new_file.go"},
		},
		{
			name: "deleted file patch",
			patch: `diff --git a/internal/old_file.go b/internal/old_file.go
--- a/internal/old_file.go
+++ /dev/null
@@ -1,3 +0,0 @@
-package internal`,
			wantPaths: []string{"internal/old_file.go"},
		},
		{
			name:      "empty patch",
			patch:     "",
			wantPaths: nil,
		},
		{
			name:      "not a patch",
			patch:     "just some random text",
			wantPaths: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractPathsFromPatch(tt.patch)

			if len(got) != len(tt.wantPaths) {
				t.Errorf("extractPathsFromPatch() got %d paths, want %d: %v vs %v", len(got), len(tt.wantPaths), got, tt.wantPaths)
				return
			}

			for i, p := range got {
				if p != tt.wantPaths[i] {
					t.Errorf("extractPathsFromPatch() path[%d] = %v, want %v", i, p, tt.wantPaths[i])
				}
			}
		})
	}
}

func TestIsFileModificationTool(t *testing.T) {
	modificationTools := []string{
		"write_file",
		"edit_file",
		"delete_file",
		"create_directory",
		"git_apply_patch",
		"rename_file",
		"move_file",
		"copy_file",
	}

	nonModificationTools := []string{
		"read_file",
		"list_directory",
		"search_code",
		"glob",
		"grep",
		"run_shell_command",
	}

	for _, tool := range modificationTools {
		if !isFileModificationTool(tool) {
			t.Errorf("isFileModificationTool(%q) = false, want true", tool)
		}
	}

	for _, tool := range nonModificationTools {
		if isFileModificationTool(tool) {
			t.Errorf("isFileModificationTool(%q) = true, want false", tool)
		}
	}
}

func TestOrchestratorEditReason(t *testing.T) {
	tests := []struct {
		name     string
		toolName string
		paths    []string
		want     string
	}{
		{
			name:     "no paths",
			toolName: "write_file",
			paths:    []string{},
			want:     "Orchestrator file modification detected",
		},
		{
			name:     "single path",
			toolName: "write_file",
			paths:    []string{"/internal/mcp/server.go"},
			want:     "Orchestrator file modification: /internal/mcp/server.go",
		},
		{
			name:     "multiple paths",
			toolName: "write_file",
			paths:    []string{"/internal/mcp/server.go", "/internal/loop/executor.go"},
			want:     "Orchestrator file modifications: /internal/mcp/server.go, /internal/loop/executor.go",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := OrchestratorEditReason(tt.toolName, tt.paths)
			if got != tt.want {
				t.Errorf("OrchestratorEditReason() = %v, want %v", got, tt.want)
			}
		})
	}
}

// Integration tests for Manager with orchestrator checking

func TestManager_OrchestratorEditEscalation(t *testing.T) {
	// Create in-memory database for testing
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}
	defer db.Close()

	repo, err := NewSQLiteRepository(db)
	if err != nil {
		t.Fatalf("failed to create repository: %v", err)
	}

	// Create manager with orchestrator checking enabled
	checker := newMockOrchestratorChecker("/internal/mcp/server.go", "/internal/loop/executor.go")
	manager := NewManagerWithOrchestratorCheck(repo, checker)

	// Create an auto-approve policy (auto-approve everything up to medium risk)
	policy, err := manager.CreatePolicy(context.Background(), "Auto-Approve Medium", ApprovalModeRiskBased, RiskLevelMedium)
	if err != nil {
		t.Fatalf("failed to create policy: %v", err)
	}
	if err := manager.SetDefaultPolicy(context.Background(), policy.ID); err != nil {
		t.Fatalf("failed to set default policy: %v", err)
	}

	t.Run("escalates_orchestrator_write_to_critical_and_requires_approval", func(t *testing.T) {
		// Normal write_file is medium risk and would be auto-approved
		// But orchestrator write should escalate to critical and require approval

		request, autoApproved, err := manager.RequestApproval(
			context.Background(),
			"session1",
			"toolcall1",
			"write_file",
			`{"path": "/internal/mcp/server.go", "content": "malicious code"}`,
			"agent1",
			"/projects/test",
		)

		if err != nil {
			t.Fatalf("RequestApproval() error = %v", err)
		}

		if autoApproved {
			t.Error("Expected approval to be required for orchestrator edit, got auto-approved")
		}

		if request.RiskLevel != RiskLevelCritical {
			t.Errorf("Expected risk level %v, got %v", RiskLevelCritical, request.RiskLevel)
		}
	})

	t.Run("auto_approves_non_orchestrator_writes", func(t *testing.T) {
		// Normal write_file to non-orchestrator path should be auto-approved

		request, autoApproved, err := manager.RequestApproval(
			context.Background(),
			"session2",
			"toolcall2",
			"write_file",
			`{"path": "/projects/app/main.go", "content": "normal code"}`,
			"agent1",
			"/projects/test",
		)

		if err != nil {
			t.Fatalf("RequestApproval() error = %v", err)
		}

		if !autoApproved {
			t.Error("Expected auto-approval for non-orchestrator edit")
		}

		if request.RiskLevel != RiskLevelMedium {
			t.Errorf("Expected risk level %v, got %v", RiskLevelMedium, request.RiskLevel)
		}
	})

	t.Run("escalates_git_apply_patch_with_orchestrator_paths", func(t *testing.T) {
		patch := `diff --git a/internal/mcp/server.go b/internal/mcp/server.go
--- a/internal/mcp/server.go
+++ b/internal/mcp/server.go
@@ -1,3 +1,4 @@
+// Injected comment
 package mcp`

		// Mark "internal/mcp/server.go" as orchestrator path
		checker.orchestratorPaths["internal/mcp/server.go"] = true

		request, autoApproved, err := manager.RequestApproval(
			context.Background(),
			"session3",
			"toolcall3",
			"git_apply_patch",
			`{"patch": "`+escapeJSONString(patch)+`"}`,
			"agent1",
			"/projects/test",
		)

		if err != nil {
			t.Fatalf("RequestApproval() error = %v", err)
		}

		if autoApproved {
			t.Error("Expected approval to be required for orchestrator patch")
		}

		if request.RiskLevel != RiskLevelCritical {
			t.Errorf("Expected risk level %v, got %v", RiskLevelCritical, request.RiskLevel)
		}
	})
}

func TestManager_GetEffectiveRiskLevel(t *testing.T) {
	// Create in-memory database for testing
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}
	defer db.Close()

	repo, err := NewSQLiteRepository(db)
	if err != nil {
		t.Fatalf("failed to create repository: %v", err)
	}

	checker := newMockOrchestratorChecker("/internal/mcp/server.go")
	manager := NewManagerWithOrchestratorCheck(repo, checker)

	t.Run("returns_base_risk_for_non_orchestrator_path", func(t *testing.T) {
		level := manager.GetEffectiveRiskLevel("write_file", `{"path": "/app/main.go"}`)
		if level != RiskLevelMedium {
			t.Errorf("Expected %v, got %v", RiskLevelMedium, level)
		}
	})

	t.Run("returns_critical_for_orchestrator_path", func(t *testing.T) {
		level := manager.GetEffectiveRiskLevel("write_file", `{"path": "/internal/mcp/server.go"}`)
		if level != RiskLevelCritical {
			t.Errorf("Expected %v, got %v", RiskLevelCritical, level)
		}
	})

	t.Run("returns_base_risk_when_no_checker", func(t *testing.T) {
		managerNoChecker := NewManager(repo)
		level := managerNoChecker.GetEffectiveRiskLevel("write_file", `{"path": "/internal/mcp/server.go"}`)
		if level != RiskLevelMedium {
			t.Errorf("Expected %v, got %v", RiskLevelMedium, level)
		}
	})
}

func TestManager_IsOrchestratorEdit(t *testing.T) {
	// Create in-memory database for testing
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}
	defer db.Close()

	repo, err := NewSQLiteRepository(db)
	if err != nil {
		t.Fatalf("failed to create repository: %v", err)
	}

	checker := newMockOrchestratorChecker("/internal/mcp/server.go")
	manager := NewManagerWithOrchestratorCheck(repo, checker)

	t.Run("returns_true_for_orchestrator_edit", func(t *testing.T) {
		isEdit := manager.IsOrchestratorEdit("write_file", `{"path": "/internal/mcp/server.go"}`)
		if !isEdit {
			t.Error("Expected IsOrchestratorEdit to return true for orchestrator path")
		}
	})

	t.Run("returns_false_for_non_orchestrator_edit", func(t *testing.T) {
		isEdit := manager.IsOrchestratorEdit("write_file", `{"path": "/app/main.go"}`)
		if isEdit {
			t.Error("Expected IsOrchestratorEdit to return false for non-orchestrator path")
		}
	})

	t.Run("returns_false_for_read_operation", func(t *testing.T) {
		isEdit := manager.IsOrchestratorEdit("read_file", `{"path": "/internal/mcp/server.go"}`)
		if isEdit {
			t.Error("Expected IsOrchestratorEdit to return false for read operation")
		}
	})

	t.Run("returns_false_when_no_checker", func(t *testing.T) {
		managerNoChecker := NewManager(repo)
		isEdit := managerNoChecker.IsOrchestratorEdit("write_file", `{"path": "/internal/mcp/server.go"}`)
		if isEdit {
			t.Error("Expected IsOrchestratorEdit to return false when no checker configured")
		}
	})
}

func TestManager_SetOrchestratorEditChecker(t *testing.T) {
	// Create in-memory database for testing
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}
	defer db.Close()

	repo, err := NewSQLiteRepository(db)
	if err != nil {
		t.Fatalf("failed to create repository: %v", err)
	}

	// Start without checker
	manager := NewManager(repo)

	t.Run("initially_no_orchestrator_detection", func(t *testing.T) {
		isEdit := manager.IsOrchestratorEdit("write_file", `{"path": "/internal/mcp/server.go"}`)
		if isEdit {
			t.Error("Expected no orchestrator detection without checker")
		}
	})

	t.Run("enables_orchestrator_detection_when_checker_set", func(t *testing.T) {
		checker := newMockOrchestratorChecker("/internal/mcp/server.go")
		manager.SetOrchestratorEditChecker(checker)

		isEdit := manager.IsOrchestratorEdit("write_file", `{"path": "/internal/mcp/server.go"}`)
		if !isEdit {
			t.Error("Expected orchestrator detection after setting checker")
		}
	})

	t.Run("disables_orchestrator_detection_when_checker_cleared", func(t *testing.T) {
		manager.SetOrchestratorEditChecker(nil)

		isEdit := manager.IsOrchestratorEdit("write_file", `{"path": "/internal/mcp/server.go"}`)
		if isEdit {
			t.Error("Expected no orchestrator detection after clearing checker")
		}
	})
}

// escapeJSONString escapes a string for use in JSON.
func escapeJSONString(s string) string {
	// Simple escape for newlines in test patches
	result := ""
	for _, c := range s {
		switch c {
		case '\n':
			result += "\\n"
		case '\t':
			result += "\\t"
		case '"':
			result += "\\\""
		case '\\':
			result += "\\\\"
		default:
			result += string(c)
		}
	}
	return result
}
