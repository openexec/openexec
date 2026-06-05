package release

import "testing"

func TestComputePhase(t *testing.T) {
	done := &Story{Status: StoryStatusDone}
	approved := &Story{Status: StoryStatusApproved}
	pending := &Story{Status: StoryStatusPending}
	inProgress := &Story{Status: StoryStatusInProgress}

	cases := []struct {
		name    string
		stories []*Story
		want    string
	}{
		{"no stories", nil, PhaseNew},
		{"plan exists, nothing done", []*Story{pending, inProgress}, PhasePlanned},
		{"partially done", []*Story{done, pending}, PhaseBuilding},
		{"all done", []*Story{done, approved}, PhaseMaintaining},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ComputePhase(tc.stories); got != tc.want {
				t.Errorf("ComputePhase = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestComputePhase_IgnoresMaintenanceStory(t *testing.T) {
	done := &Story{Status: StoryStatusDone}
	maint := &Story{Status: StoryStatusInProgress, StoryType: StoryTypeMaintenance}

	// All real stories done + open maintenance story → still maintaining.
	if got := ComputePhase([]*Story{done, maint}); got != PhaseMaintaining {
		t.Errorf("maintenance story must not block maintaining, got %q", got)
	}
	// Only a maintenance story (no plan) → new.
	if got := ComputePhase([]*Story{maint}); got != PhaseNew {
		t.Errorf("maintenance-only backlog should be phase new, got %q", got)
	}
}
