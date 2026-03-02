/**
 * ForkIntegration Component
 *
 * Container component that demonstrates the complete integration of fork functionality
 * into the chat UI. Connects the ForkProvider, MessageList, ChatHeader, and SessionList
 * to enable forking sessions at any message point.
 *
 * This component serves as both:
 * 1. A reference implementation for fork UI integration
 * 2. A ready-to-use integration layer for the OpenExec chat interface
 *
 * @module components/chat/session/ForkIntegration
 */

import React, { useCallback, useEffect, useMemo } from 'react'
import type { Session, Message, ProviderInfo, CostInfo, AgentLoopState, ToolCall } from '../../../types/chat'
import type { AncestorSession } from './ForkAncestryTree'
import type { ForkResult } from './SessionForkDialog'
import { ForkProvider, useForkContext } from './ForkContext'
import ChatMain from '../layout/ChatMain'

// =============================================================================
// Integration Props
// =============================================================================

export interface ForkIntegrationProps {
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
  /** Whether messages are loading */
  messagesLoading?: boolean
  /** Whether input is submitting */
  isSubmitting?: boolean
  /** Available LLM providers */
  providers?: ProviderInfo[]
  /** Base URL for API requests */
  apiBaseUrl: string
  /** Optional auth token */
  authToken?: string
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
  /** Callback when navigating to a different session */
  onNavigateToSession?: (sessionId: string) => void
  /** Callback when a fork is completed */
  onForkComplete?: (result: ForkResult) => void
}

// =============================================================================
// Internal Component (Within Fork Context)
// =============================================================================

interface ForkIntegrationInnerProps extends Omit<ForkIntegrationProps, 'apiBaseUrl' | 'authToken' | 'onForkComplete'> {
  /** Callback when navigating to a session (passed through) */
  onNavigateToSession?: (sessionId: string) => void
}

/**
 * Inner component that uses the fork context
 */
const ForkIntegrationInner: React.FC<ForkIntegrationInnerProps> = ({
  session,
  messages,
  streamingMessage,
  toolCalls = [],
  loopState,
  costInfo,
  messagesLoading = false,
  isSubmitting = false,
  // providers is available for future model picker integration
  providers: _providers = [],
  onSendMessage,
  onLoadMore,
  onPauseLoop,
  onResumeLoop,
  onStopLoop,
  onApproveTool,
  onRejectTool,
  onUpdateTitle,
  onNavigateToSession,
}) => {
  // Access the fork context
  // sessionForks is available for displaying child forks in the UI
  const { openForkDialog, getForkInfo, forkInfo, listSessionForks, sessionForks: _sessionForks } = useForkContext()

  // Fetch fork info when session changes
  useEffect(() => {
    if (session?.id) {
      getForkInfo(session.id).catch(() => {
        // Silently ignore errors for non-forked sessions
      })
    }
  }, [session?.id, getForkInfo])

  // Fetch session forks when session changes
  useEffect(() => {
    if (session?.id) {
      listSessionForks(session.id).catch(() => {
        // Silently ignore errors
      })
    }
  }, [session?.id, listSessionForks])

  // Handle forking at a specific message
  const handleForkAtMessage = useCallback(
    (message: Message) => {
      if (session) {
        openForkDialog(session, message)
      }
    },
    [session, openForkDialog]
  )

  // Handle forking from header (latest message)
  const handleForkSession = useCallback(() => {
    if (session) {
      // Fork at the latest message, or open dialog without a specific point
      const latestMessage = messages[messages.length - 1]
      openForkDialog(session, latestMessage)
    }
  }, [session, messages, openForkDialog])

  // Build ancestor sessions for the fork tree
  const ancestorSessions = useMemo<AncestorSession[]>(() => {
    if (!forkInfo || !forkInfo.ancestorChain || forkInfo.ancestorChain.length === 0) {
      return []
    }

    // Build ancestor list from chain
    // Note: In a real implementation, you would fetch session titles
    // For now, we create placeholders
    return forkInfo.ancestorChain.map((ancestorId, index) => ({
      id: ancestorId,
      title: index === 0 ? 'Root Session' : `Ancestor ${index}`,
      isRoot: index === 0,
      isCurrent: ancestorId === session?.id,
    }))
  }, [forkInfo, session?.id])

  return (
    <ChatMain
      session={session}
      messages={messages}
      streamingMessage={streamingMessage}
      toolCalls={toolCalls}
      loopState={loopState}
      costInfo={costInfo}
      forkInfo={forkInfo}
      ancestorSessions={ancestorSessions}
      messagesLoading={messagesLoading}
      isSubmitting={isSubmitting}
      onSendMessage={onSendMessage}
      onLoadMore={onLoadMore}
      onPauseLoop={onPauseLoop}
      onResumeLoop={onResumeLoop}
      onStopLoop={onStopLoop}
      onApproveTool={onApproveTool}
      onRejectTool={onRejectTool}
      onUpdateTitle={onUpdateTitle}
      onForkAtMessage={handleForkAtMessage}
      onNavigateToSession={onNavigateToSession}
      onForkSession={handleForkSession}
    />
  )
}

// =============================================================================
// Main Integration Component
// =============================================================================

/**
 * ForkIntegration - Complete fork UI integration
 *
 * Wraps the chat main area with fork functionality:
 * - Fork dialog accessible from message hover or header
 * - Fork ancestry tree in header for forked sessions
 * - Fork indicators and navigation
 *
 * @example
 * ```tsx
 * <ForkIntegration
 *   session={currentSession}
 *   messages={messages}
 *   apiBaseUrl="http://localhost:8080"
 *   providers={providers}
 *   onNavigateToSession={(id) => router.push(`/session/${id}`)}
 *   onForkComplete={(result) => {
 *     console.log('Created fork:', result.forkedSessionId)
 *   }}
 * />
 * ```
 */
const ForkIntegration: React.FC<ForkIntegrationProps> = ({
  apiBaseUrl,
  authToken,
  providers = [],
  messages,
  onNavigateToSession,
  onForkComplete,
  ...chatMainProps
}) => {
  return (
    <ForkProvider
      apiBaseUrl={apiBaseUrl}
      authToken={authToken}
      providers={providers}
      messages={messages}
      onForkComplete={onForkComplete}
      onNavigateToSession={onNavigateToSession}
    >
      <ForkIntegrationInner
        messages={messages}
        providers={providers}
        onNavigateToSession={onNavigateToSession}
        {...chatMainProps}
      />
    </ForkProvider>
  )
}

// =============================================================================
// Hook for Fork Actions (for external components)
// =============================================================================

/**
 * useForkActions - Hook for accessing fork actions from outside the integration
 *
 * Provides simplified fork actions for use in sidebar, menus, etc.
 *
 * @example
 * ```tsx
 * function SessionMenuItem({ session }: { session: Session }) {
 *   const { canFork, forkSession } = useForkActions()
 *
 *   return (
 *     <button onClick={() => forkSession(session)} disabled={!canFork}>
 *       Fork Session
 *     </button>
 *   )
 * }
 * ```
 */
export function useForkActions() {
  const context = useForkContext()

  const forkSession = useCallback(
    (session: Session, atMessage?: Message) => {
      context.openForkDialog(session, atMessage)
    },
    [context.openForkDialog]
  )

  return {
    /** Whether fork functionality is available */
    canFork: true,
    /** Whether a fork operation is in progress */
    isForking: context.isForking,
    /** Open the fork dialog for a session */
    forkSession,
    /** Close the fork dialog */
    closeForkDialog: context.closeForkDialog,
    /** Last fork result (if any) */
    lastForkResult: context.lastForkResult,
    /** Fork error (if any) */
    forkError: context.forkError,
    /** Clear fork error */
    clearForkError: context.clearForkError,
  }
}

// =============================================================================
// Session List Fork Handler Component
// =============================================================================

/** Session item for the fork handler */
interface SessionItem {
  id: string
  title: string
  provider: string
  model: string
}

export interface SessionListForkHandlerProps {
  /** Sessions to display */
  sessions: SessionItem[]
  /** Selected session ID */
  selectedSessionId?: string
  /** Callback when session is selected */
  onSelectSession: (sessionId: string) => void
  /** Render function for the session list */
  children: (props: {
    sessions: SessionItem[]
    onFork: (sessionId: string) => void
    onSelectSession: (sessionId: string) => void
  }) => React.ReactNode
}

/**
 * SessionListForkHandler - Provides fork action handler for session lists
 *
 * Use this to add fork functionality to custom session list implementations.
 *
 * @example
 * ```tsx
 * <SessionListForkHandler
 *   sessions={sessions}
 *   onSelectSession={(id) => setSelectedId(id)}
 * >
 *   {({ sessions, onFork, onSelectSession }) => (
 *     <ul>
 *       {sessions.map(session => (
 *         <li key={session.id}>
 *           <button onClick={() => onSelectSession(session.id)}>
 *             {session.title}
 *           </button>
 *           <button onClick={() => onFork(session.id)}>Fork</button>
 *         </li>
 *       ))}
 *     </ul>
 *   )}
 * </SessionListForkHandler>
 * ```
 */
export const SessionListForkHandler: React.FC<SessionListForkHandlerProps> = ({
  sessions,
  // selectedSessionId is available for future use in highlighting
  selectedSessionId: _selectedSessionId,
  onSelectSession,
  children,
}) => {
  const { openForkDialog } = useForkContext()

  const handleFork = useCallback(
    (sessionId: string) => {
      const session = sessions.find((s) => s.id === sessionId)
      if (session) {
        // Create a minimal session object for the dialog
        const sessionObj: Session = {
          id: session.id,
          title: session.title,
          provider: session.provider,
          model: session.model,
          projectPath: '',
          status: 'active',
          createdAt: new Date().toISOString(),
          updatedAt: new Date().toISOString(),
        }
        openForkDialog(sessionObj)
      }
    },
    [sessions, openForkDialog]
  )

  return <>{children({ sessions, onFork: handleFork, onSelectSession })}</>
}

export default ForkIntegration
