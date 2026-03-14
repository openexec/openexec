/**
 * WebSocket Hook for Chat UI Session Management
 *
 * Provides WebSocket connection management with:
 * - Automatic reconnection with exponential backoff
 * - Heartbeat/ping-pong for connection health
 * - Message queueing during offline periods
 * - Session subscription management
 * - Type-safe message handling
 *
 * @module hooks/useWebSocket
 */

import { useCallback, useEffect, useRef, useState } from 'react'
import type {
  WebSocketMessage,
  WebSocketMessageType,
  ClientMessage,
  Message,
  ToolCall,
  LoopEvent,
  StreamingChunk,
  CostInfo,
  AgentLoopState,
} from '../types'
import type { ConnectionStatus } from '../types/store'

// =============================================================================
// Configuration
// =============================================================================

export interface WebSocketConfig {
  /** WebSocket server URL */
  url: string
  /** Enable automatic reconnection (default: true) */
  autoReconnect?: boolean
  /** Initial reconnection delay in ms (default: 1000) */
  reconnectDelay?: number
  /** Maximum reconnection delay in ms (default: 30000) */
  maxReconnectDelay?: number
  /** Reconnection backoff multiplier (default: 2) */
  reconnectBackoff?: number
  /** Maximum reconnection attempts (default: Infinity) */
  maxReconnectAttempts?: number
  /** Heartbeat interval in ms (default: 30000) */
  heartbeatInterval?: number
  /** Heartbeat timeout in ms (default: 10000) */
  heartbeatTimeout?: number
  /** Enable debug logging (default: false) */
  debug?: boolean
}

const DEFAULT_CONFIG: Required<Omit<WebSocketConfig, 'url'>> = {
  autoReconnect: true,
  reconnectDelay: 1000,
  maxReconnectDelay: 30000,
  reconnectBackoff: 2,
  maxReconnectAttempts: Infinity,
  heartbeatInterval: 30000,
  heartbeatTimeout: 10000,
  debug: false,
}

// =============================================================================
// Event Handlers Interface
// =============================================================================

export interface WebSocketHandlers {
  /** Called when connection status changes */
  onConnectionChange?: (status: ConnectionStatus, error?: string) => void
  /** Called when a complete message is received */
  onMessage?: (message: Message) => void
  /** Called when a streaming chunk is received */
  onStreamingChunk?: (chunk: StreamingChunk) => void
  /** Called when a tool call update is received */
  onToolCallUpdate?: (toolCall: ToolCall) => void
  /** Called when a loop event is received */
  onEvent?: (event: LoopEvent) => void
  /** Called when cost info is updated */
  onCostUpdate?: (cost: CostInfo) => void
  /** Called when loop state changes */
  onLoopStateChange?: (state: Partial<AgentLoopState>) => void
  /** Called when an error is received from server */
  onError?: (error: string) => void
  /** Called for any raw WebSocket message (for debugging) */
  onRawMessage?: (message: WebSocketMessage) => void
}

// =============================================================================
// Hook Return Type
// =============================================================================

export interface UseWebSocketReturn {
  /** Current connection status */
  connectionStatus: ConnectionStatus
  /** Connection error message, if any */
  connectionError: string | undefined
  /** Whether the socket is connected */
  isConnected: boolean
  /** Currently subscribed session ID */
  subscribedSession: string | undefined
  /** Number of queued messages */
  queuedMessageCount: number
  /** Connect to WebSocket server */
  connect: () => void
  /** Disconnect from WebSocket server */
  disconnect: () => void
  /** Subscribe to a session's events */
  subscribe: (sessionId: string) => void
  /** Unsubscribe from current session */
  unsubscribe: () => void
  /** Send a chat message */
  sendMessage: (sessionId: string, content: string) => void
  /** Approve a tool call */
  approveTool: (sessionId: string, toolCallId: string) => void
  /** Reject a tool call */
  rejectTool: (sessionId: string, toolCallId: string, reason?: string) => void
  /** Pause the agent loop */
  pauseLoop: (sessionId: string) => void
  /** Resume the agent loop */
  resumeLoop: (sessionId: string) => void
  /** Stop the agent loop */
  stopLoop: (sessionId: string) => void
}

// =============================================================================
// Internal Types
// =============================================================================

interface QueuedMessage {
  message: ClientMessage
  timestamp: number
}

// =============================================================================
// useWebSocket Hook
// =============================================================================

export function useWebSocket(
  config: WebSocketConfig,
  handlers: WebSocketHandlers = {}
): UseWebSocketReturn {
  const configRef = useRef({ ...DEFAULT_CONFIG, ...config })
  const handlersRef = useRef(handlers)

  // Update refs when props change
  useEffect(() => {
    configRef.current = { ...DEFAULT_CONFIG, ...config }
  }, [config])

  useEffect(() => {
    handlersRef.current = handlers
  }, [handlers])

  // State
  const [connectionStatus, setConnectionStatus] = useState<ConnectionStatus>('disconnected')
  const [connectionError, setConnectionError] = useState<string | undefined>()
  const [subscribedSession, setSubscribedSession] = useState<string | undefined>()
  const [queuedMessageCount, setQueuedMessageCount] = useState(0)

  // Refs for WebSocket management
  const wsRef = useRef<WebSocket | null>(null)
  const reconnectAttempts = useRef(0)
  const reconnectTimeout = useRef<ReturnType<typeof setTimeout> | null>(null)
  const heartbeatInterval = useRef<ReturnType<typeof setInterval> | null>(null)
  const heartbeatTimeout = useRef<ReturnType<typeof setTimeout> | null>(null)
  const messageQueue = useRef<QueuedMessage[]>([])
  const isIntentionalDisconnect = useRef(false)

  // Debug logging
  const log = useCallback((...args: unknown[]) => {
    if (configRef.current.debug) {
      console.log('[WebSocket]', ...args)
    }
  }, [])

  // Update connection status with handler notification
  const updateConnectionStatus = useCallback(
    (status: ConnectionStatus, error?: string) => {
      log('Status change:', status, error)
      setConnectionStatus(status)
      setConnectionError(error)
      handlersRef.current.onConnectionChange?.(status, error)
    },
    [log]
  )

  // Clear all timers
  const clearTimers = useCallback(() => {
    if (reconnectTimeout.current) {
      clearTimeout(reconnectTimeout.current)
      reconnectTimeout.current = null
    }
    if (heartbeatInterval.current) {
      clearInterval(heartbeatInterval.current)
      heartbeatInterval.current = null
    }
    if (heartbeatTimeout.current) {
      clearTimeout(heartbeatTimeout.current)
      heartbeatTimeout.current = null
    }
  }, [])

  // Send message over WebSocket
  const sendRaw = useCallback(
    (message: ClientMessage | { type: string; timestamp: string }) => {
      if (wsRef.current?.readyState === WebSocket.OPEN) {
        const payload = JSON.stringify(message)
        log('Sending:', message)
        wsRef.current.send(payload)
        return true
      }
      return false
    },
    [log]
  )

  // Queue message for later delivery
  const queueMessage = useCallback((message: ClientMessage) => {
    messageQueue.current.push({
      message,
      timestamp: Date.now(),
    })
    setQueuedMessageCount(messageQueue.current.length)
  }, [])

  // Flush queued messages
  const flushQueue = useCallback(() => {
    const queue = messageQueue.current
    messageQueue.current = []
    setQueuedMessageCount(0)

    const maxAge = 5 * 60 * 1000 // 5 minutes
    const now = Date.now()

    for (const { message, timestamp } of queue) {
      if (now - timestamp < maxAge) {
        if (!sendRaw(message)) {
          // Re-queue if send fails
          messageQueue.current.push({ message, timestamp })
        }
      } else {
        log('Dropped stale message:', message)
      }
    }

    setQueuedMessageCount(messageQueue.current.length)
  }, [sendRaw, log])

  // Start heartbeat
  const startHeartbeat = useCallback(() => {
    const { heartbeatInterval: interval, heartbeatTimeout: timeout } = configRef.current

    heartbeatInterval.current = setInterval(() => {
      if (wsRef.current?.readyState === WebSocket.OPEN) {
        sendRaw({ type: 'ping', timestamp: new Date().toISOString() })

        heartbeatTimeout.current = setTimeout(() => {
          log('Heartbeat timeout - connection stale')
          wsRef.current?.close(4000, 'Heartbeat timeout')
        }, timeout)
      }
    }, interval)
  }, [sendRaw, log])

  // Handle incoming WebSocket message
  const handleMessage = useCallback(
    (event: MessageEvent) => {
      try {
        const message = JSON.parse(event.data) as WebSocketMessage

        log('Received:', message)
        handlersRef.current.onRawMessage?.(message)

        // Handle pong - clear heartbeat timeout
        if (message.type === 'pong') {
          if (heartbeatTimeout.current) {
            clearTimeout(heartbeatTimeout.current)
            heartbeatTimeout.current = null
          }
          return
        }

        // Route message to appropriate handler
        switch (message.type) {
          case 'message':
            handlersRef.current.onMessage?.(message.payload as Message)
            break

          case 'streaming_chunk':
            handlersRef.current.onStreamingChunk?.(message.payload as StreamingChunk)
            break

          case 'tool_call_update':
            handlersRef.current.onToolCallUpdate?.(message.payload as ToolCall)
            break

          case 'event':
            handlersRef.current.onEvent?.(message.payload as LoopEvent)
            // Check if event contains cost or loop state updates
            const loopEvent = message.payload as LoopEvent
            if (loopEvent.cost) {
              handlersRef.current.onCostUpdate?.(loopEvent.cost)
            }
            break

          case 'step':
            // Treat server 'step' messages as loop events and forward
            handlersRef.current.onEvent?.(message.payload as LoopEvent)
            break

          case 'error':
            handlersRef.current.onError?.(message.payload as string)
            break

          case 'connect':
            // Connection acknowledged by server
            log('Connection acknowledged')
            break

          case 'subscribe':
            // Subscription acknowledged
            log('Subscription acknowledged for session:', message.sessionId)
            break

          case 'unsubscribe':
            // Unsubscription acknowledged
            log('Unsubscription acknowledged')
            break

          default:
            log('Unknown message type:', message.type)
        }
      } catch (err) {
        log('Failed to parse message:', err, event.data)
      }
    },
    [log]
  )

  // Calculate reconnection delay with exponential backoff
  const getReconnectDelay = useCallback(() => {
    const { reconnectDelay, maxReconnectDelay, reconnectBackoff } = configRef.current
    const delay = Math.min(
      reconnectDelay * Math.pow(reconnectBackoff, reconnectAttempts.current),
      maxReconnectDelay
    )
    return delay
  }, [])

  // Schedule reconnection
  const scheduleReconnect = useCallback(() => {
    const { autoReconnect, maxReconnectAttempts } = configRef.current

    if (!autoReconnect || isIntentionalDisconnect.current) {
      return
    }

    if (reconnectAttempts.current >= maxReconnectAttempts) {
      log('Max reconnection attempts reached')
      updateConnectionStatus('error', 'Max reconnection attempts reached')
      return
    }

    const delay = getReconnectDelay()
    log(`Scheduling reconnection attempt ${reconnectAttempts.current + 1} in ${delay}ms`)

    updateConnectionStatus('reconnecting')

    reconnectTimeout.current = setTimeout(() => {
      reconnectAttempts.current++
      connect()
    }, delay)
  }, [getReconnectDelay, log, updateConnectionStatus])

  // Connect to WebSocket server
  const connect = useCallback(() => {
    const { url } = configRef.current

    // Close existing connection
    if (wsRef.current) {
      wsRef.current.close()
      wsRef.current = null
    }

    clearTimers()
    isIntentionalDisconnect.current = false
    updateConnectionStatus('connecting')

    try {
      log('Connecting to:', url)
      const ws = new WebSocket(url)

      ws.onopen = () => {
        log('Connected')
        reconnectAttempts.current = 0
        updateConnectionStatus('connected')
        startHeartbeat()
        flushQueue()

        // Re-subscribe to session if we had one
        if (subscribedSession) {
          sendRaw({
            type: 'subscribe',
            sessionId: subscribedSession,
          } as unknown as ClientMessage)
        }
      }

      ws.onclose = (event) => {
        log('Disconnected:', event.code, event.reason)
        clearTimers()
        wsRef.current = null

        if (isIntentionalDisconnect.current) {
          updateConnectionStatus('disconnected')
        } else {
          scheduleReconnect()
        }
      }

      ws.onerror = (error) => {
        log('Error:', error)
        // Error will be followed by close event
      }

      ws.onmessage = handleMessage

      wsRef.current = ws
    } catch (err) {
      log('Connection error:', err)
      updateConnectionStatus('error', err instanceof Error ? err.message : 'Connection failed')
      scheduleReconnect()
    }
  }, [
    clearTimers,
    flushQueue,
    handleMessage,
    log,
    scheduleReconnect,
    sendRaw,
    startHeartbeat,
    subscribedSession,
    updateConnectionStatus,
  ])

  // Disconnect from WebSocket server
  const disconnect = useCallback(() => {
    log('Disconnecting')
    isIntentionalDisconnect.current = true
    clearTimers()

    if (wsRef.current) {
      wsRef.current.close(1000, 'Client disconnect')
      wsRef.current = null
    }

    setSubscribedSession(undefined)
    updateConnectionStatus('disconnected')
  }, [clearTimers, log, updateConnectionStatus])

  // Ensure all timers and sockets are cleaned up on unmount
  useEffect(() => {
    return () => {
      // Prevent reconnection attempts during teardown
      isIntentionalDisconnect.current = true
      clearTimers()
      if (wsRef.current) {
        try {
          wsRef.current.close(1000, 'Unmount')
        } catch {
          // ignore close errors during teardown
        }
        wsRef.current = null
      }
    }
  }, [clearTimers])

  // Subscribe to session
  const subscribe = useCallback(
    (sessionId: string) => {
      log('Subscribing to session:', sessionId)
      setSubscribedSession(sessionId)

      const message: ClientMessage = {
        type: 'send_message', // Using send_message as placeholder for subscribe
        sessionId,
      }

      // Send subscribe message (using different envelope for subscription)
      if (wsRef.current?.readyState === WebSocket.OPEN) {
        sendRaw({
          type: 'subscribe',
          sessionId,
        } as unknown as ClientMessage)
      }
    },
    [log, sendRaw]
  )

  // Unsubscribe from session
  const unsubscribe = useCallback(() => {
    const sessionId = subscribedSession
    if (!sessionId) return

    log('Unsubscribing from session:', sessionId)

    if (wsRef.current?.readyState === WebSocket.OPEN) {
      sendRaw({
        type: 'unsubscribe',
        sessionId,
      } as unknown as ClientMessage)
    }

    setSubscribedSession(undefined)
  }, [log, sendRaw, subscribedSession])

  // Send chat message
  const sendMessage = useCallback(
    (sessionId: string, content: string) => {
      const message: ClientMessage = {
        type: 'send_message',
        sessionId,
        content,
      }

      if (!sendRaw(message)) {
        queueMessage(message)
      }
    },
    [queueMessage, sendRaw]
  )

  // Approve tool call
  const approveTool = useCallback(
    (sessionId: string, toolCallId: string) => {
      const message: ClientMessage = {
        type: 'approve_tool',
        sessionId,
        toolCallId,
      }

      if (!sendRaw(message)) {
        queueMessage(message)
      }
    },
    [queueMessage, sendRaw]
  )

  // Reject tool call
  const rejectTool = useCallback(
    (sessionId: string, toolCallId: string, reason?: string) => {
      const message: ClientMessage = {
        type: 'reject_tool',
        sessionId,
        toolCallId,
        reason,
      }

      if (!sendRaw(message)) {
        queueMessage(message)
      }
    },
    [queueMessage, sendRaw]
  )

  // Pause loop
  const pauseLoop = useCallback(
    (sessionId: string) => {
      const message: ClientMessage = {
        type: 'pause',
        sessionId,
      }

      if (!sendRaw(message)) {
        queueMessage(message)
      }
    },
    [queueMessage, sendRaw]
  )

  // Resume loop
  const resumeLoop = useCallback(
    (sessionId: string) => {
      const message: ClientMessage = {
        type: 'resume',
        sessionId,
      }

      if (!sendRaw(message)) {
        queueMessage(message)
      }
    },
    [queueMessage, sendRaw]
  )

  // Stop loop
  const stopLoop = useCallback(
    (sessionId: string) => {
      const message: ClientMessage = {
        type: 'stop',
        sessionId,
      }

      if (!sendRaw(message)) {
        queueMessage(message)
      }
    },
    [queueMessage, sendRaw]
  )

  // Cleanup on unmount
  useEffect(() => {
    return () => {
      isIntentionalDisconnect.current = true
      clearTimers()
      wsRef.current?.close(1000, 'Component unmount')
    }
  }, [clearTimers])

  return {
    connectionStatus,
    connectionError,
    isConnected: connectionStatus === 'connected',
    subscribedSession,
    queuedMessageCount,
    connect,
    disconnect,
    subscribe,
    unsubscribe,
    sendMessage,
    approveTool,
    rejectTool,
    pauseLoop,
    resumeLoop,
    stopLoop,
  }
}

export default useWebSocket
