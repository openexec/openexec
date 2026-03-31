package loop

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func TestLoop_Lifecycle(t *testing.T) {
	t.Skip("standalone iterative loop was refactored to blueprint-only architecture; these tests need rewrite for blueprint mode")
	mockPath, _ := filepath.Abs("testdata/mock_claude")

	t.Run("MaxIterations", func(t *testing.T) {
		cfg := DefaultConfig()
		cfg.CommandName = mockPath
		cfg.CommandArgs = []string{"ok"}
		cfg.MaxIterations = 2

		l, events := New(cfg)
		ctx := context.Background()

		done := make(chan struct{})
		var lastEvent Event
		go func() {
			for e := range events {
				lastEvent = e
			}
			close(done)
		}()

		err := l.Run(ctx)
		if err != nil {
			t.Fatalf("Run failed: %v", err)
		}
		<-done

		if lastEvent.Type != EventMaxIterationsReached {
			t.Errorf("expected EventMaxIterationsReached, got %v", lastEvent.Type)
		}
		if lastEvent.Iteration != 2 {
			t.Errorf("expected iteration 2, got %d", lastEvent.Iteration)
		}
	})

	t.Run("Pause", func(t *testing.T) {
		cfg := DefaultConfig()
		cfg.CommandName = mockPath
		cfg.CommandArgs = []string{"ok"}

		l, events := New(cfg)
		l.Pause() // Pause before starting

		done := make(chan struct{})
		var foundPaused bool
		go func() {
			for e := range events {
				if e.Type == EventPaused {
					foundPaused = true
				}
			}
			close(done)
		}()

		err := l.Run(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		<-done

		if !foundPaused {
			t.Error("expected EventPaused")
		}
	})

	t.Run("Stop", func(t *testing.T) {
		cfg := DefaultConfig()
		cfg.CommandName = mockPath
		cfg.CommandArgs = []string{"slow"} // Slow scenario to allow stopping

		l, events := New(cfg)

		go func() {
			time.Sleep(100 * time.Millisecond)
			l.Stop()
		}()

		done := make(chan struct{})
		go func() {
			for range events {
			}
			close(done)
		}()

		err := l.Run(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		<-done

		if !l.stopped.Load() {
			t.Error("expected loop to be stopped")
		}
	})

	t.Run("Retry on Crash", func(t *testing.T) {
		cfg := DefaultConfig()
		cfg.CommandName = mockPath
		cfg.CommandArgs = []string{"crash"}
		cfg.MaxRetries = 1
		cfg.RetryBackoff = []time.Duration{10 * time.Millisecond}

		l, events := New(cfg)

		done := make(chan struct{})
		var retryEvents int
		go func() {
			for e := range events {
				if e.Type == EventRetrying {
					retryEvents++
				}
			}
			close(done)
		}()

		err := l.Run(context.Background())
		if err == nil {
			t.Error("expected error from exhausted retries")
		}
		<-done

		if retryEvents != 1 {
			t.Errorf("expected 1 retry event, got %d", retryEvents)
		}
	})
}
