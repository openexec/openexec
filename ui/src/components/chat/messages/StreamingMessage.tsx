/**
 * StreamingMessage Component
 *
 * Displays in-progress streaming response from the assistant.
 * Shows typing indicator while actively streaming.
 *
 * @module components/chat/messages/StreamingMessage
 */

import React from 'react'
import MessageContent from './MessageContent'

export interface StreamingMessageProps {
  /** Current content being streamed */
  content: string
  /** Whether actively streaming */
  isStreaming: boolean
}

const StreamingMessage: React.FC<StreamingMessageProps> = ({ content, isStreaming }) => {
  return (
    <div className="streaming-message" style={styles.container}>
      <div className="streaming-message__header" style={styles.header}>
        <div className="streaming-message__avatar" style={styles.avatar}>
          <BotIcon />
        </div>
        <span className="streaming-message__role" style={styles.role}>Assistant</span>
        {isStreaming && (
          <span className="streaming-message__indicator" style={styles.indicator}>
            <span style={styles.dot} />
            <span style={{ ...styles.dot, animationDelay: '0.2s' }} />
            <span style={{ ...styles.dot, animationDelay: '0.4s' }} />
          </span>
        )}
      </div>

      <div className="streaming-message__body" style={styles.body}>
        <div className="streaming-message__content" style={styles.content}>
          {content ? (
            <MessageContent content={content} />
          ) : (
            <span style={styles.thinking}>Thinking...</span>
          )}
          {isStreaming && <span style={styles.cursor}>|</span>}
        </div>
      </div>
    </div>
  )
}

// Icon component
const BotIcon: React.FC = () => (
  <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
    <rect x="3" y="11" width="18" height="10" rx="2" />
    <circle cx="12" cy="5" r="2" />
    <path d="M12 7v4" />
    <circle cx="8" cy="16" r="1" fill="currentColor" />
    <circle cx="16" cy="16" r="1" fill="currentColor" />
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
    backgroundColor: '#58a6ff',
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
  indicator: {
    display: 'flex',
    gap: '4px',
    marginLeft: '8px',
  },
  dot: {
    width: '6px',
    height: '6px',
    backgroundColor: '#58a6ff',
    borderRadius: '50%',
    animation: 'pulse 1s ease-in-out infinite',
  },
  body: {
    marginLeft: '36px',
  },
  content: {
    padding: '12px 16px',
    backgroundColor: '#0d1117',
    borderRadius: '8px',
    border: '1px solid #30363d',
    position: 'relative',
  },
  thinking: {
    color: '#8b949e',
    fontStyle: 'italic',
  },
  cursor: {
    color: '#58a6ff',
    fontWeight: 'bold',
    animation: 'blink 1s step-end infinite',
  },
}

export default StreamingMessage
