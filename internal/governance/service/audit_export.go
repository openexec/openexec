package service

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"
)

// auditExportVersion identifies the export format so a verifier knows how the
// seal and per-event hashes were computed.
const auditExportVersion = "openexec.audit/v1"

// AuditExport is a self-contained, verifiable snapshot of the decision-event
// audit trail. It carries every event in insertion order with its hash-chain
// links, the result of re-verifying that chain at export time, and a Seal that
// commits to the entire history (the chain-head hash). When an HMAC key is
// supplied the Seal is additionally signed, giving authenticity on top of the
// integrity the chain already provides.
type AuditExport struct {
	Version      string       `json:"version"`
	GeneratedAt  time.Time    `json:"generated_at"`
	EventCount   int          `json:"event_count"`
	ChainOK      bool         `json:"chain_verified"`
	ChainError   string       `json:"chain_error,omitempty"`
	Seal         string       `json:"seal"`                    // chain-head hash; commits to all events
	SignatureAlg string       `json:"signature_alg,omitempty"` // e.g. HMAC-SHA256
	Signature    string       `json:"signature,omitempty"`     // hex HMAC of Seal, when a key is given
	Events       []AuditEvent `json:"events"`
}

// AuditEvent is the exported, stable representation of one decision event. Field
// names are snake_case and times are RFC3339 so the artifact is tool-agnostic.
type AuditEvent struct {
	ID              string    `json:"id"`
	ReleaseID       string    `json:"release_id,omitempty"`
	ChangeID        string    `json:"change_id,omitempty"`
	ProposalVersion int       `json:"proposal_version"`
	Actor           string    `json:"actor"`
	ActorType       string    `json:"actor_type"`
	Decision        string    `json:"decision"`
	Comment         string    `json:"comment,omitempty"`
	CreatedAt       time.Time `json:"created_at"`
	PrevHash        string    `json:"prev_hash"`
	Hash            string    `json:"hash"`
}

// ExportAudit produces a verifiable audit snapshot. It lists the whole trail (in
// insertion order), re-verifies the hash chain, seals the export with the
// chain-head hash, and — if hmacKey is non-empty — signs that seal with
// HMAC-SHA256. The export always succeeds even when the chain is broken: ChainOK
// records the verdict and ChainError names the break, so a tampered trail is
// reported, never hidden.
func (s *Service) ExportAudit(ctx context.Context, hmacKey []byte) (*AuditExport, error) {
	events, err := s.store.ListAllDecisionEvents(ctx)
	if err != nil {
		return nil, fmt.Errorf("list decision events for export: %w", err)
	}
	ok, reason, _, err := s.store.VerifyAuditChain(ctx)
	if err != nil {
		return nil, fmt.Errorf("verify audit chain for export: %w", err)
	}

	out := &AuditExport{
		Version:     auditExportVersion,
		GeneratedAt: time.Now().UTC(),
		EventCount:  len(events),
		ChainOK:     ok,
		ChainError:  reason,
		Events:      make([]AuditEvent, 0, len(events)),
	}
	for _, e := range events {
		out.Events = append(out.Events, AuditEvent{
			ID:              e.ID,
			ReleaseID:       e.ReleaseID,
			ChangeID:        e.ChangeID,
			ProposalVersion: e.ProposalVersion,
			Actor:           e.Actor,
			ActorType:       e.ActorType,
			Decision:        e.Decision,
			Comment:         e.Comment,
			CreatedAt:       e.CreatedAt,
			PrevHash:        e.PrevHash,
			Hash:            e.Hash,
		})
	}
	// The chain head hash seals the entire ordered history (each hash commits to
	// all prior events). Empty trail => empty seal.
	if n := len(events); n > 0 {
		out.Seal = events[n-1].Hash
	}
	if len(hmacKey) > 0 && out.Seal != "" {
		mac := hmac.New(sha256.New, hmacKey)
		mac.Write([]byte(out.Seal))
		out.SignatureAlg = "HMAC-SHA256"
		out.Signature = hex.EncodeToString(mac.Sum(nil))
	}
	return out, nil
}

// VerifyAudit re-verifies the live decision-event hash chain and returns the
// verdict, a reason on failure, and the number of events checked.
func (s *Service) VerifyAudit(ctx context.Context) (ok bool, reason string, count int, err error) {
	return s.store.VerifyAuditChain(ctx)
}
