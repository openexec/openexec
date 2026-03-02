/**
 * RestartRequestCard Component
 * Card component for displaying a restart request summary.
 * @module components/chat/restart/RestartRequestCard
 */
import React from 'react'
import type { RestartRequest, RestartReason, RestartStatus } from '../../../types/restart'

export interface RestartRequestCardProps {
  /** The restart request to display */
  request: RestartRequest
  /** Callback when card is clicked to view details */
  onClick?: () => void
  /** Callback when quick approve is clicked */
  onQuickApprove?: () => void
  /** Callback when quick reject is clicked */
  onQuickReject?: () => void
  /** Whether the card is in compact mode */
  compact?: boolean
}

const RestartRequestCard: React.FC<RestartRequestCardProps> = ({
  request,
  onClick,
  onQuickApprove,
  onQuickReject,
  compact = false,
}) => {
  // Get reason display info
  const getReasonInfo = (reason: RestartReason) => {
    const info: Record<RestartReason, { label: string; icon: React.ReactNode; color: string }> = {
      code_change: { label: 'Code Change', icon: <CodeIcon />, color: '#58a6ff' },
      config_change: { label: 'Config Change', icon: <SettingsIcon />, color: '#a371f7' },
      user_requested: { label: 'User Request', icon: <UserIcon />, color: '#7ee787' },
      recovery: { label: 'Recovery', icon: <RecoveryIcon />, color: '#f0883e' },
      upgrade: { label: 'Upgrade', icon: <UpgradeIcon />, color: '#79c0ff' },
    }
    return info[reason]
  }

  // Get status display info
  const getStatusInfo = (status: RestartStatus) => {
    const info: Record<RestartStatus, { label: string; color: string }> = {
      pending: { label: 'Pending', color: '#f0883e' },
      approved: { label: 'Approved', color: '#238636' },
      rejected: { label: 'Rejected', color: '#da3633' },
      in_progress: { label: 'In Progress', color: '#58a6ff' },
      complete: { label: 'Complete', color: '#238636' },
      failed: { label: 'Failed', color: '#da3633' },
      cancelled: { label: 'Cancelled', color: '#8b949e' },
    }
    return info[status]
  }

  // Format time ago
  const getTimeAgo = (timestamp: string): string => {
    const now = new Date()
    const then = new Date(timestamp)
    const seconds = Math.floor((now.getTime() - then.getTime()) / 1000)

    if (seconds < 60) return 'just now'
    if (seconds < 3600) return `${Math.floor(seconds / 60)}m ago`
    if (seconds < 86400) return `${Math.floor(seconds / 3600)}h ago`
    return `${Math.floor(seconds / 86400)}d ago`
  }

  const reasonInfo = getReasonInfo(request.reason)
  const statusInfo = getStatusInfo(request.status)
  const isPending = request.status === 'pending'

  // Handle quick actions without triggering onClick
  const handleQuickApprove = (e: React.MouseEvent) => {
    e.stopPropagation()
    if (onQuickApprove) {
      onQuickApprove()
    }
  }

  const handleQuickReject = (e: React.MouseEvent) => {
    e.stopPropagation()
    if (onQuickReject) {
      onQuickReject()
    }
  }

  return (
    <div
      className={`restart-request-card restart-request-card--${request.status} ${compact ? 'restart-request-card--compact' : ''}`}
      style={{
        ...styles.container,
        borderLeftColor: statusInfo.color,
        cursor: onClick ? 'pointer' : 'default',
      }}
      onClick={onClick}
      role={onClick ? 'button' : undefined}
      tabIndex={onClick ? 0 : undefined}
      onKeyDown={
        onClick
          ? (e) => {
              if (e.key === 'Enter' || e.key === ' ') {
                e.preventDefault()
                onClick()
              }
            }
          : undefined
      }
    >
      {/* Header row */}
      <div className="restart-request-card__header" style={styles.header}>
        {/* Reason icon and label */}
        <div className="restart-request-card__reason" style={styles.reason}>
          <span style={{ color: reasonInfo.color }}>{reasonInfo.icon}</span>
          <span style={styles.reasonLabel}>{reasonInfo.label}</span>
        </div>

        {/* Status badge */}
        <span
          className="restart-request-card__status"
          style={{
            ...styles.statusBadge,
            backgroundColor: `${statusInfo.color}20`,
            color: statusInfo.color,
            borderColor: statusInfo.color,
          }}
        >
          {statusInfo.label}
        </span>
      </div>

      {/* Description */}
      {!compact && request.description && (
        <p className="restart-request-card__description" style={styles.description}>
          {request.description}
        </p>
      )}

      {/* Meta row */}
      <div className="restart-request-card__meta" style={styles.meta}>
        <span className="restart-request-card__requested-by" style={styles.metaItem}>
          <UserSmallIcon />
          {request.requestedBy}
        </span>
        <span className="restart-request-card__time" style={styles.metaItem}>
          <ClockIcon />
          {getTimeAgo(request.createdAt)}
        </span>
        {request.buildRequired && (
          <span className="restart-request-card__build" style={{ ...styles.metaItem, color: '#f0883e' }}>
            <BuildIcon />
            Build required
          </span>
        )}
        {request.resumeEnabled && (
          <span className="restart-request-card__resume" style={{ ...styles.metaItem, color: '#238636' }}>
            <ResumeIcon />
            Resume enabled
          </span>
        )}
      </div>

      {/* Quick actions for pending requests */}
      {isPending && (onQuickApprove || onQuickReject) && (
        <div className="restart-request-card__actions" style={styles.actions}>
          {onQuickReject && (
            <button
              className="restart-request-card__action restart-request-card__action--reject"
              style={{
                ...styles.actionButton,
                ...styles.rejectAction,
              }}
              onClick={handleQuickReject}
            >
              <RejectIcon />
              Reject
            </button>
          )}
          {onQuickApprove && (
            <button
              className="restart-request-card__action restart-request-card__action--approve"
              style={{
                ...styles.actionButton,
                ...styles.approveAction,
              }}
              onClick={handleQuickApprove}
            >
              <CheckIcon />
              Approve
            </button>
          )}
        </div>
      )}
    </div>
  )
}

// Icon components
const CodeIcon: React.FC = () => (
  <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
    <polyline points="16 18 22 12 16 6" />
    <polyline points="8 6 2 12 8 18" />
  </svg>
)

const SettingsIcon: React.FC = () => (
  <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
    <circle cx="12" cy="12" r="3" />
    <path d="M19.4 15a1.65 1.65 0 00.33 1.82l.06.06a2 2 0 010 2.83 2 2 0 01-2.83 0l-.06-.06a1.65 1.65 0 00-1.82-.33 1.65 1.65 0 00-1 1.51V21a2 2 0 01-2 2 2 2 0 01-2-2v-.09A1.65 1.65 0 009 19.4a1.65 1.65 0 00-1.82.33l-.06.06a2 2 0 01-2.83 0 2 2 0 010-2.83l.06-.06a1.65 1.65 0 00.33-1.82 1.65 1.65 0 00-1.51-1H3a2 2 0 01-2-2 2 2 0 012-2h.09A1.65 1.65 0 004.6 9a1.65 1.65 0 00-.33-1.82l-.06-.06a2 2 0 010-2.83 2 2 0 012.83 0l.06.06a1.65 1.65 0 001.82.33H9a1.65 1.65 0 001-1.51V3a2 2 0 012-2 2 2 0 012 2v.09a1.65 1.65 0 001 1.51 1.65 1.65 0 001.82-.33l.06-.06a2 2 0 012.83 0 2 2 0 010 2.83l-.06.06a1.65 1.65 0 00-.33 1.82V9a1.65 1.65 0 001.51 1H21a2 2 0 012 2 2 2 0 01-2 2h-.09a1.65 1.65 0 00-1.51 1z" />
  </svg>
)

const UserIcon: React.FC = () => (
  <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
    <path d="M20 21v-2a4 4 0 00-4-4H8a4 4 0 00-4 4v2" />
    <circle cx="12" cy="7" r="4" />
  </svg>
)

const RecoveryIcon: React.FC = () => (
  <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
    <path d="M23 4v6h-6" />
    <path d="M20.49 15a9 9 0 11-2.12-9.36L23 10" />
  </svg>
)

const UpgradeIcon: React.FC = () => (
  <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
    <line x1="12" y1="19" x2="12" y2="5" />
    <polyline points="5 12 12 5 19 12" />
  </svg>
)

const UserSmallIcon: React.FC = () => (
  <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
    <path d="M20 21v-2a4 4 0 00-4-4H8a4 4 0 00-4 4v2" />
    <circle cx="12" cy="7" r="4" />
  </svg>
)

const ClockIcon: React.FC = () => (
  <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
    <circle cx="12" cy="12" r="10" />
    <polyline points="12 6 12 12 16 14" />
  </svg>
)

const BuildIcon: React.FC = () => (
  <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
    <path d="M14.7 6.3a1 1 0 000 1.4l1.6 1.6a1 1 0 001.4 0l3.77-3.77a6 6 0 01-7.94 7.94l-6.91 6.91a2.12 2.12 0 01-3-3l6.91-6.91a6 6 0 017.94-7.94l-3.76 3.76z" />
  </svg>
)

const ResumeIcon: React.FC = () => (
  <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
    <polygon points="5 3 19 12 5 21 5 3" />
  </svg>
)

const CheckIcon: React.FC = () => (
  <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
    <polyline points="20 6 9 17 4 12" />
  </svg>
)

const RejectIcon: React.FC = () => (
  <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
    <line x1="18" y1="6" x2="6" y2="18" />
    <line x1="6" y1="6" x2="18" y2="18" />
  </svg>
)

// Styles
const styles: Record<string, React.CSSProperties> = {
  container: {
    backgroundColor: '#161b22',
    borderRadius: '8px',
    border: '1px solid #30363d',
    borderLeftWidth: '3px',
    padding: '12px 16px',
    display: 'flex',
    flexDirection: 'column',
    gap: '8px',
    transition: 'background-color 0.2s, border-color 0.2s',
  },
  header: {
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'space-between',
  },
  reason: {
    display: 'flex',
    alignItems: 'center',
    gap: '8px',
  },
  reasonLabel: {
    fontSize: '14px',
    fontWeight: 500,
    color: '#c9d1d9',
  },
  statusBadge: {
    fontSize: '10px',
    fontWeight: 500,
    textTransform: 'uppercase',
    padding: '2px 6px',
    borderRadius: '4px',
    border: '1px solid',
  },
  description: {
    margin: 0,
    fontSize: '13px',
    color: '#8b949e',
    lineHeight: 1.4,
  },
  meta: {
    display: 'flex',
    flexWrap: 'wrap',
    gap: '12px',
    fontSize: '11px',
    color: '#8b949e',
  },
  metaItem: {
    display: 'flex',
    alignItems: 'center',
    gap: '4px',
  },
  actions: {
    display: 'flex',
    gap: '8px',
    marginTop: '8px',
    paddingTop: '12px',
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
  approveAction: {
    backgroundColor: '#238636',
    color: '#ffffff',
  },
  rejectAction: {
    backgroundColor: '#21262d',
    color: '#f85149',
    border: '1px solid #30363d',
  },
}

export default RestartRequestCard
