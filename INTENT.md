# Intent: openexec

## Goals
- Fix the intent routing failure in openexec chat where all inputs fail with 'model could not determine intent with high confidence'
### G-001: Fix the intent routing failure in openexec chat model where all inputs fail with 'model could not determine intent with high confidence'
- Success Criteria: User inputs are successfully routed to appropriate handlers without the low confidence error
- Verification: Run 'openexec chat' and send test queries; verify they receive valid responses from general_chat tool
- Global Success Metric: 

## Requirements
### REQ-001: Core Architecture
- Shape: cli
- Platforms: macos, linux, windows

### REQ-002: Data Source Mapping
- BitNetRouter: Source of Truth: internal/router/bitnet.go
- Coordinator: Source of Truth: internal/dcp/coordinator.go
- GeneralChatTool: Source of Truth: internal/tools/chat.go

## Constraints
- C-001: Router must always return an Intent (never error) for graceful degradation
- C-002: Confidence threshold is 0.2 - below this, fallback to general_chat
- C-003: skipAvailabilityCheck must be true for simulateInference to be used
