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
	MaxParallel int    `json:"max_parallel"`
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
		Task     *release.Task
		Deps     []string
		Finished bool
	}

	nodes := make(map[string]*node)
	for _, t := range pending {
		nodes[t.ID] = &node{Task: t, Deps: t.DependsOn}
	}

	var mu sync.Mutex
	wg := sync.WaitGroup{}
	readyTasks := make(chan *node, len(pending))
	errors := make(chan error, len(pending))
	finishedCount := 0
	totalToRun := len(pending)

	checkReady := func() {
		mu.Lock()
		defer mu.Unlock()
		for id, n := range nodes {
			if n.Finished {
				continue
			}
			allDepsDone := true
			for _, depID := range n.Deps {
				if d, ok := nodes[depID]; ok && !d.Finished {
					allDepsDone = false
					break
				}
			}
			if allDepsDone {
				readyTasks <- n
				delete(nodes, id)
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
				
				var optsList []StartOption
				if opts.IsStudy {
					optsList = append(optsList, WithIsStudy(true))
				}
				// Only override ExecMode for valid permission modes.
				// Operational modes (chat/task/run) must not replace the exec permission.
				switch opts.Mode {
				case "read-only", "workspace-write", "danger-full-access":
					optsList = append(optsList, WithExecMode(opts.Mode))
				}
				
				// Ensure blueprint metadata is passed
				optsList = append(optsList, WithBlueprint("standard_task"))
				optsList = append(optsList, WithTaskDescription(node.Task.Description))

				// Start the individual pipeline
				if err := m.Start(ctx, node.Task.ID, optsList...); err != nil {
					errors <- fmt.Errorf("failed to start task %s: %w", node.Task.ID, err)
					return
				}

				// Poll for completion (V1.0 simple wait)
				// TODO: Replace with event-driven completion signal
				for {
					info, err := m.Status(node.Task.ID)
					if err != nil {
						errors <- fmt.Errorf("status check failed for %s: %w", node.Task.ID, err)
						return
					}
					if info.Status == StatusComplete {
						break
					}
					if info.Status == StatusError {
						errors <- fmt.Errorf("task %s failed: %s", node.Task.ID, info.Error)
						return
					}
					if info.Status == StatusStopped {
						errors <- fmt.Errorf("task %s stopped manually", node.Task.ID)
						return
					}
					time.Sleep(2 * time.Second)
				}

				log.Printf("[Worker %d] ✓ Finished %s in %v (Status: %s)", id, node.Task.ID, time.Since(start).Truncate(time.Second), StatusComplete)

				mu.Lock()
				node.Finished = true
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
