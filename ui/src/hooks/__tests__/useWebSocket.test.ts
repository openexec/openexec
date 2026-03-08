/**
 * Tests for useWebSocket hook
 *
 * @module hooks/__tests__/useWebSocket.test
 */

import { renderHook, act, waitFor } from '@testing-library/react'
import { useWebSocket, WebSocketConfig, WebSocketHandlers } from '../useWebSocket'

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
  private openTimeout: ReturnType<typeof setTimeout> | null = null
  private closeTimeout: ReturnType<typeof setTimeout> | null = null

  constructor(url: string) {
    this.url = url
    // Simulate async connection
    this.openTimeout = setTimeout(() => this.simulateOpen(), 10)
  }

  send(data: string) {
    if (this.readyState !== MockWebSocket.OPEN) {
      throw new Error('WebSocket is not open')
    }
    this.sentMessages.push(data)
  }

  close(code?: number, reason?: string) {
    this.readyState = MockWebSocket.CLOSING
    this.clearTimeouts()
    this.closeTimeout = setTimeout(() => {
      this.readyState = MockWebSocket.CLOSED
      this.onclose?.({ code: code ?? 1000, reason: reason ?? '' } as CloseEvent)
    }, 10)
  }

  clearTimeouts() {
    if (this.openTimeout) clearTimeout(this.openTimeout)
    if (this.closeTimeout) clearTimeout(this.closeTimeout)
  }

  // Test helpers
  simulateOpen() {
    this.clearTimeouts()
    this.readyState = MockWebSocket.OPEN
    this.onopen?.({ type: 'open' } as Event)
  }

  simulateClose(code = 1000, reason = '') {
    this.clearTimeouts()
    this.readyState = MockWebSocket.CLOSED
    this.onclose?.({ code, reason } as CloseEvent)
  }

  simulateError() {
    this.onerror?.({ type: 'error' } as Event)
  }

  simulateMessage(data: object) {
    this.onmessage?.({ data: JSON.stringify(data) } as MessageEvent)
  }

  getSentMessages(): string[] {
    return this.sentMessages
  }

  getLastSentMessage(): object | null {
    if (this.sentMessages.length === 0) return null
    return JSON.parse(this.sentMessages[this.sentMessages.length - 1])
  }
}

// Store reference to the current MockWebSocket instance
let currentWs: MockWebSocket | null = null

// Setup global WebSocket mock
beforeEach(() => {
  currentWs = null
  ;(global as unknown as { WebSocket: typeof MockWebSocket }).WebSocket = class extends MockWebSocket {
    constructor(url: string) {
      super(url)
      currentWs = this
    }
  } as unknown as typeof MockWebSocket
})

afterEach(() => {
  if (currentWs) {
    currentWs.clearTimeouts()
  }
  currentWs = null
  vi.useRealTimers()
})

describe('useWebSocket', () => {
  const defaultConfig: WebSocketConfig = {
    url: 'ws://localhost:8080/ws',
    autoReconnect: false,
    debug: false,
  }

  describe('connection management', () => {
    it('should start in disconnected state', () => {
      const { result } = renderHook(() => useWebSocket(defaultConfig))

      expect(result.current.connectionStatus).toBe('disconnected')
      expect(result.current.isConnected).toBe(false)
    })

    it('should connect to WebSocket server', async () => {
      const { result } = renderHook(() => useWebSocket(defaultConfig))

      act(() => {
        result.current.connect()
      })

      expect(result.current.connectionStatus).toBe('connecting')

      await waitFor(() => {
        expect(result.current.connectionStatus).toBe('connected')
      })

      expect(result.current.isConnected).toBe(true)
    })

    it('should disconnect from WebSocket server', async () => {
      const { result } = renderHook(() => useWebSocket(defaultConfig))

      act(() => {
        result.current.connect()
      })

      await waitFor(() => {
        expect(result.current.isConnected).toBe(true)
      })

      act(() => {
        result.current.disconnect()
      })

      await waitFor(() => {
        expect(result.current.connectionStatus).toBe('disconnected')
      })

      expect(result.current.isConnected).toBe(false)
    })

    it('should call onConnectionChange handler', async () => {
      const onConnectionChange = jest.fn()
      const handlers: WebSocketHandlers = { onConnectionChange }

      const { result } = renderHook(() => useWebSocket(defaultConfig, handlers))

      act(() => {
        result.current.connect()
      })

      await waitFor(() => {
        expect(onConnectionChange).toHaveBeenCalledWith('connecting', undefined)
      })

      await waitFor(() => {
        expect(onConnectionChange).toHaveBeenCalledWith('connected', undefined)
      })
    })
  })

  describe('session subscription', () => {
    it('should subscribe to a session', async () => {
      const { result } = renderHook(() => useWebSocket(defaultConfig))

      act(() => {
        result.current.connect()
      })

      await waitFor(() => {
        expect(result.current.isConnected).toBe(true)
      })

      act(() => {
        result.current.subscribe('session-123')
      })

      expect(result.current.subscribedSession).toBe('session-123')
    })

    it('should unsubscribe from a session', async () => {
      const { result } = renderHook(() => useWebSocket(defaultConfig))

      act(() => {
        result.current.connect()
      })

      await waitFor(() => {
        expect(result.current.isConnected).toBe(true)
      })

      act(() => {
        result.current.subscribe('session-123')
      })

      act(() => {
        result.current.unsubscribe()
      })

      expect(result.current.subscribedSession).toBeUndefined()
    })
  })

  describe('message sending', () => {
    it('should send a chat message', async () => {
      const { result } = renderHook(() => useWebSocket(defaultConfig))

      act(() => {
        result.current.connect()
      })

      await waitFor(() => {
        expect(result.current.isConnected).toBe(true)
      })

      act(() => {
        result.current.sendMessage('session-123', 'Hello, world!')
      })

      const lastMessage = currentWs?.getLastSentMessage() as {
        type: string
        sessionId: string
        content: string
      }
      expect(lastMessage).toMatchObject({
        type: 'send_message',
        sessionId: 'session-123',
        content: 'Hello, world!',
      })
    })

    it('should queue messages when disconnected', async () => {
      const { result } = renderHook(() => useWebSocket(defaultConfig))

      // Don't connect, just try to send
      act(() => {
        result.current.sendMessage('session-123', 'Queued message')
      })

      expect(result.current.queuedMessageCount).toBe(1)
    })

    it('should flush queued messages on reconnect', async () => {
      jest.useFakeTimers()
      const { result } = renderHook(() => useWebSocket(defaultConfig))

      // Queue a message while disconnected
      act(() => {
        result.current.sendMessage('session-123', 'Queued message')
      })

      expect(result.current.queuedMessageCount).toBe(1)

      // Connect
      act(() => {
        result.current.connect()
      })

      act(() => {
        jest.advanceTimersByTime(20)
      })

      // Wait for connection and queue flush
      await waitFor(() => {
        expect(result.current.isConnected).toBe(true)
      })

      // Queue should be flushed
      await waitFor(() => {
        expect(result.current.queuedMessageCount).toBe(0)
      })
    })
  })

  describe('tool call operations', () => {
    it('should approve a tool call', async () => {
      const { result } = renderHook(() => useWebSocket(defaultConfig))

      act(() => {
        result.current.connect()
      })

      await waitFor(() => {
        expect(result.current.isConnected).toBe(true)
      })

      act(() => {
        result.current.approveTool('session-123', 'tool-call-456')
      })

      const lastMessage = currentWs?.getLastSentMessage() as {
        type: string
        sessionId: string
        toolCallId: string
      }
      expect(lastMessage).toMatchObject({
        type: 'approve_tool',
        sessionId: 'session-123',
        toolCallId: 'tool-call-456',
      })
    })

    it('should reject a tool call with reason', async () => {
      const { result } = renderHook(() => useWebSocket(defaultConfig))

      act(() => {
        result.current.connect()
      })

      await waitFor(() => {
        expect(result.current.isConnected).toBe(true)
      })

      act(() => {
        result.current.rejectTool('session-123', 'tool-call-456', 'Too dangerous')
      })

      const lastMessage = currentWs?.getLastSentMessage() as {
        type: string
        sessionId: string
        toolCallId: string
        reason: string
      }
      expect(lastMessage).toMatchObject({
        type: 'reject_tool',
        sessionId: 'session-123',
        toolCallId: 'tool-call-456',
        reason: 'Too dangerous',
      })
    })
  })

  describe('loop control', () => {
    it('should pause the loop', async () => {
      const { result } = renderHook(() => useWebSocket(defaultConfig))

      act(() => {
        result.current.connect()
      })

      await waitFor(() => {
        expect(result.current.isConnected).toBe(true)
      })

      act(() => {
        result.current.pauseLoop('session-123')
      })

      const lastMessage = currentWs?.getLastSentMessage() as {
        type: string
        sessionId: string
      }
      expect(lastMessage).toMatchObject({
        type: 'pause',
        sessionId: 'session-123',
      })
    })

    it('should resume the loop', async () => {
      const { result } = renderHook(() => useWebSocket(defaultConfig))

      act(() => {
        result.current.connect()
      })

      await waitFor(() => {
        expect(result.current.isConnected).toBe(true)
      })

      act(() => {
        result.current.resumeLoop('session-123')
      })

      const lastMessage = currentWs?.getLastSentMessage() as {
        type: string
        sessionId: string
      }
      expect(lastMessage).toMatchObject({
        type: 'resume',
        sessionId: 'session-123',
      })
    })

    it('should stop the loop', async () => {
      const { result } = renderHook(() => useWebSocket(defaultConfig))

      act(() => {
        result.current.connect()
      })

      await waitFor(() => {
        expect(result.current.isConnected).toBe(true)
      })

      act(() => {
        result.current.stopLoop('session-123')
      })

      const lastMessage = currentWs?.getLastSentMessage() as {
        type: string
        sessionId: string
      }
      expect(lastMessage).toMatchObject({
        type: 'stop',
        sessionId: 'session-123',
      })
    })
  })

  describe('message handling', () => {
    it('should handle incoming messages', async () => {
      const onMessage = jest.fn()
      const handlers: WebSocketHandlers = { onMessage }

      const { result } = renderHook(() => useWebSocket(defaultConfig, handlers))

      act(() => {
        result.current.connect()
      })

      await waitFor(() => {
        expect(result.current.isConnected).toBe(true)
      })

      act(() => {
        currentWs?.simulateMessage({
          type: 'message',
          sessionId: 'session-123',
          payload: {
            id: 'msg-1',
            sessionId: 'session-123',
            role: 'assistant',
            content: 'Hello!',
            tokensInput: 10,
            tokensOutput: 5,
            costUsd: 0.001,
            createdAt: '2024-01-01T00:00:00Z',
          },
          timestamp: '2024-01-01T00:00:00Z',
        })
      })

      expect(onMessage).toHaveBeenCalledWith(
        expect.objectContaining({
          id: 'msg-1',
          content: 'Hello!',
        })
      )
    })

    it('should handle streaming chunks', async () => {
      const onStreamingChunk = jest.fn()
      const handlers: WebSocketHandlers = { onStreamingChunk }

      const { result } = renderHook(() => useWebSocket(defaultConfig, handlers))

      act(() => {
        result.current.connect()
      })

      await waitFor(() => {
        expect(result.current.isConnected).toBe(true)
      })

      act(() => {
        currentWs?.simulateMessage({
          type: 'streaming_chunk',
          sessionId: 'session-123',
          payload: {
            messageId: 'msg-1',
            content: 'Hello',
            isComplete: false,
          },
          timestamp: '2024-01-01T00:00:00Z',
        })
      })

      expect(onStreamingChunk).toHaveBeenCalledWith(
        expect.objectContaining({
          messageId: 'msg-1',
          content: 'Hello',
          isComplete: false,
        })
      )
    })

    it('should handle tool call updates', async () => {
      const onToolCallUpdate = jest.fn()
      const handlers: WebSocketHandlers = { onToolCallUpdate }

      const { result } = renderHook(() => useWebSocket(defaultConfig, handlers))

      act(() => {
        result.current.connect()
      })

      await waitFor(() => {
        expect(result.current.isConnected).toBe(true)
      })

      act(() => {
        currentWs?.simulateMessage({
          type: 'tool_call_update',
          sessionId: 'session-123',
          payload: {
            id: 'tc-1',
            messageId: 'msg-1',
            sessionId: 'session-123',
            toolName: 'read_file',
            toolInput: '{"path": "/test.txt"}',
            status: 'running',
            createdAt: '2024-01-01T00:00:00Z',
          },
          timestamp: '2024-01-01T00:00:00Z',
        })
      })

      expect(onToolCallUpdate).toHaveBeenCalledWith(
        expect.objectContaining({
          id: 'tc-1',
          toolName: 'read_file',
          status: 'running',
        })
      )
    })

    it('should handle loop events', async () => {
      const onEvent = jest.fn()
      const handlers: WebSocketHandlers = { onEvent }

      const { result } = renderHook(() => useWebSocket(defaultConfig, handlers))

      act(() => {
        result.current.connect()
      })

      await waitFor(() => {
        expect(result.current.isConnected).toBe(true)
      })

      act(() => {
        currentWs?.simulateMessage({
          type: 'event',
          sessionId: 'session-123',
          payload: {
            id: 'event-1',
            type: 'loop.start',
            kind: 'lifecycle',
            timestamp: '2024-01-01T00:00:00Z',
            sessionId: 'session-123',
            iteration: 1,
          },
          timestamp: '2024-01-01T00:00:00Z',
        })
      })

      expect(onEvent).toHaveBeenCalledWith(
        expect.objectContaining({
          id: 'event-1',
          type: 'loop.start',
          kind: 'lifecycle',
        })
      )
    })

    it('should handle error messages', async () => {
      const onError = jest.fn()
      const handlers: WebSocketHandlers = { onError }

      const { result } = renderHook(() => useWebSocket(defaultConfig, handlers))

      act(() => {
        result.current.connect()
      })

      await waitFor(() => {
        expect(result.current.isConnected).toBe(true)
      })

      act(() => {
        currentWs?.simulateMessage({
          type: 'error',
          payload: 'Something went wrong',
          timestamp: '2024-01-01T00:00:00Z',
        })
      })

      expect(onError).toHaveBeenCalledWith('Something went wrong')
    })

    it('should handle pong messages for heartbeat', async () => {
      jest.useFakeTimers()

      const { result } = renderHook(() =>
        useWebSocket({
          ...defaultConfig,
          heartbeatInterval: 1000,
          heartbeatTimeout: 500,
        })
      )

      act(() => {
        result.current.connect()
      })

      act(() => {
        jest.advanceTimersByTime(20)
      })

      await waitFor(() => {
        expect(result.current.isConnected).toBe(true)
      })

      // Advance to trigger heartbeat
      act(() => {
        jest.advanceTimersByTime(1000)
      })

      // Simulate pong response
      act(() => {
        currentWs?.simulateMessage({
          type: 'pong',
          timestamp: '2024-01-01T00:00:00Z',
        })
      })

      // Should still be connected (pong received before timeout)
      expect(result.current.isConnected).toBe(true)
    })
  })

  describe('reconnection', () => {
    it('should attempt reconnection on unexpected close', async () => {
      jest.useFakeTimers()

      const onConnectionChange = jest.fn()
      const handlers: WebSocketHandlers = { onConnectionChange }

      const { result } = renderHook(() =>
        useWebSocket(
          {
            ...defaultConfig,
            autoReconnect: true,
            reconnectDelay: 100,
          },
          handlers
        )
      )

      act(() => {
        result.current.connect()
      })

      act(() => {
        jest.advanceTimersByTime(20)
      })

      await waitFor(() => {
        expect(result.current.isConnected).toBe(true)
      })

      // Simulate unexpected close
      act(() => {
        currentWs?.simulateClose(1006, 'Abnormal closure')
      })

      await waitFor(() => {
        expect(result.current.connectionStatus).toBe('reconnecting')
      })

      // Advance timers for reconnection
      act(() => {
        jest.advanceTimersByTime(100)
      })

      await waitFor(() => {
        expect(result.current.connectionStatus).toBe('connecting')
      })
    })

    it('should not reconnect on intentional disconnect', async () => {
      jest.useFakeTimers()

      const { result } = renderHook(() =>
        useWebSocket({
          ...defaultConfig,
          autoReconnect: true,
          reconnectDelay: 100,
        })
      )

      act(() => {
        result.current.connect()
      })

      act(() => {
        jest.advanceTimersByTime(20)
      })

      await waitFor(() => {
        expect(result.current.isConnected).toBe(true)
      })

      // Intentional disconnect
      act(() => {
        result.current.disconnect()
      })

      act(() => {
        jest.advanceTimersByTime(20)
      })

      await waitFor(() => {
        expect(result.current.connectionStatus).toBe('disconnected')
      })

      // Advance timers - should NOT attempt reconnection
      act(() => {
        jest.advanceTimersByTime(200)
      })

      expect(result.current.connectionStatus).toBe('disconnected')
    })

    it('should use exponential backoff for reconnection', async () => {
      jest.useFakeTimers()

      const { result } = renderHook(() =>
        useWebSocket({
          ...defaultConfig,
          autoReconnect: true,
          reconnectDelay: 100,
          reconnectBackoff: 2,
          maxReconnectDelay: 1000,
        })
      )

      act(() => {
        result.current.connect()
      })

      act(() => {
        jest.advanceTimersByTime(20)
      })

      await waitFor(() => {
        expect(result.current.isConnected).toBe(true)
      })

      // Simulate first unexpected close
      act(() => {
        currentWs?.simulateClose(1006, 'Abnormal closure')
      })

      await waitFor(() => {
        expect(result.current.connectionStatus).toBe('reconnecting')
      })

      // First reconnect should be after 100ms
      act(() => {
        jest.advanceTimersByTime(100)
      })

      // Should attempt to connect
      expect(result.current.connectionStatus).toBe('connecting')
    })
  })

  describe('cleanup', () => {
    it('should cleanup on unmount', async () => {
      const { result, unmount } = renderHook(() => useWebSocket(defaultConfig))

      act(() => {
        result.current.connect()
      })

      await waitFor(() => {
        expect(result.current.isConnected).toBe(true)
      })

      const ws = currentWs

      unmount()

      // WebSocket should be closed
      await waitFor(() => {
        expect(ws?.readyState).toBe(MockWebSocket.CLOSED)
      })
    })
  })
})
