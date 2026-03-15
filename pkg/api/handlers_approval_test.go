package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/openexec/openexec/internal/approval"
)

func TestHandleListApprovals_NoGate(t *testing.T) {
	s := &Server{}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/approvals", nil)
	w := httptest.NewRecorder()

	s.handleListApprovals(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("expected status %d, got %d", http.StatusServiceUnavailable, w.Code)
	}
}

func TestHandleListApprovals_EmptyList(t *testing.T) {
	gate := approval.NewInMemoryGate(nil)
	s := &Server{ApprovalGate: gate}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/approvals", nil)
	w := httptest.NewRecorder()

	s.handleListApprovals(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	approvals, ok := resp["approvals"].([]interface{})
	if !ok {
		t.Fatalf("expected approvals array in response")
	}
	if len(approvals) != 0 {
		t.Errorf("expected 0 approvals, got %d", len(approvals))
	}
}

func TestHandleGetApproval_NotFound(t *testing.T) {
	gate := approval.NewInMemoryGate(nil)
	s := &Server{ApprovalGate: gate}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/approvals/nonexistent", nil)
	req.SetPathValue("id", "nonexistent")
	w := httptest.NewRecorder()

	s.handleGetApproval(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected status %d, got %d", http.StatusNotFound, w.Code)
	}
}

func TestHandleGetApproval_NoGate(t *testing.T) {
	s := &Server{}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/approvals/test", nil)
	req.SetPathValue("id", "test")
	w := httptest.NewRecorder()

	s.handleGetApproval(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("expected status %d, got %d", http.StatusServiceUnavailable, w.Code)
	}
}

func TestHandleApproveRequest_NotFound(t *testing.T) {
	gate := approval.NewInMemoryGate(nil)
	s := &Server{ApprovalGate: gate}

	body := bytes.NewBufferString(`{"decided_by": "test"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/approvals/nonexistent/approve", body)
	req.SetPathValue("id", "nonexistent")
	w := httptest.NewRecorder()

	s.handleApproveRequest(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected status %d, got %d", http.StatusNotFound, w.Code)
	}
}

func TestHandleRejectRequest_NotFound(t *testing.T) {
	gate := approval.NewInMemoryGate(nil)
	s := &Server{ApprovalGate: gate}

	body := bytes.NewBufferString(`{"decided_by": "test", "reason": "testing"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/approvals/nonexistent/reject", body)
	req.SetPathValue("id", "nonexistent")
	w := httptest.NewRecorder()

	s.handleRejectRequest(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected status %d, got %d", http.StatusNotFound, w.Code)
	}
}

func TestHandleApproveRequest_NoGate(t *testing.T) {
	s := &Server{}

	body := bytes.NewBufferString(`{"decided_by": "test"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/approvals/test/approve", body)
	req.SetPathValue("id", "test")
	w := httptest.NewRecorder()

	s.handleApproveRequest(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("expected status %d, got %d", http.StatusServiceUnavailable, w.Code)
	}
}

func TestHandleRejectRequest_NoGate(t *testing.T) {
	s := &Server{}

	body := bytes.NewBufferString(`{"decided_by": "test", "reason": "testing"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/approvals/test/reject", body)
	req.SetPathValue("id", "test")
	w := httptest.NewRecorder()

	s.handleRejectRequest(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("expected status %d, got %d", http.StatusServiceUnavailable, w.Code)
	}
}

func TestGateRequestToResponse_Nil(t *testing.T) {
	resp := gateRequestToResponse(nil)
	if resp != nil {
		t.Errorf("expected nil response for nil input")
	}
}
