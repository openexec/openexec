package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/openexec/openexec/internal/release"
)

// loadPendingTasks loads all tasks from the release manager (Source of Truth), tasks.json, or stories.json
func loadPendingTasks(projectDir string, mgr *release.Manager) ([]Task, error) {
	// Pre-flight: Load stories.json to use as a "map of intent" for reconciliation
	incomingTaskStories := make(map[string]string)
	plannedTitles := make(map[string]string)
	storiesFile := filepath.Join(projectDir, ".openexec", "stories.json")
	if data, err := os.ReadFile(storiesFile); err == nil {
		var sf struct {
			Stories []struct {
				ID    string `json:"id"`
				Tasks []any  `json:"tasks"`
			} `json:"stories"`
		}
		if err := json.Unmarshal(data, &sf); err == nil {
			for _, s := range sf.Stories {
				for _, tRaw := range s.Tasks {
					id := ""
					title := ""
					switch v := tRaw.(type) {
					case string: id = v
					case map[string]any: 
						id, _ = v["id"].(string)
						title, _ = v["title"].(string)
					}
					if id != "" {
						incomingTaskStories[id] = s.ID
						plannedTitles[id] = title
					}
				}
			}
		}
	}

	// INTRA-STORY SEQUENCING: Map previous tasks within each story from stories.json
	prevInStory := make(map[string]string)
	if data, err := os.ReadFile(storiesFile); err == nil {
		var sf struct {
			Stories []struct {
				ID    string `json:"id"`
				Tasks []any  `json:"tasks"`
			} `json:"stories"`
		}
		if err := json.Unmarshal(data, &sf); err == nil {
			for _, s := range sf.Stories {
				var lastID string
				for _, tRaw := range s.Tasks {
					tid := ""
					switch v := tRaw.(type) {
					case string: tid = v
					case map[string]any: tid, _ = v["id"].(string)
					}
					if tid != "" {
						if lastID != "" {
							prevInStory[tid] = lastID
						}
						lastID = tid
					}
				}
			}
		}
	}

	// 1. Try Release Manager first (Source of Truth)
	if mgr != nil {
		relTasks := mgr.GetTasks()
		
		// DEEP PRE-FLIGHT HEALING: Create any planned tasks missing from Manager
		existingMap := make(map[string]bool)
		for _, rt := range relTasks { existingMap[rt.ID] = true }
		
		for tid, sid := range incomingTaskStories {
			if !existingMap[tid] {
				fmt.Printf("  ✨ Deep-Healed: Restoring missing task %s to database...\n", tid)
				title := plannedTitles[tid]
				if title == "" { title = "Imported Task " + tid }
				_ = mgr.CreateTask(&release.Task{
					ID: tid, 
					StoryID: sid, 
					Title: title, 
					Status: release.TaskStatusPending,
				})
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

				// STATUS SYNC: Check if markdown file says it's done
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

				// MATERIALIZATION: Ensure the agent's work file exists
				fwuDir := filepath.Join(projectDir, ".openexec", "fwu")
				_ = os.MkdirAll(fwuDir, 0750)
				taskFile := filepath.Join(fwuDir, rt.ID+".md")
				if _, err := os.Stat(taskFile); os.IsNotExist(err) {
					content := fmt.Sprintf("# Task %s: %s\n\n%s\n\nStatus: pending\n", rt.ID, rt.Title, rt.Description)
					_ = os.WriteFile(taskFile, []byte(content), 0644)
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
						if depStoryID == rt.StoryID { continue }
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

	// 2. Fallback to root tasks.json
	rootTasksFile := filepath.Join(projectDir, "tasks.json")
	if data, err := os.ReadFile(rootTasksFile); err == nil {
		var tf TasksFile
		if err := json.Unmarshal(data, &tf); err == nil {
			return tf.Tasks, nil
		}
	}

	// 3. Fallback to .openexec/tasks.json
	hiddenTasksFile := filepath.Join(projectDir, ".openexec", "tasks.json")
	if data, err := os.ReadFile(hiddenTasksFile); err == nil {
		var tf TasksFile
		if err := json.Unmarshal(data, &tf); err == nil {
			return tf.Tasks, nil
		}
	}

	// 4. Fallback to stories.json (Planner Output)
	storiesFile = filepath.Join(projectDir, ".openexec", "stories.json")
	storiesData, err := os.ReadFile(storiesFile)
	if err != nil {
		return nil, fmt.Errorf("no tasks found: %w", err)
	}

	var sf struct {
		Stories []struct {
			ID        string `json:"id"`
			Title     string `json:"title"`
			Status    string `json:"status"`
			DependsOn []string `json:"depends_on"`
			Tasks     []struct {
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
			var prevTaskID string
			for _, genTask := range story.Tasks {
				deps := genTask.DependsOn
				if prevTaskID != "" { deps = append(deps, prevTaskID) }
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
