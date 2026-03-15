package tui

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

// FileSource reads project data from .openexec/ directories
type FileSource struct {
	baseDir     string
	projects    map[string]ProjectInfo
	subscribers map[string][]chan ProjectInfo
	mu          sync.RWMutex
	stopCh      chan struct{}
}

// NewFileSource creates a new file-based data source
func NewFileSource(baseDir string) *FileSource {
	if baseDir == "" {
		baseDir = "."
	}
	fs := &FileSource{
		baseDir:     baseDir,
		projects:    make(map[string]ProjectInfo),
		subscribers: make(map[string][]chan ProjectInfo),
		stopCh:      make(chan struct{}),
	}
	// Start background refresh
	go fs.refreshLoop()
	return fs
}

// List returns all projects found in the base directory (sorted by name)
func (fs *FileSource) List() ([]ProjectInfo, error) {
	fs.mu.RLock()
	defer fs.mu.RUnlock()

	projects := make([]ProjectInfo, 0, len(fs.projects))
	for _, proj := range fs.projects {
		projects = append(projects, proj)
	}

	// Sort by name for stable ordering
	sort.Slice(projects, func(i, j int) bool {
		return projects[i].Name < projects[j].Name
	})

	return projects, nil
}

// Status returns the current status of a project
func (fs *FileSource) Status(name string) (ProjectInfo, error) {
	fs.mu.RLock()
	defer fs.mu.RUnlock()

	if proj, ok := fs.projects[name]; ok {
		return proj, nil
	}
	return ProjectInfo{}, nil
}

// Subscribe returns a channel for receiving project updates
func (fs *FileSource) Subscribe(name string) (<-chan ProjectInfo, func(), error) {
	ch := make(chan ProjectInfo, 64)

	fs.mu.Lock()
	fs.subscribers[name] = append(fs.subscribers[name], ch)
	fs.mu.Unlock()

	cancel := func() {
		fs.mu.Lock()
		defer fs.mu.Unlock()
		subs := fs.subscribers[name]
		for i, sub := range subs {
			if sub == ch {
				fs.subscribers[name] = append(subs[:i], subs[i+1:]...)
				break
			}
		}
		close(ch)
	}

	return ch, cancel, nil
}

// Close stops the file source
func (fs *FileSource) Close() {
	close(fs.stopCh)
}

// refreshLoop periodically scans for project updates
func (fs *FileSource) refreshLoop() {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	// Initial scan
	fs.scan()

	for {
		select {
		case <-ticker.C:
			fs.scan()
		case <-fs.stopCh:
			return
		}
	}
}

// scan looks for .openexec directories and reads project state
func (fs *FileSource) scan() {
	entries, err := os.ReadDir(fs.baseDir)
	if err != nil {
		return
	}

	found := make(map[string]bool)

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		projectDir := filepath.Join(fs.baseDir, entry.Name())
		openexecDir := filepath.Join(projectDir, ".openexec")

		// Check if .openexec directory exists
		if _, err := os.Stat(openexecDir); os.IsNotExist(err) {
			continue
		}

		projectName := entry.Name()
		found[projectName] = true

		// Read project state
		proj := fs.readProjectState(projectName, openexecDir)

		fs.mu.Lock()
		oldProj, existed := fs.projects[projectName]
		fs.projects[projectName] = proj

		// Notify subscribers if changed
		if !existed || !projectsEqual(oldProj, proj) {
			for _, ch := range fs.subscribers[projectName] {
				select {
				case ch <- proj:
				default:
				}
			}
		}
		fs.mu.Unlock()
	}

	// Remove projects that no longer exist
	fs.mu.Lock()
	for name := range fs.projects {
		if !found[name] {
			delete(fs.projects, name)
		}
	}
	fs.mu.Unlock()
}

// readProjectState reads project info from .openexec directory
func (fs *FileSource) readProjectState(name, openexecDir string) ProjectInfo {
	proj := ProjectInfo{
		Name:       name,
		Status:     "unknown",
		Phase:      "unknown",
		LastUpdate: "never",
	}

	// Try to read state.json
	stateFile := filepath.Join(openexecDir, "state.json")
	if data, err := os.ReadFile(stateFile); err == nil {
		var state projectState
		if json.Unmarshal(data, &state) == nil {
			proj.Status = state.Status
			proj.Phase = state.Phase
			proj.WorkerCount = state.WorkerCount
			proj.Progress = state.Progress
		}
	}

	// Try to read tasks from SQLite first (CANONICAL source)
	//
	// JSON READ GUARD:
	// SQLite is the canonical source of truth. JSON fallback is ONLY used when:
	//   - SQLite database doesn't exist (legacy project not yet migrated)
	//   - SQLite query fails (database corruption)
	// The JSON fallback should be considered a migration path, not a runtime source.
	dbPath := filepath.Join(openexecDir, "openexec.db")
	if progress, ok := fs.readProgressFromSQLite(dbPath); ok {
		proj.Progress = progress
	} else {
		// FALLBACK ONLY: Read from tasks.json if SQLite is unavailable
		// This is a migration path for legacy projects, not a runtime source.
		tasksFile := filepath.Join(openexecDir, "tasks.json")
		if data, err := os.ReadFile(tasksFile); err == nil {
			fmt.Fprintf(os.Stderr, "[FALLBACK] Loading from JSON (%s) because SQLite unavailable. Run 'openexec run' to migrate.\n", tasksFile)
			var tasks taskList
			if json.Unmarshal(data, &tasks) == nil {
				if len(tasks.Tasks) > 0 {
					completed := 0
					for _, t := range tasks.Tasks {
						if t.Status == "completed" || t.Status == "done" {
							completed++
						}
					}
					proj.Progress = (completed * 100) / len(tasks.Tasks)
				}
			}
		}
	}

	// Get last update time from state file
	if info, err := os.Stat(stateFile); err == nil {
		proj.LastUpdate = formatTimeSince(info.ModTime())
	} else if info, err := os.Stat(openexecDir); err == nil {
		proj.LastUpdate = formatTimeSince(info.ModTime())
	}

	// Infer status if not set
	if proj.Status == "unknown" || proj.Status == "" {
		proj.Status = inferStatus(openexecDir)
	}

	return proj
}

// projectState represents the state.json structure
type projectState struct {
	Status      string `json:"status"`
	Phase       string `json:"phase"`
	WorkerCount int    `json:"worker_count"`
	Progress    int    `json:"progress"`
}

// taskList represents the tasks.json structure
type taskList struct {
	Tasks []task `json:"tasks"`
}

type task struct {
	ID     string `json:"id"`
	Status string `json:"status"`
}

// readProgressFromSQLite reads task progress from the SQLite database.
// Returns (progress percentage, true) if successful, (0, false) otherwise.
func (fs *FileSource) readProgressFromSQLite(dbPath string) (int, bool) {
	// Check if DB file exists
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		return 0, false
	}

	db, err := sql.Open("sqlite3", dbPath+"?mode=ro")
	if err != nil {
		return 0, false
	}
	defer db.Close()

	// Query task counts from SQLite
	var total, completed int
	err = db.QueryRow(`SELECT COUNT(*) FROM tasks`).Scan(&total)
	if err != nil || total == 0 {
		return 0, false
	}

	err = db.QueryRow(`SELECT COUNT(*) FROM tasks WHERE status IN ('completed', 'done')`).Scan(&completed)
	if err != nil {
		return 0, false
	}

	return (completed * 100) / total, true
}

// inferStatus tries to determine project status from files
func inferStatus(openexecDir string) string {
	// Check for lock file (running)
	if _, err := os.Stat(filepath.Join(openexecDir, ".lock")); err == nil {
		return "running"
	}

	// Check for error log
	errorLog := filepath.Join(openexecDir, "error.log")
	if info, err := os.Stat(errorLog); err == nil && info.Size() > 0 {
		return "error"
	}

	// Check for completion marker
	if _, err := os.Stat(filepath.Join(openexecDir, "complete")); err == nil {
		return "complete"
	}

	// Check for pause marker
	if _, err := os.Stat(filepath.Join(openexecDir, "paused")); err == nil {
		return "paused"
	}

	// Default to idle
	return "idle"
}

// formatTimeSince formats a time as "X seconds/minutes/hours ago"
func formatTimeSince(t time.Time) string {
	d := time.Since(t)

	switch {
	case d < time.Minute:
		secs := int(d.Seconds())
		if secs == 1 {
			return "1 second ago"
		}
		return strings.Replace(d.Truncate(time.Second).String(), "s", " seconds ago", 1)
	case d < time.Hour:
		mins := int(d.Minutes())
		if mins == 1 {
			return "1 minute ago"
		}
		return strings.Replace(d.Truncate(time.Minute).String(), "m0s", " minutes ago", 1)
	case d < 24*time.Hour:
		hours := int(d.Hours())
		if hours == 1 {
			return "1 hour ago"
		}
		return strings.Replace(d.Truncate(time.Hour).String(), "h0m0s", " hours ago", 1)
	default:
		days := int(d.Hours() / 24)
		if days == 1 {
			return "1 day ago"
		}
		return t.Format("2006-01-02")
	}
}

// projectsEqual checks if two projects have the same values
func projectsEqual(a, b ProjectInfo) bool {
	return a.Name == b.Name &&
		a.Status == b.Status &&
		a.Phase == b.Phase &&
		a.WorkerCount == b.WorkerCount &&
		a.Progress == b.Progress
}
