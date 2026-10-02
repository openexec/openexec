package manager

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/openexec/openexec/internal/pipeline"
	"github.com/openexec/openexec/pkg/db/state"
)

// These exercise start's configuration and admission only. Cancellation before
// dispatch prevents a runner, provider, task queue or recovery journey running.
func TestStartUnitConfigurationAndRefusals(t *testing.T) {
	for _, configured := range []bool{false, true} {
		t.Run(map[bool]string{false: "defaults", true: "configured"}[configured], func(t *testing.T) {
			dir := t.TempDir()
			store, err := state.NewStore(filepath.Join(dir, "state.db"))
			if err != nil {
				t.Fatal(err)
			}
			defer store.Close()
			if configured {
				if err := os.MkdirAll(filepath.Join(dir, ".openexec"), 0755); err != nil {
					t.Fatal(err)
				}
				data := `{"execution":{"api_provider":"unit","api_key":"fixture","api_model":"fixture","bitnet_routing":true},"quality_gates":{"no_stubs_rules":{"placeholder":"warning"},"production_ready_skip":["docs"]}}`
				if err := os.WriteFile(filepath.Join(dir, ".openexec", "config.json"), []byte(data), 0600); err != nil {
					t.Fatal(err)
				}
			}
			m, err := New(Config{WorkDir: dir, StateStore: store})
			if err != nil {
				t.Fatal(err)
			}
			defer m.Close()
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			var observed pipeline.Config
			if err := m.start(ctx, "A", false, func(c *pipeline.Config) { observed = *c; c.BlueprintID = "absent" }); err != nil {
				t.Fatal(err)
			}
			m.mu.RLock()
			startedEntry := m.pipelines["A"]
			m.mu.RUnlock()
			select {
			case <-startedEntry.done:
			case <-time.After(5 * time.Second):
				t.Fatal("start did not settle")
			}
			if observed.FWUID != "A" || observed.StateDB != store.GetDB() {
				t.Fatalf("configuration not forwarded: %+v", observed)
			}
			if configured && observed.APIProvider != "unit" {
				t.Fatal("active provider not forwarded")
			}
			m.mu.Lock()
			m.taskQueueActive = true
			m.mu.Unlock()
			if err := m.start(ctx, "B", false); err == nil || !strings.Contains(err.Error(), "owns") {
				t.Fatalf("queue ownership: %v", err)
			}
			m.mu.Lock()
			m.taskQueueActive = false
			m.pipelines["busy"] = &entry{info: PipelineInfo{Status: StatusStarting}, done: make(chan struct{})}
			m.mu.Unlock()
			if err := m.start(ctx, "busy", true); err == nil || !strings.Contains(err.Error(), "already active") {
				t.Fatalf("duplicate: %v", err)
			}
			m.mu.Lock()
			delete(m.pipelines, "busy")
			m.mu.Unlock()
		})
	}
}
