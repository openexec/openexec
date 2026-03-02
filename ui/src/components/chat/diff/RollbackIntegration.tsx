/**
 * RollbackIntegration Component
 * Integration layer that combines BackupHistoryPanel and RollbackModal
 * with hooks for a complete rollback workflow.
 *
 * @module components/chat/diff/RollbackIntegration
 */
import React, { useCallback, useMemo } from 'react'
import { useRollback } from '../../../hooks/useRollback'
import { useBackupHistory } from '../../../hooks/useBackupHistory'
import BackupHistoryPanel from './BackupHistoryPanel'
import RollbackModal from './RollbackModal'
import type { BackupMetadata } from '../../../types/backup'

export interface RollbackIntegrationProps {
  /** Base URL for backup API */
  apiBaseUrl: string
  /** Current session ID */
  sessionId?: string
  /** Filter backups by file path */
  filePath?: string
  /** Panel title */
  title?: string
  /** Whether to show stats */
  showStats?: boolean
  /** Whether to show filter bar */
  showFilter?: boolean
  /** Maximum panel height */
  maxHeight?: string | number
  /** Whether panel is in compact mode */
  compact?: boolean
  /** Enable auto-refresh */
  autoRefresh?: boolean
  /** Auto-refresh interval in ms */
  refreshInterval?: number
  /** Callback when a backup is restored */
  onRestoreComplete?: (backup: BackupMetadata) => void
  /** Callback when a backup is deleted */
  onDeleteComplete?: (backupId: string) => void
  /** Callback when an error occurs */
  onError?: (error: string) => void
}

const RollbackIntegration: React.FC<RollbackIntegrationProps> = ({
  apiBaseUrl,
  sessionId,
  filePath,
  title = 'Backup History',
  showStats = true,
  showFilter = true,
  maxHeight = '400px',
  compact = false,
  autoRefresh = false,
  refreshInterval = 60000,
  onRestoreComplete,
  onDeleteComplete,
  onError,
}) => {
  // Initialize backup history hook
  const backupHistory = useBackupHistory({
    baseUrl: apiBaseUrl,
    sessionId,
    autoRefresh,
    refreshInterval,
  })

  // Initialize rollback hook
  const rollback = useRollback({
    baseUrl: apiBaseUrl,
    sessionId,
  })

  // Filter backups by file path if provided
  const filteredBackups = useMemo(() => {
    if (!filePath) return backupHistory.backups
    return backupHistory.backups.filter((b) => b.originalPath === filePath)
  }, [backupHistory.backups, filePath])

  // Handle restore initiation
  const handleRestore = useCallback(
    async (backup: BackupMetadata) => {
      try {
        await rollback.initiateRollback(backup)
      } catch (err) {
        const errorMsg = err instanceof Error ? err.message : 'Failed to initiate rollback'
        onError?.(errorMsg)
      }
    },
    [rollback, onError]
  )

  // Handle restore confirmation
  const handleConfirmRollback = useCallback(async () => {
    try {
      const result = await rollback.confirmRollback()
      if (result.success && rollback.selectedBackup) {
        // Update backup in history to show it was restored
        backupHistory.updateBackup({
          ...rollback.selectedBackup,
          restored: true,
          restoredAt: new Date().toISOString(),
        })
        onRestoreComplete?.(rollback.selectedBackup)
      }
    } catch (err) {
      const errorMsg = err instanceof Error ? err.message : 'Restore failed'
      onError?.(errorMsg)
    }
  }, [rollback, backupHistory, onRestoreComplete, onError])

  // Handle backup deletion
  const handleDelete = useCallback(
    async (backup: BackupMetadata) => {
      const success = await backupHistory.deleteBackup(backup.id)
      if (success) {
        onDeleteComplete?.(backup.id)
      } else if (backupHistory.error) {
        onError?.(backupHistory.error)
      }
    },
    [backupHistory, onDeleteComplete, onError]
  )

  // Handle modal close
  const handleModalClose = useCallback(() => {
    rollback.closeModal()
  }, [rollback])

  // Handle modal cancel
  const handleModalCancel = useCallback(() => {
    rollback.cancelRollback()
  }, [rollback])

  // Combined error from both hooks
  const combinedError = rollback.error || backupHistory.error

  return (
    <div className="rollback-integration">
      {/* Backup History Panel */}
      <BackupHistoryPanel
        backups={filteredBackups}
        stats={backupHistory.stats}
        onRestore={handleRestore}
        onDelete={handleDelete}
        disabled={rollback.isLoading}
        compact={compact}
        maxHeight={maxHeight}
        title={title}
        showFilter={showFilter}
        showStats={showStats}
        isLoading={backupHistory.isInitialLoading}
        error={combinedError}
      />

      {/* Rollback Confirmation Modal */}
      {rollback.selectedBackup && (
        <RollbackModal
          backup={rollback.selectedBackup}
          isOpen={rollback.isModalOpen}
          onClose={handleModalClose}
          onConfirm={handleConfirmRollback}
          onCancel={handleModalCancel}
          status={rollback.state.status}
          diff={rollback.diff}
          diffStats={rollback.diffStats}
          currentFileExists={rollback.currentFileExists}
          currentFileSize={rollback.currentFileSize}
          warnings={rollback.warnings}
          error={rollback.error}
        />
      )}
    </div>
  )
}

export default RollbackIntegration
