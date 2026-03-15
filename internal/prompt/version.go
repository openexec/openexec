// Package prompt provides prompt templates and versioning for AI agent interactions.
package prompt

// PromptVersion is the semantic version of the prompt templates.
// Increment this when prompt templates change in ways that affect behavior.
// Format: MAJOR.MINOR.PATCH
//   - MAJOR: Breaking changes to prompt structure or expected outputs
//   - MINOR: New capabilities or non-breaking additions
//   - PATCH: Bug fixes, typo corrections, clarifications
const PromptVersion = "1.0.0"

// RunStateMachineVersion is the version of the run state machine logic.
// Increment when the stage transitions or completion conditions change.
const RunStateMachineVersion = "1.0.0"
