package api

import (
    "context"
    "encoding/json"
    "fmt"
    "log"
    "net/http"
    "strconv"
    "strings"
    "time"

    "github.com/openexec/openexec/pkg/audit"
    "github.com/openexec/openexec/pkg/db/state"
    "github.com/openexec/openexec/pkg/manager"
)

// parseIntParam parses an integer query parameter with a default value.
func parseIntParam(r *http.Request, key string, defaultVal int) int {
    if v := r.URL.Query().Get(key); v != "" {
        if i, err := strconv.Atoi(v); err == nil && i >= 0 {
            return i
        }
    }
    return defaultVal
}

// Legacy loops endpoint removed; use /api/fwu/{id}/start or /api/v1/runs

// Legacy loops endpoint removed; use /api/fwu/{id}/status

// handlePlan executes the planning workflow on the server and returns a plan artifact.
func (s *Server) handlePlan(w http.ResponseWriter, r *http.Request) {
	var req manager.PlanRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	result, err := s.Mgr.Plan(r.Context(), req)
	if err != nil {
		// Return 400 for input validation errors (path traversal, denylist, not found)
		if _, ok := err.(*manager.PlanInputError); ok {
			WriteError(w, http.StatusBadRequest, err.Error())
			return
		}
		WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	WriteJSON(w, http.StatusOK, result)
}

// handleExecuteRuns triggers the orchestration of multiple tasks on the server.
func (s *Server) handleExecuteRuns(w http.ResponseWriter, r *http.Request) {
	var req manager.RunOptions
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	// Use background context for the long-running execution
	go func() {
		err := s.Mgr.ExecuteTasks(context.Background(), req)
		if err != nil {
			log.Printf("[Server] ExecuteTasks failed: %v", err)
		}
	}()

	WriteJSON(w, http.StatusAccepted, map[string]string{
		"status": "execution_started",
	})
}

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

// handleGetRun returns the status for a run id.
// When OPENEXEC_USE_UNIFIED_READS=1, falls back to state store if not found in memory.
func (s *Server) handleGetRun(w http.ResponseWriter, r *http.Request) {
    id := r.PathValue("id")
    if id == "" {
        WriteError(w, http.StatusBadRequest, "missing run id")
        return
    }

    // Try in-memory first (active runs)
    info, err := s.Mgr.Status(id)
    if err == nil {
        WriteJSON(w, http.StatusOK, map[string]interface{}{
            "run_id":       info.FWUID,
            "status":       info.Status,
            "phase":        info.Phase,
            "agent":        info.Agent,
            "iteration":    info.Iteration,
            "elapsed":      info.Elapsed,
            "lastActivity": info.LastActivity,
            "error":        info.Error,
        })
        return
    }

    // Fallback to unified DB if enabled
    if s.UseUnifiedReads && s.StateStore != nil {
        run, dbErr := s.StateStore.GetRun(r.Context(), id)
        if dbErr != nil {
            log.Printf("[API] GetRun DB error: %v", dbErr)
            WriteError(w, http.StatusInternalServerError, "database error")
            return
        }
        if run != nil {
            errMsg := ""
            if run.ErrorMessage.Valid {
                errMsg = run.ErrorMessage.String
            }
            WriteJSON(w, http.StatusOK, map[string]interface{}{
                "run_id":     run.ID,
                "status":     run.Status,
                "mode":       run.Mode,
                "created_at": run.CreatedAt,
                "updated_at": run.UpdatedAt,
                "error":      errMsg,
            })
            return
        }
    }

    // Friendlier error message when run not found in both memory and DB
    WriteError(w, http.StatusNotFound, fmt.Sprintf("run %q not found", id))
}

// handleGetRunSteps returns run steps for a run id.
// When OPENEXEC_USE_UNIFIED_READS=1, uses state store; otherwise falls back to audit logger.
// Query params: ?limit=100&offset=0
func (s *Server) handleGetRunSteps(w http.ResponseWriter, r *http.Request) {
    id := r.PathValue("id")
    if id == "" {
        WriteError(w, http.StatusBadRequest, "missing run id")
        return
    }

    limit := parseIntParam(r, "limit", 200)
    offset := parseIntParam(r, "offset", 0)

    // Use unified DB if enabled
    if s.UseUnifiedReads && s.StateStore != nil {
        steps, err := s.StateStore.ListRunSteps(r.Context(), id, limit, offset)
        if err != nil {
            log.Printf("[API] ListRunSteps DB error: %v", err)
            WriteError(w, http.StatusInternalServerError, "database error")
            return
        }
        result := make([]map[string]interface{}, 0, len(steps))
        for _, step := range steps {
            stepMap := map[string]interface{}{
                "id":         step.ID,
                "run_id":     step.RunID,
                "phase":      step.Phase,
                "iteration":  step.Iteration,
                "status":     step.Status,
                "started_at": step.StartedAt,
            }
            if step.TraceID.Valid {
                stepMap["trace_id"] = step.TraceID.String
            }
            if step.Agent.Valid {
                stepMap["agent"] = step.Agent.String
            }
            if step.CompletedAt.Valid {
                stepMap["completed_at"] = step.CompletedAt.String
            }
            if step.InputsHash.Valid {
                stepMap["inputs_hash"] = step.InputsHash.String
            }
            if step.OutputsHash.Valid {
                stepMap["outputs_hash"] = step.OutputsHash.String
            }
            result = append(result, stepMap)
        }
        WriteJSON(w, http.StatusOK, map[string]interface{}{"run_id": id, "steps": result})
        return
    }

    // Fallback to audit logger
    if s.AuditLogger == nil {
        WriteError(w, http.StatusServiceUnavailable, "audit logger unavailable")
        return
    }
    // Query run.step entries with limit from query params
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

// handleListRuns returns a list of runs, optionally filtered.
// When OPENEXEC_USE_UNIFIED_READS=1, uses state store; otherwise uses manager's in-memory list.
// Query params: ?project_path=...&status=...&limit=100&offset=0
func (s *Server) handleListRuns(w http.ResponseWriter, r *http.Request) {
    // Use unified DB if enabled
    if s.UseUnifiedReads && s.StateStore != nil {
        projectPath := r.URL.Query().Get("project_path")
        status := r.URL.Query().Get("status")
        limit := parseIntParam(r, "limit", 100)
        offset := parseIntParam(r, "offset", 0)

        runs, err := s.StateStore.ListRuns(r.Context(), state.RunFilter{
            ProjectPath: projectPath,
            Status:      status,
            Limit:       limit,
            Offset:      offset,
        })
        if err != nil {
            log.Printf("[API] ListRuns DB error: %v", err)
            WriteError(w, http.StatusInternalServerError, "database error")
            return
        }

        result := make([]map[string]interface{}, 0, len(runs))
        for _, run := range runs {
            runMap := map[string]interface{}{
                "run_id":       run.ID,
                "project_path": run.ProjectPath,
                "mode":         run.Mode,
                "status":       run.Status,
                "created_at":   run.CreatedAt,
                "updated_at":   run.UpdatedAt,
            }
            if run.SessionID.Valid {
                runMap["session_id"] = run.SessionID.String
            }
            if run.TaskID.Valid {
                runMap["task_id"] = run.TaskID.String
            }
            if run.ErrorMessage.Valid {
                runMap["error"] = run.ErrorMessage.String
            }
            result = append(result, runMap)
        }
        WriteJSON(w, http.StatusOK, map[string]interface{}{"runs": result})
        return
    }

    // Fallback to in-memory list
    list := s.Mgr.List()
    WriteJSON(w, http.StatusOK, map[string]interface{}{"runs": list})
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
