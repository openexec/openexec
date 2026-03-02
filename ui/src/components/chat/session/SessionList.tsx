/**
 * SessionList Component
 *
 * List of session items with loading states.
 * Supports selection and displays session previews.
 *
 * @module components/chat/session/SessionList
 */

import React, { useCallback } from 'react'
import type { SessionListItem as SessionListItemType } from '../../../types/chat'
import SessionListItem from './SessionListItem'

export interface SessionListProps {
  /** List of sessions to display */
  sessions: SessionListItemType[]
  /** Currently selected session ID */
  selectedId?: string
  /** Callback when a session is selected */
  onSelect?: (sessionId: string) => void
  /** Callback when fork action is triggered */
  onFork?: (sessionId: string) => void
  /** Callback when archive action is triggered */
  onArchive?: (sessionId: string) => void
  /** Callback when delete action is triggered */
  onDelete?: (sessionId: string) => void
  /** Whether sessions are loading */
  loading?: boolean
}

const SessionList: React.FC<SessionListProps> = ({
  sessions,
  selectedId,
  onSelect,
  onFork,
  onArchive,
  onDelete,
  loading = false,
}) => {
  const handleSelect = useCallback((sessionId: string) => {
    onSelect?.(sessionId)
  }, [onSelect])

  // Loading state
  if (loading && sessions.length === 0) {
    return (
      <div className="session-list session-list--loading" style={styles.container}>
        <div style={styles.loading}>
          <LoadingSpinner />
          <span style={styles.loadingText}>Loading sessions...</span>
        </div>
      </div>
    )
  }

  // Empty state
  if (sessions.length === 0) {
    return (
      <div className="session-list session-list--empty" style={styles.container}>
        <div style={styles.empty}>
          <EmptyIcon />
          <p style={styles.emptyText}>No sessions yet</p>
          <p style={styles.emptyHint}>Click "New" to start a conversation</p>
        </div>
      </div>
    )
  }

  // Group sessions by date
  const groupedSessions = groupSessionsByDate(sessions)

  return (
    <div className="session-list" style={styles.container}>
      {Object.entries(groupedSessions).map(([dateLabel, dateSessions]) => (
        <div key={dateLabel} className="session-list__group">
          <div style={styles.dateLabel}>{dateLabel}</div>
          {dateSessions.map((session) => (
            <SessionListItem
              key={session.id}
              session={session}
              isSelected={session.id === selectedId}
              onClick={() => handleSelect(session.id)}
              onFork={onFork}
              onArchive={onArchive}
              onDelete={onDelete}
            />
          ))}
        </div>
      ))}

      {/* Loading indicator when refreshing */}
      {loading && sessions.length > 0 && (
        <div style={styles.refreshing}>
          <LoadingSpinner size={16} />
        </div>
      )}
    </div>
  )
}

// Helper function to group sessions by date
function groupSessionsByDate(sessions: SessionListItemType[]): Record<string, SessionListItemType[]> {
  const groups: Record<string, SessionListItemType[]> = {}
  const today = new Date()
  today.setHours(0, 0, 0, 0)
  const yesterday = new Date(today)
  yesterday.setDate(yesterday.getDate() - 1)
  const lastWeek = new Date(today)
  lastWeek.setDate(lastWeek.getDate() - 7)
  const lastMonth = new Date(today)
  lastMonth.setDate(lastMonth.getDate() - 30)

  for (const session of sessions) {
    const sessionDate = new Date(session.updatedAt)
    sessionDate.setHours(0, 0, 0, 0)

    let dateLabel: string
    if (sessionDate >= today) {
      dateLabel = 'Today'
    } else if (sessionDate >= yesterday) {
      dateLabel = 'Yesterday'
    } else if (sessionDate >= lastWeek) {
      dateLabel = 'This Week'
    } else if (sessionDate >= lastMonth) {
      dateLabel = 'This Month'
    } else {
      dateLabel = 'Older'
    }

    if (!groups[dateLabel]) {
      groups[dateLabel] = []
    }
    groups[dateLabel].push(session)
  }

  return groups
}

// Loading spinner component
const LoadingSpinner: React.FC<{ size?: number }> = ({ size = 24 }) => (
  <div
    style={{
      width: size,
      height: size,
      border: `2px solid #30363d`,
      borderTopColor: '#58a6ff',
      borderRadius: '50%',
      animation: 'spin 1s linear infinite',
    }}
  />
)

// Empty state icon
const EmptyIcon: React.FC = () => (
  <svg width="32" height="32" viewBox="0 0 24 24" fill="none" stroke="#8b949e" strokeWidth="1.5">
    <path d="M21 11.5a8.38 8.38 0 01-.9 3.8 8.5 8.5 0 01-7.6 4.7 8.38 8.38 0 01-3.8-.9L3 21l1.9-5.7a8.38 8.38 0 01-.9-3.8 8.5 8.5 0 014.7-7.6 8.38 8.38 0 013.8-.9h.5a8.48 8.48 0 018 8v.5z" />
  </svg>
)

// Styles
const styles: Record<string, React.CSSProperties> = {
  container: {
    display: 'flex',
    flexDirection: 'column',
    minHeight: '100%',
  },
  loading: {
    display: 'flex',
    flexDirection: 'column',
    alignItems: 'center',
    justifyContent: 'center',
    gap: '12px',
    padding: '48px 16px',
  },
  loadingText: {
    fontSize: '13px',
    color: '#8b949e',
  },
  empty: {
    display: 'flex',
    flexDirection: 'column',
    alignItems: 'center',
    justifyContent: 'center',
    gap: '8px',
    padding: '48px 16px',
    textAlign: 'center',
  },
  emptyText: {
    fontSize: '14px',
    color: '#8b949e',
    margin: 0,
  },
  emptyHint: {
    fontSize: '12px',
    color: '#6e7681',
    margin: 0,
  },
  dateLabel: {
    fontSize: '11px',
    fontWeight: 600,
    color: '#8b949e',
    padding: '8px 16px 4px',
    textTransform: 'uppercase',
    letterSpacing: '0.5px',
  },
  refreshing: {
    display: 'flex',
    justifyContent: 'center',
    padding: '8px',
  },
}

export default SessionList
