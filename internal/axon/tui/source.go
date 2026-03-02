package axontui

import (
	"github.com/openexec/openexec/internal/loop"
	"github.com/openexec/openexec/pkg/manager"
)

// Source abstracts pipeline data access for the TUI.
// Implemented by EmbeddedSource (wraps Manager) and RemoteSource (HTTP+SSE).
type Source interface {
	List() ([]manager.PipelineInfo, error)
	Status(fwuID string) (manager.PipelineInfo, error)
	Subscribe(fwuID string) (<-chan loop.Event, func(), error)
	Pause(fwuID string) error
	Stop(fwuID string) error
}
