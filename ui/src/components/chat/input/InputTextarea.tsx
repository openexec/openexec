/**
 * InputTextarea Component
 * Auto-resizing textarea for message input.
 * @module components/chat/input/InputTextarea
 */
import React, { useRef, useEffect, useCallback } from 'react'

export interface InputTextareaProps {
  /** Current value */
  value: string
  /** Change callback */
  onChange?: (value: string) => void
  /** Submit callback (Enter without Shift) */
  onSubmit?: () => void
  /** Placeholder text */
  placeholder?: string
  /** Whether input is disabled */
  disabled?: boolean
  /** Minimum rows */
  minRows?: number
  /** Maximum rows */
  maxRows?: number
}

const InputTextarea: React.FC<InputTextareaProps> = ({
  value,
  onChange,
  onSubmit,
  placeholder,
  disabled = false,
  minRows = 1,
  maxRows = 10,
}) => {
  const textareaRef = useRef<HTMLTextAreaElement>(null)

  // Auto-resize textarea based on content
  const adjustHeight = useCallback(() => {
    const textarea = textareaRef.current
    if (!textarea) return

    // Reset height to calculate scroll height
    textarea.style.height = 'auto'

    // Calculate line height from computed style
    const computedStyle = window.getComputedStyle(textarea)
    const lineHeight = parseInt(computedStyle.lineHeight, 10) || 20
    const paddingTop = parseInt(computedStyle.paddingTop, 10) || 0
    const paddingBottom = parseInt(computedStyle.paddingBottom, 10) || 0
    const borderTop = parseInt(computedStyle.borderTopWidth, 10) || 0
    const borderBottom = parseInt(computedStyle.borderBottomWidth, 10) || 0

    const minHeight = lineHeight * minRows + paddingTop + paddingBottom + borderTop + borderBottom
    const maxHeight = lineHeight * maxRows + paddingTop + paddingBottom + borderTop + borderBottom

    // Set new height within bounds
    const newHeight = Math.min(Math.max(textarea.scrollHeight, minHeight), maxHeight)
    textarea.style.height = `${newHeight}px`
  }, [minRows, maxRows])

  // Adjust height when value changes
  useEffect(() => {
    adjustHeight()
  }, [value, adjustHeight])

  // Handle input change
  const handleChange = useCallback(
    (e: React.ChangeEvent<HTMLTextAreaElement>) => {
      onChange?.(e.target.value)
    },
    [onChange]
  )

  // Handle key down for submit
  const handleKeyDown = useCallback(
    (e: React.KeyboardEvent<HTMLTextAreaElement>) => {
      // Enter without Shift submits
      if (e.key === 'Enter' && !e.shiftKey) {
        e.preventDefault()
        if (value.trim() && !disabled) {
          onSubmit?.()
        }
      }
    },
    [value, disabled, onSubmit]
  )

  return (
    <textarea
      ref={textareaRef}
      className="input-textarea"
      style={styles.textarea}
      value={value}
      onChange={handleChange}
      onKeyDown={handleKeyDown}
      placeholder={placeholder}
      disabled={disabled}
      rows={minRows}
    />
  )
}

// Styles
const styles: Record<string, React.CSSProperties> = {
  textarea: {
    width: '100%',
    padding: '12px 16px',
    fontSize: '14px',
    lineHeight: '20px',
    color: '#c9d1d9',
    backgroundColor: '#0d1117',
    border: '1px solid #30363d',
    borderRadius: '6px',
    resize: 'none',
    outline: 'none',
    fontFamily: '-apple-system, BlinkMacSystemFont, "Segoe UI", Helvetica, Arial, sans-serif',
    overflow: 'auto',
  },
}

export default InputTextarea
