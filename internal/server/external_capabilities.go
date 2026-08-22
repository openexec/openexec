package server

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/openexec/openexec/internal/externalcap"
	"github.com/openexec/openexec/pkg/db/state"
)

const externalCapabilityBodyLimit = 64 << 10

func (s *Server) registerExternalCapabilityRoutes() {
	auth := s.externalCapabilityAuth
	s.Mux.Handle("GET /api/v1/external-connections", auth(http.HandlerFunc(s.listExternalConnections)))
	s.Mux.Handle("POST /api/v1/external-connections", auth(http.HandlerFunc(s.createExternalConnection)))
	s.Mux.Handle("POST /api/v1/external-connections/{id}/oauth/start", auth(http.HandlerFunc(s.startExternalConnectionOAuth)))
	s.Mux.Handle("POST /api/v1/external-connections/oauth/callback", auth(http.HandlerFunc(s.completeExternalConnectionOAuth)))
	s.Mux.Handle("POST /api/v1/external-connections/{id}/probe", auth(http.HandlerFunc(s.probeExternalConnection)))
	s.Mux.Handle("POST /api/v1/external-connections/{id}/disable", auth(http.HandlerFunc(s.disableExternalConnection)))
}

func (s *Server) externalCapabilityAuth(next http.Handler) http.Handler {
	return s.repositoryBearerAuth(s.externalCapabilityToken,
		"external capability control plane is not configured", "external capability bearer required", next)
}

func (s *Server) externalCapabilityService(w http.ResponseWriter) (*externalcap.Service, bool) {
	if s.ExternalCapabilities == nil {
		s.respondJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "external capability control plane is not configured"})
		return nil, false
	}
	return s.ExternalCapabilities, true
}

func (s *Server) listExternalConnections(w http.ResponseWriter, r *http.Request) {
	service, ok := s.externalCapabilityService(w)
	if !ok {
		return
	}
	projectRef := strings.TrimSpace(r.URL.Query().Get("project_ref"))
	if projectRef == "" {
		s.respondJSON(w, http.StatusBadRequest, map[string]string{"error": "project_ref is required"})
		return
	}
	connections, err := service.List(r.Context(), projectRef)
	if err != nil {
		s.respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if connections == nil {
		connections = []state.ExternalConnection{}
	}
	s.respondJSON(w, http.StatusOK, map[string]any{"connections": connections})
}

func (s *Server) createExternalConnection(w http.ResponseWriter, r *http.Request) {
	service, ok := s.externalCapabilityService(w)
	if !ok {
		return
	}
	var input externalcap.CreateInput
	if !s.decodeExternalCapabilityBody(w, r, &input) {
		return
	}
	connection, err := service.Create(r.Context(), input)
	if err != nil {
		s.respondJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	s.respondJSON(w, http.StatusCreated, connection)
}

func (s *Server) startExternalConnectionOAuth(w http.ResponseWriter, r *http.Request) {
	service, ok := s.externalCapabilityService(w)
	if !ok {
		return
	}
	var input struct {
		ProjectRef          string `json:"project_ref"`
		RedirectURL         string `json:"redirect_url"`
		LoopbackRedirectURL string `json:"loopback_redirect_url"`
		ClientMetadataURL   string `json:"client_metadata_url"`
	}
	if !s.decodeExternalCapabilityBody(w, r, &input) {
		return
	}
	started, err := service.StartOAuth(r.Context(), r.PathValue("id"), input.ProjectRef, input.RedirectURL, input.LoopbackRedirectURL, input.ClientMetadataURL)
	if err != nil {
		s.externalCapabilityError(w, err)
		return
	}
	s.respondJSON(w, http.StatusOK, started)
}

func (s *Server) completeExternalConnectionOAuth(w http.ResponseWriter, r *http.Request) {
	service, ok := s.externalCapabilityService(w)
	if !ok {
		return
	}
	var callback externalcap.OAuthCallback
	if !s.decodeExternalCapabilityBody(w, r, &callback) {
		return
	}
	connection, err := service.CompleteOAuth(r.Context(), callback)
	if err != nil {
		s.externalCapabilityError(w, err)
		return
	}
	s.respondJSON(w, http.StatusOK, connection)
}

func (s *Server) probeExternalConnection(w http.ResponseWriter, r *http.Request) {
	service, ok := s.externalCapabilityService(w)
	if !ok {
		return
	}
	var input struct {
		ProjectRef string `json:"project_ref"`
	}
	if !s.decodeExternalCapabilityBody(w, r, &input) {
		return
	}
	connection, err := service.Probe(r.Context(), r.PathValue("id"), input.ProjectRef)
	if err != nil {
		s.externalCapabilityError(w, err)
		return
	}
	s.respondJSON(w, http.StatusOK, connection)
}

func (s *Server) disableExternalConnection(w http.ResponseWriter, r *http.Request) {
	service, ok := s.externalCapabilityService(w)
	if !ok {
		return
	}
	var input struct {
		ProjectRef string `json:"project_ref"`
	}
	if !s.decodeExternalCapabilityBody(w, r, &input) {
		return
	}
	connection, err := service.Disable(r.Context(), r.PathValue("id"), input.ProjectRef)
	if err != nil {
		s.externalCapabilityError(w, err)
		return
	}
	s.respondJSON(w, http.StatusOK, connection)
}

func (s *Server) decodeExternalCapabilityBody(w http.ResponseWriter, r *http.Request, destination any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, externalCapabilityBodyLimit)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		status := http.StatusBadRequest
		if errors.As(err, new(*http.MaxBytesError)) {
			status = http.StatusRequestEntityTooLarge
		}
		s.respondJSON(w, status, map[string]string{"error": "invalid external capability request: " + err.Error()})
		return false
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		s.respondJSON(w, http.StatusBadRequest, map[string]string{"error": "external capability request must contain one JSON object"})
		return false
	}
	return true
}

func (s *Server) externalCapabilityError(w http.ResponseWriter, err error) {
	status := http.StatusBadRequest
	switch {
	case errors.Is(err, state.ErrExternalConnectionNotFound):
		status = http.StatusNotFound
	case errors.Is(err, externalcap.ErrProjectNotBound), errors.Is(err, externalcap.ErrToolNotAllowed):
		status = http.StatusForbidden
	case errors.Is(err, externalcap.ErrDisabled):
		status = http.StatusConflict
	case errors.Is(err, externalcap.ErrOAuthFlowNotFound):
		status = http.StatusGone
	}
	s.respondJSON(w, status, map[string]string{"error": err.Error()})
}
