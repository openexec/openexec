package api

import (
	"net/http"

	"github.com/openexec/openexec/internal/agent"
)

func (s *Server) handleListProviders(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Initialize default registry if not already done
	agent.RegisterDefaultFactories(agent.DefaultRegistry)
	
	statuses := agent.DefaultRegistry.CheckAllAvailability()
	WriteJSON(w, http.StatusOK, statuses)
}

func (s *Server) handleListModels(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Initialize default registry if not already done
	agent.RegisterDefaultFactories(agent.DefaultRegistry)
	
	infos := agent.DefaultRegistry.GetAllModelInfo()
	WriteJSON(w, http.StatusOK, infos)
}
