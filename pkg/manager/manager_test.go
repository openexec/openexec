package manager

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
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


// writeGateCommands gives a fixture's project config real lint and test commands.
// Deterministic gate stages fail closed when nothing resolves, so a test that
// runs a pipeline has to configure them the way a real project does.
func writeGateCommands(t *testing.T, workDir string) {
	t.Helper()
	dir := filepath.Join(workDir, ".openexec")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	const projectConfig = `{"execution":{"lint_commands":["true"],"test_commands":["true"]}}`
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte(projectConfig), 0o644); err != nil {
		t.Fatal(err)
	}
}

func managerConfig(t *testing.T, bin string) Config {
	t.Helper()
	tmpDir := t.TempDir()
	stateStore, err := state.NewStore(filepath.Join(tmpDir, "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	writeGateCommands(t, tmpDir)
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

// Status copied its info snapshot after releasing the read lock, so it raced the
// event goroutine writing the same fields. Only -race can observe this: without
// it the test passes even with the bug restored. Readers run while a real
// pipeline is progressing, which is what the API's run listing does.
func TestConcurrentReadersDoNotRaceRunningPipeline(t *testing.T) {
	bin := buildMockClaude(t)
	m, err := New(managerConfig(t, bin))
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()

	if err := m.Start(context.Background(), "FWU-RACE"); err != nil {
		t.Fatalf("Start: %v", err)
	}

	stop := make(chan struct{})
	var readers sync.WaitGroup
	for i := 0; i < 4; i++ {
		readers.Add(1)
		go func() {
			defer readers.Done()
			for {
				select {
				case <-stop:
					return
				default:
					if _, err := m.Status("FWU-RACE"); err != nil {
						return
					}
					_ = m.List()
				}
			}
		}()
	}

	deadline := time.After(30 * time.Second)
	for {
		info, err := m.Status("FWU-RACE")
		if err != nil {
			t.Fatalf("Status: %v", err)
		}
		if isTerminal(info.Status) {
			break
		}
		select {
		case <-deadline:
			close(stop)
			readers.Wait()
			t.Fatalf("timeout waiting for completion, last status: %s", info.Status)
		case <-time.After(20 * time.Millisecond):
		}
	}
	close(stop)
	readers.Wait()
}
