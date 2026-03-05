package server

import (
	"net/http"

	"github.com/openexec/openexec/internal/knowledge"
)

// handleKnowledgeSymbols returns all indexed surgical pointers
func (s *Server) handleKnowledgeSymbols(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	store, err := knowledge.NewStore(s.projectsDir)
	if err != nil {
		http.Error(w, "Failed to open knowledge store", http.StatusInternalServerError)
		return
	}
	defer store.Close()

	symbols, err := store.ListSymbols()
	if err != nil {
		http.Error(w, "Failed to list symbols", http.StatusInternalServerError)
		return
	}

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"symbols": symbols,
	})
}

// handleKnowledgeEnvs returns all environment topologies
func (s *Server) handleKnowledgeEnvs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	store, err := knowledge.NewStore(s.projectsDir)
	if err != nil {
		http.Error(w, "Failed to open knowledge store", http.StatusInternalServerError)
		return
	}
	defer store.Close()

	envs, err := store.ListEnvironments()
	if err != nil {
		http.Error(w, "Failed to list environments", http.StatusInternalServerError)
		return
	}

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"environments": envs,
	})
}

// handleKnowledgePolicies returns all hard policy gates
func (s *Server) handleKnowledgePolicies(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	store, err := knowledge.NewStore(s.projectsDir)
	if err != nil {
		http.Error(w, "Failed to open knowledge store", http.StatusInternalServerError)
		return
	}
	defer store.Close()

	policies, err := store.ListPolicies()
	if err != nil {
		http.Error(w, "Failed to list policies", http.StatusInternalServerError)
		return
	}

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"policies": policies,
	})
}
