package governance

import (
	"context"
	"strings"
	"testing"
)

// appendEvents inserts n chained decision events for a change and returns them.
func appendEvents(t *testing.T, store *SQLiteStore, changeID string, decisions ...string) {
	t.Helper()
	ctx := context.Background()
	for i, d := range decisions {
		ev := &DecisionEvent{
			ID:       changeID + "-ev-" + string(rune('a'+i)),
			ChangeID: changeID,
			Actor:    "pm",
			Decision: d,
		}
		if err := store.CreateDecisionEvent(ctx, ev); err != nil {
			t.Fatalf("CreateDecisionEvent(%s): %v", d, err)
		}
		if ev.Hash == "" {
			t.Fatalf("event %s got empty hash", ev.ID)
		}
	}
}

func TestAuditChain_IntactAfterAppends(t *testing.T) {
	_, store := newTestStore(t)
	ctx := context.Background()

	appendEvents(t, store, "C-1", "proposed", "approved", "marked_done")
	appendEvents(t, store, "C-2", "proposed", "rejected")

	ok, reason, count, err := store.VerifyAuditChain(ctx)
	if err != nil {
		t.Fatalf("VerifyAuditChain: %v", err)
	}
	if !ok {
		t.Fatalf("expected intact chain, got break: %s", reason)
	}
	if count != 5 {
		t.Fatalf("expected 5 verified events, got %d", count)
	}
}

func TestAuditChain_FirstEventHasEmptyPrev(t *testing.T) {
	_, store := newTestStore(t)
	ctx := context.Background()
	appendEvents(t, store, "C-1", "proposed", "approved")

	evs, err := store.ListDecisionEvents(ctx, "C-1")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(evs) != 2 {
		t.Fatalf("want 2, got %d", len(evs))
	}
	if evs[0].PrevHash != "" {
		t.Fatalf("first event prev_hash should be empty, got %q", evs[0].PrevHash)
	}
	if evs[1].PrevHash != evs[0].Hash {
		t.Fatalf("second event prev_hash %q should equal first hash %q", evs[1].PrevHash, evs[0].Hash)
	}
}

// TestAuditTriggers_BlockUpdateAndDelete proves the database itself refuses to
// mutate the append-only audit tables — the tamper PREVENTION layer.
func TestAuditTriggers_BlockUpdateAndDelete(t *testing.T) {
	_, store := newTestStore(t)
	ctx := context.Background()
	appendEvents(t, store, "C-1", "proposed")
	if err := store.CreateEvidence(ctx, &Evidence{ID: "E-1", ChangeID: "C-1", Kind: "test", Source: "ci"}); err != nil {
		t.Fatalf("create evidence: %v", err)
	}

	cases := []struct{ name, sql string }{
		{"update decision_events", `UPDATE decision_events SET comment='x' WHERE id='C-1-ev-a'`},
		{"delete decision_events", `DELETE FROM decision_events WHERE id='C-1-ev-a'`},
		{"update evidence", `UPDATE evidence SET summary='x' WHERE id='E-1'`},
		{"delete evidence", `DELETE FROM evidence WHERE id='E-1'`},
	}
	for _, c := range cases {
		if _, err := store.db.ExecContext(ctx, c.sql); err == nil || !strings.Contains(err.Error(), "append-only") {
			t.Fatalf("%s: expected append-only trigger to abort, got %v", c.name, err)
		}
	}
}

// TestAuditChain_DetectsTampering drops the guard trigger, alters a row directly
// (simulating an attacker with raw DB access), and confirms the hash chain still
// DETECTS the change — tamper evidence independent of the triggers.
func TestAuditChain_DetectsTampering(t *testing.T) {
	_, store := newTestStore(t)
	ctx := context.Background()
	appendEvents(t, store, "C-1", "proposed", "approved", "marked_done")

	if _, err := store.db.ExecContext(ctx, `DROP TRIGGER decision_events_no_update`); err != nil {
		t.Fatalf("drop trigger: %v", err)
	}
	if _, err := store.db.ExecContext(ctx,
		`UPDATE decision_events SET comment='forged' WHERE id='C-1-ev-b'`); err != nil {
		t.Fatalf("tamper update: %v", err)
	}

	ok, reason, _, err := store.VerifyAuditChain(ctx)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if ok {
		t.Fatalf("expected chain to detect the forged event")
	}
	if !strings.Contains(reason, "C-1-ev-b") {
		t.Fatalf("expected reason to name the tampered event, got %q", reason)
	}
}
