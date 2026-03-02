/**
 * RollbackModal Component
 * Modal dialog for previewing and confirming a file rollback/restore operation.
 * Displays backup information, optional diff, and provides confirm/cancel actions.
 * @module components/chat/diff/RollbackModal
 */
import React, { useCallback, useMemo } from 'react'
import type { BackupMetadata, BackupDisplayInfo, RollbackStatus } from '../../../types/backup'
import { toDisplayInfo, formatFileSize, ROLLBACK_STATUS_INFO } from '../../../types/backup'
import type { Patch, PatchStats } from '../../../types/diff'
import DiffViewer from './DiffViewer'

export interface RollbackModalProps {
  /** The backup to restore */
  backup: BackupMetadata
  /** Whether the modal is open */
  isOpen: boolean
  /** Callback when modal is closed */
  onClose: () => void
  /** Callback when rollback is confirmed */
  onConfirm: () => void
  /** Callback when rollback is cancelled (optional extra action beyond close) */
  onCancel?: () => void
  /** Current status of the rollback operation */
  status?: RollbackStatus
  /** Optional diff showing changes that will be reverted */
  diff?: Patch
  /** Stats for the diff */
  diffStats?: PatchStats
  /** Current file exists */
  currentFileExists?: boolean
  /** Current file size (for comparison) */
  currentFileSize?: number
  /** Title for the modal */
  title?: string
  /** Warning messages to display */
  warnings?: string[]
  /** Error message if rollback failed */
  error?: string
}

const RollbackModal: React.FC<RollbackModalProps> = ({
  backup,
  isOpen,
  onClose,
  onConfirm,
  onCancel,
  status = 'confirming',
  diff,
  diffStats,
  currentFileExists = true,
  currentFileSize,
  title = 'Rollback File',
  warnings = [],
  error,
}) => {
  // Get display info for the backup
  const displayInfo: BackupDisplayInfo = useMemo(
    () => toDisplayInfo(backup, true),
    [backup]
  )

  // Check loading/processing states
  const isLoading = status === 'loading' || status === 'restoring'
  const isComplete = status === 'success'
  const hasError = status === 'error' || !!error

  // Handle confirm
  const handleConfirm = useCallback(() => {
    if (!isLoading && !isComplete) {
      onConfirm()
    }
  }, [isLoading, isComplete, onConfirm])

  // Handle cancel
  const handleCancel = useCallback(() => {
    if (!isLoading) {
      if (onCancel) {
        onCancel()
      }
      onClose()
    }
  }, [isLoading, onCancel, onClose])

  // Handle backdrop click
  const handleBackdropClick = useCallback(
    (e: React.MouseEvent) => {
      if (e.target === e.currentTarget && !isLoading) {
        onClose()
      }
    },
    [isLoading, onClose]
  )

  // Handle escape key
  const handleKeyDown = useCallback(
    (e: React.KeyboardEvent) => {
      if (e.key === 'Escape' && !isLoading) {
        onClose()
      }
    },
    [isLoading, onClose]
  )

  if (!isOpen) return null

  const statusInfo = ROLLBACK_STATUS_INFO[status]
  const sizeDiff = currentFileSize !== undefined ? currentFileSize - backup.size : null

  return (
    <div
      className="rollback-modal__backdrop"
      style={styles.backdrop}
      onClick={handleBackdropClick}
      onKeyDown={handleKeyDown}
      role="dialog"
      aria-modal="true"
      aria-labelledby="rollback-modal-title"
      tabIndex={-1}
    >
      <div className="rollback-modal" style={styles.dialog}>
        {/* Header */}
        <div className="rollback-modal__header" style={styles.header}>
          <div style={styles.headerContent}>
            <RollbackIcon />
            <div style={styles.headerText}>
              <h2 id="rollback-modal-title" style={styles.title}>
                {title}
              </h2>
              <p style={styles.subtitle}>
                Restore file to previous version
              </p>
            </div>
          </div>
          <button
            className="rollback-modal__close"
            style={styles.closeButton}
            onClick={onClose}
            disabled={isLoading}
            aria-label="Close dialog"
          >
            <CloseIcon />
          </button>
        </div>

        {/* Backup Info */}
        <div className="rollback-modal__info" style={styles.infoSection}>
          {/* File path */}
          <div className="rollback-modal__file" style={styles.fileInfo}>
            <FileIcon />
            <div style={styles.fileDetails}>
              <span style={styles.fileName}>{displayInfo.fileName}</span>
              <span style={styles.filePath}>{displayInfo.directory}</span>
            </div>
          </div>

          {/* Backup details */}
          <div className="rollback-modal__details" style={styles.detailsGrid}>
            <div style={styles.detailItem}>
              <span style={styles.detailLabel}>Backup Created</span>
              <span style={styles.detailValue}>
                <ClockIcon />
                {displayInfo.timeAgo}
              </span>
            </div>
            <div style={styles.detailItem}>
              <span style={styles.detailLabel}>Backup Size</span>
              <span style={styles.detailValue}>
                <SizeIcon />
                {displayInfo.formattedSize}
              </span>
            </div>
            {currentFileExists && currentFileSize !== undefined && (
              <div style={styles.detailItem}>
                <span style={styles.detailLabel}>Current Size</span>
                <span style={styles.detailValue}>
                  <SizeIcon />
                  {formatFileSize(currentFileSize)}
                  {sizeDiff !== null && sizeDiff !== 0 && (
                    <span
                      style={{
                        ...styles.sizeDiff,
                        color: sizeDiff > 0 ? '#f85149' : '#7ee787',
                      }}
                    >
                      ({sizeDiff > 0 ? '+' : ''}{formatFileSize(Math.abs(sizeDiff))})
                    </span>
                  )}
                </span>
              </div>
            )}
            {backup.sessionId && (
              <div style={styles.detailItem}>
                <span style={styles.detailLabel}>Session</span>
                <span style={styles.detailValue}>
                  <SessionIcon />
                  {backup.sessionId.slice(0, 8)}...
                </span>
              </div>
            )}
          </div>

          {/* Checksum */}
          <div style={styles.checksumRow}>
            <ChecksumIcon />
            <code style={styles.checksum}>{backup.checksum.slice(0, 16)}...</code>
            {backup.restored && (
              <span style={styles.restoredBadge}>
                Previously Restored
              </span>
            )}
          </div>
        </div>

        {/* Warnings */}
        {warnings.length > 0 && (
          <div className="rollback-modal__warnings" style={styles.warningsContainer}>
            {warnings.map((warning, index) => (
              <div key={index} style={styles.warning}>
                <WarningIcon />
                <span>{warning}</span>
              </div>
            ))}
          </div>
        )}

        {/* Current file doesn't exist warning */}
        {!currentFileExists && (
          <div className="rollback-modal__warnings" style={styles.warningsContainer}>
            <div style={{ ...styles.warning, ...styles.warningInfo }}>
              <InfoIcon />
              <span>The original file no longer exists. Restoring will recreate it.</span>
            </div>
          </div>
        )}

        {/* Error display */}
        {hasError && error && (
          <div className="rollback-modal__error" style={styles.errorContainer}>
            <ErrorIcon />
            <span>{error}</span>
          </div>
        )}

        {/* Success display */}
        {isComplete && (
          <div className="rollback-modal__success" style={styles.successContainer}>
            <SuccessIcon />
            <span>File successfully restored to backup version!</span>
          </div>
        )}

        {/* Diff preview */}
        {diff && diff.files.length > 0 && (
          <div className="rollback-modal__diff" style={styles.diffSection}>
            <div style={styles.diffHeader}>
              <DiffIcon />
              <span style={styles.diffTitle}>Changes to be reverted</span>
            </div>
            <div style={styles.diffBody}>
              <DiffViewer
                patch={diff}
                stats={diffStats}
                showStats={true}
                defaultExpanded={diff.files.length <= 2}
                showRawFallback={true}
              />
            </div>
          </div>
        )}

        {/* Footer */}
        <div className="rollback-modal__footer" style={styles.footer}>
          {/* Status indicator */}
          <div style={styles.statusIndicator}>
            {isLoading && <LoadingSpinner />}
            <span style={{ color: statusInfo.color }}>{statusInfo.label}</span>
          </div>

          {/* Actions */}
          <div style={styles.footerActions}>
            <button
              className="rollback-modal__button rollback-modal__button--cancel"
              style={{
                ...styles.button,
                ...styles.secondaryButton,
              }}
              onClick={handleCancel}
              disabled={isLoading}
            >
              {isComplete ? 'Close' : 'Cancel'}
            </button>

            {!isComplete && (
              <button
                className="rollback-modal__button rollback-modal__button--confirm"
                style={{
                  ...styles.button,
                  ...styles.confirmButton,
                  ...(isLoading ? styles.buttonDisabled : {}),
                }}
                onClick={handleConfirm}
                disabled={isLoading || hasError}
              >
                {isLoading ? (
                  <>
                    <LoadingSpinner />
                    Restoring...
                  </>
                ) : (
                  <>
                    <RollbackSmallIcon />
                    Restore Backup
                  </>
                )}
              </button>
            )}
          </div>
        </div>
      </div>
    </div>
  )
}

// Icon components
const RollbackIcon: React.FC = () => (
  <svg width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
    <path d="M3 12a9 9 0 1 0 9-9 9.75 9.75 0 0 0-6.74 2.74L3 8" />
    <path d="M3 3v5h5" />
  </svg>
)

const RollbackSmallIcon: React.FC = () => (
  <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
    <path d="M3 12a9 9 0 1 0 9-9 9.75 9.75 0 0 0-6.74 2.74L3 8" />
    <path d="M3 3v5h5" />
  </svg>
)

const CloseIcon: React.FC = () => (
  <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
    <line x1="18" y1="6" x2="6" y2="18" />
    <line x1="6" y1="6" x2="18" y2="18" />
  </svg>
)

const FileIcon: React.FC = () => (
  <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
    <path d="M14.5 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V7.5L14.5 2z" />
    <polyline points="14 2 14 8 20 8" />
  </svg>
)

const ClockIcon: React.FC = () => (
  <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
    <circle cx="12" cy="12" r="10" />
    <polyline points="12 6 12 12 16 14" />
  </svg>
)

const SizeIcon: React.FC = () => (
  <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
    <path d="M21 16V8a2 2 0 0 0-1-1.73l-7-4a2 2 0 0 0-2 0l-7 4A2 2 0 0 0 3 8v8a2 2 0 0 0 1 1.73l7 4a2 2 0 0 0 2 0l7-4A2 2 0 0 0 21 16z" />
  </svg>
)

const SessionIcon: React.FC = () => (
  <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
    <rect x="3" y="3" width="18" height="18" rx="2" ry="2" />
    <line x1="9" y1="3" x2="9" y2="21" />
  </svg>
)

const ChecksumIcon: React.FC = () => (
  <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
    <path d="M12 22s8-4 8-10V5l-8-3-8 3v7c0 6 8 10 8 10z" />
  </svg>
)

const WarningIcon: React.FC = () => (
  <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
    <path d="M10.29 3.86L1.82 18a2 2 0 001.71 3h16.94a2 2 0 001.71-3L13.71 3.86a2 2 0 00-3.42 0z" />
    <line x1="12" y1="9" x2="12" y2="13" />
    <line x1="12" y1="17" x2="12.01" y2="17" />
  </svg>
)

const InfoIcon: React.FC = () => (
  <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
    <circle cx="12" cy="12" r="10" />
    <line x1="12" y1="16" x2="12" y2="12" />
    <line x1="12" y1="8" x2="12.01" y2="8" />
  </svg>
)

const ErrorIcon: React.FC = () => (
  <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
    <circle cx="12" cy="12" r="10" />
    <line x1="15" y1="9" x2="9" y2="15" />
    <line x1="9" y1="9" x2="15" y2="15" />
  </svg>
)

const SuccessIcon: React.FC = () => (
  <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
    <path d="M22 11.08V12a10 10 0 1 1-5.93-9.14" />
    <polyline points="22 4 12 14.01 9 11.01" />
  </svg>
)

const DiffIcon: React.FC = () => (
  <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
    <path d="M12 3v18" />
    <path d="M3 12h18" />
    <path d="M8 8L3 12l5 4" />
    <path d="M16 8l5 4-5 4" />
  </svg>
)

const LoadingSpinner: React.FC = () => (
  <svg
    width="14"
    height="14"
    viewBox="0 0 24 24"
    fill="none"
    stroke="currentColor"
    strokeWidth="2"
    style={{ animation: 'spin 1s linear infinite' }}
  >
    <path d="M12 2v4M12 18v4M4.93 4.93l2.83 2.83M16.24 16.24l2.83 2.83M2 12h4M18 12h4M4.93 19.07l2.83-2.83M16.24 7.76l2.83-2.83" />
  </svg>
)

// Styles
const styles: Record<string, React.CSSProperties> = {
  backdrop: {
    position: 'fixed',
    top: 0,
    left: 0,
    right: 0,
    bottom: 0,
    backgroundColor: 'rgba(0, 0, 0, 0.6)',
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'center',
    zIndex: 1000,
    padding: '20px',
  },
  dialog: {
    backgroundColor: '#161b22',
    borderRadius: '12px',
    border: '1px solid #30363d',
    width: '100%',
    maxWidth: '700px',
    maxHeight: '90vh',
    display: 'flex',
    flexDirection: 'column',
    overflow: 'hidden',
    boxShadow: '0 8px 32px rgba(0, 0, 0, 0.4)',
  },
  header: {
    display: 'flex',
    alignItems: 'flex-start',
    justifyContent: 'space-between',
    padding: '16px 20px',
    borderBottom: '1px solid #30363d',
    backgroundColor: '#0d1117',
  },
  headerContent: {
    display: 'flex',
    alignItems: 'flex-start',
    gap: '12px',
    color: '#f0883e',
  },
  headerText: {
    display: 'flex',
    flexDirection: 'column',
    gap: '2px',
  },
  title: {
    margin: 0,
    fontSize: '16px',
    fontWeight: 600,
    color: '#c9d1d9',
  },
  subtitle: {
    margin: 0,
    fontSize: '13px',
    color: '#8b949e',
  },
  closeButton: {
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'center',
    width: '32px',
    height: '32px',
    border: 'none',
    borderRadius: '6px',
    backgroundColor: 'transparent',
    color: '#8b949e',
    cursor: 'pointer',
    transition: 'background-color 0.2s',
    flexShrink: 0,
  },
  infoSection: {
    padding: '16px 20px',
    backgroundColor: '#161b22',
    borderBottom: '1px solid #30363d',
    display: 'flex',
    flexDirection: 'column',
    gap: '12px',
  },
  fileInfo: {
    display: 'flex',
    alignItems: 'center',
    gap: '12px',
    color: '#58a6ff',
  },
  fileDetails: {
    display: 'flex',
    flexDirection: 'column',
    gap: '2px',
  },
  fileName: {
    fontSize: '14px',
    fontWeight: 500,
    color: '#c9d1d9',
  },
  filePath: {
    fontSize: '12px',
    color: '#8b949e',
    fontFamily: 'ui-monospace, SFMono-Regular, "SF Mono", Menlo, Consolas, monospace',
  },
  detailsGrid: {
    display: 'grid',
    gridTemplateColumns: 'repeat(auto-fit, minmax(150px, 1fr))',
    gap: '12px',
  },
  detailItem: {
    display: 'flex',
    flexDirection: 'column',
    gap: '4px',
  },
  detailLabel: {
    fontSize: '11px',
    color: '#8b949e',
    textTransform: 'uppercase',
    letterSpacing: '0.5px',
  },
  detailValue: {
    display: 'flex',
    alignItems: 'center',
    gap: '6px',
    fontSize: '13px',
    color: '#c9d1d9',
  },
  sizeDiff: {
    fontSize: '11px',
    marginLeft: '4px',
  },
  checksumRow: {
    display: 'flex',
    alignItems: 'center',
    gap: '8px',
    color: '#8b949e',
  },
  checksum: {
    fontSize: '11px',
    fontFamily: 'ui-monospace, SFMono-Regular, "SF Mono", Menlo, Consolas, monospace',
    color: '#58a6ff',
  },
  restoredBadge: {
    fontSize: '10px',
    padding: '2px 6px',
    borderRadius: '4px',
    backgroundColor: 'rgba(163, 113, 247, 0.15)',
    color: '#a371f7',
    border: '1px solid rgba(163, 113, 247, 0.4)',
    marginLeft: 'auto',
  },
  warningsContainer: {
    display: 'flex',
    flexDirection: 'column',
    gap: '8px',
    padding: '12px 20px',
    backgroundColor: '#161b22',
    borderBottom: '1px solid #30363d',
  },
  warning: {
    display: 'flex',
    alignItems: 'flex-start',
    gap: '8px',
    padding: '8px 12px',
    borderRadius: '6px',
    fontSize: '12px',
    lineHeight: 1.4,
    backgroundColor: 'rgba(240, 136, 62, 0.1)',
    border: '1px solid rgba(240, 136, 62, 0.4)',
    color: '#f0883e',
  },
  warningInfo: {
    backgroundColor: 'rgba(88, 166, 255, 0.1)',
    border: '1px solid rgba(88, 166, 255, 0.4)',
    color: '#58a6ff',
  },
  errorContainer: {
    display: 'flex',
    alignItems: 'flex-start',
    gap: '8px',
    padding: '12px 20px',
    backgroundColor: 'rgba(248, 81, 73, 0.1)',
    borderBottom: '1px solid rgba(248, 81, 73, 0.4)',
    color: '#f85149',
    fontSize: '13px',
  },
  successContainer: {
    display: 'flex',
    alignItems: 'center',
    gap: '8px',
    padding: '12px 20px',
    backgroundColor: 'rgba(35, 134, 54, 0.1)',
    borderBottom: '1px solid rgba(35, 134, 54, 0.4)',
    color: '#7ee787',
    fontSize: '13px',
  },
  diffSection: {
    flex: 1,
    display: 'flex',
    flexDirection: 'column',
    overflow: 'hidden',
    minHeight: 0,
  },
  diffHeader: {
    display: 'flex',
    alignItems: 'center',
    gap: '8px',
    padding: '12px 20px',
    backgroundColor: '#0d1117',
    borderBottom: '1px solid #30363d',
    color: '#8b949e',
  },
  diffTitle: {
    fontSize: '13px',
    fontWeight: 500,
  },
  diffBody: {
    flex: 1,
    overflowY: 'auto',
    backgroundColor: '#0d1117',
  },
  footer: {
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'space-between',
    padding: '16px 20px',
    borderTop: '1px solid #30363d',
    backgroundColor: '#0d1117',
  },
  statusIndicator: {
    display: 'flex',
    alignItems: 'center',
    gap: '8px',
    fontSize: '12px',
  },
  footerActions: {
    display: 'flex',
    gap: '12px',
  },
  button: {
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'center',
    gap: '6px',
    padding: '8px 16px',
    fontSize: '14px',
    fontWeight: 500,
    borderRadius: '6px',
    border: 'none',
    cursor: 'pointer',
    transition: 'background-color 0.2s, opacity 0.2s',
  },
  secondaryButton: {
    backgroundColor: '#21262d',
    color: '#c9d1d9',
    border: '1px solid #30363d',
  },
  confirmButton: {
    backgroundColor: '#f0883e',
    color: '#ffffff',
  },
  buttonDisabled: {
    opacity: 0.5,
    cursor: 'not-allowed',
  },
}

export default RollbackModal
