package axontui

import "fmt"

// phases is the canonical pipeline phase order.
var phases = []string{"TD", "IM", "RV", "RF", "FL"}

// RenderPhaseProgress renders a visual phase progression indicator.
// Example: "TD ✓ → IM ▶ → RV ○ → RF ○ → FL ○"
func RenderPhaseProgress(currentPhase string, reviewCycles int) string {
	currentIdx := -1
	for i, p := range phases {
		if p == currentPhase {
			currentIdx = i
			break
		}
	}

	result := ""
	for i, p := range phases {
		if i > 0 {
			result += styleDim.Render(" → ")
		}

		switch {
		case currentIdx >= 0 && i < currentIdx:
			result += stylePhaseDone.Render(p + " ✓")
		case i == currentIdx:
			result += stylePhaseActive.Render(p + " ▶")
		default:
			result += stylePhasePend.Render(p + " ○")
		}
	}

	if currentPhase == "RV" && reviewCycles > 0 {
		result += styleDim.Render(fmt.Sprintf(" (cycle %d)", reviewCycles))
	}

	return result
}
