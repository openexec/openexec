package loop

import (
    "context"
    "fmt"
    "io"
    "os"
    "os/exec"
    "path/filepath"
    "sync"
    "sync/atomic"
    "time"

    "github.com/openexec/openexec/internal/summarize"
    "github.com/openexec/openexec/pkg/agent"
    )
// Loop is the core iteration executor. It spawns Claude Code repeatedly,
// parsing stream-JSON output into typed Events, with retry and lifecycle control.
type Loop struct {
	cfg       Config
	events    chan Event
	paused    atomic.Bool
	stopped   atomic.Bool
	iteration int
	tracker   *SignalTracker

	// lastActivity tracks the last time ANY event was emitted
	lastActivity atomic.Pointer[time.Time]

	// cancel kills the current process context when Stop is called.
	cancel context.CancelFunc
	mu     sync.Mutex

	// sleepFn is used by tests to override time.Sleep.
	sleepFn func(time.Duration)

	// middleware is the Deep-Trace middleware for ISO 27001 compliance.
	middleware Middleware

	// gateRetryCount tracks how many times we've retried after gate failures.
	gateRetryCount int

	// gateFixPrompt is appended to the prompt when gates fail.
	gateFixPrompt string

	// history tracks the message conversation for provider-backed loops.
	history []agent.Message
}

// New creates a Loop with the given config and returns it along with a
// read-only event channel. The channel is closed when Run returns.
func New(cfg Config) (*Loop, <-chan Event) {
	ch := make(chan Event, 64)

	// Initialize middleware if configured
	var m Middleware
	if cfg.DeepTraceCfg != nil && cfg.DeepTraceCfg.Enabled {
		m = NewDeepTraceMiddleware(*cfg.DeepTraceCfg)
	} else {
		// Create a no-op middleware when not configured
		m = NewDeepTraceMiddleware(DeepTraceConfig{Enabled: false})
	}

	l := &Loop{
		cfg:        cfg,
		events:     ch,
		tracker:    NewSignalTracker(cfg.ThrashThreshold),
		sleepFn:    time.Sleep,
		middleware: m,
		history:    make([]agent.Message, 0),
	}
	now := time.Now()
	l.lastActivity.Store(&now)
	return l, ch
}

// Run executes the loop until completion, max iterations, stop, or context cancellation.
// It closes the event channel when it returns.
func (l *Loop) Run(ctx context.Context) error {
	defer close(l.events)
	defer func() {
		if l.middleware != nil {
			_ = l.middleware.Close()
		}
	}()

	// Run preflight checks before starting
	if l.cfg.PreflightChecks {
		preflightReport := l.runPreflightChecks()
		if preflightReport != nil && !preflightReport.Passed {
			l.emit(Event{
				Type:    EventError,
				ErrText: "Preflight checks failed - cannot start task",
			})
			return fmt.Errorf("preflight checks failed: %s", preflightReport.Summary)
		}
	}

	recorder := NewSessionRecorder(l.cfg.EvidenceDir, l.cfg.FwuID)
	retryCount := 0

	for {
		// Check lifecycle.
		if l.stopped.Load() {
			return nil
		}
		if l.paused.Load() {
			l.emit(Event{Type: EventPaused, Iteration: l.iteration})
			return nil
		}
		if l.cfg.MaxIterations > 0 && l.iteration >= l.cfg.MaxIterations {
			l.emit(Event{Type: EventMaxIterationsReached, Iteration: l.iteration})
			return nil
		}

		// Check context before spawning.
		if ctx.Err() != nil {
			return ctx.Err()
		}

        l.iteration++
        l.emit(Event{Type: EventIterationStart, Iteration: l.iteration})

        // Provider-backed execution path: 
        // Only use this if no explicit RunnerCommand override was provided in config,
        // AND the resolved command name matches a known provider (openai/gemini).
        // This ensures local CLI binaries (like gemini-cli) take precedence if configured.
        forceProvider := os.Getenv("OPENEXEC_FORCE_PROVIDER") == "1"
        isProviderBinary := false
        if l.cfg.CommandName != "" {
            name := filepath.Base(l.cfg.CommandName)
            isProviderBinary = (name == "openai" || name == "gemini")
        }

        useProvider := (isProviderBinary && l.cfg.RunnerCommand == "") || forceProvider

        if useProvider {
            // Initialize providers from env (best-effort)
            agent.InitializeDefaultRegistry()

            // Determine model: prioritize ExecutorModel if set, else fall back to CommandArgs[0]
            model := l.cfg.ExecutorModel
            if model == "" && len(l.cfg.CommandArgs) > 0 {
                model = l.cfg.CommandArgs[0]
            }
            if model == "" {
                // Fallback: pick a default per provider name
                name := filepath.Base(l.cfg.CommandName)
                if name == "openai" {
                    model = "gpt-4o"
                } else if name == "gemini" {
                    model = "gemini-3.1-pro-preview"
                }
            }

            // 1. Add current prompt to history
            l.history = append(l.history, agent.NewTextMessage(agent.RoleUser, l.cfg.Prompt))

            // 2. Manage context window via summarization (if enabled)
            messages := l.history
            if l.cfg.Summarizer != nil {
                // Check if we need to summarize (using a standard 128k limit as heuristic if unknown)
                limit := 128000
                check := l.cfg.Summarizer.ShouldSummarize(l.history, limit)
                
                if check.ShouldSummarize {
                    l.emit(Event{
                        Type: EventProgress, 
                        Text: fmt.Sprintf("Summarizing session history to save tokens (saving ~%d tokens)", check.EstimatedSavings),
                    })
                    
                    if _, err := l.cfg.Summarizer.Summarize(ctx, l.cfg.FwuID, l.history, summarize.TriggerReasonTokenThreshold); err == nil {
                        // Build context with the new summary
                        if summarized, err := l.cfg.Summarizer.BuildContextWithSummary(ctx, l.cfg.FwuID, l.history); err == nil {
                            messages = summarized
                        }
                    }
                }
            }

            // Build a request using full history
            req := agent.Request{
                Model:  model,
                System: "You are an autonomous coding agent. " +
                    "Work independently without interactive prompts. Return only the final code changes or actionable reasoning.",
                Messages:  messages,
                MaxTokens: 4096,
            }

            resp, err := agent.DefaultRegistry.Complete(ctx, req)
            if err != nil {
                l.emit(Event{Type: EventError, ErrText: fmt.Sprintf("provider run failed: %v", err), Err: err})
                return err
            }

            // 3. Save assistant response to history
            l.history = append(l.history, agent.Message{Role: agent.RoleAssistant, Content: resp.Content})

            // On success, mark complete
            l.emit(Event{Type: EventComplete, Iteration: l.iteration})
            return nil
        }

		// Notify middleware of iteration change
		if l.middleware != nil {
			l.middleware.OnIterationChange(l.iteration)
		}

		// Start recording session evidence.
		if err := recorder.Start(l.cfg); err != nil {
			l.emit(Event{
				Type:    EventError,
				ErrText: fmt.Sprintf("failed to start session recorder: %v", err),
				Err:     err,
			})
		}

		// Create a per-iteration context so Stop can kill the process.
		iterCtx, iterCancel := context.WithCancel(ctx)
		l.mu.Lock()
		l.cancel = iterCancel
		l.mu.Unlock()

		// Build effective config (may include gate fix prompt)
		effectiveCfg := l.cfg
		if l.gateFixPrompt != "" {
			effectiveCfg.Prompt = l.cfg.Prompt + "\n\n" + l.gateFixPrompt
			// Clear after use - will be set again if gates fail
			l.gateFixPrompt = ""
		}

		proc, err := StartProcess(iterCtx, effectiveCfg, recorder.Stdout(), recorder.Stderr(), l.middleware)
		if err != nil {
			_ = recorder.Finish(1, err)
			l.uploadEvidence(ctx, recorder)
			iterCancel()
			if retryCount < l.cfg.MaxRetries {
				backoff := l.backoff(retryCount)
				l.emit(Event{
					Type:      EventRetrying,
					Iteration: l.iteration,
					ErrText:   err.Error(),
					Text:      fmt.Sprintf("attempt %d/%d, backoff %s", retryCount+1, l.cfg.MaxRetries, backoff),
				})
				l.sleep(ctx, backoff)
				retryCount++
				l.iteration-- // don't count failed spawn as iteration
				continue
			}
			l.emit(Event{
				Type:    EventError,
				ErrText: fmt.Sprintf("retries exhausted: %v", err),
				Err:     err,
			})
			return err
		}

		// Parse stdout in a goroutine.
		var wg sync.WaitGroup
		wg.Add(1)
		go func() {
			defer wg.Done()
			p := NewParser(l.events, l.iteration)
			p.tracker = l.tracker
			_ = p.Parse(proc.Stdout)
		}()

		// Capture stderr in a goroutine.
		var stderrTail string
		wg.Add(1)
		go func() {
			defer wg.Done()
			logDir := l.cfg.LogDir
			if logDir == "" {
				logDir = l.cfg.WorkDir
			}
			if logDir != "" {
				stderrTail, _ = CaptureStderr(proc.Stderr, logDir)
			} else {
				_, _ = io.Copy(io.Discard, proc.Stderr)
			}
		}()

		// Start heartbeat goroutine to keep orchestrator informed during long runs.
		heartbeatCtx, heartbeatCancel := context.WithCancel(iterCtx)
		go func() {
			ticker := time.NewTicker(30 * time.Second)
			defer ticker.Stop()
			for {
				select {
				case <-ticker.C:
					l.emit(Event{Type: EventHeartbeat, Iteration: l.iteration})
				case <-heartbeatCtx.Done():
					return
				}
			}
		}()

		// Wait for stdout/stderr goroutines to finish reading pipes.
		// This MUST happen before proc.Wait() because Wait() closes the
		// pipe file descriptors (per Go's exec.Cmd.StdoutPipe docs),
		// which would truncate in-flight reads and lose signal data.
		wg.Wait()
		heartbeatCancel()

		// Now safe to call Wait — all pipe data has been consumed.
		procErr := proc.Wait()
		iterCancel()

		// Finish recording.
		_ = recorder.Finish(getExitCode(procErr), procErr)

		// Persist middleware traces to evidence directory
		l.persistMiddlewareTraces(ctx, recorder)

		l.uploadEvidence(ctx, recorder)

		// Check if we were stopped during the process run.
		if l.stopped.Load() {
			return nil
		}

		if procErr != nil {
			// Build diagnostic error text including stderr tail if available
			errDetail := procErr.Error()
			if stderrTail != "" {
				errDetail = fmt.Sprintf("%s\nstderr: %s", procErr.Error(), stderrTail)
			}

			if retryCount < l.cfg.MaxRetries {
				backoff := l.backoff(retryCount)
				l.emit(Event{
					Type:      EventRetrying,
					Iteration: l.iteration,
					ErrText:   errDetail,
					Text:      fmt.Sprintf("attempt %d/%d, backoff %s", retryCount+1, l.cfg.MaxRetries, backoff),
				})
				l.sleep(ctx, backoff)
				retryCount++
				l.iteration-- // retry doesn't count as new iteration
				continue
			}
			l.emit(Event{
				Type:    EventError,
				ErrText: fmt.Sprintf("retries exhausted: %s", errDetail),
				Err:     procErr,
			})
			return procErr
		}

		// Clean exit — reset retry count.
		retryCount = 0

		// Signal-based completion (V2): phase-complete signal ends the loop.
		if l.tracker.PhaseComplete() {
			// Run quality gates before marking complete
			if l.cfg.QualityGates {
				gateReport := l.runQualityGates(ctx)
				if gateReport != nil && !gateReport.Passed {
					// Gates failed - check if we can retry
					l.gateRetryCount++
					if l.gateRetryCount <= l.cfg.MaxGateRetries {
						// Build fix prompt and continue loop
						l.gateFixPrompt = l.buildGateFixPrompt(gateReport, l.gateRetryCount)
						l.emit(Event{
							Type: EventGatesFixing,
							Text: fmt.Sprintf("Quality gates failed, asking executor to fix (attempt %d/%d)", l.gateRetryCount, l.cfg.MaxGateRetries),
						})
						// Reset phase complete so we can get another signal
						l.tracker.Reset()
						continue
					}
					// Max retries exceeded
					l.emit(Event{
						Type:    EventError,
						ErrText: fmt.Sprintf("Quality gates failed after %d fix attempts", l.cfg.MaxGateRetries),
					})
					return fmt.Errorf("quality gates failed: %s", gateReport.Summary)
				}
			}

			// Gates passed (or disabled) - task complete
			l.emit(Event{Type: EventComplete, Iteration: l.iteration})
			return nil
		}

		// Thrashing backstop: too many iterations without progress.
		if l.tracker.CheckThrashing(l.iteration) {
			l.emit(Event{
				Type:      EventThrashingDetected,
				Iteration: l.iteration,
				Text:      fmt.Sprintf("no progress signal in %d iterations", l.cfg.ThrashThreshold),
			})
			return nil
		}

		l.tracker.Reset()

		// Check pause after iteration completes.
		if l.paused.Load() {
			l.emit(Event{Type: EventPaused, Iteration: l.iteration})
			return nil
		}
	}
}

// Pause signals the loop to exit after the current iteration completes.
func (l *Loop) Pause() {
	l.paused.Store(true)
}

// Stop signals the loop to kill the current process and exit immediately.
func (l *Loop) Stop() {
	l.stopped.Store(true)
	l.mu.Lock()
	if l.cancel != nil {
		l.cancel()
	}
	l.mu.Unlock()
}

func (l *Loop) uploadEvidence(ctx context.Context, recorder *SessionRecorder) {
	if l.cfg.EvidenceBucket == "" || recorder.Dir() == "" {
		return
	}

	cfg := UploaderConfig{
		Bucket:   l.cfg.EvidenceBucket,
		Region:   l.cfg.EvidenceRegion,
		Endpoint: l.cfg.EvidenceEndpoint,
		Prefix:   l.cfg.EvidencePrefix,
	}

	// Use a separate context for upload to ensure it completes even if loop context is cancelled?
	// The prompt says "robustness... upload retries can happen separately".
	// But here we are inline. If the user hits Ctrl-C (ctx cancelled), we might want to try uploading anyway?
	// For now, let's use the passed context. If it's cancelled, upload stops.
	up, err := l.cfg.UploaderFactory(ctx, cfg)
	if err != nil {
		l.emit(Event{
			Type:    EventError,
			ErrText: fmt.Sprintf("failed to create evidence uploader: %v", err),
		})
		return
	}

	timestamp := filepath.Base(recorder.Dir())
	if err := up.UploadSession(ctx, recorder.Dir(), l.cfg.FwuID, timestamp); err != nil {
		l.emit(Event{
			Type:    EventError,
			ErrText: fmt.Sprintf("failed to upload evidence: %v", err),
		})
	}
}

func (l *Loop) emit(e Event) {
	now := time.Now()
	l.lastActivity.Store(&now)
	l.events <- e
}

func (l *Loop) backoff(retryCount int) time.Duration {
	if len(l.cfg.RetryBackoff) == 0 {
		return 0
	}
	idx := retryCount
	if idx >= len(l.cfg.RetryBackoff) {
		idx = len(l.cfg.RetryBackoff) - 1
	}
	return l.cfg.RetryBackoff[idx]
}

func (l *Loop) sleep(ctx context.Context, d time.Duration) {
	if d <= 0 {
		return
	}
	if l.sleepFn != nil {
		l.sleepFn(d)
		return
	}
	select {
	case <-time.After(d):
	case <-ctx.Done():
	}
}

func (l *Loop) persistMiddlewareTraces(ctx context.Context, recorder *SessionRecorder) {
	if l.middleware == nil || recorder.Dir() == "" {
		return
	}

	// Check if middleware is the Deep-Trace type
	dt, ok := l.middleware.(*DeepTraceMiddleware)
	if !ok || !dt.cfg.Enabled {
		return
	}

	traceFile := filepath.Join(recorder.Dir(), "deep_trace.jsonl")
	f, err := os.Create(traceFile) // #nosec G304
	if err != nil {
		return
	}
	defer func() { _ = f.Close() }()

	_ = dt.PersistTracesContext(ctx, f)
}

func getExitCode(err error) int {
	if err == nil {
		return 0
	}
	if exitErr, ok := err.(*exec.ExitError); ok {
		return exitErr.ExitCode()
	}
	return 1
}

// GetConfig returns a copy of the loop configuration.
func (l *Loop) GetConfig() Config {
	return l.cfg
}

// LoopHealth represents the health status of a loop.
type LoopHealth struct {
	Active       bool
	Iteration    int
	Status       string
	LastUpdate   time.Time
	LastActivity time.Time
	CurrentPID   int
}

// GetHealth returns the current health status of the loop.
func (l *Loop) GetHealth() LoopHealth {
	lastAct := time.Now()
	if ptr := l.lastActivity.Load(); ptr != nil {
		lastAct = *ptr
	}
	return LoopHealth{
		Active:       !l.stopped.Load() && !l.paused.Load(),
		Iteration:    l.iteration,
		Status:       "running", // Simplified
		LastUpdate:   time.Now(),
		LastActivity: lastAct,
		CurrentPID:   os.Getpid(),
	}
}
