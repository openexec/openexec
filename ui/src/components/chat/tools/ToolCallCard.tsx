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
import {
  colors,
  typography,
  borderRadius,
  getStatusInfo,
  getRiskColor,
} from '../../../utils/theme'
import { formatDuration, getToolDisplayName } from '../../../utils/formatters'
import {
  ErrorIcon,
  ChevronDownIcon,
  renderStatusIcon,
} from '../../../utils/icons'

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

  // Get status info from centralized config
  const statusInfo = getStatusInfo(toolCall.status)
  const riskColor = getRiskColor(toolCall.riskLevel as 'low' | 'medium' | 'high' | undefined)
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
          {renderStatusIcon(toolCall.status)}
        </span>

        {/* Tool name */}
        <span className="tool-call-card__name" style={styles.name}>
          {getToolDisplayName(toolCall.toolName)}
        </span>

        {/* Risk level badge */}
        {toolCall.riskLevel && (
          <span
            className="tool-call-card__risk"
            style={{
              ...styles.riskBadge,
              backgroundColor: `${riskColor}20`,
              color: riskColor,
              borderColor: riskColor,
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
          <ChevronDownIcon />
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

// Styles - using centralized theme
const styles: Record<string, React.CSSProperties> = {
  container: {
    backgroundColor: colors.bg.secondary,
    borderRadius: borderRadius.xl,
    border: `1px solid ${colors.bg.border}`,
    borderLeftWidth: '3px',
    overflow: 'hidden',
  },
  header: {
    display: 'flex',
    alignItems: 'center',
    gap: '8px',
    padding: '10px 12px',
    backgroundColor: colors.bg.primary,
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
    fontSize: typography.fontSize.md,
    fontWeight: 600,
    color: colors.text.primary,
    flex: 1,
  },
  riskBadge: {
    fontSize: typography.fontSize.xs,
    fontWeight: 500,
    textTransform: 'uppercase',
    padding: '2px 6px',
    borderRadius: borderRadius.md,
    border: '1px solid',
  },
  statusLabel: {
    fontSize: typography.fontSize.sm,
    fontWeight: 500,
  },
  duration: {
    fontSize: typography.fontSize.sm,
    color: colors.text.secondary,
    fontFamily: typography.fontFamily.mono,
  },
  chevron: {
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'center',
    color: colors.text.secondary,
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
    backgroundColor: colors.bg.border,
    borderRadius: '2px',
    overflow: 'hidden',
  },
  progressBar: {
    position: 'absolute',
    top: 0,
    left: 0,
    height: '100%',
    backgroundColor: colors.status.info,
    borderRadius: '2px',
    transition: 'width 0.3s ease',
  },
  progressText: {
    position: 'absolute',
    top: '8px',
    left: 0,
    fontSize: typography.fontSize.sm,
    color: colors.text.secondary,
  },
  error: {
    display: 'flex',
    alignItems: 'flex-start',
    gap: '8px',
    padding: '8px 12px',
    backgroundColor: `${colors.status.error}1a`,
    border: `1px solid ${colors.status.error}66`,
    borderRadius: borderRadius.lg,
    fontSize: typography.fontSize.base,
    color: colors.status.errorText,
  },
}

export default ToolCallCard
