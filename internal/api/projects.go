package api

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"

	"github.com/openexec/openexec/internal/gates"
)

// ProjectInfo represents metadata about an OpenExec project.
type ProjectInfo struct {
	Name string `json:"name"`
	Path string `json:"path"`
	Type string `json:"type"`
}

func (s *Server) handleListProjects(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	fmt.Printf("[API] Listing projects in: %s\n", s.projectsDir)

	if s.projectsDir == "" {
		writeJSON(w, http.StatusOK, []ProjectInfo{})
		return
	}

	absPath, _ := filepath.Abs(s.projectsDir)
	fmt.Printf("[API] Absolute projects path: %s\n", absPath)

	entries, err := os.ReadDir(s.projectsDir)
	if err != nil {
		fmt.Printf("[API] Error reading dir: %v\n", err)
		writeError(w, http.StatusInternalServerError, "failed to read projects directory: "+err.Error())
		return
	}

	projects := make([]ProjectInfo, 0)
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		projectPath := filepath.Join(s.projectsDir, entry.Name())
		
		// Check for openexec.yaml (or .openexec/openexec.yaml)
		cfg, err := gates.LoadConfig(projectPath)
		if err == nil {
			projects = append(projects, ProjectInfo{
				Name: cfg.Project.Name,
				Path: projectPath,
				Type: cfg.Project.Type,
			})
		}
	}

	fmt.Printf("[API] Found %d projects\n", len(projects))
	writeJSON(w, http.StatusOK, projects)
}
