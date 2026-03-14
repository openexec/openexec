package manager

import (
    "context"
    "encoding/json"
    "fmt"
    "log"
    "time"

    "github.com/google/uuid"
    "github.com/openexec/openexec/internal/loop"
    "github.com/openexec/openexec/internal/mcp"
    "github.com/openexec/openexec/internal/prompt"
    "github.com/openexec/openexec/pkg/audit"
    "github.com/openexec/openexec/pkg/db/state"
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
                    // Version metadata for reproducibility and debugging
                    "prompt_version":            prompt.PromptVersion,
                    "tool_registry_version":     mcp.ToolRegistryVersion,
                    "run_state_machine_version": prompt.RunStateMachineVersion,
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
            if m.state != nil {
                writeCheckpointSQLite(m.state.GetDB(), fwuID, event)
            }
        }

        // Parallel write to unified DB (non-blocking)
        if m.state != nil {
            m.writeRunStepAsync(fwuID, event)
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

// writeRunStepAsync writes run step and artifact data to the unified DB asynchronously.
// This is a parallel write that doesn't block event processing.
func (m *Manager) writeRunStepAsync(runID string, event loop.Event) {
    if m.state == nil {
        return
    }

    // Use WriteAsync for non-blocking parallel writes
    m.state.WriteAsync(context.Background(), func(ctx context.Context) error {
        // Write run step for phase events
        if event.Type == loop.EventPhaseStart || event.Type == loop.EventPhaseComplete ||
           event.Type == loop.EventIterationStart || event.Type == loop.EventComplete {
            stepID := fmt.Sprintf("%s-%d", runID, event.StepID)

            // Build metadata JSON
            md := map[string]interface{}{
                "type":         event.Type,
                "text":         event.Text,
                "prompt_hash":  event.PromptHash,
                "review_cycle": event.ReviewCycle,
            }
            mdJSON, _ := json.Marshal(md)

            status := "running"
            if event.Type == loop.EventPhaseComplete || event.Type == loop.EventComplete {
                status = "completed"
            }

            err := m.state.AddRunStepFull(ctx,
                stepID, runID, event.TraceID,
                event.Phase, event.Agent, event.Iteration,
                status, event.PromptHash, string(mdJSON))
            if err != nil {
                log.Printf("[Manager] Parallel DB write (run_step) failed: %v", err)
            }
        }

        // Write artifacts if present
        if len(event.Artifacts) > 0 {
            for hash, path := range event.Artifacts {
                if hash == "" || path == "" {
                    continue
                }
                // Determine artifact type from path or default
                artifactType := "patch"
                if len(path) > 0 {
                    if path[len(path)-4:] == ".log" {
                        artifactType = "test_log"
                    }
                }
                // Record artifact (size 0 as placeholder - actual size computed on write)
                err := m.state.RecordArtifact(ctx, hash, artifactType, path, 0)
                if err != nil {
                    log.Printf("[Manager] Parallel DB write (artifact) failed: %v", err)
                }
            }

            // Write checkpoint for resume support
            cp := state.CheckpointData{
                ID:        uuid.New().String(),
                RunID:     runID,
                Phase:     event.Phase,
                Iteration: event.Iteration,
                Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
                Artifacts: event.Artifacts,
            }
            if err := m.state.RecordCheckpoint(ctx, cp); err != nil {
                log.Printf("[Manager] Parallel DB write (checkpoint) failed: %v", err)
            }
        }

        return nil
    })
}
