package server

import (
	"crypto/sha256"
	"crypto/subtle"
	"net/http"
	"strings"
)

// registerRepositoryEvidenceRoutes is deliberately repetitive: each method
// and path is visible here. A generic proxy would let a later validation write
// silently enter this external read profile.
func (s *Server) registerRepositoryEvidenceRoutes() {
	s.Mux.Handle("GET /api/v1/external-evidence/symbols", s.repositoryEvidenceAuth(http.HandlerFunc(s.handleGraphSymbols)))
	s.Mux.Handle("GET /api/v1/external-evidence/symbols/{id}", s.repositoryEvidenceAuth(http.HandlerFunc(s.handleGraphSymbolDetail)))
	s.Mux.Handle("GET /api/v1/external-evidence/symbols/{id}/dependencies", s.repositoryEvidenceAuth(http.HandlerFunc(s.handleGraphSymbolDependencies)))
	s.Mux.Handle("GET /api/v1/external-evidence/symbols/{id}/calls", s.repositoryEvidenceAuth(http.HandlerFunc(s.handleGraphSymbolCalls)))
	s.Mux.Handle("GET /api/v1/external-evidence/symbols/{id}/impact", s.repositoryEvidenceAuth(http.HandlerFunc(s.handleGraphSymbolImpact)))
	s.Mux.Handle("GET /api/v1/external-evidence/symbols/{id}/source", s.repositoryEvidenceAuth(http.HandlerFunc(s.handleGraphSymbolSource)))
}

func (s *Server) repositoryEvidenceAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.TrimSpace(s.repositoryEvidenceToken) == "" {
			s.respondJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "repository graph credential is not configured"})
			return
		}
		scheme, supplied, ok := strings.Cut(r.Header.Get("Authorization"), " ")
		if !ok || !strings.EqualFold(scheme, "Bearer") {
			s.respondJSON(w, http.StatusUnauthorized, map[string]string{"error": "repository evidence bearer required"})
			return
		}
		suppliedHash := sha256.Sum256([]byte(supplied))
		expectedHash := sha256.Sum256([]byte(s.repositoryEvidenceToken))
		if subtle.ConstantTimeCompare(suppliedHash[:], expectedHash[:]) != 1 {
			s.respondJSON(w, http.StatusUnauthorized, map[string]string{"error": "repository evidence bearer required"})
			return
		}
		next.ServeHTTP(w, r)
	})
}

// repositoryGraphAuth protects the internal repository graph API. The bearer
// belongs to the trusted Agent Console/OpenExec server connection and is never
// exposed to an external advisory principal. Checkout identity remains a
// separate, mandatory scope check inside each handler.
func (s *Server) repositoryGraphAuth(next http.Handler) http.Handler {
	return s.repositoryEvidenceAuth(next)
}
