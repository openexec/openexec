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

// repositoryGraphReadAuth preserves the historical checkout-only API when no
// external evidence credential is configured. Once the external profile is
// enabled, the legacy read routes must require the same independent bearer;
// otherwise they are an unauthenticated alias of the protected handlers.
func (s *Server) repositoryGraphReadAuth(next http.Handler) http.Handler {
	if s.repositoryEvidenceToken == "" {
		return next
	}
	return s.repositoryEvidenceAuth(next)
}
