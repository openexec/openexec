package loop

import (
	"context"
	"time"

	"github.com/openexec/openexec/internal/blueprint"
	"github.com/openexec/openexec/internal/summarize"
	"github.com/openexec/openexec/pkg/agent"
)

// Uploader defines the interface for uploading session artifacts.
type Uploader interface {
	UploadSession(ctx context.Context, localDir, fwuID, timestamp string) error
}

// UploaderFactory creates a new Uploader instance.
type UploaderFactory func(ctx context.Context, cfg UploaderConfig) (Uploader, error)

// UploaderConfig holds S3 configuration for evidence uploads.
type UploaderConfig struct {
	Bucket   string
	Region   string
	Endpoint string
	Prefix   string
}

// Config controls loop behavior.
type Config struct {
	// UploaderFactory creates the evidence uploader.
	// If nil, no uploader is used.
	UploaderFactory UploaderFactory `json:"-"`

    // Prompt is the system prompt passed to Claude Code via -p flag.
    Prompt string

    // StablePrompt is the cached, byte-identical stable prefix for provider caching.
    StablePrompt string

    // VolatilePrompt is the dynamic tail (e.g., briefing) appended after the stable prefix.
    VolatilePrompt string

	// WorkDir is the working directory for the Claude Code process.
	WorkDir string

	// ReviewerModel is the model to use for code review (optional).
	// If set, a secondary Claude instance will review the output.
	ReviewerModel string

	// TaskID is the unique identifier for this task (optional).
	TaskID string

	// MaxIterations is the safety limit. 0 means no limit.
	MaxIterations int

	// MaxRetries is the number of retry attempts on non-zero exit. Default 3.
	MaxRetries int

	// RetryBackoff is the sequence of delays between retries. Default [0s, 5s, 15s].
	RetryBackoff []time.Duration

	// LogDir is the directory for stderr log files. Default: WorkDir.
	LogDir string

	// CommandName overrides the binary to execute (default "claude").
	// Used by tests to inject a mock command.
	CommandName string

	// CommandArgs overrides the argument list (default built from Prompt).
	// Used by tests to inject a mock command.
	CommandArgs []string

	// MCPConfigPath is the path to the MCP config JSON file.
	// When set, --mcp-config is added to the Claude Code command.
	MCPConfigPath string

	// ThrashThreshold is the number of iterations without a progress signal
	// before emitting ThrashingDetected. Default 3. 0 disables.
	ThrashThreshold int

	// FwuID is the firmware update ID for this session (used for evidence).
	FwuID string

	// EvidenceDir is the base directory for storing session evidence.
	// If empty, evidence capturing is disabled.
	EvidenceDir string

	// EvidenceBucket is the S3 bucket for evidence uploads.
	EvidenceBucket string

	// EvidenceRegion is the AWS region for the bucket.
	EvidenceRegion string

	// EvidenceEndpoint is the custom S3 endpoint URL.
	EvidenceEndpoint string

	// EvidencePrefix is the key prefix for uploaded files.
	EvidencePrefix string

    // DeepTraceCfg configures the Deep-Trace middleware for ISO 27001 compliance.
    // If nil, middleware is disabled.
    DeepTraceCfg *DeepTraceConfig

    // QualityGates enables quality gate validation after task completion.
	// When enabled, gates from openexec.yaml are run after each phase-complete signal.
	QualityGates bool

	// GateTimeout is the timeout for running quality gates. Default 5 minutes.
	GateTimeout time.Duration

	// MaxGateRetries is the number of times to let executor fix gate failures. Default 3.
	MaxGateRetries int

	// PreflightChecks enables preflight validation before task starts.
	PreflightChecks bool

	// TaskTitle is the task title (used for preflight check detection).
	TaskTitle string

	// ExecutorModel is the model name (e.g. gemini-...) used for centralized runner resolution.
	ExecutorModel string

	// RunnerCommand optionally overrides the CLI binary.
	RunnerCommand string

	// RunnerArgs optionally overrides the CLI arguments.
	RunnerArgs []string

    // Summarizer is the session history summarizer (optional).
    // If nil, no summarization is performed.
    Summarizer Summarizer `json:"-"`

    // ExecMode controls write permissions for the spawned process.
    // Accepted: "read-only", "workspace-write", "danger-full-access".
    // Propagated via environment variables to the runner.
    ExecMode string

    // PromptHash is the SHA-256 (hex) of the composed prompt for observability.
    PromptHash string

    // StablePromptHash is the SHA-256 (hex) of the stable prefix for cache keying.
    StablePromptHash string

    // RunSpecID links to the immutable RunSpec for deterministic replay.
    // When set, this run can be resumed or replayed using the same inputs.
    RunSpecID string

    // ContextCachePath is the directory for caching gathered context bundles.
    // If empty, context caching is disabled.
    // Typically set to .openexec/cache/context.
    ContextCachePath string

    // ResumeFromCheckpoint enables resuming from a specific checkpoint.
    // When set, the loop will:
    // 1. Restore message history from the checkpoint
    // 2. Skip tool calls with existing idempotency keys
    // 3. Continue from the checkpoint's phase and iteration
    ResumeFromCheckpoint *ResumeConfig

    // StallDetection configures stall detection with provider timeouts and backoff.
    // If nil, default stall detection settings are used.
    StallDetection *StallConfig

    // BlueprintEnabled enables blueprint-driven execution mode.
    // When enabled, the loop uses blueprint stages instead of linear iteration.
    BlueprintEnabled bool

    // Blueprint is the blueprint to execute.
    // If BlueprintEnabled is true and this is nil, DefaultBlueprint is used.
    Blueprint *blueprint.Blueprint

    // BlueprintExecutor is the stage executor for blueprint execution.
    // Required when BlueprintEnabled is true.
    BlueprintExecutor blueprint.StageExecutor `json:"-"`

    // BlueprintCallbacks contains callbacks for blueprint events.
    BlueprintCallbacks *BlueprintCallbacks `json:"-"`
}

// BlueprintCallbacks contains callbacks for blueprint stage events.
type BlueprintCallbacks struct {
    // OnStageStart is called when a stage begins.
    OnStageStart func(run *blueprint.Run, stage *blueprint.Stage)

    // OnStageComplete is called when a stage completes.
    OnStageComplete func(run *blueprint.Run, result *blueprint.StageResult)

    // OnCheckpoint is called when a checkpoint is created.
    OnCheckpoint func(run *blueprint.Run, stageName string)

    // OnRunComplete is called when the run completes.
    OnRunComplete func(run *blueprint.Run)
}

// ResumeConfig holds configuration for resuming from a checkpoint.
type ResumeConfig struct {
    CheckpointID     string   // ID of the checkpoint to resume from
    MessageHistory   []byte   // JSON-encoded message history
    AppliedToolCalls []string // Idempotency keys of already-applied tool calls
    Phase            string   // Phase to resume from
    Iteration        int      // Iteration to resume from
}

// Summarizer defines the interface for session history summarization.
type Summarizer interface {
	ShouldSummarize(messages []agent.Message, contextLimit int) *summarize.TriggerCheckResult
	Summarize(ctx context.Context, sessionID string, messages []agent.Message, reason summarize.TriggerReason) (*summarize.SummaryResult, error)
	BuildContextWithSummary(ctx context.Context, sessionID string, recentMessages []agent.Message) ([]agent.Message, error)
}

// summarize.TriggerCheckResult must be imported for the interface to work.
// I'll check imports in loop/config.go.

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() Config {
	stallConfig := DefaultStallConfig()
	return Config{
		MaxIterations:   10,
		MaxRetries:      3,
		RetryBackoff:    []time.Duration{0, 5 * time.Second, 15 * time.Second},
		ThrashThreshold: 3,
		DeepTraceCfg:    nil,
		QualityGates:    true,
		GateTimeout:     5 * time.Minute,
		MaxGateRetries:  3,
		PreflightChecks: true,
		StallDetection:  &stallConfig,
	}
}
