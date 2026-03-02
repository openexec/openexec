/**
 * ChatPage Component
 *
 * Main chat page that integrates all chat components with session management.
 * Connects the UI components with the useChat hook for data and actions.
 *
 * @module pages/ChatPage
 */

import React, { useEffect, useMemo, useCallback, useState } from 'react'
import {
  ChatLayout,
  ChatMain,
  SessionSidebar,
  EventPanel,
  CostPanel,
  SessionForkDialog,
} from '../components/chat'
import type { AncestorSession } from '../components/chat/session/ForkAncestryTree'
import { useChat, useFork, type ChatConfig } from '../hooks'
import type { CreateSessionParams, SessionFilters, ToolCallApproval, Session, Message } from '../types'
import type { ForkOptions, ForkResult } from '../components/chat/session/SessionForkDialog'

/**
 * ChatPage Props
 */
export interface ChatPageProps {
  /** Chat configuration */
  config: ChatConfig
}

/**
 * ChatPage Component
 *
 * Orchestrates the entire chat experience by:
 * - Managing WebSocket connections
 * - Handling session lifecycle
 * - Coordinating message flow
 * - Displaying events and costs
 */
const ChatPage: React.FC<ChatPageProps> = ({ config }) => {
  // Initialize the chat hook with configuration
  const chat = useChat(config)

  // Local state for selected project
  const [selectedProjectPath, setSelectedProjectPath] = useState<string>(() => {
    return localStorage.getItem('openexec-selected-project') || ''
  })

  // Initialize the fork hook with API configuration
  const fork = useFork({
    baseUrl: config.apiUrl,
    authToken: config.authToken,
  })

  // Fetch sessions and projects on mount
  useEffect(() => {
    chat.fetchProjects()
    chat.fetchSessions({ projectPath: selectedProjectPath })
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  // Auto-select first project if none selected
  useEffect(() => {
    if (!selectedProjectPath && chat.projects.length > 0) {
      const firstProject = chat.projects[0].path
      setSelectedProjectPath(firstProject)
      localStorage.setItem('openexec-selected-project', firstProject)
      chat.fetchSessions({ projectPath: firstProject })
    }
  }, [chat.projects, selectedProjectPath, chat])

  // Update sessions when project changes
  const handleProjectSelect = useCallback((projectPath: string) => {
    setSelectedProjectPath(projectPath)
    localStorage.setItem('openexec-selected-project', projectPath)
    chat.fetchSessions({ projectPath })
  }, [chat])

  // Fetch fork info when current session changes (if it's a forked session)
  useEffect(() => {
    if (chat.currentSession?.parentSessionId || chat.currentSession?.forkPointMessageId) {
      fork.getForkInfo(chat.currentSession.id)
    }
  }, [chat.currentSession?.id, chat.currentSession?.parentSessionId, chat.currentSession?.forkPointMessageId, fork])

  // Session sidebar handlers
  const handleSessionSelect = useCallback(
    (sessionId: string) => {
      chat.loadSession(sessionId)
    },
    [chat]
  )

  const handleNewSession = useCallback(
    (params: CreateSessionParams) => {
      chat.createSession({
        ...params,
        projectPath: selectedProjectPath || params.projectPath,
      })
    },
    [chat, selectedProjectPath]
  )

  const handleFiltersChange = useCallback(
    (filters: SessionFilters) => {
      chat.fetchSessions({
        ...filters,
        projectPath: selectedProjectPath,
      })
    },
    [chat, selectedProjectPath]
  )

  // Message handlers
  const handleSendMessage = useCallback(
    (content: string) => {
      chat.sendMessage(content)
    },
    [chat]
  )

  const handleLoadMoreMessages = useCallback(() => {
    if (chat.currentSession) {
      chat.fetchMessages(chat.currentSession.id)
    }
  }, [chat])

  // Tool call handlers
  const handleApproveTool = useCallback(
    (toolCallId: string) => {
      const approval: ToolCallApproval = {
        toolCallId,
        approved: true,
        approvedBy: 'user',
      }
      chat.approveTool(approval)
    },
    [chat]
  )

  const handleRejectTool = useCallback(
    (toolCallId: string, reason?: string) => {
      chat.rejectTool(toolCallId, reason)
    },
    [chat]
  )

  // Loop control handlers
  const handlePauseLoop = useCallback(() => {
    chat.pauseLoop()
  }, [chat])

  const handleResumeLoop = useCallback(() => {
    chat.resumeLoop()
  }, [chat])

  const handleStopLoop = useCallback(() => {
    chat.stopLoop()
  }, [chat])

  // Session title update
  const handleUpdateTitle = useCallback(
    (title: string) => {
      if (chat.currentSession) {
        chat.updateSessionTitle(chat.currentSession.id, title)
      }
    },
    [chat]
  )

  // Fork handlers
  const handleForkClick = useCallback(
    (sessionId: string) => {
      // Find the session to fork
      const sessionToFork = chat.sessions.find((s) => s.id === sessionId)
      if (sessionToFork) {
        // Convert SessionListItem to Session format for the dialog
        const session: Session = {
          id: sessionToFork.id,
          projectPath: selectedProjectPath,
          provider: sessionToFork.provider,
          model: sessionToFork.model,
          title: sessionToFork.title,
          status: sessionToFork.status,
          createdAt: sessionToFork.createdAt,
          updatedAt: sessionToFork.updatedAt,
        }
        fork.openForkDialog(session)
      }
    },
    [chat.sessions, fork, selectedProjectPath]
  )

  const handleFork = useCallback(
    async (
      sessionId: string,
      forkPointMessageId: string,
      options: ForkOptions
    ): Promise<ForkResult> => {
      const result = await fork.forkSession(sessionId, forkPointMessageId, options)
      // Refresh sessions to include the new forked session
      chat.fetchSessions({ projectPath: selectedProjectPath })
      return result
    },
    [fork, chat, selectedProjectPath]
  )

  const handleNavigateToSession = useCallback(
    (sessionId: string) => {
      chat.loadSession(sessionId)
    },
    [chat]
  )

  // Handler for forking at a specific message within the conversation
  const handleForkAtMessage = useCallback(
    (message: Message) => {
      if (chat.currentSession) {
        // Convert to full Session format for the dialog
        const session: Session = {
          id: chat.currentSession.id,
          projectPath: chat.currentSession.projectPath,
          provider: chat.currentSession.provider,
          model: chat.currentSession.model,
          title: chat.currentSession.title,
          status: chat.currentSession.status,
          createdAt: chat.currentSession.createdAt,
          updatedAt: chat.currentSession.updatedAt,
        }
        fork.openForkDialog(session, message)
      }
    },
    [chat.currentSession, fork]
  )

  // Map sessions to SessionListItem format
  const sessionListItems = useMemo(() => {
    return chat.sessions
  }, [chat.sessions])

  // Build tool calls array from hook
  const toolCalls = useMemo(() => {
    return chat.toolCalls
  }, [chat.toolCalls])

  // Build ancestor sessions for ForkAncestryTree
  const ancestorSessions = useMemo((): AncestorSession[] => {
    if (!fork.forkInfo || fork.forkInfo.ancestorChain.length === 0) {
      return []
    }

    // Build ancestor list from chain, resolving session titles where possible
    const ancestors: AncestorSession[] = fork.forkInfo.ancestorChain.map((ancestorId, index) => {
      // Try to find session info in our loaded sessions
      const sessionInfo = chat.sessions.find((s) => s.id === ancestorId)
      const isRoot = index === 0 || ancestorId === fork.forkInfo?.rootSessionId

      return {
        id: ancestorId,
        title: sessionInfo?.title || `Session ${ancestorId.slice(0, 8)}...`,
        isRoot,
        isCurrent: ancestorId === chat.currentSession?.id,
      }
    })

    // Add current session at the end if not already in the chain
    if (chat.currentSession && !ancestors.some((a) => a.id === chat.currentSession?.id)) {
      ancestors.push({
        id: chat.currentSession.id,
        title: chat.currentSession.title,
        isRoot: false,
        isCurrent: true,
      })
    }

    return ancestors
  }, [fork.forkInfo, chat.sessions, chat.currentSession])

  // Handler for forking from the header button
  const handleForkFromHeader = useCallback(() => {
    if (chat.currentSession && chat.messages.length > 0) {
      // Fork at the last message by default
      const lastMessage = chat.messages[chat.messages.length - 1]
      const session: Session = {
        id: chat.currentSession.id,
        projectPath: chat.currentSession.projectPath,
        provider: chat.currentSession.provider,
        model: chat.currentSession.model,
        title: chat.currentSession.title,
        status: chat.currentSession.status,
        createdAt: chat.currentSession.createdAt,
        updatedAt: chat.currentSession.updatedAt,
      }
      fork.openForkDialog(session, lastMessage)
    }
  }, [chat.currentSession, chat.messages, fork])

  // Session sidebar component
  const sidebar = (
    <SessionSidebar
      sessions={sessionListItems}
      projects={chat.projects}
      projectsLoading={chat.projectsLoading}
      selectedSessionId={chat.currentSession?.id}
      loading={chat.sessionsLoading}
      onSessionSelect={handleSessionSelect}
      onNewSession={handleNewSession}
      onFiltersChange={handleFiltersChange}
      onProjectSelect={handleProjectSelect}
      onFork={handleForkClick}
      defaultProvider="anthropic"
      defaultModel="claude-3-5-sonnet-20241022"
      projectPath={selectedProjectPath}
    />
  )

  // Right panel with events and cost
  const rightPanel = (
    <div style={{ display: 'flex', flexDirection: 'column', height: '100%' }}>
      <div style={{ flex: 1, overflow: 'auto', borderBottom: '1px solid #30363d' }}>
        <EventPanel
          events={chat.events}
          defaultFilters={chat.eventFilters}
          isLive={chat.loopState.isRunning}
          onFilterChange={chat.setEventFilters}
          onClear={chat.clearEvents}
        />
      </div>
      <div style={{ flexShrink: 0 }}>
        <CostPanel
          cost={chat.sessionCost}
          provider={chat.currentSession?.provider}
          model={chat.currentSession?.model}
        />
      </div>
    </div>
  )

  return (
    <>
      <ChatLayout
        sidebar={sidebar}
        rightPanel={rightPanel}
        defaultSidebarOpen={true}
        defaultRightPanelOpen={false}
      >
        <ChatMain
          session={chat.currentSession}
          messages={chat.messages}
          streamingMessage={chat.streamingMessage}
          toolCalls={toolCalls}
          loopState={chat.loopState}
          costInfo={chat.sessionCost}
          forkInfo={fork.forkInfo}
          ancestorSessions={ancestorSessions}
          messagesLoading={chat.messagesLoading}
          isSubmitting={chat.isSubmitting}
          onSendMessage={handleSendMessage}
          onLoadMore={handleLoadMoreMessages}
          onPauseLoop={handlePauseLoop}
          onResumeLoop={handleResumeLoop}
          onStopLoop={handleStopLoop}
          onApproveTool={handleApproveTool}
          onRejectTool={handleRejectTool}
          onUpdateTitle={handleUpdateTitle}
          onForkAtMessage={handleForkAtMessage}
          onNavigateToSession={handleNavigateToSession}
          onForkSession={handleForkFromHeader}
        />
      </ChatLayout>

      {/* Fork Session Dialog */}
      {fork.sessionToFork && (
        <SessionForkDialog
          session={fork.sessionToFork}
          forkPointMessage={fork.forkPointMessage}
          messages={chat.messages}
          isOpen={fork.isForkDialogOpen}
          onClose={fork.closeForkDialog}
          onFork={handleFork}
          onNavigateToSession={handleNavigateToSession}
          isLoading={fork.isForking}
        />
      )}
    </>
  )
}

export default ChatPage
