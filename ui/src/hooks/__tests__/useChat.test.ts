/**
 * Tests for useChat hook
 *
 * @module hooks/__tests__/useChat.test
 */

import { renderHook, act, waitFor } from '@testing-library/react'
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { useChat, type ChatConfig } from '../useChat'

// Mock WebSocket
class MockWebSocket {
  static CONNECTING = 0
  static OPEN = 1
  static CLOSING = 2
  static CLOSED = 3

  url: string
  readyState: number = MockWebSocket.CONNECTING
  onopen: ((event: Event) => void) | null = null
  onclose: ((event: CloseEvent) => void) | null = null
  onerror: ((event: Event) => void) | null = null
  onmessage: ((event: MessageEvent) => void) | null = null

  private sentMessages: string[] = []

  constructor(url: string) {
    this.url = url
    setTimeout(() => this.simulateOpen(), 10)
  }

  send(data: string) {
    if (this.readyState !== MockWebSocket.OPEN) {
      throw new Error('WebSocket is not open')
    }
    this.sentMessages.push(data)
  }

  close(code?: number, reason?: string) {
    this.readyState = MockWebSocket.CLOSING
    setTimeout(() => {
      this.readyState = MockWebSocket.CLOSED
      this.onclose?.({ code: code ?? 1000, reason: reason ?? '' } as CloseEvent)
    }, 10)
  }

  simulateOpen() {
    this.readyState = MockWebSocket.OPEN
    this.onopen?.({ type: 'open' } as Event)
  }

  simulateMessage(data: object) {
    this.onmessage?.({ data: JSON.stringify(data) } as MessageEvent)
  }

  getSentMessages(): string[] {
    return this.sentMessages
  }
}

// Store reference to current WebSocket instance
let currentWs: MockWebSocket | null = null

// Mock fetch
const mockFetch = vi.fn()

// Setup mocks
beforeEach(() => {
  currentWs = null
  mockFetch.mockClear()
  vi.stubGlobal('fetch', mockFetch)
  vi.stubGlobal('WebSocket', class extends MockWebSocket {
    constructor(url: string) {
      super(url)
      currentWs = this
    }
  })
})

afterEach(() => {
  currentWs = null
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

describe('useChat', () => {
  const defaultConfig: ChatConfig = {
    wsUrl: 'ws://localhost:8080/ws',
    apiUrl: 'http://localhost:8080',
    authToken: 'test-token',
    debug: false,
    wsOptions: {
      autoReconnect: false,
    },
  }

  describe('initialization', () => {
    it('should auto-connect on mount', async () => {
      const { result } = renderHook(() => useChat(defaultConfig))

      // Should start connecting
      await waitFor(() => {
        expect(result.current.connectionStatus).toBe('connected')
      })

      expect(result.current.isConnected).toBe(true)
    })
  })

  describe('session management', () => {
    it('should fetch sessions', async () => {
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

      const { result } = renderHook(() => useChat(defaultConfig))

      await waitFor(() => {
        expect(result.current.isConnected).toBe(true)
      })

      await act(async () => {
        await result.current.fetchSessions()
      })

      expect(result.current.sessions).toEqual(sessions)
    })

    it('should load an existing session', async () => {
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

      const messagesResponse = {
        messages: [
          {
            id: 'msg-1',
            sessionId: 'session-1',
            role: 'user',
            content: 'Hello',
            tokensInput: 5,
            tokensOutput: 0,
            costUsd: 0,
            createdAt: '2024-01-01T00:00:00Z',
          },
        ],
        pagination: { offset: 0, limit: 50, hasMore: false, totalCount: 1 },
      }

      mockFetch
        .mockResolvedValueOnce(createMockResponse(session))
        .mockResolvedValueOnce(createMockResponse(messagesResponse))

      const { result } = renderHook(() => useChat(defaultConfig))

      await waitFor(() => {
        expect(result.current.isConnected).toBe(true)
      })

      await act(async () => {
        await result.current.loadSession('session-1')
      })

      expect(result.current.currentSession).toEqual(session)
      expect(result.current.messages).toEqual(messagesResponse.messages)
    })
  })

  describe('message sending', () => {
    it('should send a message via WebSocket', async () => {
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

      const messagesResponse = {
        messages: [],
        pagination: { offset: 0, limit: 50, hasMore: false, totalCount: 0 },
      }

      mockFetch
        .mockResolvedValueOnce(createMockResponse(session))
        .mockResolvedValueOnce(createMockResponse(messagesResponse))

      const { result } = renderHook(() => useChat(defaultConfig))

      await waitFor(() => {
        expect(result.current.isConnected).toBe(true)
      })

      await act(async () => {
        await result.current.loadSession('session-1')
      })

      await act(async () => {
        await result.current.sendMessage('Hello, AI!')
      })

      // Should have sent message via WebSocket
      const sentMessages = currentWs?.getSentMessages() ?? []
      const messagePayload = sentMessages.find((m) => {
        const parsed = JSON.parse(m)
        return parsed.type === 'send_message'
      })

      expect(messagePayload).toBeDefined()
      expect(JSON.parse(messagePayload!)).toMatchObject({
        type: 'send_message',
        sessionId: 'session-1',
        content: 'Hello, AI!',
      })
    })
  })
})
