/**
 * useBackupHistory Hook
 * Manages backup history for files with listing, filtering, and deletion.
 *
 * Provides:
 * - Fetching backup list from API
 * - Filtering and pagination
 * - Backup deletion
 * - Statistics tracking
 * - Real-time updates via WebSocket
 *
 * @module hooks/useBackupHistory
 */

import { useState, useCallback, useEffect, useRef } from 'react'
import type {
  BackupMetadata,
  BackupStats,
  ListBackupsOptions,
} from '../types/backup'

// =============================================================================
// Configuration
// =============================================================================

export interface BackupHistoryApiConfig {
  /** Base URL for backup API endpoints */
  baseUrl: string
  /** Session ID for the current session */
  sessionId?: string
  /** Custom fetch function (for testing) */
  fetchFn?: typeof fetch
  /** Timeout for API requests in ms (default: 30000) */
  timeout?: number
  /** Enable auto-refresh (default: false) */
  autoRefresh?: boolean
  /** Auto-refresh interval in ms (default: 60000) */
  refreshInterval?: number
}

// =============================================================================
// Hook Return Type
// =============================================================================

export interface UseBackupHistoryReturn {
  /** List of backups */
  backups: BackupMetadata[]
  /** Backup statistics */
  stats: BackupStats | undefined
  /** Whether data is loading */
  isLoading: boolean
  /** Whether initial load is in progress */
  isInitialLoading: boolean
  /** Whether refresh is in progress */
  isRefreshing: boolean
  /** Error message if any */
  error: string | undefined
  /** Whether there are more backups to load */
  hasMore: boolean

  /** Fetch backups with optional filters */
  fetchBackups: (options?: ListBackupsOptions) => Promise<void>
  /** Refresh the backup list */
  refresh: () => Promise<void>
  /** Load more backups (pagination) */
  loadMore: () => Promise<void>
  /** Delete a backup */
  deleteBackup: (backupId: string) => Promise<boolean>
  /** Prune old backups */
  pruneBackups: () => Promise<number>
  /** Clear error state */
  clearError: () => void
  /** Add a backup to the list (for real-time updates) */
  addBackup: (backup: BackupMetadata) => void
  /** Remove a backup from the list (for real-time updates) */
  removeBackup: (backupId: string) => void
  /** Update a backup in the list (for real-time updates) */
  updateBackup: (backup: BackupMetadata) => void
}

// =============================================================================
// Default Configuration
// =============================================================================

const DEFAULT_CONFIG: Omit<Required<BackupHistoryApiConfig>, 'baseUrl' | 'sessionId'> = {
  fetchFn: fetch,
  timeout: 30000,
  autoRefresh: false,
  refreshInterval: 60000,
}

const DEFAULT_PAGE_SIZE = 50

// =============================================================================
// API Response Types
// =============================================================================

interface ListBackupsResponse {
  backups: BackupMetadata[]
  stats: BackupStats
  total: number
  hasMore: boolean
}

interface DeleteBackupResponse {
  success: boolean
  error?: string
}

interface PruneBackupsResponse {
  pruned: number
  error?: string
}

// =============================================================================
// Hook Implementation
// =============================================================================

export function useBackupHistory(config: BackupHistoryApiConfig): UseBackupHistoryReturn {
  const configRef = useRef({ ...DEFAULT_CONFIG, ...config })

  // Update config ref when props change
  useEffect(() => {
    configRef.current = { ...DEFAULT_CONFIG, ...config }
  }, [config])

  // State
  const [backups, setBackups] = useState<BackupMetadata[]>([])
  const [stats, setStats] = useState<BackupStats | undefined>()
  const [isLoading, setIsLoading] = useState(false)
  const [isInitialLoading, setIsInitialLoading] = useState(true)
  const [isRefreshing, setIsRefreshing] = useState(false)
  const [error, setError] = useState<string | undefined>()
  const [hasMore, setHasMore] = useState(false)

  // Pagination state
  const offsetRef = useRef(0)
  const currentOptionsRef = useRef<ListBackupsOptions>({})

  // Helper: Make API request with timeout
  const apiRequest = useCallback(
    async <T>(endpoint: string, options: RequestInit = {}): Promise<T> => {
      const { baseUrl, fetchFn, timeout, sessionId } = configRef.current

      const controller = new AbortController()
      const timeoutId = setTimeout(() => controller.abort(), timeout)

      try {
        const headers: Record<string, string> = {
          'Content-Type': 'application/json',
          ...(options.headers as Record<string, string>),
        }

        if (sessionId) {
          headers['X-Session-ID'] = sessionId
        }

        const response = await fetchFn(`${baseUrl}${endpoint}`, {
          ...options,
          headers,
          signal: controller.signal,
        })

        if (!response.ok) {
          const errorData = await response.json().catch(() => ({}))
          throw new Error(errorData.error || `HTTP ${response.status}`)
        }

        return response.json()
      } finally {
        clearTimeout(timeoutId)
      }
    },
    []
  )

  // Fetch backups
  const fetchBackups = useCallback(
    async (options: ListBackupsOptions = {}): Promise<void> => {
      setIsLoading(true)
      setError(undefined)
      currentOptionsRef.current = options
      offsetRef.current = 0

      try {
        const params = new URLSearchParams()
        if (options.filePath) params.set('filePath', options.filePath)
        if (options.sessionId) params.set('sessionId', options.sessionId)
        if (options.limit) params.set('limit', String(options.limit))
        if (options.includeRestored !== undefined) {
          params.set('includeRestored', String(options.includeRestored))
        }
        params.set('offset', '0')

        const queryString = params.toString()
        const endpoint = `/backups${queryString ? `?${queryString}` : ''}`

        const response = await apiRequest<ListBackupsResponse>(endpoint)

        setBackups(response.backups)
        setStats(response.stats)
        setHasMore(response.hasMore)
        offsetRef.current = response.backups.length
      } catch (err) {
        setError(err instanceof Error ? err.message : 'Failed to fetch backups')
      } finally {
        setIsLoading(false)
        setIsInitialLoading(false)
      }
    },
    [apiRequest]
  )

  // Refresh backups
  const refresh = useCallback(async (): Promise<void> => {
    setIsRefreshing(true)
    try {
      await fetchBackups(currentOptionsRef.current)
    } finally {
      setIsRefreshing(false)
    }
  }, [fetchBackups])

  // Load more backups
  const loadMore = useCallback(async (): Promise<void> => {
    if (isLoading || !hasMore) return

    setIsLoading(true)

    try {
      const options = currentOptionsRef.current
      const params = new URLSearchParams()
      if (options.filePath) params.set('filePath', options.filePath)
      if (options.sessionId) params.set('sessionId', options.sessionId)
      params.set('limit', String(options.limit || DEFAULT_PAGE_SIZE))
      params.set('offset', String(offsetRef.current))
      if (options.includeRestored !== undefined) {
        params.set('includeRestored', String(options.includeRestored))
      }

      const endpoint = `/backups?${params.toString()}`
      const response = await apiRequest<ListBackupsResponse>(endpoint)

      setBackups((prev) => [...prev, ...response.backups])
      setStats(response.stats)
      setHasMore(response.hasMore)
      offsetRef.current += response.backups.length
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load more backups')
    } finally {
      setIsLoading(false)
    }
  }, [apiRequest, isLoading, hasMore])

  // Delete a backup
  const deleteBackup = useCallback(
    async (backupId: string): Promise<boolean> => {
      try {
        const response = await apiRequest<DeleteBackupResponse>(
          `/backups/${backupId}`,
          { method: 'DELETE' }
        )

        if (response.success) {
          // Optimistically remove from list
          setBackups((prev) => prev.filter((b) => b.id !== backupId))
          // Update stats
          setStats((prev) => {
            if (!prev) return prev
            return {
              ...prev,
              totalBackups: Math.max(0, prev.totalBackups - 1),
            }
          })
          return true
        } else {
          setError(response.error || 'Failed to delete backup')
          return false
        }
      } catch (err) {
        setError(err instanceof Error ? err.message : 'Failed to delete backup')
        return false
      }
    },
    [apiRequest]
  )

  // Prune old backups
  const pruneBackups = useCallback(async (): Promise<number> => {
    try {
      const response = await apiRequest<PruneBackupsResponse>('/backups/prune', {
        method: 'POST',
      })

      if (response.error) {
        setError(response.error)
        return 0
      }

      // Refresh list after pruning
      await refresh()
      return response.pruned
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to prune backups')
      return 0
    }
  }, [apiRequest, refresh])

  // Clear error
  const clearError = useCallback(() => {
    setError(undefined)
  }, [])

  // Real-time update helpers
  const addBackup = useCallback((backup: BackupMetadata) => {
    setBackups((prev) => [backup, ...prev])
    setStats((prev) => {
      if (!prev) return prev
      return {
        ...prev,
        totalBackups: prev.totalBackups + 1,
        newestBackup: backup.createdAt,
      }
    })
  }, [])

  const removeBackup = useCallback((backupId: string) => {
    setBackups((prev) => prev.filter((b) => b.id !== backupId))
    setStats((prev) => {
      if (!prev) return prev
      return {
        ...prev,
        totalBackups: Math.max(0, prev.totalBackups - 1),
      }
    })
  }, [])

  const updateBackup = useCallback((backup: BackupMetadata) => {
    setBackups((prev) =>
      prev.map((b) => (b.id === backup.id ? backup : b))
    )
  }, [])

  // Initial fetch
  useEffect(() => {
    fetchBackups()
  }, []) // eslint-disable-line react-hooks/exhaustive-deps

  // Auto-refresh
  useEffect(() => {
    const { autoRefresh, refreshInterval } = configRef.current

    if (!autoRefresh) return

    const intervalId = setInterval(() => {
      refresh()
    }, refreshInterval)

    return () => clearInterval(intervalId)
  }, [refresh])

  return {
    backups,
    stats,
    isLoading,
    isInitialLoading,
    isRefreshing,
    error,
    hasMore,
    fetchBackups,
    refresh,
    loadMore,
    deleteBackup,
    pruneBackups,
    clearError,
    addBackup,
    removeBackup,
    updateBackup,
  }
}

export default useBackupHistory
