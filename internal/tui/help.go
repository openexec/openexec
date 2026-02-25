package tui

import "strings"

// Help provides keyboard navigation help
type Help struct {
	styles *Styles
}

// NewHelp creates a new Help instance
func NewHelp() *Help {
	return &Help{
		styles: NewStyles(),
	}
}

// View renders the help text
func (h *Help) View() string {
	helpText := strings.Join([]string{
		"Keyboard shortcuts:",
		"  j/↓     - Move down",
		"  k/↑     - Move up",
		"  Enter   - View project logs",
		"  Esc     - Back to dashboard",
		"  1-9     - Quick select project",
		"  g       - Jump to top",
		"  G       - Jump to bottom",
		"  ?       - Toggle help",
		"  q/C-c   - Quit",
	}, "\n")

	return h.styles.Dim.Render(helpText)
}

// Dashboard provides dashboard-specific help
func (h *Help) Dashboard() string {
	return "Press '?' for help | 'q' to quit"
}

// Logs provides log viewer help
func (h *Help) Logs() string {
	return "[ESC] Back | [g] Top | [G] Bottom | [q] Quit"
}
