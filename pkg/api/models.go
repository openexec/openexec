package api

import (
	"net/http"

	"github.com/openexec/openexec/pkg/agent"
)

func (s *Server) handleListProviders(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Initialize default registry
	agent.RegisterDefaultFactories(agent.DefaultRegistry)
	
	statuses := agent.DefaultRegistry.CheckAllAvailability()
	
	// If empty, return hardcoded defaults to ensure UI always has options
	if len(statuses) == 0 {
		statuses = []agent.ProviderStatus{
			{Name: agent.ProviderAnthropic, Available: false, Reason: "Not initialized"},
			{Name: agent.ProviderOpenAI, Available: false, Reason: "Not initialized"},
			{Name: agent.ProviderGemini, Available: false, Reason: "Not initialized"},
			{Name: "opencode", Available: false, Reason: "Not initialized"},
		}
	}
	
	WriteJSON(w, http.StatusOK, statuses)
}

func (s *Server) handleListModels(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Use GetEnabledModels instead of GetAllModels
	models := agent.DefaultModelCatalog.GetEnabledModels()
	
	// If no models enabled, fallback to all non-deprecated just in case
	if len(models) == 0 {
		models = agent.DefaultModelCatalog.GetNonDeprecatedModels()
	}

	// Convert ExtendedModelInfo to base ModelInfo for the API
	infos := make([]agent.ModelInfo, 0, len(models))
	for _, m := range models {
		infos = append(infos, m.ModelInfo)
	}
	
	WriteJSON(w, http.StatusOK, infos)
}
