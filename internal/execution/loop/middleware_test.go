package loop

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"
	"time"
)

// nopWriteCloser wraps an io.Writer to provide io.WriteCloser interface.
type nopWriteCloser struct {
	io.Writer
}

func (n *nopWriteCloser) Close() error {
	return nil
}

func TestNewDeepTraceMiddleware(t *testing.T) {
	tests := []struct {
		name      string
		cfg       DeepTraceConfig
		expectNil bool
	}{
		{
			name: "enabled middleware",
			cfg: DeepTraceConfig{
				Enabled: true,
				Level:   TraceLevelFull,
				AgentID: "test-agent",
				FWUID:   "fwu-123",
			},
			expectNil: false,
		},
		{
			name: "disabled middleware",
			cfg: DeepTraceConfig{
				Enabled: false,
				Level:   TraceLevelNone,
			},
			expectNil: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := NewDeepTraceMiddleware(tt.cfg)
			if m == nil && !tt.expectNil {
				t.Errorf("expected non-nil middleware")
			}
		})
	}
}

func TestStdoutInterception(t *testing.T) {
	cfg := DeepTraceConfig{
		Enabled: true,
		Level:   TraceLevelFull,
		AgentID: "test-agent",
		FWUID:   "fwu-123",
	}
	m := NewDeepTraceMiddleware(cfg)

	// Create a fake reader
	data := "hello world"
	underlying := io.NopCloser(strings.NewReader(data))
	wrapped := m.WrapStdout(underlying)

	// Read from wrapped reader
	buf := make([]byte, 11)
	n, err := wrapped.Read(buf)
	if err != nil && err != io.EOF {
		t.Errorf("unexpected read error: %v", err)
	}
	if n != 11 {
		t.Errorf("expected to read 11 bytes, got %d", n)
	}

	// Check that trace was recorded
	traces := m.Traces()
	if len(traces) != 1 {
		t.Errorf("expected 1 trace, got %d", len(traces))
	}
	if traces[0].Type != "stdout" {
		t.Errorf("expected trace type 'stdout', got '%s'", traces[0].Type)
	}
	if traces[0].Data != data {
		t.Errorf("expected data '%s', got '%s'", data, traces[0].Data)
	}

	_ = wrapped.Close()
}

func TestStderrInterception(t *testing.T) {
	cfg := DeepTraceConfig{
		Enabled: true,
		Level:   TraceLevelFull,
		AgentID: "test-agent",
		FWUID:   "fwu-123",
	}
	m := NewDeepTraceMiddleware(cfg)

	// Create a fake reader
	errData := "error message"
	underlying := io.NopCloser(strings.NewReader(errData))
	wrapped := m.WrapStderr(underlying)

	// Read from wrapped reader
	buf := make([]byte, len(errData))
	n, err := wrapped.Read(buf)
	if err != nil && err != io.EOF {
		t.Errorf("unexpected read error: %v", err)
	}
	if n != len(errData) {
		t.Errorf("expected to read %d bytes, got %d", len(errData), n)
	}

	// Check that trace was recorded
	traces := m.Traces()
	if len(traces) != 1 {
		t.Errorf("expected 1 trace, got %d", len(traces))
	}
	if traces[0].Type != "stderr" {
		t.Errorf("expected trace type 'stderr', got '%s'", traces[0].Type)
	}
	if traces[0].Data != errData {
		t.Errorf("expected data '%s', got '%s'", errData, traces[0].Data)
	}

	_ = wrapped.Close()
}

func TestStdinInterception(t *testing.T) {
	cfg := DeepTraceConfig{
		Enabled: true,
		Level:   TraceLevelFull,
		AgentID: "test-agent",
		FWUID:   "fwu-123",
	}
	m := NewDeepTraceMiddleware(cfg)

	// Create a buffer as underlying writer
	var buf bytes.Buffer
	underlying := &nopWriteCloser{Writer: &buf}
	wrapped := m.WrapStdin(underlying)

	// Write to stdin
	prompt := []byte("test prompt")
	n, err := wrapped.Write(prompt)
	if err != nil {
		t.Errorf("unexpected write error: %v", err)
	}
	if n != len(prompt) {
		t.Errorf("expected to write %d bytes, got %d", len(prompt), n)
	}

	// Check that trace was recorded
	traces := m.Traces()
	if len(traces) != 1 {
		t.Errorf("expected 1 trace, got %d", len(traces))
	}
	if traces[0].Type != "stdin" {
		t.Errorf("expected trace type 'stdin', got '%s'", traces[0].Type)
	}
	if traces[0].Data != string(prompt) {
		t.Errorf("expected data '%s', got '%s'", string(prompt), traces[0].Data)
	}

	_ = wrapped.Close()
}

func TestTraceContextFields(t *testing.T) {
	cfg := DeepTraceConfig{
		Enabled: true,
		Level:   TraceLevelFull,
		AgentID: "agent-42",
		FWUID:   "fwu-999",
	}
	m := NewDeepTraceMiddleware(cfg)
	m.OnIterationChange(5)
	m.OnPhaseChange("execution")

	underlying := io.NopCloser(strings.NewReader("test"))
	wrapped := m.WrapStdout(underlying)

	buf := make([]byte, 4)
	_, _ = wrapped.Read(buf)

	traces := m.Traces()
	if len(traces) != 1 {
		t.Fatalf("expected 1 trace, got %d", len(traces))
	}

	trace := traces[0]
	if trace.AgentID != "agent-42" {
		t.Errorf("expected AgentID 'agent-42', got '%s'", trace.AgentID)
	}
	if trace.FWUID != "fwu-999" {
		t.Errorf("expected FWUID 'fwu-999', got '%s'", trace.FWUID)
	}
	if trace.Iteration != 5 {
		t.Errorf("expected Iteration 5, got %d", trace.Iteration)
	}
	if trace.Phase != "execution" {
		t.Errorf("expected Phase 'execution', got '%s'", trace.Phase)
	}

	_ = wrapped.Close()
}

func TestTraceTimestamp(t *testing.T) {
	cfg := DeepTraceConfig{
		Enabled: true,
		Level:   TraceLevelFull,
		AgentID: "test-agent",
		FWUID:   "fwu-123",
	}
	m := NewDeepTraceMiddleware(cfg)

	before := time.Now().UTC()
	underlying := io.NopCloser(strings.NewReader("test"))
	wrapped := m.WrapStdout(underlying)
	buf := make([]byte, 4)
	_, _ = wrapped.Read(buf)
	after := time.Now().UTC()

	traces := m.Traces()
	if len(traces) != 1 {
		t.Fatalf("expected 1 trace, got %d", len(traces))
	}

	// Timestamp should be between before and after
	if traces[0].Timestamp.Before(before) || traces[0].Timestamp.After(after) {
		t.Errorf("timestamp out of range: %v not between %v and %v",
			traces[0].Timestamp, before, after)
	}
}

func TestTraceSize(t *testing.T) {
	cfg := DeepTraceConfig{
		Enabled: true,
		Level:   TraceLevelFull,
		AgentID: "test-agent",
		FWUID:   "fwu-123",
	}
	m := NewDeepTraceMiddleware(cfg)

	testData := "this is test data"
	underlying := io.NopCloser(strings.NewReader(testData))
	wrapped := m.WrapStdout(underlying)

	buf := make([]byte, len(testData))
	_, _ = wrapped.Read(buf)

	traces := m.Traces()
	if len(traces) != 1 {
		t.Fatalf("expected 1 trace, got %d", len(traces))
	}

	if traces[0].Size != len(testData) {
		t.Errorf("expected Size %d, got %d", len(testData), traces[0].Size)
	}

	_ = wrapped.Close()
}

func TestTraceTruncation(t *testing.T) {
	cfg := DeepTraceConfig{
		Enabled:      true,
		Level:        TraceLevelFull,
		AgentID:      "test-agent",
		FWUID:        "fwu-123",
		MaxTraceSize: 10, // Truncate at 10 bytes
	}
	m := NewDeepTraceMiddleware(cfg)

	longData := "this is a very long test data that exceeds the limit"
	underlying := io.NopCloser(strings.NewReader(longData))
	wrapped := m.WrapStdout(underlying)

	buf := make([]byte, len(longData))
	_, _ = wrapped.Read(buf)

	traces := m.Traces()
	if len(traces) != 1 {
		t.Fatalf("expected 1 trace, got %d", len(traces))
	}

	if !traces[0].Truncated {
		t.Errorf("expected Truncated to be true")
	}
	if len(traces[0].Data) != 10 {
		t.Errorf("expected truncated data length 10, got %d", len(traces[0].Data))
	}
	if traces[0].Size != len(longData) {
		t.Errorf("expected original Size %d, got %d", len(longData), traces[0].Size)
	}

	_ = wrapped.Close()
}

func TestTraceHash(t *testing.T) {
	cfg := DeepTraceConfig{
		Enabled: true,
		Level:   TraceLevelFull,
		AgentID: "test-agent",
		FWUID:   "fwu-123",
	}
	m := NewDeepTraceMiddleware(cfg)

	data := "test data for hashing"
	underlying := io.NopCloser(strings.NewReader(data))
	wrapped := m.WrapStdout(underlying)

	buf := make([]byte, len(data))
	_, _ = wrapped.Read(buf)

	traces := m.Traces()
	if len(traces) != 1 {
		t.Fatalf("expected 1 trace, got %d", len(traces))
	}

	// Verify hash is not empty and looks reasonable (hex string)
	if traces[0].Hash == "" {
		t.Errorf("expected non-empty hash")
	}
	if len(traces[0].Hash) != 64 { // SHA256 hex string is 64 chars
		t.Errorf("expected hash length 64, got %d", len(traces[0].Hash))
	}

	_ = wrapped.Close()
}

func TestMultipleTraces(t *testing.T) {
	cfg := DeepTraceConfig{
		Enabled: true,
		Level:   TraceLevelFull,
		AgentID: "test-agent",
		FWUID:   "fwu-123",
	}
	m := NewDeepTraceMiddleware(cfg)

	// Record multiple traces
	data1 := "first read"
	underlying1 := io.NopCloser(strings.NewReader(data1))
	wrapped1 := m.WrapStdout(underlying1)
	buf1 := make([]byte, len(data1))
	_, _ = wrapped1.Read(buf1)
	_ = wrapped1.Close()

	data2 := "second read"
	underlying2 := io.NopCloser(strings.NewReader(data2))
	wrapped2 := m.WrapStdout(underlying2)
	buf2 := make([]byte, len(data2))
	_, _ = wrapped2.Read(buf2)
	_ = wrapped2.Close()

	traces := m.Traces()
	if len(traces) != 2 {
		t.Errorf("expected 2 traces, got %d", len(traces))
	}

	if traces[0].Data != data1 {
		t.Errorf("expected first trace data '%s', got '%s'", data1, traces[0].Data)
	}
	if traces[1].Data != data2 {
		t.Errorf("expected second trace data '%s', got '%s'", data2, traces[1].Data)
	}
}

func TestPersistTraces(t *testing.T) {
	cfg := DeepTraceConfig{
		Enabled: true,
		Level:   TraceLevelFull,
		AgentID: "test-agent",
		FWUID:   "fwu-123",
	}
	m := NewDeepTraceMiddleware(cfg)

	// Record a trace
	data := "test data"
	underlying := io.NopCloser(strings.NewReader(data))
	wrapped := m.WrapStdout(underlying)
	buf := make([]byte, len(data))
	_, _ = wrapped.Read(buf)
	_ = wrapped.Close()

	// Persist to buffer
	var output bytes.Buffer
	err := m.PersistTraces(&output)
	if err != nil {
		t.Fatalf("unexpected error persisting traces: %v", err)
	}

	// Verify output is JSON lines
	lines := strings.Split(strings.TrimSpace(output.String()), "\n")
	if len(lines) != 1 {
		t.Errorf("expected 1 JSON line, got %d", len(lines))
	}

	// Verify it contains expected fields
	if !strings.Contains(lines[0], "test-agent") {
		t.Errorf("trace output missing agent ID")
	}
	if !strings.Contains(lines[0], "test data") {
		t.Errorf("trace output missing data")
	}
}

func TestPersistTracesContext(t *testing.T) {
	cfg := DeepTraceConfig{
		Enabled: true,
		Level:   TraceLevelFull,
		AgentID: "test-agent",
		FWUID:   "fwu-123",
	}
	m := NewDeepTraceMiddleware(cfg)

	// Record a trace
	data := "test data"
	underlying := io.NopCloser(strings.NewReader(data))
	wrapped := m.WrapStdout(underlying)
	buf := make([]byte, len(data))
	_, _ = wrapped.Read(buf)
	_ = wrapped.Close()

	// Persist with valid context
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var output bytes.Buffer
	err := m.PersistTracesContext(ctx, &output)
	if err != nil {
		t.Fatalf("unexpected error persisting traces: %v", err)
	}

	if output.Len() == 0 {
		t.Errorf("expected output, got empty")
	}
}

func TestPersistTracesContextCancellation(t *testing.T) {
	cfg := DeepTraceConfig{
		Enabled: true,
		Level:   TraceLevelFull,
		AgentID: "test-agent",
		FWUID:   "fwu-123",
	}
	m := NewDeepTraceMiddleware(cfg)

	// Record a trace
	data := "test data"
	underlying := io.NopCloser(strings.NewReader(data))
	wrapped := m.WrapStdout(underlying)
	buf := make([]byte, len(data))
	_, _ = wrapped.Read(buf)
	_ = wrapped.Close()

	// Persist with cancelled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	var output bytes.Buffer
	err := m.PersistTracesContext(ctx, &output)
	if err == nil {
		t.Errorf("expected context cancelled error, got nil")
	}
}

func TestCallback(t *testing.T) {
	callbackCalled := false
	var capturedTrace *IOTrace

	cfg := DeepTraceConfig{
		Enabled: true,
		Level:   TraceLevelFull,
		AgentID: "test-agent",
		FWUID:   "fwu-123",
		Callback: func(trace *IOTrace) error {
			callbackCalled = true
			capturedTrace = trace
			return nil
		},
	}
	m := NewDeepTraceMiddleware(cfg)

	data := "callback test"
	underlying := io.NopCloser(strings.NewReader(data))
	wrapped := m.WrapStdout(underlying)
	buf := make([]byte, len(data))
	_, _ = wrapped.Read(buf)
	_ = wrapped.Close()

	if !callbackCalled {
		t.Errorf("expected callback to be called")
	}
	if capturedTrace == nil {
		t.Errorf("expected capturedTrace to be set")
	}
	if capturedTrace.Data != data {
		t.Errorf("expected callback trace data '%s', got '%s'", data, capturedTrace.Data)
	}
}

func TestDisabledMiddleware(t *testing.T) {
	cfg := DeepTraceConfig{
		Enabled: false,
		Level:   TraceLevelNone,
	}
	m := NewDeepTraceMiddleware(cfg)

	data := "test"
	underlying := io.NopCloser(strings.NewReader(data))
	wrapped := m.WrapStdout(underlying)

	// Wrapped should be the same as underlying when disabled
	if wrapped != underlying {
		t.Errorf("expected disabled middleware to return underlying reader unchanged")
	}

	// Traces should be empty
	traces := m.Traces()
	if len(traces) != 0 {
		t.Errorf("expected no traces when disabled, got %d", len(traces))
	}
}

func TestClose(t *testing.T) {
	cfg := DeepTraceConfig{
		Enabled: true,
		Level:   TraceLevelFull,
		AgentID: "test-agent",
		FWUID:   "fwu-123",
	}
	m := NewDeepTraceMiddleware(cfg)

	// Wrap some readers/writers
	underlying := io.NopCloser(strings.NewReader("test"))
	_ = m.WrapStdout(underlying)

	// Close should not error
	err := m.Close()
	if err != nil {
		t.Errorf("unexpected error from Close: %v", err)
	}
}

func TestHashConsistency(t *testing.T) {
	cfg := DeepTraceConfig{
		Enabled: true,
		Level:   TraceLevelFull,
		AgentID: "test-agent",
		FWUID:   "fwu-123",
	}
	m := NewDeepTraceMiddleware(cfg)

	data := "consistency test"
	underlying := io.NopCloser(strings.NewReader(data))
	wrapped := m.WrapStdout(underlying)
	buf := make([]byte, len(data))
	_, _ = wrapped.Read(buf)
	_ = wrapped.Close()

	traces := m.Traces()
	firstHash := traces[0].Hash

	// Create another middleware with the same data
	m2 := NewDeepTraceMiddleware(cfg)
	underlying2 := io.NopCloser(strings.NewReader(data))
	wrapped2 := m2.WrapStdout(underlying2)
	buf2 := make([]byte, len(data))
	_, _ = wrapped2.Read(buf2)
	_ = wrapped2.Close()

	traces2 := m2.Traces()
	secondHash := traces2[0].Hash

	if firstHash != secondHash {
		t.Errorf("expected hashes to be consistent for same data")
	}
}
