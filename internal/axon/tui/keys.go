package axontui

import tea "github.com/charmbracelet/bubbletea"

// handleKey processes key events and returns a command if needed.
func (a *App) handleKey(msg tea.KeyMsg) tea.Cmd {
	// Global keys work in all views.
	switch msg.String() {
	case "q", "ctrl+c":
		return tea.Quit
	case "?":
		a.showHelp = !a.showHelp
		return nil
	}

	// Help overlay consumes all other keys.
	if a.showHelp {
		if msg.String() == "esc" {
			a.showHelp = false
		}
		return nil
	}

	// Number keys select FWU by index in any view.
	if msg.String() >= "1" && msg.String() <= "9" {
		idx := int(msg.String()[0] - '0')
		a.dashboard.SelectByIndex(idx)
		return a.subscribeToCurrent()
	}

	// Lifecycle keys work in any view.
	switch msg.String() {
	case "p":
		return a.pauseSelected()
	case "s":
		return a.stopSelected()
	}

	// View-specific keys.
	switch a.view {
	case viewDashboard:
		return a.handleDashboardKey(msg)
	case viewSplit:
		return a.handleSplitKey(msg)
	case viewLog:
		return a.handleLogKey(msg)
	}

	return nil
}

func (a *App) handleDashboardKey(msg tea.KeyMsg) tea.Cmd {
	switch msg.String() {
	case "j", "down":
		a.dashboard.CursorDown()
		return a.subscribeToCurrent()
	case "k", "up":
		a.dashboard.CursorUp()
		return a.subscribeToCurrent()
	case "enter":
		if _, ok := a.dashboard.Selected(); ok {
			a.view = viewLog
		}
		return nil
	case "t":
		a.view = viewSplit
		return nil
	}
	return nil
}

func (a *App) handleSplitKey(msg tea.KeyMsg) tea.Cmd {
	switch msg.String() {
	case "j", "down":
		a.dashboard.CursorDown()
		return a.subscribeToCurrent()
	case "k", "up":
		a.dashboard.CursorUp()
		return a.subscribeToCurrent()
	case "enter":
		if _, ok := a.dashboard.Selected(); ok {
			a.view = viewLog
		}
		return nil
	case "t", "esc":
		a.view = viewDashboard
		return nil
	}
	return nil
}

func (a *App) handleLogKey(msg tea.KeyMsg) tea.Cmd {
	switch msg.String() {
	case "esc":
		a.view = viewDashboard
		return nil
	case "t":
		a.view = viewSplit
		return nil
	case "G":
		if a.logViewer != nil {
			a.logViewer.GoToBottom()
		}
		return nil
	case "g":
		if a.logViewer != nil {
			a.logViewer.GoToTop()
		}
		return nil
	case "j", "down":
		if a.logViewer != nil {
			a.logViewer.ScrollDown(1)
		}
		return nil
	case "k", "up":
		if a.logViewer != nil {
			a.logViewer.ScrollUp(1)
		}
		return nil
	}
	return nil
}

func (a *App) pauseSelected() tea.Cmd {
	p, ok := a.dashboard.Selected()
	if !ok {
		return nil
	}
	return func() tea.Msg {
		err := a.source.Pause(p.FWUID)
		if err != nil {
			return errMsg{err: err}
		}
		return nil
	}
}

func (a *App) stopSelected() tea.Cmd {
	p, ok := a.dashboard.Selected()
	if !ok {
		return nil
	}
	return func() tea.Msg {
		err := a.source.Stop(p.FWUID)
		if err != nil {
			return errMsg{err: err}
		}
		return nil
	}
}
