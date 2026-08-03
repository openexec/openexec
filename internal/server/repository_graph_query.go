package server

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/openexec/openexec/internal/knowledge"
	"github.com/openexec/openexec/internal/repository"
)

func (s *Server) registerRepositoryGraphQueryRoutes() {
	s.Mux.HandleFunc("GET /api/v1/repository-graph/symbols", s.handleGraphSymbols)
	s.Mux.HandleFunc("GET /api/v1/repository-graph/symbols/{id}", s.handleGraphSymbolDetail)
	s.Mux.HandleFunc("GET /api/v1/repository-graph/symbols/{id}/dependencies", s.handleGraphSymbolDependencies)
	s.Mux.HandleFunc("GET /api/v1/repository-graph/symbols/{id}/calls", s.handleGraphSymbolCalls)
	s.Mux.HandleFunc("GET /api/v1/repository-graph/symbols/{id}/impact", s.handleGraphSymbolImpact)
	s.Mux.HandleFunc("GET /api/v1/repository-graph/symbols/{id}/source", s.handleGraphSymbolSource)
}

func (s *Server) authorizedGraphStore(ctx context.Context, checkoutID string) (*knowledge.Store, knowledge.RepositoryIdentity, int, error) {
	store, err := s.repositoryGraphStore()
	if err != nil {
		return nil, knowledge.RepositoryIdentity{}, http.StatusInternalServerError, err
	}
	identity, err := store.EnsureRepositoryIdentity(ctx, s.ProjectsDir, "")
	if err != nil {
		return nil, knowledge.RepositoryIdentity{}, http.StatusInternalServerError, err
	}
	if strings.TrimSpace(checkoutID) == "" || checkoutID != identity.CheckoutID {
		return nil, identity, http.StatusForbidden, errors.New("repository graph access is not authorized for this checkout")
	}
	return store, identity, 0, nil
}

func (s *Server) repositoryGraphStore() (*knowledge.Store, error) {
	if s.KnowledgeStore != nil {
		return s.KnowledgeStore, nil
	}
	// Small handler fixtures construct Server directly. Production construction
	// always installs the process-scoped store above.
	return knowledge.NewStoreWithDB(s.StateStore.GetDB())
}

func (s *Server) respondGraphError(w http.ResponseWriter, err error) {
	status := http.StatusInternalServerError
	state := knowledge.RepositoryState{}
	var stale *knowledge.StaleGraphError
	if errors.As(err, &stale) {
		status = http.StatusConflict
		state = stale.State
	} else if errors.Is(err, sql.ErrNoRows) {
		status = http.StatusNotFound
	}
	s.respondJSON(w, status, map[string]any{"error": err.Error(), "generation": state, "freshness": state.Freshness})
}

func intQuery(r *http.Request, name string, fallback int) int {
	value, err := strconv.Atoi(r.URL.Query().Get(name))
	if err != nil || value <= 0 {
		return fallback
	}
	return value
}

func (s *Server) graphAccess(w http.ResponseWriter, r *http.Request) (*knowledge.Store, knowledge.RepositoryIdentity, bool) {
	store, identity, status, err := s.authorizedGraphStore(r.Context(), r.Header.Get("X-OpenExec-Checkout-ID"))
	if err != nil {
		s.respondJSON(w, status, map[string]string{"error": err.Error()})
		return nil, knowledge.RepositoryIdentity{}, false
	}
	return store, identity, true
}

func (s *Server) handleGraphSymbols(w http.ResponseWriter, r *http.Request) {
	store, identity, ok := s.graphAccess(w, r)
	if !ok {
		return
	}
	result, err := store.FindGraphSymbols(r.Context(), identity, r.URL.Query().Get("q"), r.URL.Query().Get("file"), r.URL.Query().Get("kind"), intQuery(r, "page", 1), intQuery(r, "page_size", 25))
	if err != nil {
		s.respondGraphError(w, err)
		return
	}
	s.respondJSON(w, http.StatusOK, result)
}
func (s *Server) handleGraphSymbolDetail(w http.ResponseWriter, r *http.Request) {
	store, identity, ok := s.graphAccess(w, r)
	if !ok {
		return
	}
	result, err := store.GraphSymbolDetail(r.Context(), identity, r.PathValue("id"))
	if err != nil {
		s.respondGraphError(w, err)
		return
	}
	s.respondJSON(w, http.StatusOK, result)
}
func (s *Server) handleGraphSymbolDependencies(w http.ResponseWriter, r *http.Request) {
	store, identity, ok := s.graphAccess(w, r)
	if !ok {
		return
	}
	incoming := r.URL.Query().Get("direction") == "incoming"
	result, err := store.FindSymbolRelationships(r.Context(), identity, r.PathValue("id"), incoming, intQuery(r, "depth", 1), []string{"calls", "references"}, knowledge.DefaultGraphLimits())
	if err != nil {
		s.respondGraphError(w, err)
		return
	}
	s.respondJSON(w, http.StatusOK, result)
}
func (s *Server) handleGraphSymbolCalls(w http.ResponseWriter, r *http.Request) {
	store, identity, ok := s.graphAccess(w, r)
	if !ok {
		return
	}
	direction := r.URL.Query().Get("direction")
	if direction != "outgoing" && direction != "incoming" {
		s.respondJSON(w, http.StatusBadRequest, map[string]string{"error": "direction must be outgoing or incoming"})
		return
	}
	result, err := store.FindSymbolRelationships(r.Context(), identity, r.PathValue("id"), direction == "incoming", intQuery(r, "depth", 1), []string{"calls"}, knowledge.DefaultGraphLimits())
	if err != nil {
		s.respondGraphError(w, err)
		return
	}
	s.respondJSON(w, http.StatusOK, result)
}
func (s *Server) handleGraphSymbolImpact(w http.ResponseWriter, r *http.Request) {
	store, identity, ok := s.graphAccess(w, r)
	if !ok {
		return
	}
	result, err := store.ImpactAnalysis(r.Context(), identity, []string{r.PathValue("id")}, intQuery(r, "depth", 2), knowledge.DefaultGraphLimits())
	if err != nil {
		s.respondGraphError(w, err)
		return
	}
	s.respondJSON(w, http.StatusOK, result)
}
func (s *Server) handleGraphSymbolSource(w http.ResponseWriter, r *http.Request) {
	store, identity, ok := s.graphAccess(w, r)
	if !ok {
		return
	}
	reader, err := repository.NewRootedReader(s.ProjectsDir, identity.RepositoryID, identity.WorktreeID, knowledge.DefaultGraphLimits().MaxBytes)
	if err != nil {
		s.respondGraphError(w, err)
		return
	}
	result, err := store.ReadGraphSymbol(r.Context(), identity, r.PathValue("id"), reader)
	if err != nil {
		s.respondGraphError(w, err)
		return
	}
	s.respondJSON(w, http.StatusOK, result)
}
