package axontui

import "github.com/charmbracelet/lipgloss"

var (
	// Status colors.
	colorRunning  = lipgloss.Color("42")  // green
	colorPaused   = lipgloss.Color("214") // yellow
	colorComplete = lipgloss.Color("75")  // blue
	colorError    = lipgloss.Color("196") // red
	colorSignal   = lipgloss.Color("81")  // cyan
	colorTool     = lipgloss.Color("177") // magenta
	colorDim      = lipgloss.Color("240") // gray

	// Status icon styles.
	styleRunning  = lipgloss.NewStyle().Foreground(colorRunning)
	stylePaused   = lipgloss.NewStyle().Foreground(colorPaused)
	styleComplete = lipgloss.NewStyle().Foreground(colorComplete)
	styleError    = lipgloss.NewStyle().Foreground(colorError)

	// Phase badge styles.
	stylePhaseActive = lipgloss.NewStyle().Bold(true).Foreground(colorRunning)
	stylePhaseDone   = lipgloss.NewStyle().Foreground(colorDim)
	stylePhasePend   = lipgloss.NewStyle().Faint(true)

	// Content styles.
	styleSignal   = lipgloss.NewStyle().Foreground(colorSignal)
	styleTool     = lipgloss.NewStyle().Foreground(colorTool)
	styleErrorTxt = lipgloss.NewStyle().Foreground(colorError)
	styleDim      = lipgloss.NewStyle().Foreground(colorDim)

	// Layout styles.
	styleSelected  = lipgloss.NewStyle().Reverse(true)
	styleTitle     = lipgloss.NewStyle().Bold(true).Padding(0, 1)
	styleStatusBar = lipgloss.NewStyle().
			Background(lipgloss.Color("236")).
			Padding(0, 1)
	styleHelp = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			Padding(1, 2)
)
