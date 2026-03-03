package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/openexec/openexec/pkg/db/session"
)

type CreateSessionRequest struct {
	ProjectPath string `json:"projectPath"`
	Provider    string `json:"provider"`
	Model       string `json:"model"`
	Title       string `json:"title"`
}

type UpdateSessionRequest struct {
	Title string `json:"title"`
}

type ForkSessionRequest struct {
	ForkPointMessageID string `json:"forkPointMessageId"`
	Title              string `json:"title"`
	Provider           string `json:"provider"`
	Model              string `json:"model"`
}

// SessionDTO is a UI-friendly version of session.Session
// Using camelCase JSON tags to match UI expectations in TypeScript
type SessionDTO struct {
	ID                 string `json:"id"`
	ProjectPath        string `json:"projectPath"`
	Provider           string `json:"provider"`
	Model              string `json:"model"`
	Title              string `json:"title"`
	ParentSessionID    string `json:"parentSessionId,omitempty"`
	ForkPointMessageID string `json:"forkPointMessageId,omitempty"`
	Status             string `json:"status"`
	CreatedAt          string `json:"createdAt"`
	UpdatedAt          string `json:"updatedAt"`
}

type MessageDTO struct {
	ID           string  `json:"id"`
	SessionID    string  `json:"sessionId"`
	Role         string  `json:"role"`
	Content      string  `json:"content"`
	TokensInput  int     `json:"tokensInput"`
	TokensOutput int     `json:"tokensOutput"`
	CostUSD      float64 `json:"costUsd"`
	CreatedAt    string  `json:"createdAt"`
}

type MessagesResponse struct {
	Messages   []MessageDTO `json:"messages"`
	Pagination struct {
		Offset     int  `json:"offset"`
		Limit      int  `json:"limit"`
		HasMore    bool `json:"hasMore"`
		TotalCount int  `json:"totalCount"`
	} `json:"pagination"`
}

func toSessionDTO(s *session.Session) SessionDTO {
	dto := SessionDTO{
		ID:          s.ID,
		ProjectPath: s.ProjectPath,
		Provider:    s.Provider,
		Model:       s.Model,
		Title:       s.Title,
		Status:      string(s.Status),
		CreatedAt:   s.CreatedAt.Format(time.RFC3339),
		UpdatedAt:   s.UpdatedAt.Format(time.RFC3339),
	}
	if s.ParentSessionID.Valid {
		dto.ParentSessionID = s.ParentSessionID.String
	}
	if s.ForkPointMessageID.Valid {
		dto.ForkPointMessageID = s.ForkPointMessageID.String
	}
	return dto
}

func toMessageDTO(m *session.Message) MessageDTO {
	return MessageDTO{
		ID:           m.ID,
		SessionID:    m.SessionID,
		Role:         m.Role.String(),
		Content:      m.Content,
		TokensInput:  m.TokensInput,
		TokensOutput: m.TokensOutput,
		CostUSD:      m.CostUSD,
		CreatedAt:    m.CreatedAt.Format(time.RFC3339),
	}
}

func (s *Server) handleListSessions(w http.ResponseWriter, r *http.Request) {
	projectPath := r.URL.Query().Get("project_path")
	var sessions []*session.Session
	var err error

	if projectPath != "" {
		sessions, err = s.SessionRepo.ListSessionsByProject(r.Context(), projectPath)
	} else {
		sessions, err = s.SessionRepo.ListSessions(r.Context(), nil)
	}

	if err != nil {
		WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	dtos := make([]SessionDTO, len(sessions))
	for i, sess := range sessions {
		dtos[i] = toSessionDTO(sess)
	}

	WriteJSON(w, http.StatusOK, dtos)
}

func (s *Server) handleCreateSession(w http.ResponseWriter, r *http.Request) {
	var req CreateSessionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	now := time.Now()
	title := req.Title
	if title == "" {
		title = fmt.Sprintf("Session %s", now.Format("2006-01-02 15:04"))
	}

	newSession := &session.Session{
		ID:          uuid.New().String(),
		ProjectPath: req.ProjectPath,
		Provider:    req.Provider,
		Model:       req.Model,
		Title:       title,
		Status:      session.StatusActive,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	if err := s.SessionRepo.CreateSession(r.Context(), newSession); err != nil {
		WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	WriteJSON(w, http.StatusCreated, toSessionDTO(newSession))
}

func (s *Server) handleGetSession(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		WriteError(w, http.StatusBadRequest, "session id required")
		return
	}

	sess, err := s.SessionRepo.GetSession(r.Context(), id)
	if err != nil {
		WriteError(w, http.StatusNotFound, "session not found")
		return
	}

	WriteJSON(w, http.StatusOK, toSessionDTO(sess))
}

func (s *Server) handleUpdateSession(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		WriteError(w, http.StatusBadRequest, "session id required")
		return
	}

	var req UpdateSessionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	sess, err := s.SessionRepo.GetSession(r.Context(), id)
	if err != nil {
		WriteError(w, http.StatusNotFound, "session not found")
		return
	}

	sess.Title = req.Title
	sess.UpdatedAt = time.Now()

	if err := s.SessionRepo.UpdateSession(r.Context(), sess); err != nil {
		WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	WriteJSON(w, http.StatusOK, toSessionDTO(sess))
}

func (s *Server) handleDeleteSession(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		WriteError(w, http.StatusBadRequest, "session id required")
		return
	}

	if err := s.SessionRepo.DeleteSession(r.Context(), id); err != nil {
		WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleArchiveSession(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		WriteError(w, http.StatusBadRequest, "session id required")
		return
	}

	sess, err := s.SessionRepo.GetSession(r.Context(), id)
	if err != nil {
		WriteError(w, http.StatusNotFound, "session not found")
		return
	}

	sess.Status = session.StatusArchived
	sess.UpdatedAt = time.Now()

	if err := s.SessionRepo.UpdateSession(r.Context(), sess); err != nil {
		WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	WriteJSON(w, http.StatusOK, toSessionDTO(sess))
}

func (s *Server) handleForkSession(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		WriteError(w, http.StatusBadRequest, "parent session id required")
		return
	}

	var req ForkSessionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	opts := &session.ForkOptions{
		ForkPointMessageID: req.ForkPointMessageID,
		Title:              req.Title,
		Provider:           req.Provider,
		Model:              req.Model,
		CopyMessages:       true,
		CopyToolCalls:      true,
		CopySummaries:      true,
	}

	forked, err := s.SessionRepo.ForkSession(r.Context(), id, opts)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	WriteJSON(w, http.StatusCreated, toSessionDTO(forked))
}

func (s *Server) handleGetForkInfo(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		WriteError(w, http.StatusBadRequest, "session id required")
		return
	}

	info, err := s.SessionRepo.GetForkInfo(r.Context(), id)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	WriteJSON(w, http.StatusOK, info)
}

func (s *Server) handleListSessionForks(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		WriteError(w, http.StatusBadRequest, "session id required")
		return
	}

	forks, err := s.SessionRepo.GetSessionForks(r.Context(), id)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	dtos := make([]SessionDTO, len(forks))
	for i, f := range forks {
		dtos[i] = toSessionDTO(f)
	}

	WriteJSON(w, http.StatusOK, dtos)
}

func (s *Server) handleListMessages(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		WriteError(w, http.StatusBadRequest, "session id required")
		return
	}

	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit == 0 {
		limit = 50
	}
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))

	messages, err := s.SessionRepo.ListMessages(r.Context(), id)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	totalCount, _ := s.SessionRepo.GetMessageCount(r.Context(), id)

	var response MessagesResponse
	response.Messages = make([]MessageDTO, 0)
	
	// Apply manual pagination since ListMessages doesn't support it yet
	start := offset
	if start > len(messages) {
		start = len(messages)
	}
	end := offset + limit
	if end > len(messages) {
		end = len(messages)
	}

	for _, m := range messages[start:end] {
		response.Messages = append(response.Messages, toMessageDTO(m))
	}

	response.Pagination.Offset = offset
	response.Pagination.Limit = limit
	response.Pagination.TotalCount = totalCount
	response.Pagination.HasMore = totalCount > offset+limit

	WriteJSON(w, http.StatusOK, response)
}
