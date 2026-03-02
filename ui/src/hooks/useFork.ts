/**
 * Fork Operations Hook
 *
 * Provides comprehensive session fork operations:
 * - Fork session at specific message point
 * - Get fork information for a session
 * - List forks of a session
 * - Navigate to forked sessions
 *
 * @module hooks/useFork
 */

import { useCallback, useState } from 'react'
import type { Session, Message } from '../types'
import type { ForkOptions, ForkResult } from '../components/chat/session/SessionForkDialog'

// =============================================================================
// API Configuration
// =============================================================================

export interface ForkApiConfig {
  /** Base URL for REST API */
  baseUrl: string
  /** Optional auth token */
  authToken?: string
}

// =============================================================================
// Fork Information Types
// =============================================================================

/**
 * Fork information for a session
 */
export interface ForkInfo {
  /** The session ID */
  sessionId: string
  /** Fork depth in the tree (0 = root) */
  forkDepth: number
  /** Parent session ID (if forked) */
  parentSessionId?: string
  /** Root session ID (original ancestor) */
  rootSessionId?: string
  /** Fork point message ID */
  forkPointMessageId?: string
  /** Chain of ancestor session IDs */
  ancestorChain: string[]
}

/**
 * Fork list item
 */
export interface ForkListItem {
  /** Forked session ID */
  sessionId: string
  /** Fork title */
  title: string
  /** When the fork was created */
  createdAt: string
  /** Fork point message ID */
  forkPointMessageId?: string
}

// =============================================================================
// Hook Return Type
// =============================================================================

export interface UseForkReturn {
  /** Fork dialog open state */
  isForkDialogOpen: boolean
  /** Session being forked */
  sessionToFork: Session | undefined
  /** Pre-selected fork point message */
  forkPointMessage: Message | undefined
  /** Fork operation loading state */
  isForking: boolean
  /** Fork error message */
  forkError: string | undefined
  /** Last successful fork result */
  lastForkResult: ForkResult | undefined
  /** Fork info for current session */
  forkInfo: ForkInfo | undefined
  /** Fork info loading state */
  forkInfoLoading: boolean
  /** List of forks for current session */
  sessionForks: ForkListItem[]
  /** Session forks loading state */
  sessionForksLoading: boolean
  /** Open the fork dialog for a session */
  openForkDialog: (session: Session, forkPointMessage?: Message) => void
  /** Close the fork dialog */
  closeForkDialog: () => void
  /** Execute a fork operation */
  forkSession: (
    sessionId: string,
    forkPointMessageId: string,
    options: ForkOptions
  ) => Promise<ForkResult>
  /** Get fork info for a session */
  getForkInfo: (sessionId: string) => Promise<ForkInfo>
  /** List forks of a session */
  listSessionForks: (sessionId: string) => Promise<ForkListItem[]>
  /** Clear fork error */
  clearForkError: () => void
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
// useFork Hook
// =============================================================================

export function useFork(config: ForkApiConfig): UseForkReturn {
  const { baseUrl, authToken } = config

  // Dialog state
  const [isForkDialogOpen, setIsForkDialogOpen] = useState(false)
  const [sessionToFork, setSessionToFork] = useState<Session | undefined>()
  const [forkPointMessage, setForkPointMessage] = useState<Message | undefined>()

  // Operation state
  const [isForking, setIsForking] = useState(false)
  const [forkError, setForkError] = useState<string | undefined>()
  const [lastForkResult, setLastForkResult] = useState<ForkResult | undefined>()

  // Fork info state
  const [forkInfo, setForkInfo] = useState<ForkInfo | undefined>()
  const [forkInfoLoading, setForkInfoLoading] = useState(false)

  // Session forks state
  const [sessionForks, setSessionForks] = useState<ForkListItem[]>([])
  const [sessionForksLoading, setSessionForksLoading] = useState(false)

  // Open fork dialog
  const openForkDialog = useCallback((session: Session, message?: Message) => {
    setSessionToFork(session)
    setForkPointMessage(message)
    setForkError(undefined)
    setLastForkResult(undefined)
    setIsForkDialogOpen(true)
  }, [])

  // Close fork dialog
  const closeForkDialog = useCallback(() => {
    setIsForkDialogOpen(false)
    setSessionToFork(undefined)
    setForkPointMessage(undefined)
  }, [])

  // Execute fork operation
  const forkSession = useCallback(
    async (
      sessionId: string,
      forkPointMessageId: string,
      options: ForkOptions
    ): Promise<ForkResult> => {
      setIsForking(true)
      setForkError(undefined)

      try {
        const url = `${baseUrl}/sessions/${sessionId}/fork`
        const body = {
          fork_point_message_id: forkPointMessageId,
          title: options.title,
          provider: options.provider || undefined,
          model: options.model || undefined,
          copy_messages: options.copyMessages,
          copy_tool_calls: options.copyToolCalls,
          copy_summaries: options.copySummaries,
        }

        const result = await apiRequest<{
          forked_session_id: string
          parent_session_id: string
          fork_point_message_id: string
          title: string
          provider: string
          model: string
          messages_copied: number
          tool_calls_copied: number
          summaries_copied: number
          fork_depth: number
          ancestor_chain: string[]
        }>(url, { method: 'POST', body: JSON.stringify(body) }, authToken)

        // Transform snake_case response to camelCase
        const forkResult: ForkResult = {
          forkedSessionId: result.forked_session_id,
          parentSessionId: result.parent_session_id,
          forkPointMessageId: result.fork_point_message_id,
          title: result.title,
          provider: result.provider,
          model: result.model,
          messagesCopied: result.messages_copied,
          toolCallsCopied: result.tool_calls_copied,
          summariesCopied: result.summaries_copied,
          forkDepth: result.fork_depth,
          ancestorChain: result.ancestor_chain,
        }

        setLastForkResult(forkResult)
        return forkResult
      } catch (err) {
        const message = err instanceof Error ? err.message : 'Failed to fork session'
        setForkError(message)
        throw err
      } finally {
        setIsForking(false)
      }
    },
    [baseUrl, authToken]
  )

  // Get fork info for a session
  const getForkInfo = useCallback(
    async (sessionId: string): Promise<ForkInfo> => {
      setForkInfoLoading(true)

      try {
        const url = `${baseUrl}/sessions/${sessionId}/fork-info`
        const result = await apiRequest<{
          session_id: string
          fork_depth: number
          parent_session_id?: string
          root_session_id?: string
          fork_point_message_id?: string
          ancestor_chain: string[]
        }>(url, { method: 'GET' }, authToken)

        const info: ForkInfo = {
          sessionId: result.session_id,
          forkDepth: result.fork_depth,
          parentSessionId: result.parent_session_id,
          rootSessionId: result.root_session_id,
          forkPointMessageId: result.fork_point_message_id,
          ancestorChain: result.ancestor_chain,
        }

        setForkInfo(info)
        return info
      } finally {
        setForkInfoLoading(false)
      }
    },
    [baseUrl, authToken]
  )

  // List forks of a session
  const listSessionForks = useCallback(
    async (sessionId: string): Promise<ForkListItem[]> => {
      setSessionForksLoading(true)

      try {
        const url = `${baseUrl}/sessions/${sessionId}/forks`
        const result = await apiRequest<
          Array<{
            session_id: string
            title: string
            created_at: string
            fork_point_message_id?: string
          }>
        >(url, { method: 'GET' }, authToken)

        const forks: ForkListItem[] = result.map((item) => ({
          sessionId: item.session_id,
          title: item.title,
          createdAt: item.created_at,
          forkPointMessageId: item.fork_point_message_id,
        }))

        setSessionForks(forks)
        return forks
      } finally {
        setSessionForksLoading(false)
      }
    },
    [baseUrl, authToken]
  )

  // Clear fork error
  const clearForkError = useCallback(() => {
    setForkError(undefined)
  }, [])

  return {
    isForkDialogOpen,
    sessionToFork,
    forkPointMessage,
    isForking,
    forkError,
    lastForkResult,
    forkInfo,
    forkInfoLoading,
    sessionForks,
    sessionForksLoading,
    openForkDialog,
    closeForkDialog,
    forkSession,
    getForkInfo,
    listSessionForks,
    clearForkError,
  }
}

export default useFork
