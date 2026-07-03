package mcp

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/openexec/openexec/internal/governance"
)

// seedGovernance opens a governance store at projDir, runs fn to populate it,
// then closes it so the MCP server opens its own fresh connection (as a separate
// process would). It mirrors service/service_test.go's temp-dir + .openexec setup.
func seedGovernance(t *testing.T, projDir string, fn func(store governance.Store)) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(projDir, ".openexec"), 0o755); err != nil {
		t.Fatalf("mkdir .openexec: %v", err)
	}
	db, store, err := governance.Open(projDir)
	if err != nil {
		t.Fatalf("open governance store: %v", err)
	}
	fn(store)
	if err := store.Close(); err != nil {
		t.Fatalf("close store: %v", err)
	}
	_ = db.Close()
}

// seedApprovedChange creates an approved release and one approved-for-AI change
// linked to it, plus an unapproved change that must stay hidden from the queue.
func seedApprovedChange(t *testing.T, projDir string) {
	t.Helper()
	ctx := context.Background()
	seedGovernance(t, projDir, func(store governance.Store) {
		if err := store.CreateRelease(ctx, &governance.GovernanceRelease{
			ID:            "R-1",
			Name:          "July fixes",
			Owner:         "perttu",
			Status:        governance.ReleaseStatusApproved,
			Risk:          governance.RiskLow,
			ApprovedForAI: true,
		}); err != nil {
			t.Fatalf("create release: %v", err)
		}
		if err := store.CreateChangeRecord(ctx, &governance.ChangeRecord{
			ID:                 "C-APPROVED",
			ReleaseID:          "R-1",
			ProjectID:          "proj",
			SourceType:         governance.SourceManual,
			SourceID:           "approved-1",
			Title:              "Approved work",
			Kind:               governance.KindBug,
			Risk:               governance.RiskLow,
			Status:             governance.ChangeStatusApprovedForAI,
			ProposalVersion:    1,
			ApprovedVersion:    1,
			AcceptanceCriteria: "users can log in",
			VerificationPlan:   "go test ./...",
		}); err != nil {
			t.Fatalf("create approved change: %v", err)
		}
		if err := store.CreateChangeRecord(ctx, &governance.ChangeRecord{
			ID:         "C-UNAPPROVED",
			ReleaseID:  "R-1",
			ProjectID:  "proj",
			SourceType: governance.SourceManual,
			SourceID:   "unapproved-1",
			Title:      "Not yet approved",
			Status:     governance.ChangeStatusPlanned,
		}); err != nil {
			t.Fatalf("create unapproved change: %v", err)
		}
	})
}

// TestListApprovedWork_HidesUnapproved verifies the queue returns approved,
// claimable work and never surfaces unapproved change records.
func TestListApprovedWork_HidesUnapproved(t *testing.T) {
	projDir := t.TempDir()
	seedApprovedChange(t, projDir)
	srv, out := newBacklogTestServer(t, projDir)

	result := callTool(t, srv, out, "openexec_list_approved_work", map[string]interface{}{
		"project_id": "proj",
	})
	if isToolError(result) {
		t.Fatalf("list_approved_work returned error: %s", resultText(result))
	}

	work, _ := result["work"].([]interface{})
	if len(work) != 1 {
		t.Fatalf("expected exactly 1 approved change, got %d: %v", len(work), resultText(result))
	}
	got, _ := work[0].(map[string]interface{})
	if id, _ := got["id"].(string); id != "C-APPROVED" {
		t.Fatalf("expected C-APPROVED in queue, got %q", id)
	}
	// The unapproved change must never appear.
	for _, w := range work {
		m, _ := w.(map[string]interface{})
		if id, _ := m["id"].(string); id == "C-UNAPPROVED" {
			t.Fatalf("unapproved change C-UNAPPROVED leaked into the queue")
		}
	}
}

// TestClaimWork_PreventsSecondClaim verifies a claimed change cannot be claimed
// by a different executor while the lease is active.
func TestClaimWork_PreventsSecondClaim(t *testing.T) {
	projDir := t.TempDir()
	seedApprovedChange(t, projDir)
	srv, out := newBacklogTestServer(t, projDir)

	first := callTool(t, srv, out, "openexec_claim_work", map[string]interface{}{
		"change_id": "C-APPROVED",
		"agent":     "codex",
	})
	if isToolError(first) {
		t.Fatalf("first claim should succeed, got error: %s", resultText(first))
	}

	second := callTool(t, srv, out, "openexec_claim_work", map[string]interface{}{
		"change_id": "C-APPROVED",
		"agent":     "claude",
	})
	if !isToolError(second) {
		t.Fatalf("second claim by a different agent should be refused, but succeeded: %s", resultText(second))
	}
}
