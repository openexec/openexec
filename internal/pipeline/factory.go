package pipeline

import (
	"context"
	"fmt"
	"time"

	"github.com/openexec/openexec/internal/loop"
	"github.com/openexec/openexec/internal/prompt"
	"github.com/openexec/openexec/internal/tract"
)

// LoopFactoryConfig holds shared settings for creating Loops across pipeline phases.
type LoopFactoryConfig struct {
	FWUID                string
	WorkDir              string
	TractStore           string
	AgentsDir            string
	MCPConfigPath        string
	DefaultMaxIterations int
	MaxRetries           int
	RetryBackoff         []time.Duration
	ThrashThreshold      int
	CommandName          string   // test override
	CommandArgs          []string // test override (default for all phases)

	EvidenceDir      string
	EvidenceBucket   string
	EvidenceRegion   string
	EvidenceEndpoint string
	EvidencePrefix   string
}

// LoopFactory creates configured Loops for pipeline phases.
type LoopFactory struct {
	cfg       LoopFactoryConfig
	assembler *prompt.Assembler
}

// NewLoopFactory creates a factory using the given config.
func NewLoopFactory(cfg LoopFactoryConfig) *LoopFactory {
	return &LoopFactory{
		cfg:       cfg,
		assembler: prompt.NewAssembler(cfg.AgentsDir),
	}
}

// BriefingFunc fetches briefing text for an FWU. Abstracted for testing.
type BriefingFunc func(ctx context.Context, fwuID string) (string, error)

// TractBriefingFunc returns a BriefingFunc that uses the real TractClient.
func TractBriefingFunc(tractStore string) BriefingFunc {
	return func(ctx context.Context, fwuID string) (string, error) {
		client, err := tract.StartSubprocess(ctx, tractStore)
		if err != nil {
			return "", fmt.Errorf("start tract: %w", err)
		}
		defer func() { _ = client.Close() }()

		brief, err := client.Brief(fwuID)
		if err != nil {
			return "", fmt.Errorf("fetch briefing: %w", err)
		}

		return prompt.FormatBriefing(brief), nil
	}
}

// Create builds a Loop for the given phase configuration.
// briefing is pre-formatted text (from BriefingFunc).
func (f *LoopFactory) Create(briefing string, phaseCfg PhaseConfig) (*loop.Loop, <-chan loop.Event, error) {
	composed, err := f.assembler.Compose(phaseCfg.Agent, phaseCfg.Workflow, briefing)
	if err != nil {
		return nil, nil, fmt.Errorf("compose prompt for %s/%s: %w", phaseCfg.Agent, phaseCfg.Workflow, err)
	}

	maxIter := f.cfg.DefaultMaxIterations
	if phaseCfg.MaxIterations > 0 {
		maxIter = phaseCfg.MaxIterations
	}

	cfg := loop.Config{
		Prompt:           composed,
		WorkDir:          f.cfg.WorkDir,
		MaxIterations:    maxIter,
		MaxRetries:       f.cfg.MaxRetries,
		RetryBackoff:     f.cfg.RetryBackoff,
		MCPConfigPath:    f.cfg.MCPConfigPath,
		ThrashThreshold:  f.cfg.ThrashThreshold,
		FwuID:            f.cfg.FWUID,
		EvidenceDir:      f.cfg.EvidenceDir,
		EvidenceBucket:   f.cfg.EvidenceBucket,
		EvidenceRegion:   f.cfg.EvidenceRegion,
		EvidenceEndpoint: f.cfg.EvidenceEndpoint,
		EvidencePrefix:   f.cfg.EvidencePrefix,
	}

	// Apply command overrides: phase-specific takes precedence over factory default.
	if phaseCfg.CommandArgs != nil {
		cfg.CommandName = f.cfg.CommandName
		cfg.CommandArgs = phaseCfg.CommandArgs
	} else if f.cfg.CommandArgs != nil {
		cfg.CommandName = f.cfg.CommandName
		cfg.CommandArgs = f.cfg.CommandArgs
	}

	l, ch := loop.New(cfg)
	return l, ch, nil
}
