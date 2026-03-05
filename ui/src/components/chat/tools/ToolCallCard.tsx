/**
 * ToolCallCard Component
 * Individual tool call display card with header, input, output, and approval controls.
 * @module components/chat/tools/ToolCallCard
 */
import React, { useState } from 'react'
import type { ToolCall } from '../../../types/chat'
import ToolCallInput from './ToolCallInput'
import ToolCallOutput from './ToolCallOutput'
import ToolCallApproval from './ToolCallApproval'

export interface ToolCallCardProps {
  /** Tool call data */
  toolCall: ToolCall
  /** Callback when tool call is approved */
  onApprove?: () => void
  /** Callback when tool call is rejected */
  onReject?: (reason?: string) => void
}

const ToolCallCard: React.FC<ToolCallCardProps> = ({
  toolCall,
  onApprove,
  onReject,
}) => {
  const [isExpanded, setIsExpanded] = useState(true)

  // Determine card status for styling
  const getStatusInfo = (): { color: string; label: string; icon: React.ReactNode } => {
    switch (toolCall.status) {
      case 'pending':
        return {
          color: '#f0883e',
          label: 'Pending',
          icon: <PendingIcon />,
        }
      case 'running':
        return {
          color: '#58a6ff',
          label: 'Running',
          icon: <RunningIcon />,
        }
      case 'completed':
        return {
          color: '#238636',
          label: 'Completed',
          icon: <CompletedIcon />,
        }
      case 'failed':
        return {
          color: '#da3633',
          label: 'Failed',
          icon: <ErrorIcon />,
        }
      case 'cancelled':
        return {
          color: '#8b949e',
          label: 'Cancelled',
          icon: <CancelledIcon />,
        }
      case 'timeout':
        return {
          color: '#da3633',
          label: 'Timeout',
          icon: <TimeoutIcon />,
        }
      default:
        return {
          color: '#8b949e',
          label: 'Unknown',
          icon: <PendingIcon />,
        }
    }
  }

  // Get risk level badge color
  const getRiskColor = (): string => {
    switch (toolCall.riskLevel) {
      case 'high':
        return '#da3633'
      case 'medium':
        return '#f0883e'
      case 'low':
      default:
        return '#238636'
    }
  }

  // Format duration for display
  const formatDuration = (ms?: number): string => {
    if (!ms) return ''
    if (ms < 1000) return `${ms}ms`
    return `${(ms / 1000).toFixed(2)}s`
  }

  // Get tool name display
  const getToolNameDisplay = (): string => {
    // Map common tool names to friendlier display names
    const toolNameMap: Record<string, string> = {
      read_file: 'Read File',
      write_file: 'Write File',
      run_shell_command: 'Run Command',
      git_apply_patch: 'Apply Patch',
      openexec_signal: 'OpenExec Signal',
    }
    return toolNameMap[toolCall.toolName] || toolCall.toolName
  }

  const statusInfo = getStatusInfo()
  const isPending = toolCall.approvalStatus === 'pending'
  const hasOutput = toolCall.toolOutput !== undefined
  const hasError = toolCall.status === 'failed' || toolCall.status === 'timeout'

  return (
    <div
      className={`tool-call-card tool-call-card--${toolCall.status}`}
      style={{
        ...styles.container,
        borderColor: statusInfo.color,
      }}
    >
      {/* Header */}
      <div
        className="tool-call-card__header"
        style={styles.header}
        onClick={() => setIsExpanded(!isExpanded)}
        role="button"
        tabIndex={0}
        onKeyDown={(e) => {
          if (e.key === 'Enter' || e.key === ' ') {
            e.preventDefault()
            setIsExpanded(!isExpanded)
          }
        }}
      >
        {/* Status icon */}
        <span
          className="tool-call-card__status-icon"
          style={{
            ...styles.statusIcon,
            color: statusInfo.color,
          }}
        >
          {statusInfo.icon}
        </span>

        {/* Tool name */}
        <span className="tool-call-card__name" style={styles.name}>
          {getToolNameDisplay()}
        </span>

        {/* Risk level badge */}
        {toolCall.riskLevel && (
          <span
            className="tool-call-card__risk"
            style={{
              ...styles.riskBadge,
              backgroundColor: `${getRiskColor()}20`,
              color: getRiskColor(),
              borderColor: getRiskColor(),
            }}
          >
            {toolCall.riskLevel}
          </span>
        )}

        {/* Status label */}
        <span
          className="tool-call-card__status"
          style={{
            ...styles.statusLabel,
            color: statusInfo.color,
          }}
        >
          {statusInfo.label}
        </span>

        {/* Duration */}
        {toolCall.durationMs !== undefined && (
          <span className="tool-call-card__duration" style={styles.duration}>
            {formatDuration(toolCall.durationMs)}
          </span>
        )}

        {/* Expand/collapse chevron */}
        <span
          className="tool-call-card__chevron"
          style={{
            ...styles.chevron,
            transform: isExpanded ? 'rotate(180deg)' : 'rotate(0deg)',
          }}
        >
          <ChevronIcon />
        </span>
      </div>

      {/* Body (collapsible) */}
      {isExpanded && (
        <div className="tool-call-card__body" style={styles.body}>
          {/* Input section */}
          <ToolCallInput
            input={toolCall.toolInput}
            toolName={toolCall.toolName}
          />

          {/* Output section (if available) */}
          {hasOutput && (
            <ToolCallOutput
              output={toolCall.toolOutput}
              isError={hasError}
              duration={toolCall.durationMs}
            />
          )}

          {/* Progress indicator */}
          {toolCall.status === 'running' && toolCall.progressPercent !== undefined && (
            <div className="tool-call-card__progress" style={styles.progress}>
              <div
                className="tool-call-card__progress-bar"
                style={{
                  ...styles.progressBar,
                  width: `${toolCall.progressPercent}%`,
                }}
              />
              {toolCall.progressMessage && (
                <span style={styles.progressText}>{toolCall.progressMessage}</span>
              )}
            </div>
          )}

          {/* Error message */}
          {toolCall.error && (
            <div className="tool-call-card__error" style={styles.error}>
              <ErrorIcon />
              <span>{toolCall.error}</span>
            </div>
          )}

          {/* Approval controls */}
          {isPending && (onApprove || onReject) && (
            <ToolCallApproval
              toolCallId={toolCall.id}
              riskLevel={toolCall.riskLevel as 'low' | 'medium' | 'high' | undefined}
              onApprove={onApprove}
              onReject={onReject}
            />
          )}
        </div>
      )}
    </div>
  )
}

// Icon components
const PendingIcon: React.FC = () => (
  <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
    <circle cx="12" cy="12" r="10" />
    <polyline points="12 6 12 12 16 14" />
  </svg>
)

const RunningIcon: React.FC = () => (
  <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
    <path d="M12 2v4M12 18v4M4.93 4.93l2.83 2.83M16.24 16.24l2.83 2.83M2 12h4M18 12h4M4.93 19.07l2.83-2.83M16.24 7.76l2.83-2.83" />
  </svg>
)

const CompletedIcon: React.FC = () => (
  <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
    <path d="M22 11.08V12a10 10 0 11-5.93-9.14" />
    <polyline points="22 4 12 14.01 9 11.01" />
  </svg>
)

const ErrorIcon: React.FC = () => (
  <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
    <circle cx="12" cy="12" r="10" />
    <line x1="15" y1="9" x2="9" y2="15" />
    <line x1="9" y1="9" x2="15" y2="15" />
  </svg>
)

const CancelledIcon: React.FC = () => (
  <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
    <circle cx="12" cy="12" r="10" />
    <line x1="4.93" y1="4.93" x2="19.07" y2="19.07" />
  </svg>
)

const TimeoutIcon: React.FC = () => (
  <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
    <circle cx="12" cy="12" r="10" />
    <polyline points="12 6 12 12 16 14" />
    <line x1="2" y1="2" x2="22" y2="22" />
  </svg>
)

const ChevronIcon: React.FC = () => (
  <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
    <polyline points="6 9 12 15 18 9" />
  </svg>
)

// Styles
const styles: Record<string, React.CSSProperties> = {
  container: {
    backgroundColor: '#161b22',
    borderRadius: '8px',
    border: '1px solid #30363d',
    borderLeftWidth: '3px',
    overflow: 'hidden',
  },
  header: {
    display: 'flex',
    alignItems: 'center',
    gap: '8px',
    padding: '10px 12px',
    backgroundColor: '#0d1117',
    cursor: 'pointer',
    userSelect: 'none',
  },
  statusIcon: {
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'center',
    flexShrink: 0,
  },
  name: {
    fontSize: '13px',
    fontWeight: 600,
    color: '#c9d1d9',
    flex: 1,
  },
  riskBadge: {
    fontSize: '10px',
    fontWeight: 500,
    textTransform: 'uppercase',
    padding: '2px 6px',
    borderRadius: '4px',
    border: '1px solid',
  },
  statusLabel: {
    fontSize: '11px',
    fontWeight: 500,
  },
  duration: {
    fontSize: '11px',
    color: '#8b949e',
    fontFamily: 'ui-monospace, SFMono-Regular, "SF Mono", Menlo, Consolas, monospace',
  },
  chevron: {
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'center',
    color: '#8b949e',
    transition: 'transform 0.2s ease',
    flexShrink: 0,
  },
  body: {
    display: 'flex',
    flexDirection: 'column',
    gap: '12px',
    padding: '12px',
  },
  progress: {
    position: 'relative',
    height: '4px',
    backgroundColor: '#30363d',
    borderRadius: '2px',
    overflow: 'hidden',
  },
  progressBar: {
    position: 'absolute',
    top: 0,
    left: 0,
    height: '100%',
    backgroundColor: '#58a6ff',
    borderRadius: '2px',
    transition: 'width 0.3s ease',
  },
  progressText: {
    position: 'absolute',
    top: '8px',
    left: 0,
    fontSize: '11px',
    color: '#8b949e',
  },
  error: {
    display: 'flex',
    alignItems: 'flex-start',
    gap: '8px',
    padding: '8px 12px',
    backgroundColor: 'rgba(218, 54, 51, 0.1)',
    border: '1px solid rgba(218, 54, 51, 0.4)',
    borderRadius: '6px',
    fontSize: '12px',
    color: '#f85149',
  },
}

export default ToolCallCard
