package pipeline

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/openexec/openexec/internal/loop"
)

// buildMockClaude compiles the mock_claude test helper from the loop package.
func buildMockClaude(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	bin := filepath.Join(dir, "mock_claude")
	src := filepath.Join("..", "loop", "testdata", "mock_claude.go")

	cmd := exec.Command("go", "build", "-o", bin, src)
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("build mock_claude: %v", err)
	}
	return bin
}

func drainEvents(ch <-chan loop.Event) []loop.Event {
	var events []loop.Event
	for e := range ch {
		events = append(events, e)
	}
	return events
}

func hasEventType(events []loop.Event, typ loop.EventType) bool {
	for _, e := range events {
		if e.Type == typ {
			return true
		}
	}
	return false
}

func countEventType(events []loop.Event, typ loop.EventType) int {
	n := 0
	for _, e := range events {
		if e.Type == typ {
			n++
		}
	}
	return n
}

func mockBriefing() BriefingFunc {
	return func(ctx context.Context, fwuID string) (string, error) {
		return "## FWU Briefing: " + fwuID + "\n\n**Status:** in_progress\n**Intent:** Test intent", nil
	}
}

func mockBriefingCounter() (BriefingFunc, *atomic.Int32) {
	var count atomic.Int32
	fn := func(ctx context.Context, fwuID string) (string, error) {
		count.Add(1)
		return "## FWU Briefing: " + fwuID + "\n\nTest briefing", nil
	}
	return fn, &count
}

// allPhasesConfig returns the standard 5-phase order and configs with the given
// mock scenario for all phases. RV has routes defined.
func allPhasesConfig(scenario string) ([]Phase, map[Phase]PhaseConfig) {
	order := DefaultPhaseOrder()
	phases := map[Phase]PhaseConfig{
		PhaseTD: {Agent: "test-agent", Workflow: "technical-design", CommandArgs: []string{scenario}},
		PhaseIM: {Agent: "test-agent", Workflow: "implement", CommandArgs: []string{scenario}},
		PhaseRV: {Agent: "test-agent", Workflow: "review", CommandArgs: []string{scenario}, Routes: map[string]Phase{"spark": PhaseIM, "hon": PhaseRF}},
		PhaseRF: {Agent: "test-agent", Workflow: "refactor", CommandArgs: []string{scenario}},
		PhaseFL: {Agent: "test-agent", Workflow: "feedback-loop", CommandArgs: []string{scenario}},
	}
	return order, phases
}

func TestPipelineHappyPath(t *testing.T) {
	bin := buildMockClaude(t)
	order, phases := allPhasesConfig("signal-complete")

	cfg := Config{
		FWUID:                "FWU-01",
		WorkDir:              t.TempDir(),
		AgentsDir:            "testdata",
		Order:                order,
		Phases:               phases,
		MaxReviewCycles:      3,
		DefaultMaxIterations: 10,
		MaxRetries:           1,
		RetryBackoff:         []time.Duration{0},
		ThrashThreshold:      0,
		BriefingFunc:         mockBriefing(),
		CommandName:          bin,
	}

	p, ch := New(cfg)

	var events []loop.Event
	done := make(chan struct{})
	go func() {
		events = drainEvents(ch)
		close(done)
	}()

	err := p.Run(context.Background())
	<-done

	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	// Should have 5 phase_start and 5 phase_complete events.
	starts := countEventType(events, loop.EventPhaseStart)
	if starts != 5 {
		t.Errorf("phase_start count = %d, want 5", starts)
	}

	completes := countEventType(events, loop.EventPhaseComplete)
	if completes != 5 {
		t.Errorf("phase_complete count = %d, want 5", completes)
	}

	if !hasEventType(events, loop.EventPipelineComplete) {
		t.Error("missing pipeline_complete event")
	}

	// Verify phase order in phase_start events.
	expectedPhases := []string{"TD", "IM", "RV", "RF", "FL"}
	idx := 0
	for _, e := range events {
		if e.Type == loop.EventPhaseStart {
			if idx >= len(expectedPhases) {
				t.Fatalf("too many phase_start events")
			}
			if e.Phase != expectedPhases[idx] {
				t.Errorf("phase_start[%d].Phase = %q, want %q", idx, e.Phase, expectedPhases[idx])
			}
			idx++
		}
	}
}

func TestPipelineRouteToSpark(t *testing.T) {
	bin := buildMockClaude(t)
	order, phases := allPhasesConfig("signal-complete")

	// Override RV to route to spark.
	phases[PhaseRV] = PhaseConfig{
		Agent: "test-agent", Workflow: "review",
		CommandArgs: []string{"signal-route-spark"},
		Routes:      map[string]Phase{"spark": PhaseIM, "hon": PhaseRF},
	}

	cfg := Config{
		FWUID:                "FWU-01",
		WorkDir:              t.TempDir(),
		AgentsDir:            "testdata",
		Order:                order,
		Phases:               phases,
		MaxReviewCycles:      2,
		DefaultMaxIterations: 10,
		MaxRetries:           1,
		RetryBackoff:         []time.Duration{0},
		ThrashThreshold:      0,
		BriefingFunc:         mockBriefing(),
		CommandName:          bin,
	}

	p, ch := New(cfg)

	var events []loop.Event
	done := make(chan struct{})
	go func() {
		events = drainEvents(ch)
		close(done)
	}()

	err := p.Run(context.Background())
	<-done

	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	// Should have route_decision events.
	routeDecisions := countEventType(events, loop.EventRouteDecision)
	if routeDecisions < 1 {
		t.Error("expected at least 1 route_decision event")
	}

	// IM should run more than once (initial + re-runs from spark routing).
	imStarts := 0
	for _, e := range events {
		if e.Type == loop.EventPhaseStart && e.Phase == "IM" {
			imStarts++
		}
	}
	if imStarts < 2 {
		t.Errorf("IM phase starts = %d, want >= 2 (initial + spark rework)", imStarts)
	}

	// Pipeline should end with operator_attention (max review cycles hit on 3rd attempt).
	if !hasEventType(events, loop.EventOperatorAttention) {
		t.Error("expected operator_attention when max review cycles exceeded")
	}
}

func TestPipelineRouteToHon(t *testing.T) {
	bin := buildMockClaude(t)
	order, phases := allPhasesConfig("signal-complete")

	phases[PhaseRV] = PhaseConfig{
		Agent: "test-agent", Workflow: "review",
		CommandArgs: []string{"signal-route-hon"},
		Routes:      map[string]Phase{"spark": PhaseIM, "hon": PhaseRF},
	}

	cfg := Config{
		FWUID:                "FWU-01",
		WorkDir:              t.TempDir(),
		AgentsDir:            "testdata",
		Order:                order,
		Phases:               phases,
		MaxReviewCycles:      3,
		DefaultMaxIterations: 10,
		MaxRetries:           1,
		RetryBackoff:         []time.Duration{0},
		ThrashThreshold:      0,
		BriefingFunc:         mockBriefing(),
		CommandName:          bin,
	}

	p, ch := New(cfg)

	var events []loop.Event
	done := make(chan struct{})
	go func() {
		events = drainEvents(ch)
		close(done)
	}()

	err := p.Run(context.Background())
	<-done

	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	// Should have route_decision for hon.
	var foundHonRoute bool
	for _, e := range events {
		if e.Type == loop.EventRouteDecision && e.RouteTarget == "hon" {
			foundHonRoute = true
		}
	}
	if !foundHonRoute {
		t.Error("expected route_decision with target=hon")
	}

	// RF should run after RV.
	var phases_seen []string
	for _, e := range events {
		if e.Type == loop.EventPhaseStart {
			phases_seen = append(phases_seen, e.Phase)
		}
	}
	expected := []string{"TD", "IM", "RV", "RF", "FL"}
	if len(phases_seen) != len(expected) {
		t.Errorf("phase starts = %v, want %v", phases_seen, expected)
	} else {
		for i, p := range expected {
			if phases_seen[i] != p {
				t.Errorf("phase[%d] = %s, want %s", i, phases_seen[i], p)
			}
		}
	}

	if !hasEventType(events, loop.EventPipelineComplete) {
		t.Error("missing pipeline_complete")
	}
}

func TestPipelineBlocked(t *testing.T) {
	bin := buildMockClaude(t)
	order, phases := allPhasesConfig("signal-complete")

	// IM phase gets blocked.
	phases[PhaseIM] = PhaseConfig{Agent: "test-agent", Workflow: "implement", CommandArgs: []string{"signal-blocked"}}

	cfg := Config{
		FWUID:                "FWU-01",
		WorkDir:              t.TempDir(),
		AgentsDir:            "testdata",
		Order:                order,
		Phases:               phases,
		MaxReviewCycles:      3,
		DefaultMaxIterations: 10,
		MaxRetries:           1,
		RetryBackoff:         []time.Duration{0},
		ThrashThreshold:      0,
		BriefingFunc:         mockBriefing(),
		CommandName:          bin,
	}

	p, ch := New(cfg)

	var events []loop.Event
	done := make(chan struct{})
	go func() {
		events = drainEvents(ch)
		close(done)
	}()

	err := p.Run(context.Background())
	<-done

	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if !hasEventType(events, loop.EventOperatorAttention) {
		t.Error("expected operator_attention for blocked signal")
	}

	// Pipeline should NOT complete (no pipeline_complete).
	if hasEventType(events, loop.EventPipelineComplete) {
		t.Error("pipeline_complete should not be emitted when blocked")
	}
}

func TestPipelineMaxReviewCycles(t *testing.T) {
	bin := buildMockClaude(t)
	order, phases := allPhasesConfig("signal-complete")

	phases[PhaseRV] = PhaseConfig{
		Agent: "test-agent", Workflow: "review",
		CommandArgs: []string{"signal-route-spark"},
		Routes:      map[string]Phase{"spark": PhaseIM, "hon": PhaseRF},
	}

	cfg := Config{
		FWUID:                "FWU-01",
		WorkDir:              t.TempDir(),
		AgentsDir:            "testdata",
		Order:                order,
		Phases:               phases,
		MaxReviewCycles:      2,
		DefaultMaxIterations: 10,
		MaxRetries:           1,
		RetryBackoff:         []time.Duration{0},
		ThrashThreshold:      0,
		BriefingFunc:         mockBriefing(),
		CommandName:          bin,
	}

	p, ch := New(cfg)

	var events []loop.Event
	done := make(chan struct{})
	go func() {
		events = drainEvents(ch)
		close(done)
	}()

	err := p.Run(context.Background())
	<-done

	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	// After 2 review cycles, the 3rd route(spark) should fail.
	if !hasEventType(events, loop.EventOperatorAttention) {
		t.Error("expected operator_attention after max review cycles")
	}

	// Count review cycles: IM should start 3 times (initial + 2 reworks).
	imStarts := 0
	for _, e := range events {
		if e.Type == loop.EventPhaseStart && e.Phase == "IM" {
			imStarts++
		}
	}
	if imStarts != 3 {
		t.Errorf("IM starts = %d, want 3", imStarts)
	}
}

func TestPipelinePause(t *testing.T) {
	bin := buildMockClaude(t)
	order, phases := allPhasesConfig("signal-complete")

	// TD uses slow scenario so pause has time to take effect during first phase.
	phases[PhaseTD] = PhaseConfig{Agent: "test-agent", Workflow: "technical-design", CommandArgs: []string{"slow"}}

	cfg := Config{
		FWUID:                "FWU-01",
		WorkDir:              t.TempDir(),
		AgentsDir:            "testdata",
		Order:                order,
		Phases:               phases,
		MaxReviewCycles:      3,
		DefaultMaxIterations: 10,
		MaxRetries:           1,
		RetryBackoff:         []time.Duration{0},
		ThrashThreshold:      0,
		BriefingFunc:         mockBriefing(),
		CommandName:          bin,
	}

	p, ch := New(cfg)

	var events []loop.Event
	done := make(chan struct{})
	go func() {
		events = drainEvents(ch)
		close(done)
	}()

	go func() {
		time.Sleep(100 * time.Millisecond)
		p.Pause()
		time.Sleep(100 * time.Millisecond)
		p.Stop() // ensure process is killed so test doesn't hang
	}()

	err := p.Run(context.Background())
	<-done

	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	// Pipeline should have started TD but NOT completed all 5 phases.
	if hasEventType(events, loop.EventPipelineComplete) {
		t.Error("pipeline_complete should not be emitted when paused")
	}

	if !hasEventType(events, loop.EventPhaseStart) {
		t.Error("expected at least one phase_start")
	}

	phaseStarts := countEventType(events, loop.EventPhaseStart)
	if phaseStarts > 2 {
		t.Errorf("expected at most 2 phase starts (paused early), got %d", phaseStarts)
	}
}

func TestPipelineStop(t *testing.T) {
	bin := buildMockClaude(t)
	order, phases := allPhasesConfig("slow")

	cfg := Config{
		FWUID:                "FWU-01",
		WorkDir:              t.TempDir(),
		AgentsDir:            "testdata",
		Order:                order,
		Phases:               phases,
		MaxReviewCycles:      3,
		DefaultMaxIterations: 10,
		MaxRetries:           1,
		RetryBackoff:         []time.Duration{0},
		ThrashThreshold:      0,
		BriefingFunc:         mockBriefing(),
		CommandName:          bin,
	}

	p, ch := New(cfg)

	var events []loop.Event
	done := make(chan struct{})
	go func() {
		events = drainEvents(ch)
		close(done)
	}()

	go func() {
		time.Sleep(100 * time.Millisecond)
		p.Stop()
	}()

	err := p.Run(context.Background())
	<-done

	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if hasEventType(events, loop.EventPipelineComplete) {
		t.Error("pipeline_complete should not be emitted when stopped")
	}
}

func TestPipelineContextCancellation(t *testing.T) {
	bin := buildMockClaude(t)
	order, phases := allPhasesConfig("slow")

	cfg := Config{
		FWUID:                "FWU-01",
		WorkDir:              t.TempDir(),
		AgentsDir:            "testdata",
		Order:                order,
		Phases:               phases,
		MaxReviewCycles:      3,
		DefaultMaxIterations: 10,
		MaxRetries:           1,
		RetryBackoff:         []time.Duration{0},
		ThrashThreshold:      0,
		BriefingFunc:         mockBriefing(),
		CommandName:          bin,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	p, ch := New(cfg)

	var events []loop.Event
	done := make(chan struct{})
	go func() {
		events = drainEvents(ch)
		close(done)
	}()

	err := p.Run(ctx)
	<-done

	if err == nil {
		t.Fatal("expected context error")
	}

	_ = events
}

func TestPipelineEventsHavePhaseContext(t *testing.T) {
	bin := buildMockClaude(t)
	order, phases := allPhasesConfig("signal-complete")

	cfg := Config{
		FWUID:                "FWU-TEST-01",
		WorkDir:              t.TempDir(),
		AgentsDir:            "testdata",
		Order:                order,
		Phases:               phases,
		MaxReviewCycles:      3,
		DefaultMaxIterations: 10,
		MaxRetries:           1,
		RetryBackoff:         []time.Duration{0},
		ThrashThreshold:      0,
		BriefingFunc:         mockBriefing(),
		CommandName:          bin,
	}

	p, ch := New(cfg)

	var events []loop.Event
	done := make(chan struct{})
	go func() {
		events = drainEvents(ch)
		close(done)
	}()

	err := p.Run(context.Background())
	<-done

	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	for _, e := range events {
		switch e.Type {
		case loop.EventPipelineComplete:
			if e.FWUID != "FWU-TEST-01" {
				t.Errorf("pipeline_complete FWUID = %q, want %q", e.FWUID, "FWU-TEST-01")
			}
		case loop.EventPhaseStart, loop.EventPhaseComplete:
			if e.Phase == "" {
				t.Errorf("%s event missing Phase", e.Type)
			}
			if e.FWUID != "FWU-TEST-01" {
				t.Errorf("%s FWUID = %q, want %q", e.Type, e.FWUID, "FWU-TEST-01")
			}
			if e.Agent == "" {
				t.Errorf("%s event missing Agent", e.Type)
			}
		default:
			if e.Phase == "" {
				t.Errorf("%s event missing Phase", e.Type)
			}
			if e.FWUID != "FWU-TEST-01" {
				t.Errorf("%s FWUID = %q, want %q", e.Type, e.FWUID, "FWU-TEST-01")
			}
		}
	}
}

func TestPipelineBriefingCalledPerPhase(t *testing.T) {
	bin := buildMockClaude(t)
	order, phases := allPhasesConfig("signal-complete")

	briefingFn, counter := mockBriefingCounter()

	cfg := Config{
		FWUID:                "FWU-01",
		WorkDir:              t.TempDir(),
		AgentsDir:            "testdata",
		Order:                order,
		Phases:               phases,
		MaxReviewCycles:      3,
		DefaultMaxIterations: 10,
		MaxRetries:           1,
		RetryBackoff:         []time.Duration{0},
		ThrashThreshold:      0,
		BriefingFunc:         briefingFn,
		CommandName:          bin,
	}

	p, ch := New(cfg)

	done := make(chan struct{})
	go func() {
		for range ch {
		}
		close(done)
	}()

	err := p.Run(context.Background())
	<-done

	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	count := counter.Load()
	if count != 5 {
		t.Errorf("BriefingFunc called %d times, want 5 (once per phase)", count)
	}
}

func TestPipelineFromPipelineDef(t *testing.T) {
	bin := buildMockClaude(t)

	// Use PipelineDef instead of Phases/Order to verify the Pipeline field path.
	// Uses test-agent which exists in testdata.
	def := &PipelineDef{
		Phases: []PhaseDef{
			{ID: "TD", Agent: "test-agent", Workflow: "technical-design"},
			{ID: "IM", Agent: "test-agent", Workflow: "implement"},
			{ID: "RV", Agent: "test-agent", Workflow: "review", Routes: map[string]string{"spark": "IM", "hon": "RF"}},
			{ID: "RF", Agent: "test-agent", Workflow: "refactor"},
			{ID: "FL", Agent: "test-agent", Workflow: "feedback-loop"},
		},
	}

	cfg := Config{
		FWUID:                "FWU-01",
		WorkDir:              t.TempDir(),
		AgentsDir:            "testdata",
		Pipeline:             def,
		MaxReviewCycles:      3,
		DefaultMaxIterations: 10,
		MaxRetries:           1,
		RetryBackoff:         []time.Duration{0},
		ThrashThreshold:      0,
		BriefingFunc:         mockBriefing(),
		CommandName:          bin,
		CommandArgs:          []string{"signal-complete"},
	}

	p, ch := New(cfg)

	var events []loop.Event
	done := make(chan struct{})
	go func() {
		events = drainEvents(ch)
		close(done)
	}()

	err := p.Run(context.Background())
	<-done

	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	starts := countEventType(events, loop.EventPhaseStart)
	if starts != 5 {
		t.Errorf("phase_start count = %d, want 5", starts)
	}

	if !hasEventType(events, loop.EventPipelineComplete) {
		t.Error("missing pipeline_complete event")
	}
}
