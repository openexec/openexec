package router

import (
	"context"
	"testing"
)

func TestDeterministicRouter_ParseIntent_Deploy(t *testing.T) {
	r := NewDeterministicRouter()
	intent, err := r.ParseIntent(context.Background(), "deploy to prod")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if intent.ToolName != "deploy" {
		t.Errorf("expected tool_name 'deploy', got %q", intent.ToolName)
	}
	if intent.Confidence != 0.7 {
		t.Errorf("expected confidence 0.7, got %.2f", intent.Confidence)
	}
}

func TestDeterministicRouter_ParseIntent_Implement(t *testing.T) {
	r := NewDeterministicRouter()
	intent, err := r.ParseIntent(context.Background(), "implement the new feature")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if intent.ToolName != "run_shell_command" {
		t.Errorf("expected tool_name 'run_shell_command', got %q", intent.ToolName)
	}
	if intent.Confidence != 0.7 {
		t.Errorf("expected confidence 0.7, got %.2f", intent.Confidence)
	}
}

func TestDeterministicRouter_ParseIntent_Help(t *testing.T) {
	r := NewDeterministicRouter()
	intent, err := r.ParseIntent(context.Background(), "help me understand this code")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if intent.ToolName != GeneralChatTool {
		t.Errorf("expected tool_name %q, got %q", GeneralChatTool, intent.ToolName)
	}
	if intent.Confidence < 0.5 || intent.Confidence > 0.8 {
		t.Errorf("expected confidence in [0.5, 0.8], got %.2f", intent.Confidence)
	}
}

func TestDeterministicRouter_ParseIntent_Unknown(t *testing.T) {
	r := NewDeterministicRouter()
	intent, err := r.ParseIntent(context.Background(), "xyzzy plugh")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if intent.ToolName != GeneralChatTool {
		t.Errorf("expected fallback to %q, got %q", GeneralChatTool, intent.ToolName)
	}
	if intent.Confidence != FallbackConfidence {
		t.Errorf("expected fallback confidence %.2f, got %.2f", FallbackConfidence, intent.Confidence)
	}
}

func TestDeterministicRouter_ParseIntent_Search(t *testing.T) {
	r := NewDeterministicRouter()
	intent, err := r.ParseIntent(context.Background(), "search for all TODO comments")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if intent.ToolName != "grep" {
		t.Errorf("expected tool_name 'grep', got %q", intent.ToolName)
	}
}

func TestDeterministicRouter_ParseIntent_Explain(t *testing.T) {
	r := NewDeterministicRouter()
	intent, err := r.ParseIntent(context.Background(), "explain how the router works")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if intent.ToolName != GeneralChatTool {
		t.Errorf("expected tool_name %q, got %q", GeneralChatTool, intent.ToolName)
	}
	if intent.Confidence != 0.7 {
		t.Errorf("expected confidence 0.7, got %.2f", intent.Confidence)
	}
}

func TestDeterministicRouter_RegisterTool(t *testing.T) {
	r := NewDeterministicRouter()
	err := r.RegisterTool("my_tool", "does things", `{"type":"object"}`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := r.tools["my_tool"]; !ok {
		t.Error("expected tool to be registered")
	}
}

func TestDeterministicRouter_ConfidenceRange(t *testing.T) {
	r := NewDeterministicRouter()
	queries := []string{
		"deploy to staging",
		"explain the architecture",
		"completely unknown gibberish zxcvbn",
		"fix the broken test",
		"search for errors in logs",
	}

	for _, q := range queries {
		intent, err := r.ParseIntent(context.Background(), q)
		if err != nil {
			t.Fatalf("unexpected error for query %q: %v", q, err)
		}
		if intent.Confidence < 0.5 || intent.Confidence > 0.8 {
			t.Errorf("query %q: confidence %.2f outside expected range [0.5, 0.8]", q, intent.Confidence)
		}
	}
}
