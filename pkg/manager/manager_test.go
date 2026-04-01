package manager

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/openexec/openexec/pkg/db/state"
)

// buildMockClaude compiles the mock_claude test helper from the loop package.
func buildMockClaude(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	bin := filepath.Join(dir, "mock_claude")
	src := filepath.Join("..", "..", "internal", "loop", "testdata", "mock_claude.go")

	cmd := exec.Command("go", "build", "-o", bin, src)
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("build mock_claude: %v", err)
	}
	return bin
}


func managerConfig(t *testing.T, bin string) Config {
	t.Helper()
	tmpDir := t.TempDir()
	stateStore, err := state.NewStore(filepath.Join(tmpDir, "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	return Config{
		WorkDir:              tmpDir,
		AgentsFS:             os.DirFS(filepath.Join("..", "..", "internal", "pipeline", "testdata")),
		DefaultMaxIterations: 10,
		MaxRetries:           1,
		ThrashThreshold:      0,
		RetryBackoff:         []time.Duration{0},
		CommandName:          bin,
		StateStore:           stateStore,
	}
}

func TestNewManager(t *testing.T) {
	tmpDir := t.TempDir()
	stateStore, err := state.NewStore(filepath.Join(tmpDir, "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	m, err := New(Config{WorkDir: tmpDir, StateStore: stateStore})
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
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
	tmpDir := t.TempDir()
	stateStore, err := state.NewStore(filepath.Join(tmpDir, "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	m, err := New(Config{WorkDir: tmpDir, StateStore: stateStore})
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	list := m.List()
	if len(list) != 0 {
		t.Errorf("List() = %d items, want 0", len(list))
	}
}

func TestStatusNotFound(t *testing.T) {
	tmpDir := t.TempDir()
	stateStore, err := state.NewStore(filepath.Join(tmpDir, "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	m, err := New(Config{WorkDir: tmpDir, StateStore: stateStore})
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	_, err = m.Status("nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent pipeline")
	}
}

func TestStartAndStatus(t *testing.T) {
	bin := buildMockClaude(t)
	cfg := managerConfig(t, bin)
	m, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()

	err = m.Start(context.Background(), "FWU-01")
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
	t.Skip("LEGACY: Test uses phase-based configuration. Blueprint mode uses stages.")
}

func TestStartAfterComplete(t *testing.T) {
	bin := buildMockClaude(t)
	cfg := managerConfig(t, bin)
	m, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()

	err = m.Start(context.Background(), "FWU-01")
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
	t.Skip("LEGACY: Test uses phase-based configuration. Blueprint mode uses stages.")
}

func TestStop(t *testing.T) {
	t.Skip("LEGACY: Test uses phase-based configuration. Blueprint mode uses stages.")
}

func TestList(t *testing.T) {
	bin := buildMockClaude(t)
	cfg := managerConfig(t, bin)
	m, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()

	err = m.Start(context.Background(), "FWU-01")
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
	cfg := managerConfig(t, bin)
	m, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()

	err = m.Start(context.Background(), "FWU-01")
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
	tmpDir := t.TempDir()
	stateStore, err := state.NewStore(filepath.Join(tmpDir, "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	m, err := New(Config{WorkDir: tmpDir, StateStore: stateStore})
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	_, _, err = m.Subscribe("nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent pipeline")
	}
}
