package runtime

import "github.com/openexec/openexec/internal/execution/evidence"

// EvidenceBuffer is a draining, bounded stream capture for admitted executors.
type EvidenceBuffer = evidence.Buffer
type CommandEvidence = evidence.Command

const EvidenceStreamLimit = evidence.StreamLimit

// RetainCommandEvidence writes owner-only evidence, independently of failure classification.
func RetainCommandEvidence(dir string, command CommandEvidence) (string, string, error) {
	return evidence.Write(dir, command)
}
func ReadCommandEvidence(dir, hash string) (*CommandEvidence, error) { return evidence.Read(dir, hash) }
func PublicVerificationStream(buffer *EvidenceBuffer, secrets []string) string {
	return evidence.PublicStream(buffer, secrets)
}
