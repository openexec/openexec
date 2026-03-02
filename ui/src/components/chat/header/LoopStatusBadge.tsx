/**
 * LoopStatusBadge Component
 * Badge showing current loop state.
 * @module components/chat/header/LoopStatusBadge
 */
import React from 'react'
import type { AgentLoopState } from '../../../types/chat'

export interface LoopStatusBadgeProps {
  /** Current agent loop state */
  state: AgentLoopState
}

type StatusType = 'idle' | 'running' | 'paused'

const LoopStatusBadge: React.FC<LoopStatusBadgeProps> = ({ state }) => {
  // Determine status
  const getStatus = (): StatusType => {
    if (!state.isRunning) return 'idle'
    if (state.isPaused) return 'paused'
    return 'running'
  }

  const status = getStatus()

  // Get status-specific styles
  const getStatusStyles = (): React.CSSProperties => {
    switch (status) {
      case 'running':
        return {
          backgroundColor: '#238636',
          color: '#ffffff',
        }
      case 'paused':
        return {
          backgroundColor: '#f0883e',
          color: '#ffffff',
        }
      case 'idle':
      default:
        return {
          backgroundColor: '#21262d',
          color: '#8b949e',
        }
    }
  }

  // Get status label
  const getStatusLabel = (): string => {
    switch (status) {
      case 'running':
        return `Running (iter ${state.iteration})`
      case 'paused':
        return 'Paused'
      case 'idle':
      default:
        return 'Idle'
    }
  }

  return (
    <span
      className={`loop-status-badge loop-status-badge--${status}`}
      style={{
        ...styles.badge,
        ...getStatusStyles(),
      }}
    >
      {/* Status indicator dot with animation for running */}
      {status === 'running' && (
        <span className="loop-status-badge__pulse" style={styles.pulse} />
      )}

      {/* Status icon */}
      <span className="loop-status-badge__icon" style={styles.icon}>
        {status === 'running' && <PlayIcon />}
        {status === 'paused' && <PauseIcon />}
        {status === 'idle' && <IdleIcon />}
      </span>

      {/* Status text */}
      <span className="loop-status-badge__text" style={styles.text}>
        {getStatusLabel()}
      </span>
    </span>
  )
}

// Icon components
const PlayIcon: React.FC = () => (
  <svg width="10" height="10" viewBox="0 0 24 24" fill="currentColor">
    <polygon points="5 3 19 12 5 21 5 3" />
  </svg>
)

const PauseIcon: React.FC = () => (
  <svg width="10" height="10" viewBox="0 0 24 24" fill="currentColor">
    <rect x="6" y="4" width="4" height="16" />
    <rect x="14" y="4" width="4" height="16" />
  </svg>
)

const IdleIcon: React.FC = () => (
  <svg width="10" height="10" viewBox="0 0 24 24" fill="currentColor">
    <circle cx="12" cy="12" r="8" />
  </svg>
)

// Styles
const styles: Record<string, React.CSSProperties> = {
  badge: {
    display: 'inline-flex',
    alignItems: 'center',
    gap: '6px',
    padding: '4px 10px',
    borderRadius: '12px',
    fontSize: '12px',
    fontWeight: 500,
    position: 'relative',
  },
  pulse: {
    position: 'absolute',
    top: 0,
    left: 0,
    right: 0,
    bottom: 0,
    borderRadius: '12px',
    backgroundColor: '#238636',
    animation: 'pulse 2s ease-in-out infinite',
    opacity: 0.3,
  },
  icon: {
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'center',
  },
  text: {
    whiteSpace: 'nowrap',
  },
}

export default LoopStatusBadge
