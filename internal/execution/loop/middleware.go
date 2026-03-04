package loop

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"sync"
	"time"
)

// TraceLevel defines the verbosity of trace capture.
type TraceLevel string

const (
	TraceLevelNone  TraceLevel = "none"
	TraceLevelBasic TraceLevel = "basic"
	TraceLevelFull  TraceLevel = "full"
	TraceLevelDebug TraceLevel = "debug"
)

// IOTrace represents a single captured I/O operation with full context.
type IOTrace struct {
	// Timestamp when the I/O was captured.
	Timestamp time.Time `json:"timestamp"`

	// Type: "stdin", "stdout", "stderr"
	Type string `json:"type"`

	// Agent ID (from config or context).
	AgentID string `json:"agent_id"`

	// Phase information (e.g., planning, execution).
	Phase string `json:"phase"`

	// FWUID (Firmware Update ID) for traceability.
	FWUID string `json:"fwu_id"`

	// Iteration number in the loop.
	Iteration int `json:"iteration"`

	// Data is the raw captured content (may be truncated for very large outputs).
	Data string `json:"data"`

	// Hash of the data for integrity verification.
	Hash string `json:"hash"`

	// Size of the original data (before truncation).
	Size int `json:"size"`

	// Truncated indicates if data was truncated.
	Truncated bool `json:"truncated"`
}

// DeepTraceConfig controls the middleware behavior.
type DeepTraceConfig struct {
	// Enabled controls whether tracing is active.
	Enabled bool

	// Level controls verbosity (none, basic, full, debug).
	Level TraceLevel

	// MaxTraceSize limits the size of captured data (bytes). 0 means unlimited.
	MaxTraceSize int

	// AgentID is the identifier for this agent.
	AgentID string

	// FWUID is the firmware update ID.
	FWUID string

	// Callback is invoked for each captured trace (optional).
	// Useful for real-time logging or storage.
	Callback func(*IOTrace) error

	// EncryptionCallback is invoked before storing traces to apply encryption.
	// If provided, all traces are encrypted before storage.
	EncryptionCallback func(*IOTrace) (*IOTrace, error)
}

// Middleware defines the interface for process I/O interceptors.
type Middleware interface {
	// WrapStdin wraps stdin with tracing.
	WrapStdin(io.WriteCloser) io.WriteCloser

	// WrapStdout wraps stdout with tracing.
	WrapStdout(io.ReadCloser) io.ReadCloser

	// WrapStderr wraps stderr with tracing.
	WrapStderr(io.ReadCloser) io.ReadCloser

	// OnPhaseChange notifies middleware of phase transitions.
	OnPhaseChange(phase string)

	// OnIterationChange notifies middleware of iteration number changes.
	OnIterationChange(iteration int)

	// Traces returns all captured traces (if recording is enabled).
	Traces() []*IOTrace

	// Close finalizes the middleware and releases resources.
	Close() error
}

// DeepTraceMiddleware is the non-bypassable interceptor for subprocess I/O.
type DeepTraceMiddleware struct {
	cfg      DeepTraceConfig
	traces   []*IOTrace
	tracesMu sync.Mutex

	phase     string
	iteration int

	// stdinTracer wraps stdin writes
	stdinTracer *stdinInterceptor

	// stdoutTracer wraps stdout reads
	stdoutTracer *stdoutInterceptor

	// stderrTracer wraps stderr reads
	stderrTracer *stderrInterceptor
}

// NewDeepTraceMiddleware creates a new tracing middleware instance.
func NewDeepTraceMiddleware(cfg DeepTraceConfig) *DeepTraceMiddleware {
	if !cfg.Enabled || cfg.Level == TraceLevelNone {
		return &DeepTraceMiddleware{
			cfg:    cfg,
			traces: []*IOTrace{},
		}
	}

	return &DeepTraceMiddleware{
		cfg:       cfg,
		traces:    make([]*IOTrace, 0, 256),
		phase:     "init",
		iteration: 0,
	}
}

// WrapStdin wraps stdin to capture prompts (user input to the agent).
func (m *DeepTraceMiddleware) WrapStdin(w io.WriteCloser) io.WriteCloser {
	if !m.cfg.Enabled || m.cfg.Level == TraceLevelNone {
		return w
	}

	m.stdinTracer = &stdinInterceptor{
		underlying: w,
		middleware: m,
	}
	return m.stdinTracer
}

// WrapStdout wraps stdout to capture agent responses.
func (m *DeepTraceMiddleware) WrapStdout(r io.ReadCloser) io.ReadCloser {
	if !m.cfg.Enabled || m.cfg.Level == TraceLevelNone {
		return r
	}

	m.stdoutTracer = &stdoutInterceptor{
		underlying: r,
		middleware: m,
	}
	return m.stdoutTracer
}

// WrapStderr wraps stderr to capture errors and diagnostics.
func (m *DeepTraceMiddleware) WrapStderr(r io.ReadCloser) io.ReadCloser {
	if !m.cfg.Enabled || m.cfg.Level == TraceLevelNone {
		return r
	}

	m.stderrTracer = &stderrInterceptor{
		underlying: r,
		middleware: m,
	}
	return m.stderrTracer
}

// OnPhaseChange updates the current phase.
func (m *DeepTraceMiddleware) OnPhaseChange(phase string) {
	m.phase = phase
}

// OnIterationChange updates the current iteration.
func (m *DeepTraceMiddleware) OnIterationChange(iteration int) {
	m.iteration = iteration
}

// Traces returns a copy of all captured traces.
func (m *DeepTraceMiddleware) Traces() []*IOTrace {
	m.tracesMu.Lock()
	defer m.tracesMu.Unlock()

	result := make([]*IOTrace, len(m.traces))
	copy(result, m.traces)
	return result
}

// Close finalizes the middleware.
func (m *DeepTraceMiddleware) Close() error {
	if m.stdinTracer != nil {
		_ = m.stdinTracer.Close()
	}
	if m.stdoutTracer != nil {
		_ = m.stdoutTracer.Close()
	}
	if m.stderrTracer != nil {
		_ = m.stderrTracer.Close()
	}
	return nil
}

// recordTrace is called by interceptors to record a trace event.
func (m *DeepTraceMiddleware) recordTrace(traceType string, data []byte) {
	m.tracesMu.Lock()
	defer m.tracesMu.Unlock()

	trace := &IOTrace{
		Timestamp: time.Now().UTC(),
		Type:      traceType,
		AgentID:   m.cfg.AgentID,
		Phase:     m.phase,
		FWUID:     m.cfg.FWUID,
		Iteration: m.iteration,
		Size:      len(data),
	}

	// Truncate if necessary
	dataStr := string(data)
	if m.cfg.MaxTraceSize > 0 && len(dataStr) > m.cfg.MaxTraceSize {
		dataStr = dataStr[:m.cfg.MaxTraceSize]
		trace.Truncated = true
	}
	trace.Data = dataStr

	// Compute hash
	trace.Hash = hashData(data)

	// Apply encryption callback if provided
	if m.cfg.EncryptionCallback != nil {
		encrypted, err := m.cfg.EncryptionCallback(trace)
		if err == nil && encrypted != nil {
			trace = encrypted
		}
	}

	m.traces = append(m.traces, trace)

	// Invoke callback if provided
	if m.cfg.Callback != nil {
		_ = m.cfg.Callback(trace)
	}
}

// stdinInterceptor wraps stdin writes to capture prompts.
type stdinInterceptor struct {
	underlying io.WriteCloser
	middleware *DeepTraceMiddleware
}

func (s *stdinInterceptor) Write(p []byte) (n int, err error) {
	if len(p) > 0 {
		s.middleware.recordTrace("stdin", p)
	}
	return s.underlying.Write(p)
}

func (s *stdinInterceptor) Close() error {
	return s.underlying.Close()
}

// stdoutInterceptor wraps stdout reads to capture agent responses.
type stdoutInterceptor struct {
	underlying io.ReadCloser
	middleware *DeepTraceMiddleware
}

func (s *stdoutInterceptor) Read(p []byte) (n int, err error) {
	n, err = s.underlying.Read(p)
	if n > 0 {
		s.middleware.recordTrace("stdout", p[:n])
	}
	return n, err
}

func (s *stdoutInterceptor) Close() error {
	return s.underlying.Close()
}

// stderrInterceptor wraps stderr reads to capture diagnostic output.
type stderrInterceptor struct {
	underlying io.ReadCloser
	middleware *DeepTraceMiddleware
}

func (s *stderrInterceptor) Read(p []byte) (n int, err error) {
	n, err = s.underlying.Read(p)
	if n > 0 {
		s.middleware.recordTrace("stderr", p[:n])
	}
	return n, err
}

func (s *stderrInterceptor) Close() error {
	return s.underlying.Close()
}

// hashData computes a SHA256 hash of the data for integrity verification.
func hashData(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}

// PersistTraces writes all traces to a writer as JSON lines.
func (m *DeepTraceMiddleware) PersistTraces(w io.Writer) error {
	m.tracesMu.Lock()
	defer m.tracesMu.Unlock()

	encoder := json.NewEncoder(w)
	for _, trace := range m.traces {
		if err := encoder.Encode(trace); err != nil {
			return err
		}
	}
	return nil
}

// PersistTracesContext is like PersistTraces but respects context cancellation.
func (m *DeepTraceMiddleware) PersistTracesContext(ctx context.Context, w io.Writer) error {
	m.tracesMu.Lock()
	defer m.tracesMu.Unlock()

	encoder := json.NewEncoder(w)
	for _, trace := range m.traces {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		if err := encoder.Encode(trace); err != nil {
			return err
		}
	}
	return nil
}
