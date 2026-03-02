/**
 * NewSessionButton Component
 *
 * Button to create a new chat session.
 * Includes plus icon and styled appearance.
 *
 * @module components/chat/session/NewSessionButton
 */

import React from 'react'

export interface NewSessionButtonProps {
  /** Click handler to initiate new session creation */
  onClick?: () => void
  /** Whether the button is disabled */
  disabled?: boolean
  /** Optional custom label */
  label?: string
}

const NewSessionButton: React.FC<NewSessionButtonProps> = ({
  onClick,
  disabled = false,
  label = 'New',
}) => {
  return (
    <button
      className="new-session-button"
      style={{
        ...styles.button,
        ...(disabled ? styles.disabled : {}),
      }}
      onClick={onClick}
      disabled={disabled}
      title="Create new session"
      aria-label="Create new session"
    >
      <PlusIcon />
      <span style={styles.label}>{label}</span>
    </button>
  )
}

// Plus icon component
const PlusIcon: React.FC = () => (
  <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.5">
    <line x1="12" y1="5" x2="12" y2="19" />
    <line x1="5" y1="12" x2="19" y2="12" />
  </svg>
)

// Styles
const styles: Record<string, React.CSSProperties> = {
  button: {
    display: 'flex',
    alignItems: 'center',
    gap: '6px',
    padding: '6px 12px',
    fontSize: '12px',
    fontWeight: 500,
    color: '#ffffff',
    backgroundColor: '#238636',
    border: 'none',
    borderRadius: '6px',
    cursor: 'pointer',
    transition: 'background-color 0.15s ease',
  },
  disabled: {
    backgroundColor: '#21262d',
    color: '#8b949e',
    cursor: 'not-allowed',
  },
  label: {
    lineHeight: 1,
  },
}

export default NewSessionButton
