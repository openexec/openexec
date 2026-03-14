/**
 * Chat UI Types for OpenExec Orchestrator
 *
 * These types mirror the Go backend types from:
 * - internal/db/session/types.go (Session, Message, ToolCall)
 * - internal/loop/events.go (LoopEventType, EventKind, ToolCallInfo)
 *
 * @module types/chat
 */

// =============================================================================
// Session Types
// =============================================================================

/**
 * Session lifecycle status
 */
export type SessionStatus = 'active' | 'paused' | 'archived' | 'deleted'

/**
 * Message sender role
 */
export type MessageRole = 'user' | 'assistant' | 'system'

/**
 * Tool call execution status
 */
export type ToolCallStatus =
  | 'pending'
  | 'running'
  | 'completed'
  | 'failed'
  | 'cancelled'
  | 'timeout'

/**
 * Tool call approval status
 */
export type ApprovalStatus = 'pending' | 'approved' | 'rejected' | 'auto_approved'

/**
 * Chat session bound to a project workspace
 */
export interface Session {
  id: string
  projectPath: string
  provider: string
  model: string
  title: string
  parentSessionId?: string
  forkPointMessageId?: string
  status: SessionStatus
  createdAt: string
  updatedAt: string
}

/**
 * Metadata about an OpenExec project workspace
 */
export interface ProjectInfo {
  name: string
  path: string
  type: string
}

/**
 * Session creation parameters
 */
export interface CreateSessionParams {
  projectPath: string
  provider: string
  model: string
  title?: string
}

/**
 * Session list item with preview information
 */
export interface SessionListItem {
  id: string
  title: string
  provider: string
  model: string
  status: SessionStatus
  lastMessagePreview?: string
  messageCount: number
  totalCostUsd: number
  createdAt: string
  updatedAt: string
  /** Parent session ID if this session is a fork */
  parentSessionId?: string
  /** Fork depth in the session tree (0 = root session) */
  forkDepth?: number
}

// =============================================================================
// Message Types
// =============================================================================

/**
 * Chat message in a session conversation
 */
export interface Message {
  id: string
  sessionId: string
  role: MessageRole
  content: string
  tokensInput: number
  tokensOutput: number
  costUsd: number
  createdAt: string
  toolCalls?: ToolCall[]
}

/**
 * Message content block (for multi-part messages)
 */
export interface ContentBlock {
  type: 'text' | 'tool_use' | 'tool_result'
  text?: string
  toolUseId?: string
  toolName?: string
  toolInput?: Record<string, unknown>
  toolResult?: string
  isError?: boolean
}

/**
 * Streaming message chunk for real-time updates
 */
export interface StreamingChunk {
  messageId: string
  content: string
  isComplete: boolean
  toolCall?: ToolCall
}

// =============================================================================
// Tool Call Types
// =============================================================================

/**
 * MCP tool invocation within a conversation
 */
export interface ToolCall {
  id: string
  messageId: string
  sessionId: string
  toolName: string
  toolInput: string
  toolOutput?: string
  status: ToolCallStatus
  approvalStatus?: ApprovalStatus
  approvedBy?: string
  approvedAt?: string
  startedAt?: string
  completedAt?: string
  durationMs?: number
  error?: string
  progressPercent?: number
  progressMessage?: string
  riskLevel?: string
  createdAt: string
}

/**
 * Tool call approval action
 */
export interface ToolCallApproval {
  toolCallId: string
  approved: boolean
  approvedBy: string
  reason?: string
}

/**
 * Tool definition for display in UI
 */
export interface ToolDefinition {
  name: string
  description: string
  inputSchema: Record<string, unknown>
  riskLevel?: 'low' | 'medium' | 'high'
  requiresApproval: boolean
}

// =============================================================================
// Loop Event Types
// =============================================================================

/**
 * Loop event type categories (mirrors Go LoopEventType)
 */
export type LoopEventType =
  // Lifecycle events
  | 'loop.start'
  | 'loop.pause'
  | 'loop.resume'
  | 'loop.stop'
  | 'loop.complete'
  | 'loop.error'
  | 'loop.timeout'
  | 'loop.max_reached'
  // Iteration events
  | 'iteration.start'
  | 'iteration.complete'
  | 'iteration.retry'
  | 'iteration.skip'
  // LLM interaction events
  | 'llm.request_start'
  | 'llm.request_end'
  | 'llm.stream_start'
  | 'llm.stream_chunk'
  | 'llm.stream_end'
  | 'llm.error'
  | 'llm.rate_limit'
  | 'llm.context_window'
  // Tool execution events
  | 'tool.call_requested'
  | 'tool.call_queued'
  | 'tool.call_approved'
  | 'tool.call_rejected'
  | 'tool.call_start'
  | 'tool.call_progress'
  | 'tool.call_complete'
  | 'tool.call_error'
  | 'tool.call_timeout'
  | 'tool.call_cancelled'
  | 'tool.result_sent'
  | 'tool.auto_approved'
  // Context events
  | 'context.injected'
  | 'context.truncated'
  | 'context.summarized'
  | 'context.refreshed'
  // Message events
  | 'message.user'
  | 'message.assistant'
  | 'message.system'
  // Gate events
  | 'gate.check_start'
  | 'gate.check_pass'
  | 'gate.check_fail'
  | 'gate.fix_start'
  | 'gate.fix_success'
  | 'gate.fix_fail'
  // Signal events
  | 'signal.received'
  | 'signal.sent'
  | 'signal.phase_complete'
  // Cost tracking events
  | 'cost.updated'
  | 'cost.budget_warn'
  | 'cost.budget_exceeded'
  // Session events
  | 'session.created'
  | 'session.restored'
  | 'session.persisted'
  | 'session.forked'
  // Thrashing events
  | 'thrashing.detected'
  | 'thrashing.resolved'

/**
 * Event kind categories for filtering
 */
export type EventKind =
  | 'lifecycle'
  | 'iteration'
  | 'llm'
  | 'tool'
  | 'context'
  | 'message'
  | 'gate'
  | 'signal'
  | 'cost'
  | 'session'
  | 'thrashing'

/**
 * LLM request information
 */
export interface LLMRequestInfo {
  provider: string
  model: string
  inputTokens: number
  outputTokens: number
  totalTokens: number
  cacheReadTokens?: number
  cacheWriteTokens?: number
  costUsd: number
  durationMs: number
  stopReason?: string
  requestId?: string
}

/**
 * Cost tracking information
 */
export interface CostInfo {
  sessionTotal: number
  iterationCost: number
  budgetLimit?: number
  budgetRemaining?: number
  budgetPercent?: number
  totalTokensInput: number
  totalTokensOutput: number
}

/**
 * Context management information
 */
export interface ContextInfo {
  tokenCount: number
  maxTokens: number
  usagePercent: number
  wasTruncated?: boolean
  truncatedTokens?: number
  wasSummarized?: boolean
  sourceFiles?: string[]
}

/**
 * Quality gate check information
 */
export interface GateInfo {
  gateName: string
  passed: boolean
  message?: string
  fixAttempt?: number
  maxFixAttempts?: number
  details?: Record<string, unknown>
}

/**
 * Signal information for orchestration
 */
export interface SignalInfo {
  signalType: string
  target?: string
  reason?: string
  metadata?: Record<string, unknown>
}

/**
 * Loop event from the agent execution
 */
export interface LoopEvent {
  id: string
  type: LoopEventType
  kind: EventKind
  timestamp: string
  sessionId?: string
  iteration?: number
  message?: string
  error?: string
  toolCall?: ToolCall
  llmRequest?: LLMRequestInfo
  cost?: CostInfo
  context?: ContextInfo
  gate?: GateInfo
  signal?: SignalInfo
  metadata?: Record<string, unknown>
  currentPid?: number
  artifacts?: Record<string, string>
}

// =============================================================================
// Agent Loop State Types
// =============================================================================

/**
 * Current state of an active agent loop
 */
export interface AgentLoopState {
  iteration: number
  totalTokens: number
  totalCostUsd: number
  isRunning: boolean
  isPaused: boolean
  lastSignal?: string
  iterationsSinceProgress: number
  startedAt?: string
  lastIterationAt?: string
  lastActivity?: string
  currentPid?: number
}

/**
 * Loop configuration options
 */
export interface LoopConfig {
  maxIterations?: number
  maxTokens?: number
  budgetUsd?: number
  autoContext?: boolean
  responseTimeoutMs?: number
  thrashThreshold?: number
}

// =============================================================================
// WebSocket Message Types
// =============================================================================

/**
 * WebSocket message envelope
 */
export interface WebSocketMessage<T = unknown> {
  type: WebSocketMessageType
  sessionId?: string
  payload: T
  timestamp: string
}

/**
 * WebSocket message types
 */
export type WebSocketMessageType =
  | 'connect'
  | 'disconnect'
  | 'subscribe'
  | 'unsubscribe'
  | 'event'
  | 'step'
  | 'notice'
  | 'message'
  | 'streaming_chunk'
  | 'tool_call_update'
  | 'error'
  | 'ping'
  | 'pong'

/**
 * Client-to-server WebSocket messages
 */
export interface ClientMessage {
  type: 'send_message' | 'approve_tool' | 'reject_tool' | 'pause' | 'resume' | 'stop'
  sessionId: string
  content?: string
  toolCallId?: string
  reason?: string
}

// =============================================================================
// Provider & Model Types
// =============================================================================

/**
 * LLM provider information
 */
export interface ProviderInfo {
  id: string
  name: string
  models: ModelInfo[]
  isAvailable: boolean
  statusMessage?: string
}

/**
 * Model information with pricing
 */
export interface ModelInfo {
  id: string
  name: string
  provider: string
  contextWindow: number
  maxOutputTokens: number
  pricePerMInputTokens: number
  pricePerMOutputTokens: number
  supportsTools: boolean
  supportsStreaming: boolean
  supportsVision?: boolean
}

// =============================================================================
// UI State Types
// =============================================================================

/**
 * Chat input state
 */
export interface ChatInputState {
  content: string
  isSubmitting: boolean
  attachments?: File[]
}

/**
 * Message list pagination
 */
export interface MessagePagination {
  offset: number
  limit: number
  hasMore: boolean
  totalCount: number
}

/**
 * Session filter options
 */
export interface SessionFilters {
  projectPath?: string
  status?: SessionStatus
  search?: string
  sortBy?: 'created_at' | 'updated_at' | 'title'
  sortOrder?: 'asc' | 'desc'
}

/**
 * Event filter options for the event viewer
 */
export interface EventFilters {
  types?: LoopEventType[]
  kinds?: EventKind[]
  since?: string
  until?: string
  includeErrors?: boolean
}

// =============================================================================
// Usage Statistics Types (from audit/types.go)
// =============================================================================

/**
 * Provider-specific usage statistics
 */
export interface ProviderStats {
  /** Provider name */
  provider: string
  /** Total input tokens used */
  totalTokensInput: number
  /** Number of input tokens that were served from cache */
  cachedTokensInput: number
  /** Total output tokens used */
  totalTokensOutput: number
  /** Total cost in USD */
  totalCostUsd: number
  /** Estimated cost savings from caching in USD */
  costSavingsUsd: number
  /** Total number of requests */
  totalRequests: number
}

/**
 * Aggregated usage statistics
 */
export interface UsageStats {
  /** Total input tokens used */
  totalTokensInput: number
  /** Number of input tokens that were served from cache */
  cachedTokensInput: number
  /** Percentage of input tokens served from cache (0-100) */
  cacheHitRate: number
  /** Total output tokens used */
  totalTokensOutput: number
  /** Total cost in USD */
  totalCostUsd: number
  /** Estimated cost savings from caching in USD */
  costSavingsUsd: number
  /** Total number of LLM requests */
  totalRequests: number
  /** Number of successful requests */
  successfulRequests: number
  /** Number of failed requests */
  failedRequests: number
  /** Average request duration in milliseconds */
  averageDurationMs: number
  /** Stats broken down by provider */
  byProvider?: Record<string, ProviderStats>
}

/**
 * Tool call statistics
 */
export interface ToolCallStats {
  /** Total number of tool calls requested */
  totalRequested: number
  /** Number of approved tool calls */
  totalApproved: number
  /** Number of rejected tool calls */
  totalRejected: number
  /** Number of auto-approved tool calls */
  totalAutoApproved: number
  /** Number of successfully completed tool calls */
  totalCompleted: number
  /** Number of failed tool calls */
  totalFailed: number
  /** Stats broken down by tool name */
  byTool?: Record<string, number>
}

/**
 * Combined usage summary for dashboard display
 */
export interface UsageSummaryData {
  /** Usage statistics */
  usage: UsageStats
  /** Tool call statistics */
  toolCalls: ToolCallStats
  /** Time period start */
  periodStart?: string
  /** Time period end */
  periodEnd?: string
  /** Active session count */
  activeSessionCount?: number
  /** Total session count */
  totalSessionCount?: number
}
