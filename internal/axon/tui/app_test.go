package axontui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/openexec/openexec/internal/loop"
	"github.com/openexec/openexec/internal/manager"
)

// mockSource implements Source for testing.
type mockSource struct {
	pipelines []manager.PipelineInfo
	events    chan loop.Event
}

func (s *mockSource) List() ([]manager.PipelineInfo, error) {
	return s.pipelines, nil
}

func (s *mockSource) Status(fwuID string) (manager.PipelineInfo, error) {
	for _, p := range s.pipelines {
		if p.FWUID == fwuID {
			return p, nil
		}
	}
	return manager.PipelineInfo{}, &manager.NotFoundError{FWUID: fwuID}
}

func (s *mockSource) Subscribe(fwuID string) (<-chan loop.Event, func(), error) {
	if s.events == nil {
		s.events = make(chan loop.Event, 64)
	}
	return s.events, func() {}, nil
}

func (s *mockSource) Pause(fwuID string) error { return nil }
func (s *mockSource) Stop(fwuID string) error  { return nil }

func TestApp_Init(t *testing.T) {
	source := &mockSource{
		pipelines: []manager.PipelineInfo{
			{FWUID: "FWU-001", Status: manager.StatusRunning, Phase: "IM"},
		},
	}
	app := NewApp(source)
	cmd := app.Init()
	if cmd == nil {
		t.Fatal("Init() should return a command")
	}
}

func TestApp_ViewTransitions(t *testing.T) {
	source := &mockSource{
		pipelines: []manager.PipelineInfo{
			{FWUID: "FWU-001", Status: manager.StatusRunning, Phase: "IM", Iteration: 2},
		},
	}
	app := NewApp(source)

	// Set window size.
	app.Update(tea.WindowSizeMsg{Width: 100, Height: 40})

	// Process pipeline list.
	app.Update(pipelinesMsg(source.pipelines))

	if app.view != viewDashboard {
		t.Fatalf("expected dashboard view, got %d", app.view)
	}

	// Press 't' to toggle split view.
	app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'t'}})
	if app.view != viewSplit {
		t.Fatalf("expected split view after 't', got %d", app.view)
	}

	// Press 'esc' to go back to dashboard.
	app.Update(tea.KeyMsg{Type: tea.KeyEscape})
	if app.view != viewDashboard {
		t.Fatalf("expected dashboard view after esc, got %d", app.view)
	}

	// Press 'enter' to open log view.
	app.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if app.view != viewLog {
		t.Fatalf("expected log view after enter, got %d", app.view)
	}

	// Press 'esc' to go back.
	app.Update(tea.KeyMsg{Type: tea.KeyEscape})
	if app.view != viewDashboard {
		t.Fatalf("expected dashboard view after esc from log, got %d", app.view)
	}
}

func TestApp_HelpToggle(t *testing.T) {
	source := &mockSource{}
	app := NewApp(source)
	app.Update(tea.WindowSizeMsg{Width: 100, Height: 40})

	if app.showHelp {
		t.Fatal("help should start hidden")
	}

	app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}})
	if !app.showHelp {
		t.Fatal("help should be visible after '?'")
	}

	app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}})
	if app.showHelp {
		t.Fatal("help should be hidden after second '?'")
	}
}

func TestApp_EventMsgUpdatesLogViewer(t *testing.T) {
	source := &mockSource{
		pipelines: []manager.PipelineInfo{
			{FWUID: "FWU-001", Status: manager.StatusRunning, Phase: "IM"},
		},
	}
	app := NewApp(source)
	app.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	app.Update(pipelinesMsg(source.pipelines))

	// Should have subscribed and created logViewer.
	if app.logViewer == nil {
		t.Fatal("expected logViewer to be created after pipeline list")
	}

	// Send an event.
	app.Update(eventMsg{
		fwuID: "FWU-001",
		event: loop.Event{Type: loop.EventAssistantText, Text: "working on it"},
	})

	if len(app.logViewer.lines) != 1 {
		t.Fatalf("expected 1 log line, got %d", len(app.logViewer.lines))
	}
}

func TestApp_NumberKeySelection(t *testing.T) {
	source := &mockSource{
		pipelines: []manager.PipelineInfo{
			{FWUID: "FWU-001"},
			{FWUID: "FWU-002"},
			{FWUID: "FWU-003"},
		},
	}
	app := NewApp(source)
	app.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	app.Update(pipelinesMsg(source.pipelines))

	app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'2'}})

	p, ok := app.dashboard.Selected()
	if !ok || p.FWUID != "FWU-002" {
		t.Fatalf("expected FWU-002 selected, got %s", p.FWUID)
	}
}
