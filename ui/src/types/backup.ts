/**
 * Backup Types for OpenExec UI
 *
 * These types mirror the Go backend types from:
 * - internal/mcp/backup.go (BackupMetadata, BackupStats, etc.)
 *
 * @module types/backup
 */

// =============================================================================
// Backup Metadata Types
// =============================================================================

/**
 * Metadata about a backup file
 */
export interface BackupMetadata {
  /** Unique identifier for this backup */
  id: string
  /** Absolute path of the original file */
  originalPath: string
  /** Path where the backup is stored */
  backupPath: string
  /** SHA256 hash of the original file content */
  checksum: string
  /** Size of the original file in bytes */
  size: number
  /** When the backup was created */
  createdAt: string
  /** Original file's permissions (as octal string) */
  fileMode: number
  /** Optional session identifier for grouping backups */
  sessionId?: string
  /** Whether this backup has been restored */
  restored: boolean
  /** When the backup was restored (if applicable) */
  restoredAt?: string
}

// =============================================================================
// Backup Statistics Types
// =============================================================================

/**
 * Statistics about the backup manager state
 */
export interface BackupStats {
  /** Total number of backups */
  totalBackups: number
  /** Number of unique files backed up */
  totalFiles: number
  /** Total size of all backups in bytes */
  totalSizeBytes: number
  /** When the oldest backup was created */
  oldestBackup?: string
  /** When the newest backup was created */
  newestBackup?: string
}

// =============================================================================
// Backup Operation Types
// =============================================================================

/**
 * Result of a restore operation
 */
export interface RestoreResult {
  /** Whether the restore was successful */
  success: boolean
  /** The backup that was restored */
  backupId: string
  /** Path of the restored file */
  filePath: string
  /** Error message if restore failed */
  error?: string
  /** Whether checksum was verified */
  checksumVerified: boolean
}

/**
 * Options for listing backups
 */
export interface ListBackupsOptions {
  /** Filter by original file path */
  filePath?: string
  /** Filter by session ID */
  sessionId?: string
  /** Maximum number of backups to return */
  limit?: number
  /** Whether to include restored backups */
  includeRestored?: boolean
}

// =============================================================================
// Rollback UI State Types
// =============================================================================

/**
 * State of a rollback operation in the UI
 */
export type RollbackStatus = 'idle' | 'loading' | 'confirming' | 'restoring' | 'success' | 'error'

/**
 * Rollback operation state
 */
export interface RollbackState {
  /** Current status of the rollback */
  status: RollbackStatus
  /** The backup being rolled back to */
  backup?: BackupMetadata
  /** Error message if rollback failed */
  error?: string
  /** Result of the restore operation */
  result?: RestoreResult
}

/**
 * Information for displaying backup in UI
 */
export interface BackupDisplayInfo extends BackupMetadata {
  /** Formatted time since creation */
  timeAgo: string
  /** Human-readable file size */
  formattedSize: string
  /** Short filename for display */
  fileName: string
  /** Directory path for display */
  directory: string
  /** Whether this is the most recent backup for this file */
  isLatest: boolean
}

// =============================================================================
// Comparison Types
// =============================================================================

/**
 * Difference between backup and current file state
 */
export interface BackupDiff {
  /** The backup being compared */
  backupId: string
  /** Whether the current file exists */
  currentFileExists: boolean
  /** Current file checksum (if exists) */
  currentChecksum?: string
  /** Whether the content is different */
  isDifferent: boolean
  /** Current file size (if exists) */
  currentSize?: number
  /** Unified diff content (if text file) */
  diff?: string
  /** Whether the file is binary */
  isBinary: boolean
}

// =============================================================================
// Constants
// =============================================================================

/**
 * Rollback status display configuration
 */
export const ROLLBACK_STATUS_INFO: Record<RollbackStatus, { label: string; color: string }> = {
  idle: { label: 'Ready', color: '#8b949e' },
  loading: { label: 'Loading...', color: '#58a6ff' },
  confirming: { label: 'Confirm Rollback', color: '#f0883e' },
  restoring: { label: 'Restoring...', color: '#58a6ff' },
  success: { label: 'Restored', color: '#238636' },
  error: { label: 'Failed', color: '#da3633' },
}

/**
 * Format bytes to human-readable size
 */
export function formatFileSize(bytes: number): string {
  if (bytes === 0) return '0 B'
  const k = 1024
  const sizes = ['B', 'KB', 'MB', 'GB', 'TB']
  const i = Math.floor(Math.log(bytes) / Math.log(k))
  return `${parseFloat((bytes / Math.pow(k, i)).toFixed(1))} ${sizes[i]}`
}

/**
 * Format a timestamp to relative time
 */
export function formatTimeAgo(timestamp: string): string {
  const now = new Date()
  const then = new Date(timestamp)
  const seconds = Math.floor((now.getTime() - then.getTime()) / 1000)

  if (seconds < 60) return 'just now'
  if (seconds < 3600) return `${Math.floor(seconds / 60)}m ago`
  if (seconds < 86400) return `${Math.floor(seconds / 3600)}h ago`
  if (seconds < 604800) return `${Math.floor(seconds / 86400)}d ago`
  return then.toLocaleDateString()
}

/**
 * Convert BackupMetadata to BackupDisplayInfo
 */
export function toDisplayInfo(
  backup: BackupMetadata,
  isLatest: boolean = false
): BackupDisplayInfo {
  const pathParts = backup.originalPath.split('/')
  const fileName = pathParts.pop() || backup.originalPath
  const directory = pathParts.join('/') || '/'

  return {
    ...backup,
    timeAgo: formatTimeAgo(backup.createdAt),
    formattedSize: formatFileSize(backup.size),
    fileName,
    directory,
    isLatest,
  }
}
