package api

import (
    "context"
    "encoding/json"
    "fmt"
    "net/http"
    "strings"
    "time"

    "github.com/openexec/openexec/pkg/audit"
    "github.com/openexec/openexec/pkg/manager"
)

// Legacy loops endpoint removed; use /api/fwu/{id}/start or /api/v1/runs

// Legacy loops endpoint removed; use /api/fwu/{id}/status

// handleCreateRun creates a new run from an explicit request and returns a run_id.
// This improves determinism by persisting the inputs up front.
func (s *Server) handleCreateRun(w http.ResponseWriter, r *http.Request) {
    var req struct {
        WorkDir       string `json:"work_dir"`
        QuickfixTitle string `json:"quickfix_title,omitempty"`
        VerifyScript  string `json:"verify_script,omitempty"`
        Mode          string `json:"mode,omitempty"`
    }
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        WriteError(w, http.StatusBadRequest, "invalid request body")
        return
    }

    // Derive a run_id (reusing the FWU ID pattern)
    now := time.Now().UTC().Format("20060102-150405")
    runID := "RUN-" + now
    if req.QuickfixTitle != "" {
        runID = "T-QF-" + now
    }

    // Persist the run request to audit for replay determinism
    if s.AuditLogger != nil {
        builder, err := audit.NewEntry(audit.EventRunCreated, "openexec", "system")
        if err == nil {
            e, _ := builder.WithProject(s.ProjectsDir).
                WithMetadata(map[string]interface{}{
                    "event":          "run.created",
                    "run_id":         runID,
                    "work_dir":       req.WorkDir,
                    "quickfix_title": req.QuickfixTitle,
                    "verify_script":  req.VerifyScript,
                    "mode":           req.Mode,
                }).Build()
            _ = s.AuditLogger.Log(r.Context(), e)
        }
    }

    // Return the run_id; clients can use /api/v1/runs/{id}/start to begin execution.
    WriteJSON(w, http.StatusCreated, map[string]interface{}{
        "run_id": runID,
        "status": "created",
    })
}

// handleStartRun starts a run by id using the manager and optional options.
func (s *Server) handleStartRun(w http.ResponseWriter, r *http.Request) {
    id := r.PathValue("id")
    if id == "" {
        WriteError(w, http.StatusBadRequest, "missing run id")
        return
    }

    var body struct {
        IsStudy bool   `json:"is_study,omitempty"`
        Mode    string `json:"mode,omitempty"`
    }
    _ = json.NewDecoder(r.Body).Decode(&body)

    var opts []manager.StartOption
    if body.IsStudy { opts = append(opts, manager.WithIsStudy(true)) }
    if body.Mode != "" { opts = append(opts, manager.WithExecMode(body.Mode)) }

    if err := s.Mgr.Start(context.Background(), id, opts...); err != nil {
        if strings.Contains(err.Error(), "already active") {
            WriteError(w, http.StatusConflict, err.Error())
            return
        }
        WriteError(w, http.StatusInternalServerError, err.Error())
        return
    }

    WriteJSON(w, http.StatusCreated, map[string]interface{}{
        "run_id": id,
        "status": "starting",
    })
}

// handleGetRun returns the status for a run id (alias of loop status).
func (s *Server) handleGetRun(w http.ResponseWriter, r *http.Request) {
    id := r.PathValue("id")
    if id == "" {
        WriteError(w, http.StatusBadRequest, "missing run id")
        return
    }
    info, err := s.Mgr.Status(id)
    if err != nil {
        WriteError(w, http.StatusNotFound, err.Error())
        return
    }
    WriteJSON(w, http.StatusOK, map[string]interface{}{
        "run_id":     info.FWUID,
        "status":     info.Status,
        "phase":      info.Phase,
        "agent":      info.Agent,
        "iteration":  info.Iteration,
        "elapsed":    info.Elapsed,
        "lastActivity": info.LastActivity,
        "error":      info.Error,
    })
}

// handleGetRunSteps returns recent run.step audit events for a run id.
func (s *Server) handleGetRunSteps(w http.ResponseWriter, r *http.Request) {
    id := r.PathValue("id")
    if id == "" {
        WriteError(w, http.StatusBadRequest, "missing run id")
        return
    }
    if s.AuditLogger == nil {
        WriteError(w, http.StatusServiceUnavailable, "audit logger unavailable")
        return
    }
    // Query last N run.step entries; default limit 200
    limit := 200
    q := &audit.QueryFilter{EventTypes: []audit.EventType{audit.EventRunStep}, Limit: limit}
    res, err := s.AuditLogger.Query(r.Context(), q)
    if err != nil {
        WriteError(w, http.StatusInternalServerError, err.Error())
        return
    }
    // Filter by run_id in metadata
    steps := make([]map[string]interface{}, 0, len(res.Entries))
    for _, e := range res.Entries {
        var md map[string]interface{}
        _ = e.GetMetadata(&md)
        if md != nil && md["run_id"] == id {
            steps = append(steps, map[string]interface{}{
                "timestamp":  e.Timestamp,
                "event_type": e.EventType,
                "severity":   e.Severity,
                "metadata":   md,
            })
        }
    }
    WriteJSON(w, http.StatusOK, map[string]interface{}{"run_id": id, "steps": steps})
}

func (s *Server) handleStart(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		WriteError(w, http.StatusBadRequest, "missing fwu id")
		return
	}

	err := s.Mgr.Start(context.Background(), id)
	if err != nil {
		if strings.Contains(err.Error(), "already active") {
			WriteError(w, http.StatusConflict, err.Error())
			return
		}
		WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	WriteJSON(w, http.StatusCreated, map[string]string{
		"fwu_id": id,
		"status": "starting",
	})
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		WriteError(w, http.StatusBadRequest, "missing fwu id")
		return
	}

	info, err := s.Mgr.Status(id)
	if err != nil {
		WriteError(w, http.StatusNotFound, err.Error())
		return
	}

	WriteJSON(w, http.StatusOK, info)
}

func (s *Server) handleList(w http.ResponseWriter, r *http.Request) {
	list := s.Mgr.List()
	WriteJSON(w, http.StatusOK, list)
}

func (s *Server) handlePause(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		WriteError(w, http.StatusBadRequest, "missing fwu id")
		return
	}

	err := s.Mgr.Pause(id)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			WriteError(w, http.StatusNotFound, err.Error())
			return
		}
		WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	WriteJSON(w, http.StatusOK, map[string]string{"status": "pausing"})
}

func (s *Server) handleStop(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		WriteError(w, http.StatusBadRequest, "missing fwu id")
		return
	}

	err := s.Mgr.Stop(id)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			WriteError(w, http.StatusNotFound, err.Error())
			return
		}
		WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	WriteJSON(w, http.StatusOK, map[string]string{"status": "stopped"})
}

func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		WriteError(w, http.StatusBadRequest, "missing fwu id")
		return
	}

	sub, unsub, err := s.Mgr.Subscribe(id)
	if err != nil {
		if _, ok := err.(*manager.NotFoundError); ok {
			WriteError(w, http.StatusNotFound, err.Error())
			return
		}
		WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer unsub()

	flusher, ok := w.(http.Flusher)
	if !ok {
		WriteError(w, http.StatusInternalServerError, "streaming not supported")
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case event, ok := <-sub:
			if !ok {
				// Pipeline finished, channel closed.
				return
			}
			data, err := json.Marshal(event)
			if err != nil {
				continue
			}
			_, _ = fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()
		}
	}
}

func WriteJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func WriteError(w http.ResponseWriter, status int, msg string) {
	WriteJSON(w, status, map[string]string{"error": msg})
}

func WriteErrorWithSuggestion(w http.ResponseWriter, status int, msg string, suggestion string) {
	WriteJSON(w, status, map[string]string{
		"error":      msg,
		"suggestion": suggestion,
	})
}
