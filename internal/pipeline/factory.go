package pipeline

import (
	"io/fs"
	"time"

	"github.com/openexec/openexec/internal/prompt"
	"github.com/openexec/openexec/internal/release"
)

// LoopFactoryConfig holds shared settings for creating Loops across blueprint stages.
type LoopFactoryConfig struct {
	FWUID                string
	WorkDir              string
	AgentsFS             fs.FS
	ReleaseManager       *release.Manager
	MCPConfigPath        string
	DefaultMaxIterations int
	MaxRetries           int
	RetryBackoff         []time.Duration
	ThrashThreshold      int
	ExecutorModel        string   // used for runner resolution
	RunnerCommand        string   // explicit CLI override
	RunnerArgs           []string // explicit CLI args override
	CommandName          string   // test override
	CommandArgs          []string // test override (default for all stages)

	LogDir           string
	EvidenceDir      string
	EvidenceBucket   string
	EvidenceRegion   string
	EvidenceEndpoint string
	EvidencePrefix   string

	// ExecMode propagates execution mode to the loop.
	ExecMode string

	// ReviewEnabled controls whether the review stage runs.
	ReviewEnabled bool
	// ReviewerModel overrides the executor model for the review stage.
	ReviewerModel string

	// BlueprintID enables blueprint mode with the specified blueprint.
	BlueprintID string

	// TaskDescription is the user's task description for blueprint runs.
	TaskDescription string

	// TaskTimeout overrides the default implement stage timeout.
	// If zero, the blueprint default (10 minutes) is used.
	TaskTimeout time.Duration
}

// LoopFactory creates configured Loops for blueprint stage execution.
type LoopFactory struct {
	cfg       LoopFactoryConfig
	assembler *prompt.Assembler
}

// NewLoopFactory creates a factory using the given config.
func NewLoopFactory(cfg LoopFactoryConfig) *LoopFactory {
	return &LoopFactory{
		cfg:       cfg,
		assembler: prompt.NewAssembler(cfg.AgentsFS),
	}
}
