/**
 * SendButton Component
 * Submit button with loading state for chat input.
 * @module components/chat/input/SendButton
 */
import React from 'react'

export interface SendButtonProps {
  /** Click handler */
  onClick?: () => void
  /** Whether the button is disabled */
  disabled?: boolean
  /** Whether currently loading/submitting */
  isLoading?: boolean
}

const SendButton: React.FC<SendButtonProps> = ({
  onClick,
  disabled = false,
  isLoading = false,
}) => {
  return (
    <button
      className="send-button"
      style={{
        ...styles.button,
        ...(disabled ? styles.disabled : {}),
      }}
      onClick={onClick}
      disabled={disabled || isLoading}
      title="Send message"
      aria-label="Send message"
    >
      {isLoading ? (
        <LoadingSpinner />
      ) : (
        <SendIcon />
      )}
    </button>
  )
}

// Send icon component
const SendIcon: React.FC = () => (
  <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
    <line x1="22" y1="2" x2="11" y2="13" />
    <polygon points="22 2 15 22 11 13 2 9 22 2" />
  </svg>
)

// Loading spinner component
const LoadingSpinner: React.FC = () => (
  <div
    style={{
      width: 18,
      height: 18,
      border: '2px solid #30363d',
      borderTopColor: '#58a6ff',
      borderRadius: '50%',
      animation: 'spin 1s linear infinite',
    }}
  />
)

// Styles
const styles: Record<string, React.CSSProperties> = {
  button: {
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'center',
    width: '40px',
    height: '40px',
    backgroundColor: '#238636',
    border: 'none',
    borderRadius: '6px',
    cursor: 'pointer',
    color: '#ffffff',
    flexShrink: 0,
    transition: 'background-color 0.15s ease',
  },
  disabled: {
    backgroundColor: '#21262d',
    color: '#8b949e',
    cursor: 'not-allowed',
  },
}

export default SendButton
