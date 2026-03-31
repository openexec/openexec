package loop

import (
	"context"
	"fmt"
	"io"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/openexec/openexec/internal/blueprint"
	"github.com/openexec/openexec/internal/types"
)

// Loop handles the iterative execution of a blueprint.
// It coordinates with the blueprint engine to execute stages and emit events.
type Loop struct {
	cfg     Config
	events  chan Event
	
	blueprintEngine *blueprint.Engine
	blueprintRun    *blueprint.Run
	
	iteration int
	paused    atomic.Bool
	stopped   atomic.Bool
	mu        sync.Mutex
}

// LoopHealth represents the current health of the execution loop.
type LoopHealth struct {
	Active       bool      `json:"active"`
	Iteration    int       `json:"iteration"`
	Status       string    `json:"status"`
	LastActivity time.Time `json:"last_activity"`
	CurrentPID   int       `json:"current_pid"`
}

// New creates a new execution Loop.
func New(cfg Config) (*Loop, <-chan Event) {
	ch := make(chan Event, 100)
	
	// Create engine from config if enabled
	var engine *blueprint.Engine
	if cfg.BlueprintEnabled {
		bp := cfg.Blueprint
		if bp == nil {
			bp = blueprint.DefaultBlueprint
		}
		
		engineCfg := blueprint.DefaultEngineConfig()
		if cfg.BlueprintCallbacks != nil {
			if cfg.BlueprintCallbacks.OnStageStart != nil {
				engineCfg.OnStageStart = func(run *blueprint.Run, stageName string) {
					stage, _ := bp.GetStage(stageName)
					cfg.BlueprintCallbacks.OnStageStart(run, stage)
				}
			}
			if cfg.BlueprintCallbacks.OnStageComplete != nil {
				engineCfg.OnStageComplete = cfg.BlueprintCallbacks.OnStageComplete
			}
			if cfg.BlueprintCallbacks.OnCheckpoint != nil {
				engineCfg.OnCheckpoint = cfg.BlueprintCallbacks.OnCheckpoint
			}
			if cfg.BlueprintCallbacks.OnRunComplete != nil {
				engineCfg.OnRunComplete = cfg.BlueprintCallbacks.OnRunComplete
			}
		}
		
		var err error
		engine, err = blueprint.NewEngine(bp, cfg.BlueprintExecutor, engineCfg)
		if err != nil {
			fmt.Printf("Error creating blueprint engine: %v\n", err)
		}
	}

	l := &Loop{
		cfg:             cfg,
		events:          ch,
		blueprintEngine: engine,
	}
	return l, ch
}

// Run starts the execution loop.
func (l *Loop) Run(ctx context.Context) error {
	defer close(l.events)

	// Orchestrator mode: full blueprint execution
	if l.blueprintEngine != nil {
		return l.runBlueprint(ctx)
	}

	// Agentic subloop mode: standalone bounded execution
	return l.runStandalone(ctx)
}

// runStandalone executes a bounded execution loop without a blueprint.
// This is used by agentic stages to run their inner reasoning loops.
func (l *Loop) runStandalone(ctx context.Context) error {
	// 1. Start process
	proc, err := StartProcess(ctx, l.cfg, nil, nil, nil)
	if err != nil {
		l.emit(Event{Type: EventError, ErrText: err.Error()})
		return err
	}
	defer func() { _ = proc.Kill() }()

	// 2. Execution loop (Process based)
	go func() {
		// Capture stderr in background for diagnostics
		_, _ = CaptureStderr(proc.Stderr, l.cfg.LogDir)
	}()

	// Setup parser to pipe events directly to our channel
	parser := NewParser(l.events, 1)
	
	// Start parsing in background
	parseErr := make(chan error, 1)
	go func() {
		parseErr <- parser.Parse(proc.Stdout)
	}()

	// Wait for either process exit or parsing completion
	select {
	case <-ctx.Done():
		return ctx.Err()
	case err := <-parseErr:
		if err != nil && err != io.EOF {
			return err
		}
	}

	return proc.Wait()
}

// runBlueprint executes the configured blueprint.
func (l *Loop) runBlueprint(ctx context.Context) error {
	bp := l.blueprintEngine.GetBlueprint()
	
	l.emit(Event{
		Type:        EventBlueprintStart,
		BlueprintID: bp.ID,
		Text:        fmt.Sprintf("Starting blueprint: %s", bp.Name),
	})

	// Start the run
	input := blueprint.NewStageInput(l.cfg.FwuID, l.cfg.VolatilePrompt, l.cfg.WorkDir)
	input.Briefing = l.cfg.VolatilePrompt
	
	run, err := l.blueprintEngine.StartRun(ctx, l.cfg.FwuID, input)
	if err != nil {
		return err
	}
	l.blueprintRun = run

	// Execution loop
	for run.CurrentStage != "complete" && run.CurrentStage != "" {
		if l.stopped.Load() {
			run.Cancel()
			return nil
		}
		
		for l.paused.Load() {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(1 * time.Second):
			}
		}

		stage, ok := bp.GetStage(run.CurrentStage)
		if !ok {
			err := fmt.Errorf("stage %q not found", run.CurrentStage)
			run.Fail(err.Error())
			return err
		}

		l.iteration++
		attempt := run.GetRetries(stage.Name) + 1

		l.emit(Event{
			Type:        EventStageStart,
			Iteration:   l.iteration,
			BlueprintID: bp.ID,
			StageName:   stage.Name,
			StageType:   string(stage.Type),
			Attempt:     attempt,
		})

		// Invoke stage start callback
		if l.cfg.BlueprintCallbacks != nil && l.cfg.BlueprintCallbacks.OnStageStart != nil {
			l.cfg.BlueprintCallbacks.OnStageStart(run, stage)
		}

		// Execute stage
		result, err := l.blueprintEngine.ExecuteStage(ctx, run, stage.Name, input)
		if err != nil {
			l.emit(Event{
				Type:    EventError,
				ErrText: fmt.Sprintf("execution error in stage %q: %v", stage.Name, err),
			})
			return err
		}

		// Invoke stage complete callback
		if l.cfg.BlueprintCallbacks != nil && l.cfg.BlueprintCallbacks.OnStageComplete != nil {
			l.cfg.BlueprintCallbacks.OnStageComplete(run, result)
		}

		// Handle result
		switch result.Status {
		case types.StageStatusCompleted:
			l.emit(Event{
				Type:        EventStageComplete,
				Iteration:   l.iteration,
				BlueprintID: bp.ID,
				StageName:   stage.Name,
				StageType:   string(stage.Type),
				Attempt:     attempt,
				Text:        fmt.Sprintf("Stage %q completed successfully", stage.Name),
				Artifacts:   result.Artifacts,
			})

			// Create checkpoint if configured
			if stage.CreateCheckpoint {
				run.AddCheckpoint()
				if l.cfg.BlueprintCallbacks != nil && l.cfg.BlueprintCallbacks.OnCheckpoint != nil {
					l.cfg.BlueprintCallbacks.OnCheckpoint(run, stage.Name)
				}
				l.emit(Event{
					Type:        EventCheckpointCreated,
					BlueprintID: bp.ID,
					StageName:   stage.Name,
					Text:        fmt.Sprintf("Checkpoint created at stage %q", stage.Name),
				})
			}

			// Move to next stage
			run.CurrentStage = stage.OnSuccess
		case types.StageStatusFailed:
			l.emit(Event{
				Type:        EventStageFailed,
				Iteration:   l.iteration,
				BlueprintID: bp.ID,
				StageName:   stage.Name,
				StageType:   string(stage.Type),
				Attempt:     attempt,
				ErrText:     result.Error,
				Text:        fmt.Sprintf("Stage %q failed: %s", stage.Name, result.Error),
			})

			// Check if we can retry
			if stage.OnFailure != "" && run.GetRetries(stage.Name) < stage.MaxRetries {
				run.IncrementRetries(stage.Name)
				l.emit(Event{
					Type:        EventStageRetry,
					BlueprintID: bp.ID,
					StageName:   stage.Name,
					Attempt:     run.GetRetries(stage.Name) + 1,
					Text:        fmt.Sprintf("Retrying stage %q (attempt %d/%d)", stage.Name, run.GetRetries(stage.Name)+1, stage.MaxRetries),
				})
				run.CurrentStage = stage.OnFailure
			} else {
				run.Fail(result.Error)
				l.emit(Event{
					Type:        EventBlueprintFailed,
					BlueprintID: bp.ID,
					StageName:   stage.Name,
					ErrText:     fmt.Sprintf("Stage %q failed after max retries: %s", stage.Name, result.Error),
				})
				return fmt.Errorf("stage %q failed: %s", stage.Name, result.Error)
			}
		}
	}

	// Blueprint completed successfully
	run.Complete()

	// Invoke run complete callback
	if l.cfg.BlueprintCallbacks != nil && l.cfg.BlueprintCallbacks.OnRunComplete != nil {
		l.cfg.BlueprintCallbacks.OnRunComplete(run)
	}

	l.emit(Event{
		Type:        EventBlueprintComplete,
		Iteration:   l.iteration,
		BlueprintID: bp.ID,
		Text:        fmt.Sprintf("Blueprint %q completed successfully", bp.Name),
	})

	// Build artifacts from all stage results
	artifacts := make(map[string]string)
	for _, result := range run.Results {
		for k, v := range result.Artifacts {
			artifacts[fmt.Sprintf("%s:%s", result.StageName, k)] = v
		}
	}

	// Emit EventComplete with Result for determinism
	l.emit(Event{
		Type:        EventComplete,
		Iteration:   l.iteration,
		BlueprintID: bp.ID,
		Result: &StepResult{
			Status:     "complete",
			Reason:     "blueprint_complete",
			NextPhase:  "done",
			Artifacts:  artifacts,
			Confidence: 1.0,
		},
	})
	return nil
}

// Stop terminates the loop.
func (l *Loop) Stop() {
	l.stopped.Store(true)
}

// Pause pauses the loop.
func (l *Loop) Pause() {
	l.paused.Store(true)
}

// Resume resumes the loop.
func (l *Loop) Resume() {
	l.paused.Store(false)
}

// GetHealth returns current loop health.
func (l *Loop) GetHealth() LoopHealth {
	l.mu.Lock()
	defer l.mu.Unlock()
	return LoopHealth{
		Active:       !l.stopped.Load() && !l.paused.Load(),
		Iteration:    l.iteration,
		Status:       "running",
		LastActivity: time.Now(),
		CurrentPID:   os.Getpid(),
	}
}

func (l *Loop) emit(e Event) {
	l.events <- e
}

// GetBlueprintRun returns the current blueprint run (if any).
func (l *Loop) GetBlueprintRun() *blueprint.Run {
	return l.blueprintRun
}

// GetBlueprintEngine returns the blueprint engine (if any).
func (l *Loop) GetBlueprintEngine() *blueprint.Engine {
	return l.blueprintEngine
}
