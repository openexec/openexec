package pipeline

import (
	"context"
	"testing"

	"github.com/openexec/openexec/internal/blueprint"
	"github.com/openexec/openexec/internal/loop"
)

type activityFixture func(*blueprint.StageInput)

func (f activityFixture) Execute(_ context.Context, stage *blueprint.Stage, input *blueprint.StageInput) (*blueprint.StageResult, error) {
	f(input)
	return &blueprint.StageResult{StageName: stage.Name}, nil
}

func TestExecutionActivityScopedToCall(t *testing.T) {
	var retained *blueprint.StageInput
	var events []loop.Event
	executor := activityStageExecutor{
		executor: activityFixture(func(input *blueprint.StageInput) {
			retained = input
			input.MonitorActivity()
			input.ReportActivity()
			end := input.BeginCheck()
			nested := input.BeginCheck()
			nested()
			nested() // A duplicate completion must not consume the outer scope.
			end()
		}),
		emit: func(event loop.Event) { events = append(events, event) },
	}
	original := blueprint.NewStageInput("run", "", t.TempDir())
	if _, err := executor.Execute(context.Background(), &blueprint.Stage{Name: "implement"}, original); err != nil {
		t.Fatal(err)
	}
	want := []string{"monitor", "progress", "check_start", "check_start", "check_end", "check_end"}
	if len(events) != len(want) {
		t.Fatal(events)
	}
	for i, event := range events {
		if event.Type != loop.EventExecutionActivity || event.ExecutionActivity != want[i] || event.StageName != "implement" {
			t.Fatal(events)
		}
	}
	retained.ReportActivity()
	retained.BeginCheck()()
	if len(events) != len(want) {
		t.Fatal("late callback escaped its execution scope")
	}
	if original.OnExecutionActivity != nil {
		t.Fatal("callback leaked into shared input")
	}
}
