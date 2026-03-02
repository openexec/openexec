/**
 * Type verification tests
 *
 * These tests verify that the hooks module exports the expected types
 * and that the types are compatible with the chat types.
 *
 * @module hooks/__tests__/types.test
 */

import type {
  WebSocketConfig,
  WebSocketHandlers,
  UseWebSocketReturn,
  SessionApiConfig,
  UseSessionReturn,
  MessagesApiConfig,
  UseMessagesReturn,
  FetchMessagesOptions,
  UseToolCallsReturn,
  ChatConfig,
  UseChatReturn,
} from '../index'

import type {
  Session,
  SessionListItem,
  SessionFilters,
  CreateSessionParams,
  Message,
  StreamingChunk,
  ToolCall,
  ToolCallApproval,
  LoopEvent,
  CostInfo,
  AgentLoopState,
  EventFilters,
  WebSocketMessage,
  WebSocketMessageType,
  ClientMessage,
} from '../../types'

import type { ConnectionStatus } from '../../types/store'

// Type-only tests to verify type compatibility
describe('Hook Types', () => {
  describe('useWebSocket types', () => {
    it('should have correct WebSocketConfig type', () => {
      const config: WebSocketConfig = {
        url: 'ws://localhost:8080/ws',
        autoReconnect: true,
        reconnectDelay: 1000,
        maxReconnectDelay: 30000,
        reconnectBackoff: 2,
        maxReconnectAttempts: 10,
        heartbeatInterval: 30000,
        heartbeatTimeout: 10000,
        debug: false,
      }
      expect(config).toBeDefined()
    })

    it('should have correct WebSocketHandlers type', () => {
      const handlers: WebSocketHandlers = {
        onConnectionChange: (status: ConnectionStatus, error?: string) => {},
        onMessage: (message: Message) => {},
        onStreamingChunk: (chunk: StreamingChunk) => {},
        onToolCallUpdate: (toolCall: ToolCall) => {},
        onEvent: (event: LoopEvent) => {},
        onCostUpdate: (cost: CostInfo) => {},
        onLoopStateChange: (state: Partial<AgentLoopState>) => {},
        onError: (error: string) => {},
        onRawMessage: (message: WebSocketMessage) => {},
      }
      expect(handlers).toBeDefined()
    })

    it('should have correct UseWebSocketReturn type', () => {
      // This is a type-level assertion
      type AssertReturn = UseWebSocketReturn extends {
        connectionStatus: ConnectionStatus
        connectionError: string | undefined
        isConnected: boolean
        subscribedSession: string | undefined
        queuedMessageCount: number
        connect: () => void
        disconnect: () => void
        subscribe: (sessionId: string) => void
        unsubscribe: () => void
        sendMessage: (sessionId: string, content: string) => void
        approveTool: (sessionId: string, toolCallId: string) => void
        rejectTool: (sessionId: string, toolCallId: string, reason?: string) => void
        pauseLoop: (sessionId: string) => void
        resumeLoop: (sessionId: string) => void
        stopLoop: (sessionId: string) => void
      }
        ? true
        : false

      const check: AssertReturn = true
      expect(check).toBe(true)
    })
  })

  describe('useSession types', () => {
    it('should have correct SessionApiConfig type', () => {
      const config: SessionApiConfig = {
        baseUrl: 'http://localhost:8080',
        authToken: 'test-token',
      }
      expect(config).toBeDefined()
    })

    it('should have correct UseSessionReturn type', () => {
      type AssertReturn = UseSessionReturn extends {
        sessions: SessionListItem[]
        sessionsLoading: boolean
        sessionsError: string | undefined
        currentSession: Session | undefined
        currentSessionLoading: boolean
        currentSessionError: string | undefined
        filters: SessionFilters
        fetchSessions: (filters?: SessionFilters) => Promise<void>
        createSession: (params: CreateSessionParams) => Promise<Session>
        loadSession: (sessionId: string) => Promise<Session>
        updateSessionTitle: (sessionId: string, title: string) => Promise<void>
        archiveSession: (sessionId: string) => Promise<void>
        deleteSession: (sessionId: string) => Promise<void>
        forkSession: (sessionId: string, forkPointMessageId?: string) => Promise<Session>
        setFilters: (filters: SessionFilters) => void
        clearCurrentSession: () => void
        refreshCurrentSession: () => Promise<void>
      }
        ? true
        : false

      const check: AssertReturn = true
      expect(check).toBe(true)
    })
  })

  describe('useMessages types', () => {
    it('should have correct MessagesApiConfig type', () => {
      const config: MessagesApiConfig = {
        baseUrl: 'http://localhost:8080',
        authToken: 'test-token',
      }
      expect(config).toBeDefined()
    })

    it('should have correct FetchMessagesOptions type', () => {
      const options: FetchMessagesOptions = {
        limit: 50,
        offset: 0,
        append: true,
      }
      expect(options).toBeDefined()
    })

    it('should have correct UseMessagesReturn type', () => {
      type AssertReturn = UseMessagesReturn extends {
        messages: Message[]
        messagesLoading: boolean
        messagesError: string | undefined
        streamingMessage: Partial<Message> | undefined
        fetchMessages: (sessionId: string, options?: FetchMessagesOptions) => Promise<void>
        loadMoreMessages: (sessionId: string) => Promise<void>
        addMessage: (message: Message) => void
        handleStreamingChunk: (chunk: StreamingChunk) => void
        completeStreamingMessage: (message: Message) => void
        clearMessages: () => void
        getSortedMessages: () => Message[]
      }
        ? true
        : false

      const check: AssertReturn = true
      expect(check).toBe(true)
    })
  })

  describe('useToolCalls types', () => {
    it('should have correct UseToolCallsReturn type', () => {
      type AssertReturn = UseToolCallsReturn extends {
        toolCalls: ToolCall[]
        pendingToolCalls: ToolCall[]
        runningToolCalls: ToolCall[]
        addToolCall: (toolCall: ToolCall) => void
        updateToolCall: (toolCall: ToolCall) => void
        removeToolCall: (toolCallId: string) => void
        handleToolCallUpdate: (toolCall: ToolCall) => void
        clearToolCalls: () => void
        clearCompletedToolCalls: () => void
        createApproval: (
          toolCallId: string,
          approved: boolean,
          approvedBy: string,
          reason?: string
        ) => ToolCallApproval
      }
        ? true
        : false

      const check: AssertReturn = true
      expect(check).toBe(true)
    })
  })

  describe('useChat types', () => {
    it('should have correct ChatConfig type', () => {
      const config: ChatConfig = {
        wsUrl: 'ws://localhost:8080/ws',
        apiUrl: 'http://localhost:8080',
        authToken: 'test-token',
        wsOptions: {
          autoReconnect: true,
        },
        debug: false,
      }
      expect(config).toBeDefined()
    })

    it('should have correct UseChatReturn type', () => {
      type AssertReturn = UseChatReturn extends {
        connectionStatus: ConnectionStatus
        connectionError: string | undefined
        isConnected: boolean
        sessions: SessionListItem[]
        sessionsLoading: boolean
        currentSession: Session | undefined
        currentSessionLoading: boolean
        messages: Message[]
        messagesLoading: boolean
        streamingMessage: Partial<Message> | undefined
        toolCalls: ToolCall[]
        pendingToolCalls: ToolCall[]
        runningToolCalls: ToolCall[]
        loopState: AgentLoopState
        events: LoopEvent[]
        eventFilters: EventFilters
        sessionCost: CostInfo
        inputContent: string
        isSubmitting: boolean
        connect: () => void
        disconnect: () => void
        fetchSessions: (filters?: SessionFilters) => Promise<void>
        createSession: (params: CreateSessionParams) => Promise<Session>
        loadSession: (sessionId: string) => Promise<void>
        sendMessage: (content: string) => Promise<void>
        approveTool: (approval: ToolCallApproval) => Promise<void>
        rejectTool: (toolCallId: string, reason?: string) => Promise<void>
        pauseLoop: () => void
        resumeLoop: () => void
        stopLoop: () => void
        setEventFilters: (filters: EventFilters) => void
        clearEvents: () => void
        setInputContent: (content: string) => void
        reset: () => void
      }
        ? true
        : false

      const check: AssertReturn = true
      expect(check).toBe(true)
    })
  })
})
