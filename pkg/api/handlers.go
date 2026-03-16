package api

import (
    "context"
    "encoding/json"
    "fmt"
    "log"
    "net/http"
    "strconv"
    "strings"
    "sync"
    "time"

    "github.com/openexec/openexec/internal/approval"
    "github.com/openexec/openexec/pkg/audit"
    "github.com/openexec/openexec/pkg/db/session"
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

// Legacy loops and FWU endpoints removed in Phase Four. Use /api/v1/runs endpoints.

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
// When StateStore is available, creates a RunSpec for deterministic replay.
func (s *Server) handleCreateRun(w http.ResponseWriter, r *http.Request) {
    var req struct {
        WorkDir       string `json:"work_dir"`
        SessionID     string `json:"session_id,omitempty"`
        Intent        string `json:"intent,omitempty"`
        QuickfixTitle string `json:"quickfix_title,omitempty"`
        VerifyScript  string `json:"verify_script,omitempty"`
        Mode          string `json:"mode,omitempty"`
        Model         string `json:"model,omitempty"`
        ContextHash   string `json:"context_hash,omitempty"`
        PromptHash    string `json:"prompt_hash,omitempty"`
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

    // Create RunSpec for deterministic replay if StateStore is available
    var specID string
    if s.StateStore != nil && req.Intent != "" {
        spec := &state.RunSpec{
            SessionID:   req.SessionID,
            Intent:      req.Intent,
            ContextHash: req.ContextHash,
            PromptHash:  req.PromptHash,
            Model:       req.Model,
            Mode:        req.Mode,
        }
        if err := s.StateStore.CreateRunSpec(r.Context(), spec); err != nil {
            log.Printf("[API] CreateRunSpec failed: %v", err)
            // Continue without spec - non-fatal
        } else {
            specID = spec.ID
        }
    }

    // Persist the run request to audit for replay determinism
    if s.AuditLogger != nil {
        builder, err := audit.NewEntry(audit.EventRunCreated, "openexec", "system")
        if err == nil {
            md := map[string]interface{}{
                "event":          "run.created",
                "run_id":         runID,
                "work_dir":       req.WorkDir,
                "quickfix_title": req.QuickfixTitle,
                "verify_script":  req.VerifyScript,
                "mode":           req.Mode,
            }
            if specID != "" {
                md["spec_id"] = specID
            }
            e, _ := builder.WithProject(s.ProjectsDir).WithMetadata(md).Build()
            _ = s.AuditLogger.Log(r.Context(), e)
        }
    }

    // Return the run_id; clients can use /api/v1/runs/{id}/start to begin execution.
    resp := map[string]interface{}{
        "run_id": runID,
        "status": "created",
    }
    if specID != "" {
        resp["spec_id"] = specID
    }
    WriteJSON(w, http.StatusCreated, resp)
}

// handleStartRun starts a run by id using the manager and optional options.
func (s *Server) handleStartRun(w http.ResponseWriter, r *http.Request) {
    id := r.PathValue("id")
    if id == "" {
        WriteError(w, http.StatusBadRequest, "missing run id")
        return
    }

    var body struct {
        IsStudy         bool   `json:"is_study,omitempty"`
        Mode            string `json:"mode,omitempty"`
        BlueprintID     string `json:"blueprint_id,omitempty"`
        TaskDescription string `json:"task_description,omitempty"`
    }
    _ = json.NewDecoder(r.Body).Decode(&body)

    var opts []manager.StartOption
    if body.IsStudy { opts = append(opts, manager.WithIsStudy(true)) }
    if body.Mode != "" { opts = append(opts, manager.WithExecMode(body.Mode)) }
    if body.BlueprintID != "" { opts = append(opts, manager.WithBlueprint(body.BlueprintID)) }
    if body.TaskDescription != "" { opts = append(opts, manager.WithTaskDescription(body.TaskDescription)) }

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

// handleResumeRun resumes a run from a checkpoint.
func (s *Server) handleResumeRun(w http.ResponseWriter, r *http.Request) {
    id := r.PathValue("id")
    if id == "" {
        WriteError(w, http.StatusBadRequest, "missing run id")
        return
    }

    var body struct {
        CheckpointID string `json:"checkpoint_id,omitempty"`
    }
    _ = json.NewDecoder(r.Body).Decode(&body)

    // Get checkpoint (latest if not specified)
    var checkpoint *state.CheckpointData
    var err error
    if s.StateStore == nil {
        WriteError(w, http.StatusServiceUnavailable, "state store not available")
        return
    }

    if body.CheckpointID != "" {
        checkpoint, err = s.StateStore.GetCheckpointByID(r.Context(), body.CheckpointID)
    } else {
        checkpoint, err = s.StateStore.GetLatestCheckpoint(r.Context(), id)
    }
    if err != nil {
        WriteError(w, http.StatusInternalServerError, fmt.Sprintf("failed to get checkpoint: %v", err))
        return
    }
    if checkpoint == nil {
        WriteError(w, http.StatusNotFound, fmt.Sprintf("no checkpoint found for run %s", id))
        return
    }

    // Get list of applied tool calls for idempotency
    appliedCalls, err := s.StateStore.ListAppliedToolCalls(r.Context(), id)
    if err != nil {
        log.Printf("[API] ListAppliedToolCalls error: %v", err)
        // Non-fatal, continue with empty list
        appliedCalls = []string{}
    }

    // Start with resume options
    opts := []manager.StartOption{
        manager.WithResumeCheckpoint(checkpoint, appliedCalls),
    }

    if err := s.Mgr.Start(context.Background(), id, opts...); err != nil {
        if strings.Contains(err.Error(), "already active") {
            WriteError(w, http.StatusConflict, err.Error())
            return
        }
        WriteError(w, http.StatusInternalServerError, err.Error())
        return
    }

    WriteJSON(w, http.StatusOK, map[string]interface{}{
        "run_id":        id,
        "checkpoint_id": checkpoint.ID,
        "phase":         checkpoint.Phase,
        "iteration":     checkpoint.Iteration,
        "status":        "resuming",
    })
}

// handleGetRunCheckpoints returns checkpoints for a run.
func (s *Server) handleGetRunCheckpoints(w http.ResponseWriter, r *http.Request) {
    id := r.PathValue("id")
    if id == "" {
        WriteError(w, http.StatusBadRequest, "missing run id")
        return
    }

    if s.StateStore == nil {
        WriteError(w, http.StatusServiceUnavailable, "state store not available")
        return
    }

    checkpoints, err := s.StateStore.ListCheckpoints(r.Context(), id)
    if err != nil {
        WriteError(w, http.StatusInternalServerError, fmt.Sprintf("failed to list checkpoints: %v", err))
        return
    }

    result := make([]map[string]interface{}, 0, len(checkpoints))
    for _, cp := range checkpoints {
        result = append(result, map[string]interface{}{
            "id":        cp.ID,
            "run_id":    cp.RunID,
            "phase":     cp.Phase,
            "iteration": cp.Iteration,
            "timestamp": cp.Timestamp,
            "artifacts": cp.Artifacts,
        })
    }

    WriteJSON(w, http.StatusOK, map[string]interface{}{
        "run_id":      id,
        "checkpoints": result,
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
            "stage":        info.Stage,
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
    // Always include in-memory pipelines for real-time visibility.
    // DB runs may lag behind due to async writes; in-memory state is authoritative.
    inMemory := s.Mgr.List()

    // Also query DB for completed/historical runs not in memory
    var dbRuns []map[string]interface{}
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
        } else {
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
                dbRuns = append(dbRuns, runMap)
            }
        }
    }

    // Merge: in-memory pipelines take priority, add DB runs not already present
    inMemoryIDs := make(map[string]bool)
    result := make([]interface{}, 0, len(inMemory)+len(dbRuns))
    for _, p := range inMemory {
        inMemoryIDs[p.FWUID] = true
        result = append(result, p)
    }
    for _, r := range dbRuns {
        if id, ok := r["run_id"].(string); ok && !inMemoryIDs[id] {
            result = append(result, r)
        }
    }

    WriteJSON(w, http.StatusOK, map[string]interface{}{"runs": result})
}

// Legacy FWU handlers (handleStart, handleStatus, handleList, handlePause, handleStop, handleEvents)
// REMOVED in Phase Four. Use /api/v1/runs endpoints for all orchestration.

// handleStartParallelRuns starts multiple runs in parallel using git worktrees.
func (s *Server) handleStartParallelRuns(w http.ResponseWriter, r *http.Request) {
    var req struct {
        RunIDs []string `json:"run_ids"`
        Mode   string   `json:"mode,omitempty"`
    }
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        WriteError(w, http.StatusBadRequest, "invalid request body")
        return
    }

    if len(req.RunIDs) == 0 {
        WriteError(w, http.StatusBadRequest, "at least one run_id is required")
        return
    }

    if len(req.RunIDs) > 10 {
        WriteError(w, http.StatusBadRequest, "maximum 10 parallel runs allowed")
        return
    }

    // Start runs in parallel
    results := make([]map[string]interface{}, 0, len(req.RunIDs))
    var mu sync.Mutex
    var wg sync.WaitGroup

    for _, runID := range req.RunIDs {
        wg.Add(1)
        go func(id string) {
            defer wg.Done()

            var opts []manager.StartOption
            if req.Mode != "" {
                opts = append(opts, manager.WithExecMode(req.Mode))
            }

            err := s.Mgr.Start(context.Background(), id, opts...)

            mu.Lock()
            defer mu.Unlock()

            result := map[string]interface{}{
                "run_id": id,
            }
            if err != nil {
                result["status"] = "error"
                result["error"] = err.Error()
            } else {
                result["status"] = "starting"
            }
            results = append(results, result)
        }(runID)
    }

    wg.Wait()

    WriteJSON(w, http.StatusAccepted, map[string]interface{}{
        "runs":  results,
        "count": len(req.RunIDs),
    })
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

// handleStartBlueprintRun starts a blueprint-based run.
// POST /api/v1/runs:blueprint
func (s *Server) handleStartBlueprintRun(w http.ResponseWriter, r *http.Request) {
	var req struct {
		BlueprintID     string `json:"blueprint_id"`
		TaskDescription string `json:"task_description"`
		Mode            string `json:"mode,omitempty"`
		SessionID       string `json:"session_id,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	// Validate required fields
	if req.TaskDescription == "" {
		WriteError(w, http.StatusBadRequest, "task_description is required")
		return
	}

	// Use default blueprint if not specified
	blueprintID := req.BlueprintID
	if blueprintID == "" {
		blueprintID = "standard_task"
	}

	// Generate run ID
	now := time.Now().UTC().Format("20060102-150405")
	runID := fmt.Sprintf("BP-%s-%s", blueprintID[:min(8, len(blueprintID))], now)

	// Build start options
	opts := []manager.StartOption{
		manager.WithBlueprint(blueprintID),
		manager.WithTaskDescription(req.TaskDescription),
	}
	if req.Mode != "" {
		opts = append(opts, manager.WithExecMode(req.Mode))
	}

	// Log blueprint run creation to audit
	if s.AuditLogger != nil {
		builder, err := audit.NewEntry(audit.EventRunCreated, "openexec", "system")
		if err == nil {
			md := map[string]interface{}{
				"event":            "run.blueprint_created",
				"run_id":           runID,
				"blueprint_id":     blueprintID,
				"task_description": req.TaskDescription,
				"mode":             req.Mode,
				"session_id":       req.SessionID,
			}
			e, _ := builder.WithProject(s.ProjectsDir).WithMetadata(md).Build()
			_ = s.AuditLogger.Log(r.Context(), e)
		}
	}

	// Start the run asynchronously
	if err := s.Mgr.Start(context.Background(), runID, opts...); err != nil {
		if strings.Contains(err.Error(), "already active") {
			WriteError(w, http.StatusConflict, err.Error())
			return
		}
		WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	WriteJSON(w, http.StatusCreated, map[string]interface{}{
		"run_id":           runID,
		"blueprint_id":     blueprintID,
		"task_description": req.TaskDescription,
		"status":           "starting",
	})
}

// handleGetRunTimeline returns the timeline for a blueprint run.
// GET /api/v1/runs/{id}/timeline
func (s *Server) handleGetRunTimeline(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		WriteError(w, http.StatusBadRequest, "missing run id")
		return
	}

	// Get run status from manager
	info, err := s.Mgr.Status(id)
	if err != nil {
		// Try unified DB
		if s.UseUnifiedReads && s.StateStore != nil {
			run, dbErr := s.StateStore.GetRun(r.Context(), id)
			if dbErr != nil || run == nil {
				WriteError(w, http.StatusNotFound, fmt.Sprintf("run %q not found", id))
				return
			}
			// Return basic timeline for completed runs
			WriteJSON(w, http.StatusOK, map[string]interface{}{
				"run_id": id,
				"status": run.Status,
				"stages": []map[string]interface{}{},
			})
			return
		}
		WriteError(w, http.StatusNotFound, fmt.Sprintf("run %q not found", id))
		return
	}

	// Build timeline from run steps
	timeline := map[string]interface{}{
		"run_id":          id,
		"status":          info.Status,
		"stage":           info.Stage,
		"iteration":       info.Iteration,
		"elapsed":         info.Elapsed,
		"stages":          []map[string]interface{}{},
		"checkpoints":     []string{},
		"can_resume_from": []string{},
	}

	// Get stage history from run steps
	if s.StateStore != nil {
		steps, err := s.StateStore.ListRunSteps(r.Context(), id, 100, 0)
		if err == nil {
			stageMap := make(map[string]map[string]interface{})
			for _, step := range steps {
				stageName := step.Phase
				if stageName == "" {
					continue
				}

				// Initialize or update stage entry
				if _, exists := stageMap[stageName]; !exists {
					stageMap[stageName] = map[string]interface{}{
						"name":       stageName,
						"status":     step.Status,
						"started_at": step.StartedAt,
						"attempt":    1,
					}
				}

				// Update with latest status
				existing := stageMap[stageName]
				existing["status"] = step.Status
				if step.CompletedAt.Valid {
					existing["completed_at"] = step.CompletedAt.String
				}
				if step.Agent.Valid {
					existing["agent"] = step.Agent.String
				}
			}

			// Convert map to slice
			stages := make([]map[string]interface{}, 0, len(stageMap))
			for _, stage := range stageMap {
				stages = append(stages, stage)
			}
			timeline["stages"] = stages
		}

		// Get checkpoints
		checkpoints, err := s.StateStore.ListCheckpoints(r.Context(), id)
		if err == nil {
			cpNames := make([]string, 0, len(checkpoints))
			resumeFrom := make([]string, 0, len(checkpoints))
			for _, cp := range checkpoints {
				phase := cp.Phase
				if phase == "" {
					// Try to get stage name from artifacts
					if sn, ok := cp.Artifacts["stage_name"]; ok {
						phase = sn
					}
				}
				if phase != "" {
					cpNames = append(cpNames, fmt.Sprintf("stage:%s", phase))
					resumeFrom = append(resumeFrom, fmt.Sprintf("stage:%s", phase))
				}
			}
			timeline["checkpoints"] = cpNames
			timeline["can_resume_from"] = resumeFrom
		}
	}

	WriteJSON(w, http.StatusOK, timeline)
}

// min returns the smaller of two integers.
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// handleStartRunFromSession converts a chat session into a blueprint-driven run.
// POST /api/v1/sessions/{id}/run
func (s *Server) handleStartRunFromSession(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		WriteError(w, http.StatusBadRequest, "session id required")
		return
	}

	var req struct {
		BlueprintID     string `json:"blueprint_id"`
		Mode            string `json:"mode"`
		TaskDescription string `json:"task_description"`
		UseSummary      bool   `json:"use_summary"`
		Messages        int    `json:"messages"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil && err.Error() != "EOF" {
		WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	// Validate session exists
	if s.SessionRepo == nil {
		WriteError(w, http.StatusServiceUnavailable, "session repository not available")
		return
	}

	sess, err := s.SessionRepo.GetSession(r.Context(), id)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			WriteError(w, http.StatusNotFound, fmt.Sprintf("session %q not found", id))
			return
		}
		WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Derive task description from session if not provided
	taskDescription := req.TaskDescription
	if taskDescription == "" {
		taskDescription = deriveTaskFromSession(r.Context(), s.SessionRepo, sess.ID, req.Messages)
	}

	if taskDescription == "" {
		WriteError(w, http.StatusBadRequest, "could not derive task description from session; provide task_description")
		return
	}

	// Default blueprint
	blueprintID := req.BlueprintID
	if blueprintID == "" {
		blueprintID = "standard_task"
	}

	// Validate blueprint ID (known blueprints)
	switch blueprintID {
	case "standard_task", "quick_fix":
		// valid
	default:
		WriteError(w, http.StatusBadRequest, fmt.Sprintf("unknown blueprint_id: %s (valid: standard_task, quick_fix)", blueprintID))
		return
	}

	// Default mode
	mode := req.Mode
	if mode == "" {
		mode = "workspace-write"
	}

	// Validate mode
	switch mode {
	case "read-only", "workspace-write", "danger-full-access":
		// valid
	default:
		WriteError(w, http.StatusBadRequest, fmt.Sprintf("invalid mode: %s (valid: read-only, workspace-write, danger-full-access)", mode))
		return
	}

	// Generate run ID
	now := time.Now().UTC().Format("20060102-150405")
	bpPrefix := blueprintID
	if len(bpPrefix) > 8 {
		bpPrefix = bpPrefix[:8]
	}
	runID := fmt.Sprintf("BP-%s-%s", bpPrefix, now)

	// Build start options
	opts := []manager.StartOption{
		manager.WithBlueprint(blueprintID),
		manager.WithTaskDescription(taskDescription),
		manager.WithExecMode(mode),
	}

	// Check manager is available
	if s.Mgr == nil {
		WriteError(w, http.StatusServiceUnavailable, "manager not available")
		return
	}

	// Log run creation to audit
	if s.AuditLogger != nil {
		builder, err := audit.NewEntry(audit.EventRunCreated, "openexec", "system")
		if err == nil {
			md := map[string]interface{}{
				"event":            "run.session_to_blueprint",
				"run_id":           runID,
				"session_id":       id,
				"blueprint_id":     blueprintID,
				"task_description": truncateString(taskDescription, 500),
				"mode":             mode,
			}
			e, _ := builder.WithProject(s.ProjectsDir).WithMetadata(md).Build()
			_ = s.AuditLogger.Log(r.Context(), e)
		}
	}

	// Start the run
	if err := s.Mgr.Start(context.Background(), runID, opts...); err != nil {
		if strings.Contains(err.Error(), "already active") {
			WriteError(w, http.StatusConflict, err.Error())
			return
		}
		WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	WriteJSON(w, http.StatusCreated, map[string]interface{}{
		"run_id":       runID,
		"blueprint_id": blueprintID,
		"session_id":   id,
		"status":       "starting",
	})
}

// deriveTaskFromSession extracts a task description from session messages.
// If messagesCount > 0, considers that many recent messages for context.
func deriveTaskFromSession(ctx context.Context, repo interface {
	GetFullConversationHistory(ctx context.Context, sessionID string) ([]*session.Message, error)
}, sessionID string, messagesCount int) string {
	messages, err := repo.GetFullConversationHistory(ctx, sessionID)
	if err != nil || len(messages) == 0 {
		return ""
	}

	// Find the last user message
	var lastUserMsg *session.Message
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == session.RoleUser {
			lastUserMsg = messages[i]
			break
		}
	}

	if lastUserMsg == nil {
		return ""
	}

	// If only one message requested or found, return it directly
	if messagesCount <= 1 {
		return lastUserMsg.Content
	}

	// Build context from recent N messages
	startIdx := len(messages) - messagesCount
	if startIdx < 0 {
		startIdx = 0
	}

	var contextParts []string
	for i := startIdx; i < len(messages); i++ {
		msg := messages[i]
		if i == len(messages)-1 && msg.Role == session.RoleUser {
			continue // Skip last user msg, we'll add it as Task
		}
		role := "User"
		if msg.Role == session.RoleAssistant {
			role = "Assistant"
		}
		// Truncate long messages in context
		content := truncateString(msg.Content, 200)
		contextParts = append(contextParts, fmt.Sprintf("%s: %s", role, content))
	}

	if len(contextParts) > 0 {
		return fmt.Sprintf("Context:\n%s\n\nTask: %s",
			strings.Join(contextParts, "\n"),
			lastUserMsg.Content)
	}

	return lastUserMsg.Content
}

// truncateString truncates a string to maxLen characters, adding "..." if truncated.
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	if maxLen <= 3 {
		return s[:maxLen]
	}
	return s[:maxLen-3] + "..."
}

// handleListApprovals returns pending approval requests.
// GET /api/v1/approvals?run_id=optional&status=pending
func (s *Server) handleListApprovals(w http.ResponseWriter, r *http.Request) {
	if s.ApprovalGate == nil {
		WriteError(w, http.StatusServiceUnavailable, "approval gate not configured")
		return
	}

	// Get optional filters from query params
	runIDFilter := r.URL.Query().Get("run_id")
	statusFilter := r.URL.Query().Get("status")

	// Get approvals from gate using the interface methods
	var pending []*approval.GateRequest
	if runIDFilter != "" {
		pending = s.ApprovalGate.GetPendingApprovals(runIDFilter)
	} else {
		pending = s.ApprovalGate.GetAllPendingApprovals()
	}

	// Convert to response format with filtering
	var approvals []*approvalResponse
	for _, req := range pending {
		// Apply status filter (pending is the only status for pending approvals)
		if statusFilter != "" && statusFilter != "pending" && string(req.Status) != statusFilter {
			continue
		}
		approvals = append(approvals, gateRequestToResponse(req))
	}

	// Return empty array instead of null
	if approvals == nil {
		approvals = []*approvalResponse{}
	}

	WriteJSON(w, http.StatusOK, map[string]interface{}{
		"approvals": approvals,
	})
}

// handleGetApproval returns a single approval request by ID.
// GET /api/v1/approvals/{id}
func (s *Server) handleGetApproval(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		WriteError(w, http.StatusBadRequest, "approval id required")
		return
	}

	if s.ApprovalGate == nil {
		WriteError(w, http.StatusServiceUnavailable, "approval gate not configured")
		return
	}

	// Try to get the request from the gate using InMemoryGate's GetRequest method
	gate, ok := s.ApprovalGate.(*approval.InMemoryGate)
	if !ok {
		WriteError(w, http.StatusNotImplemented, "approval gate does not support GetRequest")
		return
	}

	req, found := gate.GetRequest(id)
	if !found {
		WriteError(w, http.StatusNotFound, fmt.Sprintf("approval request %q not found", id))
		return
	}

	WriteJSON(w, http.StatusOK, gateRequestToResponse(req))
}

// handleApproveRequest approves a pending approval request.
// POST /api/v1/approvals/{id}/approve
func (s *Server) handleApproveRequest(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		WriteError(w, http.StatusBadRequest, "approval id required")
		return
	}

	if s.ApprovalGate == nil {
		WriteError(w, http.StatusServiceUnavailable, "approval gate not configured")
		return
	}

	var body struct {
		DecidedBy string `json:"decided_by"`
		Reason    string `json:"reason"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)

	// Default to "api" if not specified
	decidedBy := body.DecidedBy
	if decidedBy == "" {
		decidedBy = "api"
	}

	// Approve the request
	err := s.ApprovalGate.Approve(id, decidedBy)
	if err != nil {
		if err == approval.ErrApprovalRequestNotFound {
			WriteError(w, http.StatusNotFound, fmt.Sprintf("approval request %q not found", id))
			return
		}
		if err == approval.ErrRequestAlreadyResolved {
			WriteError(w, http.StatusConflict, fmt.Sprintf("approval request %q already resolved", id))
			return
		}
		WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Return the updated request if possible
	if gate, ok := s.ApprovalGate.(*approval.InMemoryGate); ok {
		if req, found := gate.GetRequest(id); found {
			WriteJSON(w, http.StatusOK, gateRequestToResponse(req))
			return
		}
	}

	WriteJSON(w, http.StatusOK, map[string]interface{}{
		"id":      id,
		"status":  "approved",
		"message": "approval request approved",
	})
}

// handleRejectRequest rejects a pending approval request.
// POST /api/v1/approvals/{id}/reject
func (s *Server) handleRejectRequest(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		WriteError(w, http.StatusBadRequest, "approval id required")
		return
	}

	if s.ApprovalGate == nil {
		WriteError(w, http.StatusServiceUnavailable, "approval gate not configured")
		return
	}

	var body struct {
		DecidedBy string `json:"decided_by"`
		Reason    string `json:"reason"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)

	// Default to "api" if not specified
	decidedBy := body.DecidedBy
	if decidedBy == "" {
		decidedBy = "api"
	}

	reason := body.Reason
	if reason == "" {
		reason = "Rejected via API"
	}

	// Reject the request
	err := s.ApprovalGate.Reject(id, decidedBy, reason)
	if err != nil {
		if err == approval.ErrApprovalRequestNotFound {
			WriteError(w, http.StatusNotFound, fmt.Sprintf("approval request %q not found", id))
			return
		}
		if err == approval.ErrRequestAlreadyResolved {
			WriteError(w, http.StatusConflict, fmt.Sprintf("approval request %q already resolved", id))
			return
		}
		WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Return the updated request if possible
	if gate, ok := s.ApprovalGate.(*approval.InMemoryGate); ok {
		if req, found := gate.GetRequest(id); found {
			WriteJSON(w, http.StatusOK, gateRequestToResponse(req))
			return
		}
	}

	WriteJSON(w, http.StatusOK, map[string]interface{}{
		"id":      id,
		"status":  "rejected",
		"reason":  reason,
		"message": "approval request rejected",
	})
}

// approvalResponse is the JSON response format for approval requests.
type approvalResponse struct {
	ID           string         `json:"id"`
	RunID        string         `json:"run_id"`
	ToolName     string         `json:"tool_name"`
	ToolArgs     map[string]any `json:"tool_args,omitempty"`
	Description  string         `json:"description"`
	RiskLevel    string         `json:"risk_level"`
	Status       string         `json:"status"`
	CreatedAt    string         `json:"created_at,omitempty"`
	ResolvedAt   string         `json:"resolved_at,omitempty"`
	ResolvedBy   string         `json:"resolved_by,omitempty"`
	RejectReason string         `json:"reject_reason,omitempty"`
}

// gateRequestToResponse converts an approval.GateRequest to an approvalResponse.
func gateRequestToResponse(req *approval.GateRequest) *approvalResponse {
	if req == nil {
		return nil
	}

	resp := &approvalResponse{
		ID:           req.ID,
		RunID:        req.RunID,
		ToolName:     req.ToolName,
		ToolArgs:     req.ToolArgs,
		Description:  req.Description,
		RiskLevel:    req.RiskLevel,
		Status:       string(req.Status),
		ResolvedBy:   req.ResolvedBy,
		RejectReason: req.RejectReason,
	}

	if !req.CreatedAt.IsZero() {
		resp.CreatedAt = req.CreatedAt.Format(time.RFC3339)
	}
	if req.ResolvedAt != nil && !req.ResolvedAt.IsZero() {
		resp.ResolvedAt = req.ResolvedAt.Format(time.RFC3339)
	}

	return resp
}
