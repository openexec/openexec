# Intent: openexec

## Goals
- The OpenExec orchestrator is experiencing systemic failures in its core workflows: the Wizard/Intent planning tool is broken, story import/execution fails, self-healing loops are unstable, and the CI/CD pipeline is red.
### G-001: Fix the 'openexec wizard' and intent planning logic so intent.md files can be generated and stories can be imported.
- Success Criteria: Wizard runs successfully, generates a valid intent.md, and stories are imported without failure.
- Verification: 
### G-002: Fix the regression in self-healing loops and ensure all existing CI/CD tests pass.
- Success Criteria: CI/CD pipeline returns to green and agent loops successfully recover from simulated errors.
- Verification: 
### G-003: Address the low-confidence routing failure where chat inputs fail to reach handlers.
- Success Criteria: Chat inputs are correctly routed to general_chat or appropriate tools.
- Verification: 
- Global Success Metric: 

## Requirements
### REQ-001: Core Architecture
- Shape: cli
- Platforms: macos, linux, windows

### REQ-002: Data Source Mapping
- WizardEngine: Source of Truth: internal/wizard/
- StoryManager: Source of Truth: internal/stories/
- SelfHealingLoop: Source of Truth: internal/dcp/healing.go
- BitNetRouter: Source of Truth: internal/router/bitnet.go

## Constraints
- C-001: Must restore compatibility with existing story and intent.md schemas.
- C-002: CI/CD must pass fully before any feature work is considered complete.
- C-003: Self-healing must not enter infinite loops (exit strategy required).
