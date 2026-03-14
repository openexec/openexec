package manager

import (
    "context"
    "log"
    "time"

    "github.com/openexec/openexec/internal/loop"
    "github.com/openexec/openexec/pkg/audit"
)

const subscriberBufSize = 64

// consumeEvents reads pipeline events, updates entry info, and fans out to SSE subscribers.
// Runs as a goroutine per pipeline. Closes all subscriber channels when the pipeline channel closes.
func (m *Manager) consumeEvents(fwuID string, events <-chan loop.Event) {
    for event := range events {
        // Initialize trace on first sight
        m.mu.Lock()
        e, ok := m.pipelines[fwuID]
        if ok {
            if e.traceID == "" {
                e.traceID = fwuID // deterministic default; could be uuid
            }
            e.stepSeq++
            event.TraceID = e.traceID
            event.StepID = e.stepSeq
        }
        m.mu.Unlock()
        switch event.Type {
		case loop.EventError:
			log.Printf("[Manager] Event %s [%s]: ERROR: %s", fwuID, event.Type, event.ErrText)
		case loop.EventPhaseStart, loop.EventPhaseComplete, loop.EventPipelineComplete:
			log.Printf("[Manager] Event %s [%s]: phase=%s agent=%s", fwuID, event.Type, event.Phase, event.Agent)
		case loop.EventRetrying:
			log.Printf("[Manager] Event %s [%s]: retrying - %s", fwuID, event.Type, event.Text)
		case loop.EventComplete:
			log.Printf("[Manager] Event %s [%s]: loop complete", fwuID, event.Type)
		}

        m.mu.Lock()
        e, ok = m.pipelines[fwuID]
        if ok {
            updateInfo(&e.info, event)
        }
        m.mu.Unlock()

        if ok {
            m.fanOut(fwuID, event)
        }

        // Audit run-step event if logger is configured
        if m.cfg.AuditLogger != nil {
            // Map loop event to audit severity
            severity := audit.SeverityInfo
            if event.Type == loop.EventError {
                severity = audit.SeverityError
            }
            builder, err := audit.NewEntry(audit.EventRunStep, "openexec", "system")
            if err == nil {
                md := map[string]interface{}{
                    "run_id":          fwuID,
                    "step_id":         event.StepID,
                    "trace_id":        event.TraceID,
                    "type":            event.Type,
                    "phase":           event.Phase,
                    "agent":           event.Agent,
                    "iteration":       event.Iteration,
                    "review_cycles":   event.ReviewCycle,
                    "text":            event.Text,
                    "error":           event.ErrText,
                    "prompt_hash":     event.PromptHash,
                    "artifact_hashes": []string{},
                    "artifacts":       event.Artifacts,
                    "timestamp":       time.Now().UTC().Format(time.RFC3339Nano),
                }
                // Back-compat: if patch artifact present, populate artifact_hashes
                if event.Artifacts != nil {
                    if h, ok := event.Artifacts["patch_hash"]; ok && h != "" {
                        md["artifact_hashes"] = []string{h}
                    }
                }
                e, _ := builder.WithProject(m.cfg.WorkDir).
                    WithSeverity(severity).
                    WithMetadata(md).Build()
                _ = m.cfg.AuditLogger.Log(context.Background(), e)
            }
        }

        // Write JSONL checkpoint when artifacts are present (e.g., patch applied)
        if len(event.Artifacts) > 0 {
            writeCheckpointJSONL(m.cfg.WorkDir, fwuID, event)
            // Also write to SQLite for resume/replay
            if m.cfg.AuditLogger != nil {
                writeCheckpointSQLite(m.cfg.AuditLogger.GetDB(), fwuID, event)
            }
        }
    }

	// Pipeline channel closed — close all subscriber channels.
	m.mu.Lock()
	e, ok := m.pipelines[fwuID]
	m.mu.Unlock()
	if ok {
		e.subsMu.Lock()
		for _, ch := range e.subs {
			close(ch)
		}
		e.subs = nil
		e.subsMu.Unlock()
	}
}

// updateInfo applies a single event to PipelineInfo.
func updateInfo(info *PipelineInfo, event loop.Event) {
	switch event.Type {
	case loop.EventPhaseStart:
		info.Phase = event.Phase
		info.Agent = event.Agent
		info.Status = StatusRunning
		info.ReviewCycles = event.ReviewCycle

	case loop.EventIterationStart:
		info.Iteration = event.Iteration

	case loop.EventRouteDecision:
		info.ReviewCycles = event.ReviewCycle

	case loop.EventPipelineComplete:
		info.Status = StatusComplete

	case loop.EventOperatorAttention:
		info.Status = StatusPaused

	case loop.EventPlanningMismatch:
		info.Status = StatusPaused
		info.Error = "Planning Mismatch: " + event.Text

	case loop.EventPaused:
		info.Status = StatusPaused

	case loop.EventError:
		info.Status = StatusError
		info.Error = event.ErrText
		if info.Error == "" {
			info.Error = event.Text
		}
	}
}

// fanOut sends an event to all subscribers of a pipeline using non-blocking sends.
func (m *Manager) fanOut(fwuID string, event loop.Event) {
    m.mu.RLock()
    e, ok := m.pipelines[fwuID]
    m.mu.RUnlock()
    if !ok {
        return
    }

    e.subsMu.Lock()
    defer e.subsMu.Unlock()

    for _, ch := range e.subs {
        select {
        case ch <- event:
        default:
            // Slow subscriber — drop event. Increase drop counter.
            e.drops++
        }
    }
}

// Subscribe registers an SSE subscriber for a pipeline.
// Returns a read-only event channel and an unsubscribe function.
func (m *Manager) Subscribe(fwuID string) (<-chan loop.Event, func(), error) {
	m.mu.RLock()
	e, ok := m.pipelines[fwuID]
	m.mu.RUnlock()
	if !ok {
		return nil, nil, &NotFoundError{FWUID: fwuID}
	}

	ch := make(chan loop.Event, subscriberBufSize)

	e.subsMu.Lock()
	e.subs = append(e.subs, ch)
	e.subsMu.Unlock()

	unsub := func() {
		e.subsMu.Lock()
		defer e.subsMu.Unlock()
		for i, s := range e.subs {
			if s == ch {
				e.subs = append(e.subs[:i], e.subs[i+1:]...)
				break
			}
		}
	}

	return ch, unsub, nil
}

// NotFoundError is returned when a pipeline is not found.
type NotFoundError struct {
	FWUID string
}

func (e *NotFoundError) Error() string {
	return "pipeline " + e.FWUID + " not found"
}
