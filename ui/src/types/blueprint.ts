/**
 * Blueprint Types for OpenExec Orchestrator
 *
 * These types mirror the Go backend types from:
 * - internal/blueprint/stage.go (Stage, StageStatus, StageType)
 * - internal/loop/event.go (Blueprint events)
 * - pkg/api/handlers.go (Approval endpoints)
 *
 * @module types/blueprint
 */

// =============================================================================
// Stage Types
// =============================================================================

/**
 * Stage execution status
 */
export type StageStatus = 'pending' | 'running' | 'completed' | 'failed' | 'skipped'

/**
 * Stage execution type
 */
export type StageType = 'deterministic' | 'agentic'

/**
 * Blueprint stage names in execution order
 */
export type StageName =
  | 'gather_context'
  | 'implement'
  | 'lint'
  | 'test'
  | 'review'

/**
 * Stage definition for UI display
 */
export interface Stage {
  /** Stage name identifier */
  name: StageName
  /** Human-readable stage description */
  description: string
  /** Stage execution type */
  type: StageType
  /** Current execution status */
  status: StageStatus
  /** When the stage started */
  startedAt?: string
  /** When the stage completed */
  completedAt?: string
  /** Duration in milliseconds */
  durationMs?: number
  /** Current retry attempt (1-based) */
  attempt?: number
  /** Maximum allowed retries */
  maxRetries?: number
  /** Error message if failed */
  error?: string
  /** Stage output or artifacts */
  output?: string
  /** Artifacts produced by this stage */
  artifacts?: Record<string, string>
}

/**
 * Blueprint execution state
 */
export interface BlueprintState {
  /** Unique blueprint execution ID */
  id: string
  /** Blueprint definition ID */
  blueprintId: string
  /** Task description being executed */
  taskDescription: string
  /** Current stage being executed */
  currentStage?: StageName
  /** All stages with their status */
  stages: Stage[]
  /** Overall execution status */
  status: 'pending' | 'running' | 'completed' | 'failed'
  /** When execution started */
  startedAt?: string
  /** When execution completed */
  completedAt?: string
  /** Total duration in milliseconds */
  totalDurationMs?: number
  /** Error message if failed */
  error?: string
}

// =============================================================================
// Approval Types
// =============================================================================

/**
 * Approval request status
 */
export type ApprovalRequestStatus = 'pending' | 'approved' | 'rejected'

/**
 * Risk level for tool execution
 */
export type RiskLevel = 'low' | 'medium' | 'high'

/**
 * Approval request from the orchestrator
 */
export interface Approval {
  /** Unique identifier for the approval request */
  id: string
  /** Run ID this approval is associated with */
  runId: string
  /** Name of the tool requesting approval */
  toolName: string
  /** Tool arguments */
  toolArgs?: Record<string, unknown>
  /** Human-readable description of the action */
  description: string
  /** Risk level of the action */
  riskLevel: RiskLevel
  /** Current approval status */
  status: ApprovalRequestStatus
  /** When the request was created */
  createdAt: string
  /** When the request was resolved */
  resolvedAt?: string
  /** Who resolved the request */
  resolvedBy?: string
  /** Rejection reason if rejected */
  rejectReason?: string
}

/**
 * Approval action parameters
 */
export interface ApprovalAction {
  /** Approval request ID */
  id: string
  /** Whether to approve (true) or reject (false) */
  approved: boolean
  /** Who is making the decision */
  decidedBy: string
  /** Optional reason for the decision */
  reason?: string
}

// =============================================================================
// WebSocket Event Types for Blueprint Execution
// =============================================================================

/**
 * Blueprint WebSocket event types
 */
export type BlueprintEventType =
  | 'blueprint_start'
  | 'blueprint_complete'
  | 'blueprint_failed'
  | 'stage_start'
  | 'stage_complete'
  | 'stage_failed'
  | 'stage_retry'
  | 'checkpoint_created'

/**
 * Blueprint event payload
 */
export interface BlueprintEvent {
  /** Event type */
  type: BlueprintEventType
  /** Blueprint execution ID */
  blueprintId: string
  /** Stage name (if stage event) */
  stageName?: StageName
  /** Stage type */
  stageType?: StageType
  /** Current attempt number */
  attempt?: number
  /** Event timestamp */
  timestamp: string
  /** Error message if applicable */
  error?: string
  /** Event metadata */
  metadata?: Record<string, unknown>
}

// =============================================================================
// Constants
// =============================================================================

/**
 * Standard blueprint stages in execution order
 */
export const BLUEPRINT_STAGES: readonly StageName[] = [
  'gather_context',
  'implement',
  'lint',
  'test',
  'review',
] as const

/**
 * Stage display information
 */
export const STAGE_INFO: Record<StageName, { label: string; description: string; type: StageType }> = {
  gather_context: {
    label: 'Gather Context',
    description: 'Collect relevant files and context for the task',
    type: 'deterministic',
  },
  implement: {
    label: 'Implement',
    description: 'Implement the requested changes',
    type: 'agentic',
  },
  lint: {
    label: 'Lint',
    description: 'Run linting checks on the changes',
    type: 'deterministic',
  },
  test: {
    label: 'Test',
    description: 'Run tests to verify the changes',
    type: 'deterministic',
  },
  review: {
    label: 'Review',
    description: 'Review changes and generate summary',
    type: 'agentic',
  },
}

/**
 * Stage status display configuration
 */
export const STAGE_STATUS_INFO: Record<StageStatus, { label: string; color: string }> = {
  pending: { label: 'Pending', color: '#8b949e' },
  running: { label: 'Running', color: '#58a6ff' },
  completed: { label: 'Completed', color: '#238636' },
  failed: { label: 'Failed', color: '#da3633' },
  skipped: { label: 'Skipped', color: '#6e7681' },
}

/**
 * Risk level display configuration
 */
export const RISK_LEVEL_INFO: Record<RiskLevel, { label: string; color: string }> = {
  low: { label: 'Low', color: '#238636' },
  medium: { label: 'Medium', color: '#f0883e' },
  high: { label: 'High', color: '#da3633' },
}
