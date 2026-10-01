package release

import "reflect"

// Canonical comparisons mirror the reviewed importer columns, excluding lifecycle
// and evidence. Nil and empty arrays are equivalent, including retained legacy JSON/SQL null.
func ReviewedGoalEqual(old, next *Goal) bool {
	return old.Title == next.Title && old.Description == next.Description && old.SuccessCriteria == next.SuccessCriteria && old.VerificationMethod == next.VerificationMethod
}
func reviewedArrayEqual(old, next []string) bool {
	if old == nil {
		old = []string{}
	}
	if next == nil {
		next = []string{}
	}
	return reflect.DeepEqual(old, next)
}
func ReviewedStoryEqual(old, next *Story) bool {
	return old.GoalID == next.GoalID && old.Title == next.Title && old.Description == next.Description && reviewedArrayEqual(old.AcceptanceCriteria, next.AcceptanceCriteria) && old.VerificationScript == next.VerificationScript && old.Contract == next.Contract && reviewedArrayEqual(old.DependsOn, next.DependsOn) && old.StoryType == next.StoryType && old.Priority == next.Priority && reviewedArrayEqual(old.Tasks, next.Tasks)
}
func reviewedMode(t *Task) string {
	if v, ok := t.Metadata["mode"].(string); ok {
		return v
	}
	return TaskModeAFK
}
func ReviewedTaskEqual(old, next *Task) bool {
	return old.StoryID == next.StoryID && old.Title == next.Title && old.Description == next.Description && old.VerificationScript == next.VerificationScript && reviewedArrayEqual(old.DependsOn, next.DependsOn) && old.Priority == next.Priority && old.MaxAttempts == next.MaxAttempts && reviewedMode(old) == reviewedMode(next)
}
