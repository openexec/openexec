/**
 * SystemMessage Component
 *
 * System/context injection message display.
 * Collapsible by default to reduce visual clutter.
 *
 * @module components/chat/messages/SystemMessage
 */

import React, { useState } from 'react'
import type { Message } from '../../../types/chat'

export interface SystemMessageProps {
  /** The system message to display */
  message: Message
}

const SystemMessage: React.FC<SystemMessageProps> = ({ message }) => {
  const [expanded, setExpanded] = useState(false)

  // Truncate content for preview
  const preview = message.content.slice(0, 100)
  const needsTruncation = message.content.length > 100

  const timestamp = new Date(message.createdAt).toLocaleTimeString([], {
    hour: '2-digit',
    minute: '2-digit',
  })

  return (
    <div className="system-message" style={styles.container}>
      <div className="system-message__header" style={styles.header}>
        <div className="system-message__icon" style={styles.icon}>
          <SystemIcon />
        </div>
        <span className="system-message__label" style={styles.label}>System</span>
        <span className="system-message__time" style={styles.time}>{timestamp}</span>
        {needsTruncation && (
          <button
            className="system-message__toggle"
            style={styles.toggle}
            onClick={() => setExpanded(!expanded)}
          >
            {expanded ? 'Collapse' : 'Expand'}
          </button>
        )}
      </div>

      <div className="system-message__content" style={styles.content}>
        <pre style={styles.pre}>
          {expanded || !needsTruncation ? message.content : `${preview}...`}
        </pre>
      </div>
    </div>
  )
}

// Icon component
const SystemIcon: React.FC = () => (
  <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
    <circle cx="12" cy="12" r="10" />
    <path d="M12 16v-4" />
    <path d="M12 8h.01" />
  </svg>
)

// Styles
const styles: Record<string, React.CSSProperties> = {
  container: {
    display: 'flex',
    flexDirection: 'column',
    gap: '6px',
    opacity: 0.7,
  },
  header: {
    display: 'flex',
    alignItems: 'center',
    gap: '8px',
  },
  icon: {
    width: '24px',
    height: '24px',
    borderRadius: '50%',
    backgroundColor: '#30363d',
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'center',
    color: '#8b949e',
  },
  label: {
    fontWeight: 500,
    fontSize: '12px',
    color: '#8b949e',
  },
  time: {
    fontSize: '11px',
    color: '#6e7681',
  },
  toggle: {
    marginLeft: 'auto',
    background: 'none',
    border: 'none',
    color: '#58a6ff',
    fontSize: '12px',
    cursor: 'pointer',
    padding: '2px 8px',
    borderRadius: '4px',
  },
  content: {
    marginLeft: '32px',
    padding: '8px 12px',
    backgroundColor: '#161b22',
    borderRadius: '6px',
    border: '1px solid #21262d',
  },
  pre: {
    margin: 0,
    fontFamily: 'ui-monospace, SFMono-Regular, "SF Mono", Menlo, Consolas, monospace',
    fontSize: '12px',
    color: '#8b949e',
    whiteSpace: 'pre-wrap',
    wordBreak: 'break-word',
  },
}

export default SystemMessage
