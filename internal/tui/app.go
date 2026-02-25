package tui

import (
	"strconv"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// App represents the main TUI application model
type App struct {
	width          int
	height         int
	projects       []ProjectInfo
	selectedIdx    int
	viewMode       ViewMode
	help           *Help
	styles         *Styles
	source         Source
	cancelUpdate   func()
	logBuffer      []string
	logViewMode    bool
	scrollPosition int
}

// ProjectInfo holds information about a project
type ProjectInfo struct {
	Name        string
	Status      string
	Phase       string
	WorkerCount int
	Progress    int
	LastUpdate  string
}

// ViewMode represents the current view
type ViewMode int

const (
	ViewDashboard ViewMode = iota
	ViewLogs
)

// NewApp creates a new TUI application with a data source
func NewApp(source Source) *App {
	return &App{
		projects:     make([]ProjectInfo, 0),
		selectedIdx:  0,
		viewMode:     ViewDashboard,
		help:         NewHelp(),
		styles:       NewStyles(),
		source:       source,
		cancelUpdate: func() {},
		logBuffer:    make([]string, 0),
	}
}

// Init initializes the app and returns initial command
func (a *App) Init() tea.Cmd {
	return tea.Batch(
		tick(),
		a.fetchProjects(),
	)
}

// Update handles incoming messages
func (a *App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		a.width = msg.Width
		a.height = msg.Height
		return a, nil

	case tea.KeyMsg:
		return a.handleKeyInput(msg)

	case TickMsg:
		return a, tea.Batch(tick(), a.fetchProjects())

	case ProjectsMsg:
		a.projects = msg.Projects
		return a, nil

	case ProjectUpdateMsg:
		return a.handleProjectUpdate(msg)

	case ErrorMsg:
		a.logBuffer = append(a.logBuffer, "ERROR: "+msg.Err.Error())
		return a, nil
	}

	return a, nil
}

// View renders the app
func (a *App) View() string {
	if a.width == 0 || a.height == 0 {
		return ""
	}

	if a.logViewMode {
		return a.viewLogs()
	}

	return a.viewDashboard()
}

// viewDashboard renders the main dashboard
func (a *App) viewDashboard() string {
	var content string

	// Title
	title := a.styles.Title.Render("UAOS Dashboard - Multi-Project Execution")
	content += title + "\n\n"

	// Project list
	for i, proj := range a.projects {
		line := a.renderProjectLine(proj, i == a.selectedIdx)
		content += line + "\n"
	}

	// Help text
	content += "\n" + a.help.View()

	return content
}

// viewLogs renders the logs view
func (a *App) viewLogs() string {
	title := a.styles.Title.Render("Event Log")
	content := title + "\n\n"

	// Render log buffer
	start := 0
	if len(a.logBuffer) > a.height-5 {
		start = len(a.logBuffer) - (a.height - 5)
	}

	for i := start; i < len(a.logBuffer); i++ {
		content += a.logBuffer[i] + "\n"
	}

	content += "\n[ESC] Back to dashboard | [g] Top | [G] Bottom"

	return content
}

// renderProjectLine renders a single project line
func (a *App) renderProjectLine(proj ProjectInfo, selected bool) string {
	var prefix string

	if selected {
		prefix = "▶ "
	} else {
		prefix = "  "
	}

	statusStyle := a.getStatusColor(proj.Status)
	statusStr := statusStyle.Render(proj.Status)

	// Format the line
	line := prefix + proj.Name + " [" + statusStr + "] " +
		"Phase: " + proj.Phase + " | " +
		"Workers: " + formatInt(proj.WorkerCount) + " | " +
		"Progress: " + formatInt(proj.Progress) + "%"

	if selected {
		return a.styles.Selected.Render(line)
	}

	return line
}

// formatInt converts an integer to a string
func formatInt(n int) string {
	return strconv.Itoa(n)
}

// handleKeyInput handles keyboard input
func (a *App) handleKeyInput(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "ctrl+c":
		return a, tea.Quit

	case "up", "k":
		if a.selectedIdx > 0 {
			a.selectedIdx--
		}

	case "down", "j":
		if a.selectedIdx < len(a.projects)-1 {
			a.selectedIdx++
		}

	case "enter":
		if a.selectedIdx < len(a.projects) {
			a.logViewMode = true
		}

	case "esc":
		a.logViewMode = false
		a.scrollPosition = 0

	case "?":
		// Toggle help - not implemented yet

	case "1", "2", "3", "4", "5", "6", "7", "8", "9":
		// Quick select by number
		idx := int(msg.String()[0] - '0' - 1)
		if idx >= 0 && idx < len(a.projects) {
			a.selectedIdx = idx
		}

	case "g":
		if a.logViewMode {
			a.scrollPosition = 0
		}

	case "G":
		if a.logViewMode && len(a.logBuffer) > 0 {
			a.scrollPosition = len(a.logBuffer) - 1
		}
	}

	return a, nil
}

// handleProjectUpdate handles project status updates
func (a *App) handleProjectUpdate(msg ProjectUpdateMsg) (tea.Model, tea.Cmd) {
	for i, proj := range a.projects {
		if proj.Name == msg.Project.Name {
			a.projects[i] = msg.Project
			a.logBuffer = append(a.logBuffer, "["+msg.Project.Name+"] "+msg.Project.Phase+" - "+msg.Project.Status)
			break
		}
	}
	return a, nil
}

// getStatusColor returns the color style for a status
func (a *App) getStatusColor(status string) lipgloss.Style {
	switch status {
	case "running":
		return a.styles.Running
	case "paused":
		return a.styles.Paused
	case "complete":
		return a.styles.Complete
	case "error":
		return a.styles.Error
	default:
		return lipgloss.NewStyle()
	}
}

// Message types

// TickMsg is sent every second
type TickMsg struct{}

func tick() tea.Cmd {
	return tea.Tick(time.Second, func(time.Time) tea.Msg {
		return TickMsg{}
	})
}

// ProjectsMsg contains the list of projects
type ProjectsMsg struct {
	Projects []ProjectInfo
}

// ProjectUpdateMsg contains an updated project
type ProjectUpdateMsg struct {
	Project ProjectInfo
}

// ErrorMsg contains an error
type ErrorMsg struct {
	Err error
}

// Commands

func (a *App) fetchProjects() tea.Cmd {
	return func() tea.Msg {
		if a.source == nil {
			return ProjectsMsg{Projects: []ProjectInfo{}}
		}
		projects, err := a.source.List()
		if err != nil {
			return ErrorMsg{Err: err}
		}
		return ProjectsMsg{Projects: projects}
	}
}
