package service

import (
	"context"
	"testing"

	"github.com/openexec/openexec/internal/governance"
)

func TestExportAudit_SealsAndSigns(t *testing.T) {
	store := newTestStore(t)
	svc := NewService(store, Options{})
	ctx := context.Background()

	// Produce a few events through real operations.
	seedChange(t, store, &governance.ChangeRecord{
		ID: "C-1", Status: governance.ChangeStatusImplementing, Risk: governance.RiskLow,
	})
	if err := svc.RecordPR(ctx, "C-1", "https://github.com/o/r/pull/1", "b"); err != nil {
		t.Fatalf("RecordPR: %v", err)
	}
	if err := svc.ReadyForTest(ctx, "C-1"); err != nil {
		t.Fatalf("ReadyForTest: %v", err)
	}

	// Unsigned export: sealed by the chain head, verified, no signature.
	exp, err := svc.ExportAudit(ctx, nil)
	if err != nil {
		t.Fatalf("ExportAudit: %v", err)
	}
	if !exp.ChainOK {
		t.Fatalf("expected chain verified, got error %q", exp.ChainError)
	}
	if exp.EventCount != 2 || len(exp.Events) != 2 {
		t.Fatalf("expected 2 events, got count=%d len=%d", exp.EventCount, len(exp.Events))
	}
	if exp.Seal == "" || exp.Seal != exp.Events[len(exp.Events)-1].Hash {
		t.Fatalf("seal must equal the chain-head hash; seal=%q head=%q", exp.Seal, exp.Events[len(exp.Events)-1].Hash)
	}
	if exp.Signature != "" {
		t.Fatalf("expected no signature without a key, got %q", exp.Signature)
	}

	// Signed export: same seal, plus a deterministic HMAC.
	signed, err := svc.ExportAudit(ctx, []byte("secret-key"))
	if err != nil {
		t.Fatalf("ExportAudit(signed): %v", err)
	}
	if signed.Seal != exp.Seal {
		t.Fatalf("seal must be stable across exports: %q vs %q", signed.Seal, exp.Seal)
	}
	if signed.SignatureAlg != "HMAC-SHA256" || signed.Signature == "" {
		t.Fatalf("expected an HMAC signature, got alg=%q sig=%q", signed.SignatureAlg, signed.Signature)
	}
	again, _ := svc.ExportAudit(ctx, []byte("secret-key"))
	if again.Signature != signed.Signature {
		t.Fatalf("HMAC over a stable seal must be deterministic")
	}
	other, _ := svc.ExportAudit(ctx, []byte("different-key"))
	if other.Signature == signed.Signature {
		t.Fatalf("different keys must produce different signatures")
	}
}
