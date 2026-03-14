package pipeline

import (
    "context"
    "fmt"
    "io/fs"
    "log"
    "crypto/sha256"
    "encoding/hex"
    "os"
    "path/filepath"
    "strings"
    "time"

	"github.com/openexec/openexec/internal/loop"
	"github.com/openexec/openexec/internal/prompt"
	"github.com/openexec/openexec/internal/release"
)

// LoopFactoryConfig holds shared settings for creating Loops across pipeline phases.
type LoopFactoryConfig struct {
	FWUID                string
	WorkDir              string
	TractStore           string
	AgentsFS             fs.FS
	ReleaseManager       *release.Manager
	MCPConfigPath        string
	DefaultMaxIterations int
	MaxRetries           int
	MaxReviewCycles      int
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

    // ExecMode propagates execution mode to the loop.
    ExecMode string
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

// TractBriefingFunc returns a BriefingFunc that uses the built-in release manager.
func TractBriefingFunc(mgr *release.Manager) BriefingFunc {
	return func(ctx context.Context, fwuID string) (string, error) {
		if mgr == nil {
			return fmt.Sprintf("## FWU Briefing: %s\n\n**Status:** in_progress\n", fwuID), nil
		}

		brief, err := mgr.Brief(fwuID)
		if err != nil {
			msg := fmt.Sprintf("## FWU Briefing: %s\n\n**Status:** in_progress\n", fwuID)
			
			// Detect if this is likely a doc-only task from the ID or context
			if strings.Contains(strings.ToLower(fwuID), "study") || strings.Contains(strings.ToLower(fwuID), "mapping") {
				msg += "\n**MANDATE:** This is a documentation/analysis task. DO NOT attempt to compile code or run tests. Focus on mapping and documenting boundaries.\n"
			}

			log.Printf("[Briefing] built-in tract briefing failed for %s, using minimal briefing: %v", fwuID, err)
			return msg, nil
		}

		return prompt.FormatBriefing(brief), nil
	}
}

// Create builds a Loop for the given phase configuration.
// briefing is pre-formatted text (from BriefingFunc).
func (f *LoopFactory) Create(briefing string, phaseCfg PhaseConfig) (*loop.Loop, <-chan loop.Event, error) {
    stable, volatile, composed, err := f.assembler.ComposeParts(phaseCfg.Agent, phaseCfg.Workflow, briefing)
    if err != nil {
        return nil, nil, fmt.Errorf("compose prompt for %s/%s: %w", phaseCfg.Agent, phaseCfg.Workflow, err)
    }
    // Observability: compute prompt hash
    sum := sha256.Sum256([]byte(composed))
    promptHash := hex.EncodeToString(sum[:])
    // Stable prefix hash
    ssum := sha256.Sum256([]byte(stable))
    stableHash := hex.EncodeToString(ssum[:])
    log.Printf("[Prompt] hash=%s agent=%s workflow=%s", promptHash, phaseCfg.Agent, phaseCfg.Workflow)
    // Persist stable prompt block as artifact for reuse
    if f.cfg.WorkDir != "" {
        dir := filepath.Join(f.cfg.WorkDir, ".openexec", "artifacts", "prompts")
        _ = os.MkdirAll(dir, 0o755)
        _ = os.WriteFile(filepath.Join(dir, stableHash+".txt"), []byte(stable), 0o644)
    }

	maxIter := f.cfg.DefaultMaxIterations
	if phaseCfg.MaxIterations > 0 {
		maxIter = phaseCfg.MaxIterations
	}

    cfg := loop.Config{
        Prompt:           composed,
        StablePrompt:     stable,
        VolatilePrompt:   volatile,
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
        ExecMode:         f.cfg.ExecMode,
        PromptHash:       promptHash,
        StablePromptHash: stableHash,
    }
    // Attach prompt hash to title suffix (non-invasive propagation)
    if cfg.TaskTitle == "" {
        cfg.TaskTitle = fmt.Sprintf("%s#%s", phaseCfg.Workflow, promptHash[:8])
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
