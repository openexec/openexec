# f22 human-boundary tests — run record

- Tested commit: `1647e2f3924b11ee66cfdd3b01aa572614810767` (retained head of
  `agent-console/task-2489b7c6f947-outcome-not-met-f22-human-boundary-tests`;
  its parent is `7bd33569`, it is not on `main`). Tree checked out into a
  throwaway clone with `git checkout 1647e2f3 -- .`, removed after the run.
- Go: `go version go1.26.5 linux/amd64`
- Command: `go -C .base test -count=1 -v -run HumanBoundary ./internal/planner/ ./pkg/manager/`
- Exit status: 0 (the runner reported "Succeeded"; both packages end in `ok`).
- Run once; not repeated, output not edited (only the 3-line preamble of the
  runner is omitted).

## The two dependencies

"Both dependencies" is read as the two human-task dependencies of the
automatic neighbour `T-US-009-002` in
`TestHumanBoundaryRefinementMayMoveHumanTasksOutOfASplitStory`
(`internal/planner/human_boundary_test.go`): the retained head's only change
(3 lines) makes it depend on **both** `T-US-009-003` and `T-US-009-004`
(`DependsOn: {"T-US-009-001","T-US-009-003","T-US-009-004"}`). Confirmed by:
the fixture and the two `reflect.DeepEqual(... DependsOn ...)` assertions
("neighbour dependency on human task lost" / "neighbour not retained with its
human edge") in that test, whose subtests
`clean_split_keeps_human_tasks_and_neighbour_edges_and_validates` and
`split_accepted_and_restored` PASS below. This is the owner-question default
interpretation (test-fixture dependencies); the owner has not answered.

## Raw output

```
=== RUN   TestHumanBoundaryCompactionKeepsPreparationAndDependencies
--- PASS: TestHumanBoundaryCompactionKeepsPreparationAndDependencies (0.00s)
=== RUN   TestHumanBoundaryReviewRequiresConcreteReason
--- PASS: TestHumanBoundaryReviewRequiresConcreteReason (0.00s)
=== RUN   TestHumanBoundaryRefinementCannotRemoveOrAuthorizeRetainedWork
=== RUN   TestHumanBoundaryRefinementCannotRemoveOrAuthorizeRetainedWork/mode
=== RUN   TestHumanBoundaryRefinementCannotRemoveOrAuthorizeRetainedWork/delete
=== RUN   TestHumanBoundaryRefinementCannotRemoveOrAuthorizeRetainedWork/edge
=== RUN   TestHumanBoundaryRefinementCannotRemoveOrAuthorizeRetainedWork/reference
=== RUN   TestHumanBoundaryRefinementCannotRemoveOrAuthorizeRetainedWork/action
=== RUN   TestHumanBoundaryRefinementCannotRemoveOrAuthorizeRetainedWork/unchanged
=== RUN   TestHumanBoundaryRefinementCannotRemoveOrAuthorizeRetainedWork/legacy
=== RUN   TestHumanBoundaryRefinementCannotRemoveOrAuthorizeRetainedWork/independent
--- PASS: TestHumanBoundaryRefinementCannotRemoveOrAuthorizeRetainedWork (0.00s)
    --- PASS: TestHumanBoundaryRefinementCannotRemoveOrAuthorizeRetainedWork/mode (0.00s)
    --- PASS: TestHumanBoundaryRefinementCannotRemoveOrAuthorizeRetainedWork/delete (0.00s)
    --- PASS: TestHumanBoundaryRefinementCannotRemoveOrAuthorizeRetainedWork/edge (0.00s)
    --- PASS: TestHumanBoundaryRefinementCannotRemoveOrAuthorizeRetainedWork/reference (0.00s)
    --- PASS: TestHumanBoundaryRefinementCannotRemoveOrAuthorizeRetainedWork/action (0.00s)
    --- PASS: TestHumanBoundaryRefinementCannotRemoveOrAuthorizeRetainedWork/unchanged (0.00s)
    --- PASS: TestHumanBoundaryRefinementCannotRemoveOrAuthorizeRetainedWork/legacy (0.00s)
    --- PASS: TestHumanBoundaryRefinementCannotRemoveOrAuthorizeRetainedWork/independent (0.00s)
=== RUN   TestHumanBoundaryRefinementMayCorrectTheBoundaryStrategy
--- PASS: TestHumanBoundaryRefinementMayCorrectTheBoundaryStrategy (0.00s)
=== RUN   TestHumanBoundaryMetadataRoundTrip
--- PASS: TestHumanBoundaryMetadataRoundTrip (0.00s)
=== RUN   TestHumanBoundaryRuleSharedAcrossPlannerPrompts
--- PASS: TestHumanBoundaryRuleSharedAcrossPlannerPrompts (0.00s)
=== RUN   TestHumanBoundaryRefinementMayMergeWorkBeyondTheBoundary
--- PASS: TestHumanBoundaryRefinementMayMergeWorkBeyondTheBoundary (0.00s)
=== RUN   TestHumanBoundaryRefinementMayRenumberTheBoundaryStory
--- PASS: TestHumanBoundaryRefinementMayRenumberTheBoundaryStory (0.00s)
=== RUN   TestHumanBoundaryRuleForbidsReviewDeletingRetainedHumanTasks
--- PASS: TestHumanBoundaryRuleForbidsReviewDeletingRetainedHumanTasks (0.00s)
=== RUN   TestHumanBoundaryRefinementMayMoveHumanTasksOutOfASplitStory
=== RUN   TestHumanBoundaryRefinementMayMoveHumanTasksOutOfASplitStory/split_accepted_and_restored
=== RUN   TestHumanBoundaryRefinementMayMoveHumanTasksOutOfASplitStory/clean_split_keeps_human_tasks_and_neighbour_edges_and_validates
=== RUN   TestHumanBoundaryRefinementMayMoveHumanTasksOutOfASplitStory/split_tied_by_a_task_edge_only
=== RUN   TestHumanBoundaryRefinementMayMoveHumanTasksOutOfASplitStory/delete_human_task
=== RUN   TestHumanBoundaryRefinementMayMoveHumanTasksOutOfASplitStory/delete_automatic_neighbour
=== RUN   TestHumanBoundaryRefinementMayMoveHumanTasksOutOfASplitStory/delete_T-US-013-004
=== RUN   TestHumanBoundaryRefinementMayMoveHumanTasksOutOfASplitStory/remove_story_with_retained
=== RUN   TestHumanBoundaryRefinementMayMoveHumanTasksOutOfASplitStory/ambiguous_duplicate_id
=== RUN   TestHumanBoundaryRefinementMayMoveHumanTasksOutOfASplitStory/duplicate_id_inside_one_story
=== RUN   TestHumanBoundaryRefinementMayMoveHumanTasksOutOfASplitStory/moved_into_unrelated_existing_story
=== RUN   TestHumanBoundaryRefinementMayMoveHumanTasksOutOfASplitStory/moved_into_unrelated_new_story
=== RUN   TestHumanBoundaryRefinementMayMoveHumanTasksOutOfASplitStory/remove_story_holding_retained
=== RUN   TestHumanBoundaryRefinementMayMoveHumanTasksOutOfASplitStory/changed_identity_with_two_candidates
=== RUN   TestHumanBoundaryRefinementMayMoveHumanTasksOutOfASplitStory/changed_identity
--- PASS: TestHumanBoundaryRefinementMayMoveHumanTasksOutOfASplitStory (0.00s)
    --- PASS: TestHumanBoundaryRefinementMayMoveHumanTasksOutOfASplitStory/split_accepted_and_restored (0.00s)
    --- PASS: TestHumanBoundaryRefinementMayMoveHumanTasksOutOfASplitStory/clean_split_keeps_human_tasks_and_neighbour_edges_and_validates (0.00s)
    --- PASS: TestHumanBoundaryRefinementMayMoveHumanTasksOutOfASplitStory/split_tied_by_a_task_edge_only (0.00s)
    --- PASS: TestHumanBoundaryRefinementMayMoveHumanTasksOutOfASplitStory/delete_human_task (0.00s)
    --- PASS: TestHumanBoundaryRefinementMayMoveHumanTasksOutOfASplitStory/delete_automatic_neighbour (0.00s)
    --- PASS: TestHumanBoundaryRefinementMayMoveHumanTasksOutOfASplitStory/delete_T-US-013-004 (0.00s)
    --- PASS: TestHumanBoundaryRefinementMayMoveHumanTasksOutOfASplitStory/remove_story_with_retained (0.00s)
    --- PASS: TestHumanBoundaryRefinementMayMoveHumanTasksOutOfASplitStory/ambiguous_duplicate_id (0.00s)
    --- PASS: TestHumanBoundaryRefinementMayMoveHumanTasksOutOfASplitStory/duplicate_id_inside_one_story (0.00s)
    --- PASS: TestHumanBoundaryRefinementMayMoveHumanTasksOutOfASplitStory/moved_into_unrelated_existing_story (0.00s)
    --- PASS: TestHumanBoundaryRefinementMayMoveHumanTasksOutOfASplitStory/moved_into_unrelated_new_story (0.00s)
    --- PASS: TestHumanBoundaryRefinementMayMoveHumanTasksOutOfASplitStory/remove_story_holding_retained (0.00s)
    --- PASS: TestHumanBoundaryRefinementMayMoveHumanTasksOutOfASplitStory/changed_identity_with_two_candidates (0.00s)
    --- PASS: TestHumanBoundaryRefinementMayMoveHumanTasksOutOfASplitStory/changed_identity (0.00s)
=== RUN   TestHumanBoundaryRefinementRahoitettava53fff42cShapes
=== RUN   TestHumanBoundaryRefinementRahoitettava53fff42cShapes/US-016_human_task_renumbered_to_US-020_by_exact_reason_while_US-016_survives
=== RUN   TestHumanBoundaryRefinementRahoitettava53fff42cShapes/US-017_human_task_with_reworded_reason_is_refused,_not_guessed
--- PASS: TestHumanBoundaryRefinementRahoitettava53fff42cShapes (0.00s)
    --- PASS: TestHumanBoundaryRefinementRahoitettava53fff42cShapes/US-016_human_task_renumbered_to_US-020_by_exact_reason_while_US-016_survives (0.00s)
    --- PASS: TestHumanBoundaryRefinementRahoitettava53fff42cShapes/US-017_human_task_with_reworded_reason_is_refused,_not_guessed (0.00s)
PASS
ok  	github.com/openexec/openexec/internal/planner	0.006s
=== RUN   TestHumanBoundaryImportPathsPreserveSchedulingAndMetadata
=== RUN   TestHumanBoundaryImportPathsPreserveSchedulingAndMetadata/legacy
2026/11/18 00:36:56 [StateStore] Initialized: path=/tmp/TestHumanBoundaryImportPathsPreserveSchedulingAndMetadatalegacy833887623/002/state.db, schema=v1, async_queue_size=1000
[MIGRATION] Bootstrapping from JSON files into SQLite (one-time only)
[MIGRATION] Bootstrap complete. SQLite is now the canonical source.
2026/11/18 00:36:56 [Watchdog] Started monitoring with stall threshold 5m0s
[MIGRATION] Bootstrapping from JSON files into SQLite (one-time only)
[MIGRATION] Bootstrap complete. SQLite is now the canonical source.
2026/11/18 00:36:56 [Planner] Imported 1 goals, 1 stories, 4 tasks into database
=== RUN   TestHumanBoundaryImportPathsPreserveSchedulingAndMetadata/reviewed
2026/11/18 00:36:56 [StateStore] Initialized: path=/tmp/TestHumanBoundaryImportPathsPreserveSchedulingAndMetadatareview4198464135/002/state.db, schema=v1, async_queue_size=1000
[MIGRATION] Bootstrapping from JSON files into SQLite (one-time only)
[MIGRATION] Bootstrap complete. SQLite is now the canonical source.
2026/11/18 00:36:56 [Watchdog] Started monitoring with stall threshold 5m0s
[MIGRATION] Bootstrapping from JSON files into SQLite (one-time only)
[MIGRATION] Bootstrap complete. SQLite is now the canonical source.
[MIGRATION] Bootstrapping from JSON files into SQLite (one-time only)
[MIGRATION] Bootstrap complete. SQLite is now the canonical source.
2026/11/18 00:36:56 [Planner] Imported 1 goals, 1 stories, 4 tasks into database
=== RUN   TestHumanBoundaryImportPathsPreserveSchedulingAndMetadata/replayed
2026/11/18 00:36:56 [StateStore] Initialized: path=/tmp/TestHumanBoundaryImportPathsPreserveSchedulingAndMetadatareplay1120935058/002/state.db, schema=v1, async_queue_size=1000
[MIGRATION] Bootstrapping from JSON files into SQLite (one-time only)
[MIGRATION] Bootstrap complete. SQLite is now the canonical source.
[MIGRATION] Bootstrapping from JSON files into SQLite (one-time only)
[MIGRATION] Bootstrap complete. SQLite is now the canonical source.
[MIGRATION] Bootstrapping from JSON files into SQLite (one-time only)
[MIGRATION] Bootstrap complete. SQLite is now the canonical source.
2026/11/18 00:36:56 [Watchdog] Started monitoring with stall threshold 5m0s
--- PASS: TestHumanBoundaryImportPathsPreserveSchedulingAndMetadata (0.91s)
    --- PASS: TestHumanBoundaryImportPathsPreserveSchedulingAndMetadata/legacy (0.37s)
    --- PASS: TestHumanBoundaryImportPathsPreserveSchedulingAndMetadata/reviewed (0.22s)
    --- PASS: TestHumanBoundaryImportPathsPreserveSchedulingAndMetadata/replayed (0.32s)
PASS
ok  	github.com/openexec/openexec/pkg/manager	0.922s
```
