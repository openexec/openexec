package axontui

import (
	"strings"
	"testing"

	"github.com/openexec/openexec/pkg/manager"
)

func TestDashboard_CursorMovement(t *testing.T) {
	d := Dashboard{}
	d.SetPipelines([]manager.PipelineInfo{
		{FWUID: "FWU-001", Status: manager.StatusRunning, Phase: "IM"},
		{FWUID: "FWU-002", Status: manager.StatusPaused, Phase: "RV"},
		{FWUID: "FWU-003", Status: manager.StatusComplete, Phase: ""},
	})

	// Starts at 0.
	p, ok := d.Selected()
	if !ok || p.FWUID != "FWU-001" {
		t.Fatalf("expected FWU-001 selected, got %s (ok=%v)", p.FWUID, ok)
	}

	// Move down.
	d.CursorDown()
	p, _ = d.Selected()
	if p.FWUID != "FWU-002" {
		t.Fatalf("expected FWU-002, got %s", p.FWUID)
	}

	// Move down again.
	d.CursorDown()
	p, _ = d.Selected()
	if p.FWUID != "FWU-003" {
		t.Fatalf("expected FWU-003, got %s", p.FWUID)
	}

	// Can't go past end.
	d.CursorDown()
	p, _ = d.Selected()
	if p.FWUID != "FWU-003" {
		t.Fatalf("expected FWU-003 (clamped), got %s", p.FWUID)
	}

	// Move up.
	d.CursorUp()
	p, _ = d.Selected()
	if p.FWUID != "FWU-002" {
		t.Fatalf("expected FWU-002, got %s", p.FWUID)
	}

	// Can't go past start.
	d.CursorUp()
	d.CursorUp()
	p, _ = d.Selected()
	if p.FWUID != "FWU-001" {
		t.Fatalf("expected FWU-001 (clamped), got %s", p.FWUID)
	}
}

func TestDashboard_SelectByIndex(t *testing.T) {
	d := Dashboard{}
	d.SetPipelines([]manager.PipelineInfo{
		{FWUID: "FWU-001"},
		{FWUID: "FWU-002"},
		{FWUID: "FWU-003"},
	})

	d.SelectByIndex(2)
	p, _ := d.Selected()
	if p.FWUID != "FWU-002" {
		t.Fatalf("expected FWU-002, got %s", p.FWUID)
	}

	// Out of range — no change.
	d.SelectByIndex(5)
	p, _ = d.Selected()
	if p.FWUID != "FWU-002" {
		t.Fatalf("expected FWU-002 (unchanged), got %s", p.FWUID)
	}
}

func TestDashboard_EmptyView(t *testing.T) {
	d := Dashboard{}
	d.SetSize(80, 20)
	view := d.View()
	if !strings.Contains(view, "No pipelines") {
		t.Fatalf("expected empty state message, got: %s", view)
	}
}

func TestDashboard_View(t *testing.T) {
	d := Dashboard{}
	d.SetPipelines([]manager.PipelineInfo{
		{FWUID: "FWU-001", Status: manager.StatusRunning, Phase: "IM", Iteration: 3},
		{FWUID: "FWU-002", Status: manager.StatusPaused, Phase: "RV", Iteration: 1},
	})
	d.SetSize(100, 20)

	view := d.View()

	if !strings.Contains(view, "FWU-001") {
		t.Error("expected FWU-001 in view")
	}
	if !strings.Contains(view, "FWU-002") {
		t.Error("expected FWU-002 in view")
	}
	if !strings.Contains(view, "[IM]") {
		t.Error("expected [IM] phase badge")
	}
}

func TestDashboard_CursorClampOnShrink(t *testing.T) {
	d := Dashboard{}
	d.SetPipelines([]manager.PipelineInfo{
		{FWUID: "FWU-001"},
		{FWUID: "FWU-002"},
		{FWUID: "FWU-003"},
	})

	d.SelectByIndex(3) // cursor at index 2

	// Shrink to 2 items — cursor should clamp.
	d.SetPipelines([]manager.PipelineInfo{
		{FWUID: "FWU-001"},
		{FWUID: "FWU-002"},
	})

	p, ok := d.Selected()
	if !ok || p.FWUID != "FWU-002" {
		t.Fatalf("expected FWU-002 (clamped), got %s (ok=%v)", p.FWUID, ok)
	}
}
