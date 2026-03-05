/**
 * Main Chat Hook
 *
 * Aggregates all chat functionality into a single hook:
 * - WebSocket connection management
 * - Session management
 * - Message operations
 * - Tool call handling
 * - Loop control
 * - Event tracking
 * - Cost monitoring
 * - Project management
 *
 * @module hooks/useChat
 */

import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { useWebSocket, type WebSocketConfig, type WebSocketHandlers } from './useWebSocket'
import { useSession, type SessionApiConfig } from './useSession'
import { useMessages, type MessagesApiConfig } from './useMessages'
import { useToolCalls } from './useToolCalls'
import type {
  Session,
  Message,
  ToolCall,
  LoopEvent,
  CostInfo,
  AgentLoopState,
  EventFilters,
  CreateSessionParams,
  ToolCallApproval,
  ProjectInfo,
} from '../types'
import type { ConnectionStatus } from '../types/store'

// =============================================================================
// Configuration
// =============================================================================

export interface ChatConfig {
  /** WebSocket server URL */
  wsUrl: string
  /** REST API base URL */
  apiUrl: string
  /** Optional auth token */
  authToken?: string
  /** WebSocket config options */
  wsOptions?: Partial<Omit<WebSocketConfig, 'url'>>
  /** Enable debug mode */
  debug?: boolean
}

// =============================================================================
// Hook Return Type
// =============================================================================

export interface UseChatReturn {
  // Connection state
  connectionStatus: ConnectionStatus
  connectionError: string | undefined
  isConnected: boolean

  // Session state
  sessions: ReturnType<typeof useSession>['sessions']
  sessionsLoading: boolean
  currentSession: Session | undefined
  currentSessionLoading: boolean

  // Project state
  projects: ProjectInfo[]
  projectsLoading: boolean
  projectsError: string | undefined

  // Message state
  messages: Message[]
  messagesLoading: boolean
  streamingMessage: Partial<Message> | undefined

  // Tool call state
  toolCalls: ToolCall[]
  pendingToolCalls: ToolCall[]
  runningToolCalls: ToolCall[]

  // Loop state
  loopState: AgentLoopState
  loopConfig: LoopConfig

  // Events
  events: LoopEvent[]
  eventFilters: EventFilters

  // Cost
  sessionCost: CostInfo

  // Input state
  inputContent: string
  isSubmitting: boolean

  // Connection actions
  connect: () => void
  disconnect: () => void

  // Session actions
  fetchSessions: ReturnType<typeof useSession>['fetchSessions']
  createSession: (params: CreateSessionParams) => Promise<Session>
  initProject: (name: string, path: string) => Promise<void>
  loadSession: (sessionId: string) => Promise<void>
  updateSessionTitle: (sessionId: string, title: string) => Promise<void>
  archiveSession: (sessionId: string) => Promise<void>
  forkSession: (sessionId: string, forkPointMessageId?: string) => Promise<Session>

  // Project actions
  fetchProjects: () => Promise<void>

  // Message actions
  sendMessage: (content: string) => Promise<void>
  fetchMessages: (sessionId: string) => Promise<void>

  // Tool call actions
  approveTool: (approval: ToolCallApproval) => Promise<void>
  rejectTool: (toolCallId: string, reason?: string) => Promise<void>

  // Loop control actions
  pauseLoop: () => void
  resumeLoop: () => void
  stopLoop: () => void

  // Event actions
  setEventFilters: (filters: EventFilters) => void
  clearEvents: () => void

  // Input actions
  setInputContent: (content: string) => void

  // Utility
  reset: () => void
}

// Loop configuration
interface LoopConfig {
  maxIterations?: number
  maxTokens?: number
  budgetUsd?: number
  autoContext?: boolean
}

// =============================================================================
// useChat Hook
// =============================================================================

export function useChat(config: ChatConfig): UseChatReturn {
  const { wsUrl, apiUrl, authToken, wsOptions, debug } = config

  // Refs
  const debugRef = useRef(debug)
  useEffect(() => {
    debugRef.current = debug
  }, [debug])

  const log = useCallback((...args: unknown[]) => {
    if (debugRef.current) {
      console.log('[useChat]', ...args)
    }
  }, [])

  // ==========================================================================
  // State
  // ==========================================================================

  const [loopState, setLoopState] = useState<AgentLoopState>({
    iteration: 0,
    totalTokens: 0,
    totalCostUsd: 0,
    isRunning: false,
    isPaused: false,
    iterationsSinceProgress: 0,
  })

  const [loopConfig, setLoopConfig] = useState<LoopConfig>({
    autoContext: true,
  })

  const [events, setEvents] = useState<LoopEvent[]>([])
  const [eventFilters, setEventFilters] = useState<EventFilters>({})
  const maxEventsInMemory = 1000

  const [sessionCost, setSessionCost] = useState<CostInfo>({
    sessionTotal: 0,
    iterationCost: 0,
    totalTokensInput: 0,
    totalTokensOutput: 0,
  })

  const [inputContent, setInputContent] = useState('')
  const [isSubmitting, setIsSubmitting] = useState(false)

  // ==========================================================================
  // Composed Hooks
  // ==========================================================================

  const sessionConfig: SessionApiConfig = useMemo(
    () => ({ baseUrl: apiUrl, authToken }),
    [apiUrl, authToken]
  )

  const messagesConfig: MessagesApiConfig = useMemo(
    () => ({ baseUrl: apiUrl, authToken }),
    [apiUrl, authToken]
  )

  const session = useSession(sessionConfig)
  const messagesHook = useMessages(messagesConfig)
  const toolCallsHook = useToolCalls()

  // ==========================================================================
  // WebSocket Handlers
  // ==========================================================================

  const wsHandlers: WebSocketHandlers = useMemo(
    () => ({
      onConnectionChange: (status, error) => {
        log('Connection status:', status, error)
      },

      onMessage: (message) => {
        log('Message received:', message.id)
        messagesHook.addMessage(message)

        // Extract tool calls from message
        if (message.toolCalls) {
          for (const tc of message.toolCalls) {
            toolCallsHook.addToolCall(tc)
          }
        }
      },

      onStreamingChunk: (chunk) => {
        log('Streaming chunk:', chunk.messageId, chunk.isComplete)
        messagesHook.handleStreamingChunk(chunk)

        // Handle tool call in chunk
        if (chunk.toolCall) {
          toolCallsHook.handleToolCallUpdate(chunk.toolCall)
        }
      },

      onToolCallUpdate: (toolCall) => {
        log('Tool call update:', toolCall.id, toolCall.status)
        toolCallsHook.handleToolCallUpdate(toolCall)
      },

      onEvent: (event) => {
        log('Event:', event.type)
        addEvent(event)

        // Update loop state from event
        if (event.type.startsWith('loop.')) {
          handleLoopEvent(event)
        }

        // Update cost from event
        if (event.cost) {
          setSessionCost(event.cost)
        }
      },

      onCostUpdate: (cost) => {
        log('Cost update:', cost)
        setSessionCost(cost)
      },

      onError: (error) => {
        log('Error:', error)
      },
    }),
    [log, messagesHook, toolCallsHook]
  )

  const wsConfig: WebSocketConfig = useMemo(
    () => ({
      url: wsUrl,
      debug,
      ...wsOptions,
    }),
    [wsUrl, debug, wsOptions]
  )

  const ws = useWebSocket(wsConfig, wsHandlers)

  // ==========================================================================
  // Event Management
  // ==========================================================================

  const addEvent = useCallback(
    (event: LoopEvent) => {
      setEvents((prev) => {
        const updated = [...prev, event]
        // Trim to max events
        if (updated.length > maxEventsInMemory) {
          return updated.slice(-maxEventsInMemory)
        }
        return updated
      })
    },
    [maxEventsInMemory]
  )

  const handleLoopEvent = useCallback((event: LoopEvent) => {
    switch (event.type) {
      case 'loop.start':
        setLoopState((prev) => ({
          ...prev,
          isRunning: true,
          isPaused: false,
          startedAt: event.timestamp,
          currentPid: event.currentPid,
        }))
        break

      case 'loop.pause':
        setLoopState((prev) => ({
          ...prev,
          isPaused: true,
        }))
        break

      case 'loop.resume':
        setLoopState((prev) => ({
          ...prev,
          isPaused: false,
        }))
        break

      case 'loop.stop':
      case 'loop.complete':
      case 'loop.error':
      case 'loop.timeout':
      case 'loop.max_reached':
        setLoopState((prev) => ({
          ...prev,
          isRunning: false,
          isPaused: false,
        }))
        break

      case 'iteration.start':
        setLoopState((prev) => ({
          ...prev,
          iteration: event.iteration ?? prev.iteration + 1,
          lastIterationAt: event.timestamp, lastActivity: event.timestamp,
        }))
        break
    }
  }, [])

  const clearEvents = useCallback(() => {
    setEvents([])
  }, [])

  // ==========================================================================
  // Session Management
  // ==========================================================================

  const loadSession = useCallback(
    async (sessionId: string) => {
      log('Loading session:', sessionId)

      // Load session details
      await session.loadSession(sessionId)

      // Subscribe to WebSocket events
      ws.subscribe(sessionId)

      // Fetch messages
      await messagesHook.fetchMessages(sessionId)

      // Clear previous state
      toolCallsHook.clearToolCalls()
      setEvents([])
      setLoopState({
        iteration: 0,
        totalTokens: 0,
        totalCostUsd: 0,
        isRunning: false,
        isPaused: false,
        iterationsSinceProgress: 0,
      })
      setSessionCost({
        sessionTotal: 0,
        iterationCost: 0,
        totalTokensInput: 0,
        totalTokensOutput: 0,
      })
    },
    [log, session, ws, messagesHook, toolCallsHook]
  )

  const createSession = useCallback(
    async (params: CreateSessionParams): Promise<Session> => {
      log('Creating session:', params)
      const newSession = await session.createSession(params)

      // Auto-load the new session
      await loadSession(newSession.id)

      return newSession
    },
    [log, session, loadSession]
  )

  // ==========================================================================
  // Message Actions
  // ==========================================================================

  const sendMessage = useCallback(
    async (content: string) => {
      if (!session.currentSession) {
        throw new Error('No active session')
      }

      const sessionId = session.currentSession.id

      log('Sending message:', content.slice(0, 50))
      setIsSubmitting(true)

      try {
        // Optimistic update - add user message locally
        const optimisticMessage: Message = {
          id: `temp-${Date.now()}`,
          sessionId,
          role: 'user',
          content,
          tokensInput: 0,
          tokensOutput: 0,
          costUsd: 0,
          createdAt: new Date().toISOString(),
        }
        messagesHook.addMessage(optimisticMessage)

        // Clear input
        setInputContent('')

        // Send via WebSocket
        ws.sendMessage(sessionId, content)
      } finally {
        setIsSubmitting(false)
      }
    },
    [log, session.currentSession, messagesHook, ws]
  )

  // ==========================================================================
  // Tool Call Actions
  // ==========================================================================

  const approveTool = useCallback(
    async (approval: ToolCallApproval) => {
      if (!session.currentSession) {
        throw new Error('No active session')
      }

      log('Approving tool:', approval.toolCallId)
      ws.approveTool(session.currentSession.id, approval.toolCallId)

      // Optimistic update
      toolCallsHook.updateToolCall({
        id: approval.toolCallId,
        approvalStatus: 'approved',
        approvedBy: approval.approvedBy,
        approvedAt: new Date().toISOString(),
      } as ToolCall)
    },
    [log, session.currentSession, ws, toolCallsHook]
  )

  const rejectTool = useCallback(
    async (toolCallId: string, reason?: string) => {
      if (!session.currentSession) {
        throw new Error('No active session')
      }

      log('Rejecting tool:', toolCallId, reason)
      ws.rejectTool(session.currentSession.id, toolCallId, reason)

      // Optimistic update
      toolCallsHook.updateToolCall({
        id: toolCallId,
        approvalStatus: 'rejected',
        status: 'cancelled',
      } as ToolCall)
    },
    [log, session.currentSession, ws, toolCallsHook]
  )

  // ==========================================================================
  // Loop Control Actions
  // ==========================================================================

  const pauseLoop = useCallback(() => {
    if (!session.currentSession) return
    log('Pausing loop')
    ws.pauseLoop(session.currentSession.id)
  }, [log, session.currentSession, ws])

  const resumeLoop = useCallback(() => {
    if (!session.currentSession) return
    log('Resuming loop')
    ws.resumeLoop(session.currentSession.id)
  }, [log, session.currentSession, ws])

  const stopLoop = useCallback(() => {
    if (!session.currentSession) return
    log('Stopping loop')
    ws.stopLoop(session.currentSession.id)
  }, [log, session.currentSession, ws])

  // ==========================================================================
  // Reset
  // ==========================================================================

  const reset = useCallback(() => {
    log('Resetting chat state')
    ws.disconnect()
    session.clearCurrentSession()
    messagesHook.clearMessages()
    toolCallsHook.clearToolCalls()
    setEvents([])
    setLoopState({
      iteration: 0,
      totalTokens: 0,
      totalCostUsd: 0,
      isRunning: false,
      isPaused: false,
      iterationsSinceProgress: 0,
    })
    setSessionCost({
      sessionTotal: 0,
      iterationCost: 0,
      totalTokensInput: 0,
      totalTokensOutput: 0,
    })
    setInputContent('')
    setIsSubmitting(false)
  }, [log, ws, session, messagesHook, toolCallsHook])

  // ==========================================================================
  // Auto-connect on mount
  // ==========================================================================

  useEffect(() => {
    log('Initializing chat, connecting to WebSocket')
    ws.connect()

    return () => {
      log('Cleaning up chat')
      ws.disconnect()
    }
    // Only run on mount/unmount
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  // ==========================================================================
  // Return
  // ==========================================================================

  return {
    // Connection state
    connectionStatus: ws.connectionStatus,
    connectionError: ws.connectionError,
    isConnected: ws.isConnected,

    // Session state
    sessions: session.sessions,
    sessionsLoading: session.sessionsLoading,
    currentSession: session.currentSession,
    currentSessionLoading: session.currentSessionLoading,

    // Project state
    projects: session.projects,
    projectsLoading: session.projectsLoading,
    projectsError: session.projectsError,

    // Message state
    messages: messagesHook.messages,
    messagesLoading: messagesHook.messagesLoading,
    streamingMessage: messagesHook.streamingMessage,

    // Tool call state
    toolCalls: toolCallsHook.toolCalls,
    pendingToolCalls: toolCallsHook.pendingToolCalls,
    runningToolCalls: toolCallsHook.runningToolCalls,

    // Loop state
    loopState,
    loopConfig,

    // Events
    events,
    eventFilters,

    // Cost
    sessionCost,

    // Input state
    inputContent,
    isSubmitting,

    // Connection actions
    connect: ws.connect,
    disconnect: ws.disconnect,

    // Session actions
    fetchSessions: session.fetchSessions,
    createSession,
    initProject: session.initProject,
    loadSession,
    updateSessionTitle: session.updateSessionTitle,
    archiveSession: session.archiveSession,
    forkSession: session.forkSession,

    // Project actions
    fetchProjects: session.fetchProjects,

    // Message actions
    sendMessage,
    fetchMessages: messagesHook.fetchMessages,

    // Tool call actions
    approveTool,
    rejectTool,

    // Loop control actions
    pauseLoop,
    resumeLoop,
    stopLoop,

    // Event actions
    setEventFilters,
    clearEvents,

    // Input actions
    setInputContent,

    // Utility
    reset,
  }
}

export default useChat
