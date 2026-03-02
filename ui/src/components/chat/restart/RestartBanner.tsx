/**
 * RestartBanner Component
 * Banner notification for pending restart requests that requires user attention.
 * @module components/chat/restart/RestartBanner
 */
import React from 'react'
import type { RestartRequest, RestartReason } from '../../../types/restart'

export interface RestartBannerProps {
  /** The pending restart request */
  request: RestartRequest
  /** Callback when approve button is clicked */
  onApprove: () => void
  /** Callback when reject button is clicked */
  onReject: () => void
  /** Callback when view details is clicked */
  onViewDetails?: () => void
  /** Callback when dismiss is clicked (hides temporarily) */
  onDismiss?: () => void
  /** Whether the banner can be dismissed */
  dismissible?: boolean
}

const RestartBanner: React.FC<RestartBannerProps> = ({
  request,
  onApprove,
  onReject,
  onViewDetails,
  onDismiss,
  dismissible = true,
}) => {
  // Get reason display info
  const getReasonLabel = (reason: RestartReason): string => {
    const labels: Record<RestartReason, string> = {
      code_change: 'Code changes',
      config_change: 'Configuration changes',
      user_requested: 'A manual restart',
      recovery: 'Error recovery',
      upgrade: 'An upgrade',
    }
    return labels[reason]
  }

  return (
    <div className="restart-banner" style={styles.container}>
      <div className="restart-banner__content" style={styles.content}>
        {/* Icon */}
        <div className="restart-banner__icon" style={styles.icon}>
          <RestartIcon />
        </div>

        {/* Message */}
        <div className="restart-banner__message" style={styles.message}>
          <strong style={styles.title}>Restart Approval Required</strong>
          <span style={styles.description}>
            {getReasonLabel(request.reason)} require an orchestrator restart.
            {request.buildRequired && ' A build will run before restart.'}
            {request.resumeEnabled && ' Your session will resume automatically.'}
          </span>
        </div>

        {/* Actions */}
        <div className="restart-banner__actions" style={styles.actions}>
          {onViewDetails && (
            <button
              className="restart-banner__action restart-banner__action--details"
              style={{
                ...styles.button,
                ...styles.detailsButton,
              }}
              onClick={onViewDetails}
            >
              Details
            </button>
          )}
          <button
            className="restart-banner__action restart-banner__action--reject"
            style={{
              ...styles.button,
              ...styles.rejectButton,
            }}
            onClick={onReject}
          >
            Reject
          </button>
          <button
            className="restart-banner__action restart-banner__action--approve"
            style={{
              ...styles.button,
              ...styles.approveButton,
            }}
            onClick={onApprove}
          >
            <CheckIcon />
            Approve
          </button>
        </div>

        {/* Dismiss button */}
        {dismissible && onDismiss && (
          <button
            className="restart-banner__dismiss"
            style={styles.dismissButton}
            onClick={onDismiss}
            aria-label="Dismiss notification"
          >
            <CloseIcon />
          </button>
        )}
      </div>
    </div>
  )
}

// Icon components
const RestartIcon: React.FC = () => (
  <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
    <path d="M23 4v6h-6" />
    <path d="M1 20v-6h6" />
    <path d="M3.51 9a9 9 0 0114.85-3.36L23 10M1 14l4.64 4.36A9 9 0 0020.49 15" />
  </svg>
)

const CheckIcon: React.FC = () => (
  <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.5">
    <polyline points="20 6 9 17 4 12" />
  </svg>
)

const CloseIcon: React.FC = () => (
  <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
    <line x1="18" y1="6" x2="6" y2="18" />
    <line x1="6" y1="6" x2="18" y2="18" />
  </svg>
)

// Styles
const styles: Record<string, React.CSSProperties> = {
  container: {
    backgroundColor: 'rgba(240, 136, 62, 0.1)',
    borderBottom: '1px solid rgba(240, 136, 62, 0.4)',
  },
  content: {
    display: 'flex',
    alignItems: 'center',
    gap: '16px',
    padding: '12px 16px',
    maxWidth: '1200px',
    margin: '0 auto',
  },
  icon: {
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'center',
    width: '36px',
    height: '36px',
    borderRadius: '8px',
    backgroundColor: 'rgba(240, 136, 62, 0.2)',
    color: '#f0883e',
    flexShrink: 0,
  },
  message: {
    flex: 1,
    display: 'flex',
    flexDirection: 'column',
    gap: '2px',
    minWidth: 0,
  },
  title: {
    fontSize: '14px',
    fontWeight: 600,
    color: '#c9d1d9',
  },
  description: {
    fontSize: '13px',
    color: '#8b949e',
    overflow: 'hidden',
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
  },
  actions: {
    display: 'flex',
    alignItems: 'center',
    gap: '8px',
    flexShrink: 0,
  },
  button: {
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'center',
    gap: '6px',
    padding: '6px 14px',
    fontSize: '13px',
    fontWeight: 500,
    borderRadius: '6px',
    border: 'none',
    cursor: 'pointer',
    transition: 'background-color 0.2s, opacity 0.2s',
    whiteSpace: 'nowrap',
  },
  detailsButton: {
    backgroundColor: 'transparent',
    color: '#8b949e',
    border: '1px solid #30363d',
  },
  rejectButton: {
    backgroundColor: '#21262d',
    color: '#f85149',
    border: '1px solid #30363d',
  },
  approveButton: {
    backgroundColor: '#238636',
    color: '#ffffff',
  },
  dismissButton: {
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'center',
    width: '28px',
    height: '28px',
    borderRadius: '4px',
    border: 'none',
    backgroundColor: 'transparent',
    color: '#8b949e',
    cursor: 'pointer',
    flexShrink: 0,
  },
}

export default RestartBanner
