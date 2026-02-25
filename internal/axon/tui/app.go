package axontui

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/openexec/openexec/internal/loop"
	"github.com/openexec/openexec/internal/manager"
)

type viewMode int

const (
	viewDashboard viewMode = iota
	viewSplit
	viewLog
)

// Messages.
type tickMsg struct{}
type pipelinesMsg []manager.PipelineInfo
type eventMsg struct {
	fwuID string
	event loop.Event
	ch    <-chan loop.Event // pass channel back so we can continue reading
}
type errMsg struct{ err error }

// App is the top-level Bubble Tea model for the Axon TUI.
type App struct {
	source     Source
	dashboard  Dashboard
	logViewer  *LogViewer
	view       viewMode
	showHelp   bool
	statusText string
	width      int
	height     int
	err        error

	// Track which FWU we're subscribed to.
	subscribedFWU string
}

// NewApp creates the TUI app with the given Source.
func NewApp(source Source) *App {
	return &App{
		source: source,
		view:   viewDashboard,
	}
}

func (a *App) Init() tea.Cmd {
	return tea.Batch(a.tick(), a.fetchPipelines())
}

func (a *App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		a.width = msg.Width
		a.height = msg.Height
		a.dashboard.SetSize(msg.Width, msg.Height-4)
		if a.logViewer != nil {
			a.logViewer.SetSize(msg.Width, a.logViewHeight())
		}
		return a, nil

	case tea.KeyMsg:
		cmd := a.handleKey(msg)
		return a, cmd

	case tickMsg:
		return a, tea.Batch(a.tick(), a.fetchPipelines())

	case pipelinesMsg:
		a.dashboard.SetPipelines([]manager.PipelineInfo(msg))
		// Auto-subscribe if we have pipelines but no subscription.
		if a.logViewer == nil && len(msg) > 0 {
			return a, a.subscribeToCurrent()
		}
		return a, nil

	case eventMsg:
		if a.logViewer != nil && msg.fwuID == a.logViewer.fwuID {
			a.logViewer.AppendEvent(msg.event)
			a.statusText = renderStatusEvent(msg.event)
			// Continue listening for more events from this subscription.
			if msg.ch != nil {
				return a, a.listenEvents(msg.fwuID, msg.ch)
			}
		}
		return a, nil

	case errMsg:
		if msg.err != nil {
			a.err = msg.err
			a.statusText = styleErrorTxt.Render(msg.err.Error())
		}
		return a, nil
	}

	return a, nil
}

func (a *App) View() string {
	if a.width == 0 {
		return "Loading..."
	}

	var content string

	switch a.view {
	case viewDashboard:
		content = a.viewDashboard()
	case viewSplit:
		content = a.viewSplit()
	case viewLog:
		content = a.viewLog()
	}

	// Status bar at the bottom.
	status := a.renderStatusBar()
	content = lipgloss.JoinVertical(lipgloss.Left, content, status)

	// Help overlay on top of everything.
	if a.showHelp {
		return renderHelp(a.width, a.height)
	}

	return content
}

func (a *App) viewDashboard() string {
	a.dashboard.SetSize(a.width, a.height-2)
	return a.dashboard.View()
}

func (a *App) viewSplit() string {
	dashHeight := a.height / 3
	logHeight := a.height - dashHeight - 3 // status bar + divider

	a.dashboard.SetSize(a.width, dashHeight)
	if a.logViewer != nil {
		a.logViewer.SetSize(a.width, logHeight)
	}

	dash := a.dashboard.View()
	divider := styleDim.Render(lipgloss.NewStyle().Width(a.width).Render("─"))

	logContent := styleDim.Render("  No pipeline selected")
	if a.logViewer != nil {
		logContent = a.logViewer.View()
	}

	return lipgloss.JoinVertical(lipgloss.Left,
		lipgloss.NewStyle().Height(dashHeight).Render(dash),
		divider,
		lipgloss.NewStyle().Height(logHeight).Render(logContent),
	)
}

func (a *App) viewLog() string {
	if a.logViewer == nil {
		return styleDim.Render("  No pipeline selected. Press Esc to go back.")
	}
	a.logViewer.SetSize(a.width, a.height-2)
	header := styleTitle.Render("Log: " + a.logViewer.fwuID)
	return lipgloss.JoinVertical(lipgloss.Left,
		header,
		a.logViewer.View(),
	)
}

func (a *App) renderStatusBar() string {
	left := a.statusText
	right := "p:pause  s:stop  t:log  ?:help  q:quit"
	gap := a.width - lipgloss.Width(left) - lipgloss.Width(right) - 2
	if gap < 1 {
		gap = 1
	}
	bar := left + lipgloss.NewStyle().Width(gap).Render("") + styleDim.Render(right)
	return styleStatusBar.Width(a.width).Render(bar)
}

func (a *App) logViewHeight() int {
	switch a.view {
	case viewSplit:
		return a.height - a.height/3 - 3
	default:
		return a.height - 2
	}
}

// tick returns a command that sends a tickMsg after 1 second.
func (a *App) tick() tea.Cmd {
	return tea.Tick(time.Second, func(time.Time) tea.Msg {
		return tickMsg{}
	})
}

// fetchPipelines queries the source for pipeline list.
func (a *App) fetchPipelines() tea.Cmd {
	return func() tea.Msg {
		list, err := a.source.List()
		if err != nil {
			return errMsg{err: err}
		}
		return pipelinesMsg(list)
	}
}

// subscribeToCurrent sets up an event subscription for the currently selected pipeline.
func (a *App) subscribeToCurrent() tea.Cmd {
	p, ok := a.dashboard.Selected()
	if !ok {
		return nil
	}

	// Already subscribed to this one.
	if a.subscribedFWU == p.FWUID {
		return nil
	}

	// Close previous subscription.
	if a.logViewer != nil {
		a.logViewer.Close()
	}

	sub, unsub, err := a.source.Subscribe(p.FWUID)
	if err != nil {
		a.logViewer = NewLogViewer(p.FWUID, nil)
		a.logViewer.SetSize(a.width, a.logViewHeight())
		a.subscribedFWU = p.FWUID
		return func() tea.Msg {
			return errMsg{err: err}
		}
	}

	a.logViewer = NewLogViewer(p.FWUID, unsub)
	a.logViewer.SetSize(a.width, a.logViewHeight())
	a.subscribedFWU = p.FWUID

	return a.listenEvents(p.FWUID, sub)
}

// listenEvents returns a command that reads events from a subscription channel.
func (a *App) listenEvents(fwuID string, ch <-chan loop.Event) tea.Cmd {
	return func() tea.Msg {
		event, ok := <-ch
		if !ok {
			return nil
		}
		return eventMsg{fwuID: fwuID, event: event, ch: ch}
	}
}

// renderStatusEvent produces a short status text for the status bar.
func renderStatusEvent(e loop.Event) string {
	switch e.Type {
	case loop.EventPhaseStart:
		return styleSignal.Render("Phase " + e.Phase + " (" + e.Agent + ")")
	case loop.EventAssistantText:
		text := e.Text
		if len(text) > 80 {
			text = text[:77] + "..."
		}
		return text
	case loop.EventSignalReceived:
		return styleSignal.Render("[" + e.SignalType + "] " + e.Text)
	case loop.EventError:
		return styleErrorTxt.Render("Error: " + e.ErrText)
	case loop.EventPipelineComplete:
		return styleComplete.Render("Pipeline complete")
	default:
		return ""
	}
}
