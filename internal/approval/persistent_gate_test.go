package approval

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"github.com/openexec/openexec/pkg/db/sqlitecfg"
)

// openSecondProcess simulates the operator's separate process: a second
// connection to the same approvals database.
func openSecondProcess(t *testing.T, dbPath string) *Manager {
	t.Helper()
	db, err := sql.Open("sqlite", sqlitecfg.DSN(dbPath))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	repo, err := NewSQLiteRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	return NewManager(repo)
}

func TestPersistentGate_CrossProcessApprove(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "approvals.db")
	gate, _, err := OpenPersistentGate(dbPath, t.TempDir(), 30*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if err := gate.SetToolRisk(RiskLevelHigh, "terraform_apply"); err != nil {
		t.Fatal(err)
	}

	type outcome struct {
		req *GateRequest
		err error
	}
	done := make(chan outcome, 1)
	go func() {
		req, err := gate.RequestApproval(context.Background(), &GateRequest{
			ToolName: "terraform_apply",
			ToolArgs: map[string]any{"environment": "production"},
		})
		done <- outcome{req, err}
	}()

	// "Other terminal": wait until the request is visible, then approve it.
	operator := openSecondProcess(t, dbPath)
	var pendingID string
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		pending, err := operator.ListPendingRequests(context.Background(), PersistentGateSessionID)
		if err == nil && len(pending) == 1 {
			pendingID = pending[0].ID
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if pendingID == "" {
		t.Fatal("pending request never appeared in the shared database")
	}
	if err := operator.Approve(context.Background(), pendingID, "test-operator", "lgtm"); err != nil {
		t.Fatalf("operator approve: %v", err)
	}

	select {
	case o := <-done:
		if o.err != nil {
			t.Fatalf("RequestApproval: %v", o.err)
		}
		if o.req.Status != RequestStatusApproved {
			t.Errorf("status = %s, want approved", o.req.Status)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("gate did not observe the cross-process approval")
	}
}

func TestPersistentGate_CrossProcessReject(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "approvals.db")
	gate, _, err := OpenPersistentGate(dbPath, t.TempDir(), 30*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if err := gate.SetToolRisk(RiskLevelHigh, "salt_apply_state"); err != nil {
		t.Fatal(err)
	}

	done := make(chan *GateRequest, 1)
	go func() {
		req, _ := gate.RequestApproval(context.Background(), &GateRequest{
			ToolName: "salt_apply_state",
			ToolArgs: map[string]any{"state": "app.deploy"},
		})
		done <- req
	}()

	operator := openSecondProcess(t, dbPath)
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		pending, err := operator.ListPendingRequests(context.Background(), PersistentGateSessionID)
		if err == nil && len(pending) == 1 {
			if err := operator.Reject(context.Background(), pending[0].ID, "test-operator", "not during business hours"); err != nil {
				t.Fatalf("reject: %v", err)
			}
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	select {
	case req := <-done:
		if req.Status != RequestStatusRejected {
			t.Errorf("status = %s, want rejected", req.Status)
		}
		if req.RejectReason == "" {
			t.Error("reject reason should be propagated to the blocked caller")
		}
	case <-time.After(15 * time.Second):
		t.Fatal("gate did not observe the cross-process rejection")
	}
}

func TestPersistentGate_WaitTimeoutExpires(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "approvals.db")
	// Wait window shorter than the poll interval forces a timeout.
	gate, _, err := OpenPersistentGate(dbPath, t.TempDir(), 1*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if err := gate.SetToolRisk(RiskLevelHigh, "terraform_apply"); err != nil {
		t.Fatal(err)
	}
	req, err := gate.RequestApproval(context.Background(), &GateRequest{
		ToolName: "terraform_apply",
		ToolArgs: map[string]any{},
	})
	if err == nil {
		t.Fatal("expected a timeout error when nobody signs off")
	}
	if req == nil || req.Status != RequestStatusExpired {
		t.Errorf("timed-out request should be expired, got %+v", req)
	}
}
