package axontui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/openexec/openexec/internal/manager"
)

// Dashboard displays the FWU list and detail panel side by side.
type Dashboard struct {
	pipelines []manager.PipelineInfo
	cursor    int
	width     int
	height    int
}

// SetPipelines updates the dashboard pipeline list.
func (d *Dashboard) SetPipelines(ps []manager.PipelineInfo) {
	d.pipelines = ps
	if d.cursor >= len(ps) && len(ps) > 0 {
		d.cursor = len(ps) - 1
	}
}

// Selected returns the currently selected pipeline info, if any.
func (d *Dashboard) Selected() (manager.PipelineInfo, bool) {
	if len(d.pipelines) == 0 || d.cursor >= len(d.pipelines) {
		return manager.PipelineInfo{}, false
	}
	return d.pipelines[d.cursor], true
}

// CursorUp moves the cursor up.
func (d *Dashboard) CursorUp() {
	if d.cursor > 0 {
		d.cursor--
	}
}

// CursorDown moves the cursor down.
func (d *Dashboard) CursorDown() {
	if d.cursor < len(d.pipelines)-1 {
		d.cursor++
	}
}

// SelectByIndex selects a pipeline by 1-based index.
func (d *Dashboard) SelectByIndex(idx int) {
	if idx >= 1 && idx <= len(d.pipelines) {
		d.cursor = idx - 1
	}
}

// SetSize updates the dashboard dimensions.
func (d *Dashboard) SetSize(w, h int) {
	d.width = w
	d.height = h
}

// View renders the dashboard.
func (d *Dashboard) View() string {
	if len(d.pipelines) == 0 {
		return styleDim.Render("  No pipelines running. Start one via HTTP API or embedded mode.")
	}

	listWidth := d.width * 2 / 5
	if listWidth < 25 {
		listWidth = 25
	}
	detailWidth := d.width - listWidth - 3 // border + padding

	list := d.renderList(listWidth)
	detail := d.renderDetail(detailWidth)

	return lipgloss.JoinHorizontal(lipgloss.Top,
		lipgloss.NewStyle().Width(listWidth).Render(list),
		styleDim.Render(" │ "),
		lipgloss.NewStyle().Width(detailWidth).Render(detail),
	)
}

func (d *Dashboard) renderList(width int) string {
	var b strings.Builder
	b.WriteString(styleTitle.Render("Pipelines"))
	b.WriteString("\n")

	for i, p := range d.pipelines {
		icon := statusIcon(p.Status)
		phase := ""
		if p.Phase != "" {
			phase = fmt.Sprintf("[%s]", p.Phase)
		}
		iter := ""
		if p.Iteration > 0 {
			iter = fmt.Sprintf("iter %d", p.Iteration)
		}

		line := fmt.Sprintf(" %s %-16s %4s %s", icon, p.FWUID, phase, iter)

		// Truncate to width.
		if len(line) > width {
			line = line[:width-1] + "…"
		}

		if i == d.cursor {
			line = styleSelected.Render(line)
		}
		b.WriteString(line)
		if i < len(d.pipelines)-1 {
			b.WriteString("\n")
		}
	}
	return b.String()
}

func (d *Dashboard) renderDetail(width int) string {
	p, ok := d.Selected()
	if !ok {
		return styleDim.Render("No pipeline selected")
	}

	var b strings.Builder

	b.WriteString(styleTitle.Render(p.FWUID))
	b.WriteString("\n\n")
	b.WriteString(RenderPhaseProgress(p.Phase, p.ReviewCycles))
	b.WriteString("\n\n")
	b.WriteString(fmt.Sprintf("  Status:        %s %s\n", statusIcon(p.Status), string(p.Status)))
	if p.Phase != "" {
		b.WriteString(fmt.Sprintf("  Phase:         %s\n", p.Phase))
		b.WriteString(fmt.Sprintf("  Agent:         %s\n", p.Agent))
	}
	if p.Iteration > 0 {
		b.WriteString(fmt.Sprintf("  Iteration:     %d\n", p.Iteration))
	}
	b.WriteString(fmt.Sprintf("  Elapsed:       %s\n", p.Elapsed))
	if p.ReviewCycles > 0 {
		b.WriteString(fmt.Sprintf("  Review cycles: %d\n", p.ReviewCycles))
	}
	if p.Error != "" {
		b.WriteString(fmt.Sprintf("  Error:         %s\n", styleErrorTxt.Render(p.Error)))
	}

	return b.String()
}

func statusIcon(status manager.PipelineStatus) string {
	switch status {
	case manager.StatusRunning, manager.StatusStarting:
		return styleRunning.Render("▶")
	case manager.StatusPaused:
		return stylePaused.Render("⏸")
	case manager.StatusComplete:
		return styleComplete.Render("✓")
	case manager.StatusError, manager.StatusStopped:
		return styleError.Render("✗")
	default:
		return "○"
	}
}
