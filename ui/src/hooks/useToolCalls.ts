/**
 * Tool Call Operations Hook
 *
 * Provides tool call management:
 * - Track pending tool calls requiring approval
 * - Approve/reject tool calls
 * - Handle tool call status updates
 * - Filter by approval status
 *
 * @module hooks/useToolCalls
 */

import { useCallback, useState } from 'react'
import type {
  ToolCall,
  ToolCallApproval,
  ToolCallStatus,
  ApprovalStatus,
} from '../types'

// =============================================================================
// Hook Return Type
// =============================================================================

export interface UseToolCallsReturn {
  /** All tool calls in current session */
  toolCalls: ToolCall[]
  /** Tool calls pending approval */
  pendingToolCalls: ToolCall[]
  /** Currently running tool calls */
  runningToolCalls: ToolCall[]
  /** Add a new tool call */
  addToolCall: (toolCall: ToolCall) => void
  /** Update an existing tool call */
  updateToolCall: (toolCall: ToolCall) => void
  /** Remove a tool call by ID */
  removeToolCall: (toolCallId: string) => void
  /** Handle tool call update from WebSocket */
  handleToolCallUpdate: (toolCall: ToolCall) => void
  /** Get tool calls requiring approval */
  getPendingApprovals: () => ToolCall[]
  /** Get tool calls by status */
  getToolCallsByStatus: (status: ToolCallStatus) => ToolCall[]
  /** Get tool calls by approval status */
  getToolCallsByApprovalStatus: (status: ApprovalStatus) => ToolCall[]
  /** Clear all tool calls */
  clearToolCalls: () => void
  /** Clear completed tool calls */
  clearCompletedToolCalls: () => void
  /** Create approval object */
  createApproval: (toolCallId: string, approved: boolean, approvedBy: string, reason?: string) => ToolCallApproval
}

// =============================================================================
// useToolCalls Hook
// =============================================================================

export function useToolCalls(): UseToolCallsReturn {
  // State
  const [toolCalls, setToolCalls] = useState<ToolCall[]>([])

  // Derived state - pending tool calls
  const pendingToolCalls = toolCalls.filter(
    (tc) => tc.approvalStatus === 'pending' && tc.status === 'pending'
  )

  // Derived state - running tool calls
  const runningToolCalls = toolCalls.filter((tc) => tc.status === 'running')

  // Add tool call
  const addToolCall = useCallback((toolCall: ToolCall) => {
    setToolCalls((prev) => {
      // Check for duplicates
      const exists = prev.some((tc) => tc.id === toolCall.id)
      if (exists) {
        return prev
      }
      return [...prev, toolCall]
    })
  }, [])

  // Update tool call
  const updateToolCall = useCallback((toolCall: ToolCall) => {
    setToolCalls((prev) => {
      const index = prev.findIndex((tc) => tc.id === toolCall.id)
      if (index === -1) {
        // Add if not found
        return [...prev, toolCall]
      }
      // Update existing
      const updated = [...prev]
      updated[index] = { ...prev[index], ...toolCall }
      return updated
    })
  }, [])

  // Remove tool call
  const removeToolCall = useCallback((toolCallId: string) => {
    setToolCalls((prev) => prev.filter((tc) => tc.id !== toolCallId))
  }, [])

  // Handle tool call update from WebSocket
  const handleToolCallUpdate = useCallback(
    (toolCall: ToolCall) => {
      updateToolCall(toolCall)
    },
    [updateToolCall]
  )

  // Get pending approvals
  const getPendingApprovals = useCallback(() => {
    return toolCalls.filter(
      (tc) => tc.approvalStatus === 'pending' && tc.status === 'pending'
    )
  }, [toolCalls])

  // Get tool calls by status
  const getToolCallsByStatus = useCallback(
    (status: ToolCallStatus) => {
      return toolCalls.filter((tc) => tc.status === status)
    },
    [toolCalls]
  )

  // Get tool calls by approval status
  const getToolCallsByApprovalStatus = useCallback(
    (status: ApprovalStatus) => {
      return toolCalls.filter((tc) => tc.approvalStatus === status)
    },
    [toolCalls]
  )

  // Clear all tool calls
  const clearToolCalls = useCallback(() => {
    setToolCalls([])
  }, [])

  // Clear completed tool calls
  const clearCompletedToolCalls = useCallback(() => {
    setToolCalls((prev) =>
      prev.filter(
        (tc) =>
          tc.status !== 'completed' &&
          tc.status !== 'failed' &&
          tc.status !== 'cancelled'
      )
    )
  }, [])

  // Create approval object
  const createApproval = useCallback(
    (
      toolCallId: string,
      approved: boolean,
      approvedBy: string,
      reason?: string
    ): ToolCallApproval => {
      return {
        toolCallId,
        approved,
        approvedBy,
        reason,
      }
    },
    []
  )

  return {
    toolCalls,
    pendingToolCalls,
    runningToolCalls,
    addToolCall,
    updateToolCall,
    removeToolCall,
    handleToolCallUpdate,
    getPendingApprovals,
    getToolCallsByStatus,
    getToolCallsByApprovalStatus,
    clearToolCalls,
    clearCompletedToolCalls,
    createApproval,
  }
}

export default useToolCalls
