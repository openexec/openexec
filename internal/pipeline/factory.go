package pipeline

import (
	"context"
	"fmt"
	"io/fs"
	"log"
	"strings"
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
	AgentsFS             fs.FS
	MCPConfigPath        string
	DefaultMaxIterations int
	MaxRetries           int
	RetryBackoff         []time.Duration
	ThrashThreshold      int
	ExecutorModel        string   // used for runner resolution
	RunnerCommand        string   // explicit CLI override
	RunnerArgs           []string // explicit CLI args override
	CommandName          string   // test override
	CommandArgs          []string // test override (default for all phases)

	LogDir           string
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
		assembler: prompt.NewAssembler(cfg.AgentsFS),
	}
}

// BriefingFunc fetches briefing text for an FWU. Abstracted for testing.
type BriefingFunc func(ctx context.Context, fwuID string) (string, error)

// TractBriefingFunc returns a BriefingFunc that uses the real TractClient.
// Falls back to a minimal briefing if tract is unavailable or the FWU is not found.
func TractBriefingFunc(tractStore string) BriefingFunc {
	return func(ctx context.Context, fwuID string) (string, error) {
		client, err := tract.StartSubprocess(ctx, tractStore)
		if err != nil {
			msg := fmt.Sprintf("## FWU Briefing: %s\n\n**Status:** in_progress\n", fwuID)
			
			// Detect if this is likely a doc-only task from the ID or context
			// (Future: pass more metadata to briefingFn)
			if strings.Contains(strings.ToLower(fwuID), "study") || strings.Contains(strings.ToLower(fwuID), "mapping") {
				msg += "\n**MANDATE:** This is a documentation/analysis task. DO NOT attempt to compile code or run tests. Focus on mapping and documenting boundaries.\n"
			}

			if strings.Contains(err.Error(), "executable file not found") {
				log.Printf("[Briefing] tract binary not in path, using minimal briefing for %s", fwuID)
			} else {
				log.Printf("[Briefing] tract unavailable for %s, using minimal briefing: %v", fwuID, err)
			}
			return msg, nil
		}
		defer func() { _ = client.Close() }()

		brief, err := client.Brief(fwuID)
		if err != nil {
			// Catch read response errors (like unexpected EOF) and other protocol failures
			log.Printf("[Briefing] tract briefing failed for %s, using minimal briefing: %v", fwuID, err)
			return fmt.Sprintf("## FWU Briefing: %s\n\n**Status:** in_progress\n", fwuID), nil
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
		ExecutorModel:    f.cfg.ExecutorModel,
		RunnerCommand:    f.cfg.RunnerCommand,
		RunnerArgs:       f.cfg.RunnerArgs,
		MaxIterations:    maxIter,
		MaxRetries:       f.cfg.MaxRetries,
		RetryBackoff:     f.cfg.RetryBackoff,
		MCPConfigPath:    f.cfg.MCPConfigPath,
		ThrashThreshold:  f.cfg.ThrashThreshold,
		FwuID:            f.cfg.FWUID,
		LogDir:           f.cfg.LogDir,
		EvidenceDir:      f.cfg.EvidenceDir,
		EvidenceBucket:   f.cfg.EvidenceBucket,
		EvidenceRegion:   f.cfg.EvidenceRegion,
		EvidenceEndpoint: f.cfg.EvidenceEndpoint,
		EvidencePrefix:   f.cfg.EvidencePrefix,
	}

	// Apply command overrides: phase-specific takes precedence over factory default.
	cfg.CommandName = f.cfg.CommandName
	if phaseCfg.CommandArgs != nil {
		cfg.CommandArgs = phaseCfg.CommandArgs
	} else if f.cfg.CommandArgs != nil {
		cfg.CommandArgs = f.cfg.CommandArgs
	}

	l, ch := loop.New(cfg)
	return l, ch, nil
}
