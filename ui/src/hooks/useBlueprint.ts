/**
 * useBlueprint Hook
 *
 * Manages blueprint execution state from WebSocket events.
 * Tracks stage progress, approvals, and provides actions.
 *
 * @module hooks/useBlueprint
 */

import { useState, useCallback, useEffect, useRef } from 'react'
import type {
  Stage,
  StageName,
  StageStatus,
  BlueprintState,
  Approval,
  BlueprintEventType,
} from '../types/blueprint'
import { BLUEPRINT_STAGES, STAGE_INFO } from '../types/blueprint'
import type { LoopEvent } from '../types/chat'

// =============================================================================
// Configuration
// =============================================================================

export interface UseBlueprintConfig {
  /** API base URL for fetching approvals */
  apiUrl: string
  /** Optional auth token */
  authToken?: string
  /** Run ID to track */
  runId?: string
  /** Auto-fetch approvals on mount */
  autoFetchApprovals?: boolean
  /** Approval fetch interval in ms (0 to disable polling) */
  approvalPollInterval?: number
}

// =============================================================================
// Return Type
// =============================================================================

export interface UseBlueprintReturn {
  /** Current blueprint execution state */
  blueprintState: BlueprintState | undefined
  /** List of stages with current status */
  stages: Stage[]
  /** Current stage being executed */
  currentStage: StageName | undefined
  /** Whether blueprint is running */
  isRunning: boolean
  /** Whether blueprint completed successfully */
  isComplete: boolean
  /** Whether blueprint failed */
  isFailed: boolean
  /** Pending approval requests */
  approvals: Approval[]
  /** Whether approvals are loading */
  approvalsLoading: boolean
  /** Approvals error */
  approvalsError: string | undefined
  /** Process a loop event to update state */
  processEvent: (event: LoopEvent) => void
  /** Fetch approvals from API */
  fetchApprovals: () => Promise<void>
  /** Approve an approval request */
  approveRequest: (id: string) => Promise<void>
  /** Reject an approval request */
  rejectRequest: (id: string, reason?: string) => Promise<void>
  /** Reset blueprint state */
  reset: () => void
}

// =============================================================================
// Hook Implementation
// =============================================================================

/**
 * Hook for managing blueprint execution state
 */
export function useBlueprint(config: UseBlueprintConfig): UseBlueprintReturn {
  const { apiUrl, authToken, runId, autoFetchApprovals = true, approvalPollInterval = 5000 } = config

  // Blueprint state
  const [blueprintState, setBlueprintState] = useState<BlueprintState | undefined>()
  const [stages, setStages] = useState<Stage[]>(() =>
    BLUEPRINT_STAGES.map((name) => ({
      name,
      description: STAGE_INFO[name].description,
      type: STAGE_INFO[name].type,
      status: 'pending' as StageStatus,
    }))
  )

  // Approvals state
  const [approvals, setApprovals] = useState<Approval[]>([])
  const [approvalsLoading, setApprovalsLoading] = useState(false)
  const [approvalsError, setApprovalsError] = useState<string | undefined>()

  // Refs for polling
  const pollIntervalRef = useRef<ReturnType<typeof setInterval> | null>(null)

  // Derived state
  const currentStage = stages.find((s) => s.status === 'running')?.name
  const isRunning = stages.some((s) => s.status === 'running')
  const isComplete = stages.every((s) => s.status === 'completed' || s.status === 'skipped')
  const isFailed = stages.some((s) => s.status === 'failed')

  // Update a stage's status
  const updateStage = useCallback((stageName: StageName, updates: Partial<Stage>) => {
    setStages((prev) =>
      prev.map((s) => (s.name === stageName ? { ...s, ...updates } : s))
    )
  }, [])

  // Process a loop event
  const processEvent = useCallback(
    (event: LoopEvent) => {
      const eventType = event.type as string

      // Handle blueprint events
      if (eventType === 'blueprint_start') {
        setBlueprintState({
          id: event.metadata?.blueprint_id as string || `bp-${Date.now()}`,
          blueprintId: event.metadata?.blueprint_id as string || 'standard_task',
          taskDescription: event.metadata?.task_description as string || '',
          stages: stages,
          status: 'running',
          startedAt: event.timestamp || new Date().toISOString(),
        })
      } else if (eventType === 'blueprint_complete') {
        setBlueprintState((prev) =>
          prev
            ? {
                ...prev,
                status: 'completed',
                completedAt: event.timestamp || new Date().toISOString(),
              }
            : prev
        )
      } else if (eventType === 'blueprint_failed') {
        setBlueprintState((prev) =>
          prev
            ? {
                ...prev,
                status: 'failed',
                completedAt: event.timestamp || new Date().toISOString(),
                error: event.error || event.message || 'Blueprint execution failed',
              }
            : prev
        )
      }

      // Handle stage events
      const stageName = (event.metadata?.stage_name as StageName) ||
                        (event as unknown as { stageName?: StageName }).stageName

      if (stageName && BLUEPRINT_STAGES.includes(stageName)) {
        if (eventType === 'stage_start') {
          updateStage(stageName, {
            status: 'running',
            startedAt: event.timestamp || new Date().toISOString(),
            attempt: (event.metadata?.attempt as number) || 1,
          })
        } else if (eventType === 'stage_complete') {
          const startedAt = stages.find((s) => s.name === stageName)?.startedAt
          const completedAt = event.timestamp || new Date().toISOString()
          const durationMs = startedAt
            ? new Date(completedAt).getTime() - new Date(startedAt).getTime()
            : undefined

          updateStage(stageName, {
            status: 'completed',
            completedAt,
            durationMs,
            output: event.message || (event.metadata?.output as string),
          })
        } else if (eventType === 'stage_failed') {
          const startedAt = stages.find((s) => s.name === stageName)?.startedAt
          const completedAt = event.timestamp || new Date().toISOString()
          const durationMs = startedAt
            ? new Date(completedAt).getTime() - new Date(startedAt).getTime()
            : undefined

          updateStage(stageName, {
            status: 'failed',
            completedAt,
            durationMs,
            error: event.error || event.message || 'Stage failed',
          })
        } else if (eventType === 'stage_retry') {
          updateStage(stageName, {
            attempt: (event.metadata?.attempt as number) || 1,
          })
        }
      }
    },
    [stages, updateStage]
  )

  // Fetch approvals from API
  const fetchApprovals = useCallback(async () => {
    if (!apiUrl) return

    setApprovalsLoading(true)
    setApprovalsError(undefined)

    try {
      const params = new URLSearchParams()
      if (runId) params.set('run_id', runId)
      params.set('status', 'pending')

      const response = await fetch(`${apiUrl}/api/v1/approvals?${params}`, {
        headers: authToken ? { Authorization: `Bearer ${authToken}` } : undefined,
      })

      if (!response.ok) {
        throw new Error(`Failed to fetch approvals: ${response.statusText}`)
      }

      const data = await response.json()
      setApprovals(data.approvals || [])
    } catch (err) {
      setApprovalsError(err instanceof Error ? err.message : 'Failed to fetch approvals')
    } finally {
      setApprovalsLoading(false)
    }
  }, [apiUrl, authToken, runId])

  // Approve a request
  const approveRequest = useCallback(
    async (id: string) => {
      if (!apiUrl) return

      try {
        const response = await fetch(`${apiUrl}/api/v1/approvals/${id}/approve`, {
          method: 'POST',
          headers: {
            'Content-Type': 'application/json',
            ...(authToken ? { Authorization: `Bearer ${authToken}` } : {}),
          },
          body: JSON.stringify({ decided_by: 'user' }),
        })

        if (!response.ok) {
          throw new Error(`Failed to approve: ${response.statusText}`)
        }

        // Update local state
        setApprovals((prev) =>
          prev.map((a) => (a.id === id ? { ...a, status: 'approved' } : a))
        )
      } catch (err) {
        setApprovalsError(err instanceof Error ? err.message : 'Failed to approve')
        throw err
      }
    },
    [apiUrl, authToken]
  )

  // Reject a request
  const rejectRequest = useCallback(
    async (id: string, reason?: string) => {
      if (!apiUrl) return

      try {
        const response = await fetch(`${apiUrl}/api/v1/approvals/${id}/reject`, {
          method: 'POST',
          headers: {
            'Content-Type': 'application/json',
            ...(authToken ? { Authorization: `Bearer ${authToken}` } : {}),
          },
          body: JSON.stringify({
            decided_by: 'user',
            reason: reason || 'Rejected by user',
          }),
        })

        if (!response.ok) {
          throw new Error(`Failed to reject: ${response.statusText}`)
        }

        // Update local state
        setApprovals((prev) =>
          prev.map((a) => (a.id === id ? { ...a, status: 'rejected' } : a))
        )
      } catch (err) {
        setApprovalsError(err instanceof Error ? err.message : 'Failed to reject')
        throw err
      }
    },
    [apiUrl, authToken]
  )

  // Reset state
  const reset = useCallback(() => {
    setBlueprintState(undefined)
    setStages(
      BLUEPRINT_STAGES.map((name) => ({
        name,
        description: STAGE_INFO[name].description,
        type: STAGE_INFO[name].type,
        status: 'pending' as StageStatus,
      }))
    )
    setApprovals([])
    setApprovalsError(undefined)
  }, [])

  // Auto-fetch approvals on mount
  useEffect(() => {
    if (autoFetchApprovals && apiUrl) {
      fetchApprovals()
    }
  }, [autoFetchApprovals, apiUrl, fetchApprovals])

  // Poll for approvals
  useEffect(() => {
    if (approvalPollInterval > 0 && apiUrl && isRunning) {
      pollIntervalRef.current = setInterval(fetchApprovals, approvalPollInterval)

      return () => {
        if (pollIntervalRef.current) {
          clearInterval(pollIntervalRef.current)
          pollIntervalRef.current = null
        }
      }
    }
  }, [approvalPollInterval, apiUrl, isRunning, fetchApprovals])

  // Cleanup on unmount
  useEffect(() => {
    return () => {
      if (pollIntervalRef.current) {
        clearInterval(pollIntervalRef.current)
      }
    }
  }, [])

  return {
    blueprintState,
    stages,
    currentStage,
    isRunning,
    isComplete,
    isFailed,
    approvals,
    approvalsLoading,
    approvalsError,
    processEvent,
    fetchApprovals,
    approveRequest,
    rejectRequest,
    reset,
  }
}

export default useBlueprint
