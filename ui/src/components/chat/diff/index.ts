/**
 * Diff Components
 * Components for displaying unified diff patches and rollback functionality.
 * @module components/chat/diff
 */

// Diff visualization components
export { default as DiffViewer } from './DiffViewer'
export { default as DiffStats } from './DiffStats'
export { default as DiffFileCard } from './DiffFileCard'
export { default as DiffHunk } from './DiffHunk'
export { default as PatchPreviewModal } from './PatchPreviewModal'

// Rollback/Backup components
export { default as RollbackModal } from './RollbackModal'
export { default as BackupCard } from './BackupCard'
export { default as BackupHistoryPanel } from './BackupHistoryPanel'
export { default as RollbackIntegration } from './RollbackIntegration'

// Diff visualization types
export type { DiffViewerProps } from './DiffViewer'
export type { DiffStatsProps } from './DiffStats'
export type { DiffFileCardProps } from './DiffFileCard'
export type { DiffHunkProps } from './DiffHunk'
export type { PatchPreviewModalProps, PatchValidationWarning } from './PatchPreviewModal'

// Rollback/Backup types
export type { RollbackModalProps } from './RollbackModal'
export type { BackupCardProps } from './BackupCard'
export type { BackupHistoryPanelProps } from './BackupHistoryPanel'
export type { RollbackIntegrationProps } from './RollbackIntegration'
