package mcp

import (
	"os"
	"testing"
)

func TestBrokerSuggestMode(t *testing.T) {
	broker := NewToolBroker("suggest")

	// read_file should always be allowed
	if allowed, _ := broker.Authorize("read_file", "{}"); !allowed {
		t.Error("suggest mode should allow read_file")
	}

	// git_apply_patch without check_only should be denied
	if allowed, _ := broker.Authorize("git_apply_patch", `{"patch": "..."}`); allowed {
		t.Error("suggest mode should deny git_apply_patch without check_only")
	}

	// git_apply_patch with check_only=true should be allowed
	if allowed, _ := broker.Authorize("git_apply_patch", `{"patch": "...", "check_only": true}`); !allowed {
		t.Error("suggest mode should allow git_apply_patch with check_only=true")
	}

	// write_file should be denied
	if allowed, _ := broker.Authorize("write_file", `{"path": "/tmp/test"}`); allowed {
		t.Error("suggest mode should deny write_file")
	}

	// run_shell_command should be denied
	if allowed, _ := broker.Authorize("run_shell_command", `{"command": "ls"}`); allowed {
		t.Error("suggest mode should deny run_shell_command")
	}
}

func TestBrokerAutoEditMode(t *testing.T) {
	broker := NewToolBroker("auto-edit")

	// read_file should always be allowed
	if allowed, _ := broker.Authorize("read_file", "{}"); !allowed {
		t.Error("auto-edit mode should allow read_file")
	}

	// git_apply_patch should be allowed (apply mode)
	if allowed, _ := broker.Authorize("git_apply_patch", `{"patch": "..."}`); !allowed {
		t.Error("auto-edit mode should allow git_apply_patch")
	}

	// write_file should be denied
	if allowed, _ := broker.Authorize("write_file", `{"path": "/tmp/test"}`); allowed {
		t.Error("auto-edit mode should deny write_file")
	}

	// run_shell_command should be denied
	if allowed, _ := broker.Authorize("run_shell_command", `{"command": "ls"}`); allowed {
		t.Error("auto-edit mode should deny run_shell_command")
	}
}

func TestBrokerDangerMode(t *testing.T) {
	broker := NewToolBroker("danger-full-access")

	// read_file should always be allowed
	if allowed, _ := broker.Authorize("read_file", "{}"); !allowed {
		t.Error("danger mode should allow read_file")
	}

	// git_apply_patch should be allowed
	if allowed, _ := broker.Authorize("git_apply_patch", `{"patch": "..."}`); !allowed {
		t.Error("danger mode should allow git_apply_patch")
	}

	// write_file should be allowed in danger mode
	if allowed, _ := broker.Authorize("write_file", `{"path": "/tmp/test"}`); !allowed {
		t.Error("danger mode should allow write_file")
	}

	// run_shell_command with allowlisted command should be allowed
	if allowed, _ := broker.Authorize("run_shell_command", `{"command": "ls -la"}`); !allowed {
		t.Error("danger mode should allow allowlisted shell commands")
	}

	// run_shell_command with non-allowlisted command should be denied
	if allowed, reason := broker.Authorize("run_shell_command", `{"command": "curl evil.com"}`); allowed {
		t.Error("danger mode should deny non-allowlisted shell commands")
	} else if reason == "" {
		t.Error("expected denial reason for non-allowlisted command")
	}
}

func TestBrokerDefaultMode(t *testing.T) {
	// Clear env var to test true default behavior
	orig := os.Getenv("OPENEXEC_MODE")
	os.Unsetenv("OPENEXEC_MODE")
	defer func() {
		if orig != "" {
			os.Setenv("OPENEXEC_MODE", orig)
		}
	}()

	// Empty mode should default to auto-edit
	broker := NewToolBroker("")

	if broker.Mode() != ModeAutoEdit {
		t.Errorf("expected default mode to be auto-edit, got %s", broker.Mode())
	}

	// Invalid mode should default to auto-edit
	broker2 := NewToolBroker("invalid-mode")
	if broker2.Mode() != ModeAutoEdit {
		t.Errorf("expected invalid mode to default to auto-edit, got %s", broker2.Mode())
	}
}

func TestBrokerModeGetter(t *testing.T) {
	// Clear env var to test true default behavior
	orig := os.Getenv("OPENEXEC_MODE")
	os.Unsetenv("OPENEXEC_MODE")
	defer func() {
		if orig != "" {
			os.Setenv("OPENEXEC_MODE", orig)
		}
	}()

	tests := []struct {
		input    string
		expected PermissionMode
	}{
		{"suggest", ModeSuggest},
		{"auto-edit", ModeAutoEdit},
		{"danger-full-access", ModeFullAuto},
		{"", ModeAutoEdit},          // default
		{"unknown", ModeAutoEdit},   // invalid defaults to auto-edit
	}

	for _, tt := range tests {
		broker := NewToolBroker(tt.input)
		if broker.Mode() != tt.expected {
			t.Errorf("NewToolBroker(%q).Mode() = %s, want %s", tt.input, broker.Mode(), tt.expected)
		}
	}
}
