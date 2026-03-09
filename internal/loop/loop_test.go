package loop

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func TestLoop_Run(t *testing.T) {
	// Find absolute path to mock_claude
	mockPath, err := filepath.Abs("testdata/mock_claude")
	if err != nil {
		t.Fatalf("failed to get abs path: %v", err)
	}

	tests := []struct {
		name          string
		scenario      string
		expectedEvent EventType
	}{
		{
			name:          "successful run",
			scenario:      "ok",
			expectedEvent: EventComplete,
		},
		{
			name:          "run with signal",
			scenario:      "signal-complete",
			expectedEvent: EventComplete,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := DefaultConfig()
			cfg.CommandName = mockPath
			cfg.CommandArgs = []string{tt.scenario}

			l, events := New(cfg)

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			go func() {
				_ = l.Run(ctx)
			}()

			found := false
			for {
				select {
				case e, ok := <-events:
					if !ok {
						if !found && tt.expectedEvent == EventComplete {
							found = true
						}
						goto end
					}
					if e.Type == tt.expectedEvent {
						found = true
					}
				case <-ctx.Done():
					t.Errorf("test timed out")
					goto end
				}
			}
		end:
			if !found {
				t.Errorf("expected event %v not found", tt.expectedEvent)
			}
		})
	}
}
