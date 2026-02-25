package axontui

import (
	"fmt"
	"strings"

	"github.com/openexec/openexec/internal/loop"
)

// LogViewer displays streaming events from a single pipeline.
type LogViewer struct {
	fwuID      string
	lines      []string
	offset     int // scroll offset from bottom (0 = at bottom)
	autoScroll bool
	width      int
	height     int
	unsub      func()
}

// NewLogViewer creates a LogViewer for the given FWU.
func NewLogViewer(fwuID string, unsub func()) *LogViewer {
	return &LogViewer{
		fwuID:      fwuID,
		autoScroll: true,
		unsub:      unsub,
	}
}

// Close unsubscribes from the event stream.
func (v *LogViewer) Close() {
	if v.unsub != nil {
		v.unsub()
		v.unsub = nil
	}
}

// SetSize updates the viewer dimensions.
func (v *LogViewer) SetSize(w, h int) {
	v.width = w
	v.height = h
}

// AppendEvent renders an event and appends it to the log.
func (v *LogViewer) AppendEvent(event loop.Event) {
	line := renderEvent(event)
	if line == "" {
		return
	}
	v.lines = append(v.lines, line)
	// Keep a reasonable buffer.
	if len(v.lines) > 10000 {
		v.lines = v.lines[len(v.lines)-5000:]
	}
}

// ScrollUp moves the viewport up.
func (v *LogViewer) ScrollUp(n int) {
	v.autoScroll = false
	v.offset += n
	max := len(v.lines) - v.height
	if max < 0 {
		max = 0
	}
	if v.offset > max {
		v.offset = max
	}
}

// ScrollDown moves the viewport down.
func (v *LogViewer) ScrollDown(n int) {
	v.offset -= n
	if v.offset <= 0 {
		v.offset = 0
		v.autoScroll = true
	}
}

// GoToBottom jumps to the bottom and re-enables auto-scroll.
func (v *LogViewer) GoToBottom() {
	v.offset = 0
	v.autoScroll = true
}

// GoToTop jumps to the top.
func (v *LogViewer) GoToTop() {
	v.autoScroll = false
	max := len(v.lines) - v.height
	if max < 0 {
		max = 0
	}
	v.offset = max
}

// View renders the log viewer.
func (v *LogViewer) View() string {
	viewHeight := v.height
	if viewHeight <= 0 {
		viewHeight = 20
	}

	if len(v.lines) == 0 {
		return styleDim.Render("  Waiting for events...")
	}

	// Calculate visible window.
	end := len(v.lines) - v.offset
	if end < 0 {
		end = 0
	}
	start := end - viewHeight
	if start < 0 {
		start = 0
	}

	visible := v.lines[start:end]

	var b strings.Builder
	for i, line := range visible {
		// Truncate long lines.
		if len(line) > v.width && v.width > 0 {
			line = line[:v.width-1] + "…"
		}
		b.WriteString(line)
		if i < len(visible)-1 {
			b.WriteString("\n")
		}
	}

	// Streaming cursor at the end if auto-scrolling.
	if v.autoScroll && v.offset == 0 {
		b.WriteString("▌")
	}

	return b.String()
}

// renderEvent converts a loop.Event into a display string.
func renderEvent(e loop.Event) string {
	switch e.Type {
	case loop.EventPhaseStart:
		sep := strings.Repeat("─", 20)
		return styleSignal.Render(fmt.Sprintf("%s Phase %s (%s) %s", sep, e.Phase, e.Agent, sep))

	case loop.EventIterationStart:
		return styleDim.Render(fmt.Sprintf("── iteration %d ──", e.Iteration))

	case loop.EventAssistantText:
		return e.Text

	case loop.EventToolStart:
		input := ""
		if path, ok := e.ToolInput["file_path"]; ok {
			input = fmt.Sprintf("  %v", path)
		} else if cmd, ok := e.ToolInput["command"]; ok {
			s := fmt.Sprintf("%v", cmd)
			if len(s) > 60 {
				s = s[:57] + "..."
			}
			input = "  " + s
		}
		return styleTool.Render(fmt.Sprintf("🔧 %s%s", e.Tool, input))

	case loop.EventToolResult:
		text := e.Text
		// Truncate to 3 lines.
		lines := strings.SplitN(text, "\n", 4)
		if len(lines) > 3 {
			lines = append(lines[:3], "…")
		}
		return styleDim.Render("  " + strings.Join(lines, "\n  "))

	case loop.EventSignalReceived:
		if e.SignalType == "route" {
			return styleSignal.Render(fmt.Sprintf("[ROUTE → %s] %q", e.SignalTarget, e.Text))
		}
		label := strings.ToUpper(e.SignalType)
		reason := ""
		if e.Text != "" {
			reason = fmt.Sprintf(" %q", e.Text)
		}
		return styleSignal.Render(fmt.Sprintf("[%s]%s", label, reason))

	case loop.EventRouteDecision:
		return styleSignal.Render(fmt.Sprintf("[ROUTE → %s] %q", e.RouteTarget, e.Text))

	case loop.EventError:
		text := e.ErrText
		if text == "" {
			text = e.Text
		}
		return styleErrorTxt.Render(fmt.Sprintf("ERROR: %s", text))

	case loop.EventOperatorAttention:
		return stylePaused.Render(fmt.Sprintf("⚠ OPERATOR ATTENTION: %s", e.Text))

	case loop.EventRetrying:
		return stylePaused.Render(fmt.Sprintf("↻ Retrying (attempt %d)...", e.Iteration))

	case loop.EventThrashingDetected:
		return styleErrorTxt.Render("⚠ Thrashing detected — no progress")

	case loop.EventPipelineComplete:
		return styleComplete.Render("✓ Pipeline complete")

	case loop.EventPaused:
		return stylePaused.Render("⏸ Paused")

	case loop.EventMaxIterationsReached:
		return stylePaused.Render("⚠ Max iterations reached")

	case loop.EventComplete:
		return styleComplete.Render("✓ Complete")

	default:
		return ""
	}
}
