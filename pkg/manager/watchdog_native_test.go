package manager

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/openexec/openexec/internal/loop"
	"github.com/openexec/openexec/pkg/db/state"
	"github.com/openexec/openexec/pkg/runtime"
)

// Every probe uses the real manager, native standard_task engine, event
// ingestion and SQLite. Only the provider is replaced; no production work runs.
func nativeWatchdogProbe(t *testing.T, mode string) {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".openexec"), 0700); err != nil {
		t.Fatal(err)
	}
	store, err := state.NewStore(filepath.Join(dir, "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	entered := make(chan context.Context, 1)
	releaseStage := make(chan struct{})
	finished := make(chan struct{})
	marker := filepath.Join(dir, "completed.txt")
	executor := admittedFixture(func(ctx context.Context, s *runtime.Stage, _ *runtime.StageInput) (*runtime.StageResult, error) {
		result := &runtime.StageResult{StageName: s.Name, Status: runtime.StageStatusCompleted}
		if s.Name == "gather_context" {
			if err := os.WriteFile(marker, []byte("retained useful work"), 0600); err != nil {
				return nil, err
			}
			result.Artifacts = map[string]string{"completed-work": marker}
		}
		target := "implement"
		if mode == "check" {
			target = "test"
		}
		if s.Name == target {
			entered <- ctx
			defer close(finished)
			// A real silent subprocess is killed by the native attempt context.
			if mode == "watchdog" {
				err := exec.CommandContext(ctx, "sh", "-c", "exec sleep 30").Run()
				return result, err
			}
			select {
			case <-ctx.Done():
				return result, ctx.Err()
			case <-releaseStage:
			}
		}
		return result, nil
	})
	stallThreshold := 150 * time.Millisecond
	if mode == "timeout" {
		stallThreshold = 3 * time.Second
	}
	m, err := New(Config{WorkDir: dir, StateStore: store, StageExecutor: executor, WatchdogStallThreshold: stallThreshold, WatchdogCheckInterval: 10 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	ctx, cancel := context.WithCancelCause(context.Background())
	defer cancel(nil)
	attemptTimeout := 4 * time.Second
	if mode == "timeout" {
		attemptTimeout = 2 * time.Second
	}
	deadline, cleanup := context.WithTimeout(ctx, attemptTimeout)
	defer cleanup()
	defer func() { cancel(nil); _ = m.Wait(context.Background(), "probe") }()
	if err := m.Start(deadline, "probe", WithBlueprint("standard_task")); err != nil {
		t.Fatal(err)
	}
	var stageCtx context.Context
	select {
	case stageCtx = <-entered:
	case <-deadline.Done():
		t.Fatal("stage unreachable")
	}
	events, unsub, err := m.Subscribe("probe")
	if err != nil {
		t.Fatal(err)
	}
	defer unsub()
	expected := mode
	switch mode {
	case "watchdog":
		expected = "watchdog_stall"
	case "owner":
		expected = "owner_stop"
		if err := m.Stop("probe"); err != nil {
			t.Fatal(err)
		}
	case "unknown":
		cancel(errors.New("watchdog_stall")) // Matching text is not trusted cause identity.
	case "allowance":
		expected = "allowance_exceeded"
		cancel(ErrAllowanceExceeded)
	case "plain":
		expected = "unknown"
		cancel(nil)
	case "timeout":
	// Let the real parent deadline expire.
	case "check", "paused", "active":
		if mode == "paused" {
			if err := m.Pause("probe"); err != nil {
				t.Fatal(err)
			}
		}
		// More than three stall windows. Active receives actual provider output
		// through consumeEvents, not direct timestamp writes or fake heartbeats.
		if mode == "active" {
			input := make(chan loop.Event)
			consumed := make(chan struct{})
			go func() { m.consumeEvents("probe", input); close(consumed) }()
			ticker := time.NewTicker(30 * time.Millisecond)
			timer := time.NewTimer(500 * time.Millisecond)
		active:
			for {
				select {
				case <-ticker.C:
					input <- loop.Event{Type: loop.EventAssistantText, Text: "useful progress"}
				case <-timer.C:
					break active
				}
			}
			ticker.Stop()
			close(input)
			<-consumed
		} else {
			<-time.After(500 * time.Millisecond)
		}
		if stageCtx.Err() != nil {
			t.Fatalf("legitimate %s killed: %v", mode, context.Cause(stageCtx))
		}
		if mode == "paused" {
			_ = m.Stop("probe")
			expected = "owner_stop"
		} else {
			close(releaseStage)
			expected = ""
		}
	}
	waitCtx, waitCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer waitCancel()
	if err := m.Wait(waitCtx, "probe"); err != nil {
		t.Fatalf("native cancellation did not settle: %v", err)
	}
	<-finished
	info, err := m.Status("probe")
	if err != nil {
		t.Fatal(err)
	}
	if info.CancellationCause != expected {
		t.Fatalf("cause=%q want %q; status=%+v", info.CancellationCause, expected, info)
	}
	if expected != "" && info.Status != StatusStopped {
		t.Fatalf("interruption overwritten: %+v", info)
	}
	if expected == "" && info.Status != StatusComplete {
		t.Fatalf("healthy execution failed: %+v", info)
	}
	if info.LastActivity.IsZero() {
		t.Fatal("native event activity was never ingested")
	}
	if expected != "" {
		terminal := false
		for event := range events {
			if event.Type == loop.EventBlueprintFailed {
				terminal = true
				if event.CancellationCause != expected {
					t.Fatalf("terminal cause=%q want %q", event.CancellationCause, expected)
				}
			}
		}
		if !terminal {
			t.Fatal("no native terminal event")
		}
		if mode == "watchdog" && !errors.Is(context.Cause(stageCtx), ErrWatchdogStall) {
			t.Fatalf("context cause lost: %v", context.Cause(stageCtx))
		}
		// A later Stop cannot relabel the first interruption.
		_ = m.Stop("probe")
		again, _ := m.Status("probe")
		if again.CancellationCause != expected {
			t.Fatal("first cause overwritten")
		}
	}
	raw, err := os.ReadFile(marker)
	if err != nil || string(raw) != "retained useful work" {
		t.Fatal("completed work lost", err)
	}
	checkpoints, err := os.ReadFile(filepath.Join(dir, ".openexec", "checkpoints", "probe.jsonl"))
	if err != nil || !strings.Contains(string(checkpoints), "completed-work") {
		t.Fatal("completed evidence lost", err)
	}
	// Reopen the actual database, rather than trusting the in-memory status.
	fresh, err := state.NewStore(filepath.Join(dir, "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer fresh.Close()
	run, err := fresh.GetRun(context.Background(), "probe")
	if err != nil || run == nil {
		t.Fatal("missing persisted run", err)
	}
	if run.Status != string(info.Status) || (expected != "" && run.ErrorMessage.String != expected) {
		t.Fatalf("persisted cause/status lost: %+v", run)
	}
	t.Logf("native %s: status=%s cause=%s persisted=%s completed evidence retained", mode, info.Status, info.CancellationCause, run.Status)
}

func TestNativeWatchdogSilent(t *testing.T) { nativeWatchdogProbe(t, "watchdog") }
func TestNativeWatchdogBoundaries(t *testing.T) {
	for _, mode := range []string{"owner", "unknown", "plain", "allowance", "timeout", "check", "paused", "active"} {
		t.Run(mode, func(t *testing.T) { nativeWatchdogProbe(t, mode) })
	}
}

// These controls actually compile and run the silent-stage reproducer with
// each correction removed. go test ./... (the declared test gate) runs them.
func TestNativeWatchdogRemovalControls(t *testing.T) {
	for _, control := range []struct{ name, path, old, replacement string }{
		{"activity", "events.go", "info.LastActivity = time.Now()", "info.LastActivity = time.Time{}"},
		{"cause", "watchdog.go", "ErrWatchdogStall, w.StallThreshold", "ErrOwnerStop, w.StallThreshold"},
	} {
		t.Run(control.name, func(t *testing.T) {
			source, err := filepath.Abs(control.path)
			if err != nil {
				t.Fatal(err)
			}
			raw, err := os.ReadFile(source)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Count(string(raw), control.old) != 1 {
				t.Fatal("removal control no longer matches")
			}
			dir := t.TempDir()
			replacement := filepath.Join(dir, "replacement.go")
			if err := os.WriteFile(replacement, []byte(strings.Replace(string(raw), control.old, control.replacement, 1)), 0600); err != nil {
				t.Fatal(err)
			}
			overlay, _ := json.Marshal(map[string]any{"Replace": map[string]string{source: replacement}})
			path := filepath.Join(dir, "overlay.json")
			if err := os.WriteFile(path, overlay, 0600); err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, "go", "test", "-buildvcs=false", "-overlay", path, "-run", "^TestNativeWatchdogSilent$", "-count=1", ".")
			output, err := cmd.CombinedOutput()
			if ctx.Err() != nil || err == nil || !strings.Contains(string(output), "--- FAIL: TestNativeWatchdogSilent") {
				t.Fatalf("removal was not detected: %v\n%s", err, output)
			}
			t.Logf("%s correction removed: reproducer rejected it\n%s", control.name, output)
		})
	}
}

func TestWatchdogActivityAndStaleSnapshot(t *testing.T) {
	old := time.Now().Add(-time.Hour)
	info := PipelineInfo{Status: StatusRunning, LastActivity: old}
	updateInfo(&info, loop.Event{Type: loop.EventHeartbeat})
	if !info.LastActivity.Equal(old) {
		t.Fatal("timer heartbeat manufactured activity")
	}
	updateInfo(&info, loop.Event{Type: loop.EventAssistantText, Text: "provider output"})
	if !info.LastActivity.After(old) {
		t.Fatal("real activity ignored")
	}
	// The watchdog's earlier status snapshot cannot override activity or a
	// legitimate wait that was ingested before cancellation acquires the lock.
	for _, mode := range []string{"active", "paused", "deterministic"} {
		t.Run(mode, func(t *testing.T) {
			current := PipelineInfo{Status: StatusRunning, LastActivity: old}
			switch mode {
			case "active":
				current.LastActivity = time.Now()
			case "paused":
				current.Status = StatusPaused
			case "deterministic":
				current.StageType = "deterministic"
			}
			stopped := false
			m := &Manager{pipelines: map[string]*entry{"probe": {info: current, cancel: func() { stopped = true }}}}
			w := &Watchdog{manager: m, StallThreshold: time.Minute}
			w.remediateStall("probe", PipelineInfo{Status: StatusRunning, LastActivity: old})
			if stopped {
				t.Fatal("stale snapshot cancelled changed attempt")
			}
		})
	}
}
