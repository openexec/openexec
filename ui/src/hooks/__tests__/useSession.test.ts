/**
 * Tests for useSession hook
 *
 * @module hooks/__tests__/useSession.test
 */

import { renderHook, act, waitFor } from '@testing-library/react'
import { useSession, SessionApiConfig } from '../useSession'

// Mock fetch
const mockFetch = jest.fn()
global.fetch = mockFetch

// Mock localStorage
const localStorageMock = (() => {
  let store: Record<string, string> = {}
  return {
    getItem: jest.fn((key: string) => store[key] ?? null),
    setItem: jest.fn((key: string, value: string) => {
      store[key] = value
    }),
    removeItem: jest.fn((key: string) => {
      delete store[key]
    }),
    clear: jest.fn(() => {
      store = {}
    }),
  }
})()

Object.defineProperty(window, 'localStorage', {
  value: localStorageMock,
})

describe('useSession', () => {
  const defaultConfig: SessionApiConfig = {
    baseUrl: 'http://localhost:8080',
    authToken: 'test-token',
  }

  beforeEach(() => {
    mockFetch.mockClear()
    localStorageMock.clear()
    localStorageMock.getItem.mockClear()
    localStorageMock.setItem.mockClear()
  })

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

      mockFetch.mockResolvedValueOnce({
        ok: true,
        text: async () => JSON.stringify(sessions),
      })

      const { result } = renderHook(() => useSession(defaultConfig))

      await act(async () => {
        await result.current.fetchSessions()
      })

      expect(mockFetch).toHaveBeenCalledWith(
        'http://localhost:8080/api/sessions',
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
      mockFetch.mockResolvedValueOnce({
        ok: false,
        status: 500,
        text: async () => 'Internal Server Error',
      })

      const { result } = renderHook(() => useSession(defaultConfig))

      await act(async () => {
        await result.current.fetchSessions()
      })

      expect(result.current.sessions).toEqual([])
      expect(result.current.sessionsError).toBe('API error 500: Internal Server Error')
    })

    it('should apply filters to fetch request', async () => {
      mockFetch.mockResolvedValueOnce({
        ok: true,
        text: async () => '[]',
      })

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

      mockFetch.mockResolvedValueOnce({
        ok: true,
        text: async () => JSON.stringify(newSession),
      })

      const { result } = renderHook(() => useSession(defaultConfig))

      let session
      await act(async () => {
        session = await result.current.createSession({
          projectPath: '/test/project',
          provider: 'anthropic',
          model: 'claude-3-opus',
          title: 'New Session',
        })
      })

      expect(session).toEqual(newSession)
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

      mockFetch.mockResolvedValueOnce({
        ok: true,
        text: async () => JSON.stringify(session),
      })

      const { result } = renderHook(() => useSession(defaultConfig))

      let loadedSession
      await act(async () => {
        loadedSession = await result.current.loadSession('session-1')
      })

      expect(loadedSession).toEqual(session)
      expect(result.current.currentSession).toEqual(session)
      expect(result.current.currentSessionLoading).toBe(false)
    })

    it('should handle load error', async () => {
      mockFetch.mockResolvedValueOnce({
        ok: false,
        status: 404,
        text: async () => 'Session not found',
      })

      const { result } = renderHook(() => useSession(defaultConfig))

      await act(async () => {
        try {
          await result.current.loadSession('session-missing')
        } catch {
          // Expected error
        }
      })

      expect(result.current.currentSessionError).toBe('API error 404: Session not found')
    })
  })

  describe('updateSessionTitle', () => {
    it('should update session title', async () => {
      // First load the session
      const session = {
        id: 'session-1',
        projectPath: '/test/project',
        provider: 'anthropic',
        model: 'claude-3-opus',
        title: 'Original Title',
        status: 'active',
        createdAt: '2024-01-01T00:00:00Z',
        updatedAt: '2024-01-01T00:00:00Z',
      }

      mockFetch
        .mockResolvedValueOnce({
          ok: true,
          text: async () => JSON.stringify([{ ...session, messageCount: 0, totalCostUsd: 0 }]),
        })
        .mockResolvedValueOnce({
          ok: true,
          text: async () => JSON.stringify(session),
        })
        .mockResolvedValueOnce({
          ok: true,
          text: async () => '',
        })

      const { result } = renderHook(() => useSession(defaultConfig))

      // Fetch sessions first
      await act(async () => {
        await result.current.fetchSessions()
      })

      // Load the session
      await act(async () => {
        await result.current.loadSession('session-1')
      })

      // Update the title
      await act(async () => {
        await result.current.updateSessionTitle('session-1', 'New Title')
      })

      expect(result.current.sessions[0].title).toBe('New Title')
      expect(result.current.currentSession?.title).toBe('New Title')
    })
  })

  describe('archiveSession', () => {
    it('should archive a session', async () => {
      const session = {
        id: 'session-1',
        title: 'Test Session',
        provider: 'anthropic',
        model: 'claude-3-opus',
        status: 'active',
        messageCount: 0,
        totalCostUsd: 0,
        createdAt: '2024-01-01T00:00:00Z',
        updatedAt: '2024-01-01T00:00:00Z',
      }

      mockFetch
        .mockResolvedValueOnce({
          ok: true,
          text: async () => JSON.stringify([session]),
        })
        .mockResolvedValueOnce({
          ok: true,
          text: async () => '',
        })

      const { result } = renderHook(() => useSession(defaultConfig))

      await act(async () => {
        await result.current.fetchSessions()
      })

      await act(async () => {
        await result.current.archiveSession('session-1')
      })

      expect(result.current.sessions[0].status).toBe('archived')
    })
  })

  describe('forkSession', () => {
    it('should fork a session', async () => {
      const forkedSession = {
        id: 'session-forked',
        projectPath: '/test/project',
        provider: 'anthropic',
        model: 'claude-3-opus',
        title: 'Forked Session',
        parentSessionId: 'session-1',
        forkPointMessageId: 'msg-5',
        status: 'active',
        createdAt: '2024-01-01T00:00:00Z',
        updatedAt: '2024-01-01T00:00:00Z',
      }

      mockFetch.mockResolvedValueOnce({
        ok: true,
        text: async () => JSON.stringify(forkedSession),
      })

      const { result } = renderHook(() => useSession(defaultConfig))

      let session
      await act(async () => {
        session = await result.current.forkSession('session-1', 'msg-5')
      })

      expect(session).toEqual(forkedSession)
      expect(result.current.sessions).toHaveLength(1)
      expect(result.current.sessions[0].id).toBe('session-forked')
    })
  })

  describe('clearCurrentSession', () => {
    it('should clear current session', async () => {
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

      mockFetch.mockResolvedValueOnce({
        ok: true,
        text: async () => JSON.stringify(session),
      })

      const { result } = renderHook(() => useSession(defaultConfig))

      await act(async () => {
        await result.current.loadSession('session-1')
      })

      expect(result.current.currentSession).toBeDefined()

      act(() => {
        result.current.clearCurrentSession()
      })

      expect(result.current.currentSession).toBeUndefined()
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

      mockFetch.mockResolvedValueOnce({
        ok: true,
        text: async () => JSON.stringify(sessions),
      })

      const { result } = renderHook(() => useSession(defaultConfig))

      await act(async () => {
        await result.current.fetchSessions()
      })

      expect(localStorageMock.setItem).toHaveBeenCalledWith(
        'openexec-sessions',
        expect.stringContaining('session-1')
      )
    })

    it('should load sessions from localStorage on mount', () => {
      const cachedSessions = {
        sessions: [
          {
            id: 'cached-session',
            title: 'Cached Session',
            provider: 'anthropic',
            model: 'claude-3-opus',
            status: 'active',
            messageCount: 0,
            totalCostUsd: 0,
            createdAt: '2024-01-01T00:00:00Z',
            updatedAt: '2024-01-01T00:00:00Z',
          },
        ],
        timestamp: Date.now(),
      }

      localStorageMock.getItem.mockReturnValueOnce(JSON.stringify(cachedSessions))

      const { result } = renderHook(() => useSession(defaultConfig))

      expect(result.current.sessions).toHaveLength(1)
      expect(result.current.sessions[0].id).toBe('cached-session')
    })

    it('should not load stale sessions from localStorage', () => {
      const cachedSessions = {
        sessions: [
          {
            id: 'stale-session',
            title: 'Stale Session',
            provider: 'anthropic',
            model: 'claude-3-opus',
            status: 'active',
            messageCount: 0,
            totalCostUsd: 0,
            createdAt: '2024-01-01T00:00:00Z',
            updatedAt: '2024-01-01T00:00:00Z',
          },
        ],
        timestamp: Date.now() - 10 * 60 * 1000, // 10 minutes ago (stale)
      }

      localStorageMock.getItem.mockReturnValueOnce(JSON.stringify(cachedSessions))

      const { result } = renderHook(() => useSession(defaultConfig))

      expect(result.current.sessions).toHaveLength(0)
    })
  })
})
