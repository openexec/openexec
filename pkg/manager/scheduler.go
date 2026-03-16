package manager

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/openexec/openexec/internal/prompt"
	"github.com/openexec/openexec/internal/release"
)

// RunOptions defines settings for executing multiple tasks.
type RunOptions struct {
	WorkerCount int      `json:"worker_count"`
	TaskIDs     []string `json:"task_ids,omitempty"`
}

type taskNode struct {
	Task      *release.Task
	DependsOn map[string]bool
	Status    string // pending, running, completed, failed
}

// ExecuteTasks orchestrates the parallel execution of multiple tasks with dependency tracking.
// This moves the core loop from the CLI to the Daemon.
func (m *Manager) ExecuteTasks(ctx context.Context, opts RunOptions) error {
	rel, err := m.getInternalReleaseManager()
	if err != nil {
		return fmt.Errorf("failed to load release manager: %w", err)
	}

	allTasks := rel.GetTasks()
	if len(allTasks) == 0 {
		return fmt.Errorf("no tasks found in database")
	}

	// 1. Build and filter nodes
	nodes := make(map[string]*taskNode)
	var tasksToRun []*taskNode
	
	idMap := make(map[string]bool)
	for _, id := range opts.TaskIDs { idMap[id] = true }

	for _, t := range allTasks {
		if len(opts.TaskIDs) > 0 && !idMap[t.ID] {
			continue
		}
		
		node := &taskNode{
			Task:      t,
			DependsOn: make(map[string]bool),
			Status:    t.Status,
		}
		for _, dep := range t.DependsOn {
			node.DependsOn[dep] = true
		}
		nodes[t.ID] = node
		if t.Status != string(release.TaskStatusDone) && t.Status != "completed" {
			tasksToRun = append(tasksToRun, node)
		}
	}

	if len(tasksToRun) == 0 {
		return nil
	}

	workerCount := opts.WorkerCount
	if workerCount <= 0 { workerCount = 4 }
	if workerCount > len(tasksToRun) { workerCount = len(tasksToRun) }

	var mu sync.Mutex
	var wg sync.WaitGroup
	readyTasks := make(chan *taskNode, len(tasksToRun))
	errors := make(chan error, len(tasksToRun))
	
	finishedCount := 0
	totalToRun := len(tasksToRun)

	checkReady := func() {
		mu.Lock()
		defer mu.Unlock()

		if finishedCount == totalToRun || len(errors) > 0 {
			return
		}

		for _, node := range nodes {
			if node.Status != string(release.TaskStatusPending) {
				continue
			}

			allDone := true
			for depID := range node.DependsOn {
				depNode, exists := nodes[depID]
				if exists && depNode.Status != string(release.TaskStatusDone) && depNode.Status != "completed" {
					allDone = false
					break
				}
			}

			if allDone {
				node.Status = "ready"
				readyTasks <- node
			}
		}
	}

	// Initial check
	go checkReady()

	// Start workers
	for i := 0; i < workerCount; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for node := range readyTasks {
				log.Printf("[Scheduler] Worker %d starting task %s", id, node.Task.ID)
				
				// Determine if it's a study task
				isStudy := strings.Contains(strings.ToLower(node.Task.Title), "study") ||
						  strings.Contains(strings.ToLower(node.Task.Title), "map")

				// Build rich task briefing from release manager
				taskDesc := node.Task.Title
				if node.Task.Description != "" {
					taskDesc += "\n\n" + node.Task.Description
				}
				// Try to get full briefing with acceptance criteria, dependencies, etc.
				if brief, err := rel.Brief(node.Task.ID); err == nil {
					taskDesc = prompt.FormatBriefing(brief)
				}

				var opts []StartOption
				opts = append(opts, WithTaskDescription(taskDesc))
				opts = append(opts, WithBlueprint("standard_task"))
				if isStudy {
					opts = append(opts, WithIsStudy(true))
				}

				// Start the individual pipeline
				if err := m.Start(ctx, node.Task.ID, opts...); err != nil {
					errors <- fmt.Errorf("failed to start task %s: %w", node.Task.ID, err)
					return
				}

				// Wait for completion via polling (V1 simplicity)
				ticker := time.NewTicker(2 * time.Second)
				done := false
				for !done {
					select {
					case <-ctx.Done():
						ticker.Stop()
						return
					case <-ticker.C:
						info, err := m.Status(node.Task.ID)
						if err != nil {
							errors <- err
							ticker.Stop()
							return
						}
						if isTerminal(info.Status) {
							log.Printf("[Scheduler] Task %s finished: status=%s elapsed=%s error=%q", node.Task.ID, info.Status, info.Elapsed, info.Error)
							if info.Status == StatusError {
								errors <- fmt.Errorf("task %s failed: %s", node.Task.ID, info.Error)
							}
							done = true
						}
					}
				}
				ticker.Stop()

				mu.Lock()
				node.Status = string(release.TaskStatusDone)
				finishedCount++
				mu.Unlock()

				if finishedCount == totalToRun {
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
