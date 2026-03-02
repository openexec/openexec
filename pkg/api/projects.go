package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/openexec/openexec/internal/gates"
	"github.com/openexec/openexec/internal/project"
)

// ProjectInfo represents metadata about an OpenExec project.
type ProjectInfo struct {
	Name string `json:"name"`
	Path string `json:"path"`
	Type string `json:"type"`
}

type InitProjectRequest struct {
	Name        string `json:"name"`
	Path        string `json:"path"`
	TractStore  string `json:"tractStore"`
	EngramStore string `json:"engramStore"`
}

type WizardRequest struct {
	ProjectPath string `json:"projectPath"`
	Message     string `json:"message"`
	State       string `json:"state"` // JSON string
	Model       string `json:"model"`
	Render      bool   `json:"render"`
}

func (s *Server) handleListProjects(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	fmt.Printf("[API] Listing projects in: %s\n", s.ProjectsDir)

	if s.ProjectsDir == "" {
		WriteJSON(w, http.StatusOK, []ProjectInfo{})
		return
	}

	entries, err := os.ReadDir(s.ProjectsDir)
	if err != nil {
		fmt.Printf("[API] Error reading dir: %v\n", err)
		WriteError(w, http.StatusInternalServerError, "failed to read projects directory: "+err.Error())
		return
	}

	projects := make([]ProjectInfo, 0)
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		projectPath := filepath.Join(s.ProjectsDir, entry.Name())
		
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
	WriteJSON(w, http.StatusOK, projects)
}

func (s *Server) handleInitProject(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req InitProjectRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Path == "" {
		WriteError(w, http.StatusBadRequest, "project path required")
		return
	}

	// Ensure absolute path
	absPath := req.Path
	if !filepath.IsAbs(absPath) && s.ProjectsDir != "" {
		absPath = filepath.Join(s.ProjectsDir, req.Path)
	}

	// Check if directory exists
	if _, err := os.Stat(absPath); os.IsNotExist(err) {
		if err := os.MkdirAll(absPath, 0755); err != nil {
			WriteError(w, http.StatusInternalServerError, "failed to create directory: "+err.Error())
			return
		}
	}

	// Change working directory temporarily to init project
	oldWd, _ := os.Getwd()
	if err := os.Chdir(absPath); err != nil {
		WriteError(w, http.StatusInternalServerError, "failed to access project directory: "+err.Error())
		return
	}
	defer os.Chdir(oldWd)

	// Initialize the project
	cfg, err := project.Initialize(req.Name)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "failed to initialize project: "+err.Error())
		return
	}

	WriteJSON(w, http.StatusCreated, cfg)
}

func (s *Server) handleWizard(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req WizardRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.ProjectPath == "" {
		WriteError(w, http.StatusBadRequest, "project path required")
		return
	}

	// Ensure absolute path
	absPath := req.ProjectPath
	if !filepath.IsAbs(absPath) && s.ProjectsDir != "" {
		absPath = filepath.Join(s.ProjectsDir, req.ProjectPath)
	}

	// Use binary from PATH or common location
	cmdArgs := []string{"wizard"}
	if req.Render {
		cmdArgs = append(cmdArgs, "--render")
	} else {
		cmdArgs = append(cmdArgs, "--message", req.Message)
	}

	if req.State != "" {
		cmdArgs = append(cmdArgs, "--state", req.State)
	}
	if req.Model != "" {
		cmdArgs = append(cmdArgs, "--model", req.Model)
	}

	// Execute openexec-orchestration (assumed to be in PATH)
	cmd := exec.Command("openexec-orchestration", cmdArgs...)
	cmd.Dir = absPath
	output, err := cmd.CombinedOutput()
	if err != nil {
		WriteError(w, http.StatusInternalServerError, fmt.Sprintf("wizard failed: %v\nOutput: %s", err, string(output)))
		return
	}

	if req.Render {
		// Return raw markdown for render mode
		w.Header().Set("Content-Type", "text/markdown")
		w.WriteHeader(http.StatusOK)
		w.Write(output)
	} else {
		// Return JSON from wizard
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write(output)
	}
}
