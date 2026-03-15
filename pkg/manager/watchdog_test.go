package manager

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/openexec/openexec/internal/pipeline"
	"github.com/openexec/openexec/pkg/db/state"
)

func TestWatchdogDetection(t *testing.T) {
	tmpDir := t.TempDir()
	stateStore, err := state.NewStore(filepath.Join(tmpDir, "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	m, err := New(Config{WorkDir: tmpDir, StateStore: stateStore})
	if err != nil {
		t.Fatal(err)
	}

	// Set very short stall threshold for testing
	m.watchdog.StallThreshold = 100 * time.Millisecond
	m.watchdog.CheckInterval = 50 * time.Millisecond

	// Create a dummy pipeline
	fwuID := "STALL-01"
	p, _ := pipeline.New(pipeline.Config{FWUID: fwuID})

	e := &entry{
		pipeline: p,
		info: PipelineInfo{
			FWUID:        fwuID,
			Status:       StatusRunning,
			StartedAt:    time.Now().Add(-1 * time.Hour),
			LastActivity: time.Now().Add(-1 * time.Hour), // Explicitly old activity
		},
	}

	m.mu.Lock()
	m.pipelines[fwuID] = e
	m.mu.Unlock()

	// Wait for watchdog to trigger
	time.Sleep(300 * time.Millisecond)

	m.mu.RLock()
	status := m.pipelines[fwuID].info.Status
	m.mu.RUnlock()

	if status != StatusStopped {
		t.Errorf("status = %s, want %s (remediated)", status, StatusStopped)
	}
}
