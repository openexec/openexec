/**
 * PatchPreviewModal Component
 * Modal dialog for previewing and applying patches.
 * Displays the diff with option to approve or reject the patch application.
 * @module components/chat/diff/PatchPreviewModal
 */
import React, { useCallback, useMemo } from 'react'
import type { Patch, PatchStats } from '../../../types/diff'
import DiffViewer from './DiffViewer'
import DiffStats from './DiffStats'

export interface PatchValidationWarning {
  /** Warning type */
  type: 'line_count_mismatch' | 'missing_context' | 'file_not_found' | 'other'
  /** Human-readable message */
  message: string
  /** File path affected (if applicable) */
  filePath?: string
  /** Hunk index affected (if applicable) */
  hunkIndex?: number
}

export interface PatchPreviewModalProps {
  /** The patch to preview */
  patch: Patch
  /** Patch statistics (optional - calculated if not provided) */
  stats?: PatchStats
  /** Whether the modal is open */
  isOpen: boolean
  /** Callback when modal is closed */
  onClose: () => void
  /** Callback when patch is applied */
  onApply: () => void
  /** Callback when patch is rejected */
  onReject?: (reason?: string) => void
  /** Whether application is in progress */
  isLoading?: boolean
  /** Title for the modal (default: "Preview Changes") */
  title?: string
  /** Optional description */
  description?: string
  /** Validation warnings for the patch */
  warnings?: PatchValidationWarning[]
  /** Target file/directory for the patch */
  targetPath?: string
  /** Whether to show the raw patch toggle */
  showRawToggle?: boolean
}

const PatchPreviewModal: React.FC<PatchPreviewModalProps> = ({
  patch,
  stats: providedStats,
  isOpen,
  onClose,
  onApply,
  onReject,
  isLoading = false,
  title = 'Preview Changes',
  description,
  warnings = [],
  targetPath,
  showRawToggle = true,
}) => {
  // Calculate stats if not provided
  const stats = useMemo<PatchStats>(() => {
    if (providedStats) return providedStats

    let additions = 0
    let deletions = 0
    let hunks = 0

    for (const file of patch.files) {
      hunks += file.hunks.length
      for (const hunk of file.hunks) {
        for (const line of hunk.lines) {
          if (line.type === 'add') additions++
          if (line.type === 'remove') deletions++
        }
      }
    }

    return {
      filesChanged: patch.files.length,
      additions,
      deletions,
      hunks,
    }
  }, [patch, providedStats])

  // Check if there are any warnings
  const hasWarnings = warnings.length > 0
  const hasCriticalWarnings = warnings.some(
    (w) => w.type === 'file_not_found' || w.type === 'line_count_mismatch'
  )

  // Handle apply
  const handleApply = useCallback(() => {
    if (!isLoading) {
      onApply()
    }
  }, [isLoading, onApply])

  // Handle reject
  const handleReject = useCallback(() => {
    if (!isLoading && onReject) {
      onReject()
    }
  }, [isLoading, onReject])

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

  const canApply = !isLoading && patch.files.length > 0

  return (
    <div
      className="patch-preview-modal__backdrop"
      style={styles.backdrop}
      onClick={handleBackdropClick}
      onKeyDown={handleKeyDown}
      role="dialog"
      aria-modal="true"
      aria-labelledby="patch-preview-title"
      tabIndex={-1}
    >
      <div className="patch-preview-modal" style={styles.dialog}>
        {/* Header */}
        <div className="patch-preview-modal__header" style={styles.header}>
          <div style={styles.headerContent}>
            <DiffIcon />
            <div style={styles.headerText}>
              <h2 id="patch-preview-title" style={styles.title}>
                {title}
              </h2>
              {description && <p style={styles.description}>{description}</p>}
            </div>
          </div>
          <button
            className="patch-preview-modal__close"
            style={styles.closeButton}
            onClick={onClose}
            disabled={isLoading}
            aria-label="Close dialog"
          >
            <CloseIcon />
          </button>
        </div>

        {/* Info bar */}
        <div className="patch-preview-modal__info" style={styles.infoBar}>
          {targetPath && (
            <div style={styles.targetPath}>
              <FolderIcon />
              <code style={styles.pathCode}>{targetPath}</code>
            </div>
          )}
          <DiffStats stats={stats} compact />
        </div>

        {/* Warnings */}
        {hasWarnings && (
          <div className="patch-preview-modal__warnings" style={styles.warningsContainer}>
            {warnings.map((warning, index) => (
              <div
                key={index}
                style={{
                  ...styles.warning,
                  ...(warning.type === 'file_not_found' || warning.type === 'line_count_mismatch'
                    ? styles.warningCritical
                    : styles.warningInfo),
                }}
              >
                <WarningIcon />
                <span>{warning.message}</span>
              </div>
            ))}
          </div>
        )}

        {/* Body - Diff Viewer */}
        <div className="patch-preview-modal__body" style={styles.body}>
          {patch.files.length > 0 ? (
            <DiffViewer
              patch={patch}
              stats={stats}
              showStats={false}
              defaultExpanded={patch.files.length <= 3}
              showRawFallback={showRawToggle}
            />
          ) : (
            <div style={styles.emptyState}>
              <EmptyIcon />
              <p>No changes to preview</p>
            </div>
          )}
        </div>

        {/* Footer */}
        <div className="patch-preview-modal__footer" style={styles.footer}>
          {hasCriticalWarnings && (
            <div style={styles.footerWarning}>
              <WarningIcon />
              <span>This patch may not apply cleanly</span>
            </div>
          )}

          <div style={styles.footerActions}>
            <button
              className="patch-preview-modal__button patch-preview-modal__button--cancel"
              style={{
                ...styles.button,
                ...styles.secondaryButton,
              }}
              onClick={onClose}
              disabled={isLoading}
            >
              Cancel
            </button>

            {onReject && (
              <button
                className="patch-preview-modal__button patch-preview-modal__button--reject"
                style={{
                  ...styles.button,
                  ...styles.rejectButton,
                }}
                onClick={handleReject}
                disabled={isLoading}
              >
                {isLoading ? <LoadingSpinner /> : 'Reject'}
              </button>
            )}

            <button
              className="patch-preview-modal__button patch-preview-modal__button--apply"
              style={{
                ...styles.button,
                ...styles.applyButton,
                ...((!canApply) ? styles.buttonDisabled : {}),
              }}
              onClick={handleApply}
              disabled={!canApply}
            >
              {isLoading ? <LoadingSpinner /> : 'Apply Changes'}
            </button>
          </div>
        </div>
      </div>
    </div>
  )
}

// Icon components
const DiffIcon: React.FC = () => (
  <svg width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
    <path d="M12 3v18" />
    <path d="M3 12h18" />
    <path d="M8 8L3 12l5 4" />
    <path d="M16 8l5 4-5 4" />
  </svg>
)

const CloseIcon: React.FC = () => (
  <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
    <line x1="18" y1="6" x2="6" y2="18" />
    <line x1="6" y1="6" x2="18" y2="18" />
  </svg>
)

const WarningIcon: React.FC = () => (
  <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
    <path d="M10.29 3.86L1.82 18a2 2 0 001.71 3h16.94a2 2 0 001.71-3L13.71 3.86a2 2 0 00-3.42 0z" />
    <line x1="12" y1="9" x2="12" y2="13" />
    <line x1="12" y1="17" x2="12.01" y2="17" />
  </svg>
)

const LoadingSpinner: React.FC = () => (
  <svg
    width="16"
    height="16"
    viewBox="0 0 24 24"
    fill="none"
    stroke="currentColor"
    strokeWidth="2"
    style={{ animation: 'spin 1s linear infinite' }}
  >
    <path d="M12 2v4M12 18v4M4.93 4.93l2.83 2.83M16.24 16.24l2.83 2.83M2 12h4M18 12h4M4.93 19.07l2.83-2.83M16.24 7.76l2.83-2.83" />
  </svg>
)

const FolderIcon: React.FC = () => (
  <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
    <path d="M22 19a2 2 0 0 1-2 2H4a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h5l2 3h9a2 2 0 0 1 2 2z" />
  </svg>
)

const EmptyIcon: React.FC = () => (
  <svg width="48" height="48" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.5">
    <path d="M14.5 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V7.5L14.5 2z" />
    <polyline points="14 2 14 8 20 8" />
    <line x1="9" y1="15" x2="15" y2="15" />
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
    maxWidth: '900px',
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
    color: '#58a6ff',
  },
  headerText: {
    display: 'flex',
    flexDirection: 'column',
    gap: '4px',
  },
  title: {
    margin: 0,
    fontSize: '16px',
    fontWeight: 600,
    color: '#c9d1d9',
  },
  description: {
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
  infoBar: {
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'space-between',
    padding: '10px 20px',
    backgroundColor: '#161b22',
    borderBottom: '1px solid #30363d',
  },
  targetPath: {
    display: 'flex',
    alignItems: 'center',
    gap: '8px',
    color: '#8b949e',
  },
  pathCode: {
    fontSize: '12px',
    fontFamily: 'ui-monospace, SFMono-Regular, "SF Mono", Menlo, Consolas, monospace',
    color: '#58a6ff',
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
  },
  warningCritical: {
    backgroundColor: 'rgba(248, 81, 73, 0.1)',
    border: '1px solid rgba(248, 81, 73, 0.4)',
    color: '#f85149',
  },
  warningInfo: {
    backgroundColor: 'rgba(240, 136, 62, 0.1)',
    border: '1px solid rgba(240, 136, 62, 0.4)',
    color: '#f0883e',
  },
  body: {
    flex: 1,
    overflowY: 'auto',
    padding: '0',
    backgroundColor: '#0d1117',
  },
  emptyState: {
    display: 'flex',
    flexDirection: 'column',
    alignItems: 'center',
    justifyContent: 'center',
    padding: '48px 20px',
    color: '#8b949e',
    gap: '12px',
  },
  footer: {
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'space-between',
    padding: '16px 20px',
    borderTop: '1px solid #30363d',
    backgroundColor: '#0d1117',
  },
  footerWarning: {
    display: 'flex',
    alignItems: 'center',
    gap: '8px',
    fontSize: '12px',
    color: '#f0883e',
  },
  footerActions: {
    display: 'flex',
    gap: '12px',
    marginLeft: 'auto',
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
  rejectButton: {
    backgroundColor: '#da3633',
    color: '#ffffff',
  },
  applyButton: {
    backgroundColor: '#238636',
    color: '#ffffff',
  },
  buttonDisabled: {
    opacity: 0.5,
    cursor: 'not-allowed',
  },
}

export default PatchPreviewModal
