package manager

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/openexec/openexec/internal/release"
)

// RunOptions defines settings for executing multiple tasks.
type RunOptions struct {
	MaxParallel int    `json:"worker_count"` // Fix mismatch: CLI sends worker_count
	IsStudy     bool   `json:"is_study"`
	Mode        string `json:"mode"`
}

// ExecuteTasks runs all pending tasks in the dependency graph.
func (m *Manager) ExecuteTasks(ctx context.Context, opts RunOptions) error {
	rel, err := m.getInternalReleaseManager()
	if err != nil {
		return err
	}

	tasks := rel.GetTasks()
	if len(tasks) == 0 {
		return nil
	}

	stories := rel.GetStories()
	storyMap := make(map[string]*release.Story)
	for _, s := range stories {
		storyMap[s.ID] = s
	}

	// Filter for pending tasks only
	var pending []*release.Task
	for _, t := range tasks {
		if t.Status == "pending" || t.Status == "" {
			pending = append(pending, t)
		}
	}

	if len(pending) == 0 {
		log.Printf("[Scheduler] All tasks already complete or in progress")
		return nil
	}

	log.Printf("[Scheduler] Starting execution of %d pending tasks (parallel=%d)", len(pending), opts.MaxParallel)

	// Simple topological sort / dependency resolver
	type node struct {
		Task       *release.Task
		Deps       []string // Task-level dependencies
		StoryDeps  []string // Story-level dependencies
		Dispatched bool
		Finished   bool
	}

	nodes := make(map[string]*node)
	for _, t := range pending {
		storyDeps := []string{}
		if story, ok := storyMap[t.StoryID]; ok {
			storyDeps = story.DependsOn
		}
		nodes[t.ID] = &node{
			Task:      t,
			Deps:      t.DependsOn,
			StoryDeps: storyDeps,
		}
	}

	var mu sync.Mutex
	wg := sync.WaitGroup{}
	readyTasks := make(chan *node, len(pending))
	errors := make(chan error, len(pending))
	finishedCount := 0
	totalToRun := len(pending)

	// Helper to check if all tasks of a story are finished
	isStoryFinished := func(storyID string) bool {
		storyTasks := rel.GetTasksForStory(storyID)
		for _, st := range storyTasks {
			if st.Status != "done" && st.Status != "approved" {
				return false
			}
		}
		return true
	}

	checkReady := func() {
		mu.Lock()
		defer mu.Unlock()

		// First pass: detect tasks whose dependencies have failed (cascade failure)
		for _, n := range nodes {
			if n.Dispatched || n.Finished {
				continue
			}
			for _, depID := range n.Deps {
				if d, ok := nodes[depID]; ok && d.Finished {
					// Dependency finished — check if it failed
					t := rel.GetTask(depID)
					if t != nil && t.Status == "error" {
						// Cascade: mark this task as failed too
						n.Finished = true
						finishedCount++
						_ = rel.SetTaskStatus(n.Task.ID, "error")
						log.Printf("[Scheduler] Task %s skipped: dependency %s failed", n.Task.ID, depID)
						break
					}
				}
			}
		}

		// Check if all tasks are now finished (including cascaded failures)
		if finishedCount == totalToRun {
			close(readyTasks)
			return
		}

		// Second pass: dispatch tasks whose dependencies are satisfied
		for _, n := range nodes {
			if n.Dispatched || n.Finished {
				continue
			}

			// 1. Check Task-level dependencies
			allTaskDepsDone := true
			for _, depID := range n.Deps {
				// Check if the dependency is in the current pending set
				if d, ok := nodes[depID]; ok {
					if !d.Finished {
						allTaskDepsDone = false
						break
					}
					continue
				}
				// If not in pending set, check actual task status in DB
				t := rel.GetTask(depID)
				if t == nil || (t.Status != "done" && t.Status != "approved") {
					allTaskDepsDone = false
					break
				}
			}
			if !allTaskDepsDone {
				continue
			}

			// 2. Check Story-level dependencies
			allStoryDepsDone := true
			for _, storyDepID := range n.StoryDeps {
				if !isStoryFinished(storyDepID) {
					allStoryDepsDone = false
					break
				}
			}

			if allStoryDepsDone {
				n.Dispatched = true
				readyTasks <- n
			}
		}
	}

	// Initial check
	go checkReady()

	workerCount := opts.MaxParallel
	if workerCount <= 0 {
		workerCount = 1
	}

	for i := 0; i < workerCount; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for node := range readyTasks {
				log.Printf("[Worker %d] Executing %s: %s", id, node.Task.ID, node.Task.Title)

				start := time.Now()
				taskFailed := false

				var optsList []StartOption
				if opts.IsStudy {
					optsList = append(optsList, WithIsStudy(true))
				}
				if opts.Mode != "" {
					optsList = append(optsList, WithExecMode(opts.Mode))
				}

				// Ensure blueprint metadata is passed
				optsList = append(optsList, WithBlueprint("standard_task"))
				optsList = append(optsList, WithTaskDescription(node.Task.Description))

				// Start the individual pipeline
				if err := m.Start(ctx, node.Task.ID, optsList...); err != nil {
					log.Printf("[Worker %d] ✗ Failed to start %s: %v", id, node.Task.ID, err)
					errors <- fmt.Errorf("failed to start task %s: %w", node.Task.ID, err)
					taskFailed = true
				}

				// Poll for completion
				if !taskFailed {
					for {
						info, err := m.Status(node.Task.ID)
						if err != nil {
							log.Printf("[Worker %d] ✗ Status check failed for %s: %v", id, node.Task.ID, err)
							errors <- fmt.Errorf("status check failed for %s: %w", node.Task.ID, err)
							taskFailed = true
							break
						}
						if info.Status == StatusComplete {
							break
						}
						if info.Status == StatusError {
							log.Printf("[Worker %d] ✗ Task %s failed: %s", id, node.Task.ID, info.Error)
							errors <- fmt.Errorf("task %s failed: %s", node.Task.ID, info.Error)
							taskFailed = true
							break
						}
						if info.Status == StatusStopped {
							log.Printf("[Worker %d] ✗ Task %s stopped", id, node.Task.ID)
							errors <- fmt.Errorf("task %s stopped manually", node.Task.ID)
							taskFailed = true
							break
						}
						time.Sleep(2 * time.Second)
					}
				}

				if taskFailed {
					log.Printf("[Worker %d] ✗ %s failed after %v", id, node.Task.ID, time.Since(start).Truncate(time.Second))
					_ = rel.SetTaskStatus(node.Task.ID, "error")
				} else {
					log.Printf("[Worker %d] ✓ Finished %s in %v (Status: %s)", id, node.Task.ID, time.Since(start).Truncate(time.Second), StatusComplete)
				}

				// Update task status so dependency checks can see completion
				if !taskFailed {
					if err := rel.SetTaskStatus(node.Task.ID, "done"); err != nil {
						log.Printf("[Worker %d] Warning: failed to update task status for %s: %v", id, node.Task.ID, err)
					}
				}

				mu.Lock()
				node.Finished = true
				finishedCount++
				done := finishedCount == totalToRun
				mu.Unlock()

				if done {
					close(readyTasks)
				} else {
					checkReady()
				}
			}
		}(i)
	}

	wg.Wait()
	if len(errors) > 0 {
		return <-errors
	}

	return nil
}
