package loop

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// buildMockClaude compiles the mock_claude test helper binary into dir
// and returns the path to the binary.
func buildMockClaude(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	bin := filepath.Join(dir, "mock_claude")
	src := filepath.Join("testdata", "mock_claude.go")

	cmd := exec.Command("go", "build", "-o", bin, src)
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("build mock_claude: %v", err)
	}
	return bin
}

func drainEvents(ch <-chan Event) []Event {
	var events []Event
	for e := range ch {
		events = append(events, e)
	}
	return events
}

func hasEventType(events []Event, typ EventType) bool {
	for _, e := range events {
		if e.Type == typ {
			return true
		}
	}
	return false
}

func countEventType(events []Event, typ EventType) int {
	n := 0
	for _, e := range events {
		if e.Type == typ {
			n++
		}
	}
	return n
}

// --- Tests ---

func TestLoopSingleIteration(t *testing.T) {
	bin := buildMockClaude(t)
	workDir := t.TempDir()

	cfg := Config{
		CommandName:   bin,
		CommandArgs:   []string{"ok"},
		WorkDir:       workDir,
		MaxIterations: 1,
		MaxRetries:    3,
		RetryBackoff:  []time.Duration{0},
	}

	l, ch := New(cfg)

	var events []Event
	done := make(chan struct{})
	go func() {
		events = drainEvents(ch)
		close(done)
	}()

	err := l.Run(context.Background())
	<-done

	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if !hasEventType(events, EventIterationStart) {
		t.Error("missing iteration_start event")
	}
	if !hasEventType(events, EventAssistantText) {
		t.Error("missing assistant_text event")
	}
	if !hasEventType(events, EventMaxIterationsReached) {
		t.Error("missing max_iterations_reached event")
	}
}

func TestLoopMultipleIterations(t *testing.T) {
	bin := buildMockClaude(t)
	workDir := t.TempDir()

	cfg := Config{
		CommandName:   bin,
		CommandArgs:   []string{"ok"},
		WorkDir:       workDir,
		MaxIterations: 3,
		MaxRetries:    3,
		RetryBackoff:  []time.Duration{0},
	}

	l, ch := New(cfg)

	var events []Event
	done := make(chan struct{})
	go func() {
		events = drainEvents(ch)
		close(done)
	}()

	err := l.Run(context.Background())
	<-done

	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	starts := countEventType(events, EventIterationStart)
	if starts != 3 {
		t.Errorf("iteration_start count = %d, want 3", starts)
	}

	if !hasEventType(events, EventMaxIterationsReached) {
		t.Error("missing max_iterations_reached")
	}
}

func TestLoopRetryOnCrash(t *testing.T) {
	bin := buildMockClaude(t)
	workDir := t.TempDir()

	cfg := Config{
		CommandName:   bin,
		CommandArgs:   []string{"crash"},
		WorkDir:       workDir,
		MaxIterations: 5,
		MaxRetries:    2,
		RetryBackoff:  []time.Duration{0},
	}

	l, ch := New(cfg)

	var events []Event
	done := make(chan struct{})
	go func() {
		events = drainEvents(ch)
		close(done)
	}()

	err := l.Run(context.Background())
	<-done

	if err == nil {
		t.Fatal("expected error from exhausted retries")
	}

	retries := countEventType(events, EventRetrying)
	if retries != 2 {
		t.Errorf("retry count = %d, want 2", retries)
	}

	if !hasEventType(events, EventError) {
		t.Error("missing error event")
	}
}

func TestLoopPause(t *testing.T) {
	bin := buildMockClaude(t)
	workDir := t.TempDir()

	cfg := Config{
		CommandName:   bin,
		CommandArgs:   []string{"ok"},
		WorkDir:       workDir,
		MaxIterations: 100, // high limit — pause should stop us first
		MaxRetries:    3,
		RetryBackoff:  []time.Duration{0},
	}

	l, ch := New(cfg)

	var events []Event
	done := make(chan struct{})
	go func() {
		events = drainEvents(ch)
		close(done)
	}()

	// Pause after a short delay to let at least one iteration through.
	go func() {
		time.Sleep(50 * time.Millisecond)
		l.Pause()
	}()

	err := l.Run(context.Background())
	<-done

	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if !hasEventType(events, EventPaused) {
		t.Error("missing paused event")
	}

	if hasEventType(events, EventMaxIterationsReached) {
		t.Error("should not reach max iterations when paused")
	}
}

func TestLoopStop(t *testing.T) {
	bin := buildMockClaude(t)
	workDir := t.TempDir()

	cfg := Config{
		CommandName:   bin,
		CommandArgs:   []string{"slow"},
		WorkDir:       workDir,
		MaxIterations: 100,
		MaxRetries:    3,
		RetryBackoff:  []time.Duration{0},
	}

	l, ch := New(cfg)

	var events []Event
	done := make(chan struct{})
	go func() {
		events = drainEvents(ch)
		close(done)
	}()

	go func() {
		time.Sleep(100 * time.Millisecond)
		l.Stop()
	}()

	err := l.Run(context.Background())
	<-done

	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	// Should have exited without max_iterations_reached.
	if hasEventType(events, EventMaxIterationsReached) {
		t.Error("should not reach max iterations when stopped")
	}
}

func TestLoopContextCancellation(t *testing.T) {
	bin := buildMockClaude(t)
	workDir := t.TempDir()

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	cfg := Config{
		CommandName:   bin,
		CommandArgs:   []string{"slow"},
		WorkDir:       workDir,
		MaxIterations: 100,
		MaxRetries:    3,
		RetryBackoff:  []time.Duration{0},
	}

	l, ch := New(cfg)

	var events []Event
	done := make(chan struct{})
	go func() {
		events = drainEvents(ch)
		close(done)
	}()

	err := l.Run(ctx)
	<-done

	if err == nil {
		t.Fatal("expected context error")
	}

	_ = events // just verify no panic
}

func TestLoopMaxIterationsZeroMeansUnlimited(t *testing.T) {
	bin := buildMockClaude(t)
	workDir := t.TempDir()

	cfg := Config{
		CommandName:   bin,
		CommandArgs:   []string{"ok"},
		WorkDir:       workDir,
		MaxIterations: 0, // unlimited
		MaxRetries:    3,
		RetryBackoff:  []time.Duration{0},
	}

	l, ch := New(cfg)

	var events []Event
	done := make(chan struct{})
	go func() {
		events = drainEvents(ch)
		close(done)
	}()

	// Stop after a few iterations.
	go func() {
		time.Sleep(100 * time.Millisecond)
		l.Stop()
	}()

	err := l.Run(context.Background())
	<-done

	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	starts := countEventType(events, EventIterationStart)
	if starts < 1 {
		t.Error("expected at least 1 iteration with unlimited max")
	}

	if hasEventType(events, EventMaxIterationsReached) {
		t.Error("should not emit max_iterations_reached with 0 limit")
	}
}

func TestLoopRetryBackoff(t *testing.T) {
	bin := buildMockClaude(t)
	workDir := t.TempDir()

	var sleepCalls []time.Duration

	cfg := Config{
		CommandName:   bin,
		CommandArgs:   []string{"crash"},
		WorkDir:       workDir,
		MaxIterations: 5,
		MaxRetries:    3,
		RetryBackoff:  []time.Duration{0, 10 * time.Millisecond, 20 * time.Millisecond},
	}

	l, ch := New(cfg)
	l.sleepFn = func(d time.Duration) {
		sleepCalls = append(sleepCalls, d)
	}

	var events []Event
	done := make(chan struct{})
	go func() {
		events = drainEvents(ch)
		close(done)
	}()

	l.Run(context.Background())
	<-done

	_ = events
	if len(sleepCalls) != 2 {
		// First retry has backoff[0]=0 which doesn't call sleep.
		// Retries 1 and 2 have backoff[1]=10ms and backoff[2]=20ms.
		t.Errorf("sleep calls = %v, want 2 entries", sleepCalls)
	}

	for i, d := range sleepCalls {
		expected := cfg.RetryBackoff[i+1]
		if d != expected {
			t.Errorf("sleep[%d] = %v, want %v", i, d, expected)
		}
	}
}

func TestLoopEventsIncludeCorrectTypes(t *testing.T) {
	bin := buildMockClaude(t)
	workDir := t.TempDir()

	cfg := Config{
		CommandName:   bin,
		CommandArgs:   []string{"full"},
		WorkDir:       workDir,
		MaxIterations: 1,
		MaxRetries:    3,
		RetryBackoff:  []time.Duration{0},
	}

	l, ch := New(cfg)

	var events []Event
	done := make(chan struct{})
	go func() {
		events = drainEvents(ch)
		close(done)
	}()

	l.Run(context.Background())
	<-done

	types := make(map[EventType]bool)
	for _, e := range events {
		types[e.Type] = true
	}

	for _, want := range []EventType{EventIterationStart, EventAssistantText, EventToolStart, EventToolResult} {
		if !types[want] {
			var got []string
			for _, e := range events {
				got = append(got, string(e.Type))
			}
			t.Errorf("missing %q in events: %v", want, strings.Join(got, ", "))
		}
	}
}

// --- V2 signal tests ---

func TestLoopSignalComplete(t *testing.T) {
	bin := buildMockClaude(t)
	workDir := t.TempDir()

	cfg := Config{
		CommandName:     bin,
		CommandArgs:     []string{"signal-complete"},
		WorkDir:         workDir,
		MaxIterations:   10,
		MaxRetries:      3,
		RetryBackoff:    []time.Duration{0},
		ThrashThreshold: 0, // disable thrashing for this test
	}

	l, ch := New(cfg)

	var events []Event
	done := make(chan struct{})
	go func() {
		events = drainEvents(ch)
		close(done)
	}()

	err := l.Run(context.Background())
	<-done

	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	// Should have: iteration_start, assistant_text, signal_received, tool_result, complete.
	if !hasEventType(events, EventSignalReceived) {
		t.Error("missing signal_received event")
	}
	if !hasEventType(events, EventComplete) {
		t.Error("missing complete event — phase-complete signal should end the loop")
	}
	if hasEventType(events, EventMaxIterationsReached) {
		t.Error("should not reach max iterations when phase-complete signal received")
	}

	// Should complete after exactly 1 iteration.
	starts := countEventType(events, EventIterationStart)
	if starts != 1 {
		t.Errorf("iteration_start count = %d, want 1", starts)
	}
}

func TestLoopSignalRoute(t *testing.T) {
	bin := buildMockClaude(t)
	workDir := t.TempDir()

	cfg := Config{
		CommandName:     bin,
		CommandArgs:     []string{"signal-route-spark"},
		WorkDir:         workDir,
		MaxIterations:   10,
		MaxRetries:      3,
		RetryBackoff:    []time.Duration{0},
		ThrashThreshold: 0,
	}

	l, ch := New(cfg)

	var events []Event
	done := make(chan struct{})
	go func() {
		events = drainEvents(ch)
		close(done)
	}()

	err := l.Run(context.Background())
	<-done

	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	// Should have signal_received with type "route".
	if !hasEventType(events, EventSignalReceived) {
		t.Error("missing signal_received event")
	}

	// Route signal should end the loop like phase-complete.
	if !hasEventType(events, EventComplete) {
		t.Error("missing complete event — route signal should end the loop")
	}
	if hasEventType(events, EventMaxIterationsReached) {
		t.Error("should not reach max iterations when route signal received")
	}

	// Should complete after exactly 1 iteration.
	starts := countEventType(events, EventIterationStart)
	if starts != 1 {
		t.Errorf("iteration_start count = %d, want 1", starts)
	}

	// Verify route target is in the signal event.
	for _, e := range events {
		if e.Type == EventSignalReceived && e.SignalType == "route" {
			if e.SignalTarget != "spark" {
				t.Errorf("route target = %q, want %q", e.SignalTarget, "spark")
			}
		}
	}
}

func TestLoopSignalProgressPreventsThrasching(t *testing.T) {
	bin := buildMockClaude(t)
	workDir := t.TempDir()

	cfg := Config{
		CommandName:     bin,
		CommandArgs:     []string{"signal-progress"},
		WorkDir:         workDir,
		MaxIterations:   5,
		MaxRetries:      3,
		RetryBackoff:    []time.Duration{0},
		ThrashThreshold: 3,
	}

	l, ch := New(cfg)

	var events []Event
	done := make(chan struct{})
	go func() {
		events = drainEvents(ch)
		close(done)
	}()

	err := l.Run(context.Background())
	<-done

	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	// Agent sends progress every iteration, so no thrashing.
	if hasEventType(events, EventThrashingDetected) {
		t.Error("should not detect thrashing when progress signal sent each iteration")
	}

	// Should run to max iterations.
	if !hasEventType(events, EventMaxIterationsReached) {
		t.Error("should reach max iterations")
	}
}

func TestLoopThrashingDetected(t *testing.T) {
	bin := buildMockClaude(t)
	workDir := t.TempDir()

	cfg := Config{
		CommandName:     bin,
		CommandArgs:     []string{"ok"}, // no signals
		WorkDir:         workDir,
		MaxIterations:   10,
		MaxRetries:      3,
		RetryBackoff:    []time.Duration{0},
		ThrashThreshold: 3,
	}

	l, ch := New(cfg)

	var events []Event
	done := make(chan struct{})
	go func() {
		events = drainEvents(ch)
		close(done)
	}()

	err := l.Run(context.Background())
	<-done

	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if !hasEventType(events, EventThrashingDetected) {
		t.Error("should detect thrashing after 3 iterations without progress")
	}

	// Should stop before max iterations.
	if hasEventType(events, EventMaxIterationsReached) {
		t.Error("should not reach max iterations — thrashing should stop first")
	}

	// Should have run exactly 3 iterations.
	starts := countEventType(events, EventIterationStart)
	if starts != 3 {
		t.Errorf("iteration_start count = %d, want 3", starts)
	}
}

func TestLoopThrashingDisabledBackwardCompat(t *testing.T) {
	bin := buildMockClaude(t)
	workDir := t.TempDir()

	cfg := Config{
		CommandName:     bin,
		CommandArgs:     []string{"ok"}, // no signals
		WorkDir:         workDir,
		MaxIterations:   3,
		MaxRetries:      3,
		RetryBackoff:    []time.Duration{0},
		ThrashThreshold: 0, // disabled — V1 behavior
	}

	l, ch := New(cfg)

	var events []Event
	done := make(chan struct{})
	go func() {
		events = drainEvents(ch)
		close(done)
	}()

	err := l.Run(context.Background())
	<-done

	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	// V1 behavior: runs to max iterations without thrashing.
	if hasEventType(events, EventThrashingDetected) {
		t.Error("should not detect thrashing when threshold is 0")
	}
	if !hasEventType(events, EventMaxIterationsReached) {
		t.Error("should reach max iterations with V1 backward compat")
	}
}

func TestLoopV1BackwardCompat(t *testing.T) {
	// Ensure existing V1 tests still pass with the new SignalTracker.
	// The "ok" scenario has no signals — loop should run to max iterations
	// when thrashing is disabled (threshold 0).
	bin := buildMockClaude(t)
	workDir := t.TempDir()

	cfg := Config{
		CommandName:     bin,
		CommandArgs:     []string{"ok"},
		WorkDir:         workDir,
		MaxIterations:   2,
		MaxRetries:      3,
		RetryBackoff:    []time.Duration{0},
		ThrashThreshold: 0,
	}

	l, ch := New(cfg)

	var events []Event
	done := make(chan struct{})
	go func() {
		events = drainEvents(ch)
		close(done)
	}()

	err := l.Run(context.Background())
	<-done

	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	starts := countEventType(events, EventIterationStart)
	if starts != 2 {
		t.Errorf("expected 2 iterations, got %d", starts)
	}
	if !hasEventType(events, EventMaxIterationsReached) {
		t.Error("should reach max iterations")
	}
}

func TestSessionRecording(t *testing.T) {
	bin := buildMockClaude(t)
	workDir := t.TempDir()
	evidenceDir := t.TempDir()

	cfg := Config{
		CommandName:   bin,
		CommandArgs:   []string{"ok"},
		WorkDir:       workDir,
		MaxIterations: 1,
		EvidenceDir:   evidenceDir,
		FwuID:         "test-fwu-id",
	}

	l, ch := New(cfg)

	done := make(chan struct{})
	go func() {
		drainEvents(ch)
		close(done)
	}()

	err := l.Run(context.Background())
	<-done

	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	// Verify evidence directory structure
	fwuDir := filepath.Join(evidenceDir, "test-fwu-id")
	entries, err := os.ReadDir(fwuDir)
	if err != nil {
		t.Fatalf("ReadDir fwuDir: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 timestamp dir, got %d", len(entries))
	}
	tsDir := filepath.Join(fwuDir, entries[0].Name())

	// Verify files
	if _, err := os.Stat(filepath.Join(tsDir, "stdout.jsonl")); err != nil {
		t.Error("missing stdout.jsonl")
	}
	if _, err := os.Stat(filepath.Join(tsDir, "stderr.log")); err != nil {
		t.Error("missing stderr.log")
	}
	if _, err := os.Stat(filepath.Join(tsDir, "meta.json")); err != nil {
		t.Error("missing meta.json")
	}
}

type mockUploader struct {
	mu    sync.Mutex
	calls []string
}

func (m *mockUploader) UploadSession(ctx context.Context, localDir, fwuID, timestamp string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls = append(m.calls, fmt.Sprintf("%s|%s|%s", localDir, fwuID, timestamp))
	return nil
}

func TestLoop_UploadEvidence(t *testing.T) {
	bin := buildMockClaude(t)
	workDir := t.TempDir()
	evidenceDir := t.TempDir()

	mockUp := &mockUploader{}

	cfg := Config{
		CommandName:    bin,
		CommandArgs:    []string{"ok"},
		WorkDir:        workDir,
		MaxIterations:  1,
		EvidenceDir:    evidenceDir,
		FwuID:          "test-fwu-id",
		EvidenceBucket: "test-bucket",
		UploaderFactory: func(ctx context.Context, uCfg UploaderConfig) (Uploader, error) {
			if uCfg.Bucket != "test-bucket" {
				return nil, fmt.Errorf("unexpected bucket: %s", uCfg.Bucket)
			}
			return mockUp, nil
		},
	}

	l, ch := New(cfg)

	done := make(chan struct{})
	go func() {
		drainEvents(ch)
		close(done)
	}()

	err := l.Run(context.Background())
	<-done

	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	mockUp.mu.Lock()
	defer mockUp.mu.Unlock()

	if len(mockUp.calls) != 1 {
		t.Fatalf("expected 1 upload call, got %d", len(mockUp.calls))
	}

	parts := strings.Split(mockUp.calls[0], "|")
	if len(parts) != 3 {
		t.Fatalf("malformed call record: %s", mockUp.calls[0])
	}

	if parts[1] != "test-fwu-id" {
		t.Errorf("expected fwuID 'test-fwu-id', got %s", parts[1])
	}
	if parts[2] == "" {
		t.Errorf("expected non-empty timestamp")
	}
}
