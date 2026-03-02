/**
 * Provider Availability Hook
 *
 * Provides real-time provider availability checking:
 * - Fetches provider availability status from the backend
 * - Periodic polling for live updates
 * - Caching for performance
 * - Error handling and retry logic
 *
 * @module hooks/useProviderAvailability
 */

import { useCallback, useEffect, useRef, useState } from 'react'
import type { ProviderInfo, ModelInfo } from '../types'

// =============================================================================
// API Types
// =============================================================================

/**
 * Provider status from the backend API
 */
export interface ProviderStatus {
  /** Provider name (e.g., "openai", "anthropic", "gemini") */
  name: string
  /** Whether the provider is available */
  available: boolean
  /** Reason for availability status */
  reason?: string
  /** List of available models for this provider */
  models?: string[]
}

/**
 * Model info response from the backend
 */
export interface ModelInfoResponse {
  id: string
  name: string
  provider: string
  contextWindow: number
  maxOutputTokens: number
  pricePerMInputTokens: number
  pricePerMOutputTokens: number
  supportsTools: boolean
  supportsStreaming: boolean
  supportsVision: boolean
  deprecated?: boolean
}

/**
 * Combined availability response
 */
export interface AvailabilityResponse {
  providers: ProviderStatus[]
  models?: ModelInfoResponse[]
}

// =============================================================================
// Configuration
// =============================================================================

export interface ProviderAvailabilityConfig {
  /** Base URL for REST API */
  baseUrl: string
  /** Optional auth token */
  authToken?: string
  /** Polling interval in ms (default: 60000 = 1 minute) */
  pollingInterval?: number
  /** Whether to enable polling (default: true) */
  enablePolling?: boolean
  /** Cache TTL in ms (default: 30000 = 30 seconds) */
  cacheTTL?: number
  /** Enable debug logging (default: false) */
  debug?: boolean
}

const DEFAULT_CONFIG = {
  pollingInterval: 60000,
  enablePolling: true,
  cacheTTL: 30000,
  debug: false,
}

// =============================================================================
// Hook Return Type
// =============================================================================

export interface UseProviderAvailabilityReturn {
  /** List of providers with availability status */
  providers: ProviderInfo[]
  /** Whether availability data is loading */
  loading: boolean
  /** Error message if fetch failed */
  error: string | undefined
  /** Last successful fetch timestamp */
  lastUpdated: Date | undefined
  /** Force refresh availability data */
  refresh: () => Promise<void>
  /** Check if a specific provider is available */
  isProviderAvailable: (providerId: string) => boolean
  /** Get status message for a provider */
  getProviderStatus: (providerId: string) => string | undefined
  /** Get available models for a provider */
  getProviderModels: (providerId: string) => ModelInfo[]
  /** Check if any provider is available */
  hasAvailableProviders: boolean
  /** Get list of available provider IDs */
  availableProviderIds: string[]
}

// =============================================================================
// Cache Management
// =============================================================================

interface CacheEntry {
  providers: ProviderInfo[]
  timestamp: number
}

let availabilityCache: CacheEntry | null = null

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
    return {} as T
  }

  return JSON.parse(text) as T
}

// =============================================================================
// Provider Display Names
// =============================================================================

const PROVIDER_DISPLAY_NAMES: Record<string, string> = {
  anthropic: 'Anthropic',
  openai: 'OpenAI',
  gemini: 'Google Gemini',
  google: 'Google Gemini',
}

function getProviderDisplayName(providerId: string): string {
  return PROVIDER_DISPLAY_NAMES[providerId.toLowerCase()] || providerId
}

// =============================================================================
// Transform Functions
// =============================================================================

function transformModelInfo(model: ModelInfoResponse): ModelInfo {
  return {
    id: model.id,
    name: model.name,
    provider: model.provider,
    contextWindow: model.contextWindow,
    maxOutputTokens: model.maxOutputTokens,
    pricePerMInputTokens: model.pricePerMInputTokens,
    pricePerMOutputTokens: model.pricePerMOutputTokens,
    supportsTools: model.supportsTools,
    supportsStreaming: model.supportsStreaming,
    supportsVision: model.supportsVision,
  }
}

function transformProviderStatus(
  status: ProviderStatus,
  models: ModelInfoResponse[]
): ProviderInfo {
  const providerModels = models
    .filter((m) => m.provider.toLowerCase() === status.name.toLowerCase())
    .filter((m) => !m.deprecated)
    .map(transformModelInfo)

  return {
    id: status.name,
    name: getProviderDisplayName(status.name),
    models: providerModels,
    isAvailable: status.available,
    statusMessage: status.available ? undefined : status.reason || 'Not configured',
  }
}

// =============================================================================
// useProviderAvailability Hook
// =============================================================================

export function useProviderAvailability(
  config: ProviderAvailabilityConfig
): UseProviderAvailabilityReturn {
  const mergedConfig = { ...DEFAULT_CONFIG, ...config }
  const { baseUrl, authToken, pollingInterval, enablePolling, cacheTTL, debug } = mergedConfig

  // State
  const [providers, setProviders] = useState<ProviderInfo[]>([])
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | undefined>()
  const [lastUpdated, setLastUpdated] = useState<Date | undefined>()

  // Refs
  const pollingTimerRef = useRef<ReturnType<typeof setInterval> | null>(null)
  const isMountedRef = useRef(true)

  // Debug logging
  const log = useCallback(
    (...args: unknown[]) => {
      if (debug) {
        console.log('[ProviderAvailability]', ...args)
      }
    },
    [debug]
  )

  // Check cache validity
  const isCacheValid = useCallback(() => {
    if (!availabilityCache) return false
    return Date.now() - availabilityCache.timestamp < cacheTTL
  }, [cacheTTL])

  // Fetch availability from API
  const fetchAvailability = useCallback(async (skipCache = false): Promise<void> => {
    // Check cache first (unless skipping)
    if (!skipCache && isCacheValid()) {
      log('Using cached availability data')
      setProviders(availabilityCache!.providers)
      setLastUpdated(new Date(availabilityCache!.timestamp))
      return
    }

    log('Fetching provider availability...')
    setLoading(true)
    setError(undefined)

    try {
      // Fetch provider status
      const statusUrl = `${baseUrl}/api/providers/status`
      const statusData = await apiRequest<ProviderStatus[]>(
        statusUrl,
        { method: 'GET' },
        authToken
      )

      // Fetch model info
      const modelsUrl = `${baseUrl}/api/providers/models`
      let modelsData: ModelInfoResponse[] = []
      try {
        modelsData = await apiRequest<ModelInfoResponse[]>(
          modelsUrl,
          { method: 'GET' },
          authToken
        )
      } catch (modelErr) {
        // Models endpoint may not exist yet, continue with status only
        log('Models endpoint not available, using status only:', modelErr)
      }

      // Transform to ProviderInfo format
      const providerInfos = statusData.map((status) =>
        transformProviderStatus(status, modelsData)
      )

      // Update cache
      const now = Date.now()
      availabilityCache = {
        providers: providerInfos,
        timestamp: now,
      }

      // Update state if still mounted
      if (isMountedRef.current) {
        setProviders(providerInfos)
        setLastUpdated(new Date(now))
        log('Updated provider availability:', providerInfos.map((p) => `${p.id}:${p.isAvailable}`).join(', '))
      }
    } catch (err) {
      const message = err instanceof Error ? err.message : 'Failed to fetch provider availability'
      log('Error fetching availability:', message)

      if (isMountedRef.current) {
        setError(message)

        // Fall back to cache if available (even if stale)
        if (availabilityCache) {
          log('Falling back to stale cache')
          setProviders(availabilityCache.providers)
        }
      }
    } finally {
      if (isMountedRef.current) {
        setLoading(false)
      }
    }
  }, [baseUrl, authToken, cacheTTL, isCacheValid, log])

  // Force refresh (bypasses cache)
  const refresh = useCallback(async () => {
    await fetchAvailability(true)
  }, [fetchAvailability])

  // Check if specific provider is available
  const isProviderAvailable = useCallback(
    (providerId: string): boolean => {
      const provider = providers.find(
        (p) => p.id.toLowerCase() === providerId.toLowerCase()
      )
      return provider?.isAvailable ?? false
    },
    [providers]
  )

  // Get status message for provider
  const getProviderStatus = useCallback(
    (providerId: string): string | undefined => {
      const provider = providers.find(
        (p) => p.id.toLowerCase() === providerId.toLowerCase()
      )
      return provider?.statusMessage
    },
    [providers]
  )

  // Get models for provider
  const getProviderModels = useCallback(
    (providerId: string): ModelInfo[] => {
      const provider = providers.find(
        (p) => p.id.toLowerCase() === providerId.toLowerCase()
      )
      return provider?.models ?? []
    },
    [providers]
  )

  // Computed values
  const hasAvailableProviders = providers.some((p) => p.isAvailable)
  const availableProviderIds = providers.filter((p) => p.isAvailable).map((p) => p.id)

  // Initial fetch on mount
  useEffect(() => {
    isMountedRef.current = true
    fetchAvailability()

    return () => {
      isMountedRef.current = false
    }
  }, [fetchAvailability])

  // Set up polling
  useEffect(() => {
    if (!enablePolling || pollingInterval <= 0) {
      return
    }

    log(`Starting availability polling (interval: ${pollingInterval}ms)`)
    pollingTimerRef.current = setInterval(() => {
      fetchAvailability()
    }, pollingInterval)

    return () => {
      if (pollingTimerRef.current) {
        log('Stopping availability polling')
        clearInterval(pollingTimerRef.current)
        pollingTimerRef.current = null
      }
    }
  }, [enablePolling, pollingInterval, fetchAvailability, log])

  return {
    providers,
    loading,
    error,
    lastUpdated,
    refresh,
    isProviderAvailable,
    getProviderStatus,
    getProviderModels,
    hasAvailableProviders,
    availableProviderIds,
  }
}

export default useProviderAvailability

/**
 * Clear the provider availability cache
 * Useful for testing or forcing a full refresh
 */
export function clearProviderAvailabilityCache(): void {
  availabilityCache = null
}
