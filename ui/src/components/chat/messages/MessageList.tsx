/**
 * MessageList Component
 *
 * Scrollable container for messages with auto-scroll to bottom.
 * Supports streaming messages and tool call display.
 *
 * @module components/chat/messages/MessageList
 */

import React, { useEffect, useRef, useCallback } from 'react'
import type { Message, ToolCall } from '../../../types/chat'
import UserMessage from './UserMessage'
import AssistantMessage from './AssistantMessage'
import StreamingMessage from './StreamingMessage'
import SystemMessage from './SystemMessage'

export interface MessageListProps {
  /** List of messages to display */
  messages: Message[]
  /** Currently streaming message (partial) */
  streamingMessage?: Partial<Message>
  /** Map of tool calls by message ID */
  toolCallsByMessageId?: Map<string, ToolCall[]>
  /** Callback to load more messages (pagination) */
  onLoadMore?: () => void
  /** Callback when a tool call is approved */
  onApproveTool?: (toolCallId: string) => void
  /** Callback when a tool call is rejected */
  onRejectTool?: (toolCallId: string, reason?: string) => void
  /** Callback when forking at a specific message */
  onForkAtMessage?: (message: Message) => void
}

const MessageList: React.FC<MessageListProps> = ({
  messages,
  streamingMessage,
  toolCallsByMessageId = new Map(),
  onLoadMore,
  onApproveTool,
  onRejectTool,
  onForkAtMessage,
}) => {
  const containerRef = useRef<HTMLDivElement>(null)
  const bottomRef = useRef<HTMLDivElement>(null)
  const isAutoScrolling = useRef(true)
  const lastMessageCount = useRef(messages.length)

  // Check if user has scrolled up from bottom
  const handleScroll = useCallback(() => {
    if (!containerRef.current) return

    const { scrollTop, scrollHeight, clientHeight } = containerRef.current
    const isAtBottom = scrollHeight - scrollTop - clientHeight < 100

    isAutoScrolling.current = isAtBottom

    // Load more when scrolled to top
    if (scrollTop < 100 && onLoadMore) {
      onLoadMore()
    }
  }, [onLoadMore])

  // Auto-scroll to bottom when new messages arrive
  useEffect(() => {
    if (isAutoScrolling.current && bottomRef.current) {
      bottomRef.current.scrollIntoView({ behavior: 'smooth' })
    }
    lastMessageCount.current = messages.length
  }, [messages.length, streamingMessage?.content])

  // Scroll to bottom when streaming message appears
  useEffect(() => {
    if (streamingMessage && isAutoScrolling.current && bottomRef.current) {
      bottomRef.current.scrollIntoView({ behavior: 'smooth' })
    }
  }, [streamingMessage])

  // Group messages for display (consecutive messages from same role)
  const messageGroups = React.useMemo(() => {
    return messages
  }, [messages])

  const renderMessage = (message: Message) => {
    const toolCalls = toolCallsByMessageId.get(message.id) || message.toolCalls || []

    switch (message.role) {
      case 'user':
        return (
          <UserMessage
            key={message.id}
            message={message}
            onFork={onForkAtMessage ? () => onForkAtMessage(message) : undefined}
          />
        )
      case 'assistant':
        return (
          <AssistantMessage
            key={message.id}
            message={message}
            toolCalls={toolCalls}
            onApproveTool={onApproveTool}
            onRejectTool={onRejectTool}
            onFork={onForkAtMessage ? () => onForkAtMessage(message) : undefined}
          />
        )
      case 'system':
        return (
          <SystemMessage
            key={message.id}
            message={message}
          />
        )
      default:
        return null
    }
  }

  return (
    <div
      ref={containerRef}
      className="message-list"
      style={listStyles.container}
      onScroll={handleScroll}
    >
      <div className="message-list__content" style={listStyles.content}>
        {/* Empty state */}
        {messages.length === 0 && !streamingMessage && (
          <div className="message-list__empty" style={listStyles.empty}>
            <p style={listStyles.emptyText}>
              Start a conversation by typing a message below.
            </p>
          </div>
        )}

        {/* Rendered messages */}
        {messageGroups.map(renderMessage)}

        {/* Streaming message */}
        {streamingMessage && streamingMessage.content !== undefined && (
          <StreamingMessage
            content={streamingMessage.content}
            isStreaming={true}
          />
        )}

        {/* Bottom anchor for auto-scroll */}
        <div ref={bottomRef} className="message-list__bottom" />
      </div>
    </div>
  )
}

// Styles
const listStyles: Record<string, React.CSSProperties> = {
  container: {
    flex: 1,
    overflow: 'auto',
    scrollBehavior: 'smooth',
  },
  content: {
    display: 'flex',
    flexDirection: 'column',
    gap: '16px',
    padding: '16px',
    maxWidth: '900px',
    margin: '0 auto',
    width: '100%',
  },
  empty: {
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'center',
    minHeight: '200px',
    textAlign: 'center',
  },
  emptyText: {
    color: '#8b949e',
    fontSize: '14px',
    margin: 0,
  },
}

export default MessageList
