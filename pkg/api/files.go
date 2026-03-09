package api

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

type DirectoryInfo struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

func (s *Server) handleListDirectories(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	path := r.URL.Query().Get("path")
	if path == "" {
		path = s.ProjectsDir
	}

	// Clean path and ensure it's not trying to escape root if we wanted to enforce that
	// For now we allow browsing based on provided path

	entries, err := os.ReadDir(path)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "failed to read directory: "+err.Error())
		return
	}

	dirs := make([]DirectoryInfo, 0)

	// Add parent directory option if not at root
	parent := filepath.Dir(path)
	if parent != path {
		dirs = append(dirs, DirectoryInfo{
			Name: "..",
			Path: parent,
		})
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		// Skip hidden directories
		if strings.HasPrefix(entry.Name(), ".") {
			continue
		}

		dirs = append(dirs, DirectoryInfo{
			Name: entry.Name(),
			Path: filepath.Join(path, entry.Name()),
		})
	}

	WriteJSON(w, http.StatusOK, dirs)
}
