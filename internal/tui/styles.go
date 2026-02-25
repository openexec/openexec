package tui

import "github.com/charmbracelet/lipgloss"

// Styles holds all styling definitions
type Styles struct {
	Title    lipgloss.Style
	Selected lipgloss.Style
	Running  lipgloss.Style
	Paused   lipgloss.Style
	Complete lipgloss.Style
	Error    lipgloss.Style
	Help     lipgloss.Style
	Dim      lipgloss.Style
}

// NewStyles creates and returns a new Styles struct
func NewStyles() *Styles {
	return &Styles{
		Title: lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("39")).
			MarginBottom(1),

		Selected: lipgloss.NewStyle().
			Background(lipgloss.Color("39")).
			Foreground(lipgloss.Color("255")).
			Bold(true).
			Padding(0, 1),

		Running: lipgloss.NewStyle().
			Foreground(lipgloss.Color("46")).
			Bold(true),

		Paused: lipgloss.NewStyle().
			Foreground(lipgloss.Color("226")).
			Bold(true),

		Complete: lipgloss.NewStyle().
			Foreground(lipgloss.Color("42")).
			Bold(true),

		Error: lipgloss.NewStyle().
			Foreground(lipgloss.Color("196")).
			Bold(true),

		Help: lipgloss.NewStyle().
			Foreground(lipgloss.Color("246")).
			Italic(true),

		Dim: lipgloss.NewStyle().
			Foreground(lipgloss.Color("59")),
	}
}
