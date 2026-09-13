package manager

import (
	"context"
	"strings"
	"testing"

	"github.com/openexec/openexec/internal/release"
	"github.com/openexec/openexec/pkg/db/state"
)

func TestTaskQueueRefusesTransientBacklog(t *testing.T) {
	s, err := state.NewStore(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	m, err := New(Config{WorkDir: t.TempDir(), StateStore: s})
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	rel, err := m.GetInternalReleaseManager()
	if err != nil {
		t.Fatal(err)
	}
	createStory(t, rel, "S", nil)
	if err := rel.CreateTask(&release.Task{ID: "H", Title: "Retained boundary", StoryID: "S", Status: release.TaskStatusPending, Metadata: map[string]interface{}{"mode": release.TaskModeHITL}}); err != nil {
		t.Fatal(err)
	}
	err = m.ExecuteTasks(context.Background(), RunOptions{TaskOriented: true, StoryIDs: []string{"S"}})
	if err == nil || !strings.Contains(err.Error(), "requires durable task storage") {
		t.Fatal("transient queue reached task selection", err)
	}
}
