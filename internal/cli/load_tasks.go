package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/openexec/openexec/internal/release"
)

// storiesFileSchema represents the schema for stories.json parsing (intent reconciliation).
type storiesFileSchema struct {
	SchemaVersion string `json:"schema_version"`
	Goals         []any  `json:"goals"`
	Stories       []struct {
		ID    string `json:"id"`
		Title string `json:"title"`
		Tasks []any  `json:"tasks"`
	} `json:"stories"`
}

// extractTaskID extracts the task ID from a raw task value (string or map).
func extractTaskID(tRaw any) string {
	switch v := tRaw.(type) {
	case string:
		return v
	case map[string]any:
		if id, ok := v["id"].(string); ok {
			return id
		}
	}
	return ""
}

// extractTaskTitle extracts the task title from a raw task value (map only).
func extractTaskTitle(tRaw any) string {
	if v, ok := tRaw.(map[string]any); ok {
		if title, ok := v["title"].(string); ok {
			return title
		}
	}
	return ""
}

// loadPendingTasks loads all tasks from the release manager (Source of Truth), tasks.json, or stories.json
func loadPendingTasks(projectDir string, mgr *release.Manager, isInitial bool) ([]Task, error) {
	// Pre-flight: Load stories.json to use as a "map of intent" for reconciliation
	incomingTaskStories := make(map[string]string)
	plannedTitles := make(map[string]string)
	prevInStory := make(map[string]string)
	storiesFile := filepath.Join(projectDir, ".openexec", "stories.json")

	// Read and parse stories.json once
	if data, err := os.ReadFile(storiesFile); err == nil {
		var sf storiesFileSchema
		if err := json.Unmarshal(data, &sf); err == nil {
			for _, s := range sf.Stories {
				if len(s.Tasks) == 0 {
					// Auto-synthesize a Chassis task when a story has no explicit tasks
					synthID := "T-" + s.ID + "-CHS"
					synthTitle := "Chassis: " + s.Title
					incomingTaskStories[synthID] = s.ID
					plannedTitles[synthID] = synthTitle
					continue
				}

				// Process tasks: build intent mapping and intra-story sequencing
				var lastID string
				for _, tRaw := range s.Tasks {
					id := extractTaskID(tRaw)
					if id == "" {
						continue
					}

					// Intent mapping
					incomingTaskStories[id] = s.ID
					if title := extractTaskTitle(tRaw); title != "" {
						plannedTitles[id] = title
					}

					// Intra-story sequencing
					if lastID != "" {
						prevInStory[id] = lastID
					}
					lastID = id
				}
			}
		} else {
			fmt.Printf("  ⚠ Warning: Failed to parse stories.json: %v\n", err)
		}
	} else if os.IsNotExist(err) {
		// Try root directory as fallback
		storiesFile = filepath.Join(projectDir, "stories.json")
		if _, err := os.Stat(storiesFile); err == nil {
			// Recurse once with new path
			return loadPendingTasks(projectDir, mgr, isInitial)
		}
	} else {
		fmt.Printf("  ⚠ Warning: Failed to read stories.json: %v\n", err)
	}

	if len(incomingTaskStories) == 0 {
		fmt.Printf("  🔍 Debug: No tasks found in stories.json (path: %s)\n", storiesFile)
	}

	// 1. Try Release Manager first (Source of Truth)
	if mgr != nil {
		relTasks := mgr.GetTasks()

		// DEEP PRE-FLIGHT HEALING: Create any planned tasks missing from Manager
		existingMap := make(map[string]bool)
		for _, rt := range relTasks {
			existingMap[rt.ID] = true
		}

        for tid, sid := range incomingTaskStories {
            if !existingMap[tid] {
                if !isInitial {
                    // DB is canonical: do not deep-heal at runtime
                    continue
                }
                fmt.Printf("  ✨ Importing planned task %s...\n", tid)
                title := plannedTitles[tid]
                if title == "" { title = "Imported Task " + tid }
                var deps []string
                if prev, ok := prevInStory[tid]; ok { deps = append(deps, prev) }
                _ = mgr.CreateTask(&release.Task{ID: tid, StoryID: sid, Title: title, Status: release.TaskStatusPending, DependsOn: deps})
            }
        }
		// Refresh tasks list after healing
		relTasks = mgr.GetTasks()

		relStories := mgr.GetStories()
		storyTaskIDs := make(map[string][]string)
		storyDeps := make(map[string][]string)
		for _, s := range relStories {
			storyDeps[s.ID] = s.DependsOn
		}

		if len(relTasks) > 0 {
			var tasks []Task

			// Build task map first to identify story membership
			for _, rt := range relTasks {
				storyTaskIDs[rt.StoryID] = append(storyTaskIDs[rt.StoryID], rt.ID)
			}

			for _, rt := range relTasks {
				// RECONCILIATION: Check for StoryID mismatch
				expectedStoryID, inPlan := incomingTaskStories[rt.ID]
				if inPlan && rt.StoryID != expectedStoryID {
					rt.StoryID = expectedStoryID
					_ = mgr.UpdateTask(rt)
				}

            // STATUS SYNC: Only during initialization — runtime source of truth is DB
            if isInitial {
                storyPath := filepath.Join(projectDir, ".openexec", "stories", rt.StoryID+".md")
                if data, err := os.ReadFile(storyPath); err == nil {
                    content := strings.ToLower(string(data))
                    if strings.Contains(content, "status: completed") || strings.Contains(content, "status: done") || strings.Contains(content, "status: verified") {
                        if rt.Status == "pending" || rt.Status == "starting" || rt.Status == "running" {
                            rt.Status = "completed"
                            _ = mgr.UpdateTask(rt)
                        }
                    }
                }
            }

            // MATERIALIZATION: Ensure the agent's work file exists (skip in read-only mode)
            if os.Getenv("OPENEXEC_MODE") != "read-only" {
                fwuDir := filepath.Join(projectDir, ".openexec", "fwu")
                _ = os.MkdirAll(fwuDir, 0750)
                taskFile := filepath.Join(fwuDir, rt.ID+".md")
                if _, err := os.Stat(taskFile); os.IsNotExist(err) {
                    content := fmt.Sprintf("# Task %s: %s\n\n%s\n\nStatus: pending\n", rt.ID, rt.Title, rt.Description)
                    _ = os.WriteFile(taskFile, []byte(content), 0644)
                }
            }

				// Only include tasks that are actually in the current Plan of Record
				if len(incomingTaskStories) > 0 && !inPlan {
					continue
				}

				// EFFICIENT DEDUPING: Use a map for dependency tracking
				depSet := make(map[string]bool)
				for _, d := range rt.DependsOn {
					depSet[d] = true
				}

				// SYNTHETIC STORY BARRIER: Wait for ALL tasks in prerequisite stories
				if parentStoryDeps, ok := storyDeps[rt.StoryID]; ok {
					for _, depStoryID := range parentStoryDeps {
						if depStoryID == rt.StoryID {
							continue
						}
						if prerequisiteTasks, ok := storyTaskIDs[depStoryID]; ok {
							for _, ptid := range prerequisiteTasks {
								depSet[ptid] = true
							}
						}
					}
				}

				// INTRA-STORY SEQUENCING: Enforce order within the same story
				if prev, ok := prevInStory[rt.ID]; ok {
					depSet[prev] = true
				}

				// Final dependency list
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
			return tasks, nil
		}
	}

    // 2. Fallbacks to JSON definitions are only allowed during initialization/import
    if isInitial {
        // root tasks.json
        rootTasksFile := filepath.Join(projectDir, "tasks.json")
        if data, err := os.ReadFile(rootTasksFile); err == nil {
            var tf TasksFile
            if err := json.Unmarshal(data, &tf); err == nil {
                return tf.Tasks, nil
            }
        }

        // .openexec/tasks.json
        hiddenTasksFile := filepath.Join(projectDir, ".openexec", "tasks.json")
        if data, err := os.ReadFile(hiddenTasksFile); err == nil {
            var tf TasksFile
            if err := json.Unmarshal(data, &tf); err == nil {
                return tf.Tasks, nil
            }
        }
    }

    // 4. Fallback to stories.json (Planner Output) — only during initialization/import
    if !isInitial {
        return nil, fmt.Errorf("no tasks found")
    }
    storiesFile = filepath.Join(projectDir, ".openexec", "stories.json")
    storiesData, err := os.ReadFile(storiesFile)
    if err != nil {
        return nil, fmt.Errorf("no tasks found: %w", err)
    }

	var sf struct {
		Stories []struct {
			ID                 string   `json:"id"`
			Title              string   `json:"title"`
			Status             string   `json:"status"`
			DependsOn          []string `json:"depends_on"`
			VerificationScript string   `json:"verification_script,omitempty"`
			Tasks              []struct {
				ID                 string   `json:"id"`
				Title              string   `json:"title"`
				Description        string   `json:"description"`
				DependsOn          []string `json:"depends_on"`
				VerificationScript string   `json:"verification_script,omitempty"`
			} `json:"tasks"`
		} `json:"stories"`
	}

	if err := json.Unmarshal(storiesData, &sf); err == nil {
		var tasks []Task
		for _, story := range sf.Stories {
			// If a story has no explicit tasks, auto-synthesize a single Chassis task
			if len(story.Tasks) == 0 {
				synthID := "T-" + story.ID + "-CHS"
				synthTitle := "Chassis: " + story.Title
				tasks = append(tasks, Task{
					ID:                 synthID,
					Title:              synthTitle,
					Description:        "Auto-synthesized task for story with no explicit tasks",
					StoryID:            story.ID,
					Status:             "pending",
					DependsOn:          story.DependsOn,
					VerificationScript: story.VerificationScript,
				})
				continue
			}

			var prevTaskID string
			for _, genTask := range story.Tasks {
				deps := genTask.DependsOn
				if prevTaskID != "" {
					deps = append(deps, prevTaskID)
				}
				tasks = append(tasks, Task{
					ID:                 genTask.ID,
					Title:              genTask.Title,
					Description:        genTask.Description,
					StoryID:            story.ID,
					Status:             "pending",
					DependsOn:          deps,
					VerificationScript: genTask.VerificationScript,
				})
				prevTaskID = genTask.ID
			}
		}
		return tasks, nil
	}

	return nil, fmt.Errorf("no tasks found")
}
