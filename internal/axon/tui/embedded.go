package axontui

import (
	"github.com/openexec/openexec/internal/loop"
	"github.com/openexec/openexec/internal/manager"
)

// EmbeddedSource wraps a *manager.Manager directly.
// Used when the TUI starts with --workdir (embedded mode).
type EmbeddedSource struct {
	mgr *manager.Manager
}

// NewEmbeddedSource creates an EmbeddedSource wrapping the given Manager.
func NewEmbeddedSource(mgr *manager.Manager) *EmbeddedSource {
	return &EmbeddedSource{mgr: mgr}
}

func (s *EmbeddedSource) List() ([]manager.PipelineInfo, error) {
	return s.mgr.List(), nil
}

func (s *EmbeddedSource) Status(fwuID string) (manager.PipelineInfo, error) {
	return s.mgr.Status(fwuID)
}

func (s *EmbeddedSource) Subscribe(fwuID string) (<-chan loop.Event, func(), error) {
	return s.mgr.Subscribe(fwuID)
}

func (s *EmbeddedSource) Pause(fwuID string) error {
	return s.mgr.Pause(fwuID)
}

func (s *EmbeddedSource) Stop(fwuID string) error {
	return s.mgr.Stop(fwuID)
}
