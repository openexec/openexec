package api

import (
	"log"
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
		}
	}
	
	WriteJSON(w, http.StatusOK, statuses)
}

func (s *Server) handleListModels(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	log.Printf("[API] Fetching models. DefaultModelCatalog: %p\n", agent.DefaultModelCatalog)
	
	// Return all known models from the catalog
	models := agent.DefaultModelCatalog.GetAllModels()
	log.Printf("[API] Catalog GetAllModels returned %d models\n", len(models))
	
	if len(models) == 0 {
		// Try to use a fresh catalog if the default is somehow empty
		log.Printf("[API] Default catalog empty, creating fresh one...\n")
		freshCatalog := agent.NewModelCatalog()
		models = freshCatalog.GetAllModels()
		log.Printf("[API] Fresh catalog has %d models\n", len(models))
	}

	// Convert ExtendedModelInfo to base ModelInfo for the API
	infos := make([]agent.ModelInfo, 0, len(models))
	for _, m := range models {
		infos = append(infos, m.ModelInfo)
	}
	
	log.Printf("[API] Returning %d model infos\n", len(infos))
	WriteJSON(w, http.StatusOK, infos)
}
