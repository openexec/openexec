/**
 * useUsageData Hook
 *
 * Fetches and manages usage statistics data from the API.
 * Provides cost, token, provider, and tool call statistics
 * for the usage dashboard.
 *
 * @module hooks/useUsageData
 */

import { useState, useCallback, useEffect, useRef } from 'react'
import type {
  UsageStats,
  ProviderStats,
  ToolCallStats,
} from '../types/chat'
import type { TokenHistoryDataPoint } from '../components/chat/cost/TokenHistoryChart'
import type { CostDataPoint } from '../components/chat/cost/CostTimelineChart'

/**
 * API response types matching backend structures
 */
interface UsageSummaryResponse {
  total_tokens_input: number
  cached_tokens_input: number
  cache_hit_rate: number
  total_tokens_output: number
  total_tokens: number
  total_cost_usd: number
  cost_savings_usd: number
  total_requests: number
  successful_requests: number
  failed_requests: number
  average_duration_ms: number
  by_provider?: Record<string, ProviderStatsResponse>
  period?: {
    since?: string
    until?: string
  }
}

interface ProviderStatsResponse {
  provider: string
  total_tokens_input: number
  cached_tokens_input: number
  total_tokens_output: number
  total_cost_usd: number
  cost_savings_usd: number
  total_requests: number
}

interface ProviderUsageResponse {
  providers: Array<{
    provider: string
    model: string
    session_count: number
    message_count: number
    total_tokens_input: number
    total_tokens_output: number
    total_cost_usd: number
  }>
  total: {
    session_count: number
    message_count: number
    total_tokens_input: number
    total_tokens_output: number
    total_tokens: number
    total_cost_usd: number
  }
}

interface ToolCallStatsResponse {
  total_requested: number
  total_approved: number
  total_rejected: number
  total_auto_approved: number
  total_completed: number
  total_failed: number
  by_tool?: Record<string, number>
  period?: {
    since?: string
    until?: string
  }
}

interface SessionUsageResponse {
  session_id: string
  message_count: number
  tool_call_count: number
  total_tokens_input: number
  total_tokens_output: number
  total_tokens: number
  total_cost_usd: number
  summary_count: number
  tokens_saved: number
}

interface AuditLogEntry {
  id: string
  timestamp: string
  event_type: string
  severity: string
  session_id?: string
  provider?: string
  model?: string
  tokens_input?: number
  tokens_output?: number
  cost_usd?: number
  duration_ms?: number
  success: boolean
  error_message?: string
}

interface AuditLogResponse {
  entries: AuditLogEntry[]
  total_count: number
  has_more: boolean
  limit: number
  offset: number
}

/**
 * Configuration options for the hook
 */
export interface UseUsageDataConfig {
  /** Base URL for the API */
  apiBaseUrl?: string
  /** Session ID for session-specific data */
  sessionId?: string
  /** Whether to auto-fetch on mount */
  autoFetch?: boolean
  /** Auto-refresh interval in milliseconds (0 to disable) */
  refreshInterval?: number
  /** Time range filter - start */
  since?: Date
  /** Time range filter - end */
  until?: Date
}

/**
 * Return type for the hook
 */
export interface UseUsageDataReturn {
  /** Overall usage statistics */
  usage: UsageStats | null
  /** Provider-specific statistics */
  providers: ProviderStats[]
  /** Tool call statistics */
  toolCalls: ToolCallStats | null
  /** Token history for charts */
  tokenHistory: TokenHistoryDataPoint[]
  /** Cost timeline for charts */
  costTimeline: CostDataPoint[]
  /** Session-specific usage */
  sessionUsage: SessionUsageResponse | null
  /** Loading state */
  loading: boolean
  /** Error message */
  error: string | null
  /** Timestamp of last successful fetch */
  lastUpdated: Date | null
  /** Fetch all usage data */
  fetchAll: () => Promise<void>
  /** Fetch summary only */
  fetchSummary: () => Promise<void>
  /** Fetch provider breakdown */
  fetchProviders: () => Promise<void>
  /** Fetch tool call stats */
  fetchToolCalls: () => Promise<void>
  /** Fetch session-specific usage */
  fetchSessionUsage: (sessionId: string) => Promise<void>
  /** Fetch audit logs for timeline data */
  fetchAuditLogs: (limit?: number) => Promise<void>
  /** Clear all data */
  clear: () => void
}

/**
 * Transform API response to frontend UsageStats type
 */
function transformUsageStats(response: UsageSummaryResponse): UsageStats {
  const stats: UsageStats = {
    totalTokensInput: response.total_tokens_input,
    cachedTokensInput: response.cached_tokens_input,
    cacheHitRate: response.cache_hit_rate,
    totalTokensOutput: response.total_tokens_output,
    totalCostUsd: response.total_cost_usd,
    costSavingsUsd: response.cost_savings_usd,
    totalRequests: response.total_requests,
    successfulRequests: response.successful_requests,
    failedRequests: response.failed_requests,
    averageDurationMs: response.average_duration_ms,
  }

  if (response.by_provider) {
    stats.byProvider = {}
    for (const [key, value] of Object.entries(response.by_provider)) {
      stats.byProvider[key] = {
        provider: value.provider,
        totalTokensInput: value.total_tokens_input,
        cachedTokensInput: value.cached_tokens_input,
        totalTokensOutput: value.total_tokens_output,
        totalCostUsd: value.total_cost_usd,
        costSavingsUsd: value.cost_savings_usd,
        totalRequests: value.total_requests,
      }
    }
  }

  return stats
}

/**
 * Transform provider usage response to ProviderStats array
 */
function transformProviderStats(response: ProviderUsageResponse): ProviderStats[] {
  return response.providers.map(p => ({
    provider: p.provider,
    totalTokensInput: p.total_tokens_input,
    cachedTokensInput: 0, // Not provided by this endpoint
    totalTokensOutput: p.total_tokens_output,
    totalCostUsd: p.total_cost_usd,
    costSavingsUsd: 0, // Not provided by this endpoint
    totalRequests: p.message_count, // Use message_count as request proxy
  }))
}

/**
 * Transform tool call stats response
 */
function transformToolCallStats(response: ToolCallStatsResponse): ToolCallStats {
  return {
    totalRequested: response.total_requested,
    totalApproved: response.total_approved,
    totalRejected: response.total_rejected,
    totalAutoApproved: response.total_auto_approved,
    totalCompleted: response.total_completed,
    totalFailed: response.total_failed,
    byTool: response.by_tool,
  }
}

/**
 * Transform audit log entries to token history data points
 */
function transformToTokenHistory(entries: AuditLogEntry[]): TokenHistoryDataPoint[] {
  // Filter to LLM request events with token data
  const llmEvents = entries.filter(
    e => e.event_type.startsWith('llm.') && e.tokens_input !== undefined
  )

  // Sort by timestamp ascending
  const sorted = [...llmEvents].sort(
    (a, b) => new Date(a.timestamp).getTime() - new Date(b.timestamp).getTime()
  )

  // Accumulate tokens over time
  let cumulativeInput = 0
  let cumulativeOutput = 0

  return sorted.map((entry, index) => {
    cumulativeInput += entry.tokens_input ?? 0
    cumulativeOutput += entry.tokens_output ?? 0

    return {
      timestamp: entry.timestamp,
      inputTokens: cumulativeInput,
      outputTokens: cumulativeOutput,
      costUsd: entry.cost_usd,
      label: `#${index + 1}`,
    }
  })
}

/**
 * Transform audit log entries to cost timeline data points
 */
function transformToCostTimeline(entries: AuditLogEntry[]): CostDataPoint[] {
  // Filter to events with cost data
  const costEvents = entries.filter(e => e.cost_usd !== undefined && e.cost_usd > 0)

  // Sort by timestamp ascending
  const sorted = [...costEvents].sort(
    (a, b) => new Date(a.timestamp).getTime() - new Date(b.timestamp).getTime()
  )

  // Accumulate cost over iterations
  let cumulativeCost = 0

  return sorted.map((entry, index) => {
    const iterationCost = entry.cost_usd ?? 0
    cumulativeCost += iterationCost

    return {
      iteration: index + 1,
      timestamp: entry.timestamp,
      cumulativeCost,
      iterationCost,
      inputTokens: entry.tokens_input,
      outputTokens: entry.tokens_output,
    }
  })
}

/**
 * Hook for fetching and managing usage data
 */
export function useUsageData(config: UseUsageDataConfig = {}): UseUsageDataReturn {
  const {
    apiBaseUrl = '/api',
    sessionId,
    autoFetch = true,
    refreshInterval = 0,
    since,
    until,
  } = config

  // State
  const [usage, setUsage] = useState<UsageStats | null>(null)
  const [providers, setProviders] = useState<ProviderStats[]>([])
  const [toolCalls, setToolCalls] = useState<ToolCallStats | null>(null)
  const [tokenHistory, setTokenHistory] = useState<TokenHistoryDataPoint[]>([])
  const [costTimeline, setCostTimeline] = useState<CostDataPoint[]>([])
  const [sessionUsage, setSessionUsage] = useState<SessionUsageResponse | null>(null)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [lastUpdated, setLastUpdated] = useState<Date | null>(null)

  // Ref for abort controller
  const abortControllerRef = useRef<AbortController | null>(null)

  /**
   * Build query string from time filters
   */
  const buildQueryString = useCallback((): string => {
    const params = new URLSearchParams()
    if (since) {
      params.set('since', since.toISOString())
    }
    if (until) {
      params.set('until', until.toISOString())
    }
    if (sessionId) {
      params.set('session_id', sessionId)
    }
    const qs = params.toString()
    return qs ? `?${qs}` : ''
  }, [since, until, sessionId])

  /**
   * Fetch summary data
   */
  const fetchSummary = useCallback(async (): Promise<void> => {
    try {
      const response = await fetch(
        `${apiBaseUrl}/usage/summary${buildQueryString()}`,
        { signal: abortControllerRef.current?.signal }
      )

      if (!response.ok) {
        throw new Error(`Failed to fetch usage summary: ${response.status}`)
      }

      const data: UsageSummaryResponse = await response.json()
      setUsage(transformUsageStats(data))
    } catch (err) {
      if (err instanceof Error && err.name === 'AbortError') return
      throw err
    }
  }, [apiBaseUrl, buildQueryString])

  /**
   * Fetch provider breakdown
   */
  const fetchProviders = useCallback(async (): Promise<void> => {
    try {
      const response = await fetch(
        `${apiBaseUrl}/usage/providers`,
        { signal: abortControllerRef.current?.signal }
      )

      if (!response.ok) {
        throw new Error(`Failed to fetch provider usage: ${response.status}`)
      }

      const data: ProviderUsageResponse = await response.json()
      setProviders(transformProviderStats(data))
    } catch (err) {
      if (err instanceof Error && err.name === 'AbortError') return
      throw err
    }
  }, [apiBaseUrl])

  /**
   * Fetch tool call statistics
   */
  const fetchToolCalls = useCallback(async (): Promise<void> => {
    try {
      const response = await fetch(
        `${apiBaseUrl}/usage/tools${buildQueryString()}`,
        { signal: abortControllerRef.current?.signal }
      )

      if (!response.ok) {
        throw new Error(`Failed to fetch tool call stats: ${response.status}`)
      }

      const data: ToolCallStatsResponse = await response.json()
      setToolCalls(transformToolCallStats(data))
    } catch (err) {
      if (err instanceof Error && err.name === 'AbortError') return
      throw err
    }
  }, [apiBaseUrl, buildQueryString])

  /**
   * Fetch session-specific usage
   */
  const fetchSessionUsage = useCallback(async (sessId: string): Promise<void> => {
    try {
      const response = await fetch(
        `${apiBaseUrl}/usage/sessions/${sessId}`,
        { signal: abortControllerRef.current?.signal }
      )

      if (!response.ok) {
        throw new Error(`Failed to fetch session usage: ${response.status}`)
      }

      const data: SessionUsageResponse = await response.json()
      setSessionUsage(data)
    } catch (err) {
      if (err instanceof Error && err.name === 'AbortError') return
      throw err
    }
  }, [apiBaseUrl])

  /**
   * Fetch audit logs for timeline data
   */
  const fetchAuditLogs = useCallback(async (limit: number = 100): Promise<void> => {
    try {
      const params = new URLSearchParams()
      params.set('limit', limit.toString())
      if (since) params.set('since', since.toISOString())
      if (until) params.set('until', until.toISOString())
      if (sessionId) params.set('session_id', sessionId)

      const response = await fetch(
        `${apiBaseUrl}/usage/audit-logs?${params.toString()}`,
        { signal: abortControllerRef.current?.signal }
      )

      if (!response.ok) {
        throw new Error(`Failed to fetch audit logs: ${response.status}`)
      }

      const data: AuditLogResponse = await response.json()

      // Transform to chart data
      setTokenHistory(transformToTokenHistory(data.entries))
      setCostTimeline(transformToCostTimeline(data.entries))
    } catch (err) {
      if (err instanceof Error && err.name === 'AbortError') return
      throw err
    }
  }, [apiBaseUrl, since, until, sessionId])

  /**
   * Fetch all usage data
   */
  const fetchAll = useCallback(async (): Promise<void> => {
    // Cancel any in-flight requests
    if (abortControllerRef.current) {
      abortControllerRef.current.abort()
    }
    abortControllerRef.current = new AbortController()

    setLoading(true)
    setError(null)

    try {
      // Fetch all data in parallel
      await Promise.all([
        fetchSummary(),
        fetchProviders(),
        fetchToolCalls(),
        fetchAuditLogs(100),
        sessionId ? fetchSessionUsage(sessionId) : Promise.resolve(),
      ])

      setLastUpdated(new Date())
    } catch (err) {
      if (err instanceof Error && err.name !== 'AbortError') {
        setError(err.message)
      }
    } finally {
      setLoading(false)
    }
  }, [fetchSummary, fetchProviders, fetchToolCalls, fetchAuditLogs, fetchSessionUsage, sessionId])

  /**
   * Clear all data
   */
  const clear = useCallback(() => {
    setUsage(null)
    setProviders([])
    setToolCalls(null)
    setTokenHistory([])
    setCostTimeline([])
    setSessionUsage(null)
    setError(null)
    setLastUpdated(null)
  }, [])

  // Auto-fetch on mount
  useEffect(() => {
    if (autoFetch) {
      fetchAll()
    }

    return () => {
      if (abortControllerRef.current) {
        abortControllerRef.current.abort()
      }
    }
  }, [autoFetch]) // eslint-disable-line react-hooks/exhaustive-deps

  // Auto-refresh timer
  useEffect(() => {
    if (refreshInterval <= 0) return

    const timer = setInterval(() => {
      fetchAll()
    }, refreshInterval)

    return () => clearInterval(timer)
  }, [refreshInterval, fetchAll])

  return {
    usage,
    providers,
    toolCalls,
    tokenHistory,
    costTimeline,
    sessionUsage,
    loading,
    error,
    lastUpdated,
    fetchAll,
    fetchSummary,
    fetchProviders,
    fetchToolCalls,
    fetchSessionUsage,
    fetchAuditLogs,
    clear,
  }
}

export default useUsageData
