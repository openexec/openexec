/**
 * Tests for useToolCalls hook
 *
 * @module hooks/__tests__/useToolCalls.test
 */

import { renderHook, act } from '@testing-library/react'
import { useToolCalls } from '../useToolCalls'
import type { ToolCall } from '../../types'

describe('useToolCalls', () => {
  const createToolCall = (overrides: Partial<ToolCall> = {}): ToolCall => ({
    id: 'tc-1',
    messageId: 'msg-1',
    sessionId: 'session-1',
    toolName: 'read_file',
    toolInput: '{"path": "/test.txt"}',
    status: 'pending',
    createdAt: '2024-01-01T00:00:00Z',
    ...overrides,
  })

  describe('addToolCall', () => {
    it('should add a new tool call', () => {
      const { result } = renderHook(() => useToolCalls())

      const toolCall = createToolCall()

      act(() => {
        result.current.addToolCall(toolCall)
      })

      expect(result.current.toolCalls).toHaveLength(1)
      expect(result.current.toolCalls[0]).toEqual(toolCall)
    })

    it('should not add duplicate tool calls', () => {
      const { result } = renderHook(() => useToolCalls())

      const toolCall = createToolCall()

      act(() => {
        result.current.addToolCall(toolCall)
      })

      act(() => {
        result.current.addToolCall(toolCall)
      })

      expect(result.current.toolCalls).toHaveLength(1)
    })
  })

  describe('updateToolCall', () => {
    it('should update an existing tool call', () => {
      const { result } = renderHook(() => useToolCalls())

      const toolCall = createToolCall()

      act(() => {
        result.current.addToolCall(toolCall)
      })

      act(() => {
        result.current.updateToolCall({
          ...toolCall,
          status: 'running',
          startedAt: '2024-01-01T00:00:01Z',
        })
      })

      expect(result.current.toolCalls[0].status).toBe('running')
      expect(result.current.toolCalls[0].startedAt).toBe('2024-01-01T00:00:01Z')
    })

    it('should add tool call if not found', () => {
      const { result } = renderHook(() => useToolCalls())

      const toolCall = createToolCall()

      act(() => {
        result.current.updateToolCall(toolCall)
      })

      expect(result.current.toolCalls).toHaveLength(1)
    })
  })

  describe('removeToolCall', () => {
    it('should remove a tool call by ID', () => {
      const { result } = renderHook(() => useToolCalls())

      act(() => {
        result.current.addToolCall(createToolCall({ id: 'tc-1' }))
        result.current.addToolCall(createToolCall({ id: 'tc-2' }))
      })

      act(() => {
        result.current.removeToolCall('tc-1')
      })

      expect(result.current.toolCalls).toHaveLength(1)
      expect(result.current.toolCalls[0].id).toBe('tc-2')
    })
  })

  describe('pendingToolCalls', () => {
    it('should return tool calls pending approval', () => {
      const { result } = renderHook(() => useToolCalls())

      act(() => {
        result.current.addToolCall(
          createToolCall({
            id: 'tc-1',
            status: 'pending',
            approvalStatus: 'pending',
          })
        )
        result.current.addToolCall(
          createToolCall({
            id: 'tc-2',
            status: 'running',
            approvalStatus: 'approved',
          })
        )
        result.current.addToolCall(
          createToolCall({
            id: 'tc-3',
            status: 'pending',
            approvalStatus: 'pending',
          })
        )
      })

      expect(result.current.pendingToolCalls).toHaveLength(2)
      expect(result.current.pendingToolCalls.map((tc) => tc.id)).toEqual(['tc-1', 'tc-3'])
    })
  })

  describe('runningToolCalls', () => {
    it('should return running tool calls', () => {
      const { result } = renderHook(() => useToolCalls())

      act(() => {
        result.current.addToolCall(
          createToolCall({
            id: 'tc-1',
            status: 'pending',
          })
        )
        result.current.addToolCall(
          createToolCall({
            id: 'tc-2',
            status: 'running',
          })
        )
        result.current.addToolCall(
          createToolCall({
            id: 'tc-3',
            status: 'completed',
          })
        )
      })

      expect(result.current.runningToolCalls).toHaveLength(1)
      expect(result.current.runningToolCalls[0].id).toBe('tc-2')
    })
  })

  describe('getToolCallsByStatus', () => {
    it('should filter tool calls by status', () => {
      const { result } = renderHook(() => useToolCalls())

      act(() => {
        result.current.addToolCall(createToolCall({ id: 'tc-1', status: 'pending' }))
        result.current.addToolCall(createToolCall({ id: 'tc-2', status: 'running' }))
        result.current.addToolCall(createToolCall({ id: 'tc-3', status: 'completed' }))
        result.current.addToolCall(createToolCall({ id: 'tc-4', status: 'failed' }))
      })

      expect(result.current.getToolCallsByStatus('pending')).toHaveLength(1)
      expect(result.current.getToolCallsByStatus('completed')).toHaveLength(1)
      expect(result.current.getToolCallsByStatus('failed')).toHaveLength(1)
    })
  })

  describe('getToolCallsByApprovalStatus', () => {
    it('should filter tool calls by approval status', () => {
      const { result } = renderHook(() => useToolCalls())

      act(() => {
        result.current.addToolCall(
          createToolCall({ id: 'tc-1', approvalStatus: 'pending' })
        )
        result.current.addToolCall(
          createToolCall({ id: 'tc-2', approvalStatus: 'approved' })
        )
        result.current.addToolCall(
          createToolCall({ id: 'tc-3', approvalStatus: 'rejected' })
        )
        result.current.addToolCall(
          createToolCall({ id: 'tc-4', approvalStatus: 'auto_approved' })
        )
      })

      expect(result.current.getToolCallsByApprovalStatus('pending')).toHaveLength(1)
      expect(result.current.getToolCallsByApprovalStatus('approved')).toHaveLength(1)
      expect(result.current.getToolCallsByApprovalStatus('rejected')).toHaveLength(1)
      expect(result.current.getToolCallsByApprovalStatus('auto_approved')).toHaveLength(1)
    })
  })

  describe('clearToolCalls', () => {
    it('should clear all tool calls', () => {
      const { result } = renderHook(() => useToolCalls())

      act(() => {
        result.current.addToolCall(createToolCall({ id: 'tc-1' }))
        result.current.addToolCall(createToolCall({ id: 'tc-2' }))
      })

      act(() => {
        result.current.clearToolCalls()
      })

      expect(result.current.toolCalls).toHaveLength(0)
    })
  })

  describe('clearCompletedToolCalls', () => {
    it('should clear completed, failed, and cancelled tool calls', () => {
      const { result } = renderHook(() => useToolCalls())

      act(() => {
        result.current.addToolCall(createToolCall({ id: 'tc-1', status: 'pending' }))
        result.current.addToolCall(createToolCall({ id: 'tc-2', status: 'running' }))
        result.current.addToolCall(createToolCall({ id: 'tc-3', status: 'completed' }))
        result.current.addToolCall(createToolCall({ id: 'tc-4', status: 'failed' }))
        result.current.addToolCall(createToolCall({ id: 'tc-5', status: 'cancelled' }))
      })

      act(() => {
        result.current.clearCompletedToolCalls()
      })

      expect(result.current.toolCalls).toHaveLength(2)
      expect(result.current.toolCalls.map((tc) => tc.id)).toEqual(['tc-1', 'tc-2'])
    })
  })

  describe('createApproval', () => {
    it('should create an approval object', () => {
      const { result } = renderHook(() => useToolCalls())

      const approval = result.current.createApproval('tc-1', true, 'user-1', 'Looks safe')

      expect(approval).toEqual({
        toolCallId: 'tc-1',
        approved: true,
        approvedBy: 'user-1',
        reason: 'Looks safe',
      })
    })

    it('should create a rejection object', () => {
      const { result } = renderHook(() => useToolCalls())

      const rejection = result.current.createApproval(
        'tc-1',
        false,
        'user-1',
        'Too dangerous'
      )

      expect(rejection).toEqual({
        toolCallId: 'tc-1',
        approved: false,
        approvedBy: 'user-1',
        reason: 'Too dangerous',
      })
    })
  })

  describe('handleToolCallUpdate', () => {
    it('should handle tool call status progression', () => {
      const { result } = renderHook(() => useToolCalls())

      // Initial pending tool call
      act(() => {
        result.current.addToolCall(
          createToolCall({
            id: 'tc-1',
            status: 'pending',
            approvalStatus: 'pending',
          })
        )
      })

      expect(result.current.pendingToolCalls).toHaveLength(1)

      // Approval
      act(() => {
        result.current.handleToolCallUpdate(
          createToolCall({
            id: 'tc-1',
            status: 'pending',
            approvalStatus: 'approved',
            approvedBy: 'user-1',
            approvedAt: '2024-01-01T00:00:01Z',
          })
        )
      })

      expect(result.current.pendingToolCalls).toHaveLength(0)
      expect(result.current.toolCalls[0].approvalStatus).toBe('approved')

      // Start running
      act(() => {
        result.current.handleToolCallUpdate(
          createToolCall({
            id: 'tc-1',
            status: 'running',
            approvalStatus: 'approved',
            startedAt: '2024-01-01T00:00:02Z',
          })
        )
      })

      expect(result.current.runningToolCalls).toHaveLength(1)

      // Complete
      act(() => {
        result.current.handleToolCallUpdate(
          createToolCall({
            id: 'tc-1',
            status: 'completed',
            approvalStatus: 'approved',
            toolOutput: 'File contents...',
            completedAt: '2024-01-01T00:00:03Z',
            durationMs: 1000,
          })
        )
      })

      expect(result.current.runningToolCalls).toHaveLength(0)
      expect(result.current.toolCalls[0].status).toBe('completed')
      expect(result.current.toolCalls[0].toolOutput).toBe('File contents...')
    })
  })
})
