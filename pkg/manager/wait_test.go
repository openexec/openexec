package manager

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/openexec/openexec/pkg/runtime"
)

func TestWaitJoinsStoppedAttempt(t *testing.T) {
	e := newSchedulerTestEnv(t)
	entered := make(chan struct{})
	release := make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	marker := filepath.Join(e.dir, "shutdown-finished")
	e.mgr.cfg.StageExecutor = admittedFixture(func(ctx context.Context, _ *runtime.Stage, _ *runtime.StageInput) (*runtime.StageResult, error) {
		close(entered)
		<-ctx.Done()
		<-release // Simulate work that must finish after cancellation.
		if err := os.WriteFile(marker, []byte("finished"), 0600); err != nil {
			return nil, err
		}
		return nil, ctx.Err()
	})
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := e.mgr.Wait(ctx, "missing"); err == nil {
		t.Fatal("unknown attempt accepted")
	}
	if err := e.mgr.Start(ctx, "RUN-WAIT"); err != nil {
		t.Fatal(err)
	}
	defer func() {
		unblock()
		_ = e.mgr.Stop("RUN-WAIT")
		if err := e.mgr.Wait(ctx, "RUN-WAIT"); err != nil {
			t.Error(err)
		}
	}()
	select {
	case <-entered:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	if err := e.mgr.Stop("RUN-WAIT"); err != nil {
		t.Fatal(err)
	}
	info, err := e.mgr.Status("RUN-WAIT")
	if err != nil || info.Status != StatusStopped {
		t.Fatalf("expected stopped status: %+v, %v", info, err)
	}
	canceled, stopWaiting := context.WithCancel(ctx)
	stopWaiting()
	if err := e.mgr.Wait(canceled, "RUN-WAIT"); !errors.Is(err, context.Canceled) {
		t.Fatalf("Wait returned before stopped attempt finished: %v", err)
	}
	unblock()
	if err := e.mgr.Wait(ctx, "RUN-WAIT"); err != nil {
		t.Fatal(err)
	}
	if raw, err := os.ReadFile(marker); err != nil || string(raw) != "finished" {
		t.Fatalf("shutdown work not persisted: %q, %v", raw, err)
	}
	if err := e.mgr.Wait(ctx, "RUN-WAIT"); err != nil {
		t.Fatalf("repeat wait: %v", err)
	}
}
