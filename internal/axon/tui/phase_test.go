package axontui

import (
	"strings"
	"testing"
)

func TestRenderPhaseProgress(t *testing.T) {
	tests := []struct {
		name         string
		phase        string
		reviewCycles int
		wantParts    []string // substrings that must appear
		wantAbsent   []string // substrings that must NOT appear
	}{
		{
			name:      "TD active",
			phase:     "TD",
			wantParts: []string{"TD", "▶", "IM", "○", "RV", "RF", "FL"},
		},
		{
			name:      "IM active — TD done",
			phase:     "IM",
			wantParts: []string{"TD", "✓", "IM", "▶", "RV", "○"},
		},
		{
			name:         "RV active with review cycles",
			phase:        "RV",
			reviewCycles: 2,
			wantParts:    []string{"RV", "▶", "cycle 2"},
		},
		{
			name:         "RV active no cycles",
			phase:        "RV",
			reviewCycles: 0,
			wantParts:    []string{"RV", "▶"},
			wantAbsent:   []string{"cycle"},
		},
		{
			name:      "FL active — all prior done",
			phase:     "FL",
			wantParts: []string{"TD", "✓", "IM", "✓", "RV", "✓", "RF", "✓", "FL", "▶"},
		},
		{
			name:      "empty phase — all pending",
			phase:     "",
			wantParts: []string{"TD", "○", "IM", "○"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := RenderPhaseProgress(tt.phase, tt.reviewCycles)
			for _, part := range tt.wantParts {
				if !strings.Contains(result, part) {
					t.Errorf("expected %q in output, got: %s", part, result)
				}
			}
			for _, absent := range tt.wantAbsent {
				if strings.Contains(result, absent) {
					t.Errorf("did not expect %q in output, got: %s", absent, result)
				}
			}
		})
	}
}
