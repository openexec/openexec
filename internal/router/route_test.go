package router

import (
	"context"
	"testing"

	"github.com/openexec/openexec/internal/mode"
	"github.com/openexec/openexec/internal/toolset"
)

func TestRoute_ReturnsValidPlan(t *testing.T) {
	r := NewDeterministicRouter()
	registry := toolset.NewRegistry()
	plan, err := Route(context.Background(), r, "help me with this code", registry)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if plan == nil {
		t.Fatal("expected non-nil RoutingPlan")
	}
	if !plan.Mode.IsValid() {
		t.Errorf("expected valid mode, got %q", plan.Mode)
	}
	if plan.Toolset == "" {
		t.Error("expected non-empty toolset")
	}
	if plan.Confidence <= 0 || plan.Confidence > 1.0 {
		t.Errorf("expected confidence in (0, 1.0], got %.2f", plan.Confidence)
	}
}

func TestRoute_ModeClassification_Deploy(t *testing.T) {
	r := NewDeterministicRouter()
	registry := toolset.NewRegistry()
	plan, err := Route(context.Background(), r, "deploy the application to production", registry)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if plan.Mode != mode.ModeRun {
		t.Errorf("expected mode 'run' for deploy query, got %q", plan.Mode)
	}
}

func TestRoute_ModeClassification_Help(t *testing.T) {
	r := NewDeterministicRouter()
	registry := toolset.NewRegistry()
	plan, err := Route(context.Background(), r, "what is the purpose of this function", registry)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if plan.Mode != mode.ModeChat {
		t.Errorf("expected mode 'chat' for help query, got %q", plan.Mode)
	}
}

func TestRoute_ModeClassification_Fix(t *testing.T) {
	r := NewDeterministicRouter()
	registry := toolset.NewRegistry()
	plan, err := Route(context.Background(), r, "fix the broken test in auth module", registry)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if plan.Mode != mode.ModeTask {
		t.Errorf("expected mode 'task' for fix query, got %q", plan.Mode)
	}
}

func TestRoute_RepoZoneIdentification(t *testing.T) {
	r := NewDeterministicRouter()
	registry := toolset.NewRegistry()
	plan, err := Route(context.Background(), r, "update the api endpoint for users", registry)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	found := false
	for _, zone := range plan.RepoZones {
		if zone == "internal/api" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected 'internal/api' in repo zones, got %v", plan.RepoZones)
	}
}

func TestRoute_SensitivityDetection_High(t *testing.T) {
	r := NewDeterministicRouter()
	registry := toolset.NewRegistry()
	plan, err := Route(context.Background(), r, "update the password handling", registry)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if plan.Sensitivity != SensitivityHigh {
		t.Errorf("expected sensitivity 'high' for password query, got %q", plan.Sensitivity)
	}
}

func TestRoute_SensitivityDetection_Medium(t *testing.T) {
	r := NewDeterministicRouter()
	registry := toolset.NewRegistry()
	plan, err := Route(context.Background(), r, "update the email template", registry)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if plan.Sensitivity != SensitivityMedium {
		t.Errorf("expected sensitivity 'medium' for email query, got %q", plan.Sensitivity)
	}
}

func TestRoute_SensitivityDetection_Low(t *testing.T) {
	r := NewDeterministicRouter()
	registry := toolset.NewRegistry()
	plan, err := Route(context.Background(), r, "hello world", registry)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if plan.Sensitivity != SensitivityLow {
		t.Errorf("expected sensitivity 'low' for hello query, got %q", plan.Sensitivity)
	}
}

func TestRoute_NeedsFrontier_HighSensitivity(t *testing.T) {
	r := NewDeterministicRouter()
	registry := toolset.NewRegistry()
	plan, err := Route(context.Background(), r, "rotate the api secret key", registry)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !plan.NeedsFrontier {
		t.Error("expected NeedsFrontier=true for high sensitivity query")
	}
}

func TestRoute_NeedsFrontier_RunMode(t *testing.T) {
	r := NewDeterministicRouter()
	registry := toolset.NewRegistry()
	plan, err := Route(context.Background(), r, "implement the new feature end to end", registry)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !plan.NeedsFrontier {
		t.Error("expected NeedsFrontier=true for run mode query")
	}
}

func TestRoute_NilRegistry(t *testing.T) {
	r := NewDeterministicRouter()
	plan, err := Route(context.Background(), r, "hello", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if plan.Toolset != "repo_readonly" {
		t.Errorf("expected default toolset 'repo_readonly' with nil registry, got %q", plan.Toolset)
	}
}

func TestClassifyMode_Defaults(t *testing.T) {
	// Unknown query with general_chat intent defaults to chat
	intent := &Intent{ToolName: GeneralChatTool, Confidence: 0.5}
	m := classifyMode("xyzzy", intent)
	if m != mode.ModeChat {
		t.Errorf("expected chat mode for unknown query with general_chat intent, got %q", m)
	}

	// Unknown query with specific tool intent defaults to task
	intent2 := &Intent{ToolName: "some_tool", Confidence: 0.7}
	m2 := classifyMode("xyzzy", intent2)
	if m2 != mode.ModeTask {
		t.Errorf("expected task mode for unknown query with specific tool intent, got %q", m2)
	}
}

func TestKnowledgeSources_Default(t *testing.T) {
	sources := rankKnowledgeSources("hello world")
	if len(sources) == 0 {
		t.Error("expected at least default knowledge sources")
	}
	// Default should include code_symbols and local_docs
	hasSymbols := false
	hasDocs := false
	for _, s := range sources {
		if s == "code_symbols" {
			hasSymbols = true
		}
		if s == "local_docs" {
			hasDocs = true
		}
	}
	if !hasSymbols || !hasDocs {
		t.Errorf("expected default sources [code_symbols, local_docs], got %v", sources)
	}
}

func TestKnowledgeSources_Test(t *testing.T) {
	sources := rankKnowledgeSources("run the test suite")
	hasTests := false
	for _, s := range sources {
		if s == "test_files" {
			hasTests = true
		}
	}
	if !hasTests {
		t.Errorf("expected 'test_files' in sources for test query, got %v", sources)
	}
}
