## Blueprint executor location
File: internal/pipeline/pipeline.go
Function: runBlueprintMode
Current behaviour: It initializes the blueprint.DefaultBlueprint, configures an engine.Config with callbacks for stage events, and then calls engine.Execute. It does not currently have any logic to invoke quality gates between stages.

## Quality gate runner location
File: internal/execution/gates/runner.go
Function/type: Runner
Signature: func (r *Runner) RunAll(ctx context.Context) *GateReport

## How loop mode invokes gates
File: internal/loop/gates_integration.go
Lines: 75
Pattern: report := runner.RunAll(ctx)

## Dependency gap
Does the blueprint executor struct currently have a field for the gate runner? no
If no: what field name and type needs to be added? Field 'gateRunner' of type 'GateRunner' (interface) to both Pipeline struct and possibly passed down to blueprint.Engine or its callbacks.

## Proposed wiring — three sentences maximum
Add a 'GateRunner' interface to the internal/blueprint package and a corresponding field to the EngineConfig struct. In Pipeline.runBlueprintMode, initialize a gates.Runner and inject it into the engineConfig.OnStageComplete callback so that quality gates are executed and failures are propagated after every blueprint stage.
