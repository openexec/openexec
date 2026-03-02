/**
 * Restart Approval Hook
 *
 * Provides restart request management for the approval workflow:
 * - Fetch pending and historical restart requests
 * - Approve, reject, or cancel restart requests
 * - WebSocket subscription for real-time updates
 * - Local state caching
 *
 * @module hooks/useRestartApproval
 */

import { useCallback, useEffect, useRef, useState } from 'react'
import type {
  RestartRequest,
  RestartApprovalAction,
  PreflightResult,
} from '../types/restart'

// =============================================================================
// API Configuration
// =============================================================================

export interface RestartApprovalApiConfig {
  /** Base URL for REST API */
  baseUrl: string
  /** Optional auth token */
  authToken?: string
  /** WebSocket URL for real-time updates */
  wsUrl?: string
  /** Poll interval in ms when WebSocket is not available (default: 5000) */
  pollInterval?: number
  /** Enable debug logging (default: false) */
  debug?: boolean
}

const DEFAULT_CONFIG = {
  pollInterval: 5000,
  debug: false,
}

// =============================================================================
// Hook Return Type
// =============================================================================

export interface UseRestartApprovalReturn {
  /** List of restart requests */
  requests: RestartRequest[]
  /** Whether requests are loading */
  loading: boolean
  /** Loading error message */
  error: string | undefined
  /** Currently selected request for dialog */
  selectedRequest: RestartRequest | undefined
  /** Whether there are any pending requests */
  hasPendingRequests: boolean
  /** Count of pending requests */
  pendingCount: number
  /** Whether approval action is in progress */
  actionLoading: boolean
  /** Fetch all restart requests */
  fetchRequests: () => Promise<void>
  /** Fetch only pending restart requests */
  fetchPendingRequests: () => Promise<void>
  /** Approve a restart request */
  approveRequest: (requestId: string, reason?: string) => Promise<void>
  /** Reject a restart request */
  rejectRequest: (requestId: string, reason?: string) => Promise<void>
  /** Cancel a restart request */
  cancelRequest: (requestId: string, reason?: string) => Promise<void>
  /** Run pre-flight checks for a request */
  runPreflightChecks: (requestId: string) => Promise<PreflightResult>
  /** Select a request for viewing in dialog */
  selectRequest: (request: RestartRequest | undefined) => void
  /** Dismiss all banners (temporarily hide pending requests) */
  dismissBanners: () => void
  /** Check if banners are dismissed */
  areBannersDismissed: boolean
  /** Clear the dismissed state */
  clearDismissed: () => void
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

  // Handle empty responses
  const text = await response.text()
  if (!text) {
    return undefined as T
  }

  return JSON.parse(text) as T
}

// =============================================================================
// useRestartApproval Hook
// =============================================================================

export function useRestartApproval(
  config: RestartApprovalApiConfig
): UseRestartApprovalReturn {
  const { baseUrl, authToken, wsUrl, pollInterval, debug } = {
    ...DEFAULT_CONFIG,
    ...config,
  }

  // Debug logging
  const log = useCallback(
    (...args: unknown[]) => {
      if (debug) {
        console.log('[useRestartApproval]', ...args)
      }
    },
    [debug]
  )

  // State
  const [requests, setRequests] = useState<RestartRequest[]>([])
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | undefined>()
  const [selectedRequest, setSelectedRequest] = useState<RestartRequest | undefined>()
  const [actionLoading, setActionLoading] = useState(false)
  const [bannersDismissed, setBannersDismissed] = useState(false)

  // Refs
  const pollIntervalRef = useRef<ReturnType<typeof setInterval> | null>(null)
  const wsRef = useRef<WebSocket | null>(null)
  const isMounted = useRef(true)

  // Computed values
  const pendingRequests = requests.filter((r) => r.status === 'pending')
  const hasPendingRequests = pendingRequests.length > 0
  const pendingCount = pendingRequests.length

  // Clear polling interval
  const clearPolling = useCallback(() => {
    if (pollIntervalRef.current) {
      clearInterval(pollIntervalRef.current)
      pollIntervalRef.current = null
    }
  }, [])

  // Fetch all restart requests
  const fetchRequests = useCallback(async () => {
    setLoading(true)
    setError(undefined)

    try {
      const url = `${baseUrl}/restart/requests`
      const data = await apiRequest<RestartRequest[]>(url, { method: 'GET' }, authToken)

      if (isMounted.current) {
        setRequests(data || [])
        log('Fetched requests:', data?.length || 0)
      }
    } catch (err) {
      const message = err instanceof Error ? err.message : 'Failed to fetch restart requests'
      if (isMounted.current) {
        setError(message)
        log('Fetch error:', message)
      }
    } finally {
      if (isMounted.current) {
        setLoading(false)
      }
    }
  }, [baseUrl, authToken, log])

  // Fetch only pending restart requests
  const fetchPendingRequests = useCallback(async () => {
    setLoading(true)
    setError(undefined)

    try {
      const url = `${baseUrl}/restart/requests?status=pending`
      const data = await apiRequest<RestartRequest[]>(url, { method: 'GET' }, authToken)

      if (isMounted.current) {
        // Merge with existing non-pending requests
        setRequests((prev) => {
          const nonPending = prev.filter((r) => r.status !== 'pending')
          return [...(data || []), ...nonPending]
        })
        log('Fetched pending requests:', data?.length || 0)
      }
    } catch (err) {
      const message = err instanceof Error ? err.message : 'Failed to fetch pending restart requests'
      if (isMounted.current) {
        setError(message)
        log('Fetch pending error:', message)
      }
    } finally {
      if (isMounted.current) {
        setLoading(false)
      }
    }
  }, [baseUrl, authToken, log])

  // Approve a restart request
  const approveRequest = useCallback(
    async (requestId: string, reason?: string) => {
      setActionLoading(true)
      setError(undefined)

      try {
        const url = `${baseUrl}/restart/requests/${requestId}/approve`
        await apiRequest<void>(
          url,
          {
            method: 'POST',
            body: JSON.stringify({ reason }),
          },
          authToken
        )

        // Update local state
        setRequests((prev) =>
          prev.map((r) =>
            r.id === requestId
              ? {
                  ...r,
                  status: 'approved' as const,
                  approvedAt: new Date().toISOString(),
                  updatedAt: new Date().toISOString(),
                }
              : r
          )
        )

        // Clear selection if this was the selected request
        if (selectedRequest?.id === requestId) {
          setSelectedRequest(undefined)
        }

        log('Approved request:', requestId)
      } catch (err) {
        const message = err instanceof Error ? err.message : 'Failed to approve restart request'
        setError(message)
        log('Approve error:', message)
        throw err
      } finally {
        setActionLoading(false)
      }
    },
    [baseUrl, authToken, selectedRequest?.id, log]
  )

  // Reject a restart request
  const rejectRequest = useCallback(
    async (requestId: string, reason?: string) => {
      setActionLoading(true)
      setError(undefined)

      try {
        const url = `${baseUrl}/restart/requests/${requestId}/reject`
        await apiRequest<void>(
          url,
          {
            method: 'POST',
            body: JSON.stringify({ reason }),
          },
          authToken
        )

        // Update local state
        setRequests((prev) =>
          prev.map((r) =>
            r.id === requestId
              ? {
                  ...r,
                  status: 'rejected' as const,
                  completedAt: new Date().toISOString(),
                  updatedAt: new Date().toISOString(),
                }
              : r
          )
        )

        // Clear selection if this was the selected request
        if (selectedRequest?.id === requestId) {
          setSelectedRequest(undefined)
        }

        log('Rejected request:', requestId)
      } catch (err) {
        const message = err instanceof Error ? err.message : 'Failed to reject restart request'
        setError(message)
        log('Reject error:', message)
        throw err
      } finally {
        setActionLoading(false)
      }
    },
    [baseUrl, authToken, selectedRequest?.id, log]
  )

  // Cancel a restart request
  const cancelRequest = useCallback(
    async (requestId: string, reason?: string) => {
      setActionLoading(true)
      setError(undefined)

      try {
        const url = `${baseUrl}/restart/requests/${requestId}/cancel`
        await apiRequest<void>(
          url,
          {
            method: 'POST',
            body: JSON.stringify({ reason }),
          },
          authToken
        )

        // Update local state
        setRequests((prev) =>
          prev.map((r) =>
            r.id === requestId
              ? {
                  ...r,
                  status: 'cancelled' as const,
                  completedAt: new Date().toISOString(),
                  updatedAt: new Date().toISOString(),
                }
              : r
          )
        )

        // Clear selection if this was the selected request
        if (selectedRequest?.id === requestId) {
          setSelectedRequest(undefined)
        }

        log('Cancelled request:', requestId)
      } catch (err) {
        const message = err instanceof Error ? err.message : 'Failed to cancel restart request'
        setError(message)
        log('Cancel error:', message)
        throw err
      } finally {
        setActionLoading(false)
      }
    },
    [baseUrl, authToken, selectedRequest?.id, log]
  )

  // Run pre-flight checks
  const runPreflightChecks = useCallback(
    async (requestId: string): Promise<PreflightResult> => {
      try {
        const url = `${baseUrl}/restart/requests/${requestId}/preflight`
        const result = await apiRequest<PreflightResult>(url, { method: 'GET' }, authToken)

        log('Preflight result:', result)
        return result
      } catch (err) {
        const message = err instanceof Error ? err.message : 'Failed to run pre-flight checks'
        log('Preflight error:', message)
        throw err
      }
    },
    [baseUrl, authToken, log]
  )

  // Select request for dialog
  const selectRequest = useCallback((request: RestartRequest | undefined) => {
    setSelectedRequest(request)
  }, [])

  // Dismiss banners
  const dismissBanners = useCallback(() => {
    setBannersDismissed(true)
    log('Banners dismissed')
  }, [log])

  // Clear dismissed state
  const clearDismissed = useCallback(() => {
    setBannersDismissed(false)
    log('Dismissed state cleared')
  }, [log])

  // WebSocket connection for real-time updates
  useEffect(() => {
    if (!wsUrl) {
      // Fall back to polling if no WebSocket URL
      log('No WebSocket URL, using polling')
      pollIntervalRef.current = setInterval(fetchPendingRequests, pollInterval)
      return () => clearPolling()
    }

    const connectWebSocket = () => {
      try {
        log('Connecting to WebSocket:', wsUrl)
        const ws = new WebSocket(wsUrl)

        ws.onopen = () => {
          log('WebSocket connected')
          // Subscribe to restart events
          ws.send(JSON.stringify({ type: 'subscribe', topic: 'restart' }))
        }

        ws.onmessage = (event) => {
          try {
            const message = JSON.parse(event.data)
            log('WebSocket message:', message)

            if (message.type === 'restart_request_created') {
              setRequests((prev) => [message.payload as RestartRequest, ...prev])
              setBannersDismissed(false) // Show banner for new requests
            } else if (message.type === 'restart_request_updated') {
              const updated = message.payload as RestartRequest
              setRequests((prev) =>
                prev.map((r) => (r.id === updated.id ? updated : r))
              )
            } else if (message.type === 'restart_request_deleted') {
              setRequests((prev) => prev.filter((r) => r.id !== message.payload.id))
            }
          } catch (err) {
            log('WebSocket message parse error:', err)
          }
        }

        ws.onclose = (event) => {
          log('WebSocket closed:', event.code, event.reason)
          wsRef.current = null
          // Fall back to polling on disconnect
          if (isMounted.current) {
            pollIntervalRef.current = setInterval(fetchPendingRequests, pollInterval)
          }
        }

        ws.onerror = (error) => {
          log('WebSocket error:', error)
        }

        wsRef.current = ws
      } catch (err) {
        log('WebSocket connection error:', err)
        // Fall back to polling
        pollIntervalRef.current = setInterval(fetchPendingRequests, pollInterval)
      }
    }

    connectWebSocket()

    return () => {
      clearPolling()
      if (wsRef.current) {
        wsRef.current.close()
        wsRef.current = null
      }
    }
  }, [wsUrl, pollInterval, fetchPendingRequests, clearPolling, log])

  // Initial fetch
  useEffect(() => {
    fetchRequests()
  }, [fetchRequests])

  // Cleanup on unmount
  useEffect(() => {
    isMounted.current = true
    return () => {
      isMounted.current = false
      clearPolling()
      if (wsRef.current) {
        wsRef.current.close()
        wsRef.current = null
      }
    }
  }, [clearPolling])

  return {
    requests,
    loading,
    error,
    selectedRequest,
    hasPendingRequests,
    pendingCount,
    actionLoading,
    fetchRequests,
    fetchPendingRequests,
    approveRequest,
    rejectRequest,
    cancelRequest,
    runPreflightChecks,
    selectRequest,
    dismissBanners,
    areBannersDismissed: bannersDismissed,
    clearDismissed,
  }
}

export default useRestartApproval
