/**
 * EventItem Component
 *
 * Individual event display with type icon, timestamp, and expandable details.
 *
 * @module components/chat/events/EventItem
 */

import React, { useState, useCallback } from 'react'
import type { LoopEvent, EventKind } from '../../../types/chat'

export interface EventItemProps {
  /** The event to display */
  event: LoopEvent
  /** Whether the item is expanded by default */
  defaultExpanded?: boolean
}

/**
 * Format timestamp to HH:MM:SS.mmm
 */
const formatTime = (timestamp: string): string => {
  const date = new Date(timestamp)
  const hours = date.getHours().toString().padStart(2, '0')
  const minutes = date.getMinutes().toString().padStart(2, '0')
  const seconds = date.getSeconds().toString().padStart(2, '0')
  const ms = date.getMilliseconds().toString().padStart(3, '0')
  return `${hours}:${minutes}:${seconds}.${ms}`
}

/**
 * Get color for event kind
 */
const getKindColor = (kind: EventKind): string => {
  const colors: Record<EventKind, string> = {
    lifecycle: '#58a6ff',
    iteration: '#a371f7',
    llm: '#79c0ff',
    tool: '#ffa657',
    context: '#7ee787',
    message: '#c9d1d9',
    gate: '#ff7b72',
    signal: '#d2a8ff',
    cost: '#ffd33d',
    session: '#a5d6ff',
    thrashing: '#f85149',
  }
  return colors[kind] || '#8b949e'
}

/**
 * Get icon for event kind
 */
const getKindIcon = (kind: EventKind): string => {
  const icons: Record<EventKind, string> = {
    lifecycle: '●',
    iteration: '↻',
    llm: '◇',
    tool: '⚙',
    context: '◈',
    message: '◆',
    gate: '⊘',
    signal: '⚡',
    cost: '$',
    session: '▣',
    thrashing: '⚠',
  }
  return icons[kind] || '○'
}

const EventItem: React.FC<EventItemProps> = ({
  event,
  defaultExpanded = false,
}) => {
  const [isExpanded, setIsExpanded] = useState(defaultExpanded)
  const hasDetails =
    event.toolCall ||
    event.llmRequest ||
    event.cost ||
    event.context ||
    event.gate ||
    event.signal ||
    event.metadata

  const toggleExpand = useCallback(() => {
    if (hasDetails) {
      setIsExpanded((prev) => !prev)
    }
  }, [hasDetails])

  const kindColor = getKindColor(event.kind)
  const kindIcon = getKindIcon(event.kind)

  return (
    <div className="event-item" style={styles.container}>
      {/* Event header */}
      <div
        className="event-item__header"
        style={{
          ...styles.header,
          cursor: hasDetails ? 'pointer' : 'default',
        }}
        onClick={toggleExpand}
        role={hasDetails ? 'button' : undefined}
        aria-expanded={hasDetails ? isExpanded : undefined}
        tabIndex={hasDetails ? 0 : undefined}
        onKeyDown={(e) => {
          if (hasDetails && (e.key === 'Enter' || e.key === ' ')) {
            e.preventDefault()
            toggleExpand()
          }
        }}
      >
        {/* Kind icon */}
        <span
          className="event-item__icon"
          style={{ ...styles.icon, color: kindColor }}
          title={event.kind}
        >
          {kindIcon}
        </span>

        {/* Timestamp */}
        <span className="event-item__time" style={styles.time}>
          {formatTime(event.timestamp)}
        </span>

        {/* Event type */}
        <span className="event-item__type" style={styles.type}>
          {event.type}
        </span>

        {/* Iteration number if present */}
        {event.iteration !== undefined && (
          <span className="event-item__iteration" style={styles.iteration}>
            #{event.iteration}
          </span>
        )}

        {/* Message preview */}
        {event.message && (
          <span className="event-item__message" style={styles.message}>
            {event.message.length > 50
              ? `${event.message.substring(0, 50)}...`
              : event.message}
          </span>
        )}

        {/* Error indicator */}
        {event.error && (
          <span className="event-item__error-badge" style={styles.errorBadge}>
            Error
          </span>
        )}

        {/* Expand indicator */}
        {hasDetails && (
          <span className="event-item__expand" style={styles.expand}>
            {isExpanded ? '▼' : '▶'}
          </span>
        )}
      </div>

      {/* Expanded details */}
      {isExpanded && hasDetails && (
        <div className="event-item__details" style={styles.details}>
          {/* Error message */}
          {event.error && (
            <div className="event-item__error" style={styles.error}>
              <strong>Error:</strong> {event.error}
            </div>
          )}

          {/* Full message */}
          {event.message && event.message.length > 50 && (
            <div className="event-item__full-message" style={styles.fullMessage}>
              <strong>Message:</strong> {event.message}
            </div>
          )}

          {/* Tool call info */}
          {event.toolCall && (
            <div className="event-item__tool-call" style={styles.section}>
              <strong>Tool:</strong> {event.toolCall.toolName}
              <pre style={styles.pre}>
                {JSON.stringify(JSON.parse(event.toolCall.toolInput || '{}'), null, 2)}
              </pre>
            </div>
          )}

          {/* LLM request info */}
          {event.llmRequest && (
            <div className="event-item__llm" style={styles.section}>
              <strong>LLM Request:</strong>
              <div style={styles.grid}>
                <span>Provider: {event.llmRequest.provider}</span>
                <span>Model: {event.llmRequest.model}</span>
                <span>Input tokens: {event.llmRequest.inputTokens.toLocaleString()}</span>
                <span>Output tokens: {event.llmRequest.outputTokens.toLocaleString()}</span>
                <span>Cost: ${event.llmRequest.costUsd.toFixed(4)}</span>
                <span>Duration: {event.llmRequest.durationMs}ms</span>
              </div>
            </div>
          )}

          {/* Cost info */}
          {event.cost && (
            <div className="event-item__cost" style={styles.section}>
              <strong>Cost Update:</strong>
              <div style={styles.grid}>
                <span>Session total: ${event.cost.sessionTotal.toFixed(4)}</span>
                <span>Iteration cost: ${event.cost.iterationCost.toFixed(4)}</span>
                {event.cost.budgetLimit && (
                  <span>Budget: ${event.cost.budgetRemaining?.toFixed(2)} remaining</span>
                )}
              </div>
            </div>
          )}

          {/* Context info */}
          {event.context && (
            <div className="event-item__context" style={styles.section}>
              <strong>Context:</strong>
              <div style={styles.grid}>
                <span>Tokens: {event.context.tokenCount.toLocaleString()} / {event.context.maxTokens.toLocaleString()}</span>
                <span>Usage: {event.context.usagePercent.toFixed(1)}%</span>
                {event.context.wasTruncated && <span>Truncated: {event.context.truncatedTokens} tokens</span>}
                {event.context.wasSummarized && <span>Summarized</span>}
              </div>
            </div>
          )}

          {/* Gate info */}
          {event.gate && (
            <div className="event-item__gate" style={styles.section}>
              <strong>Gate: {event.gate.gateName}</strong>
              <div style={styles.grid}>
                <span>Status: {event.gate.passed ? 'Passed' : 'Failed'}</span>
                {event.gate.message && <span>{event.gate.message}</span>}
                {event.gate.fixAttempt !== undefined && (
                  <span>Fix attempt: {event.gate.fixAttempt} / {event.gate.maxFixAttempts}</span>
                )}
              </div>
            </div>
          )}

          {/* Signal info */}
          {event.signal && (
            <div className="event-item__signal" style={styles.section}>
              <strong>Signal: {event.signal.signalType}</strong>
              {event.signal.target && <div>Target: {event.signal.target}</div>}
              {event.signal.reason && <div>Reason: {event.signal.reason}</div>}
            </div>
          )}

          {/* Metadata */}
          {event.metadata && Object.keys(event.metadata).length > 0 && (
            <div className="event-item__metadata" style={styles.section}>
              <strong>Metadata:</strong>
              <pre style={styles.pre}>
                {JSON.stringify(event.metadata, null, 2)}
              </pre>
            </div>
          )}
        </div>
      )}
    </div>
  )
}

// Styles
const styles: Record<string, React.CSSProperties> = {
  container: {
    borderBottom: '1px solid #21262d',
    fontSize: '12px',
  },
  header: {
    display: 'flex',
    alignItems: 'center',
    gap: '8px',
    padding: '6px 8px',
    minHeight: '28px',
  },
  icon: {
    fontFamily: 'ui-monospace, SFMono-Regular, "SF Mono", Menlo, Consolas, monospace',
    fontSize: '12px',
    width: '16px',
    textAlign: 'center',
    flexShrink: 0,
  },
  time: {
    fontFamily: 'ui-monospace, SFMono-Regular, "SF Mono", Menlo, Consolas, monospace',
    fontSize: '10px',
    color: '#6e7681',
    flexShrink: 0,
  },
  type: {
    color: '#c9d1d9',
    fontWeight: 500,
    flexShrink: 0,
  },
  iteration: {
    color: '#8b949e',
    fontSize: '10px',
    backgroundColor: '#21262d',
    padding: '1px 4px',
    borderRadius: '3px',
    flexShrink: 0,
  },
  message: {
    color: '#8b949e',
    flex: 1,
    overflow: 'hidden',
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
  },
  errorBadge: {
    color: '#f85149',
    fontSize: '10px',
    fontWeight: 600,
    backgroundColor: 'rgba(248, 81, 73, 0.15)',
    padding: '1px 4px',
    borderRadius: '3px',
    flexShrink: 0,
  },
  expand: {
    color: '#6e7681',
    fontSize: '8px',
    marginLeft: 'auto',
    flexShrink: 0,
  },
  details: {
    padding: '8px 8px 12px 32px',
    backgroundColor: '#0d1117',
    borderTop: '1px solid #21262d',
  },
  error: {
    color: '#f85149',
    marginBottom: '8px',
    padding: '6px 8px',
    backgroundColor: 'rgba(248, 81, 73, 0.1)',
    borderRadius: '4px',
    fontSize: '11px',
  },
  fullMessage: {
    color: '#c9d1d9',
    marginBottom: '8px',
    fontSize: '11px',
  },
  section: {
    marginBottom: '8px',
    color: '#c9d1d9',
    fontSize: '11px',
  },
  grid: {
    display: 'grid',
    gridTemplateColumns: 'repeat(auto-fit, minmax(150px, 1fr))',
    gap: '4px',
    marginTop: '4px',
    color: '#8b949e',
    fontSize: '11px',
  },
  pre: {
    margin: '4px 0 0 0',
    padding: '8px',
    backgroundColor: '#161b22',
    borderRadius: '4px',
    overflow: 'auto',
    maxHeight: '200px',
    fontSize: '10px',
    color: '#c9d1d9',
    fontFamily: 'ui-monospace, SFMono-Regular, "SF Mono", Menlo, Consolas, monospace',
  },
}

export default EventItem
