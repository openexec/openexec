/**
 * BackupCard Component
 * Card component for displaying a single backup entry with rollback action.
 * @module components/chat/diff/BackupCard
 */
import React, { useCallback } from 'react'
import type { BackupMetadata, BackupDisplayInfo } from '../../../types/backup'
import { toDisplayInfo } from '../../../types/backup'

export interface BackupCardProps {
  /** The backup to display */
  backup: BackupMetadata
  /** Whether this is the most recent backup for this file */
  isLatest?: boolean
  /** Callback when card is clicked to view details */
  onClick?: () => void
  /** Callback when restore/rollback is clicked */
  onRestore?: () => void
  /** Callback when delete is clicked */
  onDelete?: () => void
  /** Whether the card is in compact mode */
  compact?: boolean
  /** Whether actions are disabled (e.g., during restore) */
  disabled?: boolean
  /** Whether this backup is selected */
  selected?: boolean
}

const BackupCard: React.FC<BackupCardProps> = ({
  backup,
  isLatest = false,
  onClick,
  onRestore,
  onDelete,
  compact = false,
  disabled = false,
  selected = false,
}) => {
  const displayInfo: BackupDisplayInfo = toDisplayInfo(backup, isLatest)

  // Handle restore without triggering onClick
  const handleRestore = useCallback(
    (e: React.MouseEvent) => {
      e.stopPropagation()
      if (onRestore && !disabled) {
        onRestore()
      }
    },
    [onRestore, disabled]
  )

  // Handle delete without triggering onClick
  const handleDelete = useCallback(
    (e: React.MouseEvent) => {
      e.stopPropagation()
      if (onDelete && !disabled) {
        onDelete()
      }
    },
    [onDelete, disabled]
  )

  // Handle keyboard navigation
  const handleKeyDown = useCallback(
    (e: React.KeyboardEvent) => {
      if (onClick && (e.key === 'Enter' || e.key === ' ')) {
        e.preventDefault()
        onClick()
      }
    },
    [onClick]
  )

  return (
    <div
      className={`backup-card ${compact ? 'backup-card--compact' : ''} ${selected ? 'backup-card--selected' : ''} ${disabled ? 'backup-card--disabled' : ''}`}
      style={{
        ...styles.container,
        ...(selected ? styles.containerSelected : {}),
        ...(disabled ? styles.containerDisabled : {}),
        cursor: onClick ? 'pointer' : 'default',
      }}
      onClick={onClick}
      onKeyDown={onClick ? handleKeyDown : undefined}
      role={onClick ? 'button' : undefined}
      tabIndex={onClick ? 0 : undefined}
    >
      {/* Header row */}
      <div className="backup-card__header" style={styles.header}>
        {/* File icon and name */}
        <div className="backup-card__file" style={styles.fileInfo}>
          <FileIcon />
          <span style={styles.fileName}>{displayInfo.fileName}</span>
          {isLatest && (
            <span className="backup-card__badge backup-card__badge--latest" style={styles.latestBadge}>
              Latest
            </span>
          )}
          {backup.restored && (
            <span className="backup-card__badge backup-card__badge--restored" style={styles.restoredBadge}>
              Restored
            </span>
          )}
        </div>

        {/* Time ago */}
        <span className="backup-card__time" style={styles.timeAgo}>
          {displayInfo.timeAgo}
        </span>
      </div>

      {/* Details row (non-compact only) */}
      {!compact && (
        <div className="backup-card__details" style={styles.details}>
          {/* Path */}
          <div className="backup-card__path" style={styles.pathRow}>
            <FolderIcon />
            <code style={styles.pathCode}>{displayInfo.directory}</code>
          </div>

          {/* Meta info */}
          <div className="backup-card__meta" style={styles.meta}>
            <span className="backup-card__size" style={styles.metaItem}>
              <SizeIcon />
              {displayInfo.formattedSize}
            </span>
            <span className="backup-card__checksum" style={styles.metaItem}>
              <ChecksumIcon />
              <code style={styles.checksumCode}>{backup.checksum.slice(0, 8)}</code>
            </span>
            {backup.sessionId && (
              <span className="backup-card__session" style={styles.metaItem}>
                <SessionIcon />
                {backup.sessionId.slice(0, 8)}
              </span>
            )}
          </div>
        </div>
      )}

      {/* Compact meta (compact mode only) */}
      {compact && (
        <div className="backup-card__compact-meta" style={styles.compactMeta}>
          <span style={styles.metaItem}>
            <SizeIcon />
            {displayInfo.formattedSize}
          </span>
          <code style={styles.pathCodeCompact}>{displayInfo.directory}</code>
        </div>
      )}

      {/* Actions */}
      {(onRestore || onDelete) && (
        <div className="backup-card__actions" style={styles.actions}>
          {onDelete && (
            <button
              className="backup-card__action backup-card__action--delete"
              style={{
                ...styles.actionButton,
                ...styles.deleteAction,
                ...(disabled ? styles.actionDisabled : {}),
              }}
              onClick={handleDelete}
              disabled={disabled}
              title="Delete backup"
              aria-label="Delete backup"
            >
              <DeleteIcon />
              {!compact && <span>Delete</span>}
            </button>
          )}
          {onRestore && (
            <button
              className="backup-card__action backup-card__action--restore"
              style={{
                ...styles.actionButton,
                ...styles.restoreAction,
                ...(disabled ? styles.actionDisabled : {}),
              }}
              onClick={handleRestore}
              disabled={disabled}
              title="Restore this backup"
              aria-label="Restore this backup"
            >
              <RollbackIcon />
              {!compact && <span>Restore</span>}
            </button>
          )}
        </div>
      )}
    </div>
  )
}

// Icon components
const FileIcon: React.FC = () => (
  <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
    <path d="M14.5 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V7.5L14.5 2z" />
    <polyline points="14 2 14 8 20 8" />
  </svg>
)

const FolderIcon: React.FC = () => (
  <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
    <path d="M22 19a2 2 0 0 1-2 2H4a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h5l2 3h9a2 2 0 0 1 2 2z" />
  </svg>
)

const SizeIcon: React.FC = () => (
  <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
    <path d="M21 16V8a2 2 0 0 0-1-1.73l-7-4a2 2 0 0 0-2 0l-7 4A2 2 0 0 0 3 8v8a2 2 0 0 0 1 1.73l7 4a2 2 0 0 0 2 0l7-4A2 2 0 0 0 21 16z" />
  </svg>
)

const ChecksumIcon: React.FC = () => (
  <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
    <path d="M12 22s8-4 8-10V5l-8-3-8 3v7c0 6 8 10 8 10z" />
  </svg>
)

const SessionIcon: React.FC = () => (
  <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
    <rect x="3" y="3" width="18" height="18" rx="2" ry="2" />
    <line x1="9" y1="3" x2="9" y2="21" />
  </svg>
)

const RollbackIcon: React.FC = () => (
  <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
    <path d="M3 12a9 9 0 1 0 9-9 9.75 9.75 0 0 0-6.74 2.74L3 8" />
    <path d="M3 3v5h5" />
  </svg>
)

const DeleteIcon: React.FC = () => (
  <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
    <polyline points="3 6 5 6 21 6" />
    <path d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6m3 0V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2" />
  </svg>
)

// Styles
const styles: Record<string, React.CSSProperties> = {
  container: {
    backgroundColor: '#161b22',
    borderRadius: '8px',
    border: '1px solid #30363d',
    padding: '12px 16px',
    display: 'flex',
    flexDirection: 'column',
    gap: '10px',
    transition: 'background-color 0.2s, border-color 0.2s',
  },
  containerSelected: {
    borderColor: '#58a6ff',
    backgroundColor: '#1c2128',
  },
  containerDisabled: {
    opacity: 0.6,
    pointerEvents: 'none',
  },
  header: {
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'space-between',
  },
  fileInfo: {
    display: 'flex',
    alignItems: 'center',
    gap: '8px',
    color: '#58a6ff',
  },
  fileName: {
    fontSize: '14px',
    fontWeight: 500,
    color: '#c9d1d9',
  },
  latestBadge: {
    fontSize: '10px',
    fontWeight: 500,
    textTransform: 'uppercase',
    padding: '2px 6px',
    borderRadius: '4px',
    backgroundColor: 'rgba(35, 134, 54, 0.15)',
    color: '#7ee787',
    border: '1px solid rgba(35, 134, 54, 0.4)',
  },
  restoredBadge: {
    fontSize: '10px',
    fontWeight: 500,
    textTransform: 'uppercase',
    padding: '2px 6px',
    borderRadius: '4px',
    backgroundColor: 'rgba(163, 113, 247, 0.15)',
    color: '#a371f7',
    border: '1px solid rgba(163, 113, 247, 0.4)',
  },
  timeAgo: {
    fontSize: '12px',
    color: '#8b949e',
  },
  details: {
    display: 'flex',
    flexDirection: 'column',
    gap: '8px',
  },
  pathRow: {
    display: 'flex',
    alignItems: 'center',
    gap: '6px',
    color: '#8b949e',
  },
  pathCode: {
    fontSize: '12px',
    fontFamily: 'ui-monospace, SFMono-Regular, "SF Mono", Menlo, Consolas, monospace',
    color: '#8b949e',
    overflow: 'hidden',
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
  },
  pathCodeCompact: {
    fontSize: '11px',
    fontFamily: 'ui-monospace, SFMono-Regular, "SF Mono", Menlo, Consolas, monospace',
    color: '#6e7681',
    overflow: 'hidden',
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
    flex: 1,
  },
  meta: {
    display: 'flex',
    flexWrap: 'wrap',
    gap: '12px',
    fontSize: '11px',
    color: '#8b949e',
  },
  compactMeta: {
    display: 'flex',
    alignItems: 'center',
    gap: '12px',
    fontSize: '11px',
    color: '#8b949e',
  },
  metaItem: {
    display: 'flex',
    alignItems: 'center',
    gap: '4px',
  },
  checksumCode: {
    fontFamily: 'ui-monospace, SFMono-Regular, "SF Mono", Menlo, Consolas, monospace',
    fontSize: '11px',
  },
  actions: {
    display: 'flex',
    gap: '8px',
    marginTop: '4px',
    paddingTop: '10px',
    borderTop: '1px solid #21262d',
  },
  actionButton: {
    display: 'flex',
    alignItems: 'center',
    gap: '6px',
    padding: '6px 12px',
    fontSize: '12px',
    fontWeight: 500,
    borderRadius: '6px',
    border: 'none',
    cursor: 'pointer',
    transition: 'background-color 0.2s, opacity 0.2s',
  },
  restoreAction: {
    backgroundColor: '#f0883e',
    color: '#ffffff',
  },
  deleteAction: {
    backgroundColor: '#21262d',
    color: '#f85149',
    border: '1px solid #30363d',
  },
  actionDisabled: {
    opacity: 0.5,
    cursor: 'not-allowed',
  },
}

export default BackupCard
