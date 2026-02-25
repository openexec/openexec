package axontui

import "github.com/charmbracelet/lipgloss"

var helpText = `Axon TUI — Keyboard Shortcuts

 Navigation
   j/k, ↑/↓   Move cursor in list
   Enter       Open log view for selected FWU
   Esc         Return to dashboard
   1-9         Select FWU by index
   t           Toggle split view

 Lifecycle
   p           Pause selected FWU
   s           Stop selected FWU

 Log Viewer
   G           Jump to bottom (auto-scroll on)
   g           Jump to top

 General
   ?           Toggle this help
   q           Quit`

// renderHelp renders a centered help overlay.
func renderHelp(width, height int) string {
	box := styleHelp.Render(helpText)
	return lipgloss.Place(width, height,
		lipgloss.Center, lipgloss.Center,
		box)
}
