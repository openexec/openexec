/**
 * RestartApprovalDialog Component
 * Modal dialog for reviewing and approving/rejecting restart requests.
 * @module components/chat/restart/RestartApprovalDialog
 */
import React, { useState, useCallback } from 'react'
import type {
  RestartRequest,
  RestartReason,
  RESTART_REASON_INFO,
  RESTART_STATUS_INFO,
} from '../../../types/restart'

export interface RestartApprovalDialogProps {
  /** The restart request to display */
  request: RestartRequest
  /** Whether the dialog is open */
  isOpen: boolean
  /** Callback when dialog is closed */
  onClose: () => void
  /** Callback when request is approved */
  onApprove: (requestId: string, reason?: string) => void
  /** Callback when request is rejected */
  onReject: (requestId: string, reason?: string) => void
  /** Callback when request is cancelled */
  onCancel?: (requestId: string, reason?: string) => void
  /** Whether approval is in progress */
  isLoading?: boolean
}

const RestartApprovalDialog: React.FC<RestartApprovalDialogProps> = ({
  request,
  isOpen,
  onClose,
  onApprove,
  onReject,
  onCancel,
  isLoading = false,
}) => {
  const [rejectReason, setRejectReason] = useState('')
  const [showRejectInput, setShowRejectInput] = useState(false)

  // Get reason info for display
  const getReasonInfo = (reason: RestartReason) => {
    const reasonInfo: Record<RestartReason, { label: string; description: string; color: string }> = {
      code_change: {
        label: 'Code Change',
        description: 'Code modifications require restart to take effect',
        color: '#58a6ff',
      },
      config_change: {
        label: 'Configuration Change',
        description: 'Configuration changes require restart to apply',
        color: '#a371f7',
      },
      user_requested: {
        label: 'User Requested',
        description: 'Manual restart requested by user',
        color: '#7ee787',
      },
      recovery: {
        label: 'Recovery',
        description: 'Recovery from error state requires restart',
        color: '#f0883e',
      },
      upgrade: {
        label: 'Upgrade',
        description: 'Upgrade or update operation requires restart',
        color: '#79c0ff',
      },
    }
    return reasonInfo[reason]
  }

  // Get status color
  const getStatusColor = (status: string): string => {
    const colors: Record<string, string> = {
      pending: '#f0883e',
      approved: '#238636',
      rejected: '#da3633',
      in_progress: '#58a6ff',
      complete: '#238636',
      failed: '#da3633',
      cancelled: '#8b949e',
    }
    return colors[status] || '#8b949e'
  }

  // Format timestamp for display
  const formatTimestamp = (timestamp: string): string => {
    const date = new Date(timestamp)
    return date.toLocaleString()
  }

  // Calculate time ago
  const getTimeAgo = (timestamp: string): string => {
    const now = new Date()
    const then = new Date(timestamp)
    const seconds = Math.floor((now.getTime() - then.getTime()) / 1000)

    if (seconds < 60) return 'just now'
    if (seconds < 3600) return `${Math.floor(seconds / 60)}m ago`
    if (seconds < 86400) return `${Math.floor(seconds / 3600)}h ago`
    return `${Math.floor(seconds / 86400)}d ago`
  }

  // Handle approve
  const handleApprove = useCallback(() => {
    onApprove(request.id)
  }, [request.id, onApprove])

  // Handle reject
  const handleReject = useCallback(() => {
    if (showRejectInput) {
      onReject(request.id, rejectReason || undefined)
      setShowRejectInput(false)
      setRejectReason('')
    } else {
      setShowRejectInput(true)
    }
  }, [request.id, onReject, showRejectInput, rejectReason])

  // Handle cancel
  const handleCancel = useCallback(() => {
    if (onCancel) {
      onCancel(request.id)
    }
  }, [request.id, onCancel])

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

  const reasonInfo = getReasonInfo(request.reason)
  const isPending = request.status === 'pending'
  const canApprove = isPending && !isLoading
  const canReject = isPending && !isLoading

  return (
    <div
      className="restart-approval-dialog__backdrop"
      style={styles.backdrop}
      onClick={handleBackdropClick}
      onKeyDown={handleKeyDown}
      role="dialog"
      aria-modal="true"
      aria-labelledby="restart-dialog-title"
      tabIndex={-1}
    >
      <div className="restart-approval-dialog" style={styles.dialog}>
        {/* Header */}
        <div className="restart-approval-dialog__header" style={styles.header}>
          <div style={styles.headerContent}>
            <RestartIcon />
            <h2 id="restart-dialog-title" style={styles.title}>
              Restart Approval Required
            </h2>
          </div>
          <button
            className="restart-approval-dialog__close"
            style={styles.closeButton}
            onClick={onClose}
            disabled={isLoading}
            aria-label="Close dialog"
          >
            <CloseIcon />
          </button>
        </div>

        {/* Body */}
        <div className="restart-approval-dialog__body" style={styles.body}>
          {/* Warning banner */}
          <div className="restart-approval-dialog__warning" style={styles.warningBanner}>
            <WarningIcon />
            <span>
              The orchestrator is requesting permission to restart itself. This will temporarily
              interrupt all active sessions.
            </span>
          </div>

          {/* Request details */}
          <div className="restart-approval-dialog__details" style={styles.details}>
            {/* Reason */}
            <div className="restart-approval-dialog__field" style={styles.field}>
              <label style={styles.label}>Reason</label>
              <div style={styles.reasonBadge}>
                <span
                  style={{
                    ...styles.reasonDot,
                    backgroundColor: reasonInfo.color,
                  }}
                />
                <span style={styles.reasonLabel}>{reasonInfo.label}</span>
              </div>
              <p style={styles.reasonDescription}>{reasonInfo.description}</p>
            </div>

            {/* Description */}
            {request.description && (
              <div className="restart-approval-dialog__field" style={styles.field}>
                <label style={styles.label}>Description</label>
                <p style={styles.description}>{request.description}</p>
              </div>
            )}

            {/* Requested by */}
            <div className="restart-approval-dialog__field" style={styles.field}>
              <label style={styles.label}>Requested By</label>
              <span style={styles.value}>{request.requestedBy}</span>
            </div>

            {/* Session ID */}
            {request.sessionId && (
              <div className="restart-approval-dialog__field" style={styles.field}>
                <label style={styles.label}>Session</label>
                <code style={styles.code}>{request.sessionId}</code>
              </div>
            )}

            {/* Build required */}
            <div className="restart-approval-dialog__field" style={styles.field}>
              <label style={styles.label}>Build Required</label>
              <span style={styles.value}>
                {request.buildRequired ? (
                  <span style={{ color: '#f0883e' }}>Yes - will rebuild before restart</span>
                ) : (
                  <span style={{ color: '#8b949e' }}>No</span>
                )}
              </span>
            </div>

            {/* Session resume */}
            <div className="restart-approval-dialog__field" style={styles.field}>
              <label style={styles.label}>Session Resume</label>
              <span style={styles.value}>
                {request.resumeEnabled ? (
                  <span style={{ color: '#238636' }}>
                    Enabled{request.resumeOnStartup ? ' (auto-resume on startup)' : ''}
                  </span>
                ) : (
                  <span style={{ color: '#8b949e' }}>Disabled</span>
                )}
              </span>
            </div>

            {/* Timestamps */}
            <div className="restart-approval-dialog__field" style={styles.field}>
              <label style={styles.label}>Requested</label>
              <span style={styles.value}>
                {formatTimestamp(request.createdAt)}{' '}
                <span style={styles.timeAgo}>({getTimeAgo(request.createdAt)})</span>
              </span>
            </div>

            {/* Status */}
            <div className="restart-approval-dialog__field" style={styles.field}>
              <label style={styles.label}>Status</label>
              <span
                style={{
                  ...styles.statusBadge,
                  backgroundColor: `${getStatusColor(request.status)}20`,
                  color: getStatusColor(request.status),
                  borderColor: getStatusColor(request.status),
                }}
              >
                {request.status.replace('_', ' ')}
              </span>
            </div>
          </div>

          {/* Reject reason input */}
          {showRejectInput && (
            <div className="restart-approval-dialog__reject-input" style={styles.rejectInput}>
              <label style={styles.label}>Rejection Reason (optional)</label>
              <textarea
                style={styles.textarea}
                value={rejectReason}
                onChange={(e) => setRejectReason(e.target.value)}
                placeholder="Provide a reason for rejection..."
                rows={3}
              />
            </div>
          )}
        </div>

        {/* Footer */}
        <div className="restart-approval-dialog__footer" style={styles.footer}>
          {onCancel && isPending && (
            <button
              className="restart-approval-dialog__button restart-approval-dialog__button--cancel"
              style={{
                ...styles.button,
                ...styles.cancelButton,
              }}
              onClick={handleCancel}
              disabled={isLoading}
            >
              Cancel Request
            </button>
          )}

          <div style={styles.footerActions}>
            {showRejectInput && (
              <button
                className="restart-approval-dialog__button restart-approval-dialog__button--back"
                style={{
                  ...styles.button,
                  ...styles.secondaryButton,
                }}
                onClick={() => setShowRejectInput(false)}
                disabled={isLoading}
              >
                Back
              </button>
            )}

            <button
              className="restart-approval-dialog__button restart-approval-dialog__button--reject"
              style={{
                ...styles.button,
                ...styles.rejectButton,
              }}
              onClick={handleReject}
              disabled={!canReject}
            >
              {isLoading ? (
                <LoadingSpinner />
              ) : showRejectInput ? (
                'Confirm Reject'
              ) : (
                'Reject'
              )}
            </button>

            {!showRejectInput && (
              <button
                className="restart-approval-dialog__button restart-approval-dialog__button--approve"
                style={{
                  ...styles.button,
                  ...styles.approveButton,
                }}
                onClick={handleApprove}
                disabled={!canApprove}
              >
                {isLoading ? <LoadingSpinner /> : 'Approve Restart'}
              </button>
            )}
          </div>
        </div>
      </div>
    </div>
  )
}

// Icon components
const RestartIcon: React.FC = () => (
  <svg width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
    <path d="M23 4v6h-6" />
    <path d="M1 20v-6h6" />
    <path d="M3.51 9a9 9 0 0114.85-3.36L23 10M1 14l4.64 4.36A9 9 0 0020.49 15" />
  </svg>
)

const CloseIcon: React.FC = () => (
  <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
    <line x1="18" y1="6" x2="6" y2="18" />
    <line x1="6" y1="6" x2="18" y2="18" />
  </svg>
)

const WarningIcon: React.FC = () => (
  <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
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
    maxWidth: '520px',
    maxHeight: '90vh',
    display: 'flex',
    flexDirection: 'column',
    overflow: 'hidden',
    boxShadow: '0 8px 32px rgba(0, 0, 0, 0.4)',
  },
  header: {
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'space-between',
    padding: '16px 20px',
    borderBottom: '1px solid #30363d',
    backgroundColor: '#0d1117',
  },
  headerContent: {
    display: 'flex',
    alignItems: 'center',
    gap: '12px',
    color: '#f0883e',
  },
  title: {
    margin: 0,
    fontSize: '16px',
    fontWeight: 600,
    color: '#c9d1d9',
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
  },
  body: {
    padding: '20px',
    overflowY: 'auto',
    flex: 1,
  },
  warningBanner: {
    display: 'flex',
    alignItems: 'flex-start',
    gap: '12px',
    padding: '12px 16px',
    backgroundColor: 'rgba(240, 136, 62, 0.1)',
    border: '1px solid rgba(240, 136, 62, 0.4)',
    borderRadius: '8px',
    marginBottom: '20px',
    fontSize: '13px',
    color: '#f0883e',
    lineHeight: 1.5,
  },
  details: {
    display: 'flex',
    flexDirection: 'column',
    gap: '16px',
  },
  field: {
    display: 'flex',
    flexDirection: 'column',
    gap: '4px',
  },
  label: {
    fontSize: '11px',
    fontWeight: 500,
    textTransform: 'uppercase',
    letterSpacing: '0.5px',
    color: '#8b949e',
  },
  value: {
    fontSize: '14px',
    color: '#c9d1d9',
  },
  reasonBadge: {
    display: 'flex',
    alignItems: 'center',
    gap: '8px',
  },
  reasonDot: {
    width: '8px',
    height: '8px',
    borderRadius: '50%',
  },
  reasonLabel: {
    fontSize: '14px',
    fontWeight: 500,
    color: '#c9d1d9',
  },
  reasonDescription: {
    margin: '4px 0 0 0',
    fontSize: '13px',
    color: '#8b949e',
  },
  description: {
    margin: 0,
    fontSize: '14px',
    color: '#c9d1d9',
    lineHeight: 1.5,
  },
  code: {
    fontSize: '12px',
    fontFamily: 'ui-monospace, SFMono-Regular, "SF Mono", Menlo, Consolas, monospace',
    color: '#58a6ff',
    backgroundColor: '#0d1117',
    padding: '2px 6px',
    borderRadius: '4px',
    display: 'inline-block',
  },
  timeAgo: {
    color: '#8b949e',
    fontSize: '12px',
  },
  statusBadge: {
    display: 'inline-block',
    fontSize: '11px',
    fontWeight: 500,
    textTransform: 'uppercase',
    padding: '4px 8px',
    borderRadius: '4px',
    border: '1px solid',
  },
  rejectInput: {
    marginTop: '16px',
    display: 'flex',
    flexDirection: 'column',
    gap: '8px',
  },
  textarea: {
    width: '100%',
    padding: '10px 12px',
    fontSize: '14px',
    color: '#c9d1d9',
    backgroundColor: '#0d1117',
    border: '1px solid #30363d',
    borderRadius: '6px',
    resize: 'vertical',
    fontFamily: 'inherit',
  },
  footer: {
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'space-between',
    padding: '16px 20px',
    borderTop: '1px solid #30363d',
    backgroundColor: '#0d1117',
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
  cancelButton: {
    backgroundColor: 'transparent',
    color: '#8b949e',
    border: '1px solid #30363d',
  },
  rejectButton: {
    backgroundColor: '#da3633',
    color: '#ffffff',
  },
  approveButton: {
    backgroundColor: '#238636',
    color: '#ffffff',
  },
}

export default RestartApprovalDialog
