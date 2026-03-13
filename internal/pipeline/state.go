package pipeline

import "fmt"

// Phase represents a stage in the FWU pipeline.
type Phase string

const (
	PhaseTD   Phase = "TD"
	PhaseIM   Phase = "IM"
	PhaseRV   Phase = "RV"
	PhaseRF   Phase = "RF"
	PhaseFL   Phase = "FL"
	PhaseDone Phase = "Done"
)

// PhaseConfig defines settings for a single phase.
type PhaseConfig struct {
	Agent         string           // agent definition name
	Workflow      string           // workflow prompt ID
	MaxIterations int              // phase-specific limit (0 = use pipeline default)
	Routes        map[string]Phase // signal target → destination phase
	CommandArgs   []string         // override for testing (nil = use factory default)
}

// DefaultPhaseConfigs returns the standard phase configuration for a full pipeline run.
func DefaultPhaseConfigs() map[Phase]PhaseConfig {
	return map[Phase]PhaseConfig{
		PhaseTD: {Agent: "clario", Workflow: "technical-design"},
		PhaseIM: {Agent: "spark", Workflow: "implement"},
		PhaseRV: {Agent: "blade", Workflow: "review", Routes: map[string]Phase{"spark": PhaseIM, "hon": PhaseRF}},
		PhaseRF: {Agent: "hon", Workflow: "refactor"},
		PhaseFL: {Agent: "clario", Workflow: "feedback-loop"},
	}
}

// DefaultPhaseOrder returns the standard linear phase ordering.
func DefaultPhaseOrder() []Phase {
	return []Phase{PhaseTD, PhaseIM, PhaseRV, PhaseRF, PhaseFL}
}

// StateMachine manages phase transitions for a pipeline run.
type StateMachine struct {
	phases          map[Phase]PhaseConfig
	order           []Phase
	current         Phase
	reviewCycles    int
	maxReviewCycles int
}

// NewStateMachine creates a state machine with the given phase order, configs, and review cycle limit.
func NewStateMachine(order []Phase, phases map[Phase]PhaseConfig, maxReviewCycles int) *StateMachine {
	return &StateMachine{
		phases:          phases,
		order:           order,
		current:         order[0],
		maxReviewCycles: maxReviewCycles,
	}
}

// Current returns the current phase.
func (sm *StateMachine) Current() Phase {
	return sm.current
}

// CurrentConfig returns the PhaseConfig for the current phase.
func (sm *StateMachine) CurrentConfig() (PhaseConfig, bool) {
	cfg, ok := sm.phases[sm.current]
	return cfg, ok
}

// ReviewCycles returns the number of backward routing cycles completed.
func (sm *StateMachine) ReviewCycles() int {
	return sm.reviewCycles
}

// Advance moves to the next phase in the linear sequence.
// Returns an error if the current phase has routes defined (use Route instead),
// the pipeline is done, or the current phase is unknown.
func (sm *StateMachine) Advance() (Phase, error) {
	if sm.current == PhaseDone {
		return PhaseDone, fmt.Errorf("pipeline already done")
	}
	if cfg, ok := sm.phases[sm.current]; ok && len(cfg.Routes) > 0 {
		return sm.current, fmt.Errorf("cannot advance from %s: use Route to specify target", sm.current)
	}
	return sm.advanceLinear()
}

// Route handles a routing decision from the current phase.
// The target must match a key in the current phase's Routes map.
// Backward routes (destination earlier in order) increment the review cycle counter.
func (sm *StateMachine) Route(target string) (Phase, error) {
	cfg, ok := sm.phases[sm.current]
	if !ok || len(cfg.Routes) == 0 {
		return sm.current, fmt.Errorf("Route not valid from phase %s: no routes defined", sm.current)
	}

	dest, ok := cfg.Routes[target]
	if !ok {
		return sm.current, fmt.Errorf("invalid route target %q from phase %s", target, sm.current)
	}

	// Backward route: destination appears earlier in order.
	if sm.phaseIndex(dest) <= sm.phaseIndex(sm.current) {
		if sm.reviewCycles >= sm.maxReviewCycles {
			return sm.current, fmt.Errorf("max review cycles (%d) reached", sm.maxReviewCycles)
		}
		sm.reviewCycles++
	}

	sm.current = dest
	return sm.current, nil
}

// JumpTo forces the state machine to a specific phase, bypassing normal transitions.
func (sm *StateMachine) JumpTo(phase Phase) error {
	if phase == PhaseDone {
		sm.current = PhaseDone
		return nil
	}
	// Verify phase exists in config
	found := false
	for _, p := range sm.order {
		if p == phase {
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("cannot jump to unknown phase %s", phase)
	}
	sm.current = phase
	return nil
}

// advanceLinear moves to the next phase in order, skipping route checks.
func (sm *StateMachine) advanceLinear() (Phase, error) {
	next, err := sm.nextPhase(sm.current)
	if err != nil {
		return sm.current, err
	}
	sm.current = next
	return sm.current, nil
}

// phaseIndex returns the index of the given phase in the order, or -1 if not found.
func (sm *StateMachine) phaseIndex(p Phase) int {
	for i, phase := range sm.order {
		if phase == p {
			return i
		}
	}
	return -1
}

// nextPhase returns the phase after the given phase in the linear sequence.
func (sm *StateMachine) nextPhase(p Phase) (Phase, error) {
	for i, phase := range sm.order {
		if phase == p {
			if i+1 < len(sm.order) {
				return sm.order[i+1], nil
			}
			return PhaseDone, nil
		}
	}
	return PhaseDone, fmt.Errorf("unknown phase: %s", p)
}
