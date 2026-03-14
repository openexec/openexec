package cli

import (
	"fmt"

	"github.com/openexec/openexec/internal/release"
)

// loadPendingTasks loads all tasks from the release manager (Source of Truth).
func loadPendingTasks(projectDir string, mgr *release.Manager, isInitial bool) ([]Task, error) {
	if mgr == nil {
		return nil, fmt.Errorf("release manager is required")
	}

	relTasks := mgr.GetTasks()
	relStories := mgr.GetStories()
	
	// Pre-map everything for dependency resolution
	storyTaskIDs := make(map[string][]string)
	storyDeps := make(map[string][]string)
	for _, s := range relStories {
		storyDeps[s.ID] = s.DependsOn
	}
	for _, rt := range relTasks {
		storyTaskIDs[rt.StoryID] = append(storyTaskIDs[rt.StoryID], rt.ID)
	}

	if len(relTasks) > 0 {
		var tasks []Task

		for _, rt := range relTasks {
			// Only include tasks that aren't finished yet
			if rt.Status != string(release.TaskStatusDone) && rt.Status != "completed" {
				// Reconstruct dependencies from cross-story and intra-story barriers
				depSet := make(map[string]bool)

				// 1. Explicit dependencies
				for _, d := range rt.DependsOn {
					depSet[d] = true
				}

				// 2. Cross-story barrier: add ALL tasks from prerequisite stories
				if sDeps, ok := storyDeps[rt.StoryID]; ok {
					for _, sid := range sDeps {
						if tids, ok := storyTaskIDs[sid]; ok {
							for _, tid := range tids {
								depSet[tid] = true
							}
						}
					}
				}

				// 3. Intra-story sequence: add the task immediately preceding this one in the same story
				if tids, ok := storyTaskIDs[rt.StoryID]; ok {
					for i, tid := range tids {
						if tid == rt.ID && i > 0 {
							depSet[tids[i-1]] = true
							break
						}
					}
				}

				// Convert set to list
				finalDeps := make([]string, 0, len(depSet))
				for d := range depSet {
					finalDeps = append(finalDeps, d)
				}

				tasks = append(tasks, Task{
					ID:                 rt.ID,
					Title:              rt.Title,
					Description:        rt.Description,
					StoryID:            rt.StoryID,
					Status:             rt.Status,
					DependsOn:          finalDeps,
					VerificationScript: rt.VerificationScript,
				})
			}
		}
		
		if len(tasks) == 0 {
			return nil, nil // No tasks left to do
		}
		
		return tasks, nil
	}

	return nil, fmt.Errorf("no tasks found in database")
}
