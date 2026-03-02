/**
 * ToolCallApproval Component
 * Approval/rejection controls for pending tool calls.
 * @module components/chat/tools/ToolCallApproval
 */
import React, { useCallback, useState } from 'react'

export interface ToolCallApprovalProps {
  toolCallId: string
  riskLevel?: 'low' | 'medium' | 'high'
  onApprove?: () => void
  onReject?: (reason?: string) => void
}

const ToolCallApproval: React.FC<ToolCallApprovalProps> = ({
  toolCallId,
  riskLevel = 'low',
  onApprove,
  onReject,
}) => {
  const [showRejectInput, setShowRejectInput] = useState(false)
  const [rejectReason, setRejectReason] = useState('')

  const handleApprove = useCallback(() => {
    onApprove?.()
  }, [onApprove])

  const handleReject = useCallback(() => {
    if (showRejectInput) {
      onReject?.(rejectReason || undefined)
      setShowRejectInput(false)
      setRejectReason('')
    } else {
      setShowRejectInput(true)
    }
  }, [showRejectInput, rejectReason, onReject])

  const handleCancelReject = useCallback(() => {
    setShowRejectInput(false)
    setRejectReason('')
  }, [])

  // Risk level colors
  const riskColors = {
    low: '#238636',
    medium: '#d29922',
    high: '#f85149',
  }

  return (
    <div
      className="tool-call-approval"
      style={styles.container}
      data-tool-call-id={toolCallId}
    >
      {/* Risk indicator */}
      <div style={styles.riskBadge}>
        <span
          style={{
            ...styles.riskDot,
            backgroundColor: riskColors[riskLevel],
          }}
        />
        <span style={styles.riskText}>{riskLevel} risk</span>
      </div>

      {/* Approval buttons */}
      {!showRejectInput ? (
        <div style={styles.buttons}>
          <button
            onClick={handleApprove}
            style={styles.approveButton}
            title="Approve this tool call"
          >
            Approve
          </button>
          <button
            onClick={handleReject}
            style={styles.rejectButton}
            title="Reject this tool call"
          >
            Reject
          </button>
        </div>
      ) : (
        <div style={styles.rejectForm}>
          <input
            type="text"
            value={rejectReason}
            onChange={(e) => setRejectReason(e.target.value)}
            placeholder="Reason for rejection (optional)"
            style={styles.rejectInput}
            autoFocus
          />
          <div style={styles.rejectActions}>
            <button onClick={handleReject} style={styles.confirmRejectButton}>
              Confirm
            </button>
            <button onClick={handleCancelReject} style={styles.cancelButton}>
              Cancel
            </button>
          </div>
        </div>
      )}
    </div>
  )
}

// Styles
const styles: Record<string, React.CSSProperties> = {
  container: {
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'space-between',
    padding: '8px 12px',
    backgroundColor: '#21262d',
    borderRadius: '6px',
    gap: '12px',
  },
  riskBadge: {
    display: 'flex',
    alignItems: 'center',
    gap: '6px',
  },
  riskDot: {
    width: '8px',
    height: '8px',
    borderRadius: '50%',
  },
  riskText: {
    fontSize: '12px',
    color: '#8b949e',
    textTransform: 'capitalize',
  },
  buttons: {
    display: 'flex',
    gap: '8px',
  },
  approveButton: {
    padding: '4px 12px',
    fontSize: '12px',
    fontWeight: 500,
    color: '#ffffff',
    backgroundColor: '#238636',
    border: 'none',
    borderRadius: '4px',
    cursor: 'pointer',
  },
  rejectButton: {
    padding: '4px 12px',
    fontSize: '12px',
    fontWeight: 500,
    color: '#c9d1d9',
    backgroundColor: 'transparent',
    border: '1px solid #30363d',
    borderRadius: '4px',
    cursor: 'pointer',
  },
  rejectForm: {
    display: 'flex',
    flexDirection: 'column',
    gap: '8px',
    flex: 1,
  },
  rejectInput: {
    padding: '6px 10px',
    fontSize: '12px',
    color: '#c9d1d9',
    backgroundColor: '#0d1117',
    border: '1px solid #30363d',
    borderRadius: '4px',
    outline: 'none',
  },
  rejectActions: {
    display: 'flex',
    gap: '8px',
    justifyContent: 'flex-end',
  },
  confirmRejectButton: {
    padding: '4px 12px',
    fontSize: '12px',
    fontWeight: 500,
    color: '#ffffff',
    backgroundColor: '#f85149',
    border: 'none',
    borderRadius: '4px',
    cursor: 'pointer',
  },
  cancelButton: {
    padding: '4px 12px',
    fontSize: '12px',
    fontWeight: 500,
    color: '#c9d1d9',
    backgroundColor: 'transparent',
    border: '1px solid #30363d',
    borderRadius: '4px',
    cursor: 'pointer',
  },
}

export default ToolCallApproval
