/**
 * Message Operations Hook
 *
 * Provides message management:
 * - Fetch messages with pagination
 * - Send messages via WebSocket
 * - Handle streaming message chunks
 * - Manage message state
 *
 * @module hooks/useMessages
 */

import { useCallback, useRef, useState } from 'react'
import type {
  Message,
  StreamingChunk,
  MessagePagination,
} from '../types'

// =============================================================================
// API Configuration
// =============================================================================

export interface MessagesApiConfig {
  /** Base URL for REST API */
  baseUrl: string
  /** Optional auth token */
  authToken?: string
}

// =============================================================================
// Hook Return Type
// =============================================================================

export interface UseMessagesReturn {
  /** List of messages */
  messages: Message[]
  /** Whether messages are loading */
  messagesLoading: boolean
  /** Messages loading error */
  messagesError: string | undefined
  /** Currently streaming message (partial) */
  streamingMessage: Partial<Message> | undefined
  /** Pagination info */
  pagination: MessagePagination
  /** Fetch messages for a session */
  fetchMessages: (sessionId: string, options?: FetchMessagesOptions) => Promise<void>
  /** Load more messages (pagination) */
  loadMoreMessages: (sessionId: string) => Promise<void>
  /** Add a new message locally (optimistic update) */
  addMessage: (message: Message) => void
  /** Handle streaming chunk from WebSocket */
  handleStreamingChunk: (chunk: StreamingChunk) => void
  /** Complete streaming message */
  completeStreamingMessage: (message: Message) => void
  /** Clear all messages */
  clearMessages: () => void
  /** Get messages sorted by timestamp */
  getSortedMessages: () => Message[]
}

export interface FetchMessagesOptions {
  /** Number of messages to fetch */
  limit?: number
  /** Offset for pagination */
  offset?: number
  /** Whether to append to existing messages */
  append?: boolean
}

// =============================================================================
// HTTP Client Helper
// =============================================================================

async function apiRequest<T>(
  url: string,
  options: RequestInit,
  authToken?: string
): Promise<T> {
  const headers: HeadersInit = {
    'Content-Type': 'application/json',
    ...(authToken && { Authorization: `Bearer ${authToken}` }),
    ...options.headers,
  }

  const response = await fetch(url, {
    ...options,
    headers,
  })

  if (!response.ok) {
    const errorText = await response.text()
    throw new Error(`API error ${response.status}: ${errorText}`)
  }

  const text = await response.text()
  if (!text) {
    return undefined as T
  }

  return JSON.parse(text) as T
}

// =============================================================================
// Response Types
// =============================================================================

interface MessagesResponse {
  messages: Message[]
  pagination: MessagePagination
}

// =============================================================================
// useMessages Hook
// =============================================================================

export function useMessages(config: MessagesApiConfig): UseMessagesReturn {
  const { baseUrl, authToken } = config

  // State
  const [messages, setMessages] = useState<Message[]>([])
  const [messagesLoading, setMessagesLoading] = useState(false)
  const [messagesError, setMessagesError] = useState<string | undefined>()
  const [streamingMessage, setStreamingMessage] = useState<Partial<Message> | undefined>()
  const [pagination, setPagination] = useState<MessagePagination>({
    offset: 0,
    limit: 50,
    hasMore: false,
    totalCount: 0,
  })

  // Track current session to avoid stale updates
  const currentSessionId = useRef<string | undefined>()

  // Fetch messages
  const fetchMessages = useCallback(
    async (sessionId: string, options: FetchMessagesOptions = {}) => {
      const { limit = 50, offset = 0, append = false } = options

      currentSessionId.current = sessionId
      setMessagesLoading(true)
      setMessagesError(undefined)

      if (!append) {
        setMessages([])
      }

      try {
        const params = new URLSearchParams({
          limit: limit.toString(),
          offset: offset.toString(),
        })

        const url = `${baseUrl}/sessions/${sessionId}/messages?${params}`
        const data = await apiRequest<MessagesResponse>(url, { method: 'GET' }, authToken)

        // Check if we're still on the same session
        if (currentSessionId.current !== sessionId) {
          return
        }

        if (append) {
          setMessages((prev) => [...prev, ...data.messages])
        } else {
          setMessages(data.messages)
        }

        setPagination(data.pagination)
      } catch (err) {
        if (currentSessionId.current === sessionId) {
          const message = err instanceof Error ? err.message : 'Failed to fetch messages'
          setMessagesError(message)
        }
      } finally {
        if (currentSessionId.current === sessionId) {
          setMessagesLoading(false)
        }
      }
    },
    [baseUrl, authToken]
  )

  // Load more messages (pagination)
  const loadMoreMessages = useCallback(
    async (sessionId: string) => {
      if (!pagination.hasMore || messagesLoading) {
        return
      }

      await fetchMessages(sessionId, {
        limit: pagination.limit,
        offset: pagination.offset + pagination.limit,
        append: true,
      })
    },
    [fetchMessages, pagination, messagesLoading]
  )

  // Add message locally (optimistic update or WebSocket update)
  const addMessage = useCallback((message: Message) => {
    setMessages((prev) => {
      // Check if message already exists (dedup)
      const exists = prev.some((m) => m.id === message.id)
      if (exists) {
        // Update existing message
        return prev.map((m) => (m.id === message.id ? message : m))
      }
      // Add new message
      return [...prev, message]
    })

    // Update pagination total count
    setPagination((prev) => ({
      ...prev,
      totalCount: prev.totalCount + 1,
    }))
  }, [])

  // Handle streaming chunk from WebSocket
  const handleStreamingChunk = useCallback((chunk: StreamingChunk) => {
    setStreamingMessage((prev) => {
      if (!prev || prev.id !== chunk.messageId) {
        // Start new streaming message
        return {
          id: chunk.messageId,
          content: chunk.content,
          role: 'assistant' as const,
          toolCalls: chunk.toolCall ? [chunk.toolCall] : undefined,
        }
      }

      // Append to existing streaming message
      return {
        ...prev,
        content: prev.content + chunk.content,
        toolCalls: chunk.toolCall
          ? [...(prev.toolCalls || []), chunk.toolCall]
          : prev.toolCalls,
      }
    })
  }, [])

  // Complete streaming message
  const completeStreamingMessage = useCallback(
    (message: Message) => {
      setStreamingMessage(undefined)
      addMessage(message)
    },
    [addMessage]
  )

  // Clear messages
  const clearMessages = useCallback(() => {
    setMessages([])
    setStreamingMessage(undefined)
    setMessagesError(undefined)
    currentSessionId.current = undefined
    setPagination({
      offset: 0,
      limit: 50,
      hasMore: false,
      totalCount: 0,
    })
  }, [])

  // Get sorted messages
  const getSortedMessages = useCallback(() => {
    return [...messages].sort(
      (a, b) => new Date(a.createdAt).getTime() - new Date(b.createdAt).getTime()
    )
  }, [messages])

  return {
    messages,
    messagesLoading,
    messagesError,
    streamingMessage,
    pagination,
    fetchMessages,
    loadMoreMessages,
    addMessage,
    handleStreamingChunk,
    completeStreamingMessage,
    clearMessages,
    getSortedMessages,
  }
}

export default useMessages
