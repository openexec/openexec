package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/openexec/openexec/internal/knowledge"
	statepkg "github.com/openexec/openexec/pkg/db/state"
)

type validationPlanProposalRequest struct {
	TaskID    string   `json:"task_id"`
	RunID     string   `json:"run_id,omitempty"`
	PatchHash string   `json:"patch_hash,omitempty"`
	Files     []string `json:"files"`
	SymbolIDs []string `json:"symbol_ids,omitempty"`
	MaxDepth  int      `json:"max_depth,omitempty"`
}

type validationPlanAcceptRequest struct {
	RunID string                            `json:"run_id,omitempty"`
	Items []statepkg.ValidationItemDecision `json:"items"`
}

type validationRunRequest struct {
	ID   string `json:"id"`
	Mode string `json:"mode"`
}

type validationRunStepRequest struct {
	ID        string `json:"id"`
	TraceID   string `json:"trace_id,omitempty"`
	Phase     string `json:"phase"`
	Iteration int    `json:"iteration"`
	Status    string `json:"status"`
}

type validationEvidenceRequest struct {
	ValidationItemID string `json:"validation_item_id"`
	RunID            string `json:"run_id"`
	RunStepID        string `json:"run_step_id"`
	ArtifactHash     string `json:"artifact_hash,omitempty"`
	Status           string `json:"status"`
}

func (s *Server) registerRepositoryGraphValidationRoutes() {
	s.Mux.Handle("POST /api/v1/repository-graph/validation-plans/propose", s.repositoryGraphAuth(http.HandlerFunc(s.handleValidationPlanPropose)))
	s.Mux.Handle("GET /api/v1/repository-graph/validation-plans/{id}", s.repositoryGraphAuth(http.HandlerFunc(s.handleValidationPlanRead)))
	s.Mux.Handle("POST /api/v1/repository-graph/validation-plans/{id}/accept", s.repositoryGraphAuth(http.HandlerFunc(s.handleValidationPlanAccept)))
	s.Mux.Handle("POST /api/v1/repository-graph/validation-plans/{id}/evidence", s.repositoryGraphAuth(http.HandlerFunc(s.handleValidationEvidenceLink)))
	s.Mux.Handle("POST /api/v1/repository-graph/validation-plans/{id}/completion", s.repositoryGraphAuth(http.HandlerFunc(s.handleValidationCompletionFinalize)))
	s.Mux.Handle("GET /api/v1/repository-graph/validation-plans/{id}/completion", s.repositoryGraphAuth(http.HandlerFunc(s.handleValidationCompletionRead)))
	s.Mux.Handle("POST /api/v1/repository-graph/validation-runs", s.repositoryGraphAuth(http.HandlerFunc(s.handleValidationRunRegister)))
	s.Mux.Handle("POST /api/v1/repository-graph/validation-runs/{id}/steps", s.repositoryGraphAuth(http.HandlerFunc(s.handleValidationRunStepRegister)))
}

func decodeValidationJSON(w http.ResponseWriter, r *http.Request, target any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		writeValidationError(w, http.StatusBadRequest, "invalid validation request: "+err.Error())
		return false
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		writeValidationError(w, http.StatusBadRequest, "invalid validation request: request must contain one JSON object")
		return false
	}
	return true
}

func writeValidationError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": message})
}

func validValidationIdentifier(value string) bool {
	value = strings.TrimSpace(value)
	return value != "" && len(value) <= 200 && !strings.ContainsAny(value, "\r\n\x00")
}

func (s *Server) validationPlanForCheckout(ctx context.Context, id, checkoutID string) (statepkg.ValidationPlanRevision, error) {
	if s.StateStore == nil {
		return statepkg.ValidationPlanRevision{}, fmt.Errorf("validation state is unavailable")
	}
	plan, err := s.StateStore.GetValidationPlanRevision(ctx, id)
	if err != nil {
		return statepkg.ValidationPlanRevision{}, err
	}
	var planCheckout string
	if err := s.StateStore.GetDB().QueryRowContext(ctx, `SELECT checkout_id FROM graph_generations WHERE id = ?`, plan.GenerationID).Scan(&planCheckout); err != nil {
		return statepkg.ValidationPlanRevision{}, err
	}
	if planCheckout != checkoutID {
		return statepkg.ValidationPlanRevision{}, sql.ErrNoRows
	}
	return plan, nil
}

func (s *Server) handleValidationPlanPropose(w http.ResponseWriter, r *http.Request) {
	store, identity, ok := s.graphAccess(w, r)
	if !ok {
		return
	}
	var request validationPlanProposalRequest
	if !decodeValidationJSON(w, r, &request) {
		return
	}
	if !validValidationIdentifier(request.TaskID) {
		writeValidationError(w, http.StatusUnprocessableEntity, "task_id is required and must be at most 200 characters")
		return
	}
	if (request.RunID != "" && !validValidationIdentifier(request.RunID)) || len(request.PatchHash) > 256 || strings.ContainsAny(request.PatchHash, "\r\n\x00") {
		writeValidationError(w, http.StatusUnprocessableEntity, "run_id or patch_hash is invalid")
		return
	}
	impactRequest := knowledge.ChangedImpactRequest{Files: request.Files, SymbolIDs: request.SymbolIDs, MaxDepth: request.MaxDepth}
	impact, err := store.ChangedImpactAnalysis(r.Context(), identity, impactRequest, knowledge.DefaultChangedImpactLimits())
	if err != nil {
		var requestError *knowledge.ImpactRequestError
		if errors.As(err, &requestError) {
			writeValidationError(w, http.StatusUnprocessableEntity, requestError.Error())
			return
		}
		s.respondGraphError(w, err)
		return
	}
	if len(impact.ChangedSymbols) == 0 {
		writeValidationError(w, http.StatusUnprocessableEntity, "validation proposal requires at least one resolved changed symbol")
		return
	}
	plan, err := knowledge.ProposeValidationPlanFromChangedImpact(r.Context(), s.StateStore, strings.TrimSpace(request.TaskID), strings.TrimSpace(request.RunID), strings.TrimSpace(request.PatchHash), impactRequest, impact)
	if err != nil {
		writeValidationError(w, http.StatusConflict, err.Error())
		return
	}
	s.respondJSON(w, http.StatusCreated, plan)
}

func (s *Server) handleValidationPlanRead(w http.ResponseWriter, r *http.Request) {
	_, identity, ok := s.graphAccess(w, r)
	if !ok {
		return
	}
	plan, err := s.validationPlanForCheckout(r.Context(), r.PathValue("id"), identity.CheckoutID)
	if err != nil {
		writeValidationError(w, http.StatusNotFound, "validation plan not found")
		return
	}
	s.respondJSON(w, http.StatusOK, plan)
}

func (s *Server) handleValidationPlanAccept(w http.ResponseWriter, r *http.Request) {
	store, identity, ok := s.graphAccess(w, r)
	if !ok {
		return
	}
	var request validationPlanAcceptRequest
	if !decodeValidationJSON(w, r, &request) {
		return
	}
	if request.RunID != "" && !validValidationIdentifier(request.RunID) {
		writeValidationError(w, http.StatusUnprocessableEntity, "run_id is invalid")
		return
	}
	if request.Items == nil {
		writeValidationError(w, http.StatusUnprocessableEntity, "items is required; use an empty array to reject all suggestions")
		return
	}
	plan, err := s.validationPlanForCheckout(r.Context(), r.PathValue("id"), identity.CheckoutID)
	if err != nil {
		writeValidationError(w, http.StatusNotFound, "validation plan not found")
		return
	}
	if len(plan.ImpactSummary.ChangedSymbolIDs) == 0 {
		writeValidationError(w, http.StatusUnprocessableEntity, "validation plan has no resolved changed symbol")
		return
	}
	if existing, err := s.StateStore.AcceptedValidationPlanRevisionForProposal(r.Context(), plan.ID); err == nil {
		s.respondJSON(w, http.StatusCreated, existing)
		return
	} else if !errors.Is(err, sql.ErrNoRows) {
		writeValidationError(w, http.StatusConflict, err.Error())
		return
	}
	current, err := store.CurrentRepositoryState(r.Context(), identity)
	if err != nil {
		s.respondGraphError(w, err)
		return
	}
	if current.GraphVersion != plan.GenerationID || current.WorktreeStateHash != plan.WorktreeStateHash {
		writeValidationError(w, http.StatusConflict, statepkg.ErrValidationProposalStale.Error())
		return
	}
	accepted, err := s.StateStore.AcceptValidationPlanRevision(r.Context(), plan.ID, strings.TrimSpace(request.RunID), request.Items)
	if err != nil {
		if errors.Is(err, statepkg.ErrInvalidItemDecision) {
			writeValidationError(w, http.StatusUnprocessableEntity, err.Error())
			return
		}
		writeValidationError(w, http.StatusConflict, err.Error())
		return
	}
	s.respondJSON(w, http.StatusCreated, accepted)
}

func (s *Server) handleValidationRunRegister(w http.ResponseWriter, r *http.Request) {
	_, identity, ok := s.graphAccess(w, r)
	if !ok {
		return
	}
	var request validationRunRequest
	if !decodeValidationJSON(w, r, &request) {
		return
	}
	if !validValidationIdentifier(request.ID) || (request.Mode != "read-only" && request.Mode != "workspace-write") {
		writeValidationError(w, http.StatusUnprocessableEntity, "run requires a valid id and mode read-only or workspace-write")
		return
	}
	existing, err := s.StateStore.GetRun(r.Context(), request.ID)
	if err != nil {
		writeValidationError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if existing != nil {
		if existing.ProjectPath != identity.RootPath || existing.Mode != request.Mode {
			writeValidationError(w, http.StatusConflict, "run id already belongs to a different execution context")
			return
		}
		s.respondJSON(w, http.StatusOK, map[string]string{"id": existing.ID, "status": existing.Status})
		return
	}
	if err := s.StateStore.CreateRun(r.Context(), request.ID, "", "", identity.RootPath, request.Mode); err != nil {
		writeValidationError(w, http.StatusConflict, err.Error())
		return
	}
	created, err := s.StateStore.GetRun(r.Context(), request.ID)
	if err != nil || created == nil || created.ProjectPath != identity.RootPath || created.Mode != request.Mode {
		writeValidationError(w, http.StatusConflict, "run id was claimed by a different execution context")
		return
	}
	s.respondJSON(w, http.StatusCreated, map[string]string{"id": request.ID, "status": "starting"})
}

func (s *Server) handleValidationRunStepRegister(w http.ResponseWriter, r *http.Request) {
	_, identity, ok := s.graphAccess(w, r)
	if !ok {
		return
	}
	run, err := s.StateStore.GetRun(r.Context(), r.PathValue("id"))
	if err != nil || run == nil || run.ProjectPath != identity.RootPath {
		writeValidationError(w, http.StatusNotFound, "validation run not found")
		return
	}
	var request validationRunStepRequest
	if !decodeValidationJSON(w, r, &request) {
		return
	}
	validPhase := map[string]bool{"plan": true, "execute": true, "verify": true, "finalize": true}
	validStatus := map[string]bool{"completed": true, "failed": true, "inconclusive": true, "unavailable": true}
	if !validValidationIdentifier(request.ID) || !validPhase[request.Phase] || !validStatus[request.Status] || request.Iteration < 1 {
		writeValidationError(w, http.StatusUnprocessableEntity, "step requires a valid id, phase, positive iteration, and terminal status")
		return
	}
	existing, err := s.StateStore.GetRunStep(r.Context(), request.ID)
	if err != nil {
		writeValidationError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if existing != nil {
		if existing.RunID != run.ID || existing.Phase != request.Phase || existing.Iteration != request.Iteration || existing.Status != request.Status {
			writeValidationError(w, http.StatusConflict, "run step id already belongs to different evidence")
			return
		}
		s.respondJSON(w, http.StatusOK, map[string]string{"id": existing.ID, "status": existing.Status})
		return
	}
	if err := s.StateStore.AddRunStep(r.Context(), request.ID, run.ID, strings.TrimSpace(request.TraceID), request.Phase, request.Iteration, request.Status); err != nil {
		writeValidationError(w, http.StatusConflict, err.Error())
		return
	}
	created, err := s.StateStore.GetRunStep(r.Context(), request.ID)
	if err != nil || created == nil || created.RunID != run.ID || created.Phase != request.Phase || created.Iteration != request.Iteration || created.Status != request.Status {
		writeValidationError(w, http.StatusConflict, "run step id was claimed by different evidence")
		return
	}
	s.respondJSON(w, http.StatusCreated, map[string]string{"id": request.ID, "status": request.Status})
}

func (s *Server) handleValidationEvidenceLink(w http.ResponseWriter, r *http.Request) {
	_, identity, ok := s.graphAccess(w, r)
	if !ok {
		return
	}
	plan, err := s.validationPlanForCheckout(r.Context(), r.PathValue("id"), identity.CheckoutID)
	if err != nil {
		writeValidationError(w, http.StatusNotFound, "validation plan not found")
		return
	}
	var request validationEvidenceRequest
	if !decodeValidationJSON(w, r, &request) {
		return
	}
	if !validValidationIdentifier(request.ValidationItemID) || !validValidationIdentifier(request.RunID) || !validValidationIdentifier(request.RunStepID) {
		writeValidationError(w, http.StatusUnprocessableEntity, "evidence requires valid item, run, and step identities")
		return
	}
	itemBelongs := false
	for _, item := range plan.Items {
		itemBelongs = itemBelongs || item.ID == request.ValidationItemID
	}
	if !itemBelongs {
		writeValidationError(w, http.StatusUnprocessableEntity, "validation item does not belong to this plan revision")
		return
	}
	run, err := s.StateStore.GetRun(r.Context(), request.RunID)
	if err != nil || run == nil || run.ProjectPath != identity.RootPath {
		writeValidationError(w, http.StatusNotFound, "validation run not found")
		return
	}
	step, err := s.StateStore.GetRunStep(r.Context(), request.RunStepID)
	if err != nil || step == nil || step.RunID != run.ID {
		writeValidationError(w, http.StatusNotFound, "validation run step not found")
		return
	}
	if !statepkg.EvidenceStatusMatchesRunStep(request.Status, step.Status) {
		writeValidationError(w, http.StatusUnprocessableEntity, "evidence status does not match the structured run step")
		return
	}
	link := statepkg.ValidationEvidenceLink{
		ValidationItemID: request.ValidationItemID, RunID: request.RunID, RunStepID: request.RunStepID,
		ArtifactHash: request.ArtifactHash, WorktreeStateHash: plan.WorktreeStateHash, PatchHash: plan.PatchHash, Status: request.Status,
	}
	if err := s.StateStore.LinkValidationEvidence(r.Context(), link); err != nil {
		writeValidationError(w, http.StatusConflict, err.Error())
		return
	}
	s.respondJSON(w, http.StatusCreated, link)
}

func (s *Server) handleValidationCompletionFinalize(w http.ResponseWriter, r *http.Request) {
	_, identity, ok := s.graphAccess(w, r)
	if !ok {
		return
	}
	plan, err := s.validationPlanForCheckout(r.Context(), r.PathValue("id"), identity.CheckoutID)
	if err != nil {
		writeValidationError(w, http.StatusNotFound, "validation plan not found")
		return
	}
	report, err := s.StateStore.FinalizeEvidenceCoverage(r.Context(), plan.ID)
	if err != nil {
		writeValidationError(w, http.StatusConflict, err.Error())
		return
	}
	s.respondJSON(w, http.StatusCreated, report)
}

func (s *Server) handleValidationCompletionRead(w http.ResponseWriter, r *http.Request) {
	_, identity, ok := s.graphAccess(w, r)
	if !ok {
		return
	}
	plan, err := s.validationPlanForCheckout(r.Context(), r.PathValue("id"), identity.CheckoutID)
	if err != nil {
		writeValidationError(w, http.StatusNotFound, "validation plan not found")
		return
	}
	report, err := s.StateStore.ReadCompletionReport(r.Context(), plan.ID)
	if errors.Is(err, sql.ErrNoRows) {
		writeValidationError(w, http.StatusNotFound, "completion report has not been finalized")
		return
	}
	if err != nil {
		writeValidationError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.respondJSON(w, http.StatusOK, report)
}
