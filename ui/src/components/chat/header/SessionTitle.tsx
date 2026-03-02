/**
 * SessionTitle Component
 * Editable session title with inline editing.
 * @module components/chat/header/SessionTitle
 */
import React, { useState, useRef, useEffect, useCallback } from 'react'

export interface SessionTitleProps {
  /** Current title */
  title: string
  /** Callback when title is updated */
  onUpdate?: (title: string) => void
  /** Whether editing is enabled */
  editable?: boolean
}

const SessionTitle: React.FC<SessionTitleProps> = ({
  title,
  onUpdate,
  editable = true,
}) => {
  const [isEditing, setIsEditing] = useState(false)
  const [editValue, setEditValue] = useState(title)
  const inputRef = useRef<HTMLInputElement>(null)

  // Sync edit value when title prop changes
  useEffect(() => {
    if (!isEditing) {
      setEditValue(title)
    }
  }, [title, isEditing])

  // Focus input when entering edit mode
  useEffect(() => {
    if (isEditing && inputRef.current) {
      inputRef.current.focus()
      inputRef.current.select()
    }
  }, [isEditing])

  const startEditing = useCallback(() => {
    if (editable && onUpdate) {
      setIsEditing(true)
      setEditValue(title)
    }
  }, [editable, onUpdate, title])

  const saveTitle = useCallback(() => {
    const trimmed = editValue.trim()
    if (trimmed && trimmed !== title) {
      onUpdate?.(trimmed)
    } else {
      setEditValue(title)
    }
    setIsEditing(false)
  }, [editValue, title, onUpdate])

  const cancelEditing = useCallback(() => {
    setEditValue(title)
    setIsEditing(false)
  }, [title])

  const handleKeyDown = useCallback((e: React.KeyboardEvent) => {
    if (e.key === 'Enter') {
      e.preventDefault()
      saveTitle()
    } else if (e.key === 'Escape') {
      cancelEditing()
    }
  }, [saveTitle, cancelEditing])

  const handleBlur = useCallback(() => {
    saveTitle()
  }, [saveTitle])

  if (isEditing) {
    return (
      <input
        ref={inputRef}
        type="text"
        className="session-title__input"
        style={styles.input}
        value={editValue}
        onChange={(e) => setEditValue(e.target.value)}
        onKeyDown={handleKeyDown}
        onBlur={handleBlur}
        maxLength={100}
      />
    )
  }

  return (
    <h1
      className="session-title"
      style={{
        ...styles.title,
        ...(editable && onUpdate ? styles.editable : {}),
      }}
      onClick={startEditing}
      title={editable && onUpdate ? 'Click to edit' : undefined}
    >
      {title}
      {editable && onUpdate && (
        <span className="session-title__edit-icon" style={styles.editIcon}>
          <EditIcon />
        </span>
      )}
    </h1>
  )
}

// Edit icon component
const EditIcon: React.FC = () => (
  <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
    <path d="M17 3a2.85 2.83 0 114 4L7.5 20.5 2 22l1.5-5.5L17 3z" />
  </svg>
)

// Styles
const styles: Record<string, React.CSSProperties> = {
  title: {
    fontSize: '16px',
    fontWeight: 600,
    color: '#c9d1d9',
    margin: 0,
    display: 'flex',
    alignItems: 'center',
    gap: '6px',
    whiteSpace: 'nowrap',
    overflow: 'hidden',
    textOverflow: 'ellipsis',
    maxWidth: '300px',
  },
  editable: {
    cursor: 'pointer',
  },
  editIcon: {
    opacity: 0.5,
    flexShrink: 0,
  },
  input: {
    fontSize: '16px',
    fontWeight: 600,
    color: '#c9d1d9',
    backgroundColor: '#21262d',
    border: '1px solid #58a6ff',
    borderRadius: '4px',
    padding: '4px 8px',
    outline: 'none',
    width: '250px',
    maxWidth: '300px',
  },
}

export default SessionTitle
