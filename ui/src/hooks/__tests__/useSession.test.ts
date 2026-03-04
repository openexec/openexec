/**
 * Tests for useSession hook
 *
 * @module hooks/__tests__/useSession.test
 */

import { renderHook, act, waitFor } from '@testing-library/react'
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { useSession, type SessionApiConfig } from '../useSession'

// =============================================================================
// Mock Setup
// =============================================================================

const mockFetch = vi.fn()

// Mock localStorage
const localStorageMock = (() => {
  let store: Record<string, string> = {}
  return {
    getItem: vi.fn((key: string) => store[key] ?? null),
    setItem: vi.fn((key: string, value: string) => {
      store[key] = value
    }),
    removeItem: vi.fn((key: string) => {
      delete store[key]
    }),
    clear: vi.fn(() => {
      store = {}
    }),
  }
})()

beforeEach(() => {
  vi.stubGlobal('fetch', mockFetch)
  vi.stubGlobal('localStorage', localStorageMock)
  localStorageMock.clear()
  mockFetch.mockClear()
})

afterEach(() => {
  vi.unstubAllGlobals()
})

// Helper to create mock response
function createMockResponse<T>(data: T, status = 200): Response {
  return {
    ok: status >= 200 && status < 300,
    status,
    text: async () => data ? JSON.stringify(data) : '',
  } as Response
}

// =============================================================================
// Tests
// =============================================================================

describe('useSession', () => {
  const defaultConfig: SessionApiConfig = {
    baseUrl: 'http://localhost:8080',
    authToken: 'test-token',
  }

  describe('fetchSessions', () => {
    it('should fetch sessions successfully', async () => {
      const sessions = [
        {
          id: 'session-1',
          title: 'Test Session',
          provider: 'anthropic',
          model: 'claude-3-opus',
          status: 'active',
          messageCount: 5,
          totalCostUsd: 0.05,
          createdAt: '2024-01-01T00:00:00Z',
          updatedAt: '2024-01-01T00:00:00Z',
        },
      ]

      mockFetch.mockResolvedValueOnce(createMockResponse(sessions))

      const { result } = renderHook(() => useSession(defaultConfig))

      await act(async () => {
        await result.current.fetchSessions()
      })

      expect(mockFetch).toHaveBeenCalledWith(
        'http://localhost:8080/sessions',
        expect.objectContaining({
          method: 'GET',
          headers: expect.objectContaining({
            Authorization: 'Bearer test-token',
          }),
        })
      )

      expect(result.current.sessions).toEqual(sessions)
      expect(result.current.sessionsLoading).toBe(false)
      expect(result.current.sessionsError).toBeUndefined()
    })

    it('should handle fetch error', async () => {
      mockFetch.mockResolvedValueOnce(createMockResponse('Internal Server Error', 500))

      const { result } = renderHook(() => useSession(defaultConfig))

      await act(async () => {
        await result.current.fetchSessions()
      })

      expect(result.current.sessions).toEqual([])
      expect(result.current.sessionsError).toBe('API error 500: Internal Server Error')
    })

    it('should apply filters to fetch request', async () => {
      mockFetch.mockResolvedValue(createMockResponse([]))

      const { result } = renderHook(() => useSession(defaultConfig))

      await act(async () => {
        await result.current.fetchSessions({
          projectPath: '/test/project',
          status: 'active',
          search: 'test',
          sortBy: 'created_at',
          sortOrder: 'desc',
        })
      })

      expect(mockFetch).toHaveBeenCalledWith(
        expect.stringContaining('project_path=%2Ftest%2Fproject'),
        expect.anything()
      )
      expect(mockFetch).toHaveBeenCalledWith(
        expect.stringContaining('status=active'),
        expect.anything()
      )
      expect(mockFetch).toHaveBeenCalledWith(
        expect.stringContaining('search=test'),
        expect.anything()
      )
    })
  })

  describe('createSession', () => {
    it('should create a session successfully', async () => {
      const newSession = {
        id: 'session-new',
        projectPath: '/test/project',
        provider: 'anthropic',
        model: 'claude-3-opus',
        title: 'New Session',
        status: 'active',
        createdAt: '2024-01-01T00:00:00Z',
        updatedAt: '2024-01-01T00:00:00Z',
      }

      mockFetch.mockResolvedValueOnce(createMockResponse(newSession))

      const { result } = renderHook(() => useSession(defaultConfig))

      let sessionResult
      await act(async () => {
        sessionResult = await result.current.createSession({
          projectPath: '/test/project',
          provider: 'anthropic',
          model: 'claude-3-opus',
          title: 'New Session',
        })
      })

      expect(sessionResult).toEqual(newSession)
      expect(result.current.sessions).toHaveLength(1)
      expect(result.current.sessions[0].id).toBe('session-new')
    })
  })

  describe('loadSession', () => {
    it('should load a session successfully', async () => {
      const session = {
        id: 'session-1',
        projectPath: '/test/project',
        provider: 'anthropic',
        model: 'claude-3-opus',
        title: 'Test Session',
        status: 'active',
        createdAt: '2024-01-01T00:00:00Z',
        updatedAt: '2024-01-01T00:00:00Z',
      }

      mockFetch.mockResolvedValueOnce(createMockResponse(session))

      const { result } = renderHook(() => useSession(defaultConfig))

      let loadedSession
      await act(async () => {
        loadedSession = await result.current.loadSession('session-1')
      })

      expect(loadedSession).toEqual(session)
      expect(result.current.currentSession).toEqual(session)
      expect(result.current.currentSessionLoading).toBe(false)
    })
  })

  describe('localStorage persistence', () => {
    it('should persist sessions to localStorage', async () => {
      const sessions = [
        {
          id: 'session-1',
          title: 'Test Session',
          provider: 'anthropic',
          model: 'claude-3-opus',
          status: 'active',
          messageCount: 5,
          totalCostUsd: 0.05,
          createdAt: '2024-01-01T00:00:00Z',
          updatedAt: '2024-01-01T00:00:00Z',
        },
      ]

      mockFetch.mockResolvedValueOnce(createMockResponse(sessions))

      const { result } = renderHook(() => useSession(defaultConfig))

      await act(async () => {
        await result.current.fetchSessions()
      })

      expect(localStorageMock.setItem).toHaveBeenCalledWith(
        'openexec-sessions',
        expect.stringContaining('session-1')
      )
    })
  })
})
