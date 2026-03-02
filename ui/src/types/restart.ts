/**
 * Restart Types for OpenExec Orchestrator
 *
 * These types mirror the Go backend types from:
 * - internal/mcp/restart.go (RestartRequest, RestartStatus, etc.)
 * - internal/mcp/session_resume.go (SessionResumeState, etc.)
 *
 * @module types/restart
 */

// =============================================================================
// Restart Status Types
// =============================================================================

/**
 * Restart request lifecycle status
 */
export type RestartStatus =
  | 'pending'
  | 'approved'
  | 'rejected'
  | 'in_progress'
  | 'complete'
  | 'failed'
  | 'cancelled'

/**
 * Restart reason categories
 */
export type RestartReason =
  | 'code_change'
  | 'config_change'
  | 'user_requested'
  | 'recovery'
  | 'upgrade'

/**
 * Session resume status
 */
export type SessionResumeStatus =
  | 'pending'
  | 'resuming'
  | 'resumed'
  | 'failed'
  | 'expired'

// =============================================================================
// Restart Request Types
// =============================================================================

/**
 * Build result from orchestrator rebuild
 */
export interface BuildResult {
  success: boolean
  output: string
  errors: string[]
  warnings: string[]
  duration: number
  buildTime?: string
}

/**
 * Restart request from the orchestrator
 */
export interface RestartRequest {
  /** Unique identifier for the request (UUID) */
  id: string
  /** Reason for the restart */
  reason: RestartReason
  /** Additional description/context */
  description?: string
  /** Who/what requested the restart */
  requestedBy: string
  /** Session that originated this request */
  sessionId?: string
  /** Linked approval ID */
  approvalId?: string
  /** Current status */
  status: RestartStatus
  /** Whether build is required before restart */
  buildRequired: boolean
  /** Build result if a build was performed */
  buildResult?: BuildResult
  /** Server port to restart on */
  port: number
  /** When the request was created */
  createdAt: string
  /** When the request was last modified */
  updatedAt: string
  /** When the request was approved */
  approvedAt?: string
  /** When the restart completed */
  completedAt?: string
  /** Whether session resume is enabled */
  resumeEnabled: boolean
  /** Resume state ID if resume is enabled */
  resumeStateId?: string
  /** Whether to auto-resume on startup */
  resumeOnStartup: boolean
}

/**
 * Result of a restart operation
 */
export interface RestartResult {
  success: boolean
  requestId: string
  duration: number
  buildResult?: BuildResult
  errorMessage?: string
  output?: string
  newPid?: number
  resumeStateId?: string
  sessionPersisted: boolean
}

/**
 * Pre-flight check for restart
 */
export interface PreflightCheck {
  name: string
  description: string
  passed: boolean
  message?: string
  critical: boolean
}

/**
 * Pre-flight check result
 */
export interface PreflightResult {
  allPassed: boolean
  checks: PreflightCheck[]
  errors?: string[]
}

// =============================================================================
// Session Resume Types
// =============================================================================

/**
 * Session resume state for continuing after restart
 */
export interface SessionResumeState {
  id: string
  sessionId: string
  restartRequestId?: string
  status: SessionResumeStatus
  iteration: number
  totalTokens: number
  totalCostUsd: number
  messageCount: number
  iterationsSinceProgress: number
  model: string
  systemPrompt?: string
  projectPath?: string
  workDir?: string
  pendingPrompt?: string
  contextSummary?: string
  metadata?: Record<string, string>
  expiresAt: string
  createdAt: string
  updatedAt: string
  resumedAt?: string
  error?: string
}

/**
 * Auto-resume result
 */
export interface AutoResumeResult {
  hasPendingResume: boolean
  pendingStates?: SessionResumeState[]
  resumedStateId?: string
  resumedState?: SessionResumeState
  error?: string
}

// =============================================================================
// UI State Types
// =============================================================================

/**
 * Restart approval action
 */
export interface RestartApprovalAction {
  requestId: string
  approved: boolean
  decidedBy: string
  reason?: string
}

/**
 * Display information for restart reasons
 */
export interface RestartReasonInfo {
  reason: RestartReason
  label: string
  description: string
  icon: 'code' | 'settings' | 'user' | 'recovery' | 'upgrade'
  color: string
}

/**
 * Restart request with computed UI properties
 */
export interface RestartRequestDisplay extends RestartRequest {
  /** Time since creation */
  timeAgo: string
  /** Formatted status for display */
  statusLabel: string
  /** Whether the request can be approved */
  canApprove: boolean
  /** Whether the request can be rejected */
  canReject: boolean
  /** Whether the request can be cancelled */
  canCancel: boolean
}

// =============================================================================
// Constants
// =============================================================================

/**
 * Mapping of restart reasons to display information
 */
export const RESTART_REASON_INFO: Record<RestartReason, RestartReasonInfo> = {
  code_change: {
    reason: 'code_change',
    label: 'Code Change',
    description: 'Code modifications require restart to take effect',
    icon: 'code',
    color: '#58a6ff',
  },
  config_change: {
    reason: 'config_change',
    label: 'Configuration Change',
    description: 'Configuration changes require restart to apply',
    icon: 'settings',
    color: '#a371f7',
  },
  user_requested: {
    reason: 'user_requested',
    label: 'User Requested',
    description: 'Manual restart requested by user',
    icon: 'user',
    color: '#7ee787',
  },
  recovery: {
    reason: 'recovery',
    label: 'Recovery',
    description: 'Recovery from error state requires restart',
    icon: 'recovery',
    color: '#f0883e',
  },
  upgrade: {
    reason: 'upgrade',
    label: 'Upgrade',
    description: 'Upgrade or update operation requires restart',
    icon: 'upgrade',
    color: '#79c0ff',
  },
}

/**
 * Status display configuration
 */
export const RESTART_STATUS_INFO: Record<RestartStatus, { label: string; color: string }> = {
  pending: { label: 'Pending Approval', color: '#f0883e' },
  approved: { label: 'Approved', color: '#238636' },
  rejected: { label: 'Rejected', color: '#da3633' },
  in_progress: { label: 'In Progress', color: '#58a6ff' },
  complete: { label: 'Complete', color: '#238636' },
  failed: { label: 'Failed', color: '#da3633' },
  cancelled: { label: 'Cancelled', color: '#8b949e' },
}
