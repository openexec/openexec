/**
 * ChatActions Component
 * Action buttons for loop control (pause/resume/stop).
 * @module components/chat/header/ChatActions
 */
import React, { useCallback } from 'react'
import type { AgentLoopState } from '../../../types/chat'

export interface ChatActionsProps {
  /** Current loop state */
  loopState: AgentLoopState
  /** Callback to pause the loop */
  onPause?: () => void
  /** Callback to resume the loop */
  onResume?: () => void
  /** Callback to stop the loop */
  onStop?: () => void
}

const ChatActions: React.FC<ChatActionsProps> = ({
  loopState,
  onPause,
  onResume,
  onStop,
}) => {
  const { isRunning, isPaused } = loopState

  const handlePauseResume = useCallback(() => {
    if (isPaused) {
      onResume?.()
    } else {
      onPause?.()
    }
  }, [isPaused, onPause, onResume])

  // Don't show controls if not running
  if (!isRunning) {
    return null
  }

  return (
    <div className="chat-actions" style={styles.container}>
      {/* Pause/Resume button */}
      <button
        className="chat-actions__btn chat-actions__btn--pause"
        style={{
          ...styles.button,
          ...styles.pauseButton,
        }}
        onClick={handlePauseResume}
        title={isPaused ? 'Resume' : 'Pause'}
        aria-label={isPaused ? 'Resume' : 'Pause'}
      >
        {isPaused ? <PlayIcon /> : <PauseIcon />}
        <span style={styles.buttonText}>
          {isPaused ? 'Resume' : 'Pause'}
        </span>
      </button>

      {/* Stop button */}
      <button
        className="chat-actions__btn chat-actions__btn--stop"
        style={{
          ...styles.button,
          ...styles.stopButton,
        }}
        onClick={onStop}
        title="Stop"
        aria-label="Stop"
      >
        <StopIcon />
        <span style={styles.buttonText}>Stop</span>
      </button>
    </div>
  )
}

// Icon components
const PlayIcon: React.FC = () => (
  <svg width="14" height="14" viewBox="0 0 24 24" fill="currentColor">
    <polygon points="5 3 19 12 5 21 5 3" />
  </svg>
)

const PauseIcon: React.FC = () => (
  <svg width="14" height="14" viewBox="0 0 24 24" fill="currentColor">
    <rect x="6" y="4" width="4" height="16" />
    <rect x="14" y="4" width="4" height="16" />
  </svg>
)

const StopIcon: React.FC = () => (
  <svg width="14" height="14" viewBox="0 0 24 24" fill="currentColor">
    <rect x="4" y="4" width="16" height="16" rx="2" />
  </svg>
)

// Styles
const styles: Record<string, React.CSSProperties> = {
  container: {
    display: 'flex',
    alignItems: 'center',
    gap: '8px',
  },
  button: {
    display: 'flex',
    alignItems: 'center',
    gap: '6px',
    padding: '6px 12px',
    borderRadius: '6px',
    border: 'none',
    cursor: 'pointer',
    fontSize: '12px',
    fontWeight: 500,
    transition: 'background-color 0.15s ease',
  },
  buttonText: {
    display: 'inline',
  },
  pauseButton: {
    backgroundColor: '#21262d',
    color: '#c9d1d9',
  },
  stopButton: {
    backgroundColor: '#da3633',
    color: '#ffffff',
  },
}

export default ChatActions
