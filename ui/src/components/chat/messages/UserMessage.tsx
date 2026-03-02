/**
 * UserMessage Component
 *
 * Displays a user-submitted message with proper styling.
 *
 * @module components/chat/messages/UserMessage
 */

import React, { useState } from 'react'
import type { Message } from '../../../types/chat'
import MessageContent from './MessageContent'

export interface UserMessageProps {
  /** The message to display */
  message: Message
  /** Callback when fork at this message is requested */
  onFork?: () => void
}

const UserMessage: React.FC<UserMessageProps> = ({ message, onFork }) => {
  const [showActions, setShowActions] = useState(false)
  const timestamp = new Date(message.createdAt).toLocaleTimeString([], {
    hour: '2-digit',
    minute: '2-digit',
  })

  return (
    <div
      className="user-message"
      style={styles.container}
      onMouseEnter={() => setShowActions(true)}
      onMouseLeave={() => setShowActions(false)}
    >
      <div className="user-message__header" style={styles.header}>
        <div className="user-message__avatar" style={styles.avatar}>
          <UserIcon />
        </div>
        <span className="user-message__role" style={styles.role}>You</span>
        <span className="user-message__time" style={styles.time}>{timestamp}</span>
        {/* Fork action button */}
        {showActions && onFork && (
          <button
            className="user-message__fork"
            style={styles.forkButton}
            onClick={onFork}
            title="Fork conversation at this message"
            aria-label="Fork conversation at this message"
          >
            <ForkIcon />
            <span>Fork here</span>
          </button>
        )}
      </div>
      <div className="user-message__content" style={styles.content}>
        <MessageContent content={message.content} />
      </div>
    </div>
  )
}

// Icon components
const UserIcon: React.FC = () => (
  <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
    <path d="M20 21v-2a4 4 0 00-4-4H8a4 4 0 00-4 4v2" />
    <circle cx="12" cy="7" r="4" />
  </svg>
)

const ForkIcon: React.FC = () => (
  <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
    <circle cx="12" cy="18" r="3" />
    <circle cx="6" cy="6" r="3" />
    <circle cx="18" cy="6" r="3" />
    <path d="M18 9a9 9 0 01-9 9" />
    <path d="M6 9a9 9 0 009 9" />
  </svg>
)

// Styles
const styles: Record<string, React.CSSProperties> = {
  container: {
    display: 'flex',
    flexDirection: 'column',
    gap: '8px',
  },
  header: {
    display: 'flex',
    alignItems: 'center',
    gap: '8px',
  },
  avatar: {
    width: '28px',
    height: '28px',
    borderRadius: '50%',
    backgroundColor: '#238636',
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'center',
    color: '#ffffff',
  },
  role: {
    fontWeight: 600,
    fontSize: '14px',
    color: '#c9d1d9',
  },
  time: {
    fontSize: '12px',
    color: '#8b949e',
  },
  content: {
    marginLeft: '36px',
    padding: '12px 16px',
    backgroundColor: '#161b22',
    borderRadius: '8px',
    border: '1px solid #30363d',
  },
  forkButton: {
    display: 'flex',
    alignItems: 'center',
    gap: '4px',
    padding: '4px 8px',
    fontSize: '11px',
    fontWeight: 500,
    color: '#a371f7',
    backgroundColor: 'transparent',
    border: '1px solid #a371f7',
    borderRadius: '4px',
    cursor: 'pointer',
    marginLeft: 'auto',
    transition: 'background-color 0.15s',
  },
}

export default UserMessage
