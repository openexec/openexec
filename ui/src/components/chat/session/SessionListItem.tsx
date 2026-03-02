/**
 * SessionListItem Component
 *
 * Individual session preview in the sidebar list.
 * Shows title, model info, message preview, and metadata.
 * Includes action buttons for fork, archive, and delete operations.
 *
 * @module components/chat/session/SessionListItem
 */

import React, { useState, useCallback } from 'react'
import type { SessionListItem as SessionListItemType } from '../../../types/chat'

export interface SessionListItemProps {
  /** Session data to display */
  session: SessionListItemType
  /** Whether this session is currently selected */
  isSelected?: boolean
  /** Click handler for selection */
  onClick?: () => void
  /** Callback when fork action is triggered */
  onFork?: (sessionId: string) => void
  /** Callback when archive action is triggered */
  onArchive?: (sessionId: string) => void
  /** Callback when delete action is triggered */
  onDelete?: (sessionId: string) => void
}

const SessionListItem: React.FC<SessionListItemProps> = ({
  session,
  isSelected = false,
  onClick,
  onFork,
  onArchive,
  onDelete,
}) => {
  const [showActions, setShowActions] = useState(false)

  // Handle action button clicks without triggering selection
  const handleActionClick = useCallback(
    (e: React.MouseEvent, action: () => void) => {
      e.stopPropagation()
      action()
      setShowActions(false)
    },
    []
  )
  // Format timestamp for display
  const formatTime = (dateString: string): string => {
    const date = new Date(dateString)
    const now = new Date()
    const diffMs = now.getTime() - date.getTime()
    const diffMins = Math.floor(diffMs / 60000)
    const diffHours = Math.floor(diffMs / 3600000)
    const diffDays = Math.floor(diffMs / 86400000)

    if (diffMins < 1) return 'Just now'
    if (diffMins < 60) return `${diffMins}m ago`
    if (diffHours < 24) return `${diffHours}h ago`
    if (diffDays < 7) return `${diffDays}d ago`
    return date.toLocaleDateString([], { month: 'short', day: 'numeric' })
  }

  // Format cost for display
  const formatCost = (cost: number): string => {
    if (cost === 0) return ''
    if (cost < 0.01) return '<$0.01'
    return `$${cost.toFixed(2)}`
  }

  // Truncate message preview
  const truncatePreview = (text: string | undefined, maxLength: number): string => {
    if (!text) return ''
    if (text.length <= maxLength) return text
    return text.slice(0, maxLength).trim() + '...'
  }

  // Get model display name
  const getModelDisplay = (): string => {
    const model = session.model
    if (model.includes('claude-3-5-sonnet')) return 'Sonnet 3.5'
    if (model.includes('claude-3-opus')) return 'Opus 3'
    if (model.includes('claude-3-sonnet')) return 'Sonnet 3'
    if (model.includes('claude-3-haiku')) return 'Haiku 3'
    if (model.includes('gpt-4o')) return 'GPT-4o'
    if (model.includes('gpt-4')) return 'GPT-4'
    if (model.includes('gemini')) return 'Gemini'
    return model.length > 15 ? model.slice(0, 15) + '...' : model
  }

  // Get status color
  const getStatusColor = (): string => {
    switch (session.status) {
      case 'active': return '#238636'
      case 'paused': return '#f0883e'
      case 'archived': return '#8b949e'
      default: return '#30363d'
    }
  }

  return (
    <div
      className={`session-list-item ${isSelected ? 'session-list-item--selected' : ''}`}
      style={{
        ...styles.container,
        ...(isSelected ? styles.selected : {}),
      }}
      onClick={onClick}
      onMouseEnter={() => setShowActions(true)}
      onMouseLeave={() => setShowActions(false)}
      role="button"
      tabIndex={0}
      onKeyDown={(e) => {
        if (e.key === 'Enter' || e.key === ' ') {
          e.preventDefault()
          onClick?.()
        }
      }}
    >
      {/* Status indicator */}
      <div
        className="session-list-item__status"
        style={{
          ...styles.status,
          backgroundColor: getStatusColor(),
        }}
        title={`Status: ${session.status}`}
      />

      {/* Main content */}
      <div className="session-list-item__content" style={styles.content}>
        {/* Title row */}
        <div className="session-list-item__header" style={styles.header}>
          <span className="session-list-item__title" style={styles.title}>
            {session.title || 'Untitled Session'}
          </span>
          {/* Action buttons (show on hover) */}
          {showActions && (onFork || onArchive || onDelete) && (
            <div className="session-list-item__actions" style={styles.actions}>
              {onFork && (
                <button
                  className="session-list-item__action"
                  style={styles.actionButton}
                  onClick={(e) => handleActionClick(e, () => onFork(session.id))}
                  title="Fork session"
                  aria-label="Fork session"
                >
                  <ForkIcon />
                </button>
              )}
              {onArchive && session.status !== 'archived' && (
                <button
                  className="session-list-item__action"
                  style={styles.actionButton}
                  onClick={(e) => handleActionClick(e, () => onArchive(session.id))}
                  title="Archive session"
                  aria-label="Archive session"
                >
                  <ArchiveIcon />
                </button>
              )}
              {onDelete && (
                <button
                  className="session-list-item__action"
                  style={{ ...styles.actionButton, ...styles.deleteButton }}
                  onClick={(e) => handleActionClick(e, () => onDelete(session.id))}
                  title="Delete session"
                  aria-label="Delete session"
                >
                  <DeleteIcon />
                </button>
              )}
            </div>
          )}
          {!showActions && (
            <span className="session-list-item__time" style={styles.time}>
              {formatTime(session.updatedAt)}
            </span>
          )}
        </div>

        {/* Preview row */}
        {session.lastMessagePreview && (
          <div className="session-list-item__preview" style={styles.preview}>
            {truncatePreview(session.lastMessagePreview, 60)}
          </div>
        )}

        {/* Metadata row */}
        <div className="session-list-item__meta" style={styles.meta}>
          {/* Fork indicator badge */}
          {session.parentSessionId && (
            <span
              className="session-list-item__fork-badge"
              style={styles.forkBadge}
              title={`Forked session (Level ${session.forkDepth || 1})`}
            >
              <ForkBadgeIcon />
              {session.forkDepth && session.forkDepth > 1 && (
                <span style={styles.forkDepth}>{session.forkDepth}</span>
              )}
            </span>
          )}
          <span className="session-list-item__model" style={styles.model}>
            {getModelDisplay()}
          </span>
          <span className="session-list-item__messages" style={styles.messages}>
            {session.messageCount} {session.messageCount === 1 ? 'message' : 'messages'}
          </span>
          {session.totalCostUsd > 0 && (
            <span className="session-list-item__cost" style={styles.cost}>
              {formatCost(session.totalCostUsd)}
            </span>
          )}
        </div>
      </div>
    </div>
  )
}

// Styles
const styles: Record<string, React.CSSProperties> = {
  container: {
    display: 'flex',
    alignItems: 'flex-start',
    gap: '10px',
    padding: '10px 16px',
    cursor: 'pointer',
    borderBottom: '1px solid #21262d',
    transition: 'background-color 0.15s ease',
    backgroundColor: 'transparent',
  },
  selected: {
    backgroundColor: '#21262d',
    borderLeft: '2px solid #58a6ff',
    paddingLeft: '14px',
  },
  status: {
    width: '8px',
    height: '8px',
    borderRadius: '50%',
    flexShrink: 0,
    marginTop: '6px',
  },
  content: {
    flex: 1,
    minWidth: 0,
    display: 'flex',
    flexDirection: 'column',
    gap: '4px',
  },
  header: {
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'space-between',
    gap: '8px',
  },
  title: {
    fontSize: '13px',
    fontWeight: 500,
    color: '#c9d1d9',
    whiteSpace: 'nowrap',
    overflow: 'hidden',
    textOverflow: 'ellipsis',
    flex: 1,
  },
  time: {
    fontSize: '11px',
    color: '#6e7681',
    flexShrink: 0,
  },
  preview: {
    fontSize: '12px',
    color: '#8b949e',
    whiteSpace: 'nowrap',
    overflow: 'hidden',
    textOverflow: 'ellipsis',
    lineHeight: 1.4,
  },
  meta: {
    display: 'flex',
    alignItems: 'center',
    gap: '8px',
    marginTop: '2px',
  },
  model: {
    fontSize: '11px',
    color: '#58a6ff',
    backgroundColor: '#21262d',
    padding: '1px 6px',
    borderRadius: '3px',
  },
  messages: {
    fontSize: '11px',
    color: '#6e7681',
  },
  cost: {
    fontSize: '11px',
    color: '#8b949e',
    marginLeft: 'auto',
    fontFamily: 'ui-monospace, SFMono-Regular, "SF Mono", Menlo, Consolas, monospace',
  },
  forkBadge: {
    display: 'inline-flex',
    alignItems: 'center',
    gap: '2px',
    fontSize: '10px',
    color: '#a371f7',
    backgroundColor: 'rgba(163, 113, 247, 0.15)',
    padding: '1px 5px',
    borderRadius: '3px',
  },
  forkDepth: {
    fontSize: '9px',
    fontWeight: 600,
  },
  actions: {
    display: 'flex',
    alignItems: 'center',
    gap: '4px',
    marginLeft: 'auto',
  },
  actionButton: {
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'center',
    width: '24px',
    height: '24px',
    border: 'none',
    borderRadius: '4px',
    backgroundColor: 'transparent',
    color: '#8b949e',
    cursor: 'pointer',
    transition: 'background-color 0.15s, color 0.15s',
  },
  deleteButton: {
    color: '#da3633',
  },
}

// Icon components
const ForkIcon: React.FC = () => (
  <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
    <circle cx="12" cy="18" r="3" />
    <circle cx="6" cy="6" r="3" />
    <circle cx="18" cy="6" r="3" />
    <path d="M18 9a9 9 0 01-9 9" />
    <path d="M6 9a9 9 0 009 9" />
  </svg>
)

const ArchiveIcon: React.FC = () => (
  <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
    <polyline points="21 8 21 21 3 21 3 8" />
    <rect x="1" y="3" width="22" height="5" />
    <line x1="10" y1="12" x2="14" y2="12" />
  </svg>
)

const DeleteIcon: React.FC = () => (
  <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
    <polyline points="3 6 5 6 21 6" />
    <path d="M19 6v14a2 2 0 01-2 2H7a2 2 0 01-2-2V6m3 0V4a2 2 0 012-2h4a2 2 0 012 2v2" />
    <line x1="10" y1="11" x2="10" y2="17" />
    <line x1="14" y1="11" x2="14" y2="17" />
  </svg>
)

const ForkBadgeIcon: React.FC = () => (
  <svg width="10" height="10" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.5">
    <circle cx="12" cy="18" r="3" />
    <circle cx="6" cy="6" r="3" />
    <circle cx="18" cy="6" r="3" />
    <path d="M18 9a9 9 0 01-9 9" />
    <path d="M6 9a9 9 0 009 9" />
  </svg>
)

export default SessionListItem
