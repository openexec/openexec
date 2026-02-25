package pipeline

import "testing"

func TestStateMachineHappyPath(t *testing.T) {
	sm := NewStateMachine(DefaultPhaseOrder(), DefaultPhaseConfigs(), 3)

	// TD → IM
	if sm.Current() != PhaseTD {
		t.Fatalf("expected TD, got %s", sm.Current())
	}
	next, err := sm.Advance()
	if err != nil {
		t.Fatalf("Advance from TD: %v", err)
	}
	if next != PhaseIM {
		t.Fatalf("expected IM, got %s", next)
	}

	// IM → RV
	next, err = sm.Advance()
	if err != nil {
		t.Fatalf("Advance from IM: %v", err)
	}
	if next != PhaseRV {
		t.Fatalf("expected RV, got %s", next)
	}

	// RV → RF via Route("hon")
	next, err = sm.Route("hon")
	if err != nil {
		t.Fatalf("Route(hon): %v", err)
	}
	if next != PhaseRF {
		t.Fatalf("expected RF, got %s", next)
	}

	// RF → FL
	next, err = sm.Advance()
	if err != nil {
		t.Fatalf("Advance from RF: %v", err)
	}
	if next != PhaseFL {
		t.Fatalf("expected FL, got %s", next)
	}

	// FL → Done
	next, err = sm.Advance()
	if err != nil {
		t.Fatalf("Advance from FL: %v", err)
	}
	if next != PhaseDone {
		t.Fatalf("expected Done, got %s", next)
	}
}

func TestStateMachineReviewLoop(t *testing.T) {
	sm := NewStateMachine(DefaultPhaseOrder(), DefaultPhaseConfigs(), 3)

	// Advance to RV.
	sm.Advance() // TD → IM
	sm.Advance() // IM → RV

	// Route to spark (back to IM).
	next, err := sm.Route("spark")
	if err != nil {
		t.Fatalf("Route(spark): %v", err)
	}
	if next != PhaseIM {
		t.Fatalf("expected IM, got %s", next)
	}
	if sm.ReviewCycles() != 1 {
		t.Fatalf("expected 1 review cycle, got %d", sm.ReviewCycles())
	}

	// IM → RV again.
	sm.Advance() // IM → RV

	// Route to spark again.
	next, err = sm.Route("spark")
	if err != nil {
		t.Fatalf("Route(spark) 2nd: %v", err)
	}
	if next != PhaseIM {
		t.Fatalf("expected IM, got %s", next)
	}
	if sm.ReviewCycles() != 2 {
		t.Fatalf("expected 2 review cycles, got %d", sm.ReviewCycles())
	}

	// Complete the pipeline via hon.
	sm.Advance() // IM → RV
	next, err = sm.Route("hon")
	if err != nil {
		t.Fatalf("Route(hon): %v", err)
	}
	if next != PhaseRF {
		t.Fatalf("expected RF, got %s", next)
	}
}

func TestStateMachineMaxReviewCycles(t *testing.T) {
	sm := NewStateMachine(DefaultPhaseOrder(), DefaultPhaseConfigs(), 2)

	sm.Advance() // TD → IM
	sm.Advance() // IM → RV

	// First cycle.
	sm.Route("spark")
	sm.Advance() // IM → RV

	// Second cycle.
	sm.Route("spark")
	sm.Advance() // IM → RV

	// Third cycle should fail (max is 2).
	_, err := sm.Route("spark")
	if err == nil {
		t.Fatal("expected error for exceeding max review cycles")
	}
}

func TestStateMachineAdvanceOnDone(t *testing.T) {
	sm := NewStateMachine(DefaultPhaseOrder(), DefaultPhaseConfigs(), 3)

	sm.Advance() // TD → IM
	sm.Advance() // IM → RV
	sm.Route("hon")
	sm.Advance() // RF → FL
	sm.Advance() // FL → Done

	_, err := sm.Advance()
	if err == nil {
		t.Fatal("expected error when advancing from Done")
	}
}

func TestStateMachineAdvanceOnRoutingPhase(t *testing.T) {
	sm := NewStateMachine(DefaultPhaseOrder(), DefaultPhaseConfigs(), 3)

	sm.Advance() // TD → IM
	sm.Advance() // IM → RV

	_, err := sm.Advance()
	if err == nil {
		t.Fatal("expected error when advancing from a phase with routes")
	}
}

func TestStateMachineRouteOnNonRoutingPhase(t *testing.T) {
	sm := NewStateMachine(DefaultPhaseOrder(), DefaultPhaseConfigs(), 3)

	_, err := sm.Route("spark")
	if err == nil {
		t.Fatal("expected error when routing from a phase without routes")
	}
}

func TestStateMachineRouteInvalidTarget(t *testing.T) {
	sm := NewStateMachine(DefaultPhaseOrder(), DefaultPhaseConfigs(), 3)

	sm.Advance() // TD → IM
	sm.Advance() // IM → RV

	_, err := sm.Route("invalid")
	if err == nil {
		t.Fatal("expected error for invalid route target")
	}
}

func TestStateMachineCurrentConfig(t *testing.T) {
	phases := DefaultPhaseConfigs()
	sm := NewStateMachine(DefaultPhaseOrder(), phases, 3)

	cfg, ok := sm.CurrentConfig()
	if !ok {
		t.Fatal("CurrentConfig should return true for TD")
	}
	if cfg.Agent != "clario" {
		t.Errorf("expected agent clario, got %s", cfg.Agent)
	}
	if cfg.Workflow != "technical-design" {
		t.Errorf("expected workflow technical-design, got %s", cfg.Workflow)
	}
}

func TestDefaultPhaseConfigs(t *testing.T) {
	configs := DefaultPhaseConfigs()

	expected := map[Phase]struct{ agent, workflow string }{
		PhaseTD: {"clario", "technical-design"},
		PhaseIM: {"spark", "implement"},
		PhaseRV: {"blade", "review"},
		PhaseRF: {"hon", "refactor"},
		PhaseFL: {"clario", "feedback-loop"},
	}

	for phase, want := range expected {
		cfg, ok := configs[phase]
		if !ok {
			t.Errorf("missing config for phase %s", phase)
			continue
		}
		if cfg.Agent != want.agent {
			t.Errorf("phase %s: agent = %s, want %s", phase, cfg.Agent, want.agent)
		}
		if cfg.Workflow != want.workflow {
			t.Errorf("phase %s: workflow = %s, want %s", phase, cfg.Workflow, want.workflow)
		}
	}

	// RV should have routes.
	rvCfg := configs[PhaseRV]
	if len(rvCfg.Routes) != 2 {
		t.Errorf("RV routes count = %d, want 2", len(rvCfg.Routes))
	}
	if rvCfg.Routes["spark"] != PhaseIM {
		t.Errorf("RV route spark = %s, want IM", rvCfg.Routes["spark"])
	}
	if rvCfg.Routes["hon"] != PhaseRF {
		t.Errorf("RV route hon = %s, want RF", rvCfg.Routes["hon"])
	}
}

func TestStateMachineCustomOrder(t *testing.T) {
	// Custom 3-phase pipeline: A → B → C, B has routes.
	order := []Phase{"A", "B", "C"}
	phases := map[Phase]PhaseConfig{
		"A": {Agent: "agent-a", Workflow: "wf-a"},
		"B": {Agent: "agent-b", Workflow: "wf-b", Routes: map[string]Phase{"redo": "A", "done": "C"}},
		"C": {Agent: "agent-c", Workflow: "wf-c"},
	}

	sm := NewStateMachine(order, phases, 2)

	if sm.Current() != "A" {
		t.Fatalf("expected A, got %s", sm.Current())
	}

	// A → B
	next, err := sm.Advance()
	if err != nil {
		t.Fatalf("Advance: %v", err)
	}
	if next != "B" {
		t.Fatalf("expected B, got %s", next)
	}

	// B: can't advance (has routes).
	_, err = sm.Advance()
	if err == nil {
		t.Fatal("expected error advancing from B")
	}

	// B → A (backward route, counts as review cycle).
	next, err = sm.Route("redo")
	if err != nil {
		t.Fatalf("Route(redo): %v", err)
	}
	if next != "A" {
		t.Fatalf("expected A, got %s", next)
	}
	if sm.ReviewCycles() != 1 {
		t.Errorf("review cycles = %d, want 1", sm.ReviewCycles())
	}

	// A → B again.
	sm.Advance()

	// B → C (forward route, no cycle increment).
	next, err = sm.Route("done")
	if err != nil {
		t.Fatalf("Route(done): %v", err)
	}
	if next != "C" {
		t.Fatalf("expected C, got %s", next)
	}
	if sm.ReviewCycles() != 1 {
		t.Errorf("review cycles after forward route = %d, want 1", sm.ReviewCycles())
	}

	// C → Done
	next, err = sm.Advance()
	if err != nil {
		t.Fatalf("Advance from C: %v", err)
	}
	if next != PhaseDone {
		t.Fatalf("expected Done, got %s", next)
	}
}

func TestPipelineDefPhaseOrder(t *testing.T) {
	def := DefaultPipelineDef()
	order := def.PhaseOrder()

	expected := []Phase{PhaseTD, PhaseIM, PhaseRV, PhaseRF, PhaseFL}
	if len(order) != len(expected) {
		t.Fatalf("order length = %d, want %d", len(order), len(expected))
	}
	for i, p := range expected {
		if order[i] != p {
			t.Errorf("order[%d] = %s, want %s", i, order[i], p)
		}
	}
}

func TestPipelineDefPhaseConfigs(t *testing.T) {
	def := DefaultPipelineDef()
	configs := def.PhaseConfigs()

	if len(configs) != 5 {
		t.Fatalf("configs count = %d, want 5", len(configs))
	}

	rvCfg := configs[PhaseRV]
	if rvCfg.Agent != "blade" {
		t.Errorf("RV agent = %s, want blade", rvCfg.Agent)
	}
	if len(rvCfg.Routes) != 2 {
		t.Errorf("RV routes = %d, want 2", len(rvCfg.Routes))
	}
}
