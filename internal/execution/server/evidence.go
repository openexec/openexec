package server

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/openexec/openexec/pkg/api"
)

// SessionInfo holds metadata for a single execution session (retry attempt)
type SessionInfo struct {
	ID        string    `json:"id"`
	StartTime time.Time `json:"start_time"`
	EndTime   time.Time `json:"end_time"`
	Duration  string    `json:"duration"`
	ExitCode  int       `json:"exit_code"`
	Error     string    `json:"error"`
	HasLog    bool      `json:"has_log"`
}

// TaskEvidence represents the full implementation timeline for a task
type TaskEvidence struct {
	TaskID   string        `json:"task_id"`
	Sessions []SessionInfo `json:"sessions"`
}

// handleTaskEvidence returns the execution timeline for a specific task
func (s *Server) handleTaskEvidence(w http.ResponseWriter, r *http.Request) {
	taskID := r.URL.Query().Get("taskId")
	if taskID == "" {
		api.WriteError(w, http.StatusBadRequest, "missing taskId parameter")
		return
	}

	// In the consolidated structure, evidence is in dataDir/taskID/
	evidenceDir := filepath.Join(s.projectsDir, ".openexec", "data", taskID)
	
	entries, err := os.ReadDir(evidenceDir)
	if err != nil {
		if os.IsNotExist(err) {
			api.WriteJSON(w, http.StatusOK, TaskEvidence{TaskID: taskID, Sessions: []SessionInfo{}})
			return
		}
		api.WriteError(w, http.StatusInternalServerError, "failed to read evidence: "+err.Error())
		return
	}

	var sessions []SessionInfo
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		sessionID := entry.Name()
		sessionPath := filepath.Join(evidenceDir, sessionID)
		
		// Try to load meta.json
		metaPath := filepath.Join(sessionPath, "meta.json")
		data, err := os.ReadFile(metaPath)
		
		var info SessionInfo
		info.ID = sessionID
		info.HasLog = false
		if _, err := os.Stat(filepath.Join(sessionPath, "claude.log")); err == nil {
			info.HasLog = true
		}

		if err == nil {
			var meta struct {
				StartTime string `json:"start_time"`
				EndTime   string `json:"end_time"`
				ExitCode  int    `json:"exit_code"`
				Error     string `json:"error"`
			}
			if err := json.Unmarshal(data, &meta); err == nil {
				info.StartTime, _ = time.Parse(time.RFC3339, meta.StartTime)
				info.EndTime, _ = time.Parse(time.RFC3339, meta.EndTime)
				info.ExitCode = meta.ExitCode
				info.Error = meta.Error
				if !info.EndTime.IsZero() && !info.StartTime.IsZero() {
					info.Duration = info.EndTime.Sub(info.StartTime).String()
				}
			}
		}
		
		sessions = append(sessions, info)
	}

	// Sort sessions by start time (newest last for timeline)
	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].StartTime.Before(sessions[j].StartTime)
	})

	api.WriteJSON(w, http.StatusOK, TaskEvidence{
		TaskID:   taskID,
		Sessions: sessions,
	})
}

// handleSessionLog serves the raw log file for a specific session attempt
func (s *Server) handleSessionLog(w http.ResponseWriter, r *http.Request) {
	taskID := r.URL.Query().Get("taskId")
	sessionID := r.URL.Query().Get("sessionId")
	
	if taskID == "" || sessionID == "" {
		api.WriteError(w, http.StatusBadRequest, "missing taskId or sessionId")
		return
	}

	logPath := filepath.Join(s.projectsDir, ".openexec", "data", taskID, sessionID, "claude.log")
	
	// Ensure path is safe (no .. traversal)
	if strings.Contains(taskID, "..") || strings.Contains(sessionID, "..") {
		api.WriteError(w, http.StatusForbidden, "invalid path")
		return
	}

	content, err := os.ReadFile(logPath)
	if err != nil {
		api.WriteError(w, http.StatusNotFound, "log file not found")
		return
	}

	w.Header().Set("Content-Type", "text/plain")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(content)
}
