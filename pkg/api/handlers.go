package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/openexec/openexec/pkg/manager"
)

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
