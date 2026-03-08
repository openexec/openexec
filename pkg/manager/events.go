package manager

import (
	"log"

	"github.com/openexec/openexec/internal/loop"
)

const subscriberBufSize = 64

// consumeEvents reads pipeline events, updates entry info, and fans out to SSE subscribers.
// Runs as a goroutine per pipeline. Closes all subscriber channels when the pipeline channel closes.
func (m *Manager) consumeEvents(fwuID string, events <-chan loop.Event) {
	for event := range events {
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
		e, ok := m.pipelines[fwuID]
		if ok {
			updateInfo(&e.info, event)
		}
		m.mu.Unlock()

		if ok {
			m.fanOut(fwuID, event)
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
			// Slow subscriber — drop event.
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
