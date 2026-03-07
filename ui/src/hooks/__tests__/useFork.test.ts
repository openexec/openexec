/**
 * Tests for useFork Hook
 * @module hooks/__tests__/useFork
 */
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { renderHook, act } from '@testing-library/react'
import { useFork } from '../useFork'
import type { Session, Message } from '../../types'

// Mock fetch
const mockFetch = vi.fn()
global.fetch = mockFetch

const createMockSession = (overrides: Partial<Session> = {}): Session => ({
  id: 'session-123',
  projectPath: '/test/project',
  provider: 'anthropic',
  model: 'claude-3-5-sonnet',
  title: 'Test Session',
  status: 'active',
  createdAt: new Date().toISOString(),
  updatedAt: new Date().toISOString(),
  ...overrides,
})

const createMockMessage = (overrides: Partial<Message> = {}): Message => ({
  id: 'msg-123',
  sessionId: 'session-123',
  role: 'assistant',
  content: 'Test message',
  tokensInput: 100,
  tokensOutput: 50,
  costUsd: 0.001,
  createdAt: new Date().toISOString(),
  ...overrides,
})

const mockForkResponse = {
  forked_session_id: 'fork-456',
  parent_session_id: 'session-123',
  fork_point_message_id: 'msg-123',
  title: 'Fork of Test Session',
  provider: 'anthropic',
  model: 'claude-3-5-sonnet',
  messages_copied: 5,
  tool_calls_copied: 2,
  summaries_copied: 1,
  fork_depth: 1,
  ancestor_chain: ['session-123'],
}

const mockForkInfoResponse = {
  session_id: 'session-123',
  fork_depth: 2,
  parent_session_id: 'parent-session',
  root_session_id: 'root-session',
  fork_point_message_id: 'msg-100',
  ancestor_chain: ['root-session', 'parent-session', 'session-123'],
}

const mockForksListResponse = [
  {
    session_id: 'fork-1',
    title: 'First Fork',
    created_at: '2024-01-15T10:00:00Z',
    fork_point_message_id: 'msg-50',
  },
  {
    session_id: 'fork-2',
    title: 'Second Fork',
    created_at: '2024-01-16T10:00:00Z',
    fork_point_message_id: 'msg-75',
  },
]

describe('useFork', () => {
  const config = {
    baseUrl: 'http://localhost:8080/api',
    authToken: 'test-token',
  }

  beforeEach(() => {
    mockFetch.mockReset()
  })

  afterEach(() => {
    vi.clearAllMocks()
  })

  describe('dialog state', () => {
    it('initializes with dialog closed', () => {
      const { result } = renderHook(() => useFork(config))

      expect(result.current.isForkDialogOpen).toBe(false)
      expect(result.current.sessionToFork).toBeUndefined()
      expect(result.current.forkPointMessage).toBeUndefined()
    })

    it('opens dialog with session', async () => {
      const { result } = renderHook(() => useFork(config))
      const session = createMockSession()

      await act(async () => {
        result.current.openForkDialog(session)
      })

      expect(result.current.isForkDialogOpen).toBe(true)
      expect(result.current.sessionToFork).toEqual(session)
      expect(result.current.forkPointMessage).toBeUndefined()
    })

    it('opens dialog with session and fork point message', async () => {
      const { result } = renderHook(() => useFork(config))
      const session = createMockSession()
      const message = createMockMessage()

      await act(async () => {
        result.current.openForkDialog(session, message)
      })

      expect(result.current.isForkDialogOpen).toBe(true)
      expect(result.current.sessionToFork).toEqual(session)
      expect(result.current.forkPointMessage).toEqual(message)
    })

    it('closes dialog and clears state', async () => {
      const { result } = renderHook(() => useFork(config))
      const session = createMockSession()
      const message = createMockMessage()

      await act(async () => {
        result.current.openForkDialog(session, message)
      })

      await act(async () => {
        result.current.closeForkDialog()
      })

      expect(result.current.isForkDialogOpen).toBe(false)
      expect(result.current.sessionToFork).toBeUndefined()
      expect(result.current.forkPointMessage).toBeUndefined()
    })

    it('clears previous error when opening dialog', async () => {
      const { result } = renderHook(() => useFork(config))
      const session = createMockSession()

      // Set an error manually if possible or via a failed call
      mockFetch.mockResolvedValueOnce({
        ok: false,
        status: 500,
        text: () => Promise.resolve('Error'),
      })

      await act(async () => {
        try {
          await result.current.forkSession('s1', 'm1', { title: '', provider: '', model: '', copyMessages: true, copyToolCalls: true, copySummaries: true })
        } catch (e) { /* ignore */ }
      })
      
      expect(result.current.forkError).toBeDefined()

      await act(async () => {
        result.current.openForkDialog(session)
      })

      expect(result.current.forkError).toBeUndefined()
    })
  })

  describe('forkSession', () => {
    it('forks a session successfully', async () => {
      mockFetch.mockResolvedValueOnce({
        ok: true,
        text: () => Promise.resolve(JSON.stringify(mockForkResponse)),
      })

      const { result } = renderHook(() => useFork(config))

      let forkResult: any
      await act(async () => {
        forkResult = await result.current.forkSession('session-123', 'msg-123', {
          title: 'My Fork',
          copyMessages: true,
          copyToolCalls: true,
          copySummaries: true,
          provider: '',
          model: '',
        })
      })

      expect(forkResult!.forkedSessionId).toBe('fork-456')
      expect(result.current.lastForkResult).toEqual(forkResult)
    })

    it('sends correct request body', async () => {
      mockFetch.mockResolvedValueOnce({
        ok: true,
        text: () => Promise.resolve(JSON.stringify(mockForkResponse)),
      })

      const { result } = renderHook(() => useFork(config))

      await act(async () => {
        await result.current.forkSession('session-123', 'msg-123', {
          title: 'Custom Title',
          provider: 'openai',
          model: 'gpt-4o',
          copyMessages: true,
          copyToolCalls: false,
          copySummaries: true,
        })
      })

      expect(mockFetch).toHaveBeenCalledWith(
        'http://localhost:8080/api/sessions/session-123/fork',
        expect.objectContaining({
          method: 'POST',
          body: JSON.stringify({
            fork_point_message_id: 'msg-123',
            title: 'Custom Title',
            provider: 'openai',
            model: 'gpt-4o',
            copy_messages: true,
            copy_tool_calls: false,
            copy_summaries: true,
          }),
        })
      )
    })

    it('sets loading state during fork', async () => {
      let resolvePromise: (value: any) => void
      const delayedPromise = new Promise((resolve) => {
        resolvePromise = resolve
      })

      mockFetch.mockImplementationOnce(() => delayedPromise)

      const { result } = renderHook(() => useFork(config))

      expect(result.current.isForking).toBe(false)

      let forkPromise: any
      await act(async () => {
        forkPromise = result.current.forkSession('session-123', 'msg-123', {
          title: '', provider: '', model: '', copyMessages: true, copyToolCalls: true, copySummaries: true
        })
      })

      expect(result.current.isForking).toBe(true)

      await act(async () => {
        resolvePromise!({
          ok: true,
          text: () => Promise.resolve(JSON.stringify(mockForkResponse)),
        })
        await forkPromise
      })

      expect(result.current.isForking).toBe(false)
    })

    it('handles fork error', async () => {
      mockFetch.mockResolvedValueOnce({
        ok: false,
        status: 404,
        text: () => Promise.resolve('Session not found'),
      })

      const { result } = renderHook(() => useFork(config))

      await act(async () => {
        try {
          await result.current.forkSession('invalid-session', 'msg-123', {
            title: '', provider: '', model: '', copyMessages: true, copyToolCalls: true, copySummaries: true
          })
        } catch (e) { /* ignore */ }
      })

      expect(result.current.forkError).toBe('API error 404: Session not found')
      expect(result.current.isForking).toBe(false)
    })

    it('clears fork error', async () => {
      const { result } = renderHook(() => useFork(config))

      mockFetch.mockResolvedValueOnce({
        ok: false,
        status: 500,
        text: () => Promise.resolve('Server error'),
      })

      await act(async () => {
        try {
          await result.current.forkSession('s1', 'm1', { title: '', provider: '', model: '', copyMessages: true, copyToolCalls: true, copySummaries: true })
        } catch (e) { /* ignore */ }
      })

      expect(result.current.forkError).toBe('API error 500: Server error')

      await act(async () => {
        result.current.clearForkError()
      })

      expect(result.current.forkError).toBeUndefined()
    })

    it('includes auth token in request', async () => {
      mockFetch.mockResolvedValueOnce({
        ok: true,
        text: () => Promise.resolve(JSON.stringify(mockForkResponse)),
      })

      const { result } = renderHook(() => useFork(config))

      await act(async () => {
        await result.current.forkSession('session-123', 'msg-123', {
          title: '', provider: '', model: '', copyMessages: true, copyToolCalls: true, copySummaries: true
        })
      })

      expect(mockFetch).toHaveBeenCalledWith(
        expect.stringContaining('/fork'),
        expect.objectContaining({
          headers: expect.objectContaining({
            Authorization: 'Bearer test-token',
          }),
        })
      )
    })
  })

  describe('getForkInfo', () => {
    it('fetches fork info successfully', async () => {
      mockFetch.mockResolvedValueOnce({
        ok: true,
        text: () => Promise.resolve(JSON.stringify(mockForkInfoResponse)),
      })

      const { result } = renderHook(() => useFork(config))

      let forkInfo: any
      await act(async () => {
        forkInfo = await result.current.getForkInfo('session-123')
      })

      expect(mockFetch).toHaveBeenCalledWith(
        'http://localhost:8080/api/sessions/session-123/fork-info',
        expect.objectContaining({ method: 'GET' })
      )
      expect(forkInfo.sessionId).toBe('session-123')
      expect(result.current.forkInfo).toEqual(forkInfo)
    })
  })

  describe('listSessionForks', () => {
    it('lists session forks successfully', async () => {
      mockFetch.mockResolvedValueOnce({
        ok: true,
        text: () => Promise.resolve(JSON.stringify(mockForksListResponse)),
      })

      const { result } = renderHook(() => useFork(config))

      let forks: any
      await act(async () => {
        forks = await result.current.listSessionForks('session-123')
      })

      expect(mockFetch).toHaveBeenCalledWith(
        'http://localhost:8080/api/sessions/session-123/forks',
        expect.objectContaining({ method: 'GET' })
      )
      expect(forks).toHaveLength(2)
      expect(result.current.sessionForks).toEqual(forks)
    })
  })
})
