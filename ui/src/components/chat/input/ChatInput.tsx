/**
 * ChatInput Component
 * Main message input area with submit functionality.
 * @module components/chat/input/ChatInput
 */
import React, { useState, useCallback } from 'react'
import InputTextarea from './InputTextarea'
import SendButton from './SendButton'
import InputToolbar from './InputToolbar'

export interface ChatInputProps {
  /** Submit callback */
  onSubmit?: (content: string) => void
  /** Whether input is disabled */
  disabled?: boolean
  /** Placeholder text */
  placeholder?: string
  /** Whether currently submitting */
  isSubmitting?: boolean
}

const ChatInput: React.FC<ChatInputProps> = ({
  onSubmit,
  disabled = false,
  placeholder = 'Type your message... (Shift+Enter for new line)',
  isSubmitting = false,
}) => {
  const [content, setContent] = useState('')

  // Handle submit
  const handleSubmit = useCallback(() => {
    const trimmed = content.trim()
    if (trimmed && !disabled && !isSubmitting) {
      onSubmit?.(trimmed)
      setContent('')
    }
  }, [content, disabled, isSubmitting, onSubmit])

  // Handle content change
  const handleChange = useCallback((value: string) => {
    setContent(value)
  }, [])

  // Can submit check
  const canSubmit = content.trim().length > 0 && !disabled && !isSubmitting

  return (
    <div className="chat-input" style={styles.container}>
      {/* Toolbar (optional future features) */}
      <InputToolbar />

      {/* Main input area */}
      <div className="chat-input__main" style={styles.main}>
        <InputTextarea
          value={content}
          onChange={handleChange}
          onSubmit={handleSubmit}
          placeholder={placeholder}
          disabled={disabled || isSubmitting}
          minRows={1}
          maxRows={6}
        />

        <SendButton
          onClick={handleSubmit}
          disabled={!canSubmit}
          isLoading={isSubmitting}
        />
      </div>

      {/* Help text */}
      <div className="chat-input__help" style={styles.help}>
        <span style={styles.helpText}>
          Press <kbd style={styles.kbd}>Enter</kbd> to send, <kbd style={styles.kbd}>Shift+Enter</kbd> for new line
        </span>
      </div>
    </div>
  )
}

// Styles
const styles: Record<string, React.CSSProperties> = {
  container: {
    display: 'flex',
    flexDirection: 'column',
    gap: '8px',
    maxWidth: '900px',
    margin: '0 auto',
    width: '100%',
  },
  main: {
    display: 'flex',
    alignItems: 'flex-end',
    gap: '8px',
  },
  help: {
    display: 'flex',
    justifyContent: 'flex-end',
    paddingRight: '48px',
  },
  helpText: {
    fontSize: '11px',
    color: '#6e7681',
  },
  kbd: {
    backgroundColor: '#21262d',
    border: '1px solid #30363d',
    borderRadius: '3px',
    padding: '1px 4px',
    fontSize: '10px',
    fontFamily: 'ui-monospace, SFMono-Regular, "SF Mono", Menlo, Consolas, monospace',
  },
}

export default ChatInput
