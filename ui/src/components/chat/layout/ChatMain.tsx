/**
 * ChatMain Component
 *
 * Main chat area containing header, messages, and input.
 * Orchestrates the conversation view with message list and input controls.
 * Displays fork ancestry information for forked sessions.
 *
 * @module components/chat/layout/ChatMain
 */

import React from 'react'
import type { Session, Message, ToolCall, AgentLoopState, CostInfo } from '../../../types/chat'
import type { Stage, BlueprintState } from '../../../types/blueprint'
import type { ForkInfo } from '../../../hooks/useFork'
import type { AncestorSession } from '../session/ForkAncestryTree'
import ChatHeader from '../header/ChatHeader'
import MessageList from '../messages/MessageList'
import ChatInput from '../input/ChatInput'
import StageTimeline from '../blueprint/StageTimeline'

export interface ChatMainProps {
  /** Current session */
  session?: Session
  /** Messages in the conversation */
  messages: Message[]
  /** Currently streaming message */
  streamingMessage?: Partial<Message>
  /** Tool calls associated with messages */
  toolCalls?: ToolCall[]
  /** Current loop state */
  loopState?: AgentLoopState
  /** Session cost information */
  costInfo?: CostInfo
  /** Fork information for the current session */
  forkInfo?: ForkInfo
  /** Ancestor sessions for the fork tree */
  ancestorSessions?: AncestorSession[]
  /** Whether messages are loading */
  messagesLoading?: boolean
  /** Whether input is submitting */
  isSubmitting?: boolean
  /** Callback when user sends a message */
  onSendMessage?: (content: string) => void
  /** Callback to load more messages */
  onLoadMore?: () => void
  /** Callback to pause the loop */
  onPauseLoop?: () => void
  /** Callback to resume the loop */
  onResumeLoop?: () => void
  /** Callback to stop the loop */
  onStopLoop?: () => void
  /** Callback to approve a tool call */
  onApproveTool?: (toolCallId: string) => void
  /** Callback to reject a tool call */
  onRejectTool?: (toolCallId: string, reason?: string) => void
  /** Callback when session title is updated */
  onUpdateTitle?: (title: string) => void
  /** Callback when forking at a specific message */
  onForkAtMessage?: (message: Message) => void
  /** Callback when navigating to an ancestor session */
  onNavigateToSession?: (sessionId: string) => void
  /** Callback when fork is requested from header */
  onForkSession?: () => void
  /** Blueprint stages for run detail view */
  stages?: Stage[]
  /** Blueprint execution state */
  blueprintState?: BlueprintState
  /** Whether blueprint is running */
  isBlueprintRunning?: boolean
}

const ChatMain: React.FC<ChatMainProps> = ({
  session,
  messages,
  streamingMessage,
  toolCalls = [],
  loopState,
  costInfo,
  forkInfo,
  ancestorSessions,
  messagesLoading = false,
  isSubmitting = false,
  onSendMessage,
  onLoadMore,
  onPauseLoop,
  onResumeLoop,
  onStopLoop,
  onApproveTool,
  onRejectTool,
  onUpdateTitle,
  onForkAtMessage,
  onNavigateToSession,
  onForkSession,
  stages,
  blueprintState,
  isBlueprintRunning = false,
}) => {
  // Build a map of tool calls by message ID for easy lookup
  const toolCallsByMessageId = React.useMemo(() => {
    const map = new Map<string, ToolCall[]>()
    for (const tc of toolCalls) {
      const existing = map.get(tc.messageId) || []
      existing.push(tc)
      map.set(tc.messageId, existing)
    }
    return map
  }, [toolCalls])

  // Determine if we can send messages
  // We still gate the SUBMIT, but we want the input to be interactable for typing/drafting
  const canSubmitMessage = !!session && !isSubmitting && !(loopState?.isRunning && !loopState?.isPaused)

  // Get placeholder text based on state
  const getPlaceholder = () => {
    if (!session) {
      return 'Select or create a session to start chatting...'
    }
    if (loopState?.isRunning && !loopState?.isPaused) {
      return 'Waiting for assistant response...'
    }
    if (isSubmitting) {
      return 'Sending message...'
    }
    return 'Type your message... (Shift+Enter for new line)'
  }

  return (
    <div className="chat-main" style={mainStyles.container}>
      {/* Chat Header */}
      <ChatHeader
        session={session}
        loopState={loopState}
        costInfo={costInfo}
        forkInfo={forkInfo}
        ancestorSessions={ancestorSessions}
        onPauseLoop={onPauseLoop}
        onResumeLoop={onResumeLoop}
        onStopLoop={onStopLoop}
        onUpdateTitle={onUpdateTitle}
        onNavigateToSession={onNavigateToSession}
        onForkSession={onForkSession}
      />

      {/* Stage Timeline - shown for blueprint runs */}
      {stages && stages.some((s) => s.status !== 'pending') && (
        <div className="chat-main__stages" style={mainStyles.stages}>
          <StageTimeline
            stages={stages}
            blueprintState={blueprintState}
            isRunning={isBlueprintRunning}
            compact
          />
        </div>
      )}

      {/* Messages Area */}
      <div className="chat-main__messages" style={mainStyles.messages}>
        {!session ? (
          <EmptyState />
        ) : messagesLoading && messages.length === 0 ? (
          <LoadingState />
        ) : (
          <MessageList
            messages={messages}
            streamingMessage={streamingMessage}
            toolCallsByMessageId={toolCallsByMessageId}
            onLoadMore={onLoadMore}
            onApproveTool={onApproveTool}
            onRejectTool={onRejectTool}
            onForkAtMessage={onForkAtMessage}
          />
        )}
      </div>

      {/* Input Area */}
      <div className="chat-main__input" style={mainStyles.input}>
        <ChatInput
          onSubmit={onSendMessage}
          disabled={isSubmitting} // Only hard-disable while physically submitting
          placeholder={getPlaceholder()}
          isSubmitting={isSubmitting}
        />
      </div>
    </div>
  )
}

// Empty state component
const EmptyState: React.FC = () => (
  <div className="chat-main__empty" style={mainStyles.emptyState}>
    <div style={mainStyles.emptyIcon}>
      <ChatBubbleIcon />
    </div>
    <h3 style={mainStyles.emptyTitle}>No Session Selected</h3>
    <p style={mainStyles.emptyText}>
      Select an existing session from the sidebar or create a new one to start chatting.
    </p>
  </div>
)

// Loading state component
const LoadingState: React.FC = () => (
  <div className="chat-main__loading" style={mainStyles.loadingState}>
    <div style={mainStyles.spinner} />
    <p style={mainStyles.loadingText}>Loading messages...</p>
  </div>
)

// Icon component
const ChatBubbleIcon: React.FC = () => (
  <svg width="48" height="48" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.5">
    <path d="M21 11.5a8.38 8.38 0 01-.9 3.8 8.5 8.5 0 01-7.6 4.7 8.38 8.38 0 01-3.8-.9L3 21l1.9-5.7a8.38 8.38 0 01-.9-3.8 8.5 8.5 0 014.7-7.6 8.38 8.38 0 013.8-.9h.5a8.48 8.48 0 018 8v.5z" />
  </svg>
)

// Styles
const mainStyles: Record<string, React.CSSProperties> = {
  container: {
    display: 'flex',
    flexDirection: 'column',
    height: '100%',
    width: '100%',
    backgroundColor: '#0d1117',
  },
  messages: {
    flex: 1,
    overflow: 'hidden',
    display: 'flex',
    flexDirection: 'column',
  },
  input: {
    borderTop: '1px solid #30363d',
    padding: '16px',
    backgroundColor: '#0d1117',
  },
  stages: {
    padding: '12px 16px',
    borderBottom: '1px solid #30363d',
    backgroundColor: '#161b22',
    flexShrink: 0,
  },
  emptyState: {
    flex: 1,
    display: 'flex',
    flexDirection: 'column',
    alignItems: 'center',
    justifyContent: 'center',
    padding: '48px',
    textAlign: 'center',
    color: '#8b949e',
  },
  emptyIcon: {
    marginBottom: '16px',
    opacity: 0.5,
  },
  emptyTitle: {
    fontSize: '18px',
    fontWeight: 600,
    color: '#c9d1d9',
    margin: '0 0 8px 0',
  },
  emptyText: {
    fontSize: '14px',
    margin: 0,
    maxWidth: '300px',
    lineHeight: 1.5,
  },
  loadingState: {
    flex: 1,
    display: 'flex',
    flexDirection: 'column',
    alignItems: 'center',
    justifyContent: 'center',
    gap: '16px',
  },
  spinner: {
    width: '32px',
    height: '32px',
    border: '3px solid #30363d',
    borderTopColor: '#58a6ff',
    borderRadius: '50%',
    animation: 'spin 1s linear infinite',
  },
  loadingText: {
    color: '#8b949e',
    fontSize: '14px',
    margin: 0,
  },
}

export default ChatMain
