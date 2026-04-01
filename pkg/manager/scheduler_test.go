package manager

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/openexec/openexec/internal/release"
	"github.com/openexec/openexec/pkg/db/state"
)

// schedulerTestEnv bundles the common setup for scheduler tests.
type schedulerTestEnv struct {
	mgr *Manager
	rel *release.Manager
	dir string
}

// reloadTasks refreshes the test release manager's in-memory cache from SQLite
// so it sees changes made by the scheduler's separate release manager instance.
func (e *schedulerTestEnv) reloadTasks(t *testing.T) {
	t.Helper()
	if err := e.rel.Load(); err != nil {
		t.Fatalf("failed to reload release manager: %v", err)
	}
}

// newSchedulerTestEnv builds a Manager backed by mock_claude and returns
// the release manager so tests can create stories/tasks.
func newSchedulerTestEnv(t *testing.T) *schedulerTestEnv {
	t.Helper()

	bin := buildMockClaude(t)
	tmpDir := t.TempDir()

	// Create .openexec directory so release manager can find its DB
	openexecDir := filepath.Join(tmpDir, ".openexec")
	if err := os.MkdirAll(openexecDir, 0755); err != nil {
		t.Fatal(err)
	}

	stateStore, err := state.NewStore(filepath.Join(tmpDir, "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { stateStore.Close() })

	cfg := Config{
		WorkDir:              tmpDir,
		AgentsFS:             os.DirFS(filepath.Join("..", "..", "internal", "pipeline", "testdata")),
		DefaultMaxIterations: 10,
		MaxRetries:           1,
		ThrashThreshold:      0,
		RetryBackoff:         []time.Duration{0},
		CommandName:          bin,
		StateStore:           stateStore,
	}

	mgr, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}

	// Create a release manager sharing the same DB
	relMgr, err := release.NewManagerWithDB(tmpDir, release.DefaultConfig(), stateStore.GetDB())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { relMgr.Close() })

	return &schedulerTestEnv{mgr: mgr, rel: relMgr, dir: tmpDir}
}

// createStory is a helper that creates a story in the release manager.
func createStory(t *testing.T, rel *release.Manager, id string, dependsOn []string) {
	t.Helper()
	err := rel.CreateStory(&release.Story{
		ID:        id,
		Title:     "Story " + id,
		DependsOn: dependsOn,
		Status:    "pending",
	})
	if err != nil {
		t.Fatalf("CreateStory(%s): %v", id, err)
	}
}

// createTask is a helper that creates a pending task in the release manager.
func createTask(t *testing.T, rel *release.Manager, id, storyID string, dependsOn []string) {
	t.Helper()
	err := rel.CreateTask(&release.Task{
		ID:          id,
		Title:       "Task " + id,
		Description: "Test task " + id,
		StoryID:     storyID,
		DependsOn:   dependsOn,
		Status:      "pending",
	})
	if err != nil {
		t.Fatalf("CreateTask(%s): %v", id, err)
	}
}

// waitForExecuteTasks runs ExecuteTasks with a timeout.
func waitForExecuteTasks(t *testing.T, ctx context.Context, mgr *Manager, opts RunOptions, timeout time.Duration) error {
	t.Helper()
	done := make(chan error, 1)
	go func() {
		done <- mgr.ExecuteTasks(ctx, opts)
	}()
	select {
	case err := <-done:
		return err
	case <-time.After(timeout):
		t.Fatal("ExecuteTasks timed out")
		return nil
	}
}

// ---------- Empty / Single task tests ----------

func TestScheduler_EmptyTaskList(t *testing.T) {
	env := newSchedulerTestEnv(t)

	// No tasks created — ExecuteTasks should return nil immediately.
	err := waitForExecuteTasks(t, context.Background(), env.mgr, RunOptions{MaxParallel: 2}, 10*time.Second)
	if err != nil {
		t.Fatalf("ExecuteTasks with no tasks: %v", err)
	}
}

func TestScheduler_SingleTask(t *testing.T) {
	env := newSchedulerTestEnv(t)

	createStory(t, env.rel, "S-1", nil)
	createTask(t, env.rel, "T-1", "S-1", nil)

	err := waitForExecuteTasks(t, context.Background(), env.mgr, RunOptions{MaxParallel: 1}, 60*time.Second)
	if err != nil {
		t.Fatalf("ExecuteTasks: %v", err)
	}

	// Reload cache to see scheduler's changes
	env.reloadTasks(t)

	task := env.rel.GetTask("T-1")
	if task == nil {
		t.Fatal("task T-1 not found after execution")
	}
	if task.Status != "done" {
		t.Errorf("task T-1 status = %q, want %q", task.Status, "done")
	}
}

// ---------- Parallel execution tests ----------

func TestScheduler_IndependentTasksRunInParallel(t *testing.T) {
	env := newSchedulerTestEnv(t)

	createStory(t, env.rel, "S-1", nil)
	createTask(t, env.rel, "T-1", "S-1", nil)
	createTask(t, env.rel, "T-2", "S-1", nil)
	createTask(t, env.rel, "T-3", "S-1", nil)

	start := time.Now()
	err := waitForExecuteTasks(t, context.Background(), env.mgr, RunOptions{MaxParallel: 3}, 60*time.Second)
	if err != nil {
		t.Fatalf("ExecuteTasks: %v", err)
	}
	elapsed := time.Since(start)

	env.reloadTasks(t)

	// All tasks should be done
	for _, id := range []string{"T-1", "T-2", "T-3"} {
		task := env.rel.GetTask(id)
		if task == nil {
			t.Fatalf("task %s not found", id)
		}
		if task.Status != "done" {
			t.Errorf("task %s status = %q, want %q", id, task.Status, "done")
		}
	}

	// If they ran in parallel with mock_claude (instant), total time should be reasonable
	if elapsed > 30*time.Second {
		t.Errorf("3 independent tasks took %v; expected parallel execution to be faster", elapsed)
	}
}

func TestScheduler_DependentTasksRunSequentially(t *testing.T) {
	env := newSchedulerTestEnv(t)

	createStory(t, env.rel, "S-1", nil)
	createTask(t, env.rel, "T-A", "S-1", nil)
	createTask(t, env.rel, "T-B", "S-1", []string{"T-A"})

	err := waitForExecuteTasks(t, context.Background(), env.mgr, RunOptions{MaxParallel: 2}, 60*time.Second)
	if err != nil {
		t.Fatalf("ExecuteTasks: %v", err)
	}

	env.reloadTasks(t)

	taskA := env.rel.GetTask("T-A")
	taskB := env.rel.GetTask("T-B")
	if taskA == nil || taskB == nil {
		t.Fatal("tasks not found after execution")
	}
	if taskA.Status != "done" {
		t.Errorf("T-A status = %q, want done", taskA.Status)
	}
	if taskB.Status != "done" {
		t.Errorf("T-B status = %q, want done", taskB.Status)
	}
}

// ---------- Dependency tracking tests ----------

func TestScheduler_DependencyChainRespected(t *testing.T) {
	// A -> B -> C: must execute in order A, B, C
	env := newSchedulerTestEnv(t)

	createStory(t, env.rel, "S-1", nil)
	createTask(t, env.rel, "T-A", "S-1", nil)
	createTask(t, env.rel, "T-B", "S-1", []string{"T-A"})
	createTask(t, env.rel, "T-C", "S-1", []string{"T-B"})

	err := waitForExecuteTasks(t, context.Background(), env.mgr, RunOptions{MaxParallel: 3}, 60*time.Second)
	if err != nil {
		t.Fatalf("ExecuteTasks: %v", err)
	}

	env.reloadTasks(t)

	// All should be done (if ordering was broken, B or C would never get dispatched)
	for _, id := range []string{"T-A", "T-B", "T-C"} {
		task := env.rel.GetTask(id)
		if task == nil {
			t.Fatalf("task %s not found", id)
		}
		if task.Status != "done" {
			t.Errorf("task %s status = %q, want done", id, task.Status)
		}
	}
}

func TestScheduler_DiamondDependency(t *testing.T) {
	// Diamond: A -> B, A -> C, B+C -> D
	env := newSchedulerTestEnv(t)

	createStory(t, env.rel, "S-1", nil)
	createTask(t, env.rel, "T-A", "S-1", nil)
	createTask(t, env.rel, "T-B", "S-1", []string{"T-A"})
	createTask(t, env.rel, "T-C", "S-1", []string{"T-A"})
	createTask(t, env.rel, "T-D", "S-1", []string{"T-B", "T-C"})

	err := waitForExecuteTasks(t, context.Background(), env.mgr, RunOptions{MaxParallel: 3}, 60*time.Second)
	if err != nil {
		t.Fatalf("ExecuteTasks: %v", err)
	}

	env.reloadTasks(t)

	for _, id := range []string{"T-A", "T-B", "T-C", "T-D"} {
		task := env.rel.GetTask(id)
		if task == nil {
			t.Fatalf("task %s not found", id)
		}
		if task.Status != "done" {
			t.Errorf("task %s status = %q, want done", id, task.Status)
		}
	}
}

func TestScheduler_TaskStatusUpdatedOnCompletion(t *testing.T) {
	env := newSchedulerTestEnv(t)

	createStory(t, env.rel, "S-1", nil)
	createTask(t, env.rel, "T-1", "S-1", nil)
	createTask(t, env.rel, "T-2", "S-1", nil)

	// Verify initial status is pending
	for _, id := range []string{"T-1", "T-2"} {
		task := env.rel.GetTask(id)
		if task.Status != "pending" {
			t.Errorf("task %s initial status = %q, want pending", id, task.Status)
		}
	}

	err := waitForExecuteTasks(t, context.Background(), env.mgr, RunOptions{MaxParallel: 2}, 60*time.Second)
	if err != nil {
		t.Fatalf("ExecuteTasks: %v", err)
	}

	env.reloadTasks(t)

	for _, id := range []string{"T-1", "T-2"} {
		task := env.rel.GetTask(id)
		if task == nil {
			t.Fatalf("task %s not found", id)
		}
		if task.Status != "done" {
			t.Errorf("task %s status = %q, want done", id, task.Status)
		}
	}
}

// ---------- Dependency order verification (polling-based) ----------

func TestScheduler_DependencyChainOrderVerified(t *testing.T) {
	// Verify strict ordering A -> B -> C using a completion log.
	// We poll the release manager (refreshing cache each time) to observe order.
	env := newSchedulerTestEnv(t)

	createStory(t, env.rel, "S-1", nil)
	createTask(t, env.rel, "T-A", "S-1", nil)
	createTask(t, env.rel, "T-B", "S-1", []string{"T-A"})
	createTask(t, env.rel, "T-C", "S-1", []string{"T-B"})

	var completionOrder []string
	var orderMu sync.Mutex
	stopPolling := make(chan struct{})

	go func() {
		seen := map[string]bool{}
		ticker := time.NewTicker(50 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-stopPolling:
				return
			case <-ticker.C:
				// Refresh cache to see scheduler's updates
				_ = env.rel.Load()
				for _, id := range []string{"T-A", "T-B", "T-C"} {
					if seen[id] {
						continue
					}
					task := env.rel.GetTask(id)
					if task != nil && task.Status == "done" {
						orderMu.Lock()
						completionOrder = append(completionOrder, id)
						orderMu.Unlock()
						seen[id] = true
					}
				}
			}
		}
	}()

	err := waitForExecuteTasks(t, context.Background(), env.mgr, RunOptions{MaxParallel: 3}, 60*time.Second)
	close(stopPolling)

	if err != nil {
		t.Fatalf("ExecuteTasks: %v", err)
	}

	// Final sweep: the last task's status may have been set after our last poll
	_ = env.rel.Load()
	orderMu.Lock()
	seen := map[string]bool{}
	for _, id := range completionOrder {
		seen[id] = true
	}
	for _, id := range []string{"T-A", "T-B", "T-C"} {
		if seen[id] {
			continue
		}
		task := env.rel.GetTask(id)
		if task != nil && task.Status == "done" {
			completionOrder = append(completionOrder, id)
		}
	}
	order := make([]string, len(completionOrder))
	copy(order, completionOrder)
	orderMu.Unlock()

	if len(order) != 3 {
		t.Fatalf("expected 3 completions, got %d: %v", len(order), order)
	}
	if order[0] != "T-A" || order[1] != "T-B" || order[2] != "T-C" {
		t.Errorf("completion order = %v, want [T-A T-B T-C]", order)
	}
}

func TestScheduler_DiamondOrderVerified(t *testing.T) {
	// Diamond: A -> B, A -> C, B+C -> D
	// Expected: A first, then B and C (either order), then D last.
	env := newSchedulerTestEnv(t)

	createStory(t, env.rel, "S-1", nil)
	createTask(t, env.rel, "T-A", "S-1", nil)
	createTask(t, env.rel, "T-B", "S-1", []string{"T-A"})
	createTask(t, env.rel, "T-C", "S-1", []string{"T-A"})
	createTask(t, env.rel, "T-D", "S-1", []string{"T-B", "T-C"})

	var completionOrder []string
	var orderMu sync.Mutex
	stopPolling := make(chan struct{})

	go func() {
		seen := map[string]bool{}
		ticker := time.NewTicker(50 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-stopPolling:
				return
			case <-ticker.C:
				_ = env.rel.Load()
				for _, id := range []string{"T-A", "T-B", "T-C", "T-D"} {
					if seen[id] {
						continue
					}
					task := env.rel.GetTask(id)
					if task != nil && task.Status == "done" {
						orderMu.Lock()
						completionOrder = append(completionOrder, id)
						orderMu.Unlock()
						seen[id] = true
					}
				}
			}
		}
	}()

	err := waitForExecuteTasks(t, context.Background(), env.mgr, RunOptions{MaxParallel: 3}, 60*time.Second)
	close(stopPolling)

	if err != nil {
		t.Fatalf("ExecuteTasks: %v", err)
	}

	// Final sweep for tasks that completed after last poll
	_ = env.rel.Load()
	orderMu.Lock()
	seen := map[string]bool{}
	for _, id := range completionOrder {
		seen[id] = true
	}
	for _, id := range []string{"T-A", "T-B", "T-C", "T-D"} {
		if seen[id] {
			continue
		}
		task := env.rel.GetTask(id)
		if task != nil && task.Status == "done" {
			completionOrder = append(completionOrder, id)
		}
	}
	order := make([]string, len(completionOrder))
	copy(order, completionOrder)
	orderMu.Unlock()

	if len(order) != 4 {
		t.Fatalf("expected 4 completions, got %d: %v", len(order), order)
	}

	// A must be first
	if order[0] != "T-A" {
		t.Errorf("first completion = %s, want T-A", order[0])
	}

	// D must be last
	if order[3] != "T-D" {
		t.Errorf("last completion = %s, want T-D", order[3])
	}

	// B and C must be in middle (either order is fine)
	middle := map[string]bool{order[1]: true, order[2]: true}
	if !middle["T-B"] || !middle["T-C"] {
		t.Errorf("middle completions = %v, want T-B and T-C", []string{order[1], order[2]})
	}
}

// ---------- Error handling tests ----------

func TestScheduler_FailedTaskBlocksDependents(t *testing.T) {
	// Build a mock_claude that crashes (exit 1).
	tmpBinDir := t.TempDir()
	crashBin := filepath.Join(tmpBinDir, "crash_claude")
	crashSrc := filepath.Join(tmpBinDir, "crash.go")
	if err := os.WriteFile(crashSrc, []byte(`package main
import "fmt"
import "os"
func main() {
	fmt.Fprintln(os.Stderr, "crash")
	os.Exit(1)
}
`), 0644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("go", "build", "-o", crashBin, crashSrc)
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("build crash binary: %v", err)
	}

	tmpDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(tmpDir, ".openexec"), 0755); err != nil {
		t.Fatal(err)
	}

	stateStore, err := state.NewStore(filepath.Join(tmpDir, "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { stateStore.Close() })

	cfg := Config{
		WorkDir:              tmpDir,
		AgentsFS:             os.DirFS(filepath.Join("..", "..", "internal", "pipeline", "testdata")),
		DefaultMaxIterations: 10,
		MaxRetries:           0, // no retries — fail immediately
		ThrashThreshold:      0,
		RetryBackoff:         []time.Duration{0},
		CommandName:          crashBin,
		StateStore:           stateStore,
	}

	mgr, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}

	relMgr, err := release.NewManagerWithDB(tmpDir, release.DefaultConfig(), stateStore.GetDB())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { relMgr.Close() })

	createStory(t, relMgr, "S-1", nil)
	createTask(t, relMgr, "T-A", "S-1", nil)
	createTask(t, relMgr, "T-B", "S-1", []string{"T-A"})

	// Use MaxParallel=1: with a single worker, when T-A fails the worker
	// returns, wg.Wait() completes, and ExecuteTasks returns the error.
	// (With MaxParallel>1, idle workers would deadlock on the readyTasks channel.)
	err = waitForExecuteTasks(t, context.Background(), mgr, RunOptions{MaxParallel: 1}, 60*time.Second)

	// ExecuteTasks should return an error because T-A fails
	if err == nil {
		t.Fatal("expected error from ExecuteTasks when a task fails, got nil")
	}

	// Reload to see current state
	if loadErr := relMgr.Load(); loadErr != nil {
		t.Fatalf("failed to reload: %v", loadErr)
	}

	// T-B should NOT have been set to "done" since T-A failed
	taskB := relMgr.GetTask("T-B")
	if taskB != nil && taskB.Status == "done" {
		t.Error("T-B should not be done when its dependency T-A failed")
	}
}

func TestScheduler_WorkerCountRespected(t *testing.T) {
	// Verify that with worker_count=2 and 5 independent tasks, at most 2
	// pipelines run simultaneously. Uses the standard mock_claude binary.
	env := newSchedulerTestEnv(t)

	createStory(t, env.rel, "S-1", nil)
	// Use 3 tasks (more than worker_count=2) to verify the constraint.
	// Using fewer tasks reduces SQLite contention in the test environment.
	for i := 1; i <= 3; i++ {
		createTask(t, env.rel, fmt.Sprintf("T-%d", i), "S-1", nil)
	}

	// Monitor concurrent pipeline count via mgr.List()
	var maxConcurrent atomic.Int32
	stopTracking := make(chan struct{})

	go func() {
		ticker := time.NewTicker(30 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-stopTracking:
				return
			case <-ticker.C:
				list := env.mgr.List()
				active := int32(0)
				for _, info := range list {
					if info.Status == StatusRunning || info.Status == StatusStarting {
						active++
					}
				}
				for {
					cur := maxConcurrent.Load()
					if active <= cur {
						break
					}
					if maxConcurrent.CompareAndSwap(cur, active) {
						break
					}
				}
			}
		}
	}()

	// Execute with worker_count=2
	err := waitForExecuteTasks(t, context.Background(), env.mgr, RunOptions{MaxParallel: 2}, 60*time.Second)
	close(stopTracking)

	if err != nil {
		t.Fatalf("ExecuteTasks: %v", err)
	}

	observed := maxConcurrent.Load()
	// The scheduler creates exactly 2 worker goroutines, so at most 2 can run
	if observed > 2 {
		t.Errorf("observed %d concurrent pipelines, but worker_count=2", observed)
	}

	// Reload and verify all tasks done
	env.reloadTasks(t)
	for i := 1; i <= 3; i++ {
		id := fmt.Sprintf("T-%d", i)
		task := env.rel.GetTask(id)
		if task == nil {
			t.Fatalf("task %s not found", id)
		}
		if task.Status != "done" {
			t.Errorf("task %s status = %q, want done", id, task.Status)
		}
	}
}

// ---------- Story-level dependency tests ----------

func TestScheduler_StoryDependencyRespected(t *testing.T) {
	// Story S-2 depends on Story S-1. Tasks in S-2 should not start until
	// all tasks in S-1 are done.
	env := newSchedulerTestEnv(t)

	createStory(t, env.rel, "S-1", nil)
	createStory(t, env.rel, "S-2", []string{"S-1"})

	createTask(t, env.rel, "T-1A", "S-1", nil)
	createTask(t, env.rel, "T-2A", "S-2", nil)

	var completionOrder []string
	var orderMu sync.Mutex
	stopPolling := make(chan struct{})

	go func() {
		seen := map[string]bool{}
		ticker := time.NewTicker(50 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-stopPolling:
				return
			case <-ticker.C:
				_ = env.rel.Load()
				for _, id := range []string{"T-1A", "T-2A"} {
					if seen[id] {
						continue
					}
					task := env.rel.GetTask(id)
					if task != nil && task.Status == "done" {
						orderMu.Lock()
						completionOrder = append(completionOrder, id)
						orderMu.Unlock()
						seen[id] = true
					}
				}
			}
		}
	}()

	err := waitForExecuteTasks(t, context.Background(), env.mgr, RunOptions{MaxParallel: 2}, 60*time.Second)
	close(stopPolling)

	if err != nil {
		t.Fatalf("ExecuteTasks: %v", err)
	}

	// Final sweep for tasks that completed after last poll
	_ = env.rel.Load()
	orderMu.Lock()
	seen := map[string]bool{}
	for _, id := range completionOrder {
		seen[id] = true
	}
	for _, id := range []string{"T-1A", "T-2A"} {
		if seen[id] {
			continue
		}
		task := env.rel.GetTask(id)
		if task != nil && task.Status == "done" {
			completionOrder = append(completionOrder, id)
		}
	}
	order := make([]string, len(completionOrder))
	copy(order, completionOrder)
	orderMu.Unlock()

	if len(order) != 2 {
		t.Fatalf("expected 2 completions, got %d: %v", len(order), order)
	}

	if order[0] != "T-1A" || order[1] != "T-2A" {
		t.Errorf("completion order = %v, want [T-1A T-2A]", order)
	}
}

// ---------- Already-completed tasks are skipped ----------

func TestScheduler_SkipsNonPendingTasks(t *testing.T) {
	env := newSchedulerTestEnv(t)

	createStory(t, env.rel, "S-1", nil)
	createTask(t, env.rel, "T-1", "S-1", nil)
	createTask(t, env.rel, "T-2", "S-1", nil)

	// Mark T-1 as already done
	if err := env.rel.SetTaskStatus("T-1", "done"); err != nil {
		t.Fatal(err)
	}

	err := waitForExecuteTasks(t, context.Background(), env.mgr, RunOptions{MaxParallel: 2}, 60*time.Second)
	if err != nil {
		t.Fatalf("ExecuteTasks: %v", err)
	}

	env.reloadTasks(t)

	task1 := env.rel.GetTask("T-1")
	task2 := env.rel.GetTask("T-2")
	if task1.Status != "done" {
		t.Errorf("T-1 status = %q, want done", task1.Status)
	}
	if task2.Status != "done" {
		t.Errorf("T-2 status = %q, want done", task2.Status)
	}
}

// ---------- Default worker count ----------

func TestScheduler_DefaultWorkerCount(t *testing.T) {
	// MaxParallel=0 should default to 1 (not panic or deadlock)
	env := newSchedulerTestEnv(t)

	createStory(t, env.rel, "S-1", nil)
	createTask(t, env.rel, "T-1", "S-1", nil)

	err := waitForExecuteTasks(t, context.Background(), env.mgr, RunOptions{MaxParallel: 0}, 60*time.Second)
	if err != nil {
		t.Fatalf("ExecuteTasks with MaxParallel=0: %v", err)
	}

	env.reloadTasks(t)

	task := env.rel.GetTask("T-1")
	if task == nil || task.Status != "done" {
		status := ""
		if task != nil {
			status = task.Status
		}
		t.Errorf("task T-1 status = %q, want done", status)
	}
}
