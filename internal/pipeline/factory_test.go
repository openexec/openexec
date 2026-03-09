package pipeline

import (
	"os"
	"testing"
	"time"
)

func TestFactoryCreateValid(t *testing.T) {
	cfg := LoopFactoryConfig{
		FWUID:                "FWU-01",
		WorkDir:              t.TempDir(),
		AgentsFS:             os.DirFS("testdata"),
		DefaultMaxIterations: 10,
		MaxRetries:           3,
		RetryBackoff:         []time.Duration{0},
		ThrashThreshold:      3,
		CommandName:          "echo",
		CommandArgs:          []string{"test"},
	}
	factory := NewLoopFactory(cfg)

	phaseCfg := PhaseConfig{
		Agent:    "test-agent",
		Workflow: "technical-design",
	}

	l, ch, err := factory.Create("## Briefing\nTest briefing", phaseCfg)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if l == nil {
		t.Fatal("expected non-nil Loop")
	}
	if ch == nil {
		t.Fatal("expected non-nil channel")
	}
}

func TestFactoryCreateUnknownAgent(t *testing.T) {
	cfg := LoopFactoryConfig{
		AgentsFS:             os.DirFS("testdata"),
		DefaultMaxIterations: 10,
	}
	factory := NewLoopFactory(cfg)

	phaseCfg := PhaseConfig{
		Agent:    "nonexistent",
		Workflow: "technical-design",
	}

	_, _, err := factory.Create("", phaseCfg)
	if err == nil {
		t.Fatal("expected error for unknown agent")
	}
}

func TestFactoryCreateUnknownWorkflow(t *testing.T) {
	cfg := LoopFactoryConfig{
		AgentsFS:             os.DirFS("testdata"),
		DefaultMaxIterations: 10,
	}
	factory := NewLoopFactory(cfg)

	phaseCfg := PhaseConfig{
		Agent:    "test-agent",
		Workflow: "nonexistent-workflow",
	}

	_, _, err := factory.Create("", phaseCfg)
	if err == nil {
		t.Fatal("expected error for unknown workflow")
	}
}

func TestFactoryPhaseMaxIterationsOverride(t *testing.T) {
	cfg := LoopFactoryConfig{
		AgentsFS:             os.DirFS("testdata"),
		DefaultMaxIterations: 10,
		MaxRetries:           3,
		RetryBackoff:         []time.Duration{0},
		CommandName:          "echo",
		CommandArgs:          []string{"test"},
	}
	factory := NewLoopFactory(cfg)

	// Phase with custom MaxIterations.
	phaseCfg := PhaseConfig{
		Agent:         "test-agent",
		Workflow:      "implement",
		MaxIterations: 5,
	}

	l, _, err := factory.Create("", phaseCfg)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if l == nil {
		t.Fatal("expected non-nil Loop")
	}
	// We can't directly inspect the Loop's config, but we verified it compiles
	// and creates successfully. Integration tests will verify iteration limits.
}

func TestFactoryPhaseMaxIterationsDefault(t *testing.T) {
	cfg := LoopFactoryConfig{
		AgentsFS:             os.DirFS("testdata"),
		DefaultMaxIterations: 10,
		MaxRetries:           3,
		RetryBackoff:         []time.Duration{0},
		CommandName:          "echo",
		CommandArgs:          []string{"test"},
	}
	factory := NewLoopFactory(cfg)

	// Phase with 0 MaxIterations = use default.
	phaseCfg := PhaseConfig{
		Agent:         "test-agent",
		Workflow:      "review",
		MaxIterations: 0,
	}

	l, _, err := factory.Create("", phaseCfg)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if l == nil {
		t.Fatal("expected non-nil Loop")
	}
}

func TestFactoryPhaseCommandArgsOverride(t *testing.T) {
	cfg := LoopFactoryConfig{
		AgentsFS:             os.DirFS("testdata"),
		DefaultMaxIterations: 10,
		CommandName:          "mock_claude",
		CommandArgs:          []string{"default-scenario"},
	}
	factory := NewLoopFactory(cfg)

	// Phase with custom CommandArgs should override factory default.
	phaseCfg := PhaseConfig{
		Agent:       "test-agent",
		Workflow:    "refactor",
		CommandArgs: []string{"phase-specific-scenario"},
	}

	l, _, err := factory.Create("", phaseCfg)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if l == nil {
		t.Fatal("expected non-nil Loop")
	}
}

func TestFactoryAllWorkflows(t *testing.T) {
	cfg := LoopFactoryConfig{
		AgentsFS:             os.DirFS("testdata"),
		DefaultMaxIterations: 10,
		CommandName:          "echo",
		CommandArgs:          []string{"test"},
	}
	factory := NewLoopFactory(cfg)

	workflows := []string{"technical-design", "implement", "review", "refactor", "feedback-loop"}
	for _, wf := range workflows {
		phaseCfg := PhaseConfig{
			Agent:    "test-agent",
			Workflow: wf,
		}
		l, ch, err := factory.Create("briefing text", phaseCfg)
		if err != nil {
			t.Errorf("Create(%s): %v", wf, err)
			continue
		}
		if l == nil || ch == nil {
			t.Errorf("Create(%s): nil loop or channel", wf)
		}
	}
}
