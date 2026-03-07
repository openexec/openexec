package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/openexec/openexec/pkg/manager"
)

func (s *Server) handleCreateLoop(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Prompt        string `json:"prompt"`
		WorkDir       string `json:"work_dir"`
		MaxIterations int    `json:"max_iterations,omitempty"`
		TaskID        string `json:"task_id,omitempty"`
		MCPConfigPath string `json:"mcp_config_path,omitempty"`
		ReviewerModel string `json:"reviewer_model,omitempty"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.TaskID == "" {
		WriteError(w, http.StatusBadRequest, "missing task_id")
		return
	}

	// Start the pipeline/loop via manager
	err := s.Mgr.Start(r.Context(), req.TaskID)
	if err != nil {
		if strings.Contains(err.Error(), "already active") {
			WriteErrorWithSuggestion(w, http.StatusConflict, err.Error(), "Try stopping the existing execution with 'openexec stop' before running again.")
			return
		}
		WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	WriteJSON(w, http.StatusCreated, map[string]interface{}{
		"id":     req.TaskID,
		"status": "starting",
	})
}

func (s *Server) handleGetLoop(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		WriteError(w, http.StatusBadRequest, "missing loop id")
		return
	}

	info, err := s.Mgr.Status(id)
	if err != nil {
		WriteError(w, http.StatusNotFound, err.Error())
		return
	}

	WriteJSON(w, http.StatusOK, map[string]interface{}{
		"id":        info.FWUID,
		"status":    info.Status,
		"iteration": info.Iteration,
	})
}

func (s *Server) handleStart(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		WriteError(w, http.StatusBadRequest, "missing fwu id")
		return
	}

	err := s.Mgr.Start(r.Context(), id)
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
