/**
 * AssistantMessage Component
 *
 * Displays AI assistant response with tool calls if present.
 *
 * @module components/chat/messages/AssistantMessage
 */

import React, { useState } from 'react'
import type { Message, ToolCall } from '../../../types/chat'
import MessageContent from './MessageContent'
import ToolCallCard from '../tools/ToolCallCard'

export interface AssistantMessageProps {
  /** The message to display */
  message: Message
  /** Tool calls associated with this message */
  toolCalls?: ToolCall[]
  /** Callback when a tool call is approved */
  onApproveTool?: (toolCallId: string) => void
  /** Callback when a tool call is rejected */
  onRejectTool?: (toolCallId: string, reason?: string) => void
  /** Callback when fork at this message is requested */
  onFork?: () => void
}

const AssistantMessage: React.FC<AssistantMessageProps> = ({
  message,
  toolCalls = [],
  onApproveTool,
  onRejectTool,
  onFork,
}) => {
  const [showActions, setShowActions] = useState(false)
  const timestamp = new Date(message.createdAt).toLocaleTimeString([], {
    hour: '2-digit',
    minute: '2-digit',
  })

  // Show token/cost info if available
  const hasUsageInfo = message.tokensInput > 0 || message.tokensOutput > 0

  return (
    <div
      className="assistant-message"
      style={styles.container}
      onMouseEnter={() => setShowActions(true)}
      onMouseLeave={() => setShowActions(false)}
    >
      <div className="assistant-message__header" style={styles.header}>
        <div className="assistant-message__avatar" style={styles.avatar}>
          <BotIcon />
        </div>
        <span className="assistant-message__role" style={styles.role}>Assistant</span>
        <span className="assistant-message__time" style={styles.time}>{timestamp}</span>
        {hasUsageInfo && (
          <span className="assistant-message__usage" style={styles.usage}>
            {message.tokensInput + message.tokensOutput} tokens
            {message.costUsd > 0 && ` ($${message.costUsd.toFixed(4)})`}
          </span>
        )}
        {/* Fork action button */}
        {showActions && onFork && (
          <button
            className="assistant-message__fork"
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

      <div className="assistant-message__body" style={styles.body}>
        {/* Message content */}
        {message.content && (
          <div className="assistant-message__content" style={styles.content}>
            <MessageContent content={message.content} />
          </div>
        )}

        {/* Tool calls */}
        {toolCalls.length > 0 && (
          <div className="assistant-message__tools" style={styles.tools}>
            {toolCalls.map((tc) => (
              <ToolCallCard
                key={tc.id}
                toolCall={tc}
                onApprove={onApproveTool ? () => onApproveTool(tc.id) : undefined}
                onReject={onRejectTool ? (reason) => onRejectTool(tc.id, reason) : undefined}
              />
            ))}
          </div>
        )}
      </div>
    </div>
  )
}

// Icon components
const BotIcon: React.FC = () => (
  <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
    <rect x="3" y="11" width="18" height="10" rx="2" />
    <circle cx="12" cy="5" r="2" />
    <path d="M12 7v4" />
    <circle cx="8" cy="16" r="1" fill="currentColor" />
    <circle cx="16" cy="16" r="1" fill="currentColor" />
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
  time: {
    fontSize: '12px',
    color: '#8b949e',
  },
  usage: {
    fontSize: '11px',
    color: '#8b949e',
    backgroundColor: '#21262d',
    padding: '2px 6px',
    borderRadius: '4px',
    marginLeft: 'auto',
  },
  body: {
    marginLeft: '36px',
    display: 'flex',
    flexDirection: 'column',
    gap: '12px',
  },
  content: {
    padding: '12px 16px',
    backgroundColor: '#0d1117',
    borderRadius: '8px',
    border: '1px solid #30363d',
  },
  tools: {
    display: 'flex',
    flexDirection: 'column',
    gap: '8px',
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

export default AssistantMessage
