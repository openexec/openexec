/**
 * useRollback Hook
 * Manages rollback/restore operations for file backups.
 *
 * Provides:
 * - State management for rollback UI
 * - API calls to restore backups
 * - Diff generation between backup and current file
 * - Error handling and status tracking
 *
 * @module hooks/useRollback
 */

import { useState, useCallback, useRef } from 'react'
import type {
  BackupMetadata,
  RollbackStatus,
  RollbackState,
  RestoreResult,
  BackupDiff,
} from '../types/backup'
import type { Patch, PatchStats } from '../types/diff'

// =============================================================================
// Configuration
// =============================================================================

export interface RollbackApiConfig {
  /** Base URL for backup API endpoints */
  baseUrl: string
  /** Session ID for the current session */
  sessionId?: string
  /** Custom fetch function (for testing) */
  fetchFn?: typeof fetch
  /** Timeout for API requests in ms (default: 30000) */
  timeout?: number
}

// =============================================================================
// Hook Return Type
// =============================================================================

export interface UseRollbackReturn {
  /** Current rollback state */
  state: RollbackState
  /** Whether rollback is in progress */
  isLoading: boolean
  /** Whether rollback modal should be shown */
  isModalOpen: boolean
  /** The backup being rolled back (if any) */
  selectedBackup: BackupMetadata | undefined
  /** Diff between backup and current file */
  diff: Patch | undefined
  /** Diff statistics */
  diffStats: PatchStats | undefined
  /** Whether current file exists */
  currentFileExists: boolean
  /** Current file size (if exists) */
  currentFileSize: number | undefined
  /** Warning messages for the rollback */
  warnings: string[]
  /** Error message (if any) */
  error: string | undefined

  /** Start a rollback operation - opens modal with confirmation */
  initiateRollback: (backup: BackupMetadata) => Promise<void>
  /** Confirm and execute the rollback */
  confirmRollback: () => Promise<RestoreResult>
  /** Cancel the rollback and close modal */
  cancelRollback: () => void
  /** Close the modal */
  closeModal: () => void
  /** Reset state to idle */
  reset: () => void
}

// =============================================================================
// Default configuration
// =============================================================================

const DEFAULT_CONFIG: Omit<Required<RollbackApiConfig>, 'baseUrl' | 'sessionId'> = {
  fetchFn: fetch,
  timeout: 30000,
}

// =============================================================================
// API Response Types
// =============================================================================

interface DiffResponse {
  backupId: string
  currentFileExists: boolean
  currentChecksum?: string
  isDifferent: boolean
  currentSize?: number
  diff?: string
  isBinary: boolean
}

interface RestoreResponse {
  success: boolean
  backupId: string
  filePath: string
  error?: string
  checksumVerified: boolean
}

// =============================================================================
// Hook Implementation
// =============================================================================

export function useRollback(config: RollbackApiConfig): UseRollbackReturn {
  const configRef = useRef({ ...DEFAULT_CONFIG, ...config })

  // State
  const [state, setState] = useState<RollbackState>({
    status: 'idle',
  })
  const [selectedBackup, setSelectedBackup] = useState<BackupMetadata | undefined>()
  const [isModalOpen, setIsModalOpen] = useState(false)
  const [diff, setDiff] = useState<Patch | undefined>()
  const [diffStats, setDiffStats] = useState<PatchStats | undefined>()
  const [currentFileExists, setCurrentFileExists] = useState(true)
  const [currentFileSize, setCurrentFileSize] = useState<number | undefined>()
  const [warnings, setWarnings] = useState<string[]>([])

  // Computed
  const isLoading = state.status === 'loading' || state.status === 'restoring'
  const error = state.error

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

  // Parse unified diff into Patch structure
  const parseDiff = useCallback((diffContent: string): { patch: Patch; stats: PatchStats } => {
    // Simple diff parser for unified diff format
    const files: Patch['files'] = []
    let currentFile: Patch['files'][0] | null = null
    let currentHunk: Patch['files'][0]['hunks'][0] | null = null

    const lines = diffContent.split('\n')

    for (const line of lines) {
      // Parse file headers
      if (line.startsWith('--- ')) {
        const fileName = line.slice(4).split('\t')[0]
        currentFile = {
          oldName: fileName.startsWith('a/') ? fileName.slice(2) : fileName,
          newName: '',
          hunks: [],
          isBinary: false,
          isNew: false,
          isDeleted: false,
          isRenamed: false,
          gitHeaders: [],
        }
      } else if (line.startsWith('+++ ') && currentFile) {
        const fileName = line.slice(4).split('\t')[0]
        currentFile.newName = fileName.startsWith('b/') ? fileName.slice(2) : fileName
        files.push(currentFile)
      } else if (line.startsWith('@@') && currentFile) {
        // Parse hunk header: @@ -1,3 +1,2 @@
        const match = line.match(/@@ -(\d+),?(\d*) \+(\d+),?(\d*) @@/)
        if (match) {
          currentHunk = {
            oldStart: parseInt(match[1], 10),
            oldCount: parseInt(match[2] || '1', 10),
            newStart: parseInt(match[3], 10),
            newCount: parseInt(match[4] || '1', 10),
            header: line,
            lines: [],
          }
          currentFile.hunks.push(currentHunk)
        }
      } else if (currentHunk) {
        // Parse diff lines
        if (line.startsWith('+')) {
          currentHunk.lines.push({ type: 'add', content: line.slice(1) })
        } else if (line.startsWith('-')) {
          currentHunk.lines.push({ type: 'remove', content: line.slice(1) })
        } else if (line.startsWith(' ') || line === '') {
          currentHunk.lines.push({ type: 'context', content: line.slice(1) || '' })
        }
      }
    }

    // Calculate stats
    let additions = 0
    let deletions = 0
    let totalHunks = 0

    for (const file of files) {
      totalHunks += file.hunks.length
      for (const hunk of file.hunks) {
        for (const hunkLine of hunk.lines) {
          if (hunkLine.type === 'add') additions++
          if (hunkLine.type === 'remove') deletions++
        }
      }
    }

    return {
      patch: { files, rawContent: diffContent },
      stats: {
        filesChanged: files.length,
        additions,
        deletions,
        totalHunks,
      },
    }
  }, [])

  // Generate warnings based on backup and current file state
  const generateWarnings = useCallback(
    (backup: BackupMetadata, diffData: BackupDiff): string[] => {
      const warningsList: string[] = []

      // Check if backup was previously restored
      if (backup.restored) {
        warningsList.push('This backup was previously restored')
      }

      // Check backup age
      const backupAge = Date.now() - new Date(backup.createdAt).getTime()
      const oneWeek = 7 * 24 * 60 * 60 * 1000
      if (backupAge > oneWeek) {
        warningsList.push('This backup is older than 7 days')
      }

      // Check if file was significantly modified
      if (diffData.currentSize !== undefined && backup.size > 0) {
        const sizeRatio = diffData.currentSize / backup.size
        if (sizeRatio > 2 || sizeRatio < 0.5) {
          warningsList.push('File size has changed significantly since backup')
        }
      }

      // Binary file warning
      if (diffData.isBinary) {
        warningsList.push('This is a binary file - diff preview is not available')
      }

      return warningsList
    },
    []
  )

  // Initiate rollback - fetch diff and show modal
  const initiateRollback = useCallback(
    async (backup: BackupMetadata): Promise<void> => {
      setSelectedBackup(backup)
      setState({ status: 'loading', backup })
      setIsModalOpen(true)
      setDiff(undefined)
      setDiffStats(undefined)
      setWarnings([])

      try {
        // Fetch diff between backup and current file
        const diffData = await apiRequest<DiffResponse>(
          `/backups/${backup.id}/diff`
        )

        setCurrentFileExists(diffData.currentFileExists)
        setCurrentFileSize(diffData.currentSize)

        // Parse diff if available
        if (diffData.diff) {
          const { patch, stats } = parseDiff(diffData.diff)
          setDiff(patch)
          setDiffStats(stats)
        }

        // Generate warnings
        const backupDiff: BackupDiff = {
          backupId: diffData.backupId,
          currentFileExists: diffData.currentFileExists,
          currentChecksum: diffData.currentChecksum,
          isDifferent: diffData.isDifferent,
          currentSize: diffData.currentSize,
          diff: diffData.diff,
          isBinary: diffData.isBinary,
        }
        setWarnings(generateWarnings(backup, backupDiff))

        setState({ status: 'confirming', backup })
      } catch (err) {
        const errorMessage = err instanceof Error ? err.message : 'Failed to load backup diff'
        setState({ status: 'error', backup, error: errorMessage })
      }
    },
    [apiRequest, parseDiff, generateWarnings]
  )

  // Confirm and execute rollback
  const confirmRollback = useCallback(async (): Promise<RestoreResult> => {
    if (!selectedBackup) {
      throw new Error('No backup selected')
    }

    setState({ status: 'restoring', backup: selectedBackup })

    try {
      const result = await apiRequest<RestoreResponse>(
        `/backups/${selectedBackup.id}/restore`,
        { method: 'POST' }
      )

      if (result.success) {
        setState({
          status: 'success',
          backup: selectedBackup,
          result: {
            success: true,
            backupId: result.backupId,
            filePath: result.filePath,
            checksumVerified: result.checksumVerified,
          },
        })
      } else {
        throw new Error(result.error || 'Restore failed')
      }

      return {
        success: result.success,
        backupId: result.backupId,
        filePath: result.filePath,
        error: result.error,
        checksumVerified: result.checksumVerified,
      }
    } catch (err) {
      const errorMessage = err instanceof Error ? err.message : 'Restore failed'
      setState({
        status: 'error',
        backup: selectedBackup,
        error: errorMessage,
      })
      return {
        success: false,
        backupId: selectedBackup.id,
        filePath: selectedBackup.originalPath,
        error: errorMessage,
        checksumVerified: false,
      }
    }
  }, [selectedBackup, apiRequest])

  // Cancel rollback
  const cancelRollback = useCallback(() => {
    setIsModalOpen(false)
    setState({ status: 'idle' })
    setSelectedBackup(undefined)
    setDiff(undefined)
    setDiffStats(undefined)
    setWarnings([])
  }, [])

  // Close modal
  const closeModal = useCallback(() => {
    setIsModalOpen(false)
    // Don't reset state immediately to allow success/error display
    if (state.status !== 'success' && state.status !== 'error') {
      setState({ status: 'idle' })
      setSelectedBackup(undefined)
    }
  }, [state.status])

  // Reset state
  const reset = useCallback(() => {
    setIsModalOpen(false)
    setState({ status: 'idle' })
    setSelectedBackup(undefined)
    setDiff(undefined)
    setDiffStats(undefined)
    setCurrentFileExists(true)
    setCurrentFileSize(undefined)
    setWarnings([])
  }, [])

  return {
    state,
    isLoading,
    isModalOpen,
    selectedBackup,
    diff,
    diffStats,
    currentFileExists,
    currentFileSize,
    warnings,
    error,
    initiateRollback,
    confirmRollback,
    cancelRollback,
    closeModal,
    reset,
  }
}

export default useRollback
