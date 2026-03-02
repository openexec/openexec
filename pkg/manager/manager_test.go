package manager

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/openexec/openexec/internal/pipeline"
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

func mockBriefing() pipeline.BriefingFunc {
	return func(ctx context.Context, fwuID string) (string, error) {
		return "## FWU Briefing: " + fwuID + "\n\n**Status:** in_progress\n**Intent:** Test intent", nil
	}
}

func allPhasesConfig(scenario string) ([]pipeline.Phase, map[pipeline.Phase]pipeline.PhaseConfig) {
	order := pipeline.DefaultPhaseOrder()
	phases := map[pipeline.Phase]pipeline.PhaseConfig{
		pipeline.PhaseTD: {Agent: "test-agent", Workflow: "technical-design", CommandArgs: []string{scenario}},
		pipeline.PhaseIM: {Agent: "test-agent", Workflow: "implement", CommandArgs: []string{scenario}},
		pipeline.PhaseRV: {Agent: "test-agent", Workflow: "review", CommandArgs: []string{scenario}, Routes: map[string]pipeline.Phase{"spark": pipeline.PhaseIM, "hon": pipeline.PhaseRF}},
		pipeline.PhaseRF: {Agent: "test-agent", Workflow: "refactor", CommandArgs: []string{scenario}},
		pipeline.PhaseFL: {Agent: "test-agent", Workflow: "feedback-loop", CommandArgs: []string{scenario}},
	}
	return order, phases
}

func managerConfig(bin string) Config {
	order, phases := allPhasesConfig("signal-complete")
	return Config{
		WorkDir:              "",
		AgentsDir:            filepath.Join("..", "pipeline", "testdata"),
		Order:                order,
		Phases:               phases,
		DefaultMaxIterations: 10,
		MaxRetries:           1,
		MaxReviewCycles:      3,
		ThrashThreshold:      0,
		RetryBackoff:         []time.Duration{0},
		CommandName:          bin,
		BriefingFunc:         mockBriefing(),
	}
}

func TestNewManager(t *testing.T) {
	m := New(Config{WorkDir: "/tmp"})
	if m == nil {
		t.Fatal("New returned nil")
	}
	if m.cfg.DefaultMaxIterations != 10 {
		t.Errorf("DefaultMaxIterations = %d, want 10", m.cfg.DefaultMaxIterations)
	}
	if m.cfg.MaxRetries != 3 {
		t.Errorf("MaxRetries = %d, want 3", m.cfg.MaxRetries)
	}
}

func TestListEmpty(t *testing.T) {
	m := New(Config{WorkDir: "/tmp"})
	list := m.List()
	if len(list) != 0 {
		t.Errorf("List() = %d items, want 0", len(list))
	}
}

func TestStatusNotFound(t *testing.T) {
	m := New(Config{WorkDir: "/tmp"})
	_, err := m.Status("nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent pipeline")
	}
}

func TestStartAndStatus(t *testing.T) {
	bin := buildMockClaude(t)
	cfg := managerConfig(bin)
	cfg.WorkDir = t.TempDir()
	m := New(cfg)

	err := m.Start(context.Background(), "FWU-01")
	if err != nil {
		t.Fatalf("Start: %v", err)
	}

	// Wait for pipeline to complete.
	deadline := time.After(30 * time.Second)
	for {
		info, err := m.Status("FWU-01")
		if err != nil {
			t.Fatalf("Status: %v", err)
		}
		if isTerminal(info.Status) {
			if info.Status != StatusComplete {
				t.Errorf("status = %s, want complete (error: %s)", info.Status, info.Error)
			}
			break
		}
		select {
		case <-deadline:
			t.Fatalf("timeout waiting for pipeline completion, last status: %s", info.Status)
		case <-time.After(50 * time.Millisecond):
		}
	}

	info, _ := m.Status("FWU-01")
	if info.FWUID != "FWU-01" {
		t.Errorf("FWUID = %s, want FWU-01", info.FWUID)
	}
	if info.Elapsed == "" {
		t.Error("Elapsed is empty")
	}
}

func TestStartDuplicate(t *testing.T) {
	bin := buildMockClaude(t)
	cfg := managerConfig(bin)
	cfg.WorkDir = t.TempDir()
	order, phases := allPhasesConfig("slow") // keep running so duplicate start fails
	cfg.Order = order
	cfg.Phases = phases
	m := New(cfg)

	err := m.Start(context.Background(), "FWU-01")
	if err != nil {
		t.Fatalf("Start: %v", err)
	}

	// Wait for it to be running.
	time.Sleep(100 * time.Millisecond)

	// Second start should fail.
	err = m.Start(context.Background(), "FWU-01")
	if err == nil {
		t.Fatal("expected error for duplicate start")
	}

	// Clean up.
	m.Stop("FWU-01")
}

func TestStartAfterComplete(t *testing.T) {
	bin := buildMockClaude(t)
	cfg := managerConfig(bin)
	cfg.WorkDir = t.TempDir()
	m := New(cfg)

	err := m.Start(context.Background(), "FWU-01")
	if err != nil {
		t.Fatalf("Start: %v", err)
	}

	// Wait for completion.
	deadline := time.After(30 * time.Second)
	for {
		info, _ := m.Status("FWU-01")
		if isTerminal(info.Status) {
			break
		}
		select {
		case <-deadline:
			t.Fatal("timeout")
		case <-time.After(50 * time.Millisecond):
		}
	}

	// Re-start should succeed after completion.
	err = m.Start(context.Background(), "FWU-01")
	if err != nil {
		t.Fatalf("re-Start after complete: %v", err)
	}

	// Clean up: wait or stop.
	deadline = time.After(30 * time.Second)
	for {
		info, _ := m.Status("FWU-01")
		if isTerminal(info.Status) {
			break
		}
		select {
		case <-deadline:
			m.Stop("FWU-01")
			return
		case <-time.After(50 * time.Millisecond):
		}
	}
}

func TestPause(t *testing.T) {
	bin := buildMockClaude(t)
	cfg := managerConfig(bin)
	cfg.WorkDir = t.TempDir()
	order, phases := allPhasesConfig("slow")
	cfg.Order = order
	cfg.Phases = phases
	m := New(cfg)

	err := m.Start(context.Background(), "FWU-01")
	if err != nil {
		t.Fatalf("Start: %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	err = m.Pause("FWU-01")
	if err != nil {
		t.Fatalf("Pause: %v", err)
	}

	// Also stop to ensure the test doesn't hang.
	time.Sleep(100 * time.Millisecond)
	m.Stop("FWU-01")
}

func TestStop(t *testing.T) {
	bin := buildMockClaude(t)
	cfg := managerConfig(bin)
	cfg.WorkDir = t.TempDir()
	order, phases := allPhasesConfig("slow")
	cfg.Order = order
	cfg.Phases = phases
	m := New(cfg)

	err := m.Start(context.Background(), "FWU-01")
	if err != nil {
		t.Fatalf("Start: %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	err = m.Stop("FWU-01")
	if err != nil {
		t.Fatalf("Stop: %v", err)
	}

	info, _ := m.Status("FWU-01")
	if info.Status != StatusStopped {
		t.Errorf("status = %s, want stopped", info.Status)
	}
}

func TestList(t *testing.T) {
	bin := buildMockClaude(t)
	cfg := managerConfig(bin)
	cfg.WorkDir = t.TempDir()
	m := New(cfg)

	err := m.Start(context.Background(), "FWU-01")
	if err != nil {
		t.Fatalf("Start FWU-01: %v", err)
	}

	err = m.Start(context.Background(), "FWU-02")
	if err != nil {
		t.Fatalf("Start FWU-02: %v", err)
	}

	// Wait for both to finish.
	deadline := time.After(30 * time.Second)
	for {
		list := m.List()
		allDone := true
		for _, info := range list {
			if !isTerminal(info.Status) {
				allDone = false
			}
		}
		if allDone && len(list) == 2 {
			break
		}
		select {
		case <-deadline:
			t.Fatal("timeout waiting for pipelines")
		case <-time.After(50 * time.Millisecond):
		}
	}

	list := m.List()
	if len(list) != 2 {
		t.Errorf("List() = %d, want 2", len(list))
	}
}

func TestSubscribe(t *testing.T) {
	bin := buildMockClaude(t)
	cfg := managerConfig(bin)
	cfg.WorkDir = t.TempDir()
	m := New(cfg)

	err := m.Start(context.Background(), "FWU-01")
	if err != nil {
		t.Fatalf("Start: %v", err)
	}

	sub, unsub, err := m.Subscribe("FWU-01")
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	defer unsub()

	// Read at least one event.
	select {
	case ev, ok := <-sub:
		if !ok {
			// Channel closed already — pipeline finished fast.
			return
		}
		if ev.Type == "" {
			t.Error("received event with empty type")
		}
	case <-time.After(30 * time.Second):
		t.Fatal("timeout waiting for event")
	}
}

func TestSubscribeNotFound(t *testing.T) {
	m := New(Config{WorkDir: "/tmp"})
	_, _, err := m.Subscribe("nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent pipeline")
	}
}
