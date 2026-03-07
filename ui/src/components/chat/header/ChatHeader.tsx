/**
 * ChatHeader Component
 * Header bar showing session info and loop controls.
 * Shows fork indicator and ancestry for forked sessions.
 * @module components/chat/header/ChatHeader
 */
import React, { useState } from 'react'
import type { Session, AgentLoopState, CostInfo } from '../../../types/chat'
import type { ForkInfo } from '../../../hooks/useFork'
import type { AncestorSession } from '../session/ForkAncestryTree'
import SessionTitle from './SessionTitle'
import ModelIndicator from './ModelIndicator'
import LoopStatusBadge from './LoopStatusBadge'
import ChatActions from './ChatActions'
import ForkAncestryTree from '../session/ForkAncestryTree'

export interface ChatHeaderProps {
  /** Current session */
  session?: Session
  /** Current loop state */
  loopState?: AgentLoopState
  /** Cost information */
  costInfo?: CostInfo
  /** Fork information for the current session */
  forkInfo?: ForkInfo
  /** Ancestor sessions for the fork tree */
  ancestorSessions?: AncestorSession[]
  /** Callback to pause the loop */
  onPauseLoop?: () => void
  /** Callback to resume the loop */
  onResumeLoop?: () => void
  /** Callback to stop the loop */
  onStopLoop?: () => void
  /** Callback when session title is updated */
  onUpdateTitle?: (title: string) => void
  /** Callback when navigating to an ancestor session */
  onNavigateToSession?: (sessionId: string) => void
  /** Callback when fork is requested */
  onForkSession?: () => void
}

const ChatHeader: React.FC<ChatHeaderProps> = ({
  session,
  loopState,
  costInfo,
  forkInfo,
  ancestorSessions = [],
  onPauseLoop,
  onResumeLoop,
  onStopLoop,
  onUpdateTitle,
  onNavigateToSession,
  onForkSession,
}) => {
  const [showAncestryPopover, setShowAncestryPopover] = useState(false)

  // Default loop state if not provided
  const defaultLoopState: AgentLoopState = {
    iteration: 0,
    totalTokens: 0,
    totalCostUsd: 0,
    isRunning: false,
    isPaused: false,
    iterationsSinceProgress: 0,
  }

  const currentLoopState = loopState || defaultLoopState

  // Determine model status based on loop state
  const getModelStatus = (): 'ready' | 'busy' | 'error' => {
    if (!currentLoopState.isRunning) return 'ready'
    if (currentLoopState.isPaused) return 'ready'
    return 'busy'
  }

  // Check if current session is a fork
  const isForkedSession = !!(session?.parentSessionId || (forkInfo && forkInfo.forkDepth > 0))
  const forkDepth = forkInfo?.forkDepth || (session?.parentSessionId ? 1 : 0)

  return (
    <header className="chat-header" style={styles.container}>
      <div className="chat-header__left" style={styles.left}>
        {session ? (
          <>
            {/* Fork indicator badge - shown before title for forked sessions */}
            {isForkedSession && (
              <div
                className="chat-header__fork-indicator"
                style={styles.forkIndicator}
              >
                <button
                  className="chat-header__fork-badge"
                  style={styles.forkBadge}
                  onClick={() => setShowAncestryPopover(!showAncestryPopover)}
                  title={`Forked session - Level ${forkDepth}`}
                  aria-label={`Forked session at depth ${forkDepth}. Click to view ancestry.`}
                >
                  <ForkIcon />
                  <span style={styles.forkDepthText}>Level {forkDepth}</span>
                </button>

                {/* Fork ancestry popover */}
                {showAncestryPopover && ancestorSessions.length > 0 && (
                  <div 
                    className="chat-header__ancestry-popover" 
                    style={styles.ancestryPopover}
                    onMouseLeave={() => setShowAncestryPopover(false)}
                  >
                    <ForkAncestryTree
                      ancestors={ancestorSessions}
                      currentSessionId={session.id}
                      forkDepth={forkDepth}
                      onNavigateToSession={onNavigateToSession}
                    />
                  </div>
                )}
              </div>
            )}

            <SessionTitle
              title={session.title}
              onUpdate={onUpdateTitle}
            />
            <ModelIndicator
              provider={session.provider}
              model={session.model}
              status={getModelStatus()}
            />
          </>
        ) : (
          <span style={styles.placeholder}>No session selected</span>
        )}
      </div>

      <div className="chat-header__center" style={styles.center}>
        {session && (
          <LoopStatusBadge state={currentLoopState} />
        )}
      </div>

      <div className="chat-header__right" style={styles.right}>
        {/* Fork session button */}
        {session && onForkSession && (
          <button
            className="chat-header__fork-button"
            style={styles.forkButton}
            onClick={onForkSession}
            title="Fork this session"
            aria-label="Fork this session"
          >
            <ForkIcon />
            <span>Fork</span>
          </button>
        )}

        {/* Cost display */}
        {costInfo && (
          <div className="chat-header__cost" style={styles.cost}>
            <span style={styles.costLabel}>Cost:</span>
            <span style={styles.costValue}>
              ${costInfo.sessionTotal.toFixed(4)}
            </span>
            {costInfo.budgetLimit && (
              <span style={styles.costBudget}>
                / ${costInfo.budgetLimit.toFixed(2)}
              </span>
            )}
          </div>
        )}

        {/* Loop controls */}
        {session && (
          <ChatActions
            loopState={currentLoopState}
            onPause={onPauseLoop}
            onResume={onResumeLoop}
            onStop={onStopLoop}
          />
        )}
      </div>
    </header>
  )
}

// Fork icon component
const ForkIcon: React.FC = () => (
  <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
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
    alignItems: 'center',
    justifyContent: 'space-between',
    padding: '12px 16px',
    borderBottom: '1px solid #30363d',
    backgroundColor: '#161b22',
    minHeight: '56px',
    gap: '16px',
  },
  left: {
    display: 'flex',
    alignItems: 'center',
    gap: '12px',
    flex: '1 1 auto',
    minWidth: 0,
  },
  center: {
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'center',
    flex: '0 0 auto',
  },
  right: {
    display: 'flex',
    alignItems: 'center',
    gap: '16px',
    flex: '0 0 auto',
  },
  placeholder: {
    color: '#8b949e',
    fontSize: '14px',
    fontStyle: 'italic',
  },
  cost: {
    display: 'flex',
    alignItems: 'center',
    gap: '4px',
    fontSize: '12px',
    backgroundColor: '#21262d',
    padding: '4px 8px',
    borderRadius: '4px',
  },
  costLabel: {
    color: '#8b949e',
  },
  costValue: {
    color: '#58a6ff',
    fontWeight: 500,
    fontFamily: 'ui-monospace, SFMono-Regular, "SF Mono", Menlo, Consolas, monospace',
  },
  costBudget: {
    color: '#8b949e',
    fontFamily: 'ui-monospace, SFMono-Regular, "SF Mono", Menlo, Consolas, monospace',
  },
  forkIndicator: {
    position: 'relative',
  },
  forkBadge: {
    display: 'inline-flex',
    alignItems: 'center',
    gap: '6px',
    padding: '4px 10px',
    fontSize: '12px',
    fontWeight: 500,
    color: '#a371f7',
    backgroundColor: 'rgba(163, 113, 247, 0.15)',
    border: '1px solid rgba(163, 113, 247, 0.4)',
    borderRadius: '6px',
    cursor: 'pointer',
    transition: 'background-color 0.15s, border-color 0.15s',
  },
  forkDepthText: {
    fontSize: '11px',
    fontWeight: 600,
  },
  ancestryPopover: {
    position: 'absolute',
    top: '100%',
    left: 0,
    marginTop: '8px',
    zIndex: 100,
    minWidth: '280px',
    maxWidth: '360px',
    boxShadow: '0 8px 24px rgba(0, 0, 0, 0.4)',
  },
  forkButton: {
    display: 'inline-flex',
    alignItems: 'center',
    gap: '6px',
    padding: '6px 12px',
    fontSize: '12px',
    fontWeight: 500,
    color: '#a371f7',
    backgroundColor: 'transparent',
    border: '1px solid #a371f7',
    borderRadius: '6px',
    cursor: 'pointer',
    transition: 'background-color 0.15s',
  },
}

export default ChatHeader
