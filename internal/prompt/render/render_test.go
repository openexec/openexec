package render

import (
	"strings"
	"testing"
)

const testdataDir = "../testdata"

func TestRenderAgentBasic(t *testing.T) {
	result, err := RenderAgent(testdataDir, "clario")
	if err != nil {
		t.Fatalf("RenderAgent: %v", err)
	}

	// YAML frontmatter.
	if !strings.Contains(result, `name: "clario"`) {
		t.Error("missing name in frontmatter")
	}
	if !strings.Contains(result, `description: "Architecture Agent`) {
		t.Error("missing description in frontmatter")
	}

	// Preamble.
	if !strings.Contains(result, "You must fully embody this agent") {
		t.Error("missing preamble")
	}

	// XML code block.
	if !strings.Contains(result, "```xml") {
		t.Error("missing xml code block opening")
	}
	if !strings.HasSuffix(strings.TrimSpace(result), "```") {
		t.Error("missing xml code block closing")
	}

	// Agent tag.
	if !strings.Contains(result, `<agent id="clario.agent.yaml"`) {
		t.Error("missing agent tag with id")
	}
	if !strings.Contains(result, `name="Clario"`) {
		t.Error("missing title-cased name in agent tag")
	}
}

func TestRenderAgentActivation(t *testing.T) {
	result, err := RenderAgent(testdataDir, "clario")
	if err != nil {
		t.Fatalf("RenderAgent: %v", err)
	}

	if !strings.Contains(result, `<activation critical="MANDATORY">`) {
		t.Error("missing activation block")
	}
	if !strings.Contains(result, "apexflow:agents:clario/config.yaml") {
		t.Error("activation block missing agent-specific config path")
	}
	if !strings.Contains(result, "</activation>") {
		t.Error("missing activation closing tag")
	}
	if !strings.Contains(result, "<menu-handlers>") {
		t.Error("missing menu-handlers in activation")
	}
	if !strings.Contains(result, "<rules>") {
		t.Error("missing rules in activation")
	}
}

func TestRenderAgentPersona(t *testing.T) {
	result, err := RenderAgent(testdataDir, "clario")
	if err != nil {
		t.Fatalf("RenderAgent: %v", err)
	}

	if !strings.Contains(result, "<persona>") {
		t.Error("missing persona section")
	}
	if !strings.Contains(result, "<role>") {
		t.Error("missing role tag")
	}
	if !strings.Contains(result, "Architecture specialist") {
		t.Error("missing role content")
	}
	if !strings.Contains(result, "<identity>") {
		t.Error("missing identity tag")
	}
	if !strings.Contains(result, "<communication_style>") {
		t.Error("missing communication_style tag")
	}
	if !strings.Contains(result, "<principles>") {
		t.Error("missing principles tag")
	}
}

func TestRenderAgentMergesBasePrinciples(t *testing.T) {
	result, err := RenderAgent(testdataDir, "clario")
	if err != nil {
		t.Fatalf("RenderAgent: %v", err)
	}

	// Base principles.
	if !strings.Contains(result, "Team objectives matter more than ego") {
		t.Error("missing base principles")
	}
	// Agent-specific principles.
	if !strings.Contains(result, "I turn intent into technical clarity") {
		t.Error("missing agent-specific principles")
	}
}

func TestRenderAgentPrompts(t *testing.T) {
	result, err := RenderAgent(testdataDir, "clario")
	if err != nil {
		t.Fatalf("RenderAgent: %v", err)
	}

	if !strings.Contains(result, "<prompts>") {
		t.Error("missing prompts section")
	}

	// All three workflows should be present.
	if !strings.Contains(result, `<prompt id="technical-design">`) {
		t.Error("missing technical-design prompt")
	}
	if !strings.Contains(result, `<prompt id="feedback-loop">`) {
		t.Error("missing feedback-loop prompt")
	}
	if !strings.Contains(result, `<prompt id="mutation-testing">`) {
		t.Error("missing mutation-testing prompt")
	}

	// Workflow content.
	if !strings.Contains(result, "Execute full technical design workflow") {
		t.Error("missing technical-design instructions")
	}
	if !strings.Contains(result, "gap analysis") {
		t.Error("missing technical-design process content")
	}
}

func TestRenderAgentExpandsParams(t *testing.T) {
	result, err := RenderAgent(testdataDir, "clario")
	if err != nil {
		t.Fatalf("RenderAgent: %v", err)
	}

	// mutation-testing workflow should have expanded params.
	if !strings.Contains(result, "Validate test quality for architecture") {
		t.Error("missing expanded intent param in mutation-testing")
	}
	if !strings.Contains(result, "mutation score meets standards") {
		t.Error("missing expanded stopping_criteria param in mutation-testing")
	}

	// No unexpanded placeholders.
	if strings.Contains(result, "{{") {
		t.Error("result still contains unexpanded placeholders")
	}
}

func TestRenderAgentMenu(t *testing.T) {
	result, err := RenderAgent(testdataDir, "clario")
	if err != nil {
		t.Fatalf("RenderAgent: %v", err)
	}

	if !strings.Contains(result, "<menu>") {
		t.Error("missing menu section")
	}

	// Standard items.
	if !strings.Contains(result, "[MH] Redisplay Menu Help") {
		t.Error("missing MH menu item")
	}
	if !strings.Contains(result, "[CH] Chat with the Agent") {
		t.Error("missing CH menu item")
	}
	if !strings.Contains(result, "[DA] Dismiss Agent") {
		t.Error("missing DA menu item")
	}

	// Workflow items with action references.
	if !strings.Contains(result, `action="#technical-design"`) {
		t.Error("missing technical-design action in menu")
	}
	if !strings.Contains(result, `action="#feedback-loop"`) {
		t.Error("missing feedback-loop action in menu")
	}
	if !strings.Contains(result, `action="#mutation-testing"`) {
		t.Error("missing mutation-testing action in menu")
	}
}

func TestRenderAgentSectionOrder(t *testing.T) {
	result, err := RenderAgent(testdataDir, "clario")
	if err != nil {
		t.Fatalf("RenderAgent: %v", err)
	}

	activationIdx := strings.Index(result, "<activation")
	personaIdx := strings.Index(result, "<persona>")
	promptsIdx := strings.Index(result, "<prompts>")
	menuIdx := strings.Index(result, "<menu>")

	if activationIdx < 0 || personaIdx < 0 || promptsIdx < 0 || menuIdx < 0 {
		t.Fatal("one or more sections not found")
	}

	if activationIdx >= personaIdx {
		t.Error("activation should come before persona")
	}
	if personaIdx >= promptsIdx {
		t.Error("persona should come before prompts")
	}
	if promptsIdx >= menuIdx {
		t.Error("prompts should come before menu")
	}
}

func TestRenderAgentXMLEscaping(t *testing.T) {
	result, err := RenderAgent(testdataDir, "clario")
	if err != nil {
		t.Fatalf("RenderAgent: %v", err)
	}

	// Clario's communication_style contains quotes that should be escaped.
	// "The path is clear. Build well." in the persona file.
	if !strings.Contains(result, "&quot;The path is clear. Build well.&quot;") {
		t.Error("quotes in communication_style not XML-escaped")
	}
}

func TestRenderAgentUnknown(t *testing.T) {
	_, err := RenderAgent(testdataDir, "nonexistent")
	if err == nil {
		t.Error("expected error for unknown agent, got nil")
	}
	if !strings.Contains(err.Error(), "nonexistent") {
		t.Errorf("error should mention agent name, got: %v", err)
	}
}

func TestCmdAbbrev(t *testing.T) {
	tests := []struct {
		id   string
		want string
	}{
		{"implement", "IM"},
		{"refactor", "RE"},
		{"rubber-duck", "RD"},
		{"red-green-cycle", "RGC"},
		{"mutation-testing", "MT"},
		{"acceptance-validation", "AV"},
		{"coverage-check", "CC"},
		{"technical-design", "TD"},
		{"feedback-loop", "FL"},
	}

	for _, tt := range tests {
		got := cmdAbbrev(tt.id)
		if got != tt.want {
			t.Errorf("cmdAbbrev(%q) = %q, want %q", tt.id, got, tt.want)
		}
	}
}
