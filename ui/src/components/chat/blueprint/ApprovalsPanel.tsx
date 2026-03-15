/**
 * ApprovalsPanel Component
 *
 * Displays pending approval requests and allows approve/reject actions.
 * Fetches approvals from the API and supports real-time updates via WebSocket.
 *
 * @module components/chat/blueprint/ApprovalsPanel
 */

import React, { useState, useEffect, useCallback } from 'react'
import type {
  Approval,
  ApprovalRequestStatus,
  RiskLevel,
} from '../../../types/blueprint'
import { RISK_LEVEL_INFO } from '../../../types/blueprint'

export interface ApprovalsPanelProps {
  /** API base URL for approval endpoints */
  apiUrl: string
  /** Optional auth token */
  authToken?: string
  /** Run ID to filter approvals (optional) */
  runId?: string
  /** Callback when an approval is acted upon */
  onApprovalAction?: (id: string, approved: boolean) => void
  /** External approvals list (for WebSocket updates) */
  approvals?: Approval[]
  /** Whether the panel is loading */
  isLoading?: boolean
  /** Error message */
  error?: string
}

/**
 * ApprovalsPanel Component
 *
 * Lists pending approval requests with approve/reject buttons.
 */
const ApprovalsPanel: React.FC<ApprovalsPanelProps> = ({
  apiUrl,
  authToken,
  runId,
  onApprovalAction,
  approvals: externalApprovals,
  isLoading: externalLoading,
  error: externalError,
}) => {
  const [approvals, setApprovals] = useState<Approval[]>(externalApprovals || [])
  const [isLoading, setIsLoading] = useState(false)
  const [error, setError] = useState<string | undefined>(externalError)
  const [actionInProgress, setActionInProgress] = useState<string | null>(null)

  // Sync with external approvals
  useEffect(() => {
    if (externalApprovals) {
      setApprovals(externalApprovals)
    }
  }, [externalApprovals])

  // Fetch approvals from API
  const fetchApprovals = useCallback(async () => {
    if (externalApprovals !== undefined) {
      return // Using external approvals, don't fetch
    }

    setIsLoading(true)
    setError(undefined)

    try {
      const params = new URLSearchParams()
      if (runId) params.set('run_id', runId)
      params.set('status', 'pending')

      const response = await fetch(`${apiUrl}/api/v1/approvals?${params}`, {
        headers: authToken
          ? { Authorization: `Bearer ${authToken}` }
          : undefined,
      })

      if (!response.ok) {
        throw new Error(`Failed to fetch approvals: ${response.statusText}`)
      }

      const data = await response.json()
      setApprovals(data.approvals || [])
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to fetch approvals')
    } finally {
      setIsLoading(false)
    }
  }, [apiUrl, authToken, runId, externalApprovals])

  // Fetch approvals on mount and when filters change
  useEffect(() => {
    fetchApprovals()
  }, [fetchApprovals])

  // Handle approve action
  const handleApprove = useCallback(
    async (approval: Approval) => {
      setActionInProgress(approval.id)
      setError(undefined)

      try {
        const response = await fetch(
          `${apiUrl}/api/v1/approvals/${approval.id}/approve`,
          {
            method: 'POST',
            headers: {
              'Content-Type': 'application/json',
              ...(authToken ? { Authorization: `Bearer ${authToken}` } : {}),
            },
            body: JSON.stringify({
              decided_by: 'user',
            }),
          }
        )

        if (!response.ok) {
          throw new Error(`Failed to approve: ${response.statusText}`)
        }

        // Update local state
        setApprovals((prev) =>
          prev.map((a) =>
            a.id === approval.id ? { ...a, status: 'approved' as ApprovalRequestStatus } : a
          )
        )

        onApprovalAction?.(approval.id, true)
      } catch (err) {
        setError(err instanceof Error ? err.message : 'Failed to approve')
      } finally {
        setActionInProgress(null)
      }
    },
    [apiUrl, authToken, onApprovalAction]
  )

  // Handle reject action
  const handleReject = useCallback(
    async (approval: Approval, reason?: string) => {
      setActionInProgress(approval.id)
      setError(undefined)

      try {
        const response = await fetch(
          `${apiUrl}/api/v1/approvals/${approval.id}/reject`,
          {
            method: 'POST',
            headers: {
              'Content-Type': 'application/json',
              ...(authToken ? { Authorization: `Bearer ${authToken}` } : {}),
            },
            body: JSON.stringify({
              decided_by: 'user',
              reason: reason || 'Rejected by user',
            }),
          }
        )

        if (!response.ok) {
          throw new Error(`Failed to reject: ${response.statusText}`)
        }

        // Update local state
        setApprovals((prev) =>
          prev.map((a) =>
            a.id === approval.id ? { ...a, status: 'rejected' as ApprovalRequestStatus } : a
          )
        )

        onApprovalAction?.(approval.id, false)
      } catch (err) {
        setError(err instanceof Error ? err.message : 'Failed to reject')
      } finally {
        setActionInProgress(null)
      }
    },
    [apiUrl, authToken, onApprovalAction]
  )

  // Filter to show only pending approvals
  const pendingApprovals = approvals.filter((a) => a.status === 'pending')

  // Format timestamp
  const formatTime = (timestamp: string): string => {
    const date = new Date(timestamp)
    return date.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })
  }

  // Get risk level style
  const getRiskLevelStyle = (level: RiskLevel): React.CSSProperties => {
    const info = RISK_LEVEL_INFO[level] || RISK_LEVEL_INFO.medium
    return {
      color: info.color,
      backgroundColor: `${info.color}20`,
      borderColor: info.color,
    }
  }

  const loading = externalLoading !== undefined ? externalLoading : isLoading
  const displayError = externalError !== undefined ? externalError : error

  return (
    <div className="approvals-panel" style={styles.container}>
      <div className="approvals-panel__header" style={styles.header}>
        <h3 style={styles.title}>
          Pending Approvals
          {pendingApprovals.length > 0 && (
            <span style={styles.count}>{pendingApprovals.length}</span>
          )}
        </h3>
        <button
          className="approvals-panel__refresh"
          style={styles.refreshBtn}
          onClick={() => fetchApprovals()}
          disabled={loading}
          aria-label="Refresh approvals"
          title="Refresh"
        >
          <RefreshIcon spinning={loading} />
        </button>
      </div>

      {displayError && (
        <div className="approvals-panel__error" style={styles.error}>
          <ErrorIcon />
          {displayError}
        </div>
      )}

      <div className="approvals-panel__list" style={styles.list}>
        {loading && pendingApprovals.length === 0 ? (
          <div className="approvals-panel__loading" style={styles.emptyState}>
            <RefreshIcon spinning />
            <span>Loading approvals...</span>
          </div>
        ) : pendingApprovals.length === 0 ? (
          <div className="approvals-panel__empty" style={styles.emptyState}>
            <CheckAllIcon />
            <span>No pending approvals</span>
          </div>
        ) : (
          pendingApprovals.map((approval) => (
            <ApprovalCard
              key={approval.id}
              approval={approval}
              isProcessing={actionInProgress === approval.id}
              onApprove={() => handleApprove(approval)}
              onReject={(reason) => handleReject(approval, reason)}
              getRiskLevelStyle={getRiskLevelStyle}
              formatTime={formatTime}
            />
          ))
        )}
      </div>
    </div>
  )
}

// ApprovalCard sub-component
interface ApprovalCardProps {
  approval: Approval
  isProcessing: boolean
  onApprove: () => void
  onReject: (reason?: string) => void
  getRiskLevelStyle: (level: RiskLevel) => React.CSSProperties
  formatTime: (timestamp: string) => string
}

const ApprovalCard: React.FC<ApprovalCardProps> = ({
  approval,
  isProcessing,
  onApprove,
  onReject,
  getRiskLevelStyle,
  formatTime,
}) => {
  const [showRejectInput, setShowRejectInput] = useState(false)
  const [rejectReason, setRejectReason] = useState('')

  const handleRejectClick = () => {
    if (showRejectInput) {
      onReject(rejectReason || undefined)
      setShowRejectInput(false)
      setRejectReason('')
    } else {
      setShowRejectInput(true)
    }
  }

  const riskStyle = getRiskLevelStyle(approval.riskLevel)

  return (
    <div
      className={`approval-card approval-card--${approval.riskLevel}`}
      style={{
        ...styles.card,
        borderLeftColor: riskStyle.color,
        opacity: isProcessing ? 0.7 : 1,
      }}
    >
      {/* Header */}
      <div className="approval-card__header" style={styles.cardHeader}>
        <div className="approval-card__tool" style={styles.toolName}>
          <ToolIcon />
          {approval.toolName}
        </div>
        <span
          className="approval-card__risk"
          style={{
            ...styles.riskBadge,
            ...riskStyle,
          }}
        >
          {RISK_LEVEL_INFO[approval.riskLevel]?.label || approval.riskLevel}
        </span>
      </div>

      {/* Description */}
      <p className="approval-card__description" style={styles.description}>
        {approval.description}
      </p>

      {/* Tool args preview */}
      {approval.toolArgs && Object.keys(approval.toolArgs).length > 0 && (
        <details className="approval-card__args" style={styles.argsContainer}>
          <summary style={styles.argsSummary}>View arguments</summary>
          <pre style={styles.argsCode}>
            {JSON.stringify(approval.toolArgs, null, 2)}
          </pre>
        </details>
      )}

      {/* Meta */}
      <div className="approval-card__meta" style={styles.meta}>
        <span style={styles.metaItem}>
          <ClockIcon />
          {formatTime(approval.createdAt)}
        </span>
        <span style={styles.metaItem}>
          <RunIcon />
          {approval.runId.slice(0, 8)}...
        </span>
      </div>

      {/* Reject reason input */}
      {showRejectInput && (
        <div className="approval-card__reject-input" style={styles.rejectInput}>
          <input
            type="text"
            placeholder="Reason for rejection (optional)"
            value={rejectReason}
            onChange={(e) => setRejectReason(e.target.value)}
            style={styles.input}
            autoFocus
            onKeyDown={(e) => {
              if (e.key === 'Enter') {
                handleRejectClick()
              } else if (e.key === 'Escape') {
                setShowRejectInput(false)
                setRejectReason('')
              }
            }}
          />
        </div>
      )}

      {/* Actions */}
      <div className="approval-card__actions" style={styles.actions}>
        <button
          className="approval-card__reject"
          style={{
            ...styles.actionBtn,
            ...styles.rejectBtn,
          }}
          onClick={handleRejectClick}
          disabled={isProcessing}
        >
          <RejectIcon />
          {showRejectInput ? 'Confirm Reject' : 'Reject'}
        </button>
        {showRejectInput && (
          <button
            className="approval-card__cancel"
            style={{
              ...styles.actionBtn,
              ...styles.cancelBtn,
            }}
            onClick={() => {
              setShowRejectInput(false)
              setRejectReason('')
            }}
          >
            Cancel
          </button>
        )}
        <button
          className="approval-card__approve"
          style={{
            ...styles.actionBtn,
            ...styles.approveBtn,
          }}
          onClick={onApprove}
          disabled={isProcessing || showRejectInput}
        >
          <ApproveIcon />
          Approve
        </button>
      </div>
    </div>
  )
}

// Icon components
const RefreshIcon: React.FC<{ spinning?: boolean }> = ({ spinning }) => (
  <svg
    width="14"
    height="14"
    viewBox="0 0 24 24"
    fill="none"
    stroke="currentColor"
    strokeWidth="2"
    style={spinning ? { animation: 'spin 1s linear infinite' } : undefined}
  >
    <path d="M23 4v6h-6" />
    <path d="M20.49 15a9 9 0 1 1-2.12-9.36L23 10" />
  </svg>
)

const ErrorIcon: React.FC = () => (
  <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
    <circle cx="12" cy="12" r="10" />
    <line x1="12" y1="8" x2="12" y2="12" />
    <line x1="12" y1="16" x2="12.01" y2="16" />
  </svg>
)

const CheckAllIcon: React.FC = () => (
  <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
    <polyline points="9 11 12 14 22 4" />
    <path d="M21 12v7a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h11" />
  </svg>
)

const ToolIcon: React.FC = () => (
  <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
    <path d="M14.7 6.3a1 1 0 0 0 0 1.4l1.6 1.6a1 1 0 0 0 1.4 0l3.77-3.77a6 6 0 0 1-7.94 7.94l-6.91 6.91a2.12 2.12 0 0 1-3-3l6.91-6.91a6 6 0 0 1 7.94-7.94l-3.76 3.76z" />
  </svg>
)

const ClockIcon: React.FC = () => (
  <svg width="10" height="10" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
    <circle cx="12" cy="12" r="10" />
    <polyline points="12 6 12 12 16 14" />
  </svg>
)

const RunIcon: React.FC = () => (
  <svg width="10" height="10" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
    <polygon points="5 3 19 12 5 21 5 3" />
  </svg>
)

const ApproveIcon: React.FC = () => (
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
    display: 'flex',
    flexDirection: 'column',
    maxHeight: '400px',
  },
  header: {
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'space-between',
    padding: '12px 16px',
    borderBottom: '1px solid #30363d',
    flexShrink: 0,
  },
  title: {
    margin: 0,
    fontSize: '14px',
    fontWeight: 600,
    color: '#c9d1d9',
    display: 'flex',
    alignItems: 'center',
    gap: '8px',
  },
  count: {
    fontSize: '11px',
    fontWeight: 500,
    color: '#f0883e',
    backgroundColor: '#f0883e20',
    padding: '2px 8px',
    borderRadius: '10px',
  },
  refreshBtn: {
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'center',
    width: '28px',
    height: '28px',
    backgroundColor: 'transparent',
    border: 'none',
    borderRadius: '4px',
    color: '#8b949e',
    cursor: 'pointer',
  },
  error: {
    display: 'flex',
    alignItems: 'center',
    gap: '8px',
    padding: '8px 16px',
    backgroundColor: '#da363320',
    color: '#da3633',
    fontSize: '12px',
  },
  list: {
    flex: 1,
    overflow: 'auto',
    padding: '8px',
  },
  emptyState: {
    display: 'flex',
    flexDirection: 'column',
    alignItems: 'center',
    justifyContent: 'center',
    gap: '8px',
    padding: '32px',
    color: '#8b949e',
    fontSize: '13px',
  },
  card: {
    backgroundColor: '#0d1117',
    borderRadius: '6px',
    border: '1px solid #30363d',
    borderLeftWidth: '3px',
    padding: '12px',
    marginBottom: '8px',
    display: 'flex',
    flexDirection: 'column',
    gap: '8px',
    transition: 'opacity 0.2s',
  },
  cardHeader: {
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'space-between',
  },
  toolName: {
    display: 'flex',
    alignItems: 'center',
    gap: '6px',
    fontSize: '13px',
    fontWeight: 500,
    color: '#c9d1d9',
  },
  riskBadge: {
    fontSize: '10px',
    fontWeight: 500,
    textTransform: 'uppercase',
    padding: '2px 6px',
    borderRadius: '4px',
    border: '1px solid',
  },
  description: {
    margin: 0,
    fontSize: '12px',
    color: '#8b949e',
    lineHeight: 1.4,
  },
  argsContainer: {
    fontSize: '11px',
    color: '#8b949e',
  },
  argsSummary: {
    cursor: 'pointer',
    padding: '4px 0',
  },
  argsCode: {
    margin: '4px 0 0 0',
    padding: '8px',
    backgroundColor: '#21262d',
    borderRadius: '4px',
    fontSize: '10px',
    overflow: 'auto',
    maxHeight: '100px',
  },
  meta: {
    display: 'flex',
    alignItems: 'center',
    gap: '12px',
    fontSize: '10px',
    color: '#6e7681',
  },
  metaItem: {
    display: 'flex',
    alignItems: 'center',
    gap: '4px',
  },
  rejectInput: {
    marginTop: '4px',
  },
  input: {
    width: '100%',
    padding: '6px 8px',
    backgroundColor: '#0d1117',
    border: '1px solid #30363d',
    borderRadius: '4px',
    color: '#c9d1d9',
    fontSize: '12px',
  },
  actions: {
    display: 'flex',
    gap: '8px',
    marginTop: '4px',
    paddingTop: '8px',
    borderTop: '1px solid #21262d',
  },
  actionBtn: {
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
  approveBtn: {
    marginLeft: 'auto',
    backgroundColor: '#238636',
    color: '#ffffff',
  },
  rejectBtn: {
    backgroundColor: '#21262d',
    color: '#f85149',
    border: '1px solid #30363d',
  },
  cancelBtn: {
    backgroundColor: '#21262d',
    color: '#8b949e',
    border: '1px solid #30363d',
  },
}

export default ApprovalsPanel
