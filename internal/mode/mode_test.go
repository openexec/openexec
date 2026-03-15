package mode

import (
	"testing"
)

func TestMode_IsValid(t *testing.T) {
	tests := []struct {
		mode  Mode
		valid bool
	}{
		{ModeChat, true},
		{ModeTask, true},
		{ModeRun, true},
		{Mode("invalid"), false},
		{Mode(""), false},
	}

	for _, tc := range tests {
		t.Run(string(tc.mode), func(t *testing.T) {
			if got := tc.mode.IsValid(); got != tc.valid {
				t.Errorf("Mode(%q).IsValid() = %v, want %v", tc.mode, got, tc.valid)
			}
		})
	}
}

func TestMode_AllowsWrites(t *testing.T) {
	tests := []struct {
		mode   Mode
		allows bool
	}{
		{ModeChat, false},
		{ModeTask, true},
		{ModeRun, true},
	}

	for _, tc := range tests {
		t.Run(string(tc.mode), func(t *testing.T) {
			if got := tc.mode.AllowsWrites(); got != tc.allows {
				t.Errorf("Mode(%q).AllowsWrites() = %v, want %v", tc.mode, got, tc.allows)
			}
		})
	}
}

func TestCanTransition(t *testing.T) {
	tests := []struct {
		from    Mode
		to      Mode
		allowed bool
	}{
		// From Chat
		{ModeChat, ModeTask, true},
		{ModeChat, ModeRun, true},
		{ModeChat, ModeChat, false}, // Same mode

		// From Task
		{ModeTask, ModeChat, true},
		{ModeTask, ModeRun, true},
		{ModeTask, ModeTask, false},

		// From Run
		{ModeRun, ModeChat, true},
		{ModeRun, ModeTask, true},
		{ModeRun, ModeRun, false},
	}

	for _, tc := range tests {
		t.Run(string(tc.from)+"->"+string(tc.to), func(t *testing.T) {
			if got := CanTransition(tc.from, tc.to); got != tc.allowed {
				t.Errorf("CanTransition(%q, %q) = %v, want %v", tc.from, tc.to, got, tc.allowed)
			}
		})
	}
}

func TestValidateTransition(t *testing.T) {
	tests := []struct {
		from      Mode
		to        Mode
		condition TransitionCondition
		wantErr   bool
	}{
		// Valid transitions
		{ModeChat, ModeTask, ConditionUserApproved, false},
		{ModeChat, ModeRun, ConditionUserApproved, false},
		{ModeChat, ModeRun, ConditionInputsReady, false},
		{ModeTask, ModeRun, ConditionInputsReady, false},
		{ModeRun, ModeChat, ConditionCompleted, false},
		{ModeRun, ModeChat, ConditionCheckpoint, false},
		{ModeRun, ModeTask, ConditionFailed, false},

		// Invalid transitions
		{ModeChat, ModeTask, ConditionCompleted, true}, // Wrong condition
		{ModeChat, ModeChat, ConditionUserApproved, true}, // Same mode
	}

	for _, tc := range tests {
		t.Run(string(tc.from)+"->"+string(tc.to), func(t *testing.T) {
			err := ValidateTransition(tc.from, tc.to, tc.condition)
			if (err != nil) != tc.wantErr {
				t.Errorf("ValidateTransition() error = %v, wantErr %v", err, tc.wantErr)
			}
		})
	}
}

func TestState_TransitionTo(t *testing.T) {
	t.Run("Valid transition chain", func(t *testing.T) {
		state := NewState()
		if state.Current != ModeChat {
			t.Errorf("initial mode = %s, want chat", state.Current)
		}

		// Chat -> Task
		err := state.TransitionTo(ModeTask, ConditionUserApproved, "User requested task")
		if err != nil {
			t.Fatalf("Chat -> Task failed: %v", err)
		}
		if state.Current != ModeTask {
			t.Errorf("current mode = %s, want task", state.Current)
		}

		// Task -> Run
		err = state.TransitionTo(ModeRun, ConditionInputsReady, "Blueprint ready")
		if err != nil {
			t.Fatalf("Task -> Run failed: %v", err)
		}
		if state.Current != ModeRun {
			t.Errorf("current mode = %s, want run", state.Current)
		}

		// Run -> Chat
		err = state.TransitionTo(ModeChat, ConditionCompleted, "Task completed")
		if err != nil {
			t.Fatalf("Run -> Chat failed: %v", err)
		}
		if state.Current != ModeChat {
			t.Errorf("current mode = %s, want chat", state.Current)
		}

		// Verify history
		if len(state.History) != 3 {
			t.Errorf("history length = %d, want 3", len(state.History))
		}
	})

	t.Run("Invalid transition rejected", func(t *testing.T) {
		state := NewState()

		// Try Chat -> Run with wrong condition
		err := state.TransitionTo(ModeRun, ConditionCompleted, "Invalid")
		if err == nil {
			t.Error("expected error for invalid condition")
		}
		if state.Current != ModeChat {
			t.Error("mode should not have changed")
		}
	})
}

func TestState_LastTransition(t *testing.T) {
	state := NewState()

	if state.LastTransition() != nil {
		t.Error("expected nil for no transitions")
	}

	state.TransitionTo(ModeTask, ConditionUserApproved, "First")
	state.TransitionTo(ModeRun, ConditionInputsReady, "Second")

	last := state.LastTransition()
	if last == nil {
		t.Fatal("expected non-nil last transition")
	}
	if last.From != ModeTask || last.To != ModeRun {
		t.Errorf("last transition = %s->%s, want task->run", last.From, last.To)
	}
}
