/**
 * Zustand Store Types for Chat UI
 *
 * Defines the state and action types for the chat store.
 *
 * @module types/store
 */

import type {
  Session,
  SessionListItem,
  SessionFilters,
  Message,
  ToolCall,
  ToolCallApproval,
  LoopEvent,
  EventFilters,
  AgentLoopState,
  LoopConfig,
  ProviderInfo,
  ModelInfo,
  CostInfo,
  CreateSessionParams,
  StreamingChunk,
  WebSocketMessage,
} from './chat'

// =============================================================================
// Chat Store State
// =============================================================================

/**
 * Connection status for WebSocket
 */
export type ConnectionStatus = 'disconnected' | 'connecting' | 'connected' | 'reconnecting' | 'error'

/**
 * Main chat store state
 */
export interface ChatState {
  // Connection state
  connectionStatus: ConnectionStatus
  connectionError?: string

  // Session management
  sessions: SessionListItem[]
  sessionsLoading: boolean
  sessionsError?: string
  sessionFilters: SessionFilters
  currentSession?: Session
  currentSessionLoading: boolean
  currentSessionError?: string

  // Messages
  messages: Message[]
  messagesLoading: boolean
  messagesError?: string
  streamingMessage?: Partial<Message>
  pendingToolCalls: ToolCall[]

  // Loop state
  loopState: AgentLoopState
  loopConfig: LoopConfig

  // Events
  events: LoopEvent[]
  eventFilters: EventFilters
  maxEventsInMemory: number

  // Cost tracking
  sessionCost: CostInfo

  // Provider/Model info
  providers: ProviderInfo[]
  providersLoading: boolean
  selectedProvider?: string
  selectedModel?: string

  // Input state
  inputContent: string
  isSubmitting: boolean
  inputError?: string
}

/**
 * Chat store actions
 */
export interface ChatActions {
  // Connection actions
  connect: (wsUrl: string) => void
  disconnect: () => void
  setConnectionStatus: (status: ConnectionStatus, error?: string) => void

  // Session actions
  fetchSessions: (filters?: SessionFilters) => Promise<void>
  createSession: (params: CreateSessionParams) => Promise<Session>
  loadSession: (sessionId: string) => Promise<void>
  updateSessionTitle: (sessionId: string, title: string) => Promise<void>
  archiveSession: (sessionId: string) => Promise<void>
  forkSession: (sessionId: string, forkPointMessageId?: string) => Promise<Session>
  setSessionFilters: (filters: SessionFilters) => void
  clearCurrentSession: () => void

  // Message actions
  fetchMessages: (sessionId: string, pagination?: { offset: number; limit: number }) => Promise<void>
  sendMessage: (content: string) => Promise<void>
  appendStreamingChunk: (chunk: StreamingChunk) => void
  completeStreamingMessage: (message: Message) => void
  clearMessages: () => void

  // Tool call actions
  approveTool: (approval: ToolCallApproval) => Promise<void>
  rejectTool: (toolCallId: string, reason?: string) => Promise<void>
  updateToolCall: (toolCall: ToolCall) => void
  clearPendingToolCalls: () => void

  // Loop control actions
  pauseLoop: () => Promise<void>
  resumeLoop: () => Promise<void>
  stopLoop: () => Promise<void>
  updateLoopState: (state: Partial<AgentLoopState>) => void
  setLoopConfig: (config: Partial<LoopConfig>) => void

  // Event actions
  addEvent: (event: LoopEvent) => void
  setEventFilters: (filters: EventFilters) => void
  clearEvents: () => void

  // Cost actions
  updateCost: (cost: CostInfo) => void

  // Provider actions
  fetchProviders: () => Promise<void>
  selectProvider: (providerId: string) => void
  selectModel: (modelId: string) => void

  // Input actions
  setInputContent: (content: string) => void
  setInputError: (error?: string) => void

  // WebSocket message handler
  handleWebSocketMessage: (message: WebSocketMessage) => void

  // Reset
  reset: () => void
}

/**
 * Complete chat store type
 */
export type ChatStore = ChatState & ChatActions

// =============================================================================
// Initial State Factory
// =============================================================================

/**
 * Default initial state for the chat store
 */
export const initialChatState: ChatState = {
  // Connection
  connectionStatus: 'disconnected',

  // Sessions
  sessions: [],
  sessionsLoading: false,
  sessionFilters: {},

  // Current session
  currentSessionLoading: false,

  // Messages
  messages: [],
  messagesLoading: false,
  pendingToolCalls: [],

  // Loop state
  loopState: {
    iteration: 0,
    totalTokens: 0,
    totalCostUsd: 0,
    isRunning: false,
    isPaused: false,
    iterationsSinceProgress: 0,
  },
  loopConfig: {
    autoContext: true,
  },

  // Events
  events: [],
  eventFilters: {},
  maxEventsInMemory: 1000,

  // Cost
  sessionCost: {
    sessionTotal: 0,
    iterationCost: 0,
    totalTokensInput: 0,
    totalTokensOutput: 0,
  },

  // Providers
  providers: [],
  providersLoading: false,

  // Input
  inputContent: '',
  isSubmitting: false,
}

// =============================================================================
// Selector Types
// =============================================================================

/**
 * Selectors for computed state
 */
export interface ChatSelectors {
  /** Get active sessions only */
  getActiveSessions: () => SessionListItem[]
  /** Get current session messages sorted by timestamp */
  getSortedMessages: () => Message[]
  /** Get pending tool calls that need approval */
  getPendingApprovals: () => ToolCall[]
  /** Get filtered events */
  getFilteredEvents: () => LoopEvent[]
  /** Get available models for selected provider */
  getAvailableModels: () => ModelInfo[]
  /** Check if loop can be started */
  canStartLoop: () => boolean
  /** Check if input can be submitted */
  canSubmit: () => boolean
  /** Get total session cost formatted */
  getFormattedCost: () => string
}
