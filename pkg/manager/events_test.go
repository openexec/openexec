package manager

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/openexec/openexec/internal/loop"
	"github.com/openexec/openexec/pkg/db/state"
)

func TestUpdateInfoStageStart(t *testing.T) {
	info := &PipelineInfo{Status: StatusStarting}
	updateInfo(info, loop.Event{
		Type:      loop.EventStageStart,
		StageName: "implement",
		Iteration: 1,
	})
	if info.Status != StatusRunning {
		t.Errorf("status = %s, want running", info.Status)
	}
	if info.Stage != "implement" {
		t.Errorf("stage = %s, want implement", info.Stage)
	}
	if info.Iteration != 1 {
		t.Errorf("iteration = %d, want 1", info.Iteration)
	}
}

func TestUpdateInfoIterationStart(t *testing.T) {
	info := &PipelineInfo{Status: StatusRunning}
	updateInfo(info, loop.Event{
		Type:      loop.EventIterationStart,
		Iteration: 3,
	})
	if info.Iteration != 3 {
		t.Errorf("iteration = %d, want 3", info.Iteration)
	}
}

func TestUpdateInfoRouteDecision(t *testing.T) {
	info := &PipelineInfo{Status: StatusRunning, ReviewCycles: 0}
	updateInfo(info, loop.Event{
		Type:        loop.EventRouteDecision,
		ReviewCycle: 1,
	})
	if info.ReviewCycles != 1 {
		t.Errorf("review_cycles = %d, want 1", info.ReviewCycles)
	}
}

func TestUpdateInfoPipelineComplete(t *testing.T) {
	info := &PipelineInfo{Status: StatusRunning}
	updateInfo(info, loop.Event{Type: loop.EventPipelineComplete})
	if info.Status != StatusComplete {
		t.Errorf("status = %s, want complete", info.Status)
	}
}

func TestUpdateInfoOperatorAttention(t *testing.T) {
	info := &PipelineInfo{Status: StatusRunning}
	updateInfo(info, loop.Event{Type: loop.EventOperatorAttention})
	if info.Status != StatusPaused {
		t.Errorf("status = %s, want paused", info.Status)
	}
}

func TestUpdateInfoPaused(t *testing.T) {
	info := &PipelineInfo{Status: StatusRunning}
	updateInfo(info, loop.Event{Type: loop.EventPaused})
	if info.Status != StatusPaused {
		t.Errorf("status = %s, want paused", info.Status)
	}
}

func TestUpdateInfoError(t *testing.T) {
	info := &PipelineInfo{Status: StatusRunning}
	updateInfo(info, loop.Event{Type: loop.EventError, ErrText: "something broke"})
	if info.Status != StatusError {
		t.Errorf("status = %s, want error", info.Status)
	}
	if info.Error != "something broke" {
		t.Errorf("error = %q, want %q", info.Error, "something broke")
	}
}

func TestUpdateInfoErrorFallbackText(t *testing.T) {
	info := &PipelineInfo{Status: StatusRunning}
	updateInfo(info, loop.Event{Type: loop.EventError, Text: "fallback error"})
	if info.Error != "fallback error" {
		t.Errorf("error = %q, want %q", info.Error, "fallback error")
	}
}

func TestConsumeEventsFanOut(t *testing.T) {
	tmpDir := t.TempDir()
	stateStore, err := state.NewStore(filepath.Join(tmpDir, "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	m, err := New(Config{WorkDir: tmpDir, StateStore: stateStore})
	if err != nil {
		t.Fatal(err)
	}

	ch := make(chan loop.Event, 16)
	e := &entry{
		info: PipelineInfo{FWUID: "FWU-01", Status: StatusStarting},
	}
	m.pipelines["FWU-01"] = e

	// Subscribe two listeners.
	sub1, unsub1, err := m.Subscribe("FWU-01")
	if err != nil {
		t.Fatal(err)
	}
	defer unsub1()

	sub2, unsub2, err := m.Subscribe("FWU-01")
	if err != nil {
		t.Fatal(err)
	}
	defer unsub2()

	// Start consuming events.
	go m.consumeEvents("FWU-01", ch)

	// Send event.
	ch <- loop.Event{Type: loop.EventStageStart, StageName: "implement"}

	// Both subscribers should receive it.
	select {
	case ev := <-sub1:
		if ev.Type != loop.EventStageStart {
			t.Errorf("sub1 type = %s, want stage_start", ev.Type)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("sub1 timeout")
	}

	select {
	case ev := <-sub2:
		if ev.Type != loop.EventStageStart {
			t.Errorf("sub2 type = %s, want stage_start", ev.Type)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("sub2 timeout")
	}

	close(ch)
}

func TestConsumeEventsClosesSubs(t *testing.T) {
	tmpDir := t.TempDir()
	stateStore, err := state.NewStore(filepath.Join(tmpDir, "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	m, err := New(Config{WorkDir: tmpDir, StateStore: stateStore})
	if err != nil {
		t.Fatal(err)
	}

	ch := make(chan loop.Event, 16)
	e := &entry{
		info: PipelineInfo{FWUID: "FWU-01", Status: StatusStarting},
	}
	m.pipelines["FWU-01"] = e

	sub, _, err := m.Subscribe("FWU-01")
	if err != nil {
		t.Fatal(err)
	}

	done := make(chan struct{})
	go func() {
		m.consumeEvents("FWU-01", ch)
		close(done)
	}()

	// Close the pipeline event channel.
	close(ch)
	<-done

	// Subscriber channel should be closed.
	select {
	case _, ok := <-sub:
		if ok {
			t.Error("expected subscriber channel to be closed")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for subscriber channel close")
	}
}

func TestConsumeEventsSlowSubscriber(t *testing.T) {
	tmpDir := t.TempDir()
	stateStore, err := state.NewStore(filepath.Join(tmpDir, "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	m, err := New(Config{WorkDir: tmpDir, StateStore: stateStore})
	if err != nil {
		t.Fatal(err)
	}

	ch := make(chan loop.Event, 128)
	e := &entry{
		info: PipelineInfo{FWUID: "FWU-01", Status: StatusStarting},
	}
	m.pipelines["FWU-01"] = e

	// Subscribe but never read from the channel.
	_, _, err = m.Subscribe("FWU-01")
	if err != nil {
		t.Fatal(err)
	}

	done := make(chan struct{})
	go func() {
		m.consumeEvents("FWU-01", ch)
		close(done)
	}()

	// Send more events than the subscriber buffer can hold.
	for i := 0; i < subscriberBufSize+10; i++ {
		ch <- loop.Event{Type: loop.EventIterationStart, Iteration: i}
	}
	close(ch)

	// Consumer should complete without blocking.
	select {
	case <-done:
		// OK
	case <-time.After(5 * time.Second):
		t.Fatal("consumeEvents blocked on slow subscriber")
	}
}
