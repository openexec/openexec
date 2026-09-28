package manager

import (
	"fmt"
	"sort"

	"github.com/openexec/openexec/internal/release"
)

// TaskBoundary is a projection of retained native work, not a new task state or
// a request for permission. A missing legacy decision reference remains held.
type TaskBoundary struct {
	TaskID         string `json:"task_id"`
	Status         string `json:"status"`
	Kind           string `json:"kind"`
	DecisionReason string `json:"decision_reason,omitempty"`
	DecisionRef    string `json:"decision_ref,omitempty"`
}

const (
	BoundaryHuman        = "waiting_for_human"
	BoundaryAttemptLimit = "attempt_limit"
	BoundaryFailed       = "failed"
	BoundaryReview       = "needs_review"
	BoundaryDependency   = "dependency"
	BoundaryRetained     = "retained"
)

// TaskQueueBoundary reports why a drained queue retains unfinished work. It
// grants no authority and marks no work complete. Callers must inspect every
// item: human waiting can coexist with an exhausted or failed independent task.
type TaskQueueBoundary struct {
	Tasks []TaskBoundary `json:"tasks"`
}

func (b *TaskQueueBoundary) Error() string {
	// Reasons may contain owner input. Keep them out of ordinary error logs.
	return fmt.Sprintf("no executable work: %d task(s) retained at a native boundary", len(b.Tasks))
}

func retainedTaskBoundary(tasks []*release.Task) error {
	boundary := &TaskQueueBoundary{}
	for _, task := range tasks {
		if task.Status == release.TaskStatusDone {
			continue
		}
		item := TaskBoundary{TaskID: task.ID, Status: task.Status, Kind: BoundaryRetained}
		switch task.Status {
		case release.TaskStatusFailed:
			item.Kind = BoundaryFailed
		case release.TaskStatusNeedsReview:
			item.Kind = BoundaryReview
		case release.TaskStatusPending:
			switch {
			case task.MaxAttempts <= 0 || task.AttemptCount >= task.MaxAttempts:
				item.Kind = BoundaryAttemptLimit
			case task.ExecutionMode() == release.TaskModeHITL:
				item.Kind = BoundaryHuman
				item.DecisionReason, _ = task.Metadata["decision_reason"].(string)
				item.DecisionRef, _ = task.Metadata["decision_ref"].(string)
			default:
				item.Kind = BoundaryDependency
			}
		}
		boundary.Tasks = append(boundary.Tasks, item)
	}
	if len(boundary.Tasks) == 0 {
		return nil
	}
	sort.Slice(boundary.Tasks, func(i, j int) bool { return boundary.Tasks[i].TaskID < boundary.Tasks[j].TaskID })
	return boundary
}
