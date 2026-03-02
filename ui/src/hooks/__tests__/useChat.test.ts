/**
 * Tests for useChat hook
 *
 * @module hooks/__tests__/useChat.test
 */

import { renderHook, act, waitFor } from '@testing-library/react'
import { useChat, ChatConfig } from '../useChat'

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
const mockFetch = jest.fn()
global.fetch = mockFetch

// Setup mocks
beforeEach(() => {
  currentWs = null
  mockFetch.mockClear()
  ;(global as unknown as { WebSocket: typeof MockWebSocket }).WebSocket = class extends MockWebSocket {
    constructor(url: string) {
      super(url)
      currentWs = this
    }
  } as unknown as typeof MockWebSocket
})

afterEach(() => {
  currentWs = null
  jest.useRealTimers()
})

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

    it('should disconnect on unmount', async () => {
      const { result, unmount } = renderHook(() => useChat(defaultConfig))

      await waitFor(() => {
        expect(result.current.isConnected).toBe(true)
      })

      const ws = currentWs

      unmount()

      await waitFor(() => {
        expect(ws?.readyState).toBe(MockWebSocket.CLOSED)
      })
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

      mockFetch.mockResolvedValueOnce({
        ok: true,
        text: async () => JSON.stringify(sessions),
      })

      const { result } = renderHook(() => useChat(defaultConfig))

      await waitFor(() => {
        expect(result.current.isConnected).toBe(true)
      })

      await act(async () => {
        await result.current.fetchSessions()
      })

      expect(result.current.sessions).toEqual(sessions)
    })

    it('should create and load a session', async () => {
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

      const messagesResponse = {
        messages: [],
        pagination: { offset: 0, limit: 50, hasMore: false, totalCount: 0 },
      }

      mockFetch
        .mockResolvedValueOnce({
          ok: true,
          text: async () => JSON.stringify(newSession),
        })
        .mockResolvedValueOnce({
          ok: true,
          text: async () => JSON.stringify(newSession),
        })
        .mockResolvedValueOnce({
          ok: true,
          text: async () => JSON.stringify(messagesResponse),
        })

      const { result } = renderHook(() => useChat(defaultConfig))

      await waitFor(() => {
        expect(result.current.isConnected).toBe(true)
      })

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
      expect(result.current.currentSession).toBeDefined()
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
        .mockResolvedValueOnce({
          ok: true,
          text: async () => JSON.stringify(session),
        })
        .mockResolvedValueOnce({
          ok: true,
          text: async () => JSON.stringify(messagesResponse),
        })

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
        .mockResolvedValueOnce({
          ok: true,
          text: async () => JSON.stringify(session),
        })
        .mockResolvedValueOnce({
          ok: true,
          text: async () => JSON.stringify(messagesResponse),
        })

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

      // Should have added optimistic message locally
      expect(result.current.messages).toHaveLength(1)
      expect(result.current.messages[0].content).toBe('Hello, AI!')
      expect(result.current.messages[0].role).toBe('user')
    })

    it('should clear input after sending', async () => {
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
        .mockResolvedValueOnce({
          ok: true,
          text: async () => JSON.stringify(session),
        })
        .mockResolvedValueOnce({
          ok: true,
          text: async () => JSON.stringify(messagesResponse),
        })

      const { result } = renderHook(() => useChat(defaultConfig))

      await waitFor(() => {
        expect(result.current.isConnected).toBe(true)
      })

      await act(async () => {
        await result.current.loadSession('session-1')
      })

      act(() => {
        result.current.setInputContent('Hello!')
      })

      expect(result.current.inputContent).toBe('Hello!')

      await act(async () => {
        await result.current.sendMessage('Hello!')
      })

      expect(result.current.inputContent).toBe('')
    })
  })

  describe('WebSocket message handling', () => {
    it('should handle incoming assistant messages', async () => {
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
        .mockResolvedValueOnce({
          ok: true,
          text: async () => JSON.stringify(session),
        })
        .mockResolvedValueOnce({
          ok: true,
          text: async () => JSON.stringify(messagesResponse),
        })

      const { result } = renderHook(() => useChat(defaultConfig))

      await waitFor(() => {
        expect(result.current.isConnected).toBe(true)
      })

      await act(async () => {
        await result.current.loadSession('session-1')
      })

      // Simulate incoming message from server
      act(() => {
        currentWs?.simulateMessage({
          type: 'message',
          sessionId: 'session-1',
          payload: {
            id: 'msg-assistant',
            sessionId: 'session-1',
            role: 'assistant',
            content: 'Hello! How can I help you?',
            tokensInput: 10,
            tokensOutput: 20,
            costUsd: 0.002,
            createdAt: '2024-01-01T00:00:00Z',
          },
          timestamp: '2024-01-01T00:00:00Z',
        })
      })

      expect(result.current.messages).toHaveLength(1)
      expect(result.current.messages[0].role).toBe('assistant')
      expect(result.current.messages[0].content).toBe('Hello! How can I help you?')
    })

    it('should handle streaming chunks', async () => {
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
        .mockResolvedValueOnce({
          ok: true,
          text: async () => JSON.stringify(session),
        })
        .mockResolvedValueOnce({
          ok: true,
          text: async () => JSON.stringify(messagesResponse),
        })

      const { result } = renderHook(() => useChat(defaultConfig))

      await waitFor(() => {
        expect(result.current.isConnected).toBe(true)
      })

      await act(async () => {
        await result.current.loadSession('session-1')
      })

      // Simulate streaming chunks
      act(() => {
        currentWs?.simulateMessage({
          type: 'streaming_chunk',
          sessionId: 'session-1',
          payload: {
            messageId: 'msg-streaming',
            content: 'Hello',
            isComplete: false,
          },
          timestamp: '2024-01-01T00:00:00Z',
        })
      })

      expect(result.current.streamingMessage).toBeDefined()
      expect(result.current.streamingMessage?.content).toBe('Hello')

      act(() => {
        currentWs?.simulateMessage({
          type: 'streaming_chunk',
          sessionId: 'session-1',
          payload: {
            messageId: 'msg-streaming',
            content: ' World!',
            isComplete: false,
          },
          timestamp: '2024-01-01T00:00:01Z',
        })
      })

      expect(result.current.streamingMessage?.content).toBe('Hello World!')
    })

    it('should handle loop events', async () => {
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
        .mockResolvedValueOnce({
          ok: true,
          text: async () => JSON.stringify(session),
        })
        .mockResolvedValueOnce({
          ok: true,
          text: async () => JSON.stringify(messagesResponse),
        })

      const { result } = renderHook(() => useChat(defaultConfig))

      await waitFor(() => {
        expect(result.current.isConnected).toBe(true)
      })

      await act(async () => {
        await result.current.loadSession('session-1')
      })

      // Simulate loop start event
      act(() => {
        currentWs?.simulateMessage({
          type: 'event',
          sessionId: 'session-1',
          payload: {
            id: 'event-1',
            type: 'loop.start',
            kind: 'lifecycle',
            timestamp: '2024-01-01T00:00:00Z',
            sessionId: 'session-1',
          },
          timestamp: '2024-01-01T00:00:00Z',
        })
      })

      expect(result.current.loopState.isRunning).toBe(true)
      expect(result.current.events).toHaveLength(1)
      expect(result.current.events[0].type).toBe('loop.start')
    })
  })

  describe('loop control', () => {
    it('should pause the loop', async () => {
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
        .mockResolvedValueOnce({
          ok: true,
          text: async () => JSON.stringify(session),
        })
        .mockResolvedValueOnce({
          ok: true,
          text: async () => JSON.stringify(messagesResponse),
        })

      const { result } = renderHook(() => useChat(defaultConfig))

      await waitFor(() => {
        expect(result.current.isConnected).toBe(true)
      })

      await act(async () => {
        await result.current.loadSession('session-1')
      })

      act(() => {
        result.current.pauseLoop()
      })

      const sentMessages = currentWs?.getSentMessages() ?? []
      const pauseMessage = sentMessages.find((m) => {
        const parsed = JSON.parse(m)
        return parsed.type === 'pause'
      })

      expect(pauseMessage).toBeDefined()
    })

    it('should resume the loop', async () => {
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
        .mockResolvedValueOnce({
          ok: true,
          text: async () => JSON.stringify(session),
        })
        .mockResolvedValueOnce({
          ok: true,
          text: async () => JSON.stringify(messagesResponse),
        })

      const { result } = renderHook(() => useChat(defaultConfig))

      await waitFor(() => {
        expect(result.current.isConnected).toBe(true)
      })

      await act(async () => {
        await result.current.loadSession('session-1')
      })

      act(() => {
        result.current.resumeLoop()
      })

      const sentMessages = currentWs?.getSentMessages() ?? []
      const resumeMessage = sentMessages.find((m) => {
        const parsed = JSON.parse(m)
        return parsed.type === 'resume'
      })

      expect(resumeMessage).toBeDefined()
    })

    it('should stop the loop', async () => {
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
        .mockResolvedValueOnce({
          ok: true,
          text: async () => JSON.stringify(session),
        })
        .mockResolvedValueOnce({
          ok: true,
          text: async () => JSON.stringify(messagesResponse),
        })

      const { result } = renderHook(() => useChat(defaultConfig))

      await waitFor(() => {
        expect(result.current.isConnected).toBe(true)
      })

      await act(async () => {
        await result.current.loadSession('session-1')
      })

      act(() => {
        result.current.stopLoop()
      })

      const sentMessages = currentWs?.getSentMessages() ?? []
      const stopMessage = sentMessages.find((m) => {
        const parsed = JSON.parse(m)
        return parsed.type === 'stop'
      })

      expect(stopMessage).toBeDefined()
    })
  })

  describe('reset', () => {
    it('should reset all state', async () => {
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
        .mockResolvedValueOnce({
          ok: true,
          text: async () => JSON.stringify(session),
        })
        .mockResolvedValueOnce({
          ok: true,
          text: async () => JSON.stringify(messagesResponse),
        })

      const { result } = renderHook(() => useChat(defaultConfig))

      await waitFor(() => {
        expect(result.current.isConnected).toBe(true)
      })

      await act(async () => {
        await result.current.loadSession('session-1')
      })

      expect(result.current.messages).toHaveLength(1)

      act(() => {
        result.current.reset()
      })

      expect(result.current.messages).toHaveLength(0)
      expect(result.current.currentSession).toBeUndefined()
      expect(result.current.events).toHaveLength(0)
      expect(result.current.connectionStatus).toBe('disconnected')
    })
  })
})
